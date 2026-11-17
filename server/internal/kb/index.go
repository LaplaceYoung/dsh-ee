package kb

import (
	"context"
	"fmt"
	"math"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode/utf8"
)

// Doc 是进索引的一篇文档的当前版本。
type Doc struct {
	ID      int64
	Base    int64
	Title   string
	Folder  string
	Version int
	Text    string
}

type entry struct {
	doc     *Doc
	chunk   Chunk
	flat    string // Flat(标题路径 + 正文)
	length  int    // 词数，BM25 的长度归一化用
	heading string // Flat(文档标题 + 标题路径)，命中加权
}

// baseIndex 是一个知识库的倒排索引。
type baseIndex struct {
	entries  []*entry
	postings map[string][]int32 // 词 → entries 下标（升序、去重）
	totalLen int
	docs     map[int64]*Doc
}

// Index 是全部知识库的内存索引，按库分开：库有变化时整库重建（库的修订号由调用方比对）。
type Index struct {
	mu    sync.RWMutex
	bases map[int64]*baseIndex
	revs  map[int64]string
}

func NewIndex() *Index {
	return &Index{bases: map[int64]*baseIndex{}, revs: map[int64]string{}}
}

// Revision 是这个库在索引里的修订号；没加载过返回空。
func (ix *Index) Revision(base int64) string {
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	return ix.revs[base]
}

// Load 用一个库的全部文档替换它的索引。
func (ix *Index) Load(base int64, rev string, docs []*Doc) {
	bi := &baseIndex{postings: map[string][]int32{}, docs: map[int64]*Doc{}}
	for _, d := range docs {
		bi.docs[d.ID] = d
		for _, c := range Split(d.Text) {
			body := withoutNotices(c.Text)
			e := &entry{doc: d, chunk: c, flat: Flat(c.Heading + "\n" + body), heading: Flat(d.Title + c.Heading)}
			id := int32(len(bi.entries))
			bi.entries = append(bi.entries, e)
			toks := Tokens(d.Title + " " + c.Heading + " " + body)
			e.length = len(toks)
			bi.totalLen += e.length
			seen := map[string]bool{}
			for _, t := range toks {
				if !seen[t] {
					seen[t] = true
					bi.postings[t] = append(bi.postings[t], id)
				}
			}
		}
	}
	ix.mu.Lock()
	ix.bases[base], ix.revs[base] = bi, rev
	ix.mu.Unlock()
}

