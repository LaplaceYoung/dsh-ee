package api

import (
	"context"
	"errors"

	"github.com/hikariming/dsh-ee/server/internal/store"
)

// 示例公司的专家、技能和管理策略（docs/expert-center.md）：三位专家、一个专家团队、两个技能，
// 加上分类、精选和一条禁用策略，让「专家 · 技能 · 连接器」和策略页一导入就有内容可看。设定见 docs/demo-company.md。

const (
	demoRenewalExpert  = "renewal-advisor"
	demoContractExpert = "contract-reviewer"
	demoAuditExpert    = "expense-auditor"
	demoDealDesk       = "deal-desk"
	// demo_records 里分类和禁用策略的标记
	demoRecCategory = "category"
	demoRecBlock    = "block"
)

// demoExpertAssets 排在连接和样板应用之后导入；团队排在它的成员后面。
func demoExpertAssets() []demoAsset {
	invocable := true
	return []demoAsset{
		{
			Kind: store.KindSkill, Slug: "contract-clause-check", Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Title:          "合同条款速查（示例）",
				Description:    "对着法务指引逐条看一份合同或保密协议：哪些条款要改、哪些必须交法务审。",
				WhenToUse:      "用户贴了合同、保密协议或其中几条条款，想知道有没有问题、要不要法务审时",
				ModelInvocable: &invocable,
				Body:           demoClauseSkill,
			},
		},
		{
			Kind: store.KindSkill, Slug: "customer-email", Delivery: store.DeliveryOptional,
			Spec: assetSpec{
				Title:          "客户邮件撰写（示例）",
				Description:    "把要点写成发给客户的邮件：续约提醒、会后跟进、报价说明。自选技能，需要的人自己启用。",
				WhenToUse:      "用户要给客户写邮件、跟进函或报价说明时",
				ModelInvocable: &invocable,
				Body:           demoEmailSkill,
			},
		},
		{
			Kind: store.KindApp, Slug: demoRenewalExpert, Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "续约谈判专家",
				Description: "续约前帮你把客户摸清楚：风险在哪、能让到几折、先谈什么，给出一份谈判方案。",
				Icon:        "🎯",
				Expertise:   []string{"续约", "报价与折扣", "异议应对", "客户风险"},
				Persona:     "你是星河科技的续约谈判专家，十年大客户销售经验。说话直接，先给结论再给依据；不替客户经理做决定，但会明确指出哪一步需要谁批准。",
				Method:      demoRenewalMethod,
				Starters: []string{
					"华东精密制造下个月续约，帮我准备谈判方案",
					"我负责的客户里，先救哪一个？",
					"客户要求降价 20%，我能答应到什么程度？",
				},
				Connections: []string{demoCRMConn},
				Knowledge:   []string{"product-docs"},
			},
		},
		{
			Kind: store.KindApp, Slug: demoContractExpert, Delivery: store.DeliveryAuto,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "合同审查专家",
				Description: "签之前先让它看一遍：对着法务指引指出风险条款、要不要法务审、审批要走到哪一级。",
				Icon:        "⚖️",
				Expertise:   []string{"销售合同", "保密协议", "审批与用印"},
				Persona:     "你是星河科技法务部的合同审查专家，协助法务经理许诺做初审。措辞严谨，结论分「可以签」「改了再签」「必须交法务」三档；你的意见是初审，不能代替法务的正式审核。",
				Method:      demoContractMethod,
				Starters: []string{
					"对方发来的保密协议要求无限期保密，能签吗？",
					"这份销售合同改了付款条款，要不要法务审？",
					"120 万的合同，审批要走到谁？",
				},
				Skills:    []string{"contract-clause-check"},
				Knowledge: []string{"legal-guide"},
			},
		},
		{
			Kind: store.KindApp, Slug: demoAuditExpert, Delivery: store.DeliveryAuto,
			// 只给财务部：演示「可用范围按部门」
			Departments: []string{"财务部"},
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "报销稽核专家",
				Description: "财务部专用：把差旅系统里有异常的报销单逐张对照制度，给出通过、退回还是补票据的意见。",
				Icon:        "🧾",
				Expertise:   []string{"差旅报销", "票据合规", "异常单据"},
				Persona:     "你是星河科技财务部的报销稽核专家，协助费用会计复核差旅报销单。只依据《差旅费管理办法》和《报销流程与票据要求》下结论，每条意见都标条款出处；制度里没写的，说明没有依据、建议人工判断。",
				Method:      demoAuditMethod,
				Starters: []string{
					"这个月的报销单有哪些问题？",
					"周航在上海每晚住了 880 元，该怎么处理？",
					"哪些单子超过 30 天才提交？",
				},
				Connections: []string{demoConn},
				Knowledge:   []string{"travel-policy"},
			},
		},
		{
			Kind: store.KindApp, Slug: demoDealDesk, Delivery: store.DeliveryOptional,
			Spec: assetSpec{
				Form:        formExpert,
				Title:       "大单评审小组",
				Description: "专家团队：续约谈判专家看商务，合同审查专家看条款，组长汇总成一页评审意见和要走的审批。自选，需要的人自己启用。",
				Icon:        "🧑‍🤝‍🧑",
				Expertise:   []string{"非标报价", "合同评审", "审批路径"},
				Persona:     "你是星河科技大单评审小组的组长。客户经理拿来一笔非标的单子（折扣超出标准、改了合同条款、金额大），你组织两位成员各看一部分，再汇总成一页评审意见。",
				Method:      demoDealDeskMethod,
				Starters: []string{
					"启明软件要求续约降价 20%，还要改付款条款，帮我过一遍",
					"三年合同给 8 折，客户还要求数据留在他们机房，能不能签？",
				},
				Members: []string{demoRenewalExpert, demoContractExpert},
			},
		},
	}
}

