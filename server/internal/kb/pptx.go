package kb

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strconv"
	"strings"
)

// xnode 是一个很小的 XML 树，PPT 的形状要按位置排序，流式解析不方便。
type xnode struct {
	name  string
	attrs []xml.Attr
	kids  []*xnode
	text  strings.Builder
}

func (n *xnode) attr(name string) string {
	for _, a := range n.attrs {
		if a.Name.Local == name {
			return a.Value
		}
	}
	return ""
}

func (n *xnode) child(name string) *xnode {
	for _, k := range n.kids {
		if k.name == name {
			return k
		}
	}
	return nil
}

// find 按名字一层层往下找（只取每层第一个）。
func (n *xnode) find(names ...string) *xnode {
	cur := n
	for _, name := range names {
		if cur = cur.child(name); cur == nil {
			return nil
		}
	}
	return cur
}

func (n *xnode) all(name string, out *[]*xnode) {
	for _, k := range n.kids {
		if k.name == name {
			*out = append(*out, k)
		} else {
			k.all(name, out)
		}
	}
}

// maxXNodes 是一个 XML 文件最多建多少个节点。压缩过的 XML 几 KB 就能有上百万个空元素，树比文件大得多；
// 正常的一页幻灯片只有几百到几万个。
const maxXNodes = 500000

// parseXNode 建一棵小 XML 树，返回树和最外层元素个数（后面按它记预算）。
// ctx 用来在解析大 XML 时响应取消/超时：一个 64 MB 的 XML 可以有上百万个 token。
func parseXNode(ctx context.Context, data []byte) (*xnode, int, error) {
	dec := xml.NewDecoder(bytes.NewReader(data))
	root := &xnode{}
	stack := []*xnode{root}
	nodes, tokens := 0, 0
	for {
		if tokens++; tokens&(ctxCheckEvery-1) == 0 {
			if err := checkCtx(ctx); err != nil {
				return nil, 0, err
			}
		}
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, 0, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if nodes++; nodes > maxXNodes {
				return nil, 0, fmt.Errorf("元素超过 %d 个", maxXNodes)
			}
			n := &xnode{name: t.Name.Local, attrs: t.Attr}
			top := stack[len(stack)-1]
			top.kids = append(top.kids, n)
			stack = append(stack, n)
		case xml.EndElement:
			if len(stack) > 1 {
				stack = stack[:len(stack)-1]
			}
		case xml.CharData:
			if top := stack[len(stack)-1]; top.name == "t" {
				top.text.Write(t)
			}
		}
	}
	return root, nodes, nil
}

// paraText 是一个 a:p 段落的文字（a:t 拼起来，a:br 换成空格）。
func paraText(p *xnode) string {
	var b strings.Builder
	var walk func(n *xnode)
	walk = func(n *xnode) {
		for _, k := range n.kids {
			switch k.name {
			case "t":
				b.WriteString(k.text.String())
			case "br":
				b.WriteString(" ")
			default:
				walk(k)
			}
		}
	}
	walk(p)
	return strings.TrimSpace(b.String())
}

// bodyLines 是一个 txBody 里的段落，空段落去掉。
func bodyLines(body *xnode) []string {
	var paras []*xnode
	body.all("p", &paras)
	var out []string
	for _, p := range paras {
		if t := paraText(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}

type pptShape struct {
	x, y  int64
	title bool
	lines []string
	table [][]string
}

func shapeOffset(sp *xnode) (int64, int64) {
	for _, pr := range []string{"spPr", "xfrm", "grpSpPr"} {
		n := sp.child(pr)
		if n == nil {
			continue
		}
		if pr != "xfrm" {
			n = n.child("xfrm")
		}
		if n != nil {
			if off := n.child("off"); off != nil {
				x, _ := strconv.ParseInt(off.attr("x"), 10, 64)
				y, _ := strconv.ParseInt(off.attr("y"), 10, 64)
				return x, y
			}
		}
	}
	return 0, 0
}

// slideShapes 取一页（或一页备注）上的文字框和表格。组合形状按它自己的位置排，里面的形状按原顺序。
func slideShapes(tree *xnode) []pptShape {
	var out []pptShape
	var walk func(n *xnode, inGroup bool, gx, gy int64)
	walk = func(n *xnode, inGroup bool, gx, gy int64) {
		for _, k := range n.kids {
			x, y := shapeOffset(k)
			if inGroup {
				x, y = gx, gy
			}
			switch k.name {
			case "sp":
				body := k.child("txBody")
				if body == nil {
					continue
				}
				ph := k.find("nvSpPr", "nvPr", "ph")
				typ := ""
				if ph != nil {
					typ = ph.attr("type")
				}
				if typ == "sldNum" || typ == "dt" || typ == "ftr" {
					continue
				}
				if lines := bodyLines(body); len(lines) > 0 {
					out = append(out, pptShape{x: x, y: y, title: typ == "title" || typ == "ctrTitle", lines: lines})
				}
			case "graphicFrame":
				var tbls []*xnode
				k.all("tbl", &tbls)
				for _, tbl := range tbls {
					var rows [][]string
					for _, tr := range tbl.kids {
						if tr.name != "tr" {
							continue
						}
						var row []string
						for _, tc := range tr.kids {
							if tc.name != "tc" {
								continue
							}
							cell := ""
							if body := tc.child("txBody"); body != nil {
								cell = cellText(strings.Join(bodyLines(body), " "))
							}
							row = append(row, cell)
						}
						rows = append(rows, row)
					}
					if len(rows) > 0 {
						out = append(out, pptShape{x: x, y: y, table: rows})
					}
				}
			case "grpSp":
				walk(k, true, x, y)
			}
		}
	}
	// 根元素是 p:sld（幻灯片）或 p:notes（备注页）
	if len(tree.kids) > 0 {
		if sp := tree.kids[0].find("cSld", "spTree"); sp != nil {
			walk(sp, false, 0, 0)
		}
	}
	// 从上到下、从左到右；同一组合里的保持原顺序
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].y != out[j].y {
			return out[i].y < out[j].y
		}
		return out[i].x < out[j].x
	})
	return out
}