// withoutNotices 去掉转换时写进去的提示（「> ⚠ 第 2 页是扫描页……」），它们不是文档内容，不该被搜到。
func withoutNotices(text string) string {
	if !strings.Contains(text, "> ⚠") {
		return text
	}
	lines := Lines(text)
	kept := lines[:0:0]
	for _, l := range lines {
		if !strings.HasPrefix(strings.TrimSpace(l), "> ⚠") {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// Drop 把一个库移出索引。
func (ix *Index) Drop(base int64) {
	ix.mu.Lock()
	delete(ix.bases, base)
	delete(ix.revs, base)
	ix.mu.Unlock()
}

// Query 是一次检索。
type Query struct {
	// 空格分开的多个词；mode 为 phrase 时整句当一个词，为 regex 时是正则表达式
	Text  string
	Mode  string // any（默认）/ all / phrase / regex
	Bases []int64
	// 只搜这些目录（前缀匹配）；空表示不限
	Folder string
	// 只搜这几篇文档；空表示不限。每篇都要在 Bases 里，否则报错
	Docs  []int64
	Limit int
	// 按扩展字段的访问规则过滤：返回 false 的文档在算分和截取片段之前就去掉（docs/access-control.md §5.2）。nil 表示不过滤
	Allow func(base, doc int64) bool
}

// Hit 是一个命中的段。
type Hit struct {
	Base    int64    `json:"kb_id"`
	Doc     int64    `json:"doc"`
	Title   string   `json:"title"`
	Folder  string   `json:"folder,omitempty"`
	Version int      `json:"version"`
	Heading string   `json:"heading"`
	From    int      `json:"from"`
	To      int      `json:"to"`
	Line    int      `json:"line"`
	Snippet string   `json:"snippet"`
	Matched []string `json:"matched"`
	Score   float64  `json:"score"`
}

// Result 是检索结果：命中的段，以及每个词各命中了多少段（帮模型判断要不要换词）。
type Result struct {
	Hits      []Hit          `json:"hits"`
	TermHits  map[string]int `json:"term_hits"`
	Total     int            `json:"total"`
	Truncated bool           `json:"truncated,omitempty"`
	// 标题对得上的文档（检索词像在找某一篇时最有用）
	Docs []DocHit `json:"docs,omitempty"`
	// 给模型的说明，例如正则写错了、已按普通文字匹配
	Note string `json:"note,omitempty"`
}

// DocHit 是一篇标题对得上的文档。
type DocHit struct {
	Base    int64  `json:"kb_id"`
	Doc     int64  `json:"doc"`
	Title   string `json:"title"`
	Folder  string `json:"folder,omitempty"`
	Version int    `json:"version"`
	Lines   int    `json:"lines"`
}

// 标题对得上的文档最多列几篇
const maxDocHits = 5

const (
	bm25K1 = 1.2
	bm25B  = 0.75
)

// Terms 把检索文字切成词。
func Terms(q Query) []string {
	if q.Mode == "phrase" {
		if t := strings.TrimSpace(q.Text); t != "" {
			return []string{t}
		}
		return nil
	}
	var out []string
	for _, t := range strings.Fields(q.Text) {
		if Flat(t) != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
		if len(out) == 12 {
			break
		}
	}
	return out
}

// candidates 是可能含有这个词的段：词里所有索引词的倒排表求交。
// 单个汉字没有对应的索引词（索引按两字切），这种词退回全部段，再逐段核对。
func (bi *baseIndex) candidates(term string) []int32 {
	var lists [][]int32
	for _, t := range Tokens(term) {
		if utf8.RuneCountInString(t) == 1 && isCJK([]rune(t)[0]) {
			continue
		}
		p, ok := bi.postings[t]
		if !ok {
			return nil
		}
		lists = append(lists, p)
	}
	if len(lists) == 0 {
		all := make([]int32, len(bi.entries))
		for i := range all {
			all[i] = int32(i)
		}
		return all
	}
	sort.Slice(lists, func(i, j int) bool { return len(lists[i]) < len(lists[j]) })
	out := slices.Clone(lists[0])
	for _, l := range lists[1:] {
		out = intersect(out, l)
		if len(out) == 0 {
			break
		}
	}
	return out
}

func intersect(a, b []int32) []int32 {
	out := a[:0]
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		switch {
		case a[i] == b[j]:
			out = append(out, a[i])
			i++
			j++
		case a[i] < b[j]:
			i++
		default:
			j++
		}
	}
	return out
}

func docInFolder(d *Doc, folder string) bool {
	return folder == "" || d.Folder == folder || strings.HasPrefix(d.Folder, strings.TrimSuffix(folder, "/")+"/")
}

// scope 是一次检索的文档范围（目录和指定文档）。
type scope struct {
	folder string
	docs   map[int64]bool
	allow  func(base, doc int64) bool
}

func (sc scope) has(d *Doc) bool {
	return docInFolder(d, sc.folder) && (sc.docs == nil || sc.docs[d.ID]) && (sc.allow == nil || sc.allow(d.Base, d.ID))
}

// scopeOf 检查指定的文档都在所选的库里。
func (ix *Index) scopeOf(q Query) (scope, error) {
	sc := scope{folder: q.Folder, allow: q.Allow}
	if len(q.Docs) == 0 {
		return sc, nil
	}
	sc.docs = map[int64]bool{}
	for _, id := range q.Docs {
		found := false
		for _, base := range q.Bases {
			if bi := ix.bases[base]; bi != nil && bi.docs[id] != nil && (q.Allow == nil || q.Allow(base, id)) {
				found = true
				break
			}
		}
		if !found {
			return sc, &QueryError{fmt.Sprintf("没有文档 %d，或它不在所选的知识库里", id)}
		}
		sc.docs[id] = true
	}
	return sc, nil
}

type scored struct {
	base  int64
	e     *entry
	score float64
	terms []string
	first string
}

// Search 按关键词检索。词在段里要原样连续出现（去掉空白后），排序用 BM25，标题命中加权。
func (ix *Index) Search(ctx context.Context, q Query) (*Result, error) {
	if q.Limit <= 0 {
		q.Limit = 10
	}
	ix.mu.RLock()
	defer ix.mu.RUnlock()
	sc, err := ix.scopeOf(q)
	if err != nil {
		return nil, err
	}
	if q.Mode == "regex" {
		return ix.searchRegex(ctx, q, sc)
	}
	terms := Terms(q)
	res := &Result{Hits: []Hit{}, TermHits: map[string]int{}}
	if len(terms) == 0 {
		return res, nil
	}
	res.Docs = ix.titleHits(q, sc, terms)
	var all []*scored
	for _, base := range q.Bases {
		bi := ix.bases[base]
		if bi == nil || len(bi.entries) == 0 {
			continue
		}
		n := float64(len(bi.entries))
		avg := float64(bi.totalLen) / n
		byEntry := map[int32]*scored{}
		for _, term := range terms {
			ft := Flat(term)
			type tf struct {
				id    int32
				count int
			}
			var found []tf
			for _, id := range bi.candidates(term) {
				if ctx.Err() != nil {
					return nil, ctx.Err()
				}
				e := bi.entries[id]
				if !sc.has(e.doc) {
					continue
				}
				if c := strings.Count(e.flat, ft); c > 0 {
					found = append(found, tf{id, c})
				}
			}
			res.TermHits[term] += len(found)
			df := float64(len(found))
			idf := math.Log(1 + (n-df+0.5)/(df+0.5))
			// 词越长越具体，权重略高
			weight := idf * (1 + 0.15*math.Min(float64(utf8.RuneCountInString(ft))-1, 6))
			for _, f := range found {
				e := bi.entries[f.id]
				tfv := float64(f.count)
				s := weight * tfv * (bm25K1 + 1) / (tfv + bm25K1*(1-bm25B+bm25B*float64(e.length)/avg))
				if strings.Contains(e.heading, ft) {
					s *= 1.6
				}
				sc := byEntry[f.id]
				if sc == nil {
					sc = &scored{base: base, e: e, first: term}
					byEntry[f.id] = sc
				}
				sc.score += s
				sc.terms = append(sc.terms, term)
			}
		}
		for _, sc := range byEntry {
			if q.Mode == "all" && len(sc.terms) < len(terms) {
				continue
			}
			// 多个词同时命中的段排在前面
			sc.score *= 1 + 0.5*float64(len(sc.terms)-1)
			all = append(all, sc)
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		if all[i].e.doc.ID != all[j].e.doc.ID {
			return all[i].e.doc.ID < all[j].e.doc.ID
		}
		return all[i].e.chunk.Seq < all[j].e.chunk.Seq
	})
	res.Total = len(all)
	if len(all) > q.Limit {
		all, res.Truncated = all[:q.Limit], true
	}
	for _, sc := range all {
		line, snip := snippet(sc.e.chunk, sc.terms)
		res.Hits = append(res.Hits, hitOf(sc.base, sc.e, line, snip, sc.terms, sc.score))
	}
	return res, nil
}

func hitOf(base int64, e *entry, line int, snip string, terms []string, score float64) Hit {
	return Hit{Base: base, Doc: e.doc.ID, Title: e.doc.Title, Folder: e.doc.Folder, Version: e.doc.Version, Heading: e.chunk.Heading,
		From: e.chunk.From, To: e.chunk.To, Line: line, Snippet: snip, Matched: terms, Score: math.Round(score*100) / 100}
}

const snippetRunes = 120

// snippet 找第一个命中词所在的行，取它前后一共约 240 字，命中的词用【】标出来。
func snippet(c Chunk, terms []string) (int, string) {
	lines := Lines(c.Text)
	at := -1
	for i, l := range lines {
		fl := Flat(l)
		for _, t := range terms {
			if strings.Contains(fl, Flat(t)) {
				at = i
				break
			}
		}
		if at >= 0 {
			break
		}
	}
	if at < 0 {
		// 词跨行了：从段首开始
		at = 0
	}
	// 以命中行为中心拼一段文字
	var parts []string
	size := 0
	for i := at; i < len(lines) && size < snippetRunes*2; i++ {
		if t := strings.TrimSpace(lines[i]); t != "" {
			parts = append(parts, t)
			size += utf8.RuneCountInString(t)
		}
	}
	if at > 0 && size < snippetRunes {
		if t := strings.TrimSpace(lines[at-1]); t != "" {
			parts = append([]string{t}, parts...)
		}
	}
	text := strings.Join(parts, " ")
	if r := []rune(text); len(r) > snippetRunes*2 {
		text = string(r[:snippetRunes*2]) + "…"
	}
	// 命中的是表格里的一行：前面带上表头，不用再读一次就知道每列是什么
	if head := tableHeader(lines, at); head != "" {
		text = head + " … " + text
	}
	for _, t := range terms {
		if t = strings.TrimSpace(t); t != "" && !strings.ContainsAny(t, "【】") {
			text = replaceFold(text, t)
		}
	}
	return c.From + at, text
}

var tableSep = regexp.MustCompile(`^\|(\s*:?-{3,}:?\s*\|)+$`)

// tableHeader 找第 at 行所在表格的表头行；第 at 行不是表格的数据行时返回空。
func tableHeader(lines []string, at int) string {
	if at >= len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[at]), "|") || tableSep.MatchString(strings.TrimSpace(lines[at])) {
		return ""
	}
	for i := at; i > 0; i-- {
		l := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(l, "|") {
			return ""
		}
		if tableSep.MatchString(l) {
			return strings.TrimSpace(lines[i-1])
		}
	}
	return ""
}

