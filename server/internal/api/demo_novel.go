package api

import (
	"encoding/json"
	"time"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例场景「山海工作室」：四个人连载长篇奇幻小说《雾港纪事》的小团队。没有业务系统，
// 用的是知识库（设定集、写作规范）、专家（审稿）和自带数据表的应用（章节进度）。设定见 docs/demo-company.md。
// 小说的书名、人物和设定都是为示例编的。

const demoNovelDept = "山海工作室"

var demoNovelUsers = []demoUser{
	{"demo.shen.mo", "沈墨", demoNovelDept, "主笔"},
	{"demo.gu.qing", "顾青", demoNovelDept, "责任编辑"},
	{"demo.ning.yuan", "宁远", demoNovelDept, "设定与校对"},
	{"demo.tang.xiaoman", "唐小满", demoNovelDept, "连载运营"},
}

var demoNovelKBases = []demoKBase{
	{
		Slug: "novel-bible", Title: "《雾港纪事》设定集（示例）",
		Description: "山海工作室：世界观、人物小传、已发生的事件和还没回收的伏笔。写新章节前先查这里。示例内容，都是编的。",
		Docs: []demoKDoc{
			{Title: "世界观与引雾", Folder: "设定", Markdown: demoNovelWorld},
			{Title: "人物小传", Folder: "设定", Markdown: demoNovelCast},
			{Title: "时间线（第 1～11 章）", Folder: "剧情", Markdown: demoNovelTimeline},
		},
	},
	{
		Slug: "novel-style", Title: "写作与连载规范（示例）",
		Description: "山海工作室：文风、用词禁忌、每章的结构，以及交稿、审稿、更新的流程。示例内容。",
		Docs: []demoKDoc{
			{Title: "文风与用词", Folder: "规范", Markdown: demoNovelStyle},
			{Title: "连载流程", Folder: "流程", Markdown: demoNovelFlow},
		},
	},
}

func demoNovelGuide() []demoGuideStep {
	return demoGuideSteps(demoScenarioNovel, demoNovelUsers, [][4]string{
		{"demo.shen.mo", "story-bible", "设定问答", "林栖一天能引雾几次？第三次之后会怎样？"},
		{"demo.gu.qing", "chapter-board", "章节进度", "这周还有哪几章没交稿？"},
	})
}

func demoNovelAssets() []demoAsset {
	invocable := true
	return []demoAsset{
		{
			Kind: store.KindSkill, Slug: "chapter-recap", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Title:          "章节梗概（示例）",
				Description:    "把一章正文压成梗概：发生了什么、谁在场、埋了或回收了哪些伏笔，方便更新设定集和时间线。",
				WhenToUse:      "用户贴了一章或一段小说正文，想要梗概、人物出场表或伏笔清单时",
				ModelInvocable: &invocable,
				Body:           demoNovelRecapSkill,
			},
		},
		{
			Kind: store.KindApp, Slug: "story-bible", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Title:       "设定问答",
				Description: "写到一半拿不准设定，在这里问：人物、地点、引雾的规则、前面发生过什么，答案都带设定集的出处。",
				Highlights:  []string{"问人物、地点、能力规则，答案带设定集出处", "设定集里没有的，直接说没有，不替你编", "贴一段正文，帮你对一遍设定"},
				Icon:        "📖",
				Persona:     demoNovelBiblePersona,
				Starters: []string{
					"林栖一天能引雾几次？第三次之后会怎样？",
					"陆沉舟知不知道白焰的秘密？是在第几章说的？",
					"铜罗盘现在在谁手里？",
					"还有哪些伏笔没回收？",
				},
				Knowledge: []string{"novel-bible", "novel-style"},
				Skills:    []string{"chapter-recap"},
				Web:       true,
				Workbench: &appWorkbench{Panes: []wbPane{{ID: "ask", Type: "panel", Title: "设定问答", Widgets: []wbWidget{
					{Type: "markdown", Title: "《雾港纪事》设定问答", Width: "full", Text: "这里的回答只依据《设定集》和《写作与连载规范》，点答案里的出处能看原文。\n设定集里没写的，会直接告诉你没有：那是要你们自己定的地方，定了以后记得补进设定集。"},
					{Type: "actions", Title: "常问", Width: "half", Actions: []wbAction{
						{Label: "引雾的规则", Prompt: "把引雾的规则和限制完整列一遍，标出处。"},
						{Label: "人物速查", Prompt: "列出主要人物：年龄、身份、和林栖的关系、不能写错的细节。"},
						{Label: "没回收的伏笔", Prompt: "到第 11 章为止，还有哪些伏笔没回收？分别埋在第几章？"},
						{Label: "每章的写法", Prompt: "按写作规范，一章该多长、用什么视角、结尾怎么收？"},
					}},
					{Type: "form", Title: "问设定", Width: "half", Fields: []wbField{{Name: "q", Label: "问题", Kind: "text", Required: true}},
						Submit: &wbAction{Label: "问", Prompt: "在设定集里查：{{form.q}}"}},
					{Type: "board", Title: "答案与出处", Width: "full"},
				}}}},
			},
		},
		{
			Kind: store.KindApp, Slug: "continuity-editor", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "设定一致性编辑",
				Description: "把新写的一章对着设定集查一遍：人物、能力规则、时间线有没有和前文冲突，逐条指出来。",
				Icon:        "🧭",
				Expertise:   []string{"设定核对", "时间线", "伏笔"},
				Persona:     "你是山海工作室的设定编辑，负责《雾港纪事》前后不打架。你只指出冲突和依据，不替作者改写情节；语气平实，像一位细心的同事。",
				Method:      demoNovelContinuityMethod,
				Starters: []string{
					"我贴一段第 12 章的草稿，帮我查和设定有没有冲突",
					"让林栖在第 12 章连续引雾四次，会和哪条设定冲突？",
					"第 12 章想回收铜罗盘的伏笔，前面都交代过什么？",
				},
				Knowledge: []string{"novel-bible"},
				Skills:    []string{"chapter-recap"},
			},
		},
		{
			Kind: store.KindApp, Slug: "line-editor", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "文风编辑",
				Description: "按工作室的写作规范看一章：视角有没有跳、用词有没有犯禁、篇幅和章末的钩子合不合要求。",
				Icon:        "✒️",
				Expertise:   []string{"文风", "用词", "章节结构"},
				Persona:     "你是山海工作室的文字编辑。你按《写作与连载规范》提意见，指出具体的句子和改法；不改变情节，也不把作者的句子都改成你的口气。",
				Method:      demoNovelLineMethod,
				Starters: []string{
					"我贴一段正文，帮我按规范看一遍",
					"这一章 5200 字，要不要拆？",
					"章末这句话算不算钩子？",
				},
				Knowledge: []string{"novel-style"},
			},
		},
		{
			Kind: store.KindApp, Slug: "chapter-review", Delivery: store.DeliveryOptional,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "审稿小组",
				Description: "专家团队：设定一致性编辑查设定，文风编辑看文字，组长汇总成一页审稿意见。自选，需要的人自己启用。",
				Icon:        "🧑‍🤝‍🧑",
				Expertise:   []string{"审稿", "返稿意见"},
				Persona:     "你是山海工作室审稿小组的组长。作者交来一章，你让两位成员各看一部分，再汇总成一页返稿意见，让作者知道先改什么。",
				Method:      demoNovelReviewMethod,
				Starters:    []string{"我贴第 12 章的草稿，给我一份完整的审稿意见"},
				Members:     []string{"continuity-editor", "line-editor"},
			},
		},
	}
}

