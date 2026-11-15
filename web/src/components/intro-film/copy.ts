// 介绍短片里的全部文字。不走界面的 i18n：同一份代码还要被 promo/ 渲染成 MP4，那边没有界面的翻译环境。
// 人名、数字和示例场景（docs/demo-company.md）保持一致。
export type FilmLang = "zh" | "en";

const zh = {
  s1: {
    caption: "人人都在用 AI，可 AI 不认识你的公司",
    q: "华东精密制造下周续约，我要注意什么？",
    a: "抱歉，我看不到你们公司的客户和合同。你可以把相关资料复制给我……",
  },
  s2: {
    caption: "接上 dsh-ee：AI 查公司系统，按公司制度回答",
    badge: "示意画面",
    cmd: "/customer-brief",
    q: "华东精密制造下周续约，我要注意什么？",
    tools: ["CRM 客户系统 · get_customer", "知识库 · 产品资料"],
    title: "华东精密制造",
    risk: "续约风险高",
    lines: [
      "合同 21 天后到期，已经 75 天没有拜访",
      "续约可给 9 折，三年合同 8.5 折；再低要林娜审批",
      "建议本周上门，带上续约方案和新功能演示",
    ],
    source: "出处：CRM 客户详情 ·《星河云产品介绍》价格一节",
  },
  s3: {
    caption: "专家、技能、连接器：配一次，送到每个人的 DSH",
    experts: [
      {
        icon: "🎯",
        name: "续约谈判专家",
        tags: ["续约", "报价与折扣"],
        uses: "CRM 客户系统",
      },
      {
        icon: "⚖️",
        name: "合同审查专家",
        tags: ["销售合同", "保密协议"],
        uses: "合同条款速查",
      },
      {
        icon: "🧾",
        name: "报销稽核专家",
        tags: ["差旅标准", "异常单据"],
        uses: "差旅系统",
      },
      {
        icon: "🧑‍🤝‍🧑",
        name: "大单评审小组",
        tags: ["专家团队", "3 位专家"],
        uses: "CRM · 法务指引",
      },
    ],
    usesLabel: "用到",
    rows: [
      {
        label: "技能",
        items: ["合同条款速查", "客户邮件撰写", "会议纪要整理"],
      },
      { label: "连接器", items: ["CRM", "差旅", "IT 工单"] },
      { label: "知识库", items: ["产品资料", "法务指引", "员工手册"] },
    ],
    deliver: "按部门授权后，自动出现在员工的 DSH 里",
  },
  s4: {
    caption: "凭据不出服务器，改数据先确认，每次调用都有记录",
    cardTitle: "DSH 想调用 CRM · log_visit",
    cardSub: "会修改数据：给华东精密制造记一条拜访记录",
    allow: "允许",
    deny: "拒绝",
    allowed: "已允许",
    logTitle: "调用记录",
    log: [
      {
        who: "周航 · 销售部",
        what: "CRM · get_customer",
        result: "成功",
        ok: true,
      },
      {
        who: "陈一 · 财务部",
        what: "差旅 · list_claims",
        result: "成功",
        ok: true,
      },
      {
        who: "孙悦 · 研发部",
        what: "CRM · list_customers",
        result: "已拦截：研发部不开放 CRM",
        ok: false,
      },
      {
        who: "周航 · 销售部",
        what: "CRM · log_visit",
        result: "成功（已确认）",
        ok: true,
      },
    ],
  },
  s5: {
    caption: "不只是公司：4 个人写小说的团队，也是同一套",
    team: "山海工作室 · 《雾港纪事》",
    tableTitle: "章节进度",
    cols: ["章", "章名", "状态"],
    rows: [
      { no: 11, title: "白焰", status: "已发布", tone: "ok" },
      { no: 12, title: "第四次", status: "返稿修改", tone: "bad" },
      { no: 13, title: "罗盘指向的地方", status: "待审", tone: "wait" },
      { no: 14, title: "下码头的孩子", status: "写作中", tone: "wait" },
      { no: 15, title: "季闻的条件", status: "待写", tone: "idle" },
    ],
    expert: "设定一致性编辑",
    finding: "第 12 章里林栖一天引雾四次，和设定冲突：",
    rule: "引雾一天最多三次，第三次之后失声一日。",
    source: "出处：《雾港纪事》设定集 · 世界观与引雾",
  },
  s6: {
    slogan: "让每位员工的 DSH 都认识你的公司",
    sub: "公司和小团队都能用 · 开源 · 私有部署 · 单个二进制",
    demo: "在线试一下：demo.hikariming.com",
    repo: "github.com/hikariming/dsh-ee",
  },
};

