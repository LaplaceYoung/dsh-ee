// 介绍短片里的全部文字。不走界面的 i18n：同一份代码还要被 promo/ 渲染成 MP4，那边没有界面的翻译环境。
// 人名、数字和示例场景（docs/demo-company.md）保持一致。
export type FilmLang = "zh" | "en";

const zh = {
  // 第 1 幕：大标题，高亮的词轮换三次
  s1: { lead: "AI 不认识", words: ["你的客户", "你的合同", "你的公司。"] },
  // 第 2 幕：先打字提问，再出结果
  s2: {
    chips: ["/customer-brief", "CRM 客户系统", "产品资料"],
    q: "华东精密制造下周续约，我要注意什么？",
    headline: ["接上 dsh-ee，", "它就认识了。"],
    badge: "示意画面",
    sources: [
      { tool: "CRM · get_customer", fact: "合同 21 天后到期，75 天没有拜访" },
      { tool: "知识库 · 产品资料", fact: "续约 9 折，三年合同 8.5 折" },
    ],
    title: "华东精密制造",
    risk: "续约风险高",
    lines: ["合同 21 天后到期，已经 75 天没有拜访", "续约可给 9 折；再低要林娜审批", "建议本周上门，带上续约方案"],
    source: "出处：CRM 客户详情 ·《星河云产品介绍》价格一节",
  },
  // 第 3 幕：先是卡片墙上的大标题，再是围着标志转的小方块
  s3: {
    lead: "每个岗位，一位",
    word: "专家。",
    center: "配一次，送到每个人的 DSH",
    tiles: [
      { icon: "🎯", text: "续约谈判专家", kind: "expert" },
      { icon: "⚖️", text: "合同审查专家", kind: "expert" },
      { icon: "🧾", text: "报销稽核专家", kind: "expert" },
      { icon: "🧑‍🤝‍🧑", text: "大单评审小组", kind: "expert" },
      { icon: "", text: "技能 · 合同条款速查", kind: "skill" },
      { icon: "", text: "技能 · 客户邮件撰写", kind: "skill" },
      { icon: "", text: "技能 · 会议纪要整理", kind: "skill" },
      { icon: "", text: "连接器 · CRM", kind: "conn" },
      { icon: "", text: "连接器 · 差旅", kind: "conn" },
      { icon: "", text: "连接器 · IT 工单", kind: "conn" },
      { icon: "", text: "6 个知识库", kind: "dark" },
      { icon: "", text: "按部门授权", kind: "dark" },
    ],
  },
  // 第 4 幕：左边三行大字，右边调用记录
  s4: {
    lines: [
      { a: "你的", b: "服务器。" },
      { a: "你的", b: "规矩。" },
      { a: "你的", b: "记录。" },
    ],
    cardTitle: "DSH 想调用 CRM · log_visit",
    cardSub: "会修改数据：给华东精密制造记一条拜访",
    allow: "允许",
    deny: "拒绝",
    allowed: "已允许",
    logTitle: "dsh-ee · 调用记录",
    log: [
      { who: "周航 · 销售部", what: "CRM · get_customer", result: "成功", ok: true },
      { who: "陈一 · 财务部", what: "差旅 · list_claims", result: "成功", ok: true },
      { who: "孙悦 · 研发部", what: "CRM · list_customers", result: "已拦截：研发部不开放 CRM", ok: false },
      { who: "周航 · 销售部", what: "CRM · log_visit", result: "成功（已确认）", ok: true },
    ],
  },
  // 第 5 幕：小团队
  s5: {
    lead: "不只是公司，",
    word: "小团队也行。",
    team: "山海工作室 · 4 个人写《雾港纪事》",
    tableTitle: "章节进度",
    cols: ["章", "章名", "状态"],
    rows: [
      { no: 11, title: "白焰", status: "已发布", tone: "ok" },
      { no: 12, title: "第四次", status: "返稿修改", tone: "bad" },
      { no: 13, title: "罗盘指向的地方", status: "待审", tone: "wait" },
      { no: 14, title: "下码头的孩子", status: "写作中", tone: "wait" },
    ],
    expert: "设定一致性编辑",
    finding: "第 12 章里林栖一天引雾四次，和设定冲突：",
    rule: "引雾一天最多三次，第三次之后失声一日。",
    source: "出处：《雾港纪事》设定集 · 世界观与引雾",
  },
  // 片尾
  s6: {
    slogan: "让每位员工的 DSH 都认识你的公司",
    sub: "开源 · 私有部署 · 单个二进制",
    demo: "demo.hikariming.com",
    demoTag: "在线试用",
    repo: "github.com/hikariming/dsh-ee",
    repoTag: "源码",
  },
};