// demoNovelSelfApps：责任编辑顾青做好、分享给全工作室的「章节进度」，演示应用自带数据表。
func demoNovelSelfApps() []demoSelfApp {
	day := func(n int) string { return time.Now().AddDate(0, 0, n).Format("2006-01-02") }
	statuses := []string{"待写", "写作中", "待审", "返稿修改", "已定稿", "已发布"}
	return []demoSelfApp{{Owner: "demo.gu.qing", Everyone: true, demoAsset: demoAsset{Kind: store.KindApp, Slug: "chapter-board", Delivery: store.DeliveryAuto, Spec: assetSpec{
		Title:       "章节进度",
		Description: "责任编辑顾青做的：每一章写到哪了、谁在跟、什么时候交稿，还有没回收的伏笔。全工作室都能看能改，AI 可以帮你查和更新。",
		Icon:        "🗂",
		Web:         true,
		Persona: "你是山海工作室《雾港纪事》的连载助理。章节进度在「章节」表、伏笔在「伏笔」表，都用 app__rows 查，不要凭印象回答。" +
			"每周二、周五 20:00 更新，交稿截止是更新前 48 小时；有人问「哪几章没交稿」时，列出状态是「待写」「写作中」「返稿修改」的章节和各自的交稿日期，过了交稿日期的单独提醒。" +
			"要改状态、登记新章节或新伏笔时，先把要写的内容复述一遍，得到确认后再用 app__write 写。",
		Starters: []string{"这周还有哪几章没交稿？", "第 12 章改成「待审」", "还有哪些伏笔没回收？", "存稿还剩几章？"},
		Tables: []appTable{
			{Name: "chapters", Title: "章节", Visibility: "shared", Fields: []appField{
				{Name: "no", Title: "章号", Kind: "number", Required: true},
				{Name: "title", Title: "章名", Kind: "text", Required: true},
				{Name: "status", Title: "状态", Kind: "select", Options: statuses, Required: true},
				{Name: "words", Title: "字数", Kind: "number"},
				{Name: "due", Title: "交稿日期", Kind: "date"},
				{Name: "note", Title: "备注", Kind: "text"},
			}},
			{Name: "threads", Title: "伏笔", Visibility: "shared", Fields: []appField{
				{Name: "what", Title: "伏笔", Kind: "text", Required: true},
				{Name: "planted", Title: "埋在第几章", Kind: "number", Required: true},
				{Name: "status", Title: "状态", Kind: "select", Options: []string{"未回收", "已回收"}, Required: true},
				{Name: "note", Title: "打算怎么收", Kind: "text"},
			}},
		},
		Workbench: &appWorkbench{Panes: []wbPane{
			{ID: "chapters", Type: "panel", Title: "章节", Widgets: []wbWidget{
				{Type: "chart", Title: "各状态的章节数", Chart: "pie", Source: &wbSource{Table: "chapters", Args: json.RawMessage(`{"group_by":"status"}`)}, Rows: "items", X: "status", Y: []wbColumn{{Field: "count", Title: "章节数"}}, Width: "half"},
				{Type: "form", Title: "登记一章", Width: "half", Submit: &wbAction{Label: "保存", Table: "chapters"}},
				{Type: "table", Title: "全部章节", Source: &wbSource{Table: "chapters"}, Rows: "items", Width: "full",
					Columns: []wbColumn{{Field: "no", Title: "章号"}, {Field: "title", Title: "章名"}, {Field: "status", Title: "状态", Format: "status"}, {Field: "words", Title: "字数"}, {Field: "due", Title: "交稿", Format: "date"}},
					Detail:  []wbColumn{{Field: "note", Title: "备注"}},
					RowActions: []wbAction{
						{Label: "交稿待审", Table: "chapters", Set: map[string]any{"status": "待审"}},
						{Label: "定稿", Table: "chapters", Set: map[string]any{"status": "已定稿"}},
						{Label: "这章要注意什么", Prompt: "我要写第 {{row.no}} 章「{{row.title}}」。按设定集和时间线，这一章要注意哪些设定、有哪些伏笔可以用？"},
					},
					Empty: &wbEmpty{Text: "还没有登记章节"}},
			}},
			{ID: "threads", Type: "panel", Title: "伏笔", Widgets: []wbWidget{
				{Type: "form", Title: "记一条伏笔", Width: "third", Submit: &wbAction{Label: "保存", Table: "threads"}},
				{Type: "table", Title: "伏笔清单", Source: &wbSource{Table: "threads"}, Rows: "items", Width: "wide",
					Columns:    []wbColumn{{Field: "what", Title: "伏笔"}, {Field: "planted", Title: "埋在"}, {Field: "status", Title: "状态", Format: "status"}},
					Detail:     []wbColumn{{Field: "note", Title: "打算怎么收"}},
					RowActions: []wbAction{{Label: "已回收", Table: "threads", Set: map[string]any{"status": "已回收"}}},
					Empty:      &wbEmpty{Text: "还没有记伏笔"}},
			}},
		}},
	}}, Rows: map[string][]demoRow{
		"chapters": {
			{By: "demo.gu.qing", Data: map[string]any{"no": 9, "title": "封港", "status": "已发布", "words": 3620}},
			{By: "demo.gu.qing", Data: map[string]any{"no": 10, "title": "灯塔的门", "status": "已发布", "words": 3480}},
			{By: "demo.gu.qing", Data: map[string]any{"no": 11, "title": "白焰", "status": "已发布", "words": 3910}},
			{By: "demo.shen.mo", Data: map[string]any{"no": 12, "title": "第四次", "status": "返稿修改", "words": 4350, "due": day(-1), "note": "编辑意见：林栖连续引雾四次，和设定冲突；篇幅超了"}},
			{By: "demo.shen.mo", Data: map[string]any{"no": 13, "title": "罗盘指向的地方", "status": "待审", "words": 3540, "due": day(1)}},
			{By: "demo.shen.mo", Data: map[string]any{"no": 14, "title": "下码头的孩子", "status": "写作中", "words": 1800, "due": day(3)}},
			{By: "demo.shen.mo", Data: map[string]any{"no": 15, "title": "季闻的条件", "status": "待写", "due": day(6)}},
			{By: "demo.shen.mo", Data: map[string]any{"no": 16, "title": "雾潮前夜", "status": "待写", "due": day(10)}},
		},
		"threads": {
			{By: "demo.ning.yuan", Data: map[string]any{"what": "铜罗盘的指针不指北，指向雾最浓的地方", "planted": 7, "status": "未回收", "note": "第 13 章带林栖找到父亲的船"}},
			{By: "demo.ning.yuan", Data: map[string]any{"what": "父亲失踪那晚，灯塔的白焰灭过一刻钟", "planted": 4, "status": "未回收", "note": "陆沉舟知情，留到冬至前揭开"}},
			{By: "demo.ning.yuan", Data: map[string]any{"what": "阿柒左耳后有和林栖一样的雾纹", "planted": 9, "status": "未回收"}},
			{By: "demo.ning.yuan", Data: map[string]any{"what": "林栖左手的旧伤是小时候被船钉划的", "planted": 2, "status": "已回收", "note": "第 8 章修船时交代了"}},
		},
	}}}
}