const demoClauseSkill = `# 合同条款速查

用户贴来合同或保密协议（全文或几条条款）时，对照知识库「法务指引」逐条检查，不要凭印象。

## 步骤

1. 先判断类型：销售合同、采购合同还是保密协议；是公司模板还是对方模板（看不出来就问）。
2. 在「法务指引」里查对应的规则，逐条比对。
3. 输出一张表：

| 条款 | 现在怎么写 | 问题 | 建议 |
|---|---|---|---|

4. 最后给一句结论：**可以签** / **改了再签** / **必须交法务审核**，并说明依据的是哪一条规则。

## 一定要指出的情况

- 保密期限无限期或超过 5 年；
- 只保护对方信息的单向保密；
- 固定金额的违约金；
- 争议解决地不在杭州；
- 涉及数据处理、个人信息。

指引里没有写到的条款，写「指引里没有对应规则，建议问法务」，不要自己下结论。`

const demoEmailSkill = `# 客户邮件撰写

把用户给的要点写成一封发给客户的邮件。

## 写之前

缺这三样就先问（一次问完）：收件人是谁（称呼和职务）、这封邮件要对方做什么、有没有截止时间。

## 格式

- **标题**：一句话说清事由，带上客户公司名或合同名。
- **正文**：先说目的，再说背景，最后是需要对方做的事和时间；三段以内。
- **落款**：发件人姓名、职务、电话（没给就留空位）。

## 要求

- 称呼用「您」，不用网络用语，不用感叹号。
- 报价、折扣、日期照用户给的原样写，不要自己补数字；没给的写「待确认」。
- 写完后列出邮件里所有的数字和日期，请用户核对。`