// 英文版的结构由中文版约束：少一句、多一句都编不过
const en: typeof zh = {
  s1: { lead: "AI doesn't know", words: ["your customers", "your contracts", "your company."] },
  s2: {
    chips: ["/customer-brief", "CRM", "Product docs"],
    q: "Huadong Precision renews next week. What should I watch for?",
    headline: ["Plug in dsh-ee.", "Now it does."],
    badge: "Illustration",
    sources: [
      { tool: "CRM · get_customer", fact: "Contract ends in 21 days; no visit in 75" },
      { tool: "Knowledge · Product docs", fact: "Renewal: 10% off, 15% for 3 years" },
    ],
    title: "Huadong Precision",
    risk: "High renewal risk",
    lines: ["Contract ends in 21 days; no visit in 75 days", "10% off on renewal; more needs Lin Na's approval", "Visit this week with a renewal offer"],
    source: "Sources: CRM customer record · Product docs, Pricing",
  },
  s3: {
    lead: "For every job, an",
    word: "expert.",
    center: "Set up once. In everyone's DSH.",
    tiles: [
      { icon: "🎯", text: "Renewal advisor", kind: "expert" },
      { icon: "⚖️", text: "Contract reviewer", kind: "expert" },
      { icon: "🧾", text: "Expense auditor", kind: "expert" },
      { icon: "🧑‍🤝‍🧑", text: "Deal desk", kind: "expert" },
      { icon: "", text: "Skill · Clause check", kind: "skill" },
      { icon: "", text: "Skill · Customer email", kind: "skill" },
      { icon: "", text: "Skill · Meeting notes", kind: "skill" },
      { icon: "", text: "Connector · CRM", kind: "conn" },
      { icon: "", text: "Connector · Travel", kind: "conn" },
      { icon: "", text: "Connector · IT tickets", kind: "conn" },
      { icon: "", text: "6 knowledge bases", kind: "dark" },
      { icon: "", text: "Access by department", kind: "dark" },
    ],
  },
  s4: {
    lines: [
      { a: "Your", b: "server." },
      { a: "Your", b: "rules." },
      { a: "Your", b: "log." },
    ],
    cardTitle: "DSH wants to call CRM · log_visit",
    cardSub: "Changes data: log a visit for Huadong Precision",
    allow: "Allow",
    deny: "Deny",
    allowed: "Allowed",
    logTitle: "dsh-ee · Call log",
    log: [
      { who: "Zhou Hang · Sales", what: "CRM · get_customer", result: "OK", ok: true },
      { who: "Chen Yi · Finance", what: "Travel · list_claims", result: "OK", ok: true },
      { who: "Sun Yue · R&D", what: "CRM · list_customers", result: "Blocked: CRM is closed to R&D", ok: false },
      { who: "Zhou Hang · Sales", what: "CRM · log_visit", result: "OK (confirmed)", ok: true },
    ],
  },
  s5: {
    lead: "Not just companies.",
    word: "Small teams too.",
    team: "Shanhai Studio · 4 people writing a novel",
    tableTitle: "Chapter tracker",
    cols: ["Ch.", "Title", "Status"],
    rows: [
      { no: 11, title: "White Flame", status: "Published", tone: "ok" },
      { no: 12, title: "The Fourth Time", status: "Sent back", tone: "bad" },
      { no: 13, title: "Where the Compass Points", status: "In review", tone: "wait" },
      { no: 14, title: "The Dockside Kid", status: "Writing", tone: "wait" },
    ],
    expert: "Continuity editor",
    finding: "In chapter 12, Lin Qi draws the fog four times in one day. That breaks the story bible:",
    rule: "At most three draws a day; after the third she loses her voice for a day.",
    source: "Source: Story bible · World and fog-drawing",
  },
  s6: {
    slogan: "Your company, inside every employee's Harness",
    sub: "Open source · Self-hosted · Single binary",
    demo: "demo.hikariming.com",
    demoTag: "LIVE DEMO",
    repo: "github.com/hikariming/dsh-ee",
    repoTag: "SOURCE",
  },
};

export type FilmCopy = typeof zh;
export const filmCopy: Record<FilmLang, FilmCopy> = { zh, en };