const demoNovelBiblePersona = `你是山海工作室的设定问答助手，帮作者和编辑查《雾港纪事》的设定。

## 回答
- 只依据《设定集》和《写作与连载规范》回答，先查知识库再答，每一条标出处。
- 设定集里没有的，直接说「设定集里没有写」，并提醒这是要团队自己定的地方；不要替作者编设定。
- 问到「第几章发生了什么」时，以《时间线》为准。
- 用户贴了正文想对设定时，逐条列出和设定不一致的地方，各标出处；没发现问题也要说明查了哪几项。

## 看板
每回答完一个问题，把结论和出处写到「答案与出处」看板上。

小说的内容是示例，都是编的。`

const demoNovelRecapSkill = `# 章节梗概

用户贴来一章或一段小说正文时，整理成下面的格式。只写正文里有的，不要补情节。

## 输出格式

**梗概**（150 字以内，按发生的先后写）

**出场人物**
- 人物：这一章做了什么

**地点与时间**：正文里交代的地点；提到时间（第几天、哪个时辰、是不是雾潮）就写上

**伏笔**
- 新埋的：……
- 回收的：……（没有就写「无」）

**要更新设定集的地方**：这一章新定下来的设定（人物的新信息、能力的新规则），逐条列出，方便补进设定集`