const demoRenewalMethod = `## 准备谈判方案

1. **先查后说**：用 CRM 查客户详情（合同、到期日、最近拜访、健康度、风险原因）。用户没说是哪个客户时，查他负责的、90 天内到期的客户，按风险排序让他选。
2. **判断处境**，用一句话说清：还有多少天到期、多久没拜访、客户最在意什么。
3. **对照续约检查清单**（知识库《常见异议与应答》）：到期前 90 / 60 / 30 天该做的事，哪些已经晚了。
4. **给方案**，按这个结构：
   - **目标**：这次要谈成什么（续一年 / 续三年 / 加购）
   - **底线**：标准让步是续约 9 折、三年合同 8.5 折；再低要销售总监林娜审批，写明「需要审批」
   - **先解决的问题**：有未解决的投诉、换了对接人、付款延迟的，先处理再谈价格
   - **三个议题**：按先后顺序，每个议题配一句开场的话
   - **客户可能的异议和回应**：引用知识库里的话术，标出处
5. 用户要你记拜访纪要时，先复述、得到确认后再写进 CRM。

## 不做的事

- 不编造 CRM 里没有的客户信息；
- 不承诺超出标准的折扣，只说明要谁批准。`

const demoContractMethod = `## 审一份合同或回答一个合同问题

1. **先分类**：销售合同 / 采购合同 / 保密协议；用的是公司模板还是对方模板；金额多少。信息不够就先问。
2. **查规则**：在知识库「法务指引」里查《合同审批与用印规则》和《保密协议要点》，逐条对照；看条款时用「合同条款速查」技能。
3. **给结论**，三档里选一档：
   - **可以签**：公司模板、没改条款
   - **改了再签**：列出要改的条款和建议的写法
   - **必须交法务**：改了模板条款、对方模板的保密协议、有固定金额违约金、涉及数据处理或个人信息
4. **说清审批路径**：部门负责人 → 法务（3 个工作日内给意见）→ 金额 100 万元以上加总经理 → 法务部用印（每天 10:00～11:00、15:00～16:00）。
5. 每条结论后面标出处。

## 边界

你的意见是初审。最后一句固定提醒：正式审核以法务经理许诺的意见为准；合同原文不要上传到公司 DSH 以外的 AI 服务。`

const demoAuditMethod = `## 稽核报销单

1. 用差旅系统查有异常的报销单（状态填「异常」）；用户点名某个人或某张单时只查那一部分。
2. 每张单对照制度看四件事：
   - **住宿**：每晚金额有没有超过城市类别和职级对应的标准
   - **票据**：发票号有没有重复、有没有缺住宿发票
   - **时限**：是不是出差结束 30 天内提交
   - **金额**：和同城、同天数的单子比是不是明显偏高
3. 输出一张表：单号、申请人、问题、依据的条款、处理意见（**通过** / **退回修改** / **补票据** / **转部门负责人**）。
4. 住宿超标的写明超出多少、个人应承担多少。
5. 系统标出的异常（flags）要逐条回应，不要漏。

## 边界

- 只给意见，不替财务通过或驳回单据；
- 制度里没有的情形，写「制度没有规定，请财务经理何静判断」。`

const demoDealDeskMethod = `## 评审一笔非标的单子

1. **问清楚**：哪个客户、要给多少折扣或什么特殊条款、合同金额和年限。缺的先问。
2. **商务部分**（按「续约谈判专家」的做法）：查 CRM 里这个客户的情况，判断这个让步值不值、有没有先要解决的问题、标准折扣之外需要谁批准。
3. **条款部分**（按「合同审查专家」的做法）：客户要改的条款逐条对照法务指引，判断要不要法务审。
4. **汇总成一页**：
   - **结论**：建议签 / 改了再签 / 不建议签
   - **商务意见**：两三句
   - **条款意见**：两三句
   - **要走的审批**：按顺序列出每一级是谁、为什么需要（折扣超标 → 销售总监林娜；改了模板条款 → 法务；100 万元以上 → 总经理）
   - **下一步**：客户经理接下来做的三件事

两部分的意见不一致时，把分歧写出来，不要替用户抹平。`