// 英文版的结构由中文版约束：少一句、多一句都编不过
const en: typeof zh = {
  s1: {
    caption: "Everyone uses AI. But AI doesn't know your company.",
    q: "Huadong Precision renews next week. What should I watch out for?",
    a: "Sorry, I can't see your company's customers or contracts. You could paste the details here…",
  },
  s2: {
    caption: "With dsh-ee, AI checks your systems and follows your policies",
    badge: "Illustration",
    cmd: "/customer-brief",
    q: "Huadong Precision renews next week. What should I watch out for?",
    tools: ["CRM · get_customer", "Knowledge base · Product docs"],
    title: "Huadong Precision",
    risk: "High renewal risk",
    lines: [
      "Contract ends in 21 days; no visit in 75 days",
      "Renewal: 10% off, 15% off for 3 years; more needs Lin Na's approval",
      "Visit this week with a renewal offer and a feature demo",
    ],
    source: "Sources: CRM customer record · Product docs, Pricing",
  },
  s3: {
    caption: "Experts, skills, connectors: set up once, in everyone's DSH",
    experts: [
      {
        icon: "🎯",
        name: "Renewal advisor",
        tags: ["Renewals", "Pricing"],
        uses: "CRM",
      },
      {
        icon: "⚖️",
        name: "Contract reviewer",
        tags: ["Sales contracts", "NDAs"],
        uses: "Clause check",
      },
      {
        icon: "🧾",
        name: "Expense auditor",
        tags: ["Travel policy", "Odd claims"],
        uses: "Travel system",
      },
      {
        icon: "🧑‍🤝‍🧑",
        name: "Deal desk",
        tags: ["Expert team", "3 experts"],
        uses: "CRM · Legal guide",
      },
    ],
    usesLabel: "Uses",
    rows: [
      {
        label: "Skills",
        items: ["Clause check", "Customer email", "Meeting notes"],
      },
      { label: "Connectors", items: ["CRM", "Travel", "IT tickets"] },
      {
        label: "Knowledge",
        items: ["Product docs", "Legal guide", "Handbook"],
      },
    ],
    deliver: "Granted by department, then it shows up in each employee's DSH",
  },
  s4: {
    caption:
      "Credentials stay on your server. Writes need a yes. Every call is logged.",
    cardTitle: "DSH wants to call CRM · log_visit",
    cardSub: "Changes data: log a visit for Huadong Precision",
    allow: "Allow",
    deny: "Deny",
    allowed: "Allowed",
    logTitle: "Call log",
    log: [
      {
        who: "Zhou Hang · Sales",
        what: "CRM · get_customer",
        result: "OK",
        ok: true,
      },
      {
        who: "Chen Yi · Finance",
        what: "Travel · list_claims",
        result: "OK",
        ok: true,
      },
      {
        who: "Sun Yue · R&D",
        what: "CRM · list_customers",
        result: "Blocked: CRM is closed to R&D",
        ok: false,
      },
      {
        who: "Zhou Hang · Sales",
        what: "CRM · log_visit",
        result: "OK (confirmed)",
        ok: true,
      },
    ],
  },
  s5: {
    caption: "Not just companies: a 4-person novel team runs on the same thing",
    team: "Shanhai Studio · Fog Harbor Chronicles",
    tableTitle: "Chapter tracker",
    cols: ["Ch.", "Title", "Status"],
    rows: [
      { no: 11, title: "White Flame", status: "Published", tone: "ok" },
      { no: 12, title: "The Fourth Time", status: "Sent back", tone: "bad" },
      {
        no: 13,
        title: "Where the Compass Points",
        status: "In review",
        tone: "wait",
      },
      { no: 14, title: "The Dockside Kid", status: "Writing", tone: "wait" },
      { no: 15, title: "Ji Wen's Terms", status: "Not started", tone: "idle" },
    ],
    expert: "Continuity editor",
    finding:
      "In chapter 12, Lin Qi draws the fog four times in one day. That breaks the story bible:",
    rule: "At most three draws a day; after the third she loses her voice for a day.",
    source: "Source: Story bible · World and fog-drawing",
  },
  s6: {
    slogan: "Your company, inside every employee's Harness",
    sub: "For companies and small teams · Open source · Self-hosted · Single binary",
    demo: "Try it: demo.hikariming.com",
    repo: "github.com/hikariming/dsh-ee",
  },
};

export type FilmCopy = typeof zh;
export const filmCopy: Record<FilmLang, FilmCopy> = { zh, en };