const demoNovelContinuityMethod = `## 查一章草稿

1. 先用「章节梗概」技能把草稿理一遍：谁出场、在哪、发生了什么、用了几次引雾。
2. 对着知识库《设定集》逐项查，不要凭印象：
   - **人物**：年龄、身份、外貌细节、会什么不会什么（比如林栖不会游泳、左手有旧伤）
   - **引雾**：一天最多三次、一次不超过一刻钟、第三次之后失声一日、白焰附近引不动
   - **时间线**：和前面章节发生的事对不对得上；人物这时候该知道什么、不该知道什么
   - **地点**：上城、灯塔区、下码头的位置关系和规矩
3. 输出一张表：

| 草稿里怎么写 | 设定怎么写 | 出处 | 严重程度 |
|---|---|---|---|

严重程度分三档：**硬伤**（和已发布的章节或核心规则冲突，必须改）、**要确认**（设定集里没写，作者要定下来）、**小问题**。
4. 最后列「可以用的伏笔」：这一章顺手能回收或呼应的。

## 边界

只指出冲突，不替作者改写情节。作者想改设定也可以，但要提醒：改了以后哪几章要跟着改。`

const demoNovelLineMethod = `## 看一段正文

1. 先查知识库《文风与用词》，按规范一条条对：
   - **视角**：第三人称限知，跟着林栖；一个场景里不换视角
   - **用词**：不用网络流行语；不出现现代的计量单位（用里、丈、刻、时辰）；人名不缩写
   - **篇幅**：一章 3000～4000 字，超过 4500 建议拆
   - **章末**：最后一段要留一个让人想看下一章的钩子
2. 指出具体的句子：引用原句，说问题，给一个改法。一次最多提十条，先提最要紧的。
3. 写得好的地方也点一两处，说明为什么好。

## 边界

不动情节；不把句子全改成一个口气。作者坚持的写法，提一次就够了。`