// replaceFold 不区分大小写地把 term 标成【term】（原样保留原文的写法）。
func replaceFold(text, term string) string {
	lt, lterm := strings.ToLower(text), strings.ToLower(term)
	if len(lt) != len(text) || lterm == "" {
		return text
	}
	var b strings.Builder
	i := 0
	for {
		j := strings.Index(lt[i:], lterm)
		if j < 0 {
			b.WriteString(text[i:])
			return b.String()
		}
		b.WriteString(text[i : i+j])
		b.WriteString("【" + text[i+j:i+j+len(lterm)] + "】")
		i += j + len(lterm)
	}
}

// titleHits 找标题对得上的文档：检索词都在标题里，或者整个标题出现在检索文字里（「差旅管理办法 住宿」）。
func (ix *Index) titleHits(q Query, sc scope, terms []string) []DocHit {
	whole := Flat(q.Text)
	type cand struct {
		d     *Doc
		base  int64
		exact bool
		runes int
	}
	var cands []cand
	for _, base := range q.Bases {
		bi := ix.bases[base]
		if bi == nil {
			continue
		}
		for _, d := range bi.docs {
			if !sc.has(d) {
				continue
			}
			ft := Flat(d.Title)
			n := utf8.RuneCountInString(ft)
			if n >= 4 && strings.Contains(whole, ft) {
				cands = append(cands, cand{d, base, true, n})
				continue
			}
			all, size := true, 0
			for _, t := range terms {
				f := Flat(t)
				size += utf8.RuneCountInString(f)
				if !strings.Contains(ft, f) {
					all = false
					break
				}
			}
			if all && size >= 2 {
				cands = append(cands, cand{d, base, false, n})
			}
		}
	}
	// 整个标题被提到的排前面，其次标题短的（更像就是要找的那篇）
	sort.Slice(cands, func(i, j int) bool {
		a, b := cands[i], cands[j]
		if a.exact != b.exact {
			return a.exact
		}
		if a.exact && a.runes != b.runes {
			return a.runes > b.runes
		}
		if a.runes != b.runes {
			return a.runes < b.runes
		}
		return a.d.ID < b.d.ID
	})
	var out []DocHit
	for _, c := range cands[:min(len(cands), maxDocHits)] {
		out = append(out, DocHit{Base: c.base, Doc: c.d.ID, Title: c.d.Title, Folder: c.d.Folder, Version: c.d.Version, Lines: len(Lines(strings.TrimRight(c.d.Text, "\n")))})
	}
	return out
}