// pptRels 读一个 .rels 文件：关系 id → 目标路径（已按所在目录展开），以及按类型找到的第一个目标。
func pptRels(files map[string]*zip.File, relsPath, baseDir string) (map[string]string, map[string]string) {
	byID, byType := map[string]string{}, map[string]string{}
	f := files[relsPath]
	if f == nil {
		return byID, byType
	}
	data, err := readZip(f)
	if err != nil {
		return byID, byType
	}
	var rels struct {
		Rels []struct {
			ID     string `xml:"Id,attr"`
			Type   string `xml:"Type,attr"`
			Target string `xml:"Target,attr"`
		} `xml:"Relationship"`
	}
	if xml.Unmarshal(data, &rels) != nil {
		return byID, byType
	}
	for _, r := range rels.Rels {
		target := path.Clean(path.Join(baseDir, r.Target))
		if strings.HasPrefix(r.Target, "/") {
			target = strings.TrimPrefix(r.Target, "/")
		}
		byID[r.ID] = target
		typ := path.Base(r.Type)
		if _, ok := byType[typ]; !ok {
			byType[typ] = target
		}
	}
	return byID, byType
}

// maxSlides 是一份 PPT 最多收多少页（按 sldIdLst 里的引用条数算，和转换出来的「第 N 页」一致）。
const maxSlides = 5000

// maxPPTXNodeCost 是「解析出来的 XML 节点数 × 引用次数」的预算。
// 一页幻灯片可以被 sldIdLst 引用很多次：以前每次引用都重新解压一遍、再建一遍 XML 树，4 KB 的 pptx
// 就能让一次转换跑十几分钟。现在同一页只解压/解析/排版一次（结果按幻灯片路径缓存），后面每次引用
// 只是把缓存正文写出去；这里再按真实成本（节点数 × 引用次数）记一笔账，超了整份拒收。
// 正常文件：几百页、每页几百到几千个节点，离 8M 很远；400 KB 的「一页 50 万节点 × 引用 5000 次」会被拦下。
const maxPPTXNodeCost = 8 << 20

// pptSlide 是一页幻灯片渲染好的结果。同一路径的幻灯片在 sldIdLst 里可能出现很多次，
// 解压、解析、形状排序、备注解析都只做一次，后面每次引用只写缓存正文。
type pptSlide struct {
	missing    bool // rels 指向的 slide 文件不存在（跟原来一样跳过）
	hidden     bool // sld show="0"，不输出
	nodes      int  // 解析出的 XML 节点数（幻灯片 + 备注），用于 maxPPTXNodeCost 预算
	title      string
	body       string // 形状正文 + 备注，按原来的顺序排好
	tables     int
	tableCells int
	hasNotes   bool
}