const demoNovelReviewMethod = `## 审一章

1. **设定**（按「设定一致性编辑」的做法）：对着设定集查人物、引雾规则、时间线，列出硬伤和要确认的地方。
2. **文字**（按「文风编辑」的做法）：按写作规范看视角、用词、篇幅和章末。
3. **汇总成一页返稿意见**：
   - **结论**：可以定稿 / 小改后定稿 / 需要返稿
   - **必须改**：硬伤，按先后列出，每条带出处
   - **建议改**：最多五条
   - **写得好的地方**：一两处
   - **交稿提醒**：按连载流程，这一章最晚什么时候要定稿才赶得上更新

两位成员的意见有冲突时（比如为了守设定要加一段解释，篇幅又超了），把冲突写出来，让作者选。`

const demoNovelWorld = `# 世界观与引雾（示例）

> 山海工作室《雾港纪事》设定集。示例内容，都是为演示编的。

## 雾港

雾港是一座海雾常年不散的港城，分三个区：

| 区 | 位置 | 住的人 |
|---|---|---|
| 上城 | 北面的石坡上，雾最薄 | 港务司、商会和督察署 |
| 灯塔区 | 城东的岬角，白焰灯塔所在 | 灯塔守和少数匠人，夜里不许外人进 |
| 下码头 | 南面的滩涂和栈桥，雾最浓 | 船工、修船匠、走私船 |

从下码头走到灯塔区要大半个时辰；上城到下码头只有一条石阶，叫「千级阶」。

## 雾潮

每月朔日前后三天，海上的雾会漫进全城，叫**雾潮**。雾潮期间港口封航，夜里不点灯的人会迷路。雾潮夜里，引雾者的能力最强。

## 引雾

少数人生来能让雾听话，叫**引雾者**。引雾者耳后有淡青色的纹路，叫雾纹。规则：

1. **一天最多引雾三次**（以日出为界）。
2. **一次不超过一刻钟**，超过会昏厥。
3. **第三次之后会失声一日**，说不出话，直到下一个日出。
4. 没有人能在一天里引第四次雾：前文从未出现过，也不打算写。
5. **白焰附近引不动雾**：灯塔的白焰照得到的地方，雾不听任何人的话。
6. 引雾只能改变雾的浓淡和流向，不能凭空造雾，也不能伤人。

## 白焰

灯塔顶上的白色火焰，能把雾推开，港口靠它才能通航。城里人都以为白焰永远不灭；实际上它每年冬至前会变弱（见《时间线》第 11 章）。

## 钱和计量

- 货币是**银梭**，一枚银梭够一家人吃三天。
- 距离用里、丈，时间用时辰、刻。不用米、公里、小时、分钟。`

const demoNovelCast = `# 人物小传（示例）

> 山海工作室《雾港纪事》设定集。示例内容。

## 林栖（主角）

- 19 岁，下码头修船匠的女儿，引雾者，雾纹在**左耳后**。
- **左手**手背有一道旧伤，小时候被船钉划的（第 2 章埋下，第 8 章交代）。
- **不会游泳**：在码头长大却怕水，母亲是溺水死的。
- 性子倔，话少；着急的时候会咬左手的指节。
- 第 7 章起带着父亲留下的铜罗盘。

## 陆沉舟

- 34 岁，灯塔守，**不是引雾者**。
- 知道白焰会变弱的秘密，第 11 章才告诉林栖。
- 右腿有旧疾，上灯塔的台阶要扶墙。
- 称呼林栖一直是「林姑娘」，没有改过口。

## 阿柒

- 13 岁，走私船上长大的孤儿，在下码头替人跑腿。
- 左耳后也有雾纹，他自己不知道那是什么（第 9 章埋下，还没回收）。
- 识字不多，但认得所有船的旗号。

## 季闻

- 41 岁，上城督察署的督察，第 9 章下令封锁下码头。
- 不是单纯的反派：他封港是因为知道雾潮一年比一年长。
- 从不说谎，但只说一半。

## 林父（林渡）

- 修船匠，第 4 章雾潮夜失踪，留下一只铜罗盘。生死未定。`

