// 把 OpenMotion 模板「Glean Agents 发布」改成 dsh-ee 的一分钟产品短片，出英文、中文、日文三版。
//
// 每种语言出两样东西：
//   promo/out/dsh-ee-openmotion-film-<lang>/        OpenMotion（Hyperframes）工程，用来渲染 MP4、在 Studio 里继续改
//   web/src/components/product-film/generated/      首页直接用代码播放的那一份（样式、时间轴、各语言的画面）
//
// 用法：node promo/openmotion/build-film.mjs
// 需要本机有 OpenMotion 源码（默认 ~/openmotion，可用 OPENMOTION_DIR 指定）：模板和 jsdom 都从那里取。
// 文字都在下面的 COPY 里；人名、数字和 docs/demo-company.md 保持一致，图标是示例应用自己的图标。
import { cpSync, mkdirSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { createRequire } from "node:module";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import os from "node:os";

const here = dirname(fileURLToPath(import.meta.url));
const repo = join(here, "..", "..");
const openmotion = process.env.OPENMOTION_DIR || join(os.homedir(), "openmotion");
const template = join(openmotion, "library/templates/catalog/motion-f6f244d1-c549-4081-8a5d-f0e1e3660044/source");
const { JSDOM } = createRequire(join(openmotion, "package.json"))("jsdom");

const BRAND = "#4d6bfe";
const WHALE = "M23 14.8c0 3.6-3 6.2-7.2 6.2H9.8c-3.4 0-5.6-2-5.9-5.1l-.3-4.6C2.5 10.9 1.4 9.9 1 8.4c1.6.2 2.8.7 3.5 1.7.7-1 1.9-1.5 3.5-1.7-.4 1.5-1.5 2.5-2.6 2.9l.1 1.4c.4 1.4 1.3 2.1 2.6 2.1C9.2 11.3 12.1 8.9 15.7 8.9c4.4 0 7.3 2.5 7.3 5.9Zm-3.3-.9a1.15 1.15 0 1 0-2.3 0 1.15 1.15 0 0 0 2.3 0Z";
const SPOUT = "M15.8 7.8V6m0 0c-.3-1.5-1.4-2.2-2.7-2m2.7 2c.3-1.5 1.4-2.2 2.7-2";
const whale = (fill) => `<path fill="${fill}" fill-rule="evenodd" d="${WHALE}"/><path fill="none" stroke="${fill}" stroke-width="1.6" stroke-linecap="round" d="${SPOUT}"/>`;

// 应用图标：和 server/internal/api/demo_*.go 里的 Icon 一致
const ICON = { brief: "🤝", travel: "🧳", onboarding: "🧭", it: "🛠", renewal: "🎯", contract: "⚖️", audit: "🧾", lore: "📖", chapters: "🗂", deal: "🧑‍🤝‍🧑", outing: "🏕", weekly: "📈", review: "🔍" };

const COPY = {
  en: {
    htmlLang: "en",
    opening: ["AI doesn't", "know", "your company"],
    intro: ["Introducing ", ""],
    nav: ["Home", "Apps", "Experts", "Knowledge"],
    chat: { title: "Apps", add: "＋ New app", selected: "◉ Micro-apps", recent: "Recently used", older: "Older", contract: "Contract review" },
    hub: { title: "Capability hub", create: "＋ Create app", how: "How it works", tabs: "All　 Auto-enabled　 Opt-in", search: "⌕ Search apps, experts and skills", tags: "Featured　 Micro-apps　 Experts　 Skills　 Connectors", groups: ["For your department", "For everyone at your company"], seeAll: "See all", app: "MICRO-APP", expert: "EXPERT" },
    names: { brief: "Customer visit brief", travel: "Travel expense assistant", onboarding: "Onboarding buddy", it: "IT service desk", renewal: "Renewal advisor", contract: "Contract reviewer", audit: "Expense auditor", lore: "Lore Q&A", chapters: "Chapter tracker", deal: "Deal desk", outing: "Team outing sign-up", weekly: "My customer weekly" },
    hubDesc: { brief: "Accounts you own and contracts about to expire", renewal: "An expert for renewal negotiations", contract: "An expert for contract review", travel: "Spots over-limit hotels and duplicate invoices", onboarding: "What to do in your first week", it: "Ticket backlog and common issues" },
    facts: { brief: "50 customers, 5 at renewal risk", travel: "4 flagged claims", onboarding: "First-week checklist", contract: "Checks the legal guidelines", it: "~28 tickets, mostly VPN", lore: "Answers cite the story bible", chapters: "Ch. 12 sent back for edits", renewal: "10% off; more needs approval", outing: "6 sign-ups, 2 notices", weekly: "Built by Zhou Hang" },
    networkTitle: "Your company, inside<br>every employee's<br>Harness",
    dept: { title: "Delivered to<br>every employee", end: "scoped by<br>department", names: ["Sales", "Finance", "R&D", "IT"] },
    placeholder: "Ask about your company...",
    daily: "Any problems in this month's\nexpense claims?",
    renewal: "Huadong Precision renews next week.\nWhat should I watch for?",
    buildTitle: "Here's how<br>it works",
    start: "START",
    output: "IN DSH",
    flow: [
      ["Connect", "ERP, CRM and ticketing, as MCP<br>servers or OpenAPI specs"],
      ["Add knowledge", "Policies, product docs and<br>shared datasets"],
      ["Assemble", "Micro-apps and experts from<br>skills, prompts and connections"],
      ["Review", "Automated checks, then admin<br>approval for every package"],
      ["Grant access", "By department, role or person"],
      ["Sync", "Enabled capabilities reach every employee's DSH automatically"],
      ["Ask", "Employees open an app in DSH with /name"],
      ["Call", "Credentials stay on your server, never on employee machines"],
      ["Confirm", "Changes to your data ask the employee first"],
      ["Audit", "Every install and every call is written to the log"],
    ],
    editor: {
      settings: "Settings", preview: "Preview", publish: "Publish", kind: "Micro-app",
      p1: "A persona, a workbench and its ingredients. Employees open it in DSH with /customer-brief.",
      access: "Access · Sales department",
      p2: "It uses the CRM connection, the product docs knowledge base and the renewal advisor expert.",
      p3: "Credentials are encrypted and kept on your server. Every tool is authorized on its own.",
      edit: "Describe a change...",
      nodes: [["Connection", "CRM · get_customer<br>CRM · log_visit"], ["Knowledge base", "Product docs,<br>with versions and rollback"], ["Expert", "Renewal advisor"], ["Access", "Sales department,<br>auto-enabled"], ["Publish", "Reviewed, versioned and<br>synced to every DSH"]],
    },
    orgTitle: "For small teams<br>and whole enterprises",
    tiles: ["<b>4</b>people<br>one studio", "<b>200</b>people<br>one company"],
    knowledge: "Answers that cite<br>their sources",
    data: ["Using", "your systems", "your docs"],
    report: { steps: ["CRM · get_customer　 ›", "Knowledge · Product docs, Pricing", "Summarize with sources"], th: ["Customer", "Contract ends", "Last visit"], row: ["Huadong Precision", "in 21 days", "75 days ago"], source: "CRM customer record · Product docs, Pricing" },
    built: "Your server. Your rules. Your log.",
    gov: "Every call authorized and audited.",
    closing: ["Open source.", "Self-hosted.", "One binary."],
  },
  zh: {
    htmlLang: "zh-CN",
    opening: ["AI", "不认识", "你的公司"],
    intro: ["认识一下 ", ""],
    nav: ["首页", "应用", "专家", "知识库"],
    chat: { title: "应用", add: "＋ 新建应用", selected: "◉ 微应用", recent: "最近使用", older: "更早", contract: "合同审查" },
    hub: { title: "能力中心", create: "＋ 创建应用", how: "工作原理", tabs: "全部　 默认生效　 自选", search: "⌕ 搜索应用、专家和技能", tags: "精选　 微应用　 专家　 技能　 连接器", groups: ["你所在的部门", "全公司"], seeAll: "查看全部", app: "微应用", expert: "专家" },
    names: { brief: "客户拜访简报", travel: "出差报销助手", onboarding: "新人入职助手", it: "IT 服务台", renewal: "续约谈判专家", contract: "合同审查专家", audit: "报销稽核专家", lore: "设定问答", chapters: "章节进度", deal: "大单评审小组", outing: "团建报名", weekly: "我的客户周报" },
    hubDesc: { brief: "你负责的客户和快到期的合同", renewal: "续约前帮你把客户摸清楚", contract: "签之前先让它看一遍", travel: "找出住宿超标和重复发票", onboarding: "第一周要做什么", it: "工单积压和常见故障" },
    facts: { brief: "50 个客户，5 个续约风险高", travel: "4 张异常报销单", onboarding: "第一周清单", contract: "对照法务指引逐条看", it: "约 28 张工单，VPN 最多", lore: "答案带设定集的出处", chapters: "第 12 章返稿修改", renewal: "续约 9 折，再低要审批", outing: "6 条报名，2 条公告", weekly: "周航自己搭的" },
    networkTitle: "让每位员工的 DSH<br>都认识你的公司",
    dept: { title: "送到<br>每位员工手里", end: "按部门<br>授权", names: ["销售部", "财务部", "研发部", "IT 部"] },
    placeholder: "问问你的公司…",
    daily: "这个月的报销单\n有什么问题？",
    renewal: "华东精密制造下周续约，\n我要注意什么？",
    buildTitle: "它是这样<br>工作的",
    start: "开始",
    output: "进入 DSH",
    flow: [
      ["接入系统", "ERP、CRM、工单系统，<br>用 MCP 或 OpenAPI 接入"],
      ["加入知识", "制度、产品资料<br>和共享数据集"],
      ["组装", "用技能、提示词和连接<br>组装微应用和专家"],
      ["审核", "自动检查，<br>再由管理员逐个审批"],
      ["授权", "按部门、角色或个人"],
      ["同步", "启用的能力自动同步到每位员工的 DSH"],
      ["提问", "员工在 DSH 里用 /名称 打开应用"],
      ["调用", "凭据留在你的服务器上，不下发到员工电脑"],
      ["确认", "会修改数据的操作，先问员工"],
      ["审计", "每次安装、每次调用都写进审计日志"],
    ],
    editor: {
      settings: "设置", preview: "预览", publish: "发布", kind: "微应用",
      p1: "人设、工作台加上配料。员工在 DSH 里用 /customer-brief 打开。",
      access: "可用范围 · 销售部",
      p2: "用到 CRM 连接、产品资料知识库和续约谈判专家。",
      p3: "凭据加密保存在你的服务器上，每个工具单独授权。",
      edit: "描述要改什么…",
      nodes: [["连接", "CRM · get_customer<br>CRM · log_visit"], ["知识库", "产品资料，<br>带版本和回滚"], ["专家", "续约谈判专家"], ["可用范围", "销售部，<br>默认生效"], ["发布", "审核、带版本，<br>同步到每个 DSH"]],
    },
    orgTitle: "小团队和大企业<br>都能用",
    tiles: ["<b>4</b>个人<br>一间工作室", "<b>200</b>人<br>一家公司"],
    knowledge: "每个回答<br>都带出处",
    data: ["用上", "你的系统", "你的文档"],
    report: { steps: ["CRM · get_customer　 ›", "知识库 · 产品资料，价格一节", "汇总并标注出处"], th: ["客户", "合同到期", "最近拜访"], row: ["华东精密制造", "21 天后", "75 天前"], source: "CRM 客户详情 · 产品资料，价格一节" },
    built: "你的服务器。你的规矩。你的记录。",
    gov: "每次调用都经过授权，留有记录。",
    closing: ["开源。", "私有部署。", "单个二进制。"],
  },
  ja: {
    htmlLang: "ja",
    opening: ["AI は", "あなたの会社を", "知らない"],
    intro: ["", " 登場"],
    nav: ["ホーム", "アプリ", "専門家", "ナレッジ"],
    chat: { title: "アプリ", add: "＋ 新規アプリ", selected: "◉ マイクロアプリ", recent: "最近使った項目", older: "以前", contract: "契約レビュー" },
    hub: { title: "ケイパビリティハブ", create: "＋ アプリ作成", how: "仕組み", tabs: "すべて　 自動で有効　 選択制", search: "⌕ アプリ・専門家・スキルを検索", tags: "おすすめ　 マイクロアプリ　 専門家　 スキル　 コネクタ", groups: ["あなたの部署向け", "全社向け"], seeAll: "すべて見る", app: "マイクロアプリ", expert: "専門家" },
    names: { brief: "顧客訪問ブリーフ", travel: "出張経費アシスタント", onboarding: "新人オンボーディング", it: "IT サービスデスク", renewal: "更新交渉アドバイザー", contract: "契約レビュー担当", audit: "経費監査担当", lore: "設定Q&A", chapters: "章の進捗", deal: "大型案件レビュー班", outing: "社内イベント申込", weekly: "マイ顧客週報" },
    hubDesc: { brief: "担当顧客と期限が近い契約", renewal: "更新交渉の準備を支援", contract: "署名前に契約を確認", travel: "宿泊費の超過と重複領収書を検出", onboarding: "入社1週目にやること", it: "チケットの滞留とよくある障害" },
    facts: { brief: "顧客50社、5社が更新リスク高", travel: "異常な経費申請 4 件", onboarding: "入社1週目のチェックリスト", contract: "法務ガイドラインと照合", it: "チケット約28件、VPN が最多", lore: "回答に設定集の出典付き", chapters: "第12章は差し戻し", renewal: "更新は10%引き、以降は要承認", outing: "申込 6 件、お知らせ 2 件", weekly: "周航さんが自作" },
    networkTitle: "あなたの会社を、<br>社員全員の<br>Harness に",
    dept: { title: "社員一人ひとりに<br>届ける", end: "部署ごとに<br>権限設定", names: ["営業", "財務", "開発", "IT"] },
    placeholder: "会社のことを聞いてみよう…",
    daily: "今月の経費申請に\n問題はありますか？",
    renewal: "華東精密製造が来週更新です。\n何に注意すべきですか？",
    buildTitle: "その<br>仕組み",
    start: "スタート",
    output: "DSH へ",
    flow: [
      ["接続", "ERP・CRM・チケット管理を<br>MCP か OpenAPI で接続"],
      ["ナレッジ追加", "社内規程、製品資料、<br>共有データセット"],
      ["組み立て", "スキル・プロンプト・接続から<br>マイクロアプリと専門家を作成"],
      ["レビュー", "自動チェックの後、<br>管理者がパッケージごとに承認"],
      ["権限付与", "部署・ロール・個人単位で"],
      ["同期", "有効化した機能は全社員の DSH に自動で届く"],
      ["質問", "社員は DSH で /名前 と打ってアプリを開く"],
      ["呼び出し", "認証情報は自社サーバーに残り、社員の端末には渡らない"],
      ["確認", "データを変更する操作は、まず社員に確認"],
      ["監査", "インストールも呼び出しも、すべてログに記録"],
    ],
    editor: {
      settings: "設定", preview: "プレビュー", publish: "公開", kind: "マイクロアプリ",
      p1: "ペルソナ、ワークベンチ、そして材料。社員は DSH で /customer-brief と打って開きます。",
      access: "公開範囲 · 営業部",
      p2: "CRM 接続、製品資料ナレッジベース、更新交渉アドバイザーを使います。",
      p3: "認証情報は暗号化して自社サーバーに保存。ツールは一つずつ認可します。",
      edit: "変更内容を入力…",
      nodes: [["接続", "CRM · get_customer<br>CRM · log_visit"], ["ナレッジベース", "製品資料<br>版管理・ロールバック付き"], ["専門家", "更新交渉アドバイザー"], ["公開範囲", "営業部、<br>自動で有効"], ["公開", "レビュー済み・版管理、<br>全員の DSH に同期"]],
    },
    orgTitle: "小さなチームにも<br>大企業にも",
    tiles: ["<b>4</b>人の<br>スタジオ", "<b>200</b>人の<br>会社"],
    knowledge: "すべての回答に<br>出典を明記",
    data: ["ともに", "自社システム", "社内文書"],
    report: { steps: ["CRM · get_customer　 ›", "ナレッジ · 製品資料「価格」", "出典付きで要約"], th: ["顧客", "契約満了", "最終訪問"], row: ["華東精密製造", "21日後", "75日前"], source: "CRM 顧客レコード · 製品資料「価格」" },
    built: "自社サーバー。自社ルール。自社ログ。",
    gov: "すべての呼び出しを認可し、監査。",
    closing: ["オープンソース。", "セルフホスト。", "単一バイナリ。"],
  },
};

// 模板卡片墙上的原文 → 应用
const MOSAIC = {
  "Train 100s of new or seasonal support members": "brief",
  "Provide actionable feedback from last meeting": "travel",
  "Daily catch-up": "onboarding",
  "Determine the likelihood of the deal closing this quarter": "it",
  "Check the product documentation for consistency": "renewal",
  "Write a PRD": "contract",
  "Summarize product insights from 1000s of calls": "audit",
  "Analyze historical sales data to determine the ICP.": "lore",
  "Generate slide copy for a marketing launch.": "chapters",
  "Provide feedback on product UI from the perspective of users": "deal",
};

const EXTRA_CSS = `
#root{--brand:${BRAND}}
#root:lang(zh) .txt,#root:lang(ja) .txt{letter-spacing:-1px;line-height:1.14}
#root:lang(zh) .prompt-copy,#root:lang(ja) .prompt-copy{letter-spacing:0;line-height:1.3}
#root:lang(zh) .flow-card,#root:lang(ja) .flow-card,#root:lang(zh) .dash-shell,#root:lang(ja) .dash-shell,#root:lang(zh) .builder-node,#root:lang(ja) .builder-node,#root:lang(zh) .card-copy,#root:lang(ja) .card-copy,#root:lang(zh) .report h2,#root:lang(ja) .report h2{letter-spacing:0}
.avatar-initials{display:flex;align-items:center;justify-content:center;color:#fff;font-size:15px;font-weight:700;letter-spacing:0}
.portrait-tile{width:100%;height:100%;border-radius:inherit;display:flex;flex-direction:column;align-items:center;justify-content:center;text-align:center;font-size:21px;line-height:1.15;letter-spacing:-.6px;color:#1b1b2a}
.portrait-tile b{font-size:74px;line-height:1;font-weight:700;letter-spacing:-3px}
.tile-team{background:linear-gradient(150deg,#f6eed9,#f4ab90 60%,#b7a9f3)}
.tile-company{background:linear-gradient(170deg,#e8fd9a,#9cdeee 55%,#a6a0f3)}
.app-name{display:block;font-size:17px;font-weight:700;letter-spacing:-.5px;line-height:1.15}
.app-fact{display:block;margin-top:6px;font-size:13px;letter-spacing:-.2px;line-height:1.25;color:#5b5870}
.app-emoji{font-size:15px!important;padding:4px 5px!important}
.wordmark-row{display:flex;align-items:center;justify-content:center;gap:18px;height:132px}
.wordmark-row span{font-size:112px;font-weight:700;letter-spacing:-5px;line-height:1;color:#14142b}
.wordmark-sub{text-align:center;font-size:19px;letter-spacing:-.3px;color:#55556a;margin-top:14px;white-space:nowrap}
`;

function build(lang) {
  const c = COPY[lang];
  const src = readFileSync(join(template, "compositions/glean-film.html"), "utf8");
  const open = src.indexOf("<template>") + "<template>".length, close = src.lastIndexOf("</template>");
  const dom = new JSDOM(`<!DOCTYPE html><body><div id="__wrap">${src.slice(open, close)}</div></body>`);
  const d = dom.window.document, wrap = d.getElementById("__wrap");
  const q = (s) => { const e = wrap.querySelector(s); if (!e) throw new Error("missing " + s); return e; };
  const qa = (s, n) => { const l = [...wrap.querySelectorAll(s)]; if (n != null && l.length !== n) throw new Error(`${s}: expected ${n}, got ${l.length}`); return l; };
  const html = (s, v) => { q(s).innerHTML = v; };
  const br = (s) => s.replace(/\n/g, "<br>");
  q("#root").setAttribute("lang", c.htmlLang);

  // 品牌标志
  let logos = 0, icons = 0;
  for (const svg of qa("svg.agent-mark")) {
    const big = svg.querySelector("circle[r='50']");
    if (!big) continue;
    svg.setAttribute("aria-label", "dsh-ee");
    big.setAttribute("fill", BRAND);
    svg.querySelector("defs")?.remove();
    const g = svg.querySelector(":scope > g");
    g.setAttribute("transform", "translate(14 15) scale(3)");
    g.innerHTML = whale("#ffffff");
    logos++;
  }
  if (logos !== 3) throw new Error("logo count " + logos);
  const setIcon = (svg, key) => {
    const emoji = ICON[key];
    if (!emoji) throw new Error("no icon for " + key);
    const id = svg.querySelector("linearGradient").id, g = svg.querySelector(":scope > g");
    svg.setAttribute("aria-label", c.names[key] ?? "micro-app");
    g.setAttribute("transform", "translate(0 0) scale(1)");
    g.innerHTML = `<rect x="5" y="5" width="90" height="90" rx="25" fill="url(#${id})"/><rect x="5" y="5" width="90" height="90" rx="25" fill="#ffffff" fill-opacity=".38" stroke="${BRAND}" stroke-opacity=".28" stroke-width="1.5"/><text x="50" y="52" font-size="${emoji.length > 4 ? 40 : 50}" text-anchor="middle" dominant-baseline="central">${emoji}</text>`;
    icons++;
  };
  const appCard = (key) => `<div><b class="app-name">${c.names[key]}</b><span class="app-fact">${c.facts[key]}</span></div>`;

  // 1 开场
  html("#opening-text > span", `${c.opening[0]}<br><span id="opening-differently">${c.opening[1]}</span><br><span id="opening-ai">${c.opening[2]}</span>`);

  // 2 能力中心
  html("#intro-title > span", `${c.intro[0]}<span class="blue" data-var-text="product">dsh-ee</span>${c.intro[1]}`);
  html(".glean-g", `<svg viewBox="0 0 24 24" width="30" height="30" style="vertical-align:middle">${whale(BRAND)}</svg>`);
  q(".glean-g").className = "brand-mark";
  qa(".dash-shell .nav p", 4).forEach((p, i) => { p.innerHTML = `${["⌂", "◉", "♧", "▣"][i]}<br>${c.nav[i]}`; });
  html(".chat h3", c.chat.title);
  html(".chat .pill", c.chat.add);
  html(".chat .selected", c.chat.selected);
  html(".chat small", c.chat.recent);
  const recent = [c.names.brief, c.names.travel, c.names.onboarding, c.names.it, c.names.lore, c.chat.older, c.names.chapters, c.chat.contract];
  qa(".chat p", 8).forEach((p, i) => { p.textContent = recent[i]; });
  html(".dash-title", `${c.hub.title} <button>${c.hub.create}</button><small>${c.hub.how}</small>`);
  html(".tabs", c.hub.tabs);
  html("#dashboard .search", c.hub.search);
  html(".tags", c.hub.tags);
  qa(".group-label", 2).forEach((p, i) => { p.innerHTML = `${c.hub.groups[i]}<span>${c.hub.seeAll}</span>`; });
  const hubApps = [["app", "brief"], ["expert", "renewal"], ["expert", "contract"], ["app", "travel"], ["app", "onboarding"], ["app", "it"]];
  qa(".app-card", 6).forEach((card, i) => {
    const [kind, key] = hubApps[i];
    const sym = card.querySelector(".app-symbol");
    sym.textContent = ICON[key];
    sym.classList.add("app-emoji");
    card.querySelector("small").textContent = c.hub[kind];
    card.querySelector("b").textContent = c.names[key];
    card.querySelector("p").textContent = c.hubDesc[key];
  });

  // 3 应用围着标志
  const networkApps = ["brief", "travel", "onboarding", "contract", "it"];
  qa(".network-card .card-copy", 5).forEach((el, i) => { el.innerHTML = appCard(networkApps[i]); });
  qa(".network-card svg.agent-mark", 5).forEach((s, i) => setIcon(s, networkApps[i]));
  html("#network-title > span", c.networkTitle);

  // 4 部门
  html("#dept-title > span", c.dept.title);
  html("#dept-end > span", c.dept.end);
  ["#department-0", "#department-3", "#department-5", "#department-7"].forEach((s, i) => { q(s).textContent = c.dept.names[i]; });
  qa(".dept-icon svg.agent-mark", 5).forEach((s, i) => setIcon(s, ["brief", "renewal", "travel", "review", "it"][i]));
  // 头像：不用模板里的照片，换成示例员工的姓名缩写
  const initials = ["ZH", "CY", "ZX", "LY", "LN", "SY", "SM", "GQ"];
  const tones = ["#4d6bfe,#8de4de", "#f4a78e,#8061c3", "#19c97e,#a7d8e7", "#db4bea,#f6a98d", "#6142d2,#9cdeee"];
  qa("img.avatar").forEach((img, i) => {
    const el = d.createElement("div");
    el.className = "avatar avatar-initials";
    el.setAttribute("style", `${img.getAttribute("style")};background:linear-gradient(140deg,${tones[i % tones.length]})`);
    if (img.getAttribute("data-hf-id")) el.setAttribute("data-hf-id", img.getAttribute("data-hf-id"));
    el.textContent = initials[i % initials.length];
    img.replaceWith(el);
  });

  // 5、11 提问框
  for (const [id, full] of [["daily", c.daily], ["opportunities", c.renewal]]) {
    html(`#${id} .placeholder`, c.placeholder);
    q(`#${id} .typed`).setAttribute("data-full", br(full));
  }
  qa("#daily .icon-card svg.agent-mark", 4).forEach((s, i) => setIcon(s, ["travel", "audit", "contract", "onboarding"][i]));

  // 6 怎么工作
  html("#build-title > span", c.buildTitle);
  html("#start-tag", c.start);
  const marks = ["▷", "⊕", "▦", "◇", "♧", "⌕", "▤", "✦", "✦", "▤"];
  qa(".flow-card", 10).forEach((card, i) => { card.querySelector("small").innerHTML = `<span>${marks[i]}</span> ${c.flow[i][0]}`; card.querySelector("p").innerHTML = c.flow[i][1]; });
  html("#flow-output", c.output);

  // 7 应用编辑器
  const e = c.editor;
  html(".builder-shell header", `←　▣ ${c.names.brief} <span class="muted">●</span><div>⚙ ${e.settings}　 ▷ ${e.preview}　 <button>✓ ${e.publish}</button></div>`);
  html(".builder-shell aside", `<b>${e.kind}</b><p>${e.p1}</p><div class="search">${e.access}　↗</div><p>${e.p2}</p><p>${e.p3}</p><div class="edit-agent">${e.edit}　↑</div>`);
  qa(".builder-node", 5).forEach((n, i) => { n.querySelector("small").textContent = `♧　${e.nodes[i][0]}`; n.querySelector("p").innerHTML = e.nodes[i][1]; });

  // 8 小团队和企业
  html("#org-title > span", c.orgTitle);
  const orgApps = ["lore", "chapters", "renewal", "outing", "weekly"];
  qa(".org-card .card-copy", 5).forEach((el, i) => { el.innerHTML = appCard(orgApps[i]); });
  qa(".org-card svg.agent-mark", 5).forEach((s, i) => setIcon(s, orgApps[i]));
  html(".portrait-one", `<div class="portrait-tile tile-team">${c.tiles[0]}</div>`);
  html(".portrait-two", `<div class="portrait-tile tile-company">${c.tiles[1]}</div>`);

  // 9 出处，10 系统和文档
  html("#knowledge-title > span", c.knowledge);
  html("#both > span", c.data[0]);
  html("#world-copy > span", c.data[1]);
  html("#work-copy > span", c.data[2]);

  // 12 回答
  html("#report h2", br(c.renewal));
  qa(".report-steps p", 3).forEach((p, i) => { p.textContent = `◉　${c.report.steps[i]}`; });
  qa("#report th", 3).forEach((th, i) => { th.textContent = c.report.th[i]; });
  const rows = qa("#report tbody tr", 4);
  [...rows[0].children].forEach((td, i) => { td.textContent = c.report.row[i]; });
  rows.slice(1).forEach((tr) => { const td = tr.children[2]; const bar = td.querySelector("i"); td.textContent = ""; td.appendChild(bar); });
  html("#report .source", `▣ ${c.report.source}`);
  q(".report-prompt").firstChild.textContent = c.placeholder;

  // 13 管控
  html("#lock-title > span", c.built);
  html("#governance-copy > span", c.gov);

  // 14 卡片墙
  for (const card of qa(".mosaic-card")) {
    const copy = card.querySelector(".card-copy");
    const key = MOSAIC[copy.textContent.trim().replace(/\s+/g, " ")];
    if (!key) throw new Error("unmapped mosaic text: " + copy.textContent);
    copy.textContent = c.names[key];
    setIcon(card.querySelector("svg.agent-mark"), key);
  }
  qa(".mosaic-line", 3).forEach((l, i) => { l.textContent = c.closing[i]; });
  if (logos + icons !== qa("svg.agent-mark").length) throw new Error("some icons were not assigned");

  // 15 片尾
  const wm = q("#wordmark");
  wm.setAttribute("style", "left:390px;top:262px;width:500px;height:196px");
  wm.innerHTML = `<div class="wordmark-row"><svg viewBox="0 0 24 24" width="104" height="104">${whale(BRAND)}</svg><span>dsh-ee</span></div><div class="wordmark-sub">demo.hikariming.com　·　github.com/hikariming/dsh-ee</div>`;

  // 样式和脚本里的文字替换
  let body = wrap.innerHTML;
  const swap = (a, b, min = 1) => { const n = body.split(a).length - 1; if (n < min) throw new Error(`"${a}" found ${n} times`); body = body.split(a).join(b); };
  for (const color of ["#3729eb", "#3828ed", "#3828eb", "#3829ee", "#3828ee", "#3929e8"]) swap(color, BRAND);
  swap("#4d48ef,#2620bd", "#7f95ff,#3550e6");
  swap("GleanSans", "FilmSans", 3);
  swap(".glean-g{", ".brand-mark{");
  // 打字的内容从画面上的 data-full 读，脚本不带任何语言的文字
  swap("full=id==='daily'?(settings.dailyPrompt||'Prepare me for my day'):(settings.opportunityPrompt||'What are the top 10 opportunities open\\nwith Acme Corp?')", "full=(id==='daily'?settings.dailyPrompt:settings.opportunityPrompt)||($(`#${id} .typed`).dataset.full||'').replace(/<br>/g,'\\n')");
  swap("$('#report h2').textContent=settings.opportunityPrompt||'What are the top 10 opportunities open\\nwith Acme Corp?';", "if(settings.opportunityPrompt)$('#report h2').textContent=settings.opportunityPrompt;");
  swap("</style>", `${EXTRA_CSS}</style>`);
  const rest = body.replace(/glean-film/g, "");
  if (/glean|acme/i.test(rest)) throw new Error("leftover template text: " + rest.match(/.{30}(glean|acme).{30}/i)[0]);
  const composition = src.slice(0, open) + body + src.slice(close);

  let index = readFileSync(join(template, "index.html"), "utf8");
  const vars = [
    { id: "product", type: "string", label: "Product name", default: "dsh-ee" },
    { id: "brand", type: "color", label: "Brand blue", default: BRAND },
    { id: "dailyPrompt", type: "string", label: "First question", default: c.daily },
    { id: "opportunityPrompt", type: "string", label: "Second question", default: c.renewal },
    { id: "builtTitle", type: "string", label: "Governance title", default: c.built },
    { id: "governanceLine", type: "string", label: "Governance line", default: c.gov },
  ];
  index = index.replace(/data-composition-variables='[^']*'/, `data-composition-variables='${JSON.stringify(vars).replace(/&/g, "&amp;").replace(/'/g, "&#39;")}'`);
  index = index.replace(/<title>[^<]*<\/title>/, "<title>dsh-ee product film</title>");
  index = index.replace('<html lang="en"', `<html lang="${c.htmlLang}"`);
  // 模板的配乐是原广告的音乐，不带
  index = index.replace(/<audio[^>]*id="original-soundtrack"[^>]*><\/audio>/, "");
  if (/<audio/.test(index) || /Glean/.test(index)) throw new Error("index.html edit incomplete");
  return { composition, index, body };
}

const banner = "// 由 promo/openmotion/build-film.mjs 生成，不要手改：改文字去改那个脚本里的 COPY，再重新生成。\n";
const webDir = join(repo, "web/src/components/product-film/generated");
mkdirSync(webDir, { recursive: true });
let shared = null;
for (const lang of Object.keys(COPY)) {
  const { composition, index, body } = build(lang);
  const dir = join(repo, `promo/out/dsh-ee-openmotion-film-${lang}`);
  rmSync(dir, { recursive: true, force: true });
  mkdirSync(join(dir, "compositions"), { recursive: true });
  mkdirSync(join(dir, "assets"), { recursive: true });
  writeFileSync(join(dir, "index.html"), index);
  writeFileSync(join(dir, "compositions/glean-film.html"), composition);
  writeFileSync(join(dir, "hyperframes.json"), readFileSync(join(template, "hyperframes.json")));
  cpSync(join(template, "assets/fonts"), join(dir, "assets/fonts"), { recursive: true });
  cpSync(join(template, "assets/gsap.min.js"), join(dir, "assets/gsap.min.js"));

  // 首页用的那一份：画面（去掉编辑器的 data-hf-id）、样式、时间轴分开
  const css = body.slice(body.indexOf("<style>") + 7, body.indexOf("</style>"));
  const markup = body.slice(body.indexOf("</style>") + 8, body.indexOf("<script>")).replace(/ data-hf-id="[^"]*"/g, "").trim();
  const script = body.slice(body.indexOf("<script>") + 8, body.lastIndexOf("</script>"));
  writeFileSync(join(webDir, `film.${lang}.ts`), `${banner}export const html = ${JSON.stringify(markup)};\n`);
  if (!shared) {
    // 字体在页面里注册（影子 DOM 里的 @font-face 不生效）；html,body 的字体和颜色挪到宿主上
    const webCss = css.replace(/@font-face\{[^}]*\}\s*/g, "").replace("html,body{", ":host{all:initial;display:block;");
    if (webCss.includes("@font-face") || !webCss.includes(":host{")) throw new Error("web css rewrite failed");
    const head = script.indexOf("(() => {"), tail = script.indexOf(" const clock={time:0};");
    if (head < 0 || tail < 0) throw new Error("runtime rewrite failed");
    let runtime = script.slice(head + "(() => {".length, tail);
    const patch = (a, b) => { if (!runtime.includes(a)) throw new Error("runtime: missing " + a); runtime = runtime.replace(a, () => b); };
    patch("const $=s=>document.querySelector(s), $$=s=>[...document.querySelectorAll(s)];", "const $=s=>root.querySelector(s), $$=s=>[...root.querySelectorAll(s)];");
    patch("const settings=window.__hyperframes?.getVariables?.()||{};", "const settings={};");
    if (/\b(document|window|gsap)\b/.test(runtime)) throw new Error("runtime still touches the page");
    shared = { webCss, runtime };
    writeFileSync(join(webDir, "film-style.ts"), `${banner}export const css = ${JSON.stringify(webCss)};\n`);
    writeFileSync(join(webDir, "film-runtime.ts"), `// @ts-nocheck\n${banner}// 整部片子是时间的纯函数：createFilm(root) 返回 draw(秒)，root 是放着画面的影子根。\nexport const FILM_SECONDS = 61;\nexport const FILM_WIDTH = 1280;\nexport const FILM_HEIGHT = 720;\nexport function createFilm(root) {${runtime} return draw;\n}\n`);
  }
  console.log(`${lang}: composition ${composition.length} bytes, web markup ${markup.length} bytes`);
}