// searchRegex 在原文里逐行匹配正则，用来查条款号、金额、型号这类精确内容。
// 先从正则里推出一定会出现的词，用索引缩小候选段，再逐行核对；推不出来时逐段扫。
// 正则写错了就当普通文字匹配。
func (ix *Index) searchRegex(ctx context.Context, q Query, sc scope) (*Result, error) {
	res := &Result{Hits: []Hit{}, TermHits: map[string]int{}}
	expr := q.Text
	re, err := regexp.Compile("(?i)" + expr)
	if err != nil {
		expr = regexp.QuoteMeta(q.Text)
		re = regexp.MustCompile("(?i)" + expr)
		res.Note = "正则表达式写得不对（" + err.Error() + "），已按普通文字匹配"
	}
	nd := regexNeed("(?i)" + expr)
	for _, base := range q.Bases {
		bi := ix.bases[base]
		if bi == nil {
			continue
		}
		ids, narrowed := bi.match(nd)
		n := len(bi.entries)
		if narrowed {
			n = len(ids)
		}
		for k := 0; k < n; k++ {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			e := bi.entries[k]
			if narrowed {
				e = bi.entries[ids[k]]
			}
			if !sc.has(e.doc) {
				continue
			}
			for i, l := range Lines(e.chunk.Text) {
				if loc := re.FindStringIndex(l); loc != nil {
					res.Total++
					if len(res.Hits) < q.Limit {
						snip := strings.TrimSpace(l)
						if r := []rune(snip); len(r) > snippetRunes*2 {
							snip = string(r[:snippetRunes*2]) + "…"
						}
						res.Hits = append(res.Hits, hitOf(base, e, e.chunk.From+i, snip, []string{l[loc[0]:loc[1]]}, 0))
					}
				}
			}
		}
	}
	res.TermHits[q.Text] = res.Total
	res.Truncated = res.Total > len(res.Hits)
	return res, nil
}

// QueryError 是检索参数的问题，原样告诉模型。
type QueryError struct{ Msg string }

func (e *QueryError) Error() string { return e.Msg }

// VerifyQuote 判断摘句是否出现在正文的 from～to 行里（两边各放宽 2 行）。
// 摘句里的「…」「...」当作省略，分成几截按顺序都能找到就算对得上。
func VerifyQuote(text string, from, to int, quote string) bool {
	lines := Lines(text)
	if from < 1 {
		from = 1
	}
	if to < from {
		to = from
	}
	lo, hi := max(from-2, 1), min(to+2, len(lines))
	if lo > hi {
		return false
	}
	hay := Loose(strings.Join(lines[lo-1:hi], "\n"))
	quote = strings.ReplaceAll(quote, "...", "…")
	var parts []string
	for _, p := range strings.Split(quote, "…") {
		if lp := Loose(p); lp != "" {
			parts = append(parts, lp)
		}
	}
	total := 0
	for _, p := range parts {
		total += utf8.RuneCountInString(p)
	}
	if total < 2 {
		return false
	}
	pos := 0
	for _, p := range parts {
		i := strings.Index(hay[pos:], p)
		if i < 0 {
			return false
		}
		pos += i + len(p)
	}
	return true
}