const demoNovelTimeline = `# 时间线（第 1～11 章）（示例）

> 山海工作室《雾港纪事》设定集。每发布一章，由宁远更新。示例内容。

| 章 | 章名 | 发生了什么 |
|---|---|---|
| 1 | 雾潮夜 | 雾潮夜里，林栖为了找走失的小船第一次引雾 |
| 2 | 修船匠的女儿 | 交代林家的铺子；林栖左手旧伤第一次出现 |
| 3 | 千级阶 | 林栖上城送货，第一次见到督察季闻 |
| 4 | 灭了一刻钟 | 雾潮夜林父出海未归；当晚白焰灭过一刻钟，只有少数人看见 |
| 5 | 空铺子 | 林栖独自撑起铺子，发现父亲的工具箱被人翻过 |
| 6 | 跑腿的孩子 | 阿柒登场，替人给林栖送来一封没有署名的信 |
| 7 | 铜罗盘 | 林栖在船底夹层里找到父亲留下的铜罗盘，指针不指北 |
| 8 | 船钉 | 林栖修一条走私船，回忆起左手旧伤的来历 |
| 9 | 封港 | 季闻下令封锁下码头；林栖看见阿柒耳后的雾纹 |
| 10 | 灯塔的门 | 林栖夜里闯进灯塔区，被陆沉舟拦下 |
| 11 | 白焰 | 陆沉舟告诉林栖：白焰每年冬至前会变弱，今年可能会灭 |

## 到第 11 章为止，人物知道什么

- 林栖知道白焰会变弱；**不知道**父亲失踪那晚白焰灭过。
- 陆沉舟知道那晚白焰灭过，还没有说。
- 阿柒不知道自己是引雾者。
- 季闻知道雾潮在变长，不知道林栖是引雾者。

## 还没回收的伏笔

1. 铜罗盘的指针指向雾最浓的地方（第 7 章）。
2. 父亲失踪那晚白焰灭过一刻钟（第 4 章）。
3. 阿柒耳后的雾纹（第 9 章）。
4. 第 6 章那封没有署名的信是谁写的。`

const demoNovelStyle = `# 文风与用词（示例）

> 山海工作室写作规范，责任编辑顾青维护。示例内容。

## 视角

- 第三人称限知，跟着林栖写。读者只知道林栖知道的事。
- 一个场景里不换视角。确实要写别人的视角，单独起一章，章名下注明。

## 篇幅与结构

- 一章 **3000～4000 字**。超过 4500 字建议拆成两章。
- 开头三百字内要让读者知道：在哪、谁在场、这一章要解决什么。
- **章末留钩子**：最后一段要有一个没解决的问题或一个新的发现。

## 用词

- 不用网络流行语，不用英文字母缩写。
- 不出现现代的计量单位：用里、丈、时辰、刻，不用米、公里、小时、分钟。
- 人名不缩写：写「陆沉舟」，不写「老陆」「沉舟哥」（人物对话里的称呼按《人物小传》）。
- 少用形容词堆叠；写雾的时候写它怎么动，不写它「神秘」「诡异」。

## 不写的内容

- 不写引雾伤人或杀人（设定里引雾不能伤人）。
- 不让林栖突然会游泳。`

const demoNovelFlow = `# 连载流程（示例）

> 山海工作室写作规范。示例内容。

## 更新

- **每周二、周五 20:00** 更新，一次一章。
- **存稿至少 6 章**：已定稿但还没发布的章节少于 6 章时，暂停加更。
- 要断更，至少**提前 3 天**发公告，由唐小满负责。

## 一章从写到发

1. **交稿**：主笔在更新前 **48 小时**把稿子交给责任编辑，在「章节进度」里把状态改成「待审」。
2. **审稿**：责任编辑 **24 小时内**返稿。先查设定（宁远），再看文字（顾青）。
3. **修改**：有硬伤的必须改；建议改的由主笔决定。
4. **定稿**：责任编辑把状态改成「已定稿」。
5. **发布**：唐小满按时发布，把状态改成「已发布」；宁远在一天内更新《时间线》。

## 分工

| 谁 | 管什么 |
|---|---|
| 沈墨（主笔） | 写稿、定情节 |
| 顾青（责任编辑） | 审稿、排期、维护写作规范 |
| 宁远（设定与校对） | 维护设定集和时间线，查设定冲突，校错字 |
| 唐小满（连载运营） | 发布、公告、读者评论 |`