// renderPPTXSlide 解压并解析一页幻灯片，把正文排版好缓存下来。
func renderPPTXSlide(ctx context.Context, files map[string]*zip.File, slidePath string) (*pptSlide, error) {
	f := files[slidePath]
	if f == nil {
		return &pptSlide{missing: true}, nil
	}
	sdata, err := readZip(f)
	if err != nil {
		return nil, err
	}
	tree, nodes, err := parseXNode(ctx, sdata)
	if err != nil {
		return nil, errors.New(".pptx 内容解析失败：" + err.Error())
	}
	sr := &pptSlide{nodes: nodes}
	if sld := tree.child("sld"); sld != nil && sld.attr("show") == "0" {
		sr.hidden = true
		return sr, nil
	}
	shapes := slideShapes(tree)
	for k, sh := range shapes {
		if sh.title {
			sr.title = strings.Join(sh.lines, " ")
			shapes = append(shapes[:k:k], shapes[k+1:]...)
			break
		}
	}
	var b strings.Builder
	cells := 0
	for _, sh := range shapes {
		if sh.table != nil {
			width := 0
			for _, r := range sh.table {
				width = max(width, len(r))
			}
			if cells += len(sh.table) * width; cells > maxTableCells {
				return nil, errTooBig
			}
			b.WriteString(markdownTable(sh.table) + "\n")
			sr.tables++
			if b.Len() > maxOutput {
				return nil, errTooBig
			}
			continue
		}
		b.WriteString(strings.Join(sh.lines, "\n") + "\n\n")
		if b.Len() > maxOutput {
			return nil, errTooBig
		}
	}
	sr.tableCells = cells
	// 备注（讲者笔记）：原来每引用一次就读一遍 rels、解压、解析一遍备注页，现在跟着这页一起缓存。
	_, byType := pptRels(files, path.Join(path.Dir(slidePath), "_rels", path.Base(slidePath)+".rels"), path.Dir(slidePath))
	if np := byType["notesSlide"]; files[np] != nil {
		if ndata, err := readZip(files[np]); err == nil {
			if nt, notesNodes, err := parseXNode(ctx, ndata); err == nil {
				sr.nodes += notesNodes
				var notes []string
				for _, sh := range slideShapes(nt) {
					notes = append(notes, sh.lines...)
				}
				if len(notes) > 0 {
					sr.hasNotes = true
					b.WriteString("备注：" + strings.Join(notes, " ") + "\n\n")
				}
			}
		}
	}
	if b.Len() > maxOutput {
		return nil, errTooBig
	}
	sr.body = b.String()
	return sr, nil
}

// convertPPTX 按页抽取：每页一节「## 第 N 页 标题」，文字框从上到下、从左到右，表格转成 Markdown 表格，备注放在最后。
// 图片、图表、SmartArt 不收；隐藏的页跳过。
func convertPPTX(ctx context.Context, data []byte) (*Converted, error) {
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, errors.New("不是有效的 .pptx 文件")
	}
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	pres := files["ppt/presentation.xml"]
	if pres == nil {
		return nil, errors.New("不是有效的 .pptx 文件：缺少 ppt/presentation.xml")
	}
	pdata, err := readZip(pres)
	if err != nil {
		return nil, err
	}
	var p struct {
		Slides []struct {
			RID string `xml:"http://schemas.openxmlformats.org/officeDocument/2006/relationships id,attr"`
		} `xml:"sldIdLst>sldId"`
	}
	if err := xml.Unmarshal(pdata, &p); err != nil {
		return nil, errors.New(".pptx 内容解析失败：" + err.Error())
	}
	if err := checkCtx(ctx); err != nil {
		return nil, err
	}
	if len(p.Slides) > maxSlides {
		return nil, fmt.Errorf("幻灯片超过 %d 页，请拆成几个文件再上传", maxSlides)
	}
	rels, _ := pptRels(files, "ppt/_rels/presentation.xml.rels", "ppt")
	info := Info{Format: "pptx"}
	var b strings.Builder
	cells, cost := 0, 0
	cache := map[string]*pptSlide{}
	for i, s := range p.Slides {
		if err := checkCtx(ctx); err != nil {
			return nil, err
		}
		// 同一页可以被引用很多次，每次都会再写一遍
		if b.Len() > maxOutput {
			return nil, errTooBig
		}
		slidePath := rels[s.RID]
		sr, ok := cache[slidePath]
		if !ok {
			if sr, err = renderPPTXSlide(ctx, files, slidePath); err != nil {
				return nil, err
			}
			cache[slidePath] = sr
		}
		// 预算按真实成本算：节点数 × 引用次数。解析只做一次，但重复引用仍要在这里记账。
		cost += sr.nodes
		if cost > maxPPTXNodeCost {
			return nil, fmt.Errorf(".pptx 内容太多（幻灯片重复引用太多或单页太复杂），请拆成几个文件再上传")
		}
		if sr.missing || sr.hidden {
			continue
		}
		info.Pages++
		if cells += sr.tableCells; cells > maxTableCells {
			return nil, errTooBig
		}
		info.Tables += sr.tables
		if sr.hasNotes {
			info.Notes++
		}
		fmt.Fprintf(&b, "## 第 %d 页", i+1)
		if sr.title != "" {
			b.WriteString(" " + sr.title)
		}
		b.WriteString("\n\n")
		b.WriteString(sr.body)
		if b.Len() > maxOutput {
			return nil, errTooBig
		}
	}
	// 最后一页没有下一轮循环可以检查，收尾时再确认一次
	if b.Len() > maxOutput {
		return nil, errTooBig
	}
	return &Converted{Markdown: b.String(), Mime: "application/vnd.openxmlformats-officedocument.presentationml.presentation", Info: info}, nil
}