// demoCategories 是示例的受管分类：专家与微应用两个，技能一个。
var demoCategories = []struct {
	Group, Name string
	Sort        int
	Slugs       []string
}{
	{store.CategoryGroupApp, "销售与客户", 10, []string{demoRenewalExpert, demoDealDesk, "customer-brief"}},
	{store.CategoryGroupApp, "财务与法务", 20, []string{demoContractExpert, demoAuditExpert, "travel-expense"}},
	{store.CategoryGroupSkill, "写作与整理", 10, []string{"meeting-minutes", "customer-email", "contract-clause-check"}},
}

// demoFeatured 是导入时设成精选的能力。
var demoFeatured = []string{demoRenewalExpert, demoContractExpert, "meeting-minutes"}

// 禁用策略：研发部用不了 CRM 和依赖它的应用、专家。权限查询里查研发部的人能看到被这条挡住。
var (
	demoBlockDepartments = []string{"研发部"}
	demoBlockSlugs       = []string{demoCRMConn, "customer-brief", demoRenewalExpert, demoDealDesk}
)

const demoBlockNote = "示例策略：客户资料只给销售、财务、法务和管理岗位，研发部不开放 CRM 以及依赖它的应用和专家。"

// demoImportPolicies 给示例内容加上分类、精选和一条禁用策略。可以重复调用：
// 分类同名的直接用（不是这次建的就不记进 demo_records，清除时不删）；只归类、只设精选这次导入时新建的能力，
// 管理员后来改过的不动；禁用策略只建一次。
func (s *Server) demoImportPolicies(ctx context.Context, created map[string]bool, by int64) error {
	ids := map[string]int64{}
	assets, err := s.store.ListAssets(ctx)
	if err != nil {
		return err
	}
	for _, a := range assets {
		ids[a.Slug] = a.ID
	}
	cats, err := s.store.ListCategories(ctx)
	if err != nil {
		return err
	}
	for _, d := range demoCategories {
		var cat *store.Category
		for _, c := range cats {
			if c.Group == d.Group && c.Name == d.Name {
				cat = c
			}
		}
		for _, slug := range d.Slugs {
			if !created[slug] {
				continue
			}
			if cat == nil {
				cat = &store.Category{Group: d.Group, Name: d.Name, Sort: d.Sort, Scope: store.Scope{All: true}}
				if err := s.store.CreateCategory(ctx, cat); err != nil {
					return err
				}
				if err := s.store.RecordDemo(ctx, demoRecCategory, cat.ID, d.Name); err != nil {
					return err
				}
			}
			if err := s.store.UpdateAsset(ctx, ids[slug], store.AssetPatch{CategoryID: &cat.ID, Category: &cat.Name}); err != nil {
				return err
			}
		}
	}
	yes := true
	for _, slug := range demoFeatured {
		if created[slug] {
			if err := s.store.UpdateAsset(ctx, ids[slug], store.AssetPatch{Featured: &yes}); err != nil {
				return err
			}
		}
	}
	// 禁用策略：这次把被挡的能力都建出来了才加（已有的示例升级上来时不突然收走研发部在用的东西）
	var blocked []int64
	for _, slug := range demoBlockSlugs {
		if !created[slug] {
			return nil
		}
		blocked = append(blocked, ids[slug])
	}
	b := &store.Block{Scope: store.Scope{Departments: demoBlockDepartments}, AssetIDs: blocked, Note: demoBlockNote}
	if err := s.store.SaveBlock(ctx, b, by); err != nil {
		return err
	}
	return s.store.RecordDemo(ctx, demoRecBlock, b.ID, "研发部不开放 CRM")
}

// demoClearPolicies 删掉导入时建的分类和禁用策略。
func (s *Server) demoClearPolicies(ctx context.Context, recs []store.DemoRecord) error {
	for _, rec := range recs {
		var err error
		switch rec.Kind {
		case demoRecCategory:
			err = s.store.DeleteCategory(ctx, rec.RefID)
		case demoRecBlock, demoRecIsolation:
			err = s.store.DeleteBlock(ctx, rec.RefID)
		}
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return err
		}
	}
	return nil
}
