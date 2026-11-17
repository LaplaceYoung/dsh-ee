// 由 dsh-plugin/ 生成，勿手改。

var __create = Object.create;
var __defProp = Object.defineProperty;
var __getOwnPropDesc = Object.getOwnPropertyDescriptor;
var __knownSymbol = (name2, symbol) => (symbol = Symbol[name2]) ? symbol : Symbol.for("Symbol." + name2);
var __typeError = (msg) => {
  throw TypeError(msg);
};
var __defNormalProp = (obj, key, value) => key in obj ? __defProp(obj, key, { enumerable: true, configurable: true, writable: true, value }) : obj[key] = value;
var __name = (target, value) => __defProp(target, "name", { value, configurable: true });
var __decoratorStart = (base) => [, , , __create(base?.[__knownSymbol("metadata")] ?? null)];
var __decoratorStrings = ["class", "method", "getter", "setter", "accessor", "field", "value", "get", "set"];
var __expectFn = (fn) => fn !== void 0 && typeof fn !== "function" ? __typeError("Function expected") : fn;
var __decoratorContext = (kind, name2, done, metadata, fns) => ({ kind: __decoratorStrings[kind], name: name2, metadata, addInitializer: (fn) => done._ ? __typeError("Already initialized") : fns.push(__expectFn(fn || null)) });
var __decoratorMetadata = (array, target) => __defNormalProp(target, __knownSymbol("metadata"), array[3]);
var __runInitializers = (array, flags, self, value) => {
  for (var i = 0, fns = array[flags >> 1], n = fns && fns.length; i < n; i++) flags & 1 ? fns[i].call(self) : value = fns[i].call(self, value);
  return value;
};
var __decorateElement = (array, flags, name2, decorators, target, extra) => {
  var fn, it, done, ctx, access, k = flags & 7, s = !!(flags & 8), p = !!(flags & 16);
  var j = k > 3 ? array.length + 1 : k ? s ? 1 : 2 : 0, key = __decoratorStrings[k + 5];
  var initializers = k > 3 && (array[j - 1] = []), extraInitializers = array[j] || (array[j] = []);
  var desc = k && (!p && !s && (target = target.prototype), k < 5 && (k > 3 || !p) && __getOwnPropDesc(k < 4 ? target : { get [name2]() {
    return __privateGet(this, extra);
  }, set [name2](x) {
    return __privateSet(this, extra, x);
  } }, name2));
  k ? p && k < 4 && __name(extra, (k > 2 ? "set " : k > 1 ? "get " : "") + name2) : __name(target, name2);
  for (var i = decorators.length - 1; i >= 0; i--) {
    ctx = __decoratorContext(k, name2, done = {}, array[3], extraInitializers);
    if (k) {
      ctx.static = s, ctx.private = p, access = ctx.access = { has: p ? (x) => __privateIn(target, x) : (x) => name2 in x };
      if (k ^ 3) access.get = p ? (x) => (k ^ 1 ? __privateGet : __privateMethod)(x, target, k ^ 4 ? extra : desc.get) : (x) => x[name2];
      if (k > 2) access.set = p ? (x, y) => __privateSet(x, target, y, k ^ 4 ? extra : desc.set) : (x, y) => x[name2] = y;
    }
    it = (0, decorators[i])(k ? k < 4 ? p ? extra : desc[key] : k > 4 ? void 0 : { get: desc.get, set: desc.set } : target, ctx), done._ = 1;
    if (k ^ 4 || it === void 0) __expectFn(it) && (k > 4 ? initializers.unshift(it) : k ? p ? extra = it : desc[key] = it : target = it);
    else if (typeof it !== "object" || it === null) __typeError("Object expected");
    else __expectFn(fn = it.get) && (desc.get = fn), __expectFn(fn = it.set) && (desc.set = fn), __expectFn(fn = it.init) && initializers.unshift(fn);
  }
  return k || __decoratorMetadata(array, target), desc && __defProp(target, name2, desc), p ? k ^ 4 ? extra : desc : target;
};
var __accessCheck = (obj, member, msg) => member.has(obj) || __typeError("Cannot " + msg);
var __privateIn = (member, obj) => Object(obj) !== obj ? __typeError('Cannot use the "in" operator on this value') : member.has(obj);
var __privateGet = (obj, member, getter) => (__accessCheck(obj, member, "read from private field"), getter ? getter.call(obj) : member.get(obj));
var __privateSet = (obj, member, value, setter) => (__accessCheck(obj, member, "write to private field"), setter ? setter.call(obj, value) : member.set(obj, value), value);
var __privateMethod = (obj, member, method) => (__accessCheck(obj, member, "access private method"), method);

// src/config.ts
import os from "node:os";
import path from "node:path";

// src/i18n/zh.ts
var zh = {
  // 多个模块共用的词（视图标题/按钮/空态）；页面专属文案放各自命名空间
  common: {
    login: "登录",
    refresh: "刷新",
    retry: "重试",
    save: "保存",
    submit: "提交",
    close: "关闭",
    hide: "隐藏",
    loading: "加载中…",
    noData: "暂无数据",
    notSet: "未设置",
    viewInWeb: "在网页查看",
    delete: "删除",
    newBadge: "新",
    inProgress: "进行中",
    pendingDecision: "待你决定",
    working: "工作中",
    turnEnded: "本轮结束",
    viewMoreCount: "查看更多（{{n}}）",
    listSeparator: "、"
  },
  // client.ts：客户端插件入口（远程调用兜底、登录/诊断提示、默认企业名）
  client: {
    remoteFailed: "企业远程调用失败",
    overviewTimeout: "读取企业账号超时",
    brandTimeout: "读取企业品牌超时",
    defaultOrg: "企业",
    loginInBrowser: "请在浏览器完成企业登录。",
    loginStartFailed: "发起登录失败，请检查网络后重试。",
    diagnoseFailed: "诊断失败，请检查网络后重试。"
  },
  // apps-client.ts：企业应用目录的客户端状态与操作提示
  apps: {
    copied: "已复制 {{command}}，粘贴到输入框发送",
    filled: "已填进输入框，确认后发送",
    loginInBrowser: "请在浏览器完成企业登录。",
    loginStartFailed: "发起登录失败，请稍后重试。",
    openWorkbenchFirst: "先点应用回到它的会话，再打开工作台",
    workbenchNotOpen: "工作台没有打开，请稍后再试",
    enableFailed: "启用失败：{{error}}",
    enabled: "已启用「{{title}}」，点一下开始使用",
    retryLater: "请稍后重试",
    enableFailedNetwork: "启用失败，请检查网络后重试。",
    disableFailed: "停用失败：{{error}}",
    disabled: "已停用「{{title}}」",
    disableFailedNetwork: "停用失败，请检查网络后重试。",
    unbound: "已解除绑定",
    unbindFailed: "解除绑定失败，请稍后重试。",
    enterpriseApps: "企业应用"
  },
  // commands.ts：/ee-login /ee-status /ee-logout 的对话文案与命令描述
  commands: {
    pending: "请在浏览器确认，确认码 {{code}}，链接 {{url}}",
    starting: "正在发起企业登录，请稍候…",
    badSetupCode: "接入码格式不对，应形如 ABCD-EFGH；不带接入码直接输入 /ee-login 也可以",
    setupConfirm: "回到 dsh-ee 网页的「开始使用」页，核对确认码 {{code}} 后点「确认授权」（网页关了的话打开 {{url}}）。确认后回到这里继续对话。",
    loginDescription: "登录企业账号（可以带上网页「开始使用」页给的接入码）",
    setupCodeHint: "[接入码]",
    statusDescription: "查看企业登录状态",
    notLoggedIn: "未登录，在对话框输入 /ee-login",
    unreachable: "连不上 dsh-ee，请检查网络或 VPN 后重试",
    sessionExpired: "登录已失效，在对话框输入 /ee-login",
    httpError: "dsh-ee 返回 HTTP {{status}}，稍后再试（不是登录失效）",
    expiresOn: " · {{date}} 到期",
    neverExpires: " · 长期有效",
    statusLine: "已登录 {{username}}（{{department}}）· 令牌 {{token}}{{expiry}} · 插件 {{version}} · npmrc {{npmrc}}",
    npmrcMatch: "一致",
    npmrcMismatch: "不一致",
    logoutDescription: "退出企业账号",
    noAccount: "当前没有登录企业账号",
    revokedPrefix: "已在服务端吊销该令牌，",
    envToken: "但它来自环境变量或 $DSH_HOME/.env（{{ref}}），请手动删除后重启 DSH",
    loggedOut: "已退出企业账号",
    loggedOutOffline: "本机已退出；连不上 dsh-ee，令牌未能在服务端吊销，可在网页「我的令牌」里吊销"
  },
  // doctor.ts：ee_local_doctor 诊断项与修复建议
  doctor: {
    appBindingsFix: "重新打开微应用会建立新绑定；原损坏文件无需手动删除",
    noCredential: "本机没有企业登录凭据",
    loginFix: "在对话框输入 /ee-login 登录",
    hasCredential: "本机有企业登录凭据",
    source: "来源：{{source}}",
    expiringSoon: "登录将在 {{days}} 天内到期",
    reLoginFix: "到期前输入 /ee-login 重新登录",
    signedInAs: "已登录 {{username}}（{{department}}）",
    loginExpired: "企业登录已失效",
    reLoginFix2: "在对话框输入 /ee-login 重新登录",
    readAccountFailed: "读取企业账号失败（HTTP {{status}}）",
    serverRestarting: "dsh-ee 可能正在重启或暂时不可用；不是登录失效，稍后再试",
    retryDiagnose: "稍等片刻重新运行诊断；持续失败请检查服务端状态",
    unreachable: "连不上 dsh-ee（{{base}}）",
    unreachableFix: "检查网络或 VPN；地址不对的话在 DSH 的「设置 → dsh-ee」里改成浏览器能打开的 dsh-ee 地址并测试",
    npmrcOk: "DSH 的 .npmrc 里企业源配置与当前登录一致",
    npmrcMissing: "DSH 的 .npmrc 里缺少企业 npm 源配置",
    npmrcMismatch: "DSH 的 .npmrc 里企业源令牌与当前登录不一致",
    npmrcDetail: "{{path}}；装企业插件会因 401 失败",
    npmrcFix: "重启 DSH 或重新登录会自动同步",
    modelsAvailable: "可用模型 {{models}}",
    noModels: "没有可用的模型",
    readModelsFailed: "读取模型列表失败",
    readModelsHttp: "读取模型列表失败（HTTP {{status}}）",
    modelPermissionFix: "联系管理员开通模型权限",
    defaultModelMissing: "企业默认模型 {{model}} 不在你的可用列表里",
    chooseModelFix: "在 DSH 里改选可用的模型",
    pluginNewer: "接入插件有新版本 {{latest}}（当前 {{version}}）",
    updateFix: "在 DSH 的「设置 → dsh-ee」点「更新到最新版」，装好后重启 DSH",
    pluginLatest: "接入插件 {{version}} 已是最新",
    missingPlugins: "缺少必装插件 {{plugins}}",
    missingPluginsFix: "在 DSH 插件页安装，或重启 DSH 让接入插件自动安装",
    pluginsOk: "必装插件都已安装",
    bridgeFailed: "企业工具同步失败",
    bridgeFix: "检查网络；输入 /ee-status 看登录状态",
    bridgeOk: "企业工具已同步（{{count}} 个）",
    summaryFail: "{{fails}} 项失败，{{warns}} 项需要注意",
    summaryWarn: "{{warns}} 项需要注意",
    summaryOk: "全部正常",
    description: "检查这台电脑上 DSH 与企业工作台 dsh-ee 的接入：登录凭据、令牌是否有效、.npmrc、接入插件版本、必装插件、模型、企业工具同步，并附带服务端检查。只读，不修改任何东西。"
  },
  // status-view.ts：右侧企业面板标题栏与头部按钮
  statusView: {
    adminOn: "已开启",
    adminOff: "未开启",
    adminRemainingHours: "还剩 {{hours}} 小时 {{minutes}} 分钟",
    adminRemainingMinutes: "还剩 {{minutes}} 分钟",
    failureAuth: "还没有登录企业账号，或登录已失效，请登录。",
    failureServer: "dsh-ee 返回错误（{{detail}}），请稍后刷新。",
    failureUnreachable: "连不上 dsh-ee（{{base}}，{{detail}}）。请确认这台电脑能在浏览器里打开这个地址；地址不对的话，在 DSH 的「设置 → dsh-ee」里改成能打开的地址并测试。",
    failureDefault: "未登录或连接不可用，请重新登录。",
    reading: "正在读取企业账号…",
    startingLogin: "正在发起登录…",
    account: "账号",
    department: "部门",
    tokenExpiry: "令牌有效期",
    adminMode: "管理模式",
    expiresOn: "{{date}} 到期",
    neverExpires: "长期有效",
    quota: "本月额度",
    quotaUnavailable: "额度信息不可用",
    quotaUnlimited: "不限额",
    quotaRemaining: "剩余 {{percent}}%",
    models: "企业模型",
    defaultModel: "默认：{{model}}",
    modelsUnavailable: "没有可用模型或模型列表不可用",
    status: "接入状态",
    pluginVersion: "插件 {{version}}{{latest}}",
    pluginNewVersion: " · 新版本 {{version}}",
    pluginUpToDate: " · 已是最新",
    diagnosing: "诊断中…",
    diagnose: "诊断",
    diagnoseResult: "诊断结果：{{summary}}",
    checkLine: "{{title}}{{fix}}",
    checkFixSeparator: "；",
    headerDisconnected: "企业 · 未登录",
    headerChecking: "企业 · 检查中",
    headerAdmin: "管理模式 · {{remaining}}",
    headerQuota: "企业模型 · 剩余 {{percent}}%",
    headerUser: "企业模型 · {{username}}",
    headerTitleUser: "{{username}}{{department}}",
    headerDepartment: "（{{department}}）",
    headerTitle: "重新登录企业账号",
    guideDescription: "查看企业账号、额度、模型与接入状态"
  },
  // turn-writes-view.ts：一轮末尾「已留痕」提示
  turnWrites: {
    failedOp: "{{label}} 未成功{{count}}",
    failedCount: " ×{{count}}",
    failedNotice: "这一轮在企业系统里的写操作（有未成功的），已在 dsh-ee 留痕",
    okNotice: "这一轮改了企业系统里的数据，已在 dsh-ee 留痕",
    systemLabel: "「{{system}}」"
  },
  // votes.ts：助手消息尾的 👍👎 反馈
  votes: {
    notePrompt: "这次回答哪里没帮上忙？（可留空，会按问题聚合给应用作者）",
    helpful: "有帮助",
    notHelpful: "没帮上忙",
    label: "评价这次回答"
  },
  // write-log.ts：本机写操作日志
  writeLog: {
    invalidSessionId: "会话 id 不合法"
  },
  // overview.ts：读取企业账号失败原因
  overview: {
    timeout: "连接超时",
    toolFailed: "企业工具调用失败",
    toolNoJson: "企业工具没有返回 JSON"
  },
  // config.ts：插件配置默认值与校验
  config: {
    badBase: "[dsh-ee] base 必须是 http(s) 地址",
    defaultOrg: "企业"
  },
  // app-launcher.ts：启动微应用时的发送结果不确定提示
  appLauncher: {
    sendUnknown: "消息可能没有发出，请查看会话后再试"
  },
  // host-session.ts：宿主会话兼容层的失败提示
  hostSession: {
    switchUnsupported: "宿主不支持切换会话",
    noCwd: "当前会话没有可用于新建会话的工作目录",
    createWorkspaceUnsupported: "宿主不支持创建工作区",
    noWorkspace: "无法为新会话取得工作区",
    createSessionUnsupported: "宿主不支持新建会话",
    rejected: "宿主拒绝了消息",
    sendUnsupported: "宿主不支持发送消息"
  },
  // device.ts：设备码登录
  device: {
    startFailed: "设备登录发起失败：{{error}}",
    startHttpFailed: "设备登录发起失败：HTTP {{status}}",
    cancelled: "设备登录已取消",
    failed: "设备登录失败：HTTP {{status}}",
    timeout: "设备登录超时"
  },
  // connect-update.ts：一键更新接入插件
  connectUpdate: {
    noManager: "这台 DSH 没有插件管理接口，请在插件页手动更新接入插件",
    noLatest: "查不到企业端的接入插件版本：{{why}}{{hint}}",
    signInHint: "；先登录企业账号再试",
    notPublished: "企业端还没有发布接入插件，请管理员先在系统设置里发布",
    upToDate: "已是最新（{{version}}）",
    installed: "已安装 {{pkg}}@{{latest}}，重启 DSH 后生效",
    installFailed: "安装失败：{{error}}"
  },
  // upload.ts：ee_plugins.upload 上传 .tgz
  upload: {
    missingPath: "缺少 path：.tgz 文件的本机路径",
    notTarball: "只能上传 npm pack 生成的 .tgz 文件",
    fileNotFound: "找不到文件：{{file}}",
    notAFile: "不是文件：{{file}}",
    tooLarge: "文件太大",
    timeout: "上传超时，请重试；网络较慢时可改用网页上传",
    unreachable: "连不上 dsh-ee，请检查网络或 VPN",
    httpFailed: "上传失败（HTTP {{status}}）"
  },
  // ee-save.ts：/ee-save 命令描述（提炼请求正文本身保留中文，e2e 按原文比对）
  eeSave: {
    description: "把这段对话沉淀成能力（提示词 / 技能 / 应用草稿）"
  },
  // app-bindings.ts：微应用会话绑定文件
  appBindings: {
    invalidAppName: "微应用名不合法",
    invalidSessionId: "会话 id 不合法",
    corruptFile: "微应用会话绑定文件已损坏，已按空绑定处理（{{file}}）"
  },
  // app-subscription.ts：启用/停用自选微应用
  appSubscription: {
    invalidAppName: "微应用名不合法",
    invalidSkillName: "技能名不合法",
    invalidConnectionName: "连接名不合法",
    invalidKind: "能力类型不合法"
  },
  // apps-view.ts：企业应用侧栏/首屏横排/「查看更多」浮层/进行中
  appsView: {
    startersTitle: "从示例问题开始",
    confirmDisable: "确认停用",
    disable: "停用",
    newSession: "新开一个会话",
    openWorkbench: "打开工作台",
    fillInput: "填进输入框",
    unbind: "解除绑定",
    moreActions: "{{title}}更多操作",
    missingList: "不可用：{{missing}}",
    missing: "部分能力不可用",
    missingHint: "{{title}}。点击查看并申请",
    missingApply: "部分能力不可用 · 申请",
    todoBadge: "{{count}} 待处理",
    openArrow: "打开 →",
    enable: "启用",
    gallerySubtitle: "{{count}} 个应用 · 点一个开始，AI 会带着工作台一起打开",
    searchAppsAria: "搜索企业应用",
    searchAppsPlaceholder: "搜索应用…",
    noMatches: "没有匹配的应用",
    availableCount: "可启用（{{count}}）",
    showInSidebar: "在左侧栏显示企业应用",
    loginToSee: "登录后查看企业应用",
    expandApps: "展开企业应用",
    collapseApps: "收起企业应用",
    hideSidebar: "在左侧栏隐藏企业应用（在「查看更多」或 设置 → dsh-ee 里可以再打开）",
    collapsedCount: "（{{count}}）",
    allApps: "全部应用",
    availablePlus: "＋ 可启用（{{count}}）",
    heroSearchPlaceholder: "搜索…",
    itemTitle: "{{title}}：{{label}}",
    todosNeed: "需要你处理：{{count}} 项",
    startHint: "点一个开始，工作台会在右侧打开",
    tabApps: "应用",
    tabSkills: "技能",
    tabConnections: "连接器",
    skillsSubtitle: "{{count}} 个技能 · 启用后在每个会话里都能用",
    connectionsSubtitle: "{{count}} 个连接器 · 让 AI 直接查询、操作企业系统",
    searchSkillsPlaceholder: "搜索技能…",
    searchConnectionsPlaceholder: "搜索连接器…",
    noSkills: "还没有可用的技能",
    noConnections: "还没有可用的连接器",
    featured: "精选",
    statusAuto: "默认生效",
    statusEnabled: "已启用",
    statusOff: "未启用",
    statusNeedsAuth: "待授权",
    toolsCount: "{{count}} 个工具",
    readWrite: "可读写",
    readOnly: "只读",
    authorize: "去授权",
    preview: "预览",
    details: "详情",
    needsAuthHint: "有 {{count}} 个连接器要先授权才能用，授权后重新打开这里",
    capEnabled: "已启用「{{title}}」"
  },
  // workbench-client.ts：业务工作台标签页（面板/原文窗格、写表、清单、继续追问）
  workbenchClient: {
    fallbackTitle: "工作台",
    noSession: "没有找到当前会话，请先打开这个应用的会话",
    sentToAi: "已发给 AI：{{label}}",
    networkError: "网络错误",
    noSessionCheck: "没有找到当前会话，勾选不会保存",
    checkNotSaved: "勾选没有保存，请稍后再试",
    updatePlugin: "请把接入插件更新到最新版本",
    saved: "已保存：{{label}}",
    saveFailedNetwork: "没有保存，请检查网络后重试",
    writeFailed: "没有{{action}}：{{error}}",
    deletedRow: "已删除这一行",
    updated: "已更新：{{label}}",
    docLabel: "文档 {{doc}}",
    askPrompt: "接着看知识库文档「{{title}}」（文档编号 {{doc}}）第 {{from}}～{{to}} 行前后的内容：这一部分还有哪些相关规定或例外情况？请带出处回答。",
    ask: "在这篇里继续问",
    saveFailedDefault: "没有保存，请把接入插件更新到最新版本"
  },
  // settings-view.ts：DSH 设置里的「dsh-ee」页（地址、测试、更新、侧栏开关）
  settingsView: {
    title: "dsh-ee 连接",
    description: "接入插件通过这个地址登录企业账号、同步企业技能和微应用、调用企业模型。连不上时，先在浏览器里打开 dsh-ee 网页，把能打开的地址填在这里。",
    reading: "正在读取…",
    addressLabel: "dsh-ee 地址",
    packagedOverridden: "插件包里的地址是 {{packaged}}，你改成了现在这个。",
    packagedDefault: "这是管理员发布接入插件时写进去的地址。",
    testing: "正在测试…",
    testConnection: "测试连接",
    resetAddress: "恢复插件包里的地址",
    plugin: "接入插件",
    currentVersion: "当前版本 {{version}}",
    updating: "正在更新…",
    updateLatest: "更新到最新版",
    updateHint: "更新到企业端已发布的最新版；装好后需要重启 DSH 才生效。",
    changeHint: "改地址只影响这台电脑。如果大家都连不上，请管理员在 dsh-ee 网页的系统设置里填写「访问地址」，再重新发布接入插件。",
    readFailed: "读取设置失败：{{error}}",
    testFailed: "测试失败：{{error}}",
    saveFailed: "保存失败：{{error}}",
    updateFailed: "更新失败：{{error}}",
    savedNotice: "已保存并生效。企业技能、微应用和工具正在按新地址同步；登录无效的话，去侧栏企业面板重新登录。",
    sidebarHint: "左侧栏只列前 3 个应用，其余在「查看更多」里；隐藏后首屏输入框下方和右侧企业面板里照样能打开企业应用。",
    saving: "正在保存…"
  },
  // server-settings.ts：dsh-ee 地址的保存与连接测试
  serverSettings: {
    reasonTimeout: "连接超时：这台电脑访问不到这个地址（内网地址、VPN 没连，或者被防火墙挡住）",
    reasonRefused: "连接被拒绝：地址能找到，但这个端口上没有服务",
    reasonDns: "找不到这个域名：检查拼写，或者是否需要连 VPN",
    reasonTls: "HTTPS 证书有问题（{{code}}）",
    badAddress: "地址格式不对",
    badAddressDetail: "要写完整的 http:// 或 https:// 地址，例如 https://ee.example.com，不要带路径以外的参数",
    unreachable: "连不上",
    reachable: "能连上（{{ms}} ms）",
    notDshEe: "这个地址上不是 dsh-ee",
    notDshEeDetail: "访问 /healthz 返回 HTTP {{status}}；确认地址是 dsh-ee 网页的地址",
    isDshEe: "是 dsh-ee{{version}}",
    versionSuffix: "（{{version}}）",
    noLogin: "这台电脑还没有登录企业账号",
    noLoginDetail: "保存地址后，在侧栏企业面板点「登录」，或在对话框输入 /ee-login",
    signedIn: "已登录 {{username}}{{department}}",
    departmentSuffix: "（{{department}}）",
    loginInvalid: "本机的登录在这台 dsh-ee 上无效",
    loginInvalidDetail: "保存地址后重新登录即可（令牌是按 dsh-ee 发的，换了服务器要重新登录）",
    loginCheckFailed: "检查登录失败",
    saveBadAddress: "地址格式不对：要写完整的 http:// 或 https:// 地址"
  },
  // workbench.ts：工作台数据格式化、占位符、表单与看板校验（纯函数）
  workbench: {
    unitHundredMillion: " 亿",
    unitTenThousand: " 万",
    unitBillion: " 十亿",
    unitMillion: " 百万",
    flat: "持平",
    allKbs: "全部知识库",
    scopePicked: "知识库{{titles}}（标识 {{ids}}）",
    kbTitle: "「{{title}}」",
    formRequired: "请填写「{{label}}」",
    formNumber: "「{{label}}」要填数字",
    formSelect: "「{{label}}」请从选项里选",
    blocksRequired: "blocks 必须是非空数组",
    blocksTooMany: "看板最多 20 块",
    boardTooLarge: "看板内容不能超过 32 KB",
    blockWhere: "第 {{index}} 块",
    textEmpty: "{{where}}：text 不能为空",
    itemsRange: "{{where}}：items 要有 1～50 项",
    columnsRange: "{{where}}：columns 要有 1～12 列",
    rowsRange: "{{where}}：rows 必须是最多 50 行的二维数组",
    citationsRange: "{{where}}：items 要有 1～30 条引用",
    citationAt: "{{where}}第 {{index}} 条引用",
    citationNumbers: "{{at}}：doc、from、to 要是正整数，且 to 不小于 from",
    citationVersion: "{{at}}：version 要是正整数",
    citationQuote: "{{at}}：quote 要写 1～500 字的原文摘句",
    blockType: "{{where}}：type 只能是 text、list、table 或 citations"
  },
  // workbench-host.ts：ee_workbench 工具与面板取数/写表
  workbenchHost: {
    noComponent: "目录里没有这个组件",
    notReadOnly: "这个数据源不是只读工具，面板不能用它取数",
    formNoTable: "这个表单没有配置提交到表",
    rowNoAction: "这个按钮不能改表",
    rowNoId: "这一行没有编号",
    badDoc: "文档编号不对",
    invalidAppName: "微应用名不合法",
    invalidSessionId: "会话 id 不合法",
    invalidChecklistKey: "清单编号不合法",
    actionInvalid: "action 只能是 board 或 checklist",
    appNotFound: "没有微应用 {{app}}，或它不在你的可用范围内",
    noBoard: "微应用 {{app}} 的工作台没有 AI 看板",
    noBoardSession: "找不到要更新看板的会话",
    boardUpdated: "已更新工作台看板",
    badCitations: "引用 {{numbers}} 对不上原文：请用 kb__read 重新读那几行，改正后再写一次看板，并在对话里向用户更正。",
    staleCitations: "引用 {{numbers}} 的文档已经更新，请读最新版本确认结论没有变。",
    checklistUnsupported: "这个版本的接入插件不支持清单",
    noChecklist: "微应用 {{app}} 的工作台没有清单",
    checklistNotFound: "没有标题是「{{title}}」的清单；可选：{{options}}",
    checklistAmbiguous: "有 {{count}} 个清单，请用 title 指定：{{options}}",
    checklistOption: "「{{title}}」",
    doneOrUndone: "done 与 undone 至少填一个",
    itemsNotFound: "清单「{{title}}」里没有条目 {{unknown}}；条目编号：{{known}}",
    noChecklistSession: "找不到要更新清单的会话",
    checklistUpdated: "已更新清单「{{title}}」：完成 {{done}}/{{total}}",
    checklistMarked: "已更新清单「{{title}}」：已完成 {{count}} 项",
    description: "更新当前微应用工作台：action=board 把本轮处理结果写到「AI 看板」（每次整体替换，citations 块里的知识库引用会自动核对）；action=checklist 勾选或取消工作台清单里的条目（done / undone 填条目编号）。不会改任何业务数据。",
    paramApp: "微应用名，也就是 /名字 里的名字",
    paramTitle: "checklist：清单的标题；工作台只有一个清单时可以不填",
    paramDone: "checklist：标成完成的条目编号",
    paramUndone: "checklist：撤回完成的条目编号",
    paramBlocks: "看板内容，从上到下显示；只当文字显示，不支持 HTML",
    paramItems: "list：每项一段文字；citations：每项一条知识库引用 {doc, version, from, to, quote, title}"
  },
  // workbench-view.ts：业务工作台面板（指标/表格/图表/清单/表单/看板/原文）
  workbenchView: {
    widgetMetric: "指标",
    widgetTable: "表格",
    widgetChart: "图表",
    widgetChecklist: "清单",
    widgetForm: "表单",
    widgetActions: "快捷按钮",
    widgetMarkdown: "说明",
    widgetBoard: "AI 看板",
    widgetKbScope: "查哪些库",
    confirmDelete: "确认删除",
    forbidden: "无权限：你没有这个数据源的使用权限",
    fetchFailed: "取数失败：{{error}}",
    forbiddenDetail: "无权限：你还没有「{{connection}}」里「{{tool}}」的使用权限。",
    thisDataSource: "这个数据源",
    requestAccess: "去申请权限 ↗",
    contactAdmin: "请联系管理员开通。",
    countRows: "{{total}} 条",
    expandDetail: "展开详情",
    collapseDetail: "收起详情",
    truncatedRows: "共 {{total}} 行，只显示前 {{shown}} 行",
    pointTip: "{{label}} · {{series}}：{{value}}",
    pieTip: "{{label}}：{{value}}（{{percent}}%）",
    selectPlaceholder: "请选择",
    citationOk: "✔ 已核对原文",
    citationStale: "原文已更新",
    citationMismatch: "✖ 对不上原文",
    citationMissing: "✖ 找不到这篇文档",
    citationUnchecked: "未核对",
    citationWhere: "{{title}} · 第 {{range}} 行",
    citationTip: "{{where}}\n“{{quote}}”",
    webSourceTip: "{{tip}}\n在 dsh-ee 网页上看原文",
    quoteText: "“{{quote}}”",
    viewSource: "查看原文：{{where}}",
    nearSkippedTip: "那一页是扫描页或乱码，没有收录进知识库；答案可能漏了那一页的内容，请看原件",
    nearSkipped: "⚠ 第 {{page}} 页没收录",
    noKbs: "你还没有可以查的知识库",
    kbDocs: "{{count}} 篇",
    scopeOnly: "只查勾选的 {{count}} 个库",
    scopeAllHint: "没有勾选时查全部",
    boardEmpty: "AI 处理完以后会把结果写在这里。",
    updatedAt: "更新于 {{time}}",
    unavailable: "这个工作台已经不可用：应用可能已停用，或者工作台配置改了。可以关掉这个标签页。",
    openCitationHint: "点「答案与出处」里的引用，会在这里打开原文。",
    versionLine: "第 {{version}} 版 · 第 {{from}}～{{to}} 行 / 共 {{total}} 行",
    staleNotice: "{{stale}}；高亮的是引用时的版本。",
    prevPage: "上一页",
    nextPage: "下一页",
    readView: "阅读视图",
    rawView: "看原文（带行号）",
    quotedSource: "引用的原文：“{{quote}}”"
  },
  // local-mcp.ts：/ee-local 命令与本机扩展 MCP 注册
  localMcp: {
    missingPackage: "要带 npm 包名，例如 /ee-local add @acme/legacy-oa-local@0.1.0",
    notNpmPackage: "「{{text}}」不是 npm 包名：端侧扩展只从企业 npm 源安装，不写 URL、本地路径或 git 地址",
    invalidPackage: "「{{pkg}}」不是合法的 npm 包名（应形如 @scope/name 或 name）",
    invalidVersion: "「{{version}}」不是合法的版本号",
    unknownSubcommand: "子命令只能是 list / add / remove / snippet；收到「{{action}}」",
    idHint: "--id 后面要跟扩展 id，例如 acme/legacy-oa",
    invalidId: "「{{value}}」不是合法扩展 id（应形如 acme/legacy-oa）",
    unknownFlag: "不认识的参数「{{token}}」",
    missingTarget: "{{action}} 后面要跟包名或扩展 id",
    extraArgs: "带了多余参数「{{args}}」",
    duplicateService: "已有同名 MCP 服务 {{name}}，不覆盖",
    noManagerHint: "\n这台 DSH 没有 MCP 管理接口（@deepseek-ai/dsh-mcp-manager），不能自动注册；用 /ee-local snippet <包名> 生成配置片段。\n",
    noExtensions: "还没有添加本机扩展。\n用法：/ee-local add <包名>[@版本] [--id 作者/名字]\n{{hint}}",
    registered: "已注册",
    unregistered: "未注册",
    unregisteredNoManager: "未注册（没有 MCP 管理接口）",
    listHeader: "本机扩展（{{count}} 个）：\n{{lines}}\n{{hint}}\n移除：/ee-local remove <扩展 id 或包名>",
    alreadyAdded: "已经添加过 {{spec}}；要改 id 或版本先 /ee-local remove。",
    idTaken: "扩展 id {{id}} 已经用在 {{pkg}} 上；先 /ee-local remove {{id}}，或换一个 --id。",
    packageBound: "这个包已经以扩展 id {{id}} 添加过；要换 id 先 /ee-local remove {{id}}。",
    noManagerAdd: "这台 DSH 没有 MCP 管理接口（@deepseek-ai/dsh-mcp-manager 没有挂载，旧版 DSH 或精简组合没有它），不能自动注册。\n把下面这段加进 profile 的 cordis.patch.yml（或把 DSH 升级到 0.2.2 再试）：\n\n{{snippet}}\n",
    serviceConflict: "这里已经有名为 {{name}} 的 MCP 服务（员工自己配的、别的插件注册的，或之前加的），不覆盖。\n换个 --id 会得到另一个服务名，例如：/ee-local add {{spec}} --id {{scope}}/<另一个名字>\n",
    confirmPlan: "准备把端侧扩展{{update}}注册成本机 MCP 服务（只写这台电脑，不改 dsh-ee，不覆盖已有服务）：\n  扩展      {{id}}\n  包        {{pkg}}{{latest}}\n  MCP 服务  {{server}}（工具名 mcp__{{server}}__<工具>）\n  env       DSH_EE_URL、DSH_EE_TOKEN（走 DSH 凭据库）、DSH_EE_HARNESS=dsh、企业 npm 源\n\n确认后执行：/ee-local add {{spec}}{{idFlag}} --yes\n",
    updateSuffix: "更新为新的版本并",
    latestVersion: "（最新版本）",
    registerFailed: "注册失败：{{error}}",
    registeredText: "已注册（下次启动 DSH 自动恢复）：\n  {{id}} → {{spec}}，服务名 {{server}}\n工具名以 mcp__{{server}}__ 开头；第一次调用如果扩展要求登录，会弹出系统自己的登录窗口。\n移除：/ee-local remove {{id}}\n",
    notFound: "没有找到「{{target}}」；/ee-local list 看已添加的扩展。",
    removed: "已移除 {{id}}（{{pkg}}），MCP 服务 {{server}} 已注销。",
    snippetIntro: "把下面这段加进对应 profile 的 cordis.patch.yml 末尾（DSH 没有 mcpServers 配置文件，这是官方的配置方式）：\n\n{{snippet}}\n\n{{hint}}\n",
    snippetHint: "这台 DSH 有 MCP 管理接口，也可以用 /ee-local add 自动注册。",
    description: "本机扩展：注册 / 查看 / 移除端侧扩展的 MCP 服务（只写这台电脑）",
    inputHint: "[list|add|remove|snippet] [包名[@版本]] [--id 作者/名字] [--yes]"
  },
  // index.ts（宿主端）：远程方法里的兜底错误
  serverIndex: {
    microAppNotFound: "目录里没有这个微应用",
    notLoggedIn: "没有登录企业账号",
    feedbackFailed: "反馈失败：HTTP {{status}}",
    invalidEntryId: "条目编号不合法"
  },
  // kb-doc-render.ts：知识库原文阅读视图（分页标签）
  kbDocRender: {
    page: "第 {{page}} 页"
  },
  // bridge.ts：MCP 工具结果文字
  bridge: {
    error: "错误：{{text}}"
  },
  // brand-view.ts：没有 logo 时的首字母兜底
  brandView: {
    fallbackLetter: "企"
  }
};
var zh_default = zh;

// src/i18n/en.ts
var en = {
  // 多个模块共用的词（视图标题/按钮/空态）；页面专属文案放各自命名空间
  common: {
    login: "Log in",
    refresh: "Refresh",
    retry: "Retry",
    save: "Save",
    submit: "Submit",
    close: "Close",
    hide: "Hide",
    loading: "Loading…",
    noData: "No data",
    notSet: "Not set",
    viewInWeb: "View on web",
    delete: "Delete",
    newBadge: "New",
    inProgress: "In progress",
    pendingDecision: "Needs your decision",
    working: "Working",
    turnEnded: "Turn ended",
    viewMoreCount: "View more ({{n}})",
    listSeparator: ", "
  },
  // client.ts：客户端插件入口（远程调用兜底、登录/诊断提示、默认企业名）
  client: {
    remoteFailed: "Enterprise remote call failed",
    overviewTimeout: "Timed out reading the enterprise account",
    brandTimeout: "Timed out reading the enterprise brand",
    defaultOrg: "Enterprise",
    loginInBrowser: "Complete enterprise sign-in in your browser.",
    loginStartFailed: "Could not start sign-in. Check your network and try again.",
    diagnoseFailed: "Diagnostics failed. Check your network and try again."
  },
  // apps-client.ts：企业应用目录的客户端状态与操作提示
  apps: {
    copied: "Copied {{command}}. Paste it into the input box and send.",
    filled: "Filled into the input box. Review and send.",
    loginInBrowser: "Complete enterprise sign-in in your browser.",
    loginStartFailed: "Could not start sign-in. Try again later.",
    openWorkbenchFirst: "Open the app's session first, then open its workbench.",
    workbenchNotOpen: "The workbench did not open. Try again later.",
    enableFailed: "Enable failed: {{error}}",
    enabled: "Enabled “{{title}}”. Click it to start.",
    retryLater: "Try again later",
    enableFailedNetwork: "Enable failed. Check your network and try again.",
    disableFailed: "Disable failed: {{error}}",
    disabled: "Disabled “{{title}}”.",
    disableFailedNetwork: "Disable failed. Check your network and try again.",
    unbound: "Unbound",
    unbindFailed: "Unbind failed. Try again later.",
    enterpriseApps: "Enterprise apps"
  },
  // commands.ts：/ee-login /ee-status /ee-logout 的对话文案与命令描述
  commands: {
    pending: "Confirm in your browser. Code {{code}}, link {{url}}",
    starting: "Starting enterprise sign-in…",
    badSetupCode: "Invalid access code. Expected a code like ABCD-EFGH; you can also run /ee-login without one.",
    setupConfirm: "Go back to the Get started page on the dsh-ee website, check code {{code}}, and click “Authorize” (if the page is closed, open {{url}}). Then come back here to continue.",
    loginDescription: "Log in to your enterprise account (optionally with the access code from the web Get started page)",
    setupCodeHint: "[access code]",
    statusDescription: "Check enterprise sign-in status",
    notLoggedIn: "Not signed in. Type /ee-login in the chat.",
    unreachable: "Cannot reach dsh-ee. Check your network or VPN and try again.",
    sessionExpired: "Sign-in expired. Type /ee-login in the chat.",
    httpError: "dsh-ee returned HTTP {{status}}. Try again later (this is not a sign-in failure).",
    expiresOn: " · expires {{date}}",
    neverExpires: " · never expires",
    statusLine: "Signed in as {{username}} ({{department}}) · token {{token}}{{expiry}} · plugin {{version}} · npmrc {{npmrc}}",
    npmrcMatch: "matches",
    npmrcMismatch: "mismatch",
    logoutDescription: "Sign out of the enterprise account",
    noAccount: "No enterprise account is signed in",
    revokedPrefix: "The token was revoked on the server, ",
    envToken: "but it comes from an environment variable or $DSH_HOME/.env ({{ref}}). Delete it manually and restart DSH.",
    loggedOut: "Signed out of the enterprise account",
    loggedOutOffline: "Signed out locally; dsh-ee was unreachable, so the token could not be revoked on the server. You can revoke it under My tokens on the web."
  },
  // doctor.ts：ee_local_doctor 诊断项与修复建议
  doctor: {
    appBindingsFix: "Reopening the micro-app creates a new binding; the corrupt file does not need manual cleanup.",
    noCredential: "No enterprise sign-in credential on this machine",
    loginFix: "Type /ee-login in the chat to sign in",
    hasCredential: "Enterprise sign-in credential found on this machine",
    source: "Source: {{source}}",
    expiringSoon: "Sign-in expires within {{days}} day(s)",
    reLoginFix: "Run /ee-login before it expires to sign in again",
    signedInAs: "Signed in as {{username}} ({{department}})",
    loginExpired: "Enterprise sign-in has expired",
    reLoginFix2: "Type /ee-login in the chat to sign in again",
    readAccountFailed: "Failed to read the enterprise account (HTTP {{status}})",
    serverRestarting: "dsh-ee may be restarting or temporarily unavailable. This is not a sign-in failure; try again later.",
    retryDiagnose: "Run diagnostics again shortly; if it keeps failing, check the server status.",
    unreachable: "Cannot reach dsh-ee ({{base}})",
    unreachableFix: "Check your network or VPN. If the address is wrong, change it under Settings → dsh-ee in DSH to a dsh-ee address that opens in your browser, then test it.",
    npmrcOk: "The enterprise registry config in DSH's .npmrc matches the current sign-in",
    npmrcMissing: "DSH's .npmrc is missing the enterprise npm registry config",
    npmrcMismatch: "The enterprise registry token in DSH's .npmrc does not match the current sign-in",
    npmrcDetail: "{{path}}; installing enterprise plugins will fail with 401",
    npmrcFix: "Restarting DSH or signing in again syncs it automatically",
    modelsAvailable: "Available models: {{models}}",
    noModels: "No models available",
    readModelsFailed: "Failed to read the model list",
    readModelsHttp: "Failed to read the model list (HTTP {{status}})",
    modelPermissionFix: "Ask your administrator to grant model access",
    defaultModelMissing: "The enterprise default model {{model}} is not in your available list",
    chooseModelFix: "Pick an available model in DSH",
    pluginNewer: "A newer version of the connect plugin is available: {{latest}} (current {{version}})",
    updateFix: "Click “Update to latest” under Settings → dsh-ee in DSH, then restart DSH after it installs.",
    pluginLatest: "Connect plugin {{version}} is up to date",
    missingPlugins: "Missing required plugins: {{plugins}}",
    missingPluginsFix: "Install them on the DSH plugins page, or restart DSH so the connect plugin installs them automatically.",
    pluginsOk: "All required plugins are installed",
    bridgeFailed: "Enterprise tool sync failed",
    bridgeFix: "Check your network; type /ee-status to see sign-in status.",
    bridgeOk: "Enterprise tools synced ({{count}} tool(s))",
    summaryFail: "{{fails}} failed, {{warns}} need attention",
    summaryWarn: "{{warns}} need attention",
    summaryOk: "All good",
    description: "Check how DSH on this machine connects to the dsh-ee enterprise workbench: sign-in credential, token validity, .npmrc, connect plugin version, required plugins, models, enterprise tool sync, plus server-side checks. Read-only; changes nothing."
  },
  // status-view.ts：右侧企业面板标题栏与头部按钮
  statusView: {
    adminOn: "On",
    adminOff: "Off",
    adminRemainingHours: "{{hours}} h {{minutes}} min left",
    adminRemainingMinutes: "{{minutes}} min left",
    failureAuth: "Not signed in to the enterprise account, or the sign-in has expired. Please sign in.",
    failureServer: "dsh-ee returned an error ({{detail}}). Refresh later.",
    failureUnreachable: "Cannot reach dsh-ee ({{base}}, {{detail}}). Make sure this machine can open that address in a browser; if the address is wrong, change it under Settings → dsh-ee in DSH to one that opens, then test it.",
    failureDefault: "Not signed in or the connection is unavailable. Please sign in again.",
    reading: "Reading enterprise account…",
    startingLogin: "Starting sign-in…",
    account: "Account",
    department: "Department",
    tokenExpiry: "Token expiry",
    adminMode: "Admin mode",
    expiresOn: "expires {{date}}",
    neverExpires: "Never expires",
    quota: "This month's quota",
    quotaUnavailable: "Quota information unavailable",
    quotaUnlimited: "Unlimited",
    quotaRemaining: "{{percent}}% left",
    models: "Enterprise models",
    defaultModel: "Default: {{model}}",
    modelsUnavailable: "No models available, or the model list is unavailable",
    status: "Connection status",
    pluginVersion: "Plugin {{version}}{{latest}}",
    pluginNewVersion: " · new version {{version}}",
    pluginUpToDate: " · up to date",
    diagnosing: "Diagnosing…",
    diagnose: "Diagnostics",
    diagnoseResult: "Diagnostics: {{summary}}",
    checkLine: "{{title}}{{fix}}",
    checkFixSeparator: "; ",
    headerDisconnected: "Enterprise · Not signed in",
    headerChecking: "Enterprise · Checking",
    headerAdmin: "Admin mode · {{remaining}}",
    headerQuota: "Enterprise models · {{percent}}% left",
    headerUser: "Enterprise models · {{username}}",
    headerTitleUser: "{{username}}{{department}}",
    headerDepartment: " ({{department}})",
    headerTitle: "Sign in to the enterprise account again",
    guideDescription: "View the enterprise account, quota, models and connection status"
  },
  // turn-writes-view.ts：一轮末尾「已留痕」提示
  turnWrites: {
    failedOp: "{{label}} failed{{count}}",
    failedCount: " ×{{count}}",
    failedNotice: "This turn's write operations in enterprise systems (some failed) have been logged in dsh-ee",
    okNotice: "This turn changed data in enterprise systems and has been logged in dsh-ee",
    systemLabel: "“{{system}}”"
  },
  // votes.ts：助手消息尾的 👍👎 反馈
  votes: {
    notePrompt: "What was unhelpful about this answer? (Optional; grouped by question for the app author.)",
    helpful: "Helpful",
    notHelpful: "Not helpful",
    label: "Rate this answer"
  },
  // write-log.ts：本机写操作日志
  writeLog: {
    invalidSessionId: "Invalid session id"
  },
  // overview.ts：读取企业账号失败原因
  overview: {
    timeout: "Connection timed out",
    toolFailed: "Enterprise tool call failed",
    toolNoJson: "Enterprise tool returned no JSON"
  },
  // config.ts：插件配置默认值与校验
  config: {
    badBase: "[dsh-ee] base must be an http(s) URL",
    defaultOrg: "Enterprise"
  },
  // app-launcher.ts：启动微应用时的发送结果不确定提示
  appLauncher: {
    sendUnknown: "The message may not have been sent. Check the session and try again."
  },
  // host-session.ts：宿主会话兼容层的失败提示
  hostSession: {
    switchUnsupported: "The host does not support switching sessions",
    noCwd: "The current session has no working directory for a new session",
    createWorkspaceUnsupported: "The host does not support creating workspaces",
    noWorkspace: "Could not get a workspace for the new session",
    createSessionUnsupported: "The host does not support creating sessions",
    rejected: "The host rejected the message",
    sendUnsupported: "The host does not support sending messages"
  },
  // device.ts：设备码登录
  device: {
    startFailed: "Failed to start device sign-in: {{error}}",
    startHttpFailed: "Failed to start device sign-in: HTTP {{status}}",
    cancelled: "Device sign-in cancelled",
    failed: "Device sign-in failed: HTTP {{status}}",
    timeout: "Device sign-in timed out"
  },
  // connect-update.ts：一键更新接入插件
  connectUpdate: {
    noManager: "This DSH has no plugin management API. Update the connect plugin manually on the plugins page.",
    noLatest: "Could not get the enterprise connect plugin version: {{why}}{{hint}}",
    signInHint: "; sign in to your enterprise account and try again",
    notPublished: "The enterprise server has not published the connect plugin yet. Ask your administrator to publish it in the system settings first.",
    upToDate: "Already up to date ({{version}})",
    installed: "Installed {{pkg}}@{{latest}}; takes effect after DSH restarts",
    installFailed: "Install failed: {{error}}"
  },
  // upload.ts：ee_plugins.upload 上传 .tgz
  upload: {
    missingPath: "Missing path: local path of the .tgz file",
    notTarball: "Only .tgz files produced by npm pack can be uploaded",
    fileNotFound: "File not found: {{file}}",
    notAFile: "Not a file: {{file}}",
    tooLarge: "File is too large",
    timeout: "Upload timed out. Try again, or upload on the web if your network is slow.",
    unreachable: "Cannot reach dsh-ee. Check your network or VPN.",
    httpFailed: "Upload failed (HTTP {{status}})"
  },
  // ee-save.ts：/ee-save 命令描述（提炼请求正文本身保留中文，e2e 按原文比对）
  eeSave: {
    description: "Turn this conversation into a capability (prompt / skill / app draft)"
  },
  // app-bindings.ts：微应用会话绑定文件
  appBindings: {
    invalidAppName: "Invalid micro-app name",
    invalidSessionId: "Invalid session id",
    corruptFile: "The micro-app session binding file is corrupt; treated as empty bindings ({{file}})"
  },
  // app-subscription.ts：启用/停用自选微应用
  appSubscription: {
    invalidAppName: "Invalid micro-app name",
    invalidSkillName: "Invalid skill name",
    invalidConnectionName: "Invalid connector name",
    invalidKind: "Invalid capability type"
  },
  // apps-view.ts：企业应用侧栏/首屏横排/「查看更多」浮层/进行中
  appsView: {
    startersTitle: "Start from an example question",
    confirmDisable: "Confirm disable",
    disable: "Disable",
    newSession: "Start a new session",
    openWorkbench: "Open workbench",
    fillInput: "Fill into input box",
    unbind: "Unbind",
    moreActions: "More actions for {{title}}",
    missingList: "Unavailable: {{missing}}",
    missing: "Some capabilities unavailable",
    missingHint: "{{title}}. Click to view and request access",
    missingApply: "Some capabilities unavailable · Request",
    todoBadge: "{{count}} pending",
    openArrow: "Open →",
    enable: "Enable",
    gallerySubtitle: "{{count}} apps · Click one to start; the AI opens the workbench with it",
    searchAppsAria: "Search enterprise apps",
    searchAppsPlaceholder: "Search apps…",
    noMatches: "No matching apps",
    availableCount: "Available ({{count}})",
    showInSidebar: "Show enterprise apps in the sidebar",
    loginToSee: "Sign in to see enterprise apps",
    expandApps: "Expand enterprise apps",
    collapseApps: "Collapse enterprise apps",
    hideSidebar: "Hide enterprise apps from the sidebar (turn them back on in View more or Settings → dsh-ee)",
    collapsedCount: " ({{count}})",
    allApps: "All apps",
    availablePlus: "＋ Available ({{count}})",
    heroSearchPlaceholder: "Search…",
    itemTitle: "{{title}}: {{label}}",
    todosNeed: "{{count}} items need your attention",
    startHint: "Click one to start; the workbench opens on the right",
    tabApps: "Apps",
    tabSkills: "Skills",
    tabConnections: "Connectors",
    skillsSubtitle: "{{count}} skills · once enabled, usable in every session",
    connectionsSubtitle: "{{count}} connectors · let the AI query and act on company systems",
    searchSkillsPlaceholder: "Search skills…",
    searchConnectionsPlaceholder: "Search connectors…",
    noSkills: "No skills available yet",
    noConnections: "No connectors available yet",
    featured: "Featured",
    statusAuto: "On by default",
    statusEnabled: "Enabled",
    statusOff: "Not enabled",
    statusNeedsAuth: "Needs authorization",
    toolsCount: "{{count}} tools",
    readWrite: "Read & write",
    readOnly: "Read-only",
    authorize: "Authorize",
    preview: "Preview",
    details: "Details",
    needsAuthHint: "{{count}} connectors need authorization first; reopen this after authorizing",
    capEnabled: "Enabled “{{title}}”"
  },
  // workbench-client.ts：业务工作台标签页（面板/原文窗格、写表、清单、继续追问）
  workbenchClient: {
    fallbackTitle: "Workbench",
    noSession: "No current session found. Open the app's session first.",
    sentToAi: "Sent to the AI: {{label}}",
    networkError: "Network error",
    noSessionCheck: "No current session found; the check will not be saved",
    checkNotSaved: "The check was not saved. Try again later.",
    updatePlugin: "Update the connect plugin to the latest version",
    saved: "Saved: {{label}}",
    saveFailedNetwork: "Not saved. Check your network and try again.",
    writeFailed: "Could not {{action}}: {{error}}",
    deletedRow: "Row deleted",
    updated: "Updated: {{label}}",
    docLabel: "Document {{doc}}",
    askPrompt: "Continue with knowledge base document “{{title}}” (document {{doc}}), around lines {{from}}–{{to}}: what other relevant rules or exceptions apply in that part? Answer with citations.",
    ask: "Ask more in this document",
    saveFailedDefault: "Not saved. Update the connect plugin to the latest version."
  },
  // settings-view.ts：DSH 设置里的「dsh-ee」页（地址、测试、更新、侧栏开关）
  settingsView: {
    title: "dsh-ee connection",
    description: "The connect plugin uses this address to sign in to your enterprise account, sync enterprise skills and micro-apps, and call enterprise models. If it cannot connect, first open the dsh-ee website in your browser, then enter the address that works here.",
    reading: "Reading…",
    addressLabel: "dsh-ee address",
    packagedOverridden: "The address in the plugin package is {{packaged}}; you changed it to the current one.",
    packagedDefault: "This is the address your administrator wrote when publishing the connect plugin.",
    testing: "Testing…",
    testConnection: "Test connection",
    resetAddress: "Restore the address from the plugin package",
    plugin: "Connect plugin",
    currentVersion: "Current version {{version}}",
    updating: "Updating…",
    updateLatest: "Update to latest",
    updateHint: "Update to the latest version published by the enterprise server; restart DSH for it to take effect.",
    changeHint: "Changing the address only affects this machine. If nobody can connect, ask your administrator to fill in the access address in the dsh-ee website's system settings and publish the connect plugin again.",
    readFailed: "Failed to read settings: {{error}}",
    testFailed: "Test failed: {{error}}",
    saveFailed: "Save failed: {{error}}",
    updateFailed: "Update failed: {{error}}",
    savedNotice: "Saved and applied. Enterprise skills, micro-apps and tools are syncing to the new address; if the sign-in is no longer valid, sign in again from the sidebar enterprise panel.",
    sidebarHint: "The sidebar lists only the first 3 apps; the rest are under View more. When hidden, enterprise apps can still be opened below the composer on the home screen and from the enterprise panel on the right.",
    saving: "Saving…"
  },
  // server-settings.ts：dsh-ee 地址的保存与连接测试
  serverSettings: {
    reasonTimeout: "Connection timed out: this machine cannot reach that address (internal address, VPN not connected, or blocked by a firewall)",
    reasonRefused: "Connection refused: the address resolves, but nothing is listening on that port",
    reasonDns: "Domain not found: check the spelling, or whether you need to connect to a VPN",
    reasonTls: "HTTPS certificate problem ({{code}})",
    badAddress: "Invalid address format",
    badAddressDetail: "Enter a full http:// or https:// address, for example https://ee.example.com, with no parameters other than the path",
    unreachable: "Unreachable",
    reachable: "Reachable ({{ms}} ms)",
    notDshEe: "That address does not serve dsh-ee",
    notDshEeDetail: "GET /healthz returned HTTP {{status}}; make sure the address is the dsh-ee website address",
    isDshEe: "This is dsh-ee{{version}}",
    versionSuffix: " ({{version}})",
    noLogin: "This machine has not signed in to an enterprise account",
    noLoginDetail: "After saving the address, click “Log in” in the sidebar enterprise panel, or type /ee-login in the chat.",
    signedIn: "Signed in as {{username}}{{department}}",
    departmentSuffix: " ({{department}})",
    loginInvalid: "The local sign-in is not valid for this dsh-ee server",
    loginInvalidDetail: "Save the address and sign in again (the token is issued per dsh-ee server; switching servers requires a new sign-in).",
    loginCheckFailed: "Sign-in check failed",
    saveBadAddress: "Invalid address format: enter a full http:// or https:// address"
  },
  // workbench.ts：工作台数据格式化、占位符、表单与看板校验（纯函数）
  workbench: {
    unitHundredMillion: " hundred million",
    unitTenThousand: " ten thousand",
    unitBillion: "B",
    unitMillion: "M",
    flat: "Flat",
    allKbs: "All knowledge bases",
    scopePicked: "Knowledge bases {{titles}} (ids {{ids}})",
    kbTitle: "“{{title}}”",
    formRequired: "Fill in “{{label}}”",
    formNumber: "“{{label}}” must be a number",
    formSelect: "Choose “{{label}}” from the options",
    blocksRequired: "blocks must be a non-empty array",
    blocksTooMany: "A board can have at most 20 blocks",
    boardTooLarge: "Board content cannot exceed 32 KB",
    blockWhere: "Block {{index}}",
    textEmpty: "{{where}}: text cannot be empty",
    itemsRange: "{{where}}: items must contain 1–50 entries",
    columnsRange: "{{where}}: columns must contain 1–12 columns",
    rowsRange: "{{where}}: rows must be a 2-D array with at most 50 rows",
    citationsRange: "{{where}}: items must contain 1–30 citations",
    citationAt: "{{where}}, citation {{index}}",
    citationNumbers: "{{at}}: doc, from and to must be positive integers, and to must not be less than from",
    citationVersion: "{{at}}: version must be a positive integer",
    citationQuote: "{{at}}: quote must be a 1–500 character excerpt from the source",
    blockType: "{{where}}: type must be text, list, table or citations"
  },
  // workbench-host.ts：ee_workbench 工具与面板取数/写表
  workbenchHost: {
    noComponent: "No such component in the catalog",
    notReadOnly: "This data source is not a read-only tool; the panel cannot use it for data",
    formNoTable: "This form is not configured to submit to a table",
    rowNoAction: "This button cannot modify the table",
    rowNoId: "This row has no id",
    badDoc: "Invalid document id",
    invalidAppName: "Invalid micro-app name",
    invalidSessionId: "Invalid session id",
    invalidChecklistKey: "Invalid checklist key",
    actionInvalid: "action must be board or checklist",
    appNotFound: "No micro-app {{app}}, or it is not in your available scope",
    noBoard: "The workbench of micro-app {{app}} has no AI board",
    noBoardSession: "Cannot find the session whose board should be updated",
    boardUpdated: "Workbench board updated",
    badCitations: "Citation(s) {{numbers}} do not match the source: re-read those lines with kb__read, correct them, write the board again, and correct the user in the conversation.",
    staleCitations: "The document(s) for citation(s) {{numbers}} have been updated; read the latest version to confirm the conclusions still hold.",
    checklistUnsupported: "This version of the connect plugin does not support checklists",
    noChecklist: "The workbench of micro-app {{app}} has no checklist",
    checklistNotFound: "No checklist titled “{{title}}”. Available: {{options}}",
    checklistAmbiguous: "There are {{count}} checklists; specify one with title: {{options}}",
    checklistOption: "“{{title}}”",
    doneOrUndone: "Fill in at least one of done and undone",
    itemsNotFound: "Checklist “{{title}}” has no entries {{unknown}}; entry ids: {{known}}",
    noChecklistSession: "Cannot find the session whose checklist should be updated",
    checklistUpdated: "Checklist “{{title}}” updated: {{done}}/{{total}} done",
    checklistMarked: "Checklist “{{title}}” updated: {{count}} entries done",
    description: "Update the current micro-app workbench: action=board writes this turn's results to the AI board (replacing it entirely; knowledge base citations in citations blocks are verified automatically); action=checklist checks or unchecks entries in a workbench checklist (fill entry ids in done / undone). Changes no business data.",
    paramApp: "Micro-app name, i.e. the name in /name",
    paramTitle: "checklist: title of the checklist; optional when the workbench has only one checklist",
    paramDone: "checklist: entry ids to mark as done",
    paramUndone: "checklist: entry ids to unmark as done",
    paramBlocks: "Board content, shown top to bottom; rendered as text only, HTML not supported",
    paramItems: "list: one text per entry; citations: one knowledge base citation per entry {doc, version, from, to, quote, title}"
  },
  // workbench-view.ts：业务工作台面板（指标/表格/图表/清单/表单/看板/原文）
  workbenchView: {
    widgetMetric: "Metric",
    widgetTable: "Table",
    widgetChart: "Chart",
    widgetChecklist: "Checklist",
    widgetForm: "Form",
    widgetActions: "Quick actions",
    widgetMarkdown: "Description",
    widgetBoard: "AI board",
    widgetKbScope: "Knowledge bases to search",
    confirmDelete: "Confirm delete",
    forbidden: "No access: you do not have permission to use this data source",
    fetchFailed: "Failed to load data: {{error}}",
    forbiddenDetail: "No access: you do not have permission to use “{{tool}}” in “{{connection}}”.",
    thisDataSource: "this data source",
    requestAccess: "Request access ↗",
    contactAdmin: "Ask your administrator for access.",
    countRows: "{{total}} rows",
    expandDetail: "Show details",
    collapseDetail: "Hide details",
    truncatedRows: "{{total}} rows total; showing the first {{shown}}",
    pointTip: "{{label}} · {{series}}: {{value}}",
    pieTip: "{{label}}: {{value}} ({{percent}}%)",
    selectPlaceholder: "Select",
    citationOk: "✔ Verified against source",
    citationStale: "Source updated",
    citationMismatch: "✖ Does not match source",
    citationMissing: "✖ Document not found",
    citationUnchecked: "Not verified",
    citationWhere: "{{title}} · line {{range}}",
    citationTip: "{{where}}\n“{{quote}}”",
    webSourceTip: "{{tip}}\nView the source on the dsh-ee website",
    quoteText: "“{{quote}}”",
    viewSource: "View source: {{where}}",
    nearSkippedTip: "That page is scanned or garbled and was not indexed in the knowledge base; the answer may be missing its content. Check the original document.",
    nearSkipped: "⚠ Page {{page}} not indexed",
    noKbs: "You have no knowledge bases to search yet",
    kbDocs: "{{count}} docs",
    scopeOnly: "Search only the {{count}} selected knowledge bases",
    scopeAllHint: "All are searched when nothing is selected",
    boardEmpty: "The AI writes its results here when it finishes.",
    updatedAt: "Updated {{time}}",
    unavailable: "This workbench is no longer available: the app may be disabled or its configuration changed. You can close this tab.",
    openCitationHint: "Click a citation in Answers and sources to open the source here.",
    versionLine: "Version {{version}} · lines {{from}}–{{to}} of {{total}}",
    staleNotice: "{{stale}}; the highlight shows the version at citation time.",
    prevPage: "Previous page",
    nextPage: "Next page",
    readView: "Reading view",
    rawView: "View source (with line numbers)",
    quotedSource: "Quoted source: “{{quote}}”"
  },
  // local-mcp.ts：/ee-local 命令与本机扩展 MCP 注册
  localMcp: {
    missingPackage: "An npm package name is required, for example /ee-local add @acme/legacy-oa-local@0.1.0",
    notNpmPackage: "“{{text}}” is not an npm package name: local extensions are installed only from the enterprise npm registry; URLs, local paths and git addresses are not accepted",
    invalidPackage: "“{{pkg}}” is not a valid npm package name (expected @scope/name or name)",
    invalidVersion: "“{{version}}” is not a valid version",
    unknownSubcommand: "Subcommands are list / add / remove / snippet; received “{{action}}”",
    idHint: "--id must be followed by an extension id, for example acme/legacy-oa",
    invalidId: "“{{value}}” is not a valid extension id (expected acme/legacy-oa)",
    unknownFlag: "Unknown option “{{token}}”",
    missingTarget: "{{action}} must be followed by a package name or extension id",
    extraArgs: "Unexpected extra arguments “{{args}}”",
    duplicateService: "An MCP service named {{name}} already exists; not overwriting",
    noManagerHint: "\nThis DSH has no MCP management API (@deepseek-ai/dsh-mcp-manager), so automatic registration is unavailable; use /ee-local snippet <package> to generate a config snippet.\n",
    noExtensions: "No local extensions added yet.\nUsage: /ee-local add <package>[@version] [--id author/name]\n{{hint}}",
    registered: "Registered",
    unregistered: "Not registered",
    unregisteredNoManager: "Not registered (no MCP management API)",
    listHeader: "Local extensions ({{count}}):\n{{lines}}\n{{hint}}\nRemove: /ee-local remove <extension id or package name>",
    alreadyAdded: "{{spec}} has already been added; to change the id or version, run /ee-local remove first.",
    idTaken: "Extension id {{id}} is already used by {{pkg}}; run /ee-local remove {{id}} first, or use another --id.",
    packageBound: "This package was already added under extension id {{id}}; to change the id, run /ee-local remove {{id}} first.",
    noManagerAdd: "This DSH has no MCP management API (@deepseek-ai/dsh-mcp-manager is not mounted; older DSH versions or minimal compositions lack it), so automatic registration is unavailable.\nAdd the snippet below to the end of the profile's cordis.patch.yml (or upgrade DSH to 0.2.2 and try again):\n\n{{snippet}}\n",
    serviceConflict: "An MCP service named {{name}} already exists (configured by the employee, registered by another plugin, or added earlier); not overwriting.\nUsing a different --id yields another service name, for example: /ee-local add {{spec}} --id {{scope}}/<another name>\n",
    confirmPlan: "{{update}}register the local extension as a local MCP service (writes this machine only, does not change dsh-ee, does not overwrite existing services):\n  Extension    {{id}}\n  Package      {{pkg}}{{latest}}\n  MCP service  {{server}} (tool names mcp__{{server}}__<tool>)\n  env          DSH_EE_URL, DSH_EE_TOKEN (from the DSH credential store), DSH_EE_HARNESS=dsh, enterprise npm registry\n\nRun after confirming: /ee-local add {{spec}}{{idFlag}} --yes\n",
    updateSuffix: "After updating it to the new version, ",
    latestVersion: " (latest version)",
    registerFailed: "Registration failed: {{error}}",
    registeredText: "Registered (restored automatically on the next DSH start):\n  {{id}} → {{spec}}, service name {{server}}\nTool names start with mcp__{{server}}__; if the extension requires sign-in on first use, the system's own sign-in window appears.\nRemove: /ee-local remove {{id}}\n",
    notFound: "“{{target}}” not found; run /ee-local list to see added extensions.",
    removed: "Removed {{id}} ({{pkg}}); MCP service {{server}} unregistered.",
    snippetIntro: "Add the snippet below to the end of the corresponding profile's cordis.patch.yml (DSH has no mcpServers config file; this is the official configuration method):\n\n{{snippet}}\n\n{{hint}}\n",
    snippetHint: "This DSH has the MCP management API; /ee-local add can also register it automatically.",
    description: "Local extensions: register / list / remove MCP services for local extensions (writes this machine only)",
    inputHint: "[list|add|remove|snippet] [package[@version]] [--id author/name] [--yes]"
  },
  // index.ts（宿主端）：远程方法里的兜底错误
  serverIndex: {
    microAppNotFound: "No such micro-app in the catalog",
    notLoggedIn: "Not signed in to an enterprise account",
    feedbackFailed: "Feedback failed: HTTP {{status}}",
    invalidEntryId: "Invalid entry id"
  },
  // kb-doc-render.ts：知识库原文阅读视图（分页标签）
  kbDocRender: {
    page: "Page {{page}}"
  },
  // bridge.ts：MCP 工具结果文字
  bridge: {
    error: "Error: {{text}}"
  },
  // brand-view.ts：没有 logo 时的首字母兜底
  brandView: {
    fallbackLetter: "e"
  }
};
var en_default = en;

// src/i18n/es.ts
var es = {
  // 多个模块共用的词（视图标题/按钮/空态）；页面专属文案放各自命名空间
  common: {
    login: "Iniciar sesión",
    refresh: "Actualizar",
    retry: "Reintentar",
    save: "Guardar",
    submit: "Enviar",
    close: "Cerrar",
    hide: "Ocultar",
    loading: "Cargando…",
    noData: "No hay datos",
    notSet: "Sin definir",
    viewInWeb: "Ver en la web",
    delete: "Eliminar",
    newBadge: "Nuevo",
    inProgress: "En curso",
    pendingDecision: "Pendiente de tu decisión",
    working: "Trabajando",
    turnEnded: "Turno finalizado",
    viewMoreCount: "Ver más ({{n}})",
    listSeparator: ", "
  },
  // client.ts：客户端插件入口（远程调用兜底、登录/诊断提示、默认企业名）
  client: {
    remoteFailed: "Falló la llamada remota a la empresa",
    overviewTimeout: "Se agotó el tiempo al leer la cuenta de empresa",
    brandTimeout: "Se agotó el tiempo al leer la marca de la empresa",
    defaultOrg: "Empresa",
    loginInBrowser: "Completa el inicio de sesión de empresa en el navegador.",
    loginStartFailed: "No se pudo iniciar sesión. Comprueba la red y vuelve a intentarlo.",
    diagnoseFailed: "El diagnóstico falló. Comprueba la red y vuelve a intentarlo."
  },
  // apps-client.ts：企业应用目录的客户端状态与操作提示
  apps: {
    copied: "Se copió {{command}}. Pégalo en el cuadro de texto y envíalo.",
    filled: "Se rellenó el cuadro de texto. Revisa y envía.",
    loginInBrowser: "Completa el inicio de sesión de empresa en el navegador.",
    loginStartFailed: "No se pudo iniciar sesión. Vuelve a intentarlo más tarde.",
    openWorkbenchFirst: "Haz clic en la aplicación para volver a su sesión y luego abre el banco de trabajo.",
    workbenchNotOpen: "El banco de trabajo no se abrió. Vuelve a intentarlo más tarde.",
    enableFailed: "No se pudo habilitar: {{error}}",
    enabled: "Se habilitó “{{title}}”. Haz clic para empezar a usarla.",
    retryLater: "Vuelve a intentarlo más tarde",
    enableFailedNetwork: "No se pudo habilitar. Comprueba la red y vuelve a intentarlo.",
    disableFailed: "No se pudo deshabilitar: {{error}}",
    disabled: "Se deshabilitó “{{title}}”.",
    disableFailedNetwork: "No se pudo deshabilitar. Comprueba la red y vuelve a intentarlo.",
    unbound: "Se desvinculó",
    unbindFailed: "No se pudo desvincular. Vuelve a intentarlo más tarde.",
    enterpriseApps: "Aplicaciones de empresa"
  },
  // commands.ts：/ee-login /ee-status /ee-logout 的对话文案与命令描述
  commands: {
    pending: "Confirma en el navegador. Código de confirmación {{code}}, enlace {{url}}",
    starting: "Iniciando sesión de empresa…",
    badSetupCode: "El código de acceso no tiene el formato correcto: debe ser como ABCD-EFGH; también puedes ejecutar /ee-login sin código.",
    setupConfirm: "Vuelve a la página «Empezar» del sitio web de dsh-ee, comprueba el código {{code}} y haz clic en “Autorizar” (si la página está cerrada, abre {{url}}). Después vuelve aquí para continuar la conversación.",
    loginDescription: "Iniciar sesión en la cuenta de empresa (opcionalmente con el código de acceso de la página «Empezar» de la web)",
    setupCodeHint: "[código de acceso]",
    statusDescription: "Ver el estado del inicio de sesión de empresa",
    notLoggedIn: "No has iniciado sesión. Escribe /ee-login en el chat.",
    unreachable: "No se puede conectar con dsh-ee. Comprueba la red o la VPN y vuelve a intentarlo.",
    sessionExpired: "La sesión ha caducado. Escribe /ee-login en el chat.",
    httpError: "dsh-ee devolvió HTTP {{status}}. Reinténtalo más tarde (no es un fallo de inicio de sesión).",
    expiresOn: " · caduca el {{date}}",
    neverExpires: " · sin caducidad",
    statusLine: "Sesión iniciada como {{username}} ({{department}}) · token {{token}}{{expiry}} · plugin {{version}} · npmrc {{npmrc}}",
    npmrcMatch: "coincide",
    npmrcMismatch: "no coincide",
    logoutDescription: "Cerrar la sesión de la cuenta de empresa",
    noAccount: "No hay ninguna cuenta de empresa con sesión iniciada",
    revokedPrefix: "El token se revocó en el servidor, ",
    envToken: "pero procede de una variable de entorno o de $DSH_HOME/.env ({{ref}}). Elimínalo manualmente y reinicia DSH.",
    loggedOut: "Se cerró la sesión de la cuenta de empresa",
    loggedOutOffline: "La sesión se cerró en este equipo; no se pudo conectar con dsh-ee, así que el token no se pudo revocar en el servidor. Puedes revocarlo en “Mis tokens” de la web."
  },
  // doctor.ts：ee_local_doctor 诊断项与修复建议
  doctor: {
    appBindingsFix: "Volver a abrir la microaplicación crea una nueva vinculación; el archivo dañado no hay que eliminarlo manualmente.",
    noCredential: "No hay credenciales de inicio de sesión de empresa en este equipo",
    loginFix: "Escribe /ee-login en el chat para iniciar sesión",
    hasCredential: "Se encontraron credenciales de inicio de sesión de empresa en este equipo",
    source: "Origen: {{source}}",
    expiringSoon: "La sesión caduca en {{days}} día(s)",
    reLoginFix: "Ejecuta /ee-login antes de que caduque para volver a iniciar sesión",
    signedInAs: "Sesión iniciada como {{username}} ({{department}})",
    loginExpired: "La sesión de empresa ha caducado",
    reLoginFix2: "Escribe /ee-login en el chat para volver a iniciar sesión",
    readAccountFailed: "No se pudo leer la cuenta de empresa (HTTP {{status}})",
    serverRestarting: "Es posible que dsh-ee se esté reiniciando o no esté disponible temporalmente. No es un fallo de inicio de sesión; vuelve a intentarlo más tarde.",
    retryDiagnose: "Vuelve a ejecutar el diagnóstico en un momento; si sigue fallando, revisa el estado del servidor.",
    unreachable: "No se puede conectar con dsh-ee ({{base}})",
    unreachableFix: "Comprueba la red o la VPN; si la dirección no es correcta, cámbiala en «Ajustes → dsh-ee» de DSH por una dirección de dsh-ee que se abra en el navegador y pruébala.",
    npmrcOk: "La configuración del registro npm de empresa en el .npmrc de DSH coincide con la sesión actual",
    npmrcMissing: "Al .npmrc de DSH le falta la configuración del registro npm de empresa",
    npmrcMismatch: "El token del registro npm de empresa en el .npmrc de DSH no coincide con la sesión actual",
    npmrcDetail: "{{path}}; la instalación de plugins de empresa fallará con 401",
    npmrcFix: "Reiniciar DSH o volver a iniciar sesión lo sincroniza automáticamente",
    modelsAvailable: "Modelos disponibles: {{models}}",
    noModels: "No hay modelos disponibles",
    readModelsFailed: "No se pudo leer la lista de modelos",
    readModelsHttp: "No se pudo leer la lista de modelos (HTTP {{status}})",
    modelPermissionFix: "Pide a tu administrador que habilite el acceso a los modelos",
    defaultModelMissing: "El modelo predeterminado de empresa {{model}} no está en tu lista de disponibles",
    chooseModelFix: "Elige un modelo disponible en DSH",
    pluginNewer: "Hay una versión más reciente del plugin de conexión: {{latest}} (actual {{version}})",
    updateFix: "Haz clic en “Actualizar a la última versión” en «Ajustes → dsh-ee» de DSH y reinicia DSH cuando termine la instalación.",
    pluginLatest: "El plugin de conexión {{version}} está actualizado",
    missingPlugins: "Faltan plugins obligatorios: {{plugins}}",
    missingPluginsFix: "Instálalos en la página de plugins de DSH o reinicia DSH para que el plugin de conexión los instale automáticamente.",
    pluginsOk: "Todos los plugins obligatorios están instalados",
    bridgeFailed: "Falló la sincronización de herramientas de empresa",
    bridgeFix: "Comprueba la red; escribe /ee-status para ver el estado de la sesión.",
    bridgeOk: "Herramientas de empresa sincronizadas ({{count}} herramienta(s))",
    summaryFail: "{{fails}} con error, {{warns}} requieren atención",
    summaryWarn: "{{warns}} requieren atención",
    summaryOk: "Todo correcto",
    description: "Comprueba cómo se conecta DSH en este equipo con el banco de trabajo de empresa dsh-ee: credenciales de inicio de sesión, validez del token, .npmrc, versión del plugin de conexión, plugins obligatorios, modelos, sincronización de herramientas de empresa y comprobaciones del servidor. Solo lectura; no modifica nada."
  },
  // status-view.ts：右侧企业面板标题栏与头部按钮
  statusView: {
    adminOn: "Activado",
    adminOff: "Desactivado",
    adminRemainingHours: "Quedan {{hours}} h {{minutes}} min",
    adminRemainingMinutes: "Quedan {{minutes}} min",
    failureAuth: "No has iniciado sesión en la cuenta de empresa o la sesión ha caducado. Inicia sesión.",
    failureServer: "dsh-ee devolvió un error ({{detail}}). Actualiza más tarde.",
    failureUnreachable: "No se puede conectar con dsh-ee ({{base}}, {{detail}}). Asegúrate de que este equipo pueda abrir esa dirección en el navegador; si la dirección no es correcta, cámbiala en «Ajustes → dsh-ee» de DSH por una que se abra y pruébala.",
    failureDefault: "No has iniciado sesión o la conexión no está disponible. Vuelve a iniciar sesión.",
    reading: "Leyendo la cuenta de empresa…",
    startingLogin: "Iniciando sesión…",
    account: "Cuenta",
    department: "Departamento",
    tokenExpiry: "Caducidad del token",
    adminMode: "Modo administrador",
    expiresOn: "Caduca el {{date}}",
    neverExpires: "Sin caducidad",
    quota: "Cuota de este mes",
    quotaUnavailable: "Información de cuota no disponible",
    quotaUnlimited: "Ilimitada",
    quotaRemaining: "Queda {{percent}}%",
    models: "Modelos de empresa",
    defaultModel: "Predeterminado: {{model}}",
    modelsUnavailable: "No hay modelos disponibles o la lista de modelos no está disponible",
    status: "Estado de la conexión",
    pluginVersion: "Plugin {{version}}{{latest}}",
    pluginNewVersion: " · nueva versión {{version}}",
    pluginUpToDate: " · actualizado",
    diagnosing: "Diagnosticando…",
    diagnose: "Diagnóstico",
    diagnoseResult: "Diagnóstico: {{summary}}",
    checkLine: "{{title}}{{fix}}",
    checkFixSeparator: "; ",
    headerDisconnected: "Empresa · Sin sesión",
    headerChecking: "Empresa · Comprobando",
    headerAdmin: "Modo administrador · {{remaining}}",
    headerQuota: "Modelos de empresa · queda {{percent}}%",
    headerUser: "Modelos de empresa · {{username}}",
    headerTitleUser: "{{username}}{{department}}",
    headerDepartment: " ({{department}})",
    headerTitle: "Volver a iniciar sesión en la cuenta de empresa",
    guideDescription: "Ver la cuenta de empresa, la cuota, los modelos y el estado de la conexión"
  },
  // turn-writes-view.ts：一轮末尾「已留痕」提示
  turnWrites: {
    failedOp: "{{label}} falló{{count}}",
    failedCount: " ×{{count}}",
    failedNotice: "Las operaciones de escritura de este turno en los sistemas de empresa (algunas fallidas) se han registrado en dsh-ee",
    okNotice: "Este turno modificó datos de los sistemas de empresa y se ha registrado en dsh-ee",
    systemLabel: "“{{system}}”"
  },
  // votes.ts：助手消息尾的 👍👎 反馈
  votes: {
    notePrompt: "¿Qué no resultó útil de esta respuesta? (Opcional; se agrupa por pregunta para el autor de la aplicación).",
    helpful: "Útil",
    notHelpful: "No útil",
    label: "Valorar esta respuesta"
  },
  // write-log.ts：本机写操作日志
  writeLog: {
    invalidSessionId: "El id de sesión no es válido"
  },
  // overview.ts：读取企业账号失败原因
  overview: {
    timeout: "Se agotó el tiempo de conexión",
    toolFailed: "Falló la llamada a la herramienta de empresa",
    toolNoJson: "La herramienta de empresa no devolvió JSON"
  },
  // config.ts：插件配置默认值与校验
  config: {
    badBase: "[dsh-ee] base debe ser una dirección http(s)",
    defaultOrg: "Empresa"
  },
  // app-launcher.ts：启动微应用时的发送结果不确定提示
  appLauncher: {
    sendUnknown: "Es posible que el mensaje no se haya enviado. Revisa la sesión y vuelve a intentarlo."
  },
  // host-session.ts：宿主会话兼容层的失败提示
  hostSession: {
    switchUnsupported: "El host no permite cambiar de sesión",
    noCwd: "La sesión actual no tiene un directorio de trabajo para crear una nueva sesión",
    createWorkspaceUnsupported: "El host no permite crear espacios de trabajo",
    noWorkspace: "No se pudo obtener un espacio de trabajo para la nueva sesión",
    createSessionUnsupported: "El host no permite crear sesiones",
    rejected: "El host rechazó el mensaje",
    sendUnsupported: "El host no permite enviar mensajes"
  },
  // device.ts：设备码登录
  device: {
    startFailed: "No se pudo iniciar la sesión con código de dispositivo: {{error}}",
    startHttpFailed: "No se pudo iniciar la sesión con código de dispositivo: HTTP {{status}}",
    cancelled: "Se canceló el inicio de sesión con código de dispositivo",
    failed: "Falló el inicio de sesión con código de dispositivo: HTTP {{status}}",
    timeout: "Se agotó el tiempo de espera del inicio de sesión con código de dispositivo"
  },
  // connect-update.ts：一键更新接入插件
  connectUpdate: {
    noManager: "Este DSH no tiene API de gestión de plugins. Actualiza el plugin de conexión manualmente en la página de plugins.",
    noLatest: "No se pudo obtener la versión del plugin de conexión de empresa: {{why}}{{hint}}",
    signInHint: "; inicia sesión en la cuenta de empresa y vuelve a intentarlo",
    notPublished: "El servidor de empresa aún no ha publicado el plugin de conexión. Pide a tu administrador que lo publique primero en los ajustes del sistema.",
    upToDate: "Ya está actualizado ({{version}})",
    installed: "Se instaló {{pkg}}@{{latest}}; se aplicará al reiniciar DSH",
    installFailed: "Falló la instalación: {{error}}"
  },
  // upload.ts：ee_plugins.upload 上传 .tgz
  upload: {
    missingPath: "Falta path: la ruta local del archivo .tgz",
    notTarball: "Solo se pueden subir archivos .tgz generados por npm pack",
    fileNotFound: "No se encontró el archivo: {{file}}",
    notAFile: "No es un archivo: {{file}}",
    tooLarge: "El archivo es demasiado grande",
    timeout: "Se agotó el tiempo de subida. Vuelve a intentarlo; si la red es lenta, puedes subirlo desde la web.",
    unreachable: "No se puede conectar con dsh-ee. Comprueba la red o la VPN.",
    httpFailed: "Falló la subida (HTTP {{status}})"
  },
  // ee-save.ts：/ee-save 命令描述（提炼请求正文本身保留中文，e2e 按原文比对）
  eeSave: {
    description: "Convertir esta conversación en una capacidad (borrador de prompt / habilidad / aplicación)"
  },
  // app-bindings.ts：微应用会话绑定文件
  appBindings: {
    invalidAppName: "El nombre de la microaplicación no es válido",
    invalidSessionId: "El id de sesión no es válido",
    corruptFile: "El archivo de vinculación de sesión de la microaplicación está dañado; se trató como vinculaciones vacías ({{file}})"
  },
  // app-subscription.ts：启用/停用自选微应用
  appSubscription: {
    invalidAppName: "El nombre de la microaplicación no es válido",
    invalidSkillName: "El nombre de la habilidad no es válido",
    invalidConnectionName: "El nombre del conector no es válido",
    invalidKind: "El tipo de capacidad no es válido"
  },
  // apps-view.ts：企业应用侧栏/首屏横排/「查看更多」浮层/进行中
  appsView: {
    startersTitle: "Empieza con una pregunta de ejemplo",
    confirmDisable: "Confirmar deshabilitación",
    disable: "Deshabilitar",
    newSession: "Iniciar una sesión nueva",
    openWorkbench: "Abrir banco de trabajo",
    fillInput: "Rellenar en el cuadro de texto",
    unbind: "Desvincular",
    moreActions: "Más acciones de {{title}}",
    missingList: "No disponibles: {{missing}}",
    missing: "Algunas capacidades no están disponibles",
    missingHint: "{{title}}. Haz clic para ver y solicitar acceso",
    missingApply: "Algunas capacidades no disponibles · Solicitar",
    todoBadge: "{{count}} pendientes",
    openArrow: "Abrir →",
    enable: "Habilitar",
    gallerySubtitle: "{{count}} aplicaciones · Haz clic en una para empezar; la IA abrirá el banco de trabajo junto con ella",
    searchAppsAria: "Buscar aplicaciones de empresa",
    searchAppsPlaceholder: "Buscar aplicaciones…",
    noMatches: "No hay aplicaciones coincidentes",
    availableCount: "Disponibles ({{count}})",
    showInSidebar: "Mostrar las aplicaciones de empresa en la barra lateral",
    loginToSee: "Inicia sesión para ver las aplicaciones de empresa",
    expandApps: "Expandir las aplicaciones de empresa",
    collapseApps: "Contraer las aplicaciones de empresa",
    hideSidebar: "Ocultar las aplicaciones de empresa de la barra lateral (puedes volver a mostrarlas en “Ver más” o en «Ajustes → dsh-ee»)",
    collapsedCount: " ({{count}})",
    allApps: "Todas las aplicaciones",
    availablePlus: "＋ Disponibles ({{count}})",
    heroSearchPlaceholder: "Buscar…",
    itemTitle: "{{title}}: {{label}}",
    todosNeed: "{{count}} elementos requieren tu atención",
    startHint: "Haz clic en una para empezar; el banco de trabajo se abrirá a la derecha",
    tabApps: "Apps",
    tabSkills: "Habilidades",
    tabConnections: "Conectores",
    skillsSubtitle: "{{count}} habilidades · una vez activadas, disponibles en cada sesión",
    connectionsSubtitle: "{{count}} conectores · permiten a la IA consultar y operar sistemas de la empresa",
    searchSkillsPlaceholder: "Buscar habilidades…",
    searchConnectionsPlaceholder: "Buscar conectores…",
    noSkills: "Aún no hay habilidades disponibles",
    noConnections: "Aún no hay conectores disponibles",
    featured: "Destacado",
    statusAuto: "Activo por defecto",
    statusEnabled: "Activado",
    statusOff: "No activado",
    statusNeedsAuth: "Requiere autorización",
    toolsCount: "{{count}} herramientas",
    readWrite: "Lectura y escritura",
    readOnly: "Solo lectura",
    authorize: "Autorizar",
    preview: "Vista previa",
    details: "Detalles",
    needsAuthHint: "{{count}} conectores requieren autorización; vuelve a abrir esto después de autorizar",
    capEnabled: "Se activó «{{title}}»"
  },
  // workbench-client.ts：业务工作台标签页（面板/原文窗格、写表、清单、继续追问）
  workbenchClient: {
    fallbackTitle: "Banco de trabajo",
    noSession: "No se encontró la sesión actual. Abre primero la sesión de esta aplicación.",
    sentToAi: "Enviado a la IA: {{label}}",
    networkError: "Error de red",
    noSessionCheck: "No se encontró la sesión actual; la selección no se guardará",
    checkNotSaved: "La selección no se guardó. Vuelve a intentarlo más tarde.",
    updatePlugin: "Actualiza el plugin de conexión a la última versión",
    saved: "Guardado: {{label}}",
    saveFailedNetwork: "No se guardó. Comprueba la red y vuelve a intentarlo.",
    writeFailed: "No se pudo {{action}}: {{error}}",
    deletedRow: "Se eliminó esta fila",
    updated: "Actualizado: {{label}}",
    docLabel: "Documento {{doc}}",
    askPrompt: "Continúa con el documento de la base de conocimiento “{{title}}” (documento {{doc}}), alrededor de las líneas {{from}}–{{to}}: ¿qué otras normas o excepciones se aplican en esa parte? Responde indicando las fuentes.",
    ask: "Preguntar más en este documento",
    saveFailedDefault: "No se guardó. Actualiza el plugin de conexión a la última versión."
  },
  // settings-view.ts：DSH 设置里的「dsh-ee」页（地址、测试、更新、侧栏开关）
  settingsView: {
    title: "Conexión con dsh-ee",
    description: "El plugin de conexión usa esta dirección para iniciar sesión en tu cuenta de empresa, sincronizar habilidades y microaplicaciones de empresa y llamar a los modelos de empresa. Si no se puede conectar, abre primero el sitio web de dsh-ee en el navegador y escribe aquí la dirección que funcione.",
    reading: "Leyendo…",
    addressLabel: "Dirección de dsh-ee",
    packagedOverridden: "La dirección del paquete del plugin es {{packaged}}; la cambiaste por la actual.",
    packagedDefault: "Esta es la dirección que escribió tu administrador al publicar el plugin de conexión.",
    testing: "Probando…",
    testConnection: "Probar conexión",
    resetAddress: "Restaurar la dirección del paquete del plugin",
    plugin: "Plugin de conexión",
    currentVersion: "Versión actual {{version}}",
    updating: "Actualizando…",
    updateLatest: "Actualizar a la última versión",
    updateHint: "Actualiza a la última versión publicada por el servidor de empresa; reinicia DSH para que se aplique.",
    changeHint: "Cambiar la dirección solo afecta a este equipo. Si nadie puede conectarse, pide a tu administrador que indique la dirección de acceso en los ajustes del sistema del sitio web de dsh-ee y vuelva a publicar el plugin de conexión.",
    readFailed: "No se pudo leer la configuración: {{error}}",
    testFailed: "La prueba falló: {{error}}",
    saveFailed: "No se pudo guardar: {{error}}",
    updateFailed: "La actualización falló: {{error}}",
    savedNotice: "Guardado y aplicado. Las habilidades, microaplicaciones y herramientas de empresa se están sincronizando con la nueva dirección; si la sesión ya no es válida, vuelve a iniciarla desde el panel de empresa de la barra lateral.",
    sidebarHint: "La barra lateral solo muestra las 3 primeras aplicaciones; el resto está en “Ver más”. Si las ocultas, las aplicaciones de empresa se pueden abrir igualmente debajo del cuadro de texto de la pantalla de inicio y desde el panel de empresa de la derecha.",
    saving: "Guardando…"
  },
  // server-settings.ts：dsh-ee 地址的保存与连接测试
  serverSettings: {
    reasonTimeout: "Se agotó el tiempo de conexión: este equipo no puede acceder a esa dirección (dirección interna, VPN sin conectar o bloqueada por el cortafuegos)",
    reasonRefused: "Conexión rechazada: la dirección se resuelve, pero no hay ningún servicio en ese puerto",
    reasonDns: "No se encontró el dominio: comprueba la ortografía o si necesitas conectarte a una VPN",
    reasonTls: "Problema con el certificado HTTPS ({{code}})",
    badAddress: "El formato de la dirección no es válido",
    badAddressDetail: "Escribe una dirección completa http:// o https://, por ejemplo https://ee.example.com, sin parámetros más allá de la ruta",
    unreachable: "No se puede conectar",
    reachable: "Se puede conectar ({{ms}} ms)",
    notDshEe: "Esa dirección no corresponde a dsh-ee",
    notDshEeDetail: "GET /healthz devolvió HTTP {{status}}; confirma que la dirección es la del sitio web de dsh-ee",
    isDshEe: "Es dsh-ee{{version}}",
    versionSuffix: " ({{version}})",
    noLogin: "Este equipo aún no ha iniciado sesión en una cuenta de empresa",
    noLoginDetail: "Después de guardar la dirección, haz clic en “Iniciar sesión” en el panel de empresa de la barra lateral o escribe /ee-login en el chat.",
    signedIn: "Sesión iniciada como {{username}}{{department}}",
    departmentSuffix: " ({{department}})",
    loginInvalid: "La sesión de este equipo no es válida en este servidor dsh-ee",
    loginInvalidDetail: "Guarda la dirección y vuelve a iniciar sesión (el token se emite por servidor dsh-ee; al cambiar de servidor hay que iniciar sesión de nuevo).",
    loginCheckFailed: "La comprobación de la sesión falló",
    saveBadAddress: "El formato de la dirección no es válido: escribe una dirección completa http:// o https://"
  },
  // workbench.ts：工作台数据格式化、占位符、表单与看板校验（纯函数）
  workbench: {
    unitHundredMillion: " cien millones",
    unitTenThousand: " diez mil",
    unitBillion: " mil millones",
    unitMillion: " millones",
    flat: "Sin cambios",
    allKbs: "Todas las bases de conocimiento",
    scopePicked: "Bases de conocimiento {{titles}} (ids {{ids}})",
    kbTitle: "“{{title}}”",
    formRequired: "Rellena “{{label}}”",
    formNumber: "“{{label}}” debe ser un número",
    formSelect: "Elige “{{label}}” entre las opciones",
    blocksRequired: "blocks debe ser una lista no vacía",
    blocksTooMany: "Un tablero puede tener como máximo 20 bloques",
    boardTooLarge: "El contenido del tablero no puede superar 32 KB",
    blockWhere: "Bloque {{index}}",
    textEmpty: "{{where}}: text no puede estar vacío",
    itemsRange: "{{where}}: items debe tener de 1 a 50 entradas",
    columnsRange: "{{where}}: columns debe tener de 1 a 12 columnas",
    rowsRange: "{{where}}: rows debe ser una matriz de como máximo 50 filas",
    citationsRange: "{{where}}: items debe tener de 1 a 30 citas",
    citationAt: "{{where}}, cita {{index}}",
    citationNumbers: "{{at}}: doc, from y to deben ser enteros positivos, y to no puede ser menor que from",
    citationVersion: "{{at}}: version debe ser un entero positivo",
    citationQuote: "{{at}}: quote debe ser un fragmento del texto original de 1 a 500 caracteres",
    blockType: "{{where}}: type solo puede ser text, list, table o citations"
  },
  // workbench-host.ts：ee_workbench 工具与面板取数/写表
  workbenchHost: {
    noComponent: "No hay ningún componente con ese nombre en el directorio",
    notReadOnly: "Esta fuente de datos no es una herramienta de solo lectura; el panel no puede usarla para obtener datos",
    formNoTable: "Este formulario no está configurado para enviarse a una tabla",
    rowNoAction: "Este botón no puede modificar la tabla",
    rowNoId: "Esta fila no tiene id",
    badDoc: "El id del documento no es válido",
    invalidAppName: "El nombre de la microaplicación no es válido",
    invalidSessionId: "El id de sesión no es válido",
    invalidChecklistKey: "La clave de la lista de verificación no es válida",
    actionInvalid: "action solo puede ser board o checklist",
    appNotFound: "No existe la microaplicación {{app}} o no está en tu ámbito disponible",
    noBoard: "El banco de trabajo de la microaplicación {{app}} no tiene tablero de IA",
    noBoardSession: "No se encuentra la sesión cuyo tablero hay que actualizar",
    boardUpdated: "Se actualizó el tablero del banco de trabajo",
    badCitations: "Las citas {{numbers}} no coinciden con el original: vuelve a leer esas líneas con kb__read, corrígelas, vuelve a escribir el tablero y corrige al usuario en la conversación.",
    staleCitations: "Los documentos de las citas {{numbers}} se han actualizado; lee la última versión para confirmar que las conclusiones siguen siendo válidas.",
    checklistUnsupported: "Esta versión del plugin de conexión no admite listas de verificación",
    noChecklist: "El banco de trabajo de la microaplicación {{app}} no tiene lista de verificación",
    checklistNotFound: "No hay ninguna lista de verificación titulada “{{title}}”. Disponibles: {{options}}",
    checklistAmbiguous: "Hay {{count}} listas de verificación; especifica una con title: {{options}}",
    checklistOption: "“{{title}}”",
    doneOrUndone: "Rellena al menos uno de done y undone",
    itemsNotFound: "La lista de verificación “{{title}}” no tiene las entradas {{unknown}}; ids de entrada: {{known}}",
    noChecklistSession: "No se encuentra la sesión cuya lista de verificación hay que actualizar",
    checklistUpdated: "Lista de verificación “{{title}}” actualizada: {{done}}/{{total}} completadas",
    checklistMarked: "Lista de verificación “{{title}}” actualizada: {{count}} entradas completadas",
    description: "Actualiza el banco de trabajo de la microaplicación actual: action=board escribe los resultados de este turno en el tablero de IA (lo reemplaza por completo; las citas de la base de conocimiento de los bloques citations se verifican automáticamente); action=checklist marca o desmarca entradas de una lista de verificación del banco de trabajo (indica los ids de entrada en done / undone). No modifica ningún dato de negocio.",
    paramApp: "Nombre de la microaplicación, es decir, el nombre que se escribe después de /",
    paramTitle: "checklist: título de la lista de verificación; opcional cuando el banco de trabajo solo tiene una",
    paramDone: "checklist: ids de las entradas que se marcan como completadas",
    paramUndone: "checklist: ids de las entradas a las que se retira la marca de completadas",
    paramBlocks: "Contenido del tablero, de arriba abajo; solo se muestra como texto, HTML no admitido",
    paramItems: "list: un texto por entrada; citations: una cita de la base de conocimiento por entrada {doc, version, from, to, quote, title}"
  },
  // workbench-view.ts：业务工作台面板（指标/表格/图表/清单/表单/看板/原文）
  workbenchView: {
    widgetMetric: "Indicador",
    widgetTable: "Tabla",
    widgetChart: "Gráfico",
    widgetChecklist: "Lista de verificación",
    widgetForm: "Formulario",
    widgetActions: "Acciones rápidas",
    widgetMarkdown: "Descripción",
    widgetBoard: "Tablero de IA",
    widgetKbScope: "Bases de conocimiento a consultar",
    confirmDelete: "Confirmar eliminación",
    forbidden: "Sin permiso: no tienes permiso para usar esta fuente de datos",
    fetchFailed: "No se pudieron obtener los datos: {{error}}",
    forbiddenDetail: "Sin permiso: aún no tienes permiso para usar “{{tool}}” en “{{connection}}”.",
    thisDataSource: "esta fuente de datos",
    requestAccess: "Solicitar acceso ↗",
    contactAdmin: "Pide acceso a tu administrador.",
    countRows: "{{total}} filas",
    expandDetail: "Ver detalles",
    collapseDetail: "Ocultar detalles",
    truncatedRows: "{{total}} filas en total; se muestran las primeras {{shown}}",
    pointTip: "{{label}} · {{series}}: {{value}}",
    pieTip: "{{label}}: {{value}} ({{percent}}%)",
    selectPlaceholder: "Selecciona",
    citationOk: "✔ Verificado con el original",
    citationStale: "El original se ha actualizado",
    citationMismatch: "✖ No coincide con el original",
    citationMissing: "✖ No se encontró el documento",
    citationUnchecked: "Sin verificar",
    citationWhere: "{{title}} · línea {{range}}",
    citationTip: "{{where}}\n“{{quote}}”",
    webSourceTip: "{{tip}}\nVer el original en el sitio web de dsh-ee",
    quoteText: "“{{quote}}”",
    viewSource: "Ver el original: {{where}}",
    nearSkippedTip: "Esa página está escaneada o es ilegible y no se indexó en la base de conocimiento; puede que la respuesta no incluya su contenido. Consulta el documento original.",
    nearSkipped: "⚠ Página {{page}} no indexada",
    noKbs: "Aún no tienes bases de conocimiento que consultar",
    kbDocs: "{{count}} documentos",
    scopeOnly: "Consultar solo las {{count}} bases de conocimiento seleccionadas",
    scopeAllHint: "Si no seleccionas ninguna, se consultan todas",
    boardEmpty: "La IA escribirá aquí sus resultados cuando termine.",
    updatedAt: "Actualizado el {{time}}",
    unavailable: "Este banco de trabajo ya no está disponible: puede que la aplicación esté deshabilitada o que su configuración haya cambiado. Puedes cerrar esta pestaña.",
    openCitationHint: "Haz clic en una cita de “Respuestas y fuentes” para abrir el original aquí.",
    versionLine: "Versión {{version}} · líneas {{from}}–{{to}} de {{total}}",
    staleNotice: "{{stale}}; el resaltado muestra la versión del momento de la cita.",
    prevPage: "Página anterior",
    nextPage: "Página siguiente",
    readView: "Vista de lectura",
    rawView: "Ver original (con números de línea)",
    quotedSource: "Texto original citado: “{{quote}}”"
  },
  // local-mcp.ts：/ee-local 命令与本机扩展 MCP 注册
  localMcp: {
    missingPackage: "Hay que indicar el nombre del paquete npm, por ejemplo /ee-local add @acme/legacy-oa-local@0.1.0",
    notNpmPackage: "“{{text}}” no es un nombre de paquete npm: las extensiones locales solo se instalan desde el registro npm de empresa; no se aceptan URL, rutas locales ni direcciones git",
    invalidPackage: "“{{pkg}}” no es un nombre de paquete npm válido (debe tener el formato @scope/name o name)",
    invalidVersion: "“{{version}}” no es una versión válida",
    unknownSubcommand: "Los subcomandos son list / add / remove / snippet; se recibió “{{action}}”",
    idHint: "--id debe ir seguido de un id de extensión, por ejemplo acme/legacy-oa",
    invalidId: "“{{value}}” no es un id de extensión válido (debe tener el formato acme/legacy-oa)",
    unknownFlag: "Parámetro desconocido “{{token}}”",
    missingTarget: "{{action}} debe ir seguido de un nombre de paquete o un id de extensión",
    extraArgs: "Se pasaron argumentos de más “{{args}}”",
    duplicateService: "Ya existe un servicio MCP llamado {{name}}; no se sobrescribe",
    noManagerHint: "\nEste DSH no tiene API de gestión de MCP (@deepseek-ai/dsh-mcp-manager), así que no se puede registrar automáticamente; usa /ee-local snippet <paquete> para generar un fragmento de configuración.\n",
    noExtensions: "Aún no se han añadido extensiones locales.\nUso: /ee-local add <paquete>[@versión] [--id autor/nombre]\n{{hint}}",
    registered: "Registrado",
    unregistered: "No registrado",
    unregisteredNoManager: "No registrado (sin API de gestión de MCP)",
    listHeader: "Extensiones locales ({{count}}):\n{{lines}}\n{{hint}}\nEliminar: /ee-local remove <id de extensión o nombre del paquete>",
    alreadyAdded: "{{spec}} ya se añadió; para cambiar el id o la versión, ejecuta /ee-local remove primero.",
    idTaken: "El id de extensión {{id}} ya lo usa {{pkg}}; ejecuta /ee-local remove {{id}} primero o usa otro --id.",
    packageBound: "Este paquete ya se añadió con el id de extensión {{id}}; para cambiar el id, ejecuta /ee-local remove {{id}} primero.",
    noManagerAdd: "Este DSH no tiene API de gestión de MCP (@deepseek-ai/dsh-mcp-manager no está montado; las versiones antiguas de DSH o las composiciones mínimas no lo incluyen), así que no se puede registrar automáticamente.\nAñade el fragmento siguiente al final del cordis.patch.yml del perfil (o actualiza DSH a 0.2.2 y vuelve a intentarlo):\n\n{{snippet}}\n",
    serviceConflict: "Ya existe un servicio MCP llamado {{name}} (configurado por el empleado, registrado por otro plugin o añadido antes); no se sobrescribe.\nCon otro --id se obtiene otro nombre de servicio, por ejemplo: /ee-local add {{spec}} --id {{scope}}/<otro nombre>\n",
    confirmPlan: "{{update}}Registrar la extensión local como servicio MCP local (solo escribe en este equipo, no modifica dsh-ee ni sobrescribe servicios existentes):\n  Extensión    {{id}}\n  Paquete      {{pkg}}{{latest}}\n  Servicio MCP {{server}} (nombres de herramienta mcp__{{server}}__<herramienta>)\n  env          DSH_EE_URL, DSH_EE_TOKEN (desde el almacén de credenciales de DSH), DSH_EE_HARNESS=dsh, registro npm de empresa\n\nCuando confirmes, ejecuta: /ee-local add {{spec}}{{idFlag}} --yes\n",
    updateSuffix: "Para actualizarla a la nueva versión y ",
    latestVersion: " (última versión)",
    registerFailed: "No se pudo registrar: {{error}}",
    registeredText: "Registrado (se restaura automáticamente en el próximo inicio de DSH):\n  {{id}} → {{spec}}, nombre de servicio {{server}}\nLos nombres de herramienta empiezan por mcp__{{server}}__; si la extensión requiere iniciar sesión la primera vez, aparecerá la ventana de inicio de sesión del sistema.\nEliminar: /ee-local remove {{id}}\n",
    notFound: "No se encontró “{{target}}”; ejecuta /ee-local list para ver las extensiones añadidas.",
    removed: "Se eliminó {{id}} ({{pkg}}); el servicio MCP {{server}} se ha dado de baja.",
    snippetIntro: "Añade el fragmento siguiente al final del cordis.patch.yml del perfil correspondiente (DSH no tiene un archivo de configuración mcpServers; esta es la forma oficial de configurarlo):\n\n{{snippet}}\n\n{{hint}}\n",
    snippetHint: "Este DSH tiene API de gestión de MCP; también puedes registrarlo automáticamente con /ee-local add.",
    description: "Extensiones locales: registrar / ver / eliminar servicios MCP de extensiones locales (solo escribe en este equipo)",
    inputHint: "[list|add|remove|snippet] [paquete[@versión]] [--id autor/nombre] [--yes]"
  },
  // index.ts（宿主端）：远程方法里的兜底错误
  serverIndex: {
    microAppNotFound: "No existe esa microaplicación en el directorio",
    notLoggedIn: "No has iniciado sesión en ninguna cuenta de empresa",
    feedbackFailed: "Falló el envío de la valoración: HTTP {{status}}",
    invalidEntryId: "El id de entrada no es válido"
  },
  // kb-doc-render.ts：知识库原文阅读视图（分页标签）
  kbDocRender: {
    page: "Página {{page}}"
  },
  // bridge.ts：MCP 工具结果文字
  bridge: {
    error: "Error: {{text}}"
  },
  // brand-view.ts：没有 logo 时的首字母兜底
  brandView: {
    fallbackLetter: "E"
  }
};
var es_default = es;

// src/i18n/pt.ts
var pt = {
  // Palavras compartilhadas por vários módulos (títulos de view, botões, estado vazio); textos de página ficam no namespace próprio
  common: {
    login: "Entrar",
    refresh: "Atualizar",
    retry: "Tentar novamente",
    save: "Salvar",
    submit: "Enviar",
    close: "Fechar",
    hide: "Ocultar",
    loading: "Carregando…",
    noData: "Nenhum dado",
    notSet: "Não definido",
    viewInWeb: "Ver na web",
    delete: "Excluir",
    newBadge: "Novo",
    inProgress: "Em andamento",
    pendingDecision: "Aguardando sua decisão",
    working: "Trabalhando",
    turnEnded: "Rodada encerrada",
    viewMoreCount: "Ver mais ({{n}})",
    listSeparator: ", "
  },
  // client.ts: entrada do plugin de cliente (fallback de chamada remota, avisos de login/diagnóstico, nome padrão da empresa)
  client: {
    remoteFailed: "Falha na chamada remota da empresa",
    overviewTimeout: "Tempo esgotado ao ler a conta da empresa",
    brandTimeout: "Tempo esgotado ao ler a marca da empresa",
    defaultOrg: "Empresa",
    loginInBrowser: "Conclua o login da empresa no navegador.",
    loginStartFailed: "Falha ao iniciar o login. Verifique a rede e tente novamente.",
    diagnoseFailed: "Falha no diagnóstico. Verifique a rede e tente novamente."
  },
  // apps-client.ts: estado do cliente e avisos de ação do catálogo de aplicativos da empresa
  apps: {
    copied: "{{command}} copiado. Cole no campo de entrada e envie.",
    filled: "Preenchido no campo de entrada. Confirme e envie.",
    loginInBrowser: "Conclua o login da empresa no navegador.",
    loginStartFailed: "Falha ao iniciar o login. Tente novamente mais tarde.",
    openWorkbenchFirst: "Abra primeiro a sessão do aplicativo e depois a área de trabalho.",
    workbenchNotOpen: "A área de trabalho não abriu. Tente novamente mais tarde.",
    enableFailed: "Falha ao ativar: {{error}}",
    enabled: "“{{title}}” ativado. Clique para começar a usar.",
    retryLater: "Tente novamente mais tarde",
    enableFailedNetwork: "Falha ao ativar. Verifique a rede e tente novamente.",
    disableFailed: "Falha ao desativar: {{error}}",
    disabled: "“{{title}}” desativado.",
    disableFailedNetwork: "Falha ao desativar. Verifique a rede e tente novamente.",
    unbound: "Vínculo removido",
    unbindFailed: "Falha ao desvincular. Tente novamente mais tarde.",
    enterpriseApps: "Aplicativos da empresa"
  },
  // commands.ts: textos de conversa e descrições dos comandos /ee-login /ee-status /ee-logout
  commands: {
    pending: "Confirme no navegador. Código {{code}}, link {{url}}",
    starting: "Iniciando o login da empresa…",
    badSetupCode: "Formato de código de acesso inválido. Use algo como ABCD-EFGH; também é possível executar /ee-login sem código de acesso.",
    setupConfirm: "Volte à página “Primeiros passos” do site dsh-ee, confira o código {{code}} e clique em “Autorizar” (se a página estiver fechada, abra {{url}}). Depois volte aqui para continuar a conversa.",
    loginDescription: "Entrar na conta da empresa (opcionalmente com o código de acesso da página “Primeiros passos” da web)",
    setupCodeHint: "[código de acesso]",
    statusDescription: "Ver o status do login da empresa",
    notLoggedIn: "Sem login. Digite /ee-login na conversa.",
    unreachable: "Não foi possível conectar ao dsh-ee. Verifique a rede ou a VPN e tente novamente.",
    sessionExpired: "Sessão expirada. Digite /ee-login na conversa.",
    httpError: "O dsh-ee retornou HTTP {{status}}. Tente novamente mais tarde (não é falha de login).",
    expiresOn: " · expira em {{date}}",
    neverExpires: " · sem validade",
    statusLine: "Conectado como {{username}} ({{department}}) · token {{token}}{{expiry}} · plugin {{version}} · npmrc {{npmrc}}",
    npmrcMatch: "consistente",
    npmrcMismatch: "inconsistente",
    logoutDescription: "Sair da conta da empresa",
    noAccount: "Nenhuma conta da empresa está conectada",
    revokedPrefix: "O token foi revogado no servidor, ",
    envToken: "mas ele vem de uma variável de ambiente ou de $DSH_HOME/.env ({{ref}}). Remova-o manualmente e reinicie o DSH.",
    loggedOut: "Você saiu da conta da empresa",
    loggedOutOffline: "Sessão encerrada nesta máquina; não foi possível conectar ao dsh-ee, então o token não pôde ser revogado no servidor. Você pode revogá-lo em “Meus tokens” na web."
  },
  // doctor.ts: itens de diagnóstico do ee_local_doctor e sugestões de correção
  doctor: {
    appBindingsFix: "Reabrir o microapp cria um novo vínculo; o arquivo corrompido não precisa ser removido manualmente.",
    noCredential: "Não há credencial de login da empresa nesta máquina",
    loginFix: "Digite /ee-login na conversa para entrar",
    hasCredential: "Há credencial de login da empresa nesta máquina",
    source: "Origem: {{source}}",
    expiringSoon: "O login expira em {{days}} dia(s)",
    reLoginFix: "Execute /ee-login antes do vencimento para entrar novamente",
    signedInAs: "Conectado como {{username}} ({{department}})",
    loginExpired: "O login da empresa expirou",
    reLoginFix2: "Digite /ee-login na conversa para entrar novamente",
    readAccountFailed: "Falha ao ler a conta da empresa (HTTP {{status}})",
    serverRestarting: "O dsh-ee pode estar reiniciando ou temporariamente indisponível. Não é falha de login; tente novamente mais tarde.",
    retryDiagnose: "Execute o diagnóstico novamente em instantes; se continuar falhando, verifique o status do servidor.",
    unreachable: "Não foi possível conectar ao dsh-ee ({{base}})",
    unreachableFix: "Verifique a rede ou a VPN. Se o endereço estiver errado, altere em Configurações → dsh-ee no DSH para um endereço dsh-ee que abra no navegador e teste.",
    npmrcOk: "A configuração do registry da empresa no .npmrc do DSH corresponde ao login atual",
    npmrcMissing: "Falta a configuração do registry npm da empresa no .npmrc do DSH",
    npmrcMismatch: "O token do registry da empresa no .npmrc do DSH não corresponde ao login atual",
    npmrcDetail: "{{path}}; instalar plugins da empresa falhará com 401",
    npmrcFix: "Reiniciar o DSH ou entrar novamente sincroniza automaticamente",
    modelsAvailable: "Modelos disponíveis: {{models}}",
    noModels: "Nenhum modelo disponível",
    readModelsFailed: "Falha ao ler a lista de modelos",
    readModelsHttp: "Falha ao ler a lista de modelos (HTTP {{status}})",
    modelPermissionFix: "Peça ao administrador para liberar o acesso aos modelos",
    defaultModelMissing: "O modelo padrão da empresa {{model}} não está na sua lista de modelos disponíveis",
    chooseModelFix: "Escolha um modelo disponível no DSH",
    pluginNewer: "Há uma nova versão do plugin de conexão: {{latest}} (atual {{version}})",
    updateFix: "Em Configurações → dsh-ee no DSH, clique em “Atualizar para a versão mais recente” e reinicie o DSH depois de instalar.",
    pluginLatest: "O plugin de conexão {{version}} está atualizado",
    missingPlugins: "Plugins obrigatórios ausentes: {{plugins}}",
    missingPluginsFix: "Instale-os na página de plugins do DSH, ou reinicie o DSH para que o plugin de conexão os instale automaticamente.",
    pluginsOk: "Todos os plugins obrigatórios estão instalados",
    bridgeFailed: "Falha na sincronização das ferramentas da empresa",
    bridgeFix: "Verifique a rede; digite /ee-status para ver o status do login.",
    bridgeOk: "Ferramentas da empresa sincronizadas ({{count}})",
    summaryFail: "{{fails}} com falha, {{warns}} exigem atenção",
    summaryWarn: "{{warns}} exigem atenção",
    summaryOk: "Tudo certo",
    description: "Verifica a conexão do DSH desta máquina com a área de trabalho da empresa dsh-ee: credencial de login, validade do token, .npmrc, versão do plugin de conexão, plugins obrigatórios, modelos, sincronização das ferramentas da empresa, além de verificações no servidor. Somente leitura; não altera nada."
  },
  // status-view.ts: barra de título e botões do cabeçalho do painel da empresa à direita
  statusView: {
    adminOn: "Ativado",
    adminOff: "Desativado",
    adminRemainingHours: "restam {{hours}} h {{minutes}} min",
    adminRemainingMinutes: "restam {{minutes}} min",
    failureAuth: "Sem login na conta da empresa, ou o login expirou. Faça login.",
    failureServer: "O dsh-ee retornou um erro ({{detail}}). Atualize mais tarde.",
    failureUnreachable: "Não foi possível conectar ao dsh-ee ({{base}}, {{detail}}). Confirme que esta máquina consegue abrir esse endereço no navegador; se o endereço estiver errado, altere em Configurações → dsh-ee no DSH para um que abra e teste.",
    failureDefault: "Sem login ou conexão indisponível. Faça login novamente.",
    reading: "Lendo a conta da empresa…",
    startingLogin: "Iniciando o login…",
    account: "Conta",
    department: "Departamento",
    tokenExpiry: "Validade do token",
    adminMode: "Modo de administração",
    expiresOn: "expira em {{date}}",
    neverExpires: "Sem validade",
    quota: "Cota do mês",
    quotaUnavailable: "Informação de cota indisponível",
    quotaUnlimited: "Ilimitado",
    quotaRemaining: "Restam {{percent}}%",
    models: "Modelos da empresa",
    defaultModel: "Padrão: {{model}}",
    modelsUnavailable: "Nenhum modelo disponível, ou a lista de modelos está indisponível",
    status: "Status da conexão",
    pluginVersion: "Plugin {{version}}{{latest}}",
    pluginNewVersion: " · nova versão {{version}}",
    pluginUpToDate: " · atualizado",
    diagnosing: "Diagnosticando…",
    diagnose: "Diagnóstico",
    diagnoseResult: "Diagnóstico: {{summary}}",
    checkLine: "{{title}}{{fix}}",
    checkFixSeparator: "; ",
    headerDisconnected: "Empresa · Sem login",
    headerChecking: "Empresa · Verificando",
    headerAdmin: "Modo de administração · {{remaining}}",
    headerQuota: "Modelos da empresa · Restam {{percent}}%",
    headerUser: "Modelos da empresa · {{username}}",
    headerTitleUser: "{{username}}{{department}}",
    headerDepartment: " ({{department}})",
    headerTitle: "Entrar novamente na conta da empresa",
    guideDescription: "Ver a conta da empresa, a cota, os modelos e o status da conexão"
  },
  // turn-writes-view.ts: aviso de “registrado” no fim da rodada
  turnWrites: {
    failedOp: "{{label}} falhou{{count}}",
    failedCount: " ×{{count}}",
    failedNotice: "As operações de escrita desta rodada nos sistemas da empresa (algumas com falha) foram registradas no dsh-ee",
    okNotice: "Esta rodada alterou dados nos sistemas da empresa e foi registrada no dsh-ee",
    systemLabel: "“{{system}}”"
  },
  // votes.ts: feedback 👍👎 no fim da mensagem do assistente
  votes: {
    notePrompt: "O que não ajudou nesta resposta? (Opcional; agrupado por pergunta para o autor do aplicativo.)",
    helpful: "Ajudou",
    notHelpful: "Não ajudou",
    label: "Avaliar esta resposta"
  },
  // write-log.ts: log local de operações de escrita
  writeLog: {
    invalidSessionId: "ID de sessão inválido"
  },
  // overview.ts: motivos de falha ao ler a conta da empresa
  overview: {
    timeout: "Tempo de conexão esgotado",
    toolFailed: "Falha na chamada da ferramenta da empresa",
    toolNoJson: "A ferramenta da empresa não retornou JSON"
  },
  // config.ts: valores padrão e validação da configuração do plugin
  config: {
    badBase: "[dsh-ee] base deve ser um endereço http(s)",
    defaultOrg: "Empresa"
  },
  // app-launcher.ts: aviso de resultado incerto ao iniciar um microapp
  appLauncher: {
    sendUnknown: "A mensagem pode não ter sido enviada. Verifique a sessão e tente novamente."
  },
  // host-session.ts: avisos de falha da camada de compatibilidade de sessão do host
  hostSession: {
    switchUnsupported: "O host não suporta trocar de sessão",
    noCwd: "A sessão atual não tem um diretório de trabalho para criar uma nova sessão",
    createWorkspaceUnsupported: "O host não suporta criar espaços de trabalho",
    noWorkspace: "Não foi possível obter um espaço de trabalho para a nova sessão",
    createSessionUnsupported: "O host não suporta criar sessões",
    rejected: "O host rejeitou a mensagem",
    sendUnsupported: "O host não suporta enviar mensagens"
  },
  // device.ts: login por código de dispositivo
  device: {
    startFailed: "Falha ao iniciar o login por dispositivo: {{error}}",
    startHttpFailed: "Falha ao iniciar o login por dispositivo: HTTP {{status}}",
    cancelled: "Login por dispositivo cancelado",
    failed: "Falha no login por dispositivo: HTTP {{status}}",
    timeout: "Tempo esgotado no login por dispositivo"
  },
  // connect-update.ts: atualização do plugin de conexão em um clique
  connectUpdate: {
    noManager: "Este DSH não tem API de gerenciamento de plugins. Atualize o plugin de conexão manualmente na página de plugins.",
    noLatest: "Não foi possível obter a versão do plugin de conexão da empresa: {{why}}{{hint}}",
    signInHint: "; entre na conta da empresa e tente novamente",
    notPublished: "O servidor da empresa ainda não publicou o plugin de conexão. Peça ao administrador para publicá-lo nas configurações do sistema primeiro.",
    upToDate: "Já está atualizado ({{version}})",
    installed: "{{pkg}}@{{latest}} instalado; entra em vigor após reiniciar o DSH",
    installFailed: "Falha na instalação: {{error}}"
  },
  // upload.ts: envio de .tgz pelo ee_plugins.upload
  upload: {
    missingPath: "Falta path: caminho local do arquivo .tgz",
    notTarball: "Só é possível enviar arquivos .tgz gerados por npm pack",
    fileNotFound: "Arquivo não encontrado: {{file}}",
    notAFile: "Não é um arquivo: {{file}}",
    tooLarge: "Arquivo muito grande",
    timeout: "Tempo esgotado no envio. Tente novamente; se a rede estiver lenta, envie pela web.",
    unreachable: "Não foi possível conectar ao dsh-ee. Verifique a rede ou a VPN",
    httpFailed: "Falha no envio (HTTP {{status}})"
  },
  // ee-save.ts: descrição do comando /ee-save (o corpo da solicitação extraída permanece em chinês; o e2e compara com o original)
  eeSave: {
    description: "Transformar esta conversa em uma capacidade (prompt / skill / rascunho de aplicativo)"
  },
  // app-bindings.ts: arquivo de vínculo de sessão do microapp
  appBindings: {
    invalidAppName: "Nome de microapp inválido",
    invalidSessionId: "ID de sessão inválido",
    corruptFile: "O arquivo de vínculo de sessão do microapp está corrompido; tratado como vínculo vazio ({{file}})"
  },
  // app-subscription.ts: ativar/desativar microapps escolhidos
  appSubscription: {
    invalidAppName: "Nome de microapp inválido",
    invalidSkillName: "Nome de habilidade inválido",
    invalidConnectionName: "Nome de conector inválido",
    invalidKind: "Tipo de capacidade inválido"
  },
  // apps-view.ts: barra lateral de aplicativos da empresa, faixa da tela inicial, “Ver mais” e itens em andamento
  appsView: {
    startersTitle: "Comece com uma pergunta de exemplo",
    confirmDisable: "Confirmar desativação",
    disable: "Desativar",
    newSession: "Iniciar uma nova sessão",
    openWorkbench: "Abrir área de trabalho",
    fillInput: "Preencher o campo de entrada",
    unbind: "Desvincular",
    moreActions: "Mais ações para {{title}}",
    missingList: "Indisponível: {{missing}}",
    missing: "Algumas capacidades indisponíveis",
    missingHint: "{{title}}. Clique para ver e solicitar acesso",
    missingApply: "Algumas capacidades indisponíveis · Solicitar",
    todoBadge: "{{count}} pendentes",
    openArrow: "Abrir →",
    enable: "Ativar",
    gallerySubtitle: "{{count}} aplicativos · Clique em um para começar; a IA abre a área de trabalho junto",
    searchAppsAria: "Buscar aplicativos da empresa",
    searchAppsPlaceholder: "Buscar aplicativos…",
    noMatches: "Nenhum aplicativo correspondente",
    availableCount: "Disponíveis ({{count}})",
    showInSidebar: "Mostrar aplicativos da empresa na barra lateral",
    loginToSee: "Entre para ver os aplicativos da empresa",
    expandApps: "Expandir aplicativos da empresa",
    collapseApps: "Recolher aplicativos da empresa",
    hideSidebar: "Ocultar os aplicativos da empresa da barra lateral (para mostrar de novo, use “Ver mais” ou Configurações → dsh-ee)",
    collapsedCount: " ({{count}})",
    allApps: "Todos os aplicativos",
    availablePlus: "+ Disponíveis ({{count}})",
    heroSearchPlaceholder: "Buscar…",
    itemTitle: "{{title}}: {{label}}",
    todosNeed: "{{count}} itens precisam da sua atenção",
    startHint: "Clique em um para começar; a área de trabalho abre à direita",
    tabApps: "Apps",
    tabSkills: "Habilidades",
    tabConnections: "Conectores",
    skillsSubtitle: "{{count}} habilidades · depois de ativadas, disponíveis em todas as sessões",
    connectionsSubtitle: "{{count}} conectores · deixam a IA consultar e operar sistemas da empresa",
    searchSkillsPlaceholder: "Pesquisar habilidades…",
    searchConnectionsPlaceholder: "Pesquisar conectores…",
    noSkills: "Ainda não há habilidades disponíveis",
    noConnections: "Ainda não há conectores disponíveis",
    featured: "Destaque",
    statusAuto: "Ativo por padrão",
    statusEnabled: "Ativado",
    statusOff: "Não ativado",
    statusNeedsAuth: "Requer autorização",
    toolsCount: "{{count}} ferramentas",
    readWrite: "Leitura e escrita",
    readOnly: "Somente leitura",
    authorize: "Autorizar",
    preview: "Pré-visualizar",
    details: "Detalhes",
    needsAuthHint: "{{count}} conectores precisam de autorização; reabra isto depois de autorizar",
    capEnabled: "“{{title}}” ativado"
  },
  // workbench-client.ts: aba da área de trabalho de negócio (painel/fonte, escrita em tabela, lista de verificação, continuar perguntando)
  workbenchClient: {
    fallbackTitle: "Área de trabalho",
    noSession: "Nenhuma sessão atual encontrada. Abra primeiro a sessão deste aplicativo.",
    sentToAi: "Enviado à IA: {{label}}",
    networkError: "Erro de rede",
    noSessionCheck: "Nenhuma sessão atual encontrada; a marcação não será salva",
    checkNotSaved: "A marcação não foi salva. Tente novamente mais tarde.",
    updatePlugin: "Atualize o plugin de conexão para a versão mais recente",
    saved: "Salvo: {{label}}",
    saveFailedNetwork: "Não foi salvo. Verifique a rede e tente novamente.",
    writeFailed: "Não foi possível {{action}}: {{error}}",
    deletedRow: "Linha excluída",
    updated: "Atualizado: {{label}}",
    docLabel: "Documento {{doc}}",
    askPrompt: "Continuar com o documento “{{title}}” da base de conhecimento (documento {{doc}}), nas linhas {{from}}–{{to}} e arredores: quais outras regras ou exceções se aplicam a essa parte? Responda com as fontes.",
    ask: "Perguntar mais neste documento",
    saveFailedDefault: "Não foi salvo. Atualize o plugin de conexão para a versão mais recente."
  },
  // settings-view.ts: página “dsh-ee” nas configurações do DSH (endereço, teste, atualização, barra lateral)
  settingsView: {
    title: "Conexão dsh-ee",
    description: "O plugin de conexão usa este endereço para entrar na conta da empresa, sincronizar skills e microapps da empresa e chamar modelos da empresa. Se não conectar, primeiro abra o site do dsh-ee no navegador e insira aqui o endereço que funciona.",
    reading: "Lendo…",
    addressLabel: "Endereço do dsh-ee",
    packagedOverridden: "O endereço no pacote do plugin é {{packaged}}; você o alterou para o atual.",
    packagedDefault: "Este é o endereço que o administrador gravou ao publicar o plugin de conexão.",
    testing: "Testando…",
    testConnection: "Testar conexão",
    resetAddress: "Restaurar o endereço do pacote do plugin",
    plugin: "Plugin de conexão",
    currentVersion: "Versão atual {{version}}",
    updating: "Atualizando…",
    updateLatest: "Atualizar para a versão mais recente",
    updateHint: "Atualiza para a versão mais recente publicada pelo servidor da empresa; reinicie o DSH para entrar em vigor.",
    changeHint: "Alterar o endereço afeta apenas esta máquina. Se ninguém conseguir conectar, peça ao administrador para preencher o endereço de acesso nas configurações do sistema do site dsh-ee e publicar o plugin de conexão novamente.",
    readFailed: "Falha ao ler as configurações: {{error}}",
    testFailed: "Falha no teste: {{error}}",
    saveFailed: "Falha ao salvar: {{error}}",
    updateFailed: "Falha na atualização: {{error}}",
    savedNotice: "Salvo e aplicado. As skills, os microapps e as ferramentas da empresa estão sendo sincronizados com o novo endereço; se o login não for mais válido, entre novamente pelo painel da empresa na barra lateral.",
    sidebarHint: "A barra lateral lista apenas os 3 primeiros aplicativos; os demais ficam em “Ver mais”. Quando ocultos, os aplicativos da empresa ainda podem ser abertos abaixo do campo de entrada na tela inicial e no painel da empresa à direita.",
    saving: "Salvando…"
  },
  // server-settings.ts: salvamento e teste de conexão do endereço dsh-ee
  serverSettings: {
    reasonTimeout: "Tempo de conexão esgotado: esta máquina não alcança esse endereço (endereço interno, VPN desconectada ou bloqueio por firewall)",
    reasonRefused: "Conexão recusada: o endereço resolve, mas nenhum serviço escuta nessa porta",
    reasonDns: "Domínio não encontrado: verifique a grafia ou se é preciso conectar uma VPN",
    reasonTls: "Problema no certificado HTTPS ({{code}})",
    badAddress: "Formato de endereço inválido",
    badAddressDetail: "Informe um endereço completo http:// ou https://, por exemplo https://ee.example.com, sem parâmetros além do caminho",
    unreachable: "Sem conexão",
    reachable: "Conectável ({{ms}} ms)",
    notDshEe: "Esse endereço não serve o dsh-ee",
    notDshEeDetail: "GET /healthz retornou HTTP {{status}}; confirme se o endereço é o do site do dsh-ee",
    isDshEe: "É o dsh-ee{{version}}",
    versionSuffix: " ({{version}})",
    noLogin: "Esta máquina ainda não entrou em uma conta da empresa",
    noLoginDetail: "Depois de salvar o endereço, clique em “Entrar” no painel da empresa da barra lateral, ou digite /ee-login na conversa.",
    signedIn: "Conectado como {{username}}{{department}}",
    departmentSuffix: " ({{department}})",
    loginInvalid: "O login local não é válido neste servidor dsh-ee",
    loginInvalidDetail: "Salve o endereço e entre novamente (o token é emitido por servidor dsh-ee; trocar de servidor exige um novo login).",
    loginCheckFailed: "Falha ao verificar o login",
    saveBadAddress: "Formato de endereço inválido: informe um endereço completo http:// ou https://"
  },
  // workbench.ts: formatação de dados, placeholders, validação de formulários e do painel (funções puras)
  workbench: {
    unitHundredMillion: " ×100 mi",
    unitTenThousand: " mil",
    unitBillion: " bi",
    unitMillion: " mi",
    flat: "Estável",
    allKbs: "Todas as bases de conhecimento",
    scopePicked: "Bases de conhecimento {{titles}} (IDs {{ids}})",
    kbTitle: "“{{title}}”",
    formRequired: "Preencha “{{label}}”",
    formNumber: "“{{label}}” deve ser um número",
    formSelect: "Escolha “{{label}}” entre as opções",
    blocksRequired: "blocks deve ser um array não vazio",
    blocksTooMany: "O painel de IA pode ter no máximo 20 blocos",
    boardTooLarge: "O conteúdo do painel não pode exceder 32 KB",
    blockWhere: "Bloco {{index}}",
    textEmpty: "{{where}}: text não pode ficar vazio",
    itemsRange: "{{where}}: items deve ter de 1 a 50 itens",
    columnsRange: "{{where}}: columns deve ter de 1 a 12 colunas",
    rowsRange: "{{where}}: rows deve ser um array bidimensional com no máximo 50 linhas",
    citationsRange: "{{where}}: items deve ter de 1 a 30 citações",
    citationAt: "{{where}}, citação {{index}}",
    citationNumbers: "{{at}}: doc, from e to devem ser inteiros positivos, e to não pode ser menor que from",
    citationVersion: "{{at}}: version deve ser um inteiro positivo",
    citationQuote: "{{at}}: quote deve ser um trecho do texto original com 1 a 500 caracteres",
    blockType: "{{where}}: type só pode ser text, list, table ou citations"
  },
  // workbench-host.ts: ferramenta ee_workbench e leitura/escrita do painel e da lista de verificação
  workbenchHost: {
    noComponent: "Não existe este componente no catálogo",
    notReadOnly: "Esta fonte de dados não é uma ferramenta somente leitura; o painel não pode usá-la para obter dados",
    formNoTable: "Este formulário não está configurado para enviar para uma tabela",
    rowNoAction: "Este botão não pode alterar a tabela",
    rowNoId: "Esta linha não tem ID",
    badDoc: "ID de documento inválido",
    invalidAppName: "Nome de microapp inválido",
    invalidSessionId: "ID de sessão inválido",
    invalidChecklistKey: "Chave da lista de verificação inválida",
    actionInvalid: "action só pode ser board ou checklist",
    appNotFound: "Não existe o microapp {{app}}, ou ele não está no seu escopo disponível",
    noBoard: "A área de trabalho do microapp {{app}} não tem painel de IA",
    noBoardSession: "Não foi possível encontrar a sessão cujo painel deve ser atualizado",
    boardUpdated: "Painel de IA da área de trabalho atualizado",
    badCitations: "As citações {{numbers}} não correspondem ao texto original: releia essas linhas com kb__read, corrija e escreva o painel novamente, e corrija o usuário na conversa.",
    staleCitations: "Os documentos das citações {{numbers}} foram atualizados; leia a versão mais recente para confirmar que as conclusões não mudaram.",
    checklistUnsupported: "Esta versão do plugin de conexão não suporta listas de verificação",
    noChecklist: "A área de trabalho do microapp {{app}} não tem lista de verificação",
    checklistNotFound: "Não há lista de verificação com o título “{{title}}”. Disponíveis: {{options}}",
    checklistAmbiguous: "Há {{count}} listas de verificação; especifique uma com title: {{options}}",
    checklistOption: "“{{title}}”",
    doneOrUndone: "Preencha pelo menos um de done e undone",
    itemsNotFound: "A lista de verificação “{{title}}” não tem os itens {{unknown}}; IDs dos itens: {{known}}",
    noChecklistSession: "Não foi possível encontrar a sessão cuja lista de verificação deve ser atualizada",
    checklistUpdated: "Lista de verificação “{{title}}” atualizada: {{done}}/{{total}} concluídos",
    checklistMarked: "Lista de verificação “{{title}}” atualizada: {{count}} itens concluídos",
    description: "Atualiza a área de trabalho do microapp atual: action=board grava o resultado desta rodada no “Painel de IA” (substituindo todo o conteúdo; as citações de base de conhecimento nos blocos citations são verificadas automaticamente); action=checklist marca ou desmarca itens da lista de verificação da área de trabalho (preencha os IDs dos itens em done / undone). Não altera nenhum dado de negócio.",
    paramApp: "Nome do microapp, ou seja, o nome em /nome",
    paramTitle: "checklist: título da lista de verificação; pode ser omitido quando a área de trabalho tem só uma",
    paramDone: "checklist: IDs dos itens a marcar como concluídos",
    paramUndone: "checklist: IDs dos itens a desmarcar",
    paramBlocks: "Conteúdo do painel, exibido de cima para baixo; aparece apenas como texto, sem suporte a HTML",
    paramItems: "list: um texto por item; citations: uma citação de base de conhecimento por item {doc, version, from, to, quote, title}"
  },
  // workbench-view.ts: painel da área de trabalho de negócio (métrica/tabela/gráfico/lista de verificação/formulário/painel/texto original)
  workbenchView: {
    widgetMetric: "Métrica",
    widgetTable: "Tabela",
    widgetChart: "Gráfico",
    widgetChecklist: "Lista de verificação",
    widgetForm: "Formulário",
    widgetActions: "Botões de ação",
    widgetMarkdown: "Notas",
    widgetBoard: "Painel de IA",
    widgetKbScope: "Quais bases de conhecimento",
    confirmDelete: "Confirmar exclusão",
    forbidden: "Sem permissão: você não tem acesso a esta fonte de dados",
    fetchFailed: "Falha ao obter dados: {{error}}",
    forbiddenDetail: "Sem permissão: você ainda não tem acesso a “{{tool}}” em “{{connection}}”.",
    thisDataSource: "esta fonte de dados",
    requestAccess: "Solicitar acesso ↗",
    contactAdmin: "Fale com o administrador para liberar o acesso.",
    countRows: "{{total}} linhas",
    expandDetail: "Expandir detalhes",
    collapseDetail: "Recolher detalhes",
    truncatedRows: "{{total}} linhas no total; exibindo as primeiras {{shown}}",
    pointTip: "{{label}} · {{series}}: {{value}}",
    pieTip: "{{label}}: {{value}} ({{percent}}%)",
    selectPlaceholder: "Selecione",
    citationOk: "✔ Confere com o texto original",
    citationStale: "Texto original atualizado",
    citationMismatch: "✖ Não corresponde ao texto original",
    citationMissing: "✖ Documento não encontrado",
    citationUnchecked: "Sem verificação",
    citationWhere: "{{title}} · linhas {{range}}",
    citationTip: "{{where}}\n“{{quote}}”",
    webSourceTip: "{{tip}}\nVer o texto original no site do dsh-ee",
    quoteText: "“{{quote}}”",
    viewSource: "Ver texto original: {{where}}",
    nearSkippedTip: "Essa página é digitalizada ou está ilegível e não foi indexada na base de conhecimento; a resposta pode estar sem o conteúdo dela. Consulte o documento original.",
    nearSkipped: "⚠ Página {{page}} não indexada",
    noKbs: "Você ainda não tem bases de conhecimento para pesquisar",
    kbDocs: "{{count}} documentos",
    scopeOnly: "Pesquisar apenas as {{count}} bases selecionadas",
    scopeAllHint: "Quando nada está selecionado, pesquisa todas",
    boardEmpty: "A IA escreve o resultado aqui quando terminar.",
    updatedAt: "Atualizado em {{time}}",
    unavailable: "Esta área de trabalho não está mais disponível: o aplicativo pode ter sido desativado ou a configuração da área de trabalho mudou. Você pode fechar esta aba.",
    openCitationHint: "Clique em uma citação em “Respostas e fontes” para abrir o texto original aqui.",
    versionLine: "Versão {{version}} · linhas {{from}}–{{to}} de {{total}}",
    staleNotice: "{{stale}}; o destaque mostra a versão no momento da citação.",
    prevPage: "Página anterior",
    nextPage: "Próxima página",
    readView: "Modo leitura",
    rawView: "Ver o texto original (com números de linha)",
    quotedSource: "Texto original citado: “{{quote}}”"
  },
  // local-mcp.ts: comando /ee-local e registro MCP de extensões locais
  localMcp: {
    missingPackage: "É preciso informar o nome do pacote npm, por exemplo /ee-local add @acme/legacy-oa-local@0.1.0",
    notNpmPackage: "“{{text}}” não é um nome de pacote npm: as extensões locais são instaladas apenas a partir do registry npm da empresa; URLs, caminhos locais e endereços git não são aceitos",
    invalidPackage: "“{{pkg}}” não é um nome de pacote npm válido (deve ser @scope/name ou name)",
    invalidVersion: "“{{version}}” não é uma versão válida",
    unknownSubcommand: "Os subcomandos são list / add / remove / snippet; recebido “{{action}}”",
    idHint: "--id deve ser seguido de um ID de extensão, por exemplo acme/legacy-oa",
    invalidId: "“{{value}}” não é um ID de extensão válido (deve ser acme/legacy-oa)",
    unknownFlag: "Opção desconhecida “{{token}}”",
    missingTarget: "{{action}} deve ser seguido de um nome de pacote ou ID de extensão",
    extraArgs: "Argumentos extras inesperados “{{args}}”",
    duplicateService: "Já existe um serviço MCP com o nome {{name}}; não será sobrescrito",
    noManagerHint: "\nEste DSH não tem API de gerenciamento de MCP (@deepseek-ai/dsh-mcp-manager), então o registro automático não está disponível; use /ee-local snippet <pacote> para gerar o trecho de configuração.\n",
    noExtensions: "Ainda não há extensões locais.\nUso: /ee-local add <pacote>[@versão] [--id autor/nome]\n{{hint}}",
    registered: "Registrado",
    unregistered: "Não registrado",
    unregisteredNoManager: "Não registrado (sem API de gerenciamento de MCP)",
    listHeader: "Extensões locais ({{count}}):\n{{lines}}\n{{hint}}\nRemover: /ee-local remove <ID de extensão ou nome do pacote>",
    alreadyAdded: "{{spec}} já foi adicionado; para alterar o ID ou a versão, execute /ee-local remove primeiro.",
    idTaken: "O ID de extensão {{id}} já está em uso por {{pkg}}; execute /ee-local remove {{id}} primeiro, ou use outro --id.",
    packageBound: "Este pacote já foi adicionado com o ID de extensão {{id}}; para trocar o ID, execute /ee-local remove {{id}} primeiro.",
    noManagerAdd: "Este DSH não tem API de gerenciamento de MCP (@deepseek-ai/dsh-mcp-manager não está montado; versões antigas do DSH ou composições mínimas não o têm), então o registro automático não está disponível.\nAcrescente o trecho abaixo ao fim do cordis.patch.yml do profile (ou atualize o DSH para a 0.2.2 e tente novamente):\n\n{{snippet}}\n",
    serviceConflict: "Já existe aqui um serviço MCP com o nome {{name}} (configurado pelo próprio funcionário, registrado por outro plugin ou adicionado antes); não será sobrescrito.\nUsar outro --id gera outro nome de serviço, por exemplo: /ee-local add {{spec}} --id {{scope}}/<outro nome>\n",
    confirmPlan: "{{update}}registrar a extensão local como serviço MCP nesta máquina (grava apenas esta máquina, não altera o dsh-ee e não sobrescreve serviços existentes):\n  Extensão      {{id}}\n  Pacote        {{pkg}}{{latest}}\n  Serviço MCP   {{server}} (nomes de ferramenta mcp__{{server}}__<ferramenta>)\n  env           DSH_EE_URL, DSH_EE_TOKEN (via cofre de credenciais do DSH), DSH_EE_HARNESS=dsh, registry npm da empresa\n\nApós confirmar, execute: /ee-local add {{spec}}{{idFlag}} --yes\n",
    updateSuffix: "Depois de atualizar para a nova versão, ",
    latestVersion: " (versão mais recente)",
    registerFailed: "Falha no registro: {{error}}",
    registeredText: "Registrado (restaurado automaticamente na próxima inicialização do DSH):\n  {{id}} → {{spec}}, nome do serviço {{server}}\nOs nomes das ferramentas começam com mcp__{{server}}__; se a extensão exigir login na primeira chamada, aparecerá a janela de login do próprio sistema.\nRemover: /ee-local remove {{id}}\n",
    notFound: "“{{target}}” não encontrado; execute /ee-local list para ver as extensões adicionadas.",
    removed: "{{id}} ({{pkg}}) removido; o serviço MCP {{server}} foi removido do registro.",
    snippetIntro: "Acrescente o trecho abaixo ao fim do cordis.patch.yml do profile correspondente (o DSH não tem arquivo de configuração mcpServers; esta é a forma oficial de configuração):\n\n{{snippet}}\n\n{{hint}}\n",
    snippetHint: "Este DSH tem API de gerenciamento de MCP; também é possível registrar automaticamente com /ee-local add.",
    description: "Extensões locais: registrar / listar / remover serviços MCP de extensões locais (grava apenas esta máquina)",
    inputHint: "[list|add|remove|snippet] [pacote[@versão]] [--id autor/nome] [--yes]"
  },
  // index.ts (lado do host): erros de fallback dos métodos remotos
  serverIndex: {
    microAppNotFound: "Não existe este microapp no catálogo",
    notLoggedIn: "Sem login na conta da empresa",
    feedbackFailed: "Falha no feedback: HTTP {{status}}",
    invalidEntryId: "ID de entrada inválido"
  },
  // kb-doc-render.ts: leitura do texto original da base de conhecimento (rótulo de paginação)
  kbDocRender: {
    page: "Página {{page}}"
  },
  // bridge.ts: texto do resultado das ferramentas MCP
  bridge: {
    error: "Erro: {{text}}"
  },
  // brand-view.ts: letra inicial de fallback quando não há logo
  brandView: {
    fallbackLetter: "E"
  }
};
var pt_default = pt;

// src/i18n/hi.ts
var hi = {
  // words shared by several modules (view titles / buttons / empty states)
  common: {
    login: "साइन इन करें",
    refresh: "रीफ़्रेश करें",
    retry: "फिर से प्रयास करें",
    save: "सहेजें",
    submit: "सबमिट करें",
    close: "बंद करें",
    hide: "छिपाएँ",
    loading: "लोड हो रहा है…",
    noData: "कोई डेटा नहीं",
    notSet: "सेट नहीं है",
    viewInWeb: "वेब पर देखें",
    delete: "हटाएँ",
    newBadge: "नया",
    inProgress: "प्रगति पर",
    pendingDecision: "आपका निर्णय लंबित",
    working: "काम जारी है",
    turnEnded: "टर्न समाप्त",
    viewMoreCount: "और देखें ({{n}})",
    listSeparator: ", "
  },
  // client.ts: client plugin entry (remote-call fallbacks, sign-in / diagnostics hints, default org name)
  client: {
    remoteFailed: "एंटरप्राइज़ रिमोट कॉल विफल रहा",
    overviewTimeout: "एंटरप्राइज़ खाता पढ़ने में समय समाप्त",
    brandTimeout: "एंटरप्राइज़ ब्रांड पढ़ने में समय समाप्त",
    defaultOrg: "एंटरप्राइज़",
    loginInBrowser: "कृपया ब्राउज़र में एंटरप्राइज़ साइन इन पूरा करें।",
    loginStartFailed: "साइन इन शुरू नहीं हो सका। नेटवर्क जाँचें और फिर से प्रयास करें।",
    diagnoseFailed: "निदान विफल रहा। नेटवर्क जाँचें और फिर से प्रयास करें।"
  },
  // apps-client.ts: client-side state and action hints for the enterprise app catalog
  apps: {
    copied: "{{command}} कॉपी किया गया। इसे इनपुट बॉक्स में पेस्ट करके भेजें।",
    filled: "इनपुट बॉक्स में भर दिया गया है, जाँचकर भेजें।",
    loginInBrowser: "कृपया ब्राउज़र में एंटरप्राइज़ साइन इन पूरा करें।",
    loginStartFailed: "साइन इन शुरू नहीं हो सका। थोड़ी देर बाद फिर से प्रयास करें।",
    openWorkbenchFirst: "पहले ऐप पर क्लिक करके उसका सत्र खोलें, फिर वर्कबेंच खोलें",
    workbenchNotOpen: "वर्कबेंच नहीं खुला, थोड़ी देर बाद फिर से प्रयास करें",
    enableFailed: "सक्षम करने में विफल: {{error}}",
    enabled: "“{{title}}” सक्षम कर दिया गया, शुरू करने के लिए उस पर क्लिक करें",
    retryLater: "थोड़ी देर बाद प्रयास करें",
    enableFailedNetwork: "सक्षम नहीं हो सका, नेटवर्क जाँचें और फिर से प्रयास करें।",
    disableFailed: "अक्षम करने में विफल: {{error}}",
    disabled: "“{{title}}” अक्षम कर दिया गया",
    disableFailedNetwork: "अक्षम नहीं हो सका, नेटवर्क जाँचें और फिर से प्रयास करें।",
    unbound: "अनबाइंड कर दिया गया",
    unbindFailed: "अनबाइंड नहीं हो सका, थोड़ी देर बाद फिर से प्रयास करें।",
    enterpriseApps: "एंटरप्राइज़ ऐप्स"
  },
  // commands.ts: chat messages and command descriptions for /ee-login /ee-status /ee-logout
  commands: {
    pending: "कृपया ब्राउज़र में पुष्टि करें, पुष्टि कोड {{code}}, लिंक {{url}}",
    starting: "एंटरप्राइज़ साइन इन शुरू हो रहा है, कृपया प्रतीक्षा करें…",
    badSetupCode: "एक्सेस कोड का प्रारूप गलत है, यह ABCD-EFGH जैसा होना चाहिए; आप बिना एक्सेस कोड के भी सीधे /ee-login चला सकते हैं",
    setupConfirm: "dsh-ee वेबसाइट के “शुरू करें” पृष्ठ पर लौटें, पुष्टि कोड {{code}} जाँचें और “अधिकृत करें” पर क्लिक करें (पृष्ठ बंद हो तो {{url}} खोलें)। इसके बाद बातचीत जारी रखने के लिए यहाँ वापस आएँ।",
    loginDescription: "एंटरप्राइज़ खाते में साइन इन करें (वेब के “शुरू करें” पृष्ठ से मिला एक्सेस कोड भी दे सकते हैं)",
    setupCodeHint: "[एक्सेस कोड]",
    statusDescription: "एंटरप्राइज़ साइन इन स्थिति देखें",
    notLoggedIn: "साइन इन नहीं है, चैट में /ee-login टाइप करें",
    unreachable: "dsh-ee से कनेक्ट नहीं हो सका, नेटवर्क या VPN जाँचें और फिर से प्रयास करें",
    sessionExpired: "साइन इन समाप्त हो गया, चैट में /ee-login टाइप करें",
    httpError: "dsh-ee ने HTTP {{status}} लौटाया, बाद में प्रयास करें (यह साइन इन विफलता नहीं है)",
    expiresOn: " · {{date}} को समाप्त",
    neverExpires: " · कभी समाप्त नहीं होता",
    statusLine: "{{username}} ({{department}}) के रूप में साइन इन · टोकन {{token}}{{expiry}} · प्लगइन {{version}} · npmrc {{npmrc}}",
    npmrcMatch: "मेल खाता है",
    npmrcMismatch: "मेल नहीं खाता",
    logoutDescription: "एंटरप्राइज़ खाते से साइन आउट करें",
    noAccount: "कोई एंटरप्राइज़ खाता साइन इन नहीं है",
    revokedPrefix: "टोकन सर्वर पर रद्द कर दिया गया है, ",
    envToken: "लेकिन यह एनवायरनमेंट वेरिएबल या $DSH_HOME/.env ({{ref}}) से आता है, कृपया इसे मैन्युअल रूप से हटाएँ और फिर DSH को पुनः आरंभ करें",
    loggedOut: "एंटरप्राइज़ खाते से साइन आउट कर दिया गया",
    loggedOutOffline: "इस मशीन पर साइन आउट हो गया; dsh-ee से कनेक्ट नहीं हो सका, इसलिए टोकन सर्वर पर रद्द नहीं किया जा सका; आप इसे वेब के “मेरे टोकन” में रद्द कर सकते हैं"
  },
  // doctor.ts: ee_local_doctor checks and suggested fixes
  doctor: {
    appBindingsFix: "माइक्रो-ऐप फिर से खोलने पर नई बाइंडिंग बन जाएगी; क्षतिग्रस्त फ़ाइल को मैन्युअल रूप से हटाने की आवश्यकता नहीं है",
    noCredential: "इस मशीन पर एंटरप्राइज़ साइन इन क्रेडेंशियल नहीं है",
    loginFix: "साइन इन करने के लिए चैट में /ee-login टाइप करें",
    hasCredential: "इस मशीन पर एंटरप्राइज़ साइन इन क्रेडेंशियल मौजूद है",
    source: "स्रोत: {{source}}",
    expiringSoon: "साइन इन {{days}} दिनों में समाप्त हो जाएगा",
    reLoginFix: "समाप्त होने से पहले /ee-login चलाकर फिर से साइन इन करें",
    signedInAs: "{{username}} ({{department}}) के रूप में साइन इन",
    loginExpired: "एंटरप्राइज़ साइन इन समाप्त हो गया है",
    reLoginFix2: "फिर से साइन इन करने के लिए चैट में /ee-login टाइप करें",
    readAccountFailed: "एंटरप्राइज़ खाता पढ़ने में विफल (HTTP {{status}})",
    serverRestarting: "dsh-ee पुनः आरंभ हो रहा हो सकता है या अस्थायी रूप से उपलब्ध नहीं है; यह साइन इन विफलता नहीं है, बाद में प्रयास करें",
    retryDiagnose: "थोड़ी देर बाद निदान फिर चलाएँ; लगातार विफल हो तो सर्वर की स्थिति जाँचें",
    unreachable: "dsh-ee से कनेक्ट नहीं हो सका ({{base}})",
    unreachableFix: "नेटवर्क या VPN जाँचें; पता गलत हो तो DSH में “सेटिंग्स → dsh-ee” के अंतर्गत इसे ऐसे dsh-ee पते में बदलें जो ब्राउज़र में खुल जाए और फिर जाँचें",
    npmrcOk: "DSH की .npmrc में एंटरप्राइज़ रजिस्ट्री कॉन्फ़िग वर्तमान साइन इन से मेल खाता है",
    npmrcMissing: "DSH की .npmrc में एंटरप्राइज़ npm रजिस्ट्री कॉन्फ़िग नहीं है",
    npmrcMismatch: "DSH की .npmrc में एंटरप्राइज़ रजिस्ट्री टोकन वर्तमान साइन इन से मेल नहीं खाता",
    npmrcDetail: "{{path}}; एंटरप्राइज़ प्लगइन इंस्टॉल करने पर 401 त्रुटि आएगी",
    npmrcFix: "DSH पुनः आरंभ करने या फिर से साइन इन करने पर यह अपने आप सिंक हो जाएगा",
    modelsAvailable: "उपलब्ध मॉडल: {{models}}",
    noModels: "कोई मॉडल उपलब्ध नहीं है",
    readModelsFailed: "मॉडल सूची पढ़ने में विफल",
    readModelsHttp: "मॉडल सूची पढ़ने में विफल (HTTP {{status}})",
    modelPermissionFix: "मॉडल तक पहुँच के लिए प्रशासक से संपर्क करें",
    defaultModelMissing: "एंटरप्राइज़ डिफ़ॉल्ट मॉडल {{model}} आपकी उपलब्ध सूची में नहीं है",
    chooseModelFix: "DSH में उपलब्ध कोई मॉडल चुनें",
    pluginNewer: "कनेक्ट प्लगइन का नया संस्करण उपलब्ध है {{latest}} (वर्तमान {{version}})",
    updateFix: "DSH में “सेटिंग्स → dsh-ee” के अंतर्गत “नवीनतम में अपडेट करें” पर क्लिक करें, इंस्टॉल होने पर DSH पुनः आरंभ करें",
    pluginLatest: "कनेक्ट प्लगइन {{version}} नवीनतम है",
    missingPlugins: "आवश्यक प्लगइन गायब हैं: {{plugins}}",
    missingPluginsFix: "DSH के प्लगइन पृष्ठ पर इंस्टॉल करें, या DSH पुनः आरंभ करें ताकि कनेक्ट प्लगइन उन्हें अपने आप इंस्टॉल कर दे",
    pluginsOk: "सभी आवश्यक प्लगइन इंस्टॉल हैं",
    bridgeFailed: "एंटरप्राइज़ टूल सिंक विफल",
    bridgeFix: "नेटवर्क जाँचें; साइन इन स्थिति देखने के लिए /ee-status टाइप करें",
    bridgeOk: "एंटरप्राइज़ टूल सिंक हो गए ({{count}})",
    summaryFail: "{{fails}} विफल, {{warns}} पर ध्यान दें",
    summaryWarn: "{{warns}} पर ध्यान दें",
    summaryOk: "सब ठीक है",
    description: "इस मशीन पर DSH और एंटरप्राइज़ वर्कबेंच dsh-ee के कनेक्शन की जाँच करें: साइन इन क्रेडेंशियल, टोकन की वैधता, .npmrc, कनेक्ट प्लगइन संस्करण, आवश्यक प्लगइन, मॉडल, एंटरप्राइज़ टूल सिंक, साथ ही सर्वर-साइड जाँच। केवल पढ़ता है, कुछ भी नहीं बदलता।"
  },
  // status-view.ts: enterprise panel title bar and header buttons
  statusView: {
    adminOn: "चालू",
    adminOff: "बंद",
    adminRemainingHours: "{{hours}} घंटे {{minutes}} मिनट शेष",
    adminRemainingMinutes: "{{minutes}} मिनट शेष",
    failureAuth: "एंटरप्राइज़ खाते में साइन इन नहीं है, या साइन इन समाप्त हो गया है। कृपया साइन इन करें।",
    failureServer: "dsh-ee ने त्रुटि लौटाई ({{detail}}), बाद में रीफ़्रेश करें।",
    failureUnreachable: "dsh-ee से कनेक्ट नहीं हो सका ({{base}}, {{detail}})। सुनिश्चित करें कि यह मशीन ब्राउज़र में उस पते को खोल सकती है; पता गलत हो तो DSH में “सेटिंग्स → dsh-ee” के अंतर्गत इसे खुलने वाले पते में बदलें और जाँचें।",
    failureDefault: "साइन इन नहीं है या कनेक्शन उपलब्ध नहीं है, कृपया फिर से साइन इन करें।",
    reading: "एंटरप्राइज़ खाता पढ़ा जा रहा है…",
    startingLogin: "साइन इन शुरू हो रहा है…",
    account: "खाता",
    department: "विभाग",
    tokenExpiry: "टोकन की वैधता",
    adminMode: "प्रशासन मोड",
    expiresOn: "{{date}} को समाप्त",
    neverExpires: "कभी समाप्त नहीं होता",
    quota: "इस माह का कोटा",
    quotaUnavailable: "कोटा जानकारी उपलब्ध नहीं है",
    quotaUnlimited: "असीमित",
    quotaRemaining: "{{percent}}% शेष",
    models: "एंटरप्राइज़ मॉडल",
    defaultModel: "डिफ़ॉल्ट: {{model}}",
    modelsUnavailable: "कोई मॉडल उपलब्ध नहीं है या मॉडल सूची उपलब्ध नहीं है",
    status: "कनेक्शन स्थिति",
    pluginVersion: "प्लगइन {{version}}{{latest}}",
    pluginNewVersion: " · नया संस्करण {{version}}",
    pluginUpToDate: " · नवीनतम",
    diagnosing: "निदान जारी है…",
    diagnose: "निदान",
    diagnoseResult: "निदान: {{summary}}",
    checkLine: "{{title}}{{fix}}",
    checkFixSeparator: "; ",
    headerDisconnected: "एंटरप्राइज़ · साइन इन नहीं",
    headerChecking: "एंटरप्राइज़ · जाँच जारी",
    headerAdmin: "प्रशासन मोड · {{remaining}}",
    headerQuota: "एंटरप्राइज़ मॉडल · {{percent}}% शेष",
    headerUser: "एंटरप्राइज़ मॉडल · {{username}}",
    headerTitleUser: "{{username}}{{department}}",
    headerDepartment: " ({{department}})",
    headerTitle: "एंटरप्राइज़ खाते में फिर से साइन इन करें",
    guideDescription: "एंटरप्राइज़ खाता, कोटा, मॉडल और कनेक्शन स्थिति देखें"
  },
  // turn-writes-view.ts: the "logged in dsh-ee" notice at the end of a turn
  turnWrites: {
    failedOp: "{{label}} विफल{{count}}",
    failedCount: " ×{{count}}",
    failedNotice: "इस टर्न में एंटरप्राइज़ सिस्टम में किए गए लेखन कार्य (कुछ विफल) dsh-ee में दर्ज कर दिए गए हैं",
    okNotice: "इस टर्न में एंटरप्राइज़ सिस्टम का डेटा बदला गया, dsh-ee में दर्ज कर दिया गया है",
    systemLabel: "“{{system}}”"
  },
  // votes.ts: 👍👎 feedback at the end of assistant messages
  votes: {
    notePrompt: "इस उत्तर में कौन-सी बात सहायक नहीं रही? (खाली छोड़ सकते हैं, प्रश्न के अनुसार समूहित कर ऐप लेखक को दी जाएगी)",
    helpful: "सहायक",
    notHelpful: "सहायक नहीं",
    label: "इस उत्तर को रेट करें"
  },
  // write-log.ts: local write-operation log
  writeLog: {
    invalidSessionId: "अमान्य सत्र id"
  },
  // overview.ts: reasons an enterprise account read can fail
  overview: {
    timeout: "कनेक्शन का समय समाप्त",
    toolFailed: "एंटरप्राइज़ टूल कॉल विफल",
    toolNoJson: "एंटरप्राइज़ टूल ने JSON नहीं लौटाया"
  },
  // config.ts: plugin config defaults and validation
  config: {
    badBase: "[dsh-ee] base एक http(s) पता होना चाहिए",
    defaultOrg: "एंटरप्राइज़"
  },
  // app-launcher.ts: uncertain send result when launching a micro-app
  appLauncher: {
    sendUnknown: "संभव है कि संदेश न भेजा गया हो, सत्र देखकर फिर से प्रयास करें"
  },
  // host-session.ts: failure hints from the host session compatibility layer
  hostSession: {
    switchUnsupported: "होस्ट सत्र बदलने का समर्थन नहीं करता",
    noCwd: "वर्तमान सत्र में नए सत्र के लिए कोई कार्य निर्देशिका नहीं है",
    createWorkspaceUnsupported: "होस्ट वर्कस्पेस बनाने का समर्थन नहीं करता",
    noWorkspace: "नए सत्र के लिए वर्कस्पेस प्राप्त नहीं हो सका",
    createSessionUnsupported: "होस्ट नया सत्र बनाने का समर्थन नहीं करता",
    rejected: "होस्ट ने संदेश अस्वीकार कर दिया",
    sendUnsupported: "होस्ट संदेश भेजने का समर्थन नहीं करता"
  },
  // device.ts: device-code sign-in
  device: {
    startFailed: "डिवाइस कोड साइन इन शुरू करने में विफल: {{error}}",
    startHttpFailed: "डिवाइस कोड साइन इन शुरू करने में विफल: HTTP {{status}}",
    cancelled: "डिवाइस कोड साइन इन रद्द कर दिया गया",
    failed: "डिवाइस कोड साइन इन विफल: HTTP {{status}}",
    timeout: "डिवाइस कोड साइन इन का समय समाप्त"
  },
  // connect-update.ts: one-click update of the connect plugin
  connectUpdate: {
    noManager: "इस DSH में प्लगइन प्रबंधन API नहीं है, कृपया प्लगइन पृष्ठ पर कनेक्ट प्लगइन मैन्युअल रूप से अपडेट करें",
    noLatest: "एंटरप्राइज़ का कनेक्ट प्लगइन संस्करण नहीं मिला: {{why}}{{hint}}",
    signInHint: "; पहले एंटरप्राइज़ खाते में साइन इन करके फिर प्रयास करें",
    notPublished: "एंटरप्राइज़ ने अभी तक कनेक्ट प्लगइन प्रकाशित नहीं किया है, कृपया प्रशासक पहले सिस्टम सेटिंग्स में इसे प्रकाशित करें",
    upToDate: "पहले से नवीनतम ({{version}})",
    installed: "{{pkg}}@{{latest}} इंस्टॉल हो गया, DSH पुनः आरंभ करने के बाद प्रभावी होगा",
    installFailed: "इंस्टॉल विफल: {{error}}"
  },
  // upload.ts: ee_plugins.upload of .tgz files
  upload: {
    missingPath: "path गायब है: .tgz फ़ाइल का स्थानीय पथ",
    notTarball: "केवल npm pack से बनी .tgz फ़ाइलें अपलोड की जा सकती हैं",
    fileNotFound: "फ़ाइल नहीं मिली: {{file}}",
    notAFile: "यह फ़ाइल नहीं है: {{file}}",
    tooLarge: "फ़ाइल बहुत बड़ी है",
    timeout: "अपलोड का समय समाप्त, फिर से प्रयास करें; नेटवर्क धीमा हो तो वेब से अपलोड करें",
    unreachable: "dsh-ee से कनेक्ट नहीं हो सका, नेटवर्क या VPN जाँचें",
    httpFailed: "अपलोड विफल (HTTP {{status}})"
  },
  // ee-save.ts: /ee-save command description (the distilled request body stays Chinese; e2e compares it)
  eeSave: {
    description: "इस बातचीत को क्षमता में बदलें (प्रॉम्प्ट / स्किल / ऐप ड्राफ़्ट)"
  },
  // app-bindings.ts: micro-app session binding file
  appBindings: {
    invalidAppName: "अमान्य माइक्रो-ऐप नाम",
    invalidSessionId: "अमान्य सत्र id",
    corruptFile: "माइक्रो-ऐप सत्र बाइंडिंग फ़ाइल क्षतिग्रस्त है, इसे खाली बाइंडिंग मानकर संभाला गया ({{file}})"
  },
  // app-subscription.ts: enabling / disabling self-selected micro-apps
  appSubscription: {
    invalidAppName: "अमान्य माइक्रो-ऐप नाम",
    invalidSkillName: "अमान्य स्किल नाम",
    invalidConnectionName: "अमान्य कनेक्टर नाम",
    invalidKind: "अमान्य क्षमता प्रकार"
  },
  // apps-view.ts: enterprise app sidebar / home row / "view more" flyout / in-progress
  appsView: {
    startersTitle: "उदाहरण प्रश्न से शुरू करें",
    confirmDisable: "अक्षम करने की पुष्टि करें",
    disable: "अक्षम करें",
    newSession: "नया सत्र शुरू करें",
    openWorkbench: "वर्कबेंच खोलें",
    fillInput: "इनपुट बॉक्स में भरें",
    unbind: "अनबाइंड करें",
    moreActions: "{{title}} के लिए और क्रियाएँ",
    missingList: "अनुपलब्ध: {{missing}}",
    missing: "कुछ क्षमताएँ अनुपलब्ध हैं",
    missingHint: "{{title}}। देखने और अनुरोध करने के लिए क्लिक करें",
    missingApply: "कुछ क्षमताएँ अनुपलब्ध · अनुरोध करें",
    todoBadge: "{{count}} लंबित",
    openArrow: "खोलें →",
    enable: "सक्षम करें",
    gallerySubtitle: "{{count}} ऐप्स · किसी एक पर क्लिक करके शुरू करें, AI उसे वर्कबेंच के साथ खोलेगा",
    searchAppsAria: "एंटरप्राइज़ ऐप्स खोजें",
    searchAppsPlaceholder: "ऐप्स खोजें…",
    noMatches: "कोई मेल खाता ऐप नहीं",
    availableCount: "सक्षम करने योग्य ({{count}})",
    showInSidebar: "साइडबार में एंटरप्राइज़ ऐप्स दिखाएँ",
    loginToSee: "एंटरप्राइज़ ऐप्स देखने के लिए साइन इन करें",
    expandApps: "एंटरप्राइज़ ऐप्स विस्तारित करें",
    collapseApps: "एंटरप्राइज़ ऐप्स संक्षिप्त करें",
    hideSidebar: "साइडबार से एंटरप्राइज़ ऐप्स छिपाएँ (“और देखें” या सेटिंग्स → dsh-ee में इन्हें फिर से चालू किया जा सकता है)",
    collapsedCount: " ({{count}})",
    allApps: "सभी ऐप्स",
    availablePlus: "+ सक्षम करने योग्य ({{count}})",
    heroSearchPlaceholder: "खोजें…",
    itemTitle: "{{title}}: {{label}}",
    todosNeed: "आपके लिए {{count}} आइटम लंबित",
    startHint: "शुरू करने के लिए किसी एक पर क्लिक करें, वर्कबेंच दाईं ओर खुलेगा",
    tabApps: "ऐप्स",
    tabSkills: "स्किल्स",
    tabConnections: "कनेक्टर",
    skillsSubtitle: "{{count}} स्किल्स · चालू करने के बाद हर सत्र में उपलब्ध",
    connectionsSubtitle: "{{count}} कनेक्टर · AI को कंपनी के सिस्टम से जानकारी लेने और काम करने देते हैं",
    searchSkillsPlaceholder: "स्किल्स खोजें…",
    searchConnectionsPlaceholder: "कनेक्टर खोजें…",
    noSkills: "अभी कोई स्किल उपलब्ध नहीं है",
    noConnections: "अभी कोई कनेक्टर उपलब्ध नहीं है",
    featured: "चुनिंदा",
    statusAuto: "डिफ़ॉल्ट रूप से चालू",
    statusEnabled: "चालू",
    statusOff: "चालू नहीं",
    statusNeedsAuth: "अनुमति चाहिए",
    toolsCount: "{{count}} टूल",
    readWrite: "पढ़ना और लिखना",
    readOnly: "केवल पढ़ना",
    authorize: "अनुमति दें",
    preview: "पूर्वावलोकन",
    details: "विवरण",
    needsAuthHint: "{{count}} कनेक्टर को पहले अनुमति चाहिए; अनुमति देने के बाद इसे फिर खोलें",
    capEnabled: "“{{title}}” चालू किया गया"
  },
  // workbench-client.ts: business workbench tab (panel / source pane, row writes, checklists, follow-up)
  workbenchClient: {
    fallbackTitle: "वर्कबेंच",
    noSession: "वर्तमान सत्र नहीं मिला, कृपया पहले इस ऐप का सत्र खोलें",
    sentToAi: "AI को भेजा गया: {{label}}",
    networkError: "नेटवर्क त्रुटि",
    noSessionCheck: "वर्तमान सत्र नहीं मिला, चयन सहेजा नहीं जाएगा",
    checkNotSaved: "चयन सहेजा नहीं गया, थोड़ी देर बाद फिर से प्रयास करें",
    updatePlugin: "कृपया कनेक्ट प्लगइन को नवीनतम संस्करण में अपडेट करें",
    saved: "सहेजा गया: {{label}}",
    saveFailedNetwork: "सहेजा नहीं गया, नेटवर्क जाँचें और फिर से प्रयास करें",
    writeFailed: "कार्रवाई “{{action}}” विफल: {{error}}",
    deletedRow: "यह पंक्ति हटा दी गई",
    updated: "अपडेट किया गया: {{label}}",
    docLabel: "दस्तावेज़ {{doc}}",
    askPrompt: "ज्ञान आधार दस्तावेज़ “{{title}}” (दस्तावेज़ {{doc}}) की पंक्ति {{from}}–{{to}} के आसपास के भाग पर आगे बताएँ: उस भाग में और कौन-से संबंधित नियम या अपवाद लागू होते हैं? कृपया स्रोत का उल्लेख करके उत्तर दें।",
    ask: "इस दस्तावेज़ में और पूछें",
    saveFailedDefault: "सहेजा नहीं गया, कृपया कनेक्ट प्लगइन को नवीनतम संस्करण में अपडेट करें"
  },
  // settings-view.ts: the "dsh-ee" page in DSH settings (address, test, update, sidebar toggle)
  settingsView: {
    title: "dsh-ee कनेक्शन",
    description: "कनेक्ट प्लगइन इस पते से एंटरप्राइज़ खाते में साइन इन करता है, एंटरप्राइज़ स्किल और माइक्रो-ऐप्स सिंक करता है, और एंटरप्राइज़ मॉडल कॉल करता है। कनेक्ट न हो तो पहले ब्राउज़र में dsh-ee वेबसाइट खोलें, फिर जो पता खुलता हो उसे यहाँ भरें।",
    reading: "पढ़ा जा रहा है…",
    addressLabel: "dsh-ee पता",
    packagedOverridden: "प्लगइन पैकेज में पता {{packaged}} है, आपने इसे बदलकर यह वर्तमान पता बना दिया है।",
    packagedDefault: "यह वह पता है जो प्रशासक ने कनेक्ट प्लगइन प्रकाशित करते समय लिखा था।",
    testing: "जाँच जारी है…",
    testConnection: "कनेक्शन जाँचें",
    resetAddress: "प्लगइन पैकेज का पता बहाल करें",
    plugin: "कनेक्ट प्लगइन",
    currentVersion: "वर्तमान संस्करण {{version}}",
    updating: "अपडेट हो रहा है…",
    updateLatest: "नवीनतम में अपडेट करें",
    updateHint: "एंटरप्राइज़ द्वारा प्रकाशित नवीनतम संस्करण में अपडेट करें; इंस्टॉल होने के बाद प्रभावी होने के लिए DSH पुनः आरंभ करना होगा।",
    changeHint: "पता बदलने का असर केवल इस मशीन पर होता है। यदि सभी कनेक्ट नहीं कर पा रहे हैं, तो प्रशासक को dsh-ee वेबसाइट की सिस्टम सेटिंग्स में “एक्सेस पता” भरने और कनेक्ट प्लगइन फिर से प्रकाशित करने के लिए कहें।",
    readFailed: "सेटिंग्स पढ़ने में विफल: {{error}}",
    testFailed: "जाँच विफल: {{error}}",
    saveFailed: "सहेजने में विफल: {{error}}",
    updateFailed: "अपडेट विफल: {{error}}",
    savedNotice: "सहेजा और लागू कर दिया गया। एंटरप्राइज़ स्किल, माइक्रो-ऐप्स और टूल नए पते पर सिंक हो रहे हैं; साइन इन अमान्य हो तो साइडबार के एंटरप्राइज़ पैनल से फिर से साइन इन करें।",
    sidebarHint: "साइडबार में केवल पहले 3 ऐप्स दिखाई देते हैं, बाकी “और देखें” में हैं; छिपाने के बाद भी होम स्क्रीन पर इनपुट बॉक्स के नीचे और दाईं ओर के एंटरप्राइज़ पैनल से एंटरप्राइज़ ऐप्स खोले जा सकते हैं।",
    saving: "सहेजा जा रहा है…"
  },
  // server-settings.ts: saving the dsh-ee address and testing the connection
  serverSettings: {
    reasonTimeout: "कनेक्शन का समय समाप्त: यह मशीन उस पते तक नहीं पहुँच सकती (आंतरिक पता, VPN कनेक्ट नहीं, या फ़ायरवॉल द्वारा अवरुद्ध)",
    reasonRefused: "कनेक्शन अस्वीकृत: पता मिल गया, लेकिन उस पोर्ट पर कोई सेवा नहीं है",
    reasonDns: "डोमेन नहीं मिला: वर्तनी जाँचें, या देखें कि VPN कनेक्ट करना आवश्यक है या नहीं",
    reasonTls: "HTTPS प्रमाणपत्र में समस्या ({{code}})",
    badAddress: "पते का प्रारूप गलत है",
    badAddressDetail: "पूरा http:// या https:// पता लिखें, जैसे https://ee.example.com, पथ के अलावा कोई पैरामीटर न जोड़ें",
    unreachable: "कनेक्ट नहीं हो सका",
    reachable: "कनेक्ट हो सका ({{ms}} ms)",
    notDshEe: "इस पते पर dsh-ee नहीं है",
    notDshEeDetail: "/healthz पर जाने से HTTP {{status}} लौटा; सुनिश्चित करें कि पता dsh-ee वेबसाइट का है",
    isDshEe: "यह dsh-ee है{{version}}",
    versionSuffix: " ({{version}})",
    noLogin: "इस मशीन पर अभी एंटरप्राइज़ खाते में साइन इन नहीं है",
    noLoginDetail: "पता सहेजने के बाद साइडबार के एंटरप्राइज़ पैनल में “साइन इन करें” पर क्लिक करें, या चैट में /ee-login टाइप करें",
    signedIn: "{{username}}{{department}} के रूप में साइन इन",
    departmentSuffix: " ({{department}})",
    loginInvalid: "इस मशीन का साइन इन इस dsh-ee पर मान्य नहीं है",
    loginInvalidDetail: "पता सहेजें और फिर से साइन इन करें (टोकन प्रति dsh-ee सर्वर जारी होता है, सर्वर बदलने पर फिर से साइन इन करना होगा)",
    loginCheckFailed: "साइन इन जाँच विफल",
    saveBadAddress: "पते का प्रारूप गलत है: पूरा http:// या https:// पता लिखें"
  },
  // workbench.ts: workbench data formatting, placeholders, form and board validation (pure functions)
  workbench: {
    unitHundredMillion: " सौ मिलियन",
    unitTenThousand: " दस हज़ार",
    unitBillion: " अरब",
    unitMillion: " मिलियन",
    flat: "स्थिर",
    allKbs: "सभी ज्ञान आधार",
    scopePicked: "ज्ञान आधार {{titles}} (आईडी {{ids}})",
    kbTitle: "“{{title}}”",
    formRequired: "“{{label}}” भरें",
    formNumber: "“{{label}}” में संख्या भरनी होगी",
    formSelect: "“{{label}}” विकल्पों में से चुनें",
    blocksRequired: "blocks एक खाली न होने वाली सूची होनी चाहिए",
    blocksTooMany: "बोर्ड में अधिकतम 20 ब्लॉक हो सकते हैं",
    boardTooLarge: "बोर्ड सामग्री 32 KB से अधिक नहीं हो सकती",
    blockWhere: "ब्लॉक {{index}}",
    textEmpty: "{{where}}: text खाली नहीं हो सकता",
    itemsRange: "{{where}}: items में 1–50 प्रविष्टियाँ होनी चाहिए",
    columnsRange: "{{where}}: columns में 1–12 कॉलम होने चाहिए",
    rowsRange: "{{where}}: rows अधिकतम 50 पंक्तियों वाली द्वि-आयामी सूची होनी चाहिए",
    citationsRange: "{{where}}: items में 1–30 उद्धरण होने चाहिए",
    citationAt: "{{where}}, उद्धरण {{index}}",
    citationNumbers: "{{at}}: doc, from और to धनात्मक पूर्णांक होने चाहिए, और to from से छोटा नहीं होना चाहिए",
    citationVersion: "{{at}}: version धनात्मक पूर्णांक होना चाहिए",
    citationQuote: "{{at}}: quote स्रोत से 1–500 अक्षरों का अंश होना चाहिए",
    blockType: "{{where}}: type केवल text, list, table या citations हो सकता है"
  },
  // workbench-host.ts: ee_workbench tool and panel data reads / row writes
  workbenchHost: {
    noComponent: "निर्देशिका में यह घटक नहीं है",
    notReadOnly: "यह डेटा स्रोत केवल-पढ़ने वाला टूल नहीं है, पैनल इसे डेटा लाने के लिए उपयोग नहीं कर सकता",
    formNoTable: "यह फ़ॉर्म तालिका में सबमिट करने के लिए कॉन्फ़िगर नहीं है",
    rowNoAction: "यह बटन तालिका नहीं बदल सकता",
    rowNoId: "इस पंक्ति में आईडी नहीं है",
    badDoc: "दस्तावेज़ आईडी गलत है",
    invalidAppName: "अमान्य माइक्रो-ऐप नाम",
    invalidSessionId: "अमान्य सत्र id",
    invalidChecklistKey: "अमान्य चेकलिस्ट आईडी",
    actionInvalid: "action केवल board या checklist हो सकता है",
    appNotFound: "माइक्रो-ऐप {{app}} नहीं है, या यह आपकी उपलब्ध सीमा में नहीं है",
    noBoard: "माइक्रो-ऐप {{app}} के वर्कबेंच में AI बोर्ड नहीं है",
    noBoardSession: "बोर्ड अपडेट करने वाला सत्र नहीं मिला",
    boardUpdated: "वर्कबेंच बोर्ड अपडेट कर दिया गया",
    badCitations: "उद्धरण {{numbers}} मूल से मेल नहीं खाते: कृपया kb__read से उन पंक्तियों को फिर से पढ़ें, सुधारें, बोर्ड फिर से लिखें, और बातचीत में उपयोगकर्ता को सुधार बताएँ।",
    staleCitations: "उद्धरण {{numbers}} के दस्तावेज़ अपडेट हो गए हैं, कृपया नवीनतम संस्करण पढ़कर पुष्टि करें कि निष्कर्ष नहीं बदले।",
    checklistUnsupported: "कनेक्ट प्लगइन का यह संस्करण चेकलिस्ट का समर्थन नहीं करता",
    noChecklist: "माइक्रो-ऐप {{app}} के वर्कबेंच में चेकलिस्ट नहीं है",
    checklistNotFound: "“{{title}}” शीर्षक वाली कोई चेकलिस्ट नहीं है; उपलब्ध: {{options}}",
    checklistAmbiguous: "{{count}} चेकलिस्ट हैं, कृपया title से एक निर्दिष्ट करें: {{options}}",
    checklistOption: "“{{title}}”",
    doneOrUndone: "done और undone में से कम-से-कम एक भरें",
    itemsNotFound: "चेकलिस्ट “{{title}}” में प्रविष्टियाँ {{unknown}} नहीं हैं; प्रविष्टि आईडी: {{known}}",
    noChecklistSession: "चेकलिस्ट अपडेट करने वाला सत्र नहीं मिला",
    checklistUpdated: "चेकलिस्ट “{{title}}” अपडेट हो गई: {{done}}/{{total}} पूर्ण",
    checklistMarked: "चेकलिस्ट “{{title}}” अपडेट हो गई: {{count}} प्रविष्टियाँ पूर्ण",
    description: "वर्तमान माइक्रो-ऐप वर्कबेंच अपडेट करें: action=board इस टर्न के परिणाम “AI बोर्ड” में लिखता है (हर बार पूरी तरह बदलता है, citations ब्लॉकों में ज्ञान आधार उद्धरण अपने आप सत्यापित होते हैं); action=checklist वर्कबेंच चेकलिस्ट की प्रविष्टियों पर चेक लगाता या हटाता है (done / undone में प्रविष्टि आईडी भरें)। कोई व्यावसायिक डेटा नहीं बदलता।",
    paramApp: "माइक्रो-ऐप नाम, यानी /नाम में दिया गया नाम",
    paramTitle: "checklist: चेकलिस्ट का शीर्षक; वर्कबेंच में केवल एक चेकलिस्ट हो तो खाली छोड़ सकते हैं",
    paramDone: "checklist: पूर्ण के रूप में चिह्नित करने वाली प्रविष्टियों की आईडी",
    paramUndone: "checklist: पूर्ण का चिह्न हटाने वाली प्रविष्टियों की आईडी",
    paramBlocks: "बोर्ड सामग्री, ऊपर से नीचे दिखाई जाती है; केवल पाठ के रूप में प्रदर्शित होती है, HTML समर्थित नहीं है",
    paramItems: "list: प्रत्येक प्रविष्टि के लिए एक पाठ; citations: प्रत्येक प्रविष्टि के लिए एक ज्ञान आधार उद्धरण {doc, version, from, to, quote, title}"
  },
  // workbench-view.ts: business workbench panel (metrics / tables / charts / checklists / forms / board / source)
  workbenchView: {
    widgetMetric: "मीट्रिक",
    widgetTable: "तालिका",
    widgetChart: "चार्ट",
    widgetChecklist: "चेकलिस्ट",
    widgetForm: "फ़ॉर्म",
    widgetActions: "त्वरित बटन",
    widgetMarkdown: "विवरण",
    widgetBoard: "AI बोर्ड",
    widgetKbScope: "किन ज्ञान आधारों में खोजें",
    confirmDelete: "हटाने की पुष्टि करें",
    forbidden: "अनुमति नहीं: आपके पास इस डेटा स्रोत के उपयोग की अनुमति नहीं है",
    fetchFailed: "डेटा लाने में विफल: {{error}}",
    forbiddenDetail: "अनुमति नहीं: आपके पास “{{connection}}” में “{{tool}}” के उपयोग की अभी अनुमति नहीं है।",
    thisDataSource: "इस डेटा स्रोत",
    requestAccess: "अनुमति के लिए अनुरोध करें ↗",
    contactAdmin: "कृपया प्रशासक से संपर्क करके पहुँच प्राप्त करें।",
    countRows: "{{total}} पंक्तियाँ",
    expandDetail: "विवरण विस्तारित करें",
    collapseDetail: "विवरण संक्षिप्त करें",
    truncatedRows: "कुल {{total}} पंक्तियाँ, पहली {{shown}} पंक्तियाँ दिखाई जा रही हैं",
    pointTip: "{{label}} · {{series}}: {{value}}",
    pieTip: "{{label}}: {{value}} ({{percent}}%)",
    selectPlaceholder: "चुनें",
    citationOk: "✔ मूल से सत्यापित",
    citationStale: "मूल अपडेट हो गया है",
    citationMismatch: "✖ मूल से मेल नहीं खाता",
    citationMissing: "✖ यह दस्तावेज़ नहीं मिला",
    citationUnchecked: "सत्यापित नहीं",
    citationWhere: "{{title}} · पंक्ति {{range}}",
    citationTip: "{{where}}\n“{{quote}}”",
    webSourceTip: "{{tip}}\ndsh-ee वेबसाइट पर मूल देखें",
    quoteText: "“{{quote}}”",
    viewSource: "मूल देखें: {{where}}",
    nearSkippedTip: "वह पृष्ठ स्कैन किया गया या विकृत है, ज्ञान आधार में शामिल नहीं है; उत्तर में उस पृष्ठ की सामग्री छूट सकती है, कृपया मूल दस्तावेज़ देखें",
    nearSkipped: "⚠ पृष्ठ {{page}} शामिल नहीं",
    noKbs: "आपके पास अभी खोजने के लिए कोई ज्ञान आधार नहीं है",
    kbDocs: "{{count}} दस्तावेज़",
    scopeOnly: "केवल चयनित {{count}} ज्ञान आधारों में खोजें",
    scopeAllHint: "कुछ भी चयनित न हो तो सभी में खोजा जाता है",
    boardEmpty: "AI काम पूरा करने पर परिणाम यहाँ लिखेगा।",
    updatedAt: "{{time}} पर अपडेट",
    unavailable: "यह वर्कबेंच अब उपलब्ध नहीं है: ऐप अक्षम हो सकता है, या वर्कबेंच कॉन्फ़िगरेशन बदल गया है। आप यह टैब बंद कर सकते हैं।",
    openCitationHint: "“उत्तर और स्रोत” में उद्धरण पर क्लिक करने से मूल यहाँ खुलेगा।",
    versionLine: "संस्करण {{version}} · पंक्तियाँ {{from}}–{{to}} / कुल {{total}}",
    staleNotice: "{{stale}}; हाइलाइट उद्धरण के समय का संस्करण दिखाता है।",
    prevPage: "पिछला पृष्ठ",
    nextPage: "अगला पृष्ठ",
    readView: "पठन दृश्य",
    rawView: "मूल देखें (पंक्ति संख्याओं के साथ)",
    quotedSource: "उद्धृत मूल: “{{quote}}”"
  },
  // local-mcp.ts: /ee-local command and local extension MCP registration
  localMcp: {
    missingPackage: "npm पैकेज नाम आवश्यक है, जैसे /ee-local add @acme/legacy-oa-local@0.1.0",
    notNpmPackage: "“{{text}}” npm पैकेज नाम नहीं है: स्थानीय एक्सटेंशन केवल एंटरप्राइज़ npm रजिस्ट्री से इंस्टॉल होते हैं, URL, स्थानीय पथ या git पता स्वीकार नहीं किए जाते",
    invalidPackage: "“{{pkg}}” मान्य npm पैकेज नाम नहीं है (@scope/name या name जैसा होना चाहिए)",
    invalidVersion: "“{{version}}” मान्य संस्करण नहीं है",
    unknownSubcommand: "उप-कमांड केवल list / add / remove / snippet हो सकते हैं; प्राप्त हुआ “{{action}}”",
    idHint: "--id के बाद एक्सटेंशन id होना चाहिए, जैसे acme/legacy-oa",
    invalidId: "“{{value}}” मान्य एक्सटेंशन id नहीं है (acme/legacy-oa जैसा होना चाहिए)",
    unknownFlag: "अज्ञात पैरामीटर “{{token}}”",
    missingTarget: "{{action}} के बाद पैकेज नाम या एक्सटेंशन id होना चाहिए",
    extraArgs: "अतिरिक्त पैरामीटर “{{args}}” दिए गए हैं",
    duplicateService: "{{name}} नाम की MCP सेवा पहले से है, उसे ओवरराइट नहीं किया जाएगा",
    noManagerHint: "\nइस DSH में MCP प्रबंधन API (@deepseek-ai/dsh-mcp-manager) नहीं है, इसलिए स्वतः पंजीकरण संभव नहीं; कॉन्फ़िग स्निपेट बनाने के लिए /ee-local snippet <पैकेज> चलाएँ।\n",
    noExtensions: "अभी कोई स्थानीय एक्सटेंशन नहीं जोड़ा गया।\nउपयोग: /ee-local add <पैकेज>[@संस्करण] [--id लेखक/नाम]\n{{hint}}",
    registered: "पंजीकृत",
    unregistered: "पंजीकृत नहीं",
    unregisteredNoManager: "पंजीकृत नहीं (MCP प्रबंधन API नहीं है)",
    listHeader: "स्थानीय एक्सटेंशन ({{count}}):\n{{lines}}\n{{hint}}\nहटाएँ: /ee-local remove <एक्सटेंशन id या पैकेज नाम>",
    alreadyAdded: "{{spec}} पहले ही जोड़ा जा चुका है; id या संस्करण बदलने के लिए पहले /ee-local remove चलाएँ।",
    idTaken: "एक्सटेंशन id {{id}} पहले से {{pkg}} पर उपयोग में है; पहले /ee-local remove {{id}} चलाएँ, या दूसरा --id चुनें।",
    packageBound: "यह पैकेज एक्सटेंशन id {{id}} के अंतर्गत पहले ही जोड़ा जा चुका है; id बदलने के लिए पहले /ee-local remove {{id}} चलाएँ।",
    noManagerAdd: "इस DSH में MCP प्रबंधन API नहीं है (@deepseek-ai/dsh-mcp-manager माउंट नहीं है, पुराने DSH संस्करणों या न्यूनतम संयोजनों में यह नहीं होता), इसलिए स्वतः पंजीकरण संभव नहीं।\nनीचे दिया स्निपेट profile की cordis.patch.yml में जोड़ें (या DSH को 0.2.2 में अपग्रेड करके फिर प्रयास करें):\n\n{{snippet}}\n",
    serviceConflict: "यहाँ पहले से {{name}} नाम की MCP सेवा है (कर्मचारी द्वारा कॉन्फ़िगर, किसी अन्य प्लगइन द्वारा पंजीकृत, या पहले जोड़ी गई), उसे ओवरराइट नहीं किया जाएगा।\nदूसरा --id लेने पर दूसरा सेवा नाम मिलेगा, जैसे: /ee-local add {{spec}} --id {{scope}}/<दूसरा नाम>\n",
    confirmPlan: "{{update}}स्थानीय एक्सटेंशन को स्थानीय MCP सेवा के रूप में पंजीकृत करने की तैयारी (केवल इस मशीन पर लिखता है, dsh-ee नहीं बदलता, मौजूदा सेवाओं को ओवरराइट नहीं करता):\n  एक्सटेंशन  {{id}}\n  पैकेज      {{pkg}}{{latest}}\n  MCP सेवा  {{server}} (टूल नाम mcp__{{server}}__<टूल>)\n  env        DSH_EE_URL, DSH_EE_TOKEN (DSH क्रेडेंशियल स्टोर से), DSH_EE_HARNESS=dsh, एंटरप्राइज़ npm रजिस्ट्री\n\nपुष्टि के बाद चलाएँ: /ee-local add {{spec}}{{idFlag}} --yes\n",
    updateSuffix: "नए संस्करण में अपडेट करने के बाद ",
    latestVersion: " (नवीनतम संस्करण)",
    registerFailed: "पंजीकरण विफल: {{error}}",
    registeredText: "पंजीकृत (अगली बार DSH शुरू होने पर अपने आप बहाल हो जाएगा):\n  {{id}} → {{spec}}, सेवा नाम {{server}}\nटूल नाम mcp__{{server}}__ से शुरू होते हैं; पहली बार कॉल करने पर यदि एक्सटेंशन साइन इन माँगता है तो सिस्टम की अपनी साइन इन विंडो खुलेगी।\nहटाएँ: /ee-local remove {{id}}\n",
    notFound: "“{{target}}” नहीं मिला; जोड़े गए एक्सटेंशन देखने के लिए /ee-local list चलाएँ।",
    removed: "{{id}} ({{pkg}}) हटा दिया गया। MCP सेवा {{server}} अपंजीकृत कर दी गई।",
    snippetIntro: "नीचे दिया स्निपेट संबंधित profile की cordis.patch.yml के अंत में जोड़ें (DSH में mcpServers कॉन्फ़िग फ़ाइल नहीं होती, यही आधिकारिक तरीका है):\n\n{{snippet}}\n\n{{hint}}\n",
    snippetHint: "इस DSH में MCP प्रबंधन API है, /ee-local add से स्वतः पंजीकरण भी किया जा सकता है।",
    description: "स्थानीय एक्सटेंशन: स्थानीय एक्सटेंशन की MCP सेवाएँ पंजीकृत / देखें / हटाएँ (केवल इस मशीन पर लिखता है)",
    inputHint: "[list|add|remove|snippet] [पैकेज[@संस्करण]] [--id लेखक/नाम] [--yes]"
  },
  // index.ts (host side): fallback errors for remote methods
  serverIndex: {
    microAppNotFound: "निर्देशिका में यह माइक्रो-ऐप नहीं है",
    notLoggedIn: "एंटरप्राइज़ खाते में साइन इन नहीं है",
    feedbackFailed: "फ़ीडबैक विफल: HTTP {{status}}",
    invalidEntryId: "अमान्य प्रविष्टि आईडी"
  },
  // kb-doc-render.ts: knowledge base source reading view (pagination tabs)
  kbDocRender: {
    page: "पृष्ठ {{page}}"
  },
  // bridge.ts: MCP tool result text
  bridge: {
    error: "त्रुटि: {{text}}"
  },
  // brand-view.ts: initial-letter fallback when there is no logo
  brandView: {
    fallbackLetter: "ए"
  }
};
var hi_default = hi;

// src/i18n/langs.ts
var DEFAULT_LANG = "zh-CN";
var PRIMARY_LANG = { zh: "zh-CN", en: "en", es: "es", pt: "pt", hi: "hi" };
function normalizeLang(tag) {
  if (typeof tag !== "string") return void 0;
  const primary = tag.trim().toLowerCase().replace(/_/g, "-").split("-")[0];
  return primary ? PRIMARY_LANG[primary] : void 0;
}
function detectLang(sources) {
  for (const source of [sources.host, sources.browser, sources.env]) {
    if (source === void 0) continue;
    for (const tag of typeof source === "string" ? [source] : source) {
      const lang = normalizeLang(tag);
      if (lang) return lang;
    }
  }
  return DEFAULT_LANG;
}
function hostLocaleId(ctx) {
  try {
    const service = typeof ctx?.get === "function" ? ctx.get("locale") : void 0;
    const snapshot = typeof service?.getSnapshot === "function" ? service.getSnapshot() : typeof service?.getLocale === "function" ? service.getLocale() : void 0;
    return typeof snapshot?.active === "string" ? snapshot.active : void 0;
  } catch {
    return void 0;
  }
}
function browserLanguages() {
  if (typeof window === "undefined") return [];
  try {
    const nav = globalThis.navigator;
    const list = [...Array.isArray(nav?.languages) ? nav.languages : [], nav?.language];
    return list.filter((tag) => typeof tag === "string" && tag !== "");
  } catch {
    return [];
  }
}
function envLanguages(env = typeof process === "undefined" ? void 0 : process.env) {
  if (!env) return [];
  return [env.DSH_EE_LANG, env.LC_ALL, env.LC_MESSAGES, env.LANG];
}

// src/i18n/index.ts
var TABLES = { "zh-CN": zh_default, en: en_default, es: es_default, pt: pt_default, hi: hi_default };
var active;
var listeners = /* @__PURE__ */ new Set();
function t(key, params) {
  const template = lookup(getLang(), key) ?? lookup("en", key) ?? key;
  if (!params) return template;
  return template.replace(/\{\{(\w+)\}\}/g, (match, name2) => {
    const value = params[name2];
    return value === void 0 || value === null ? match : String(value);
  });
}
function getLang() {
  if (active === void 0) active = detectLang({ browser: browserLanguages(), env: envLanguages() });
  return active;
}
function setLang(lang) {
  const next = normalizeLang(lang) ?? DEFAULT_LANG;
  if (next === getLang()) return next;
  active = next;
  for (const listener of [...listeners]) {
    try {
      listener();
    } catch {
    }
  }
  return next;
}
function initI18n(ctx) {
  active = detectLang({ host: hostLocaleId(ctx), browser: browserLanguages(), env: envLanguages() });
  watchHostLang(ctx);
  return active;
}
function watchHostLang(ctx) {
  try {
    const service = typeof ctx?.get === "function" ? ctx.get("locale") : void 0;
    if (typeof service?.subscribe !== "function") return;
    const sync = () => {
      const lang = normalizeLang(hostLocaleId(ctx));
      if (lang) setLang(lang);
    };
    if (typeof ctx?.effect === "function") {
      ctx.effect(() => {
        const off = service.subscribe(sync);
        sync();
        return () => {
          try {
            off?.();
          } catch {
          }
        };
      });
    } else {
      sync();
    }
  } catch {
  }
}
function lookup(lang, key) {
  let node = TABLES[lang];
  for (const part of key.split(".")) {
    if (node === null || typeof node !== "object") return void 0;
    node = node[part];
  }
  return typeof node === "string" ? node : void 0;
}

// src/config.ts
function normalizeConfig(input) {
  const base = String(input.base ?? "").replace(/\/+$/, "");
  if (!/^https?:\/\//.test(base)) throw new Error(t("config.badBase"));
  return {
    base,
    org: input.org || t("config.defaultOrg"),
    serverName: input.serverName || "ee",
    tokenRef: input.tokenRef || "DSHEE_TOKEN",
    autoInstall: input.autoInstall !== false,
    version: input.version || "dev",
    scopes: Array.isArray(input.scopes) ? input.scopes.filter((s) => typeof s === "string" && s.startsWith("@")) : []
  };
}
function dshHome() {
  return process.env.DSH_HOME || path.join(os.homedir(), ".dsh");
}
function npmrcPath(ctx) {
  const dir = ctx.get("profileContext")?.dir;
  if (typeof dir === "string" && dir) return path.join(dir, ".npmrc");
  return process.env.NPM_CONFIG_USERCONFIG || path.join(os.homedir(), ".npmrc");
}
function npmrcEntries(config, token) {
  const registry = `${config.base}/npm/`;
  const entries = /* @__PURE__ */ new Map();
  for (const scope of config.scopes) entries.set(`${scope}:registry`, registry);
  entries.set(`${registry.replace(/^https?:/, "")}:_authToken`, token);
  return entries;
}

// src/token.ts
import fs from "node:fs";
import path2 from "node:path";
function restrictCredentialFile(file, platform = process.platform) {
  if (platform === "win32") return false;
  try {
    fs.chmodSync(file, 384);
    return true;
  } catch {
    return false;
  }
}
function npmrcState(ctx, config, token) {
  const file = npmrcPath(ctx);
  if (!fs.existsSync(file)) return "missing";
  const lines = new Map(fs.readFileSync(file, "utf8").split(/\r?\n/).filter((l) => l.includes("=")).map((l) => [l.slice(0, l.indexOf("=")), l.slice(l.indexOf("=") + 1)]));
  let state = "ok";
  for (const [key, value] of npmrcEntries(config, token)) {
    const have = lines.get(key);
    if (have === void 0) return "missing";
    if (have !== value) state = "mismatch";
  }
  return state;
}
var TokenManager = class {
  constructor(ctx, config) {
    this.ctx = ctx;
    this.config = config;
  }
  tokenPath() {
    return path2.join(dshHome(), "dsh-ee", "token");
  }
  envPath() {
    return path2.join(dshHome(), ".env");
  }
  async resolve() {
    let stored;
    try {
      stored = await this.ctx.get("credentials")?.resolve?.(this.config.tokenRef);
    } catch {
      stored = void 0;
    }
    if (stored?.value) return stored;
    try {
      const value = fs.readFileSync(this.tokenPath(), "utf8").trim();
      if (value) return { value, source: "file" };
    } catch {
    }
    return void 0;
  }
  async set(value) {
    const credentials = this.ctx.get("credentials");
    if (typeof credentials?.set === "function") {
      try {
        await credentials.set(this.config.tokenRef, value);
        fs.rmSync(this.tokenPath(), { force: true });
        return;
      } catch {
      }
    }
    const file = this.tokenPath();
    fs.mkdirSync(path2.dirname(file), { recursive: true, mode: 448 });
    fs.writeFileSync(file, `${value}
`, { mode: 384 });
    restrictCredentialFile(file);
    this.writeEnvToken(value);
  }
  async unset(expected) {
    fs.rmSync(this.tokenPath(), { force: true });
    try {
      await this.ctx.get("credentials")?.unset(this.config.tokenRef);
    } catch {
    }
    this.stripEnvToken(expected);
  }
  writeEnvToken(value) {
    const file = this.envPath();
    const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
    const kept = before.split(/\r?\n/).filter((line) => line.trim() && !line.startsWith(`${this.config.tokenRef}=`));
    const after = `${[...kept, `${this.config.tokenRef}=${value}`].join("\n")}
`;
    if (after === before) {
      restrictCredentialFile(file);
      return;
    }
    fs.mkdirSync(path2.dirname(file), { recursive: true, mode: 448 });
    fs.writeFileSync(file, after, { mode: 384 });
    restrictCredentialFile(file);
  }
  stripEnvToken(expected) {
    const file = this.envPath();
    if (!fs.existsSync(file)) return;
    const prefix = `${this.config.tokenRef}=`;
    const lines = fs.readFileSync(file, "utf8").split(/\r?\n/);
    const hit = lines.find((line) => line.startsWith(prefix));
    if (!hit) return;
    if (expected !== void 0 && hit.slice(prefix.length) !== expected) return;
    fs.writeFileSync(file, lines.filter((line) => line !== hit).join("\n"), { mode: 384 });
    restrictCredentialFile(file);
  }
  pendingPath() {
    return path2.join(dshHome(), "dsh-ee", "token.pending");
  }
  async importPending() {
    const file = this.pendingPath();
    if (!fs.existsSync(file)) return false;
    const token = fs.readFileSync(file, "utf8").trim();
    if (!token) {
      fs.unlinkSync(file);
      return false;
    }
    await this.set(token);
    fs.unlinkSync(file);
    return true;
  }
  async invalidate(failedToken) {
    const current = await this.resolve();
    if (current?.value !== failedToken) return false;
    await this.unset(failedToken);
    return true;
  }
  /** 把企业作用域与令牌写进 profile 的 .npmrc，其它行原样保留；内容没变就不写。返回是否改动了文件。 */
  async syncNpmrc(token) {
    const file = npmrcPath(this.ctx);
    const entries = npmrcEntries(this.config, token);
    const before = fs.existsSync(file) ? fs.readFileSync(file, "utf8") : "";
    const kept = before.split(/\r?\n/).filter((line) => line && !entries.has(line.split("=", 1)[0]));
    const after = [...kept, ...[...entries].map(([key, value]) => `${key}=${value}`)].join("\n") + "\n";
    if (after === before) {
      restrictCredentialFile(file);
      return false;
    }
    fs.mkdirSync(path2.dirname(file), { recursive: true, mode: 448 });
    fs.writeFileSync(file, after, { mode: 384 });
    restrictCredentialFile(file);
    return true;
  }
  async npmrcMatches(token) {
    return npmrcState(this.ctx, this.config, token) === "ok";
  }
};

// src/http.ts
function dshHost(versions = process.versions) {
  const electron = versions.electron;
  if (!electron) return "cli";
  return `desktop/${process.env.DSH_DESKTOP_VERSION || process.env.DSH_VERSION || electron}`;
}
var HttpClient = class {
  constructor(tokens) {
    this.tokens = tokens;
  }
  async fetch(path9, init = {}, timeoutMs = 3e4) {
    const current = await this.tokens.resolve();
    const headers = new Headers(init.headers);
    if (current?.value) headers.set("Authorization", `Bearer ${current.value}`);
    const config = this.tokens.config;
    if (!headers.has("X-DshEE-Client")) headers.set("X-DshEE-Client", `dsh-connect/${config.version || "dev"}`);
    if (!headers.has("X-DshEE-Host")) headers.set("X-DshEE-Host", dshHost());
    const timeout = timeoutMs > 0 ? AbortSignal.timeout(timeoutMs) : void 0;
    const signal = init.signal && timeout ? AbortSignal.any([init.signal, timeout]) : timeout ?? init.signal ?? void 0;
    const response = await fetch(`${config.base}${path9}`, { ...init, headers, signal });
    if (response.status === 401 && current?.value) {
      const body = await response.clone().json().catch(() => null);
      if (body?.code?.startsWith("token_")) await this.tokens.invalidate(current.value);
    }
    return response;
  }
  async json(path9, init) {
    const response = await this.fetch(path9, init);
    if (!response.ok) throw new Error(`HTTP ${response.status}`);
    return response.json();
  }
};

// src/device.ts
import { execFile } from "node:child_process";
import os2 from "node:os";
var DeviceFlow = class {
  constructor(config, fetchImpl = fetch) {
    this.config = config;
    this.fetchImpl = fetchImpl;
  }
  /** setupCode：网页「开始使用」页给的接入码，带上后这次登录会出现在那个页面上，可以就地确认。 */
  async start(setupCode = "") {
    const response = await this.fetchImpl(`${this.config.base}/api/device/code`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      signal: AbortSignal.timeout(3e4),
      body: JSON.stringify({ client_name: os2.hostname(), client_os: `${process.platform} ${os2.release()}`, client_version: this.config.version, ...setupCode ? { setup_code: setupCode } : {} })
    });
    if (!response.ok) {
      const body = await response.json().catch(() => null);
      throw new Error(body?.error ? t("device.startFailed", { error: body.error }) : t("device.startHttpFailed", { status: response.status }));
    }
    return response.json();
  }
  async wait(deviceCode, intervalSeconds = 5, timeoutMs = 6e5, signal) {
    const deadline = Date.now() + timeoutMs;
    let interval = Math.max(1, intervalSeconds) * 1e3;
    while (Date.now() < deadline) {
      if (signal?.aborted) throw new Error(t("device.cancelled"));
      const response = await this.fetchImpl(`${this.config.base}/api/device/token`, {
        method: "POST",
        headers: { "content-type": "application/json" },
        body: JSON.stringify({ device_code: deviceCode }),
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(3e4)]) : AbortSignal.timeout(3e4)
      });
      const body = await response.json();
      if (body.access_token) return body;
      if (body.error === "slow_down" || body.error === "rate_limited") {
        interval += 5e3;
      } else if (body.error !== "authorization_pending") {
        throw new Error(body.error || t("device.failed", { status: response.status }));
      }
      let onAbort;
      try {
        await new Promise((resolve, reject) => {
          const timer = setTimeout(resolve, interval);
          onAbort = () => {
            clearTimeout(timer);
            reject(new Error(t("device.cancelled")));
          };
          signal?.addEventListener("abort", onAbort, { once: true });
        });
      } finally {
        if (onAbort) signal?.removeEventListener("abort", onAbort);
      }
    }
    throw new Error(t("device.timeout"));
  }
};
function openBrowser(url) {
  if (process.platform === "win32") execFile("cmd", ["/c", "start", "", url], () => {
  });
  else execFile(process.platform === "darwin" ? "open" : "xdg-open", [url], () => {
  });
}

// src/commands.ts
function parseSetupCode(rawInput) {
  const code = String(rawInput ?? "").trim().toUpperCase();
  return /^[A-Z0-9]{4}-?[A-Z0-9]{4}$/.test(code) ? code : "";
}
function registerCommands(ctx, config, tokens, onLogin = async () => {
}) {
  const flow = new DeviceFlow(config);
  let pending = null;
  let starting = false;
  const login = async (rawInput) => {
    if (pending) return { kind: "success", text: t("commands.pending", { code: pending.code, url: pending.url }) };
    if (starting) return { kind: "success", text: t("commands.starting") };
    const setupCode = parseSetupCode(rawInput);
    if (String(rawInput ?? "").trim() && !setupCode) return { kind: "error", text: t("commands.badSetupCode") };
    starting = true;
    let started;
    try {
      started = await flow.start(setupCode);
    } finally {
      starting = false;
    }
    const url = started.verification_uri_complete;
    if (!setupCode) openBrowser(url);
    const task = (async () => {
      try {
        const result = await flow.wait(started.device_code, started.interval);
        await tokens.set(result.access_token);
        await tokens.syncNpmrc(result.access_token);
        await onLogin().catch(() => void 0);
      } finally {
        pending = null;
      }
    })();
    pending = { code: started.user_code, url, task };
    void task.catch(() => void 0);
    if (setupCode) return { kind: "success", text: t("commands.setupConfirm", { code: started.user_code, url }) };
    return { kind: "success", text: t("commands.pending", { code: started.user_code, url }) };
  };
  const commands = ctx.get("commands");
  if (commands) {
    const register = (definition) => {
      const dispose = commands.register(definition);
      ctx.effect(() => dispose);
    };
    register({
      name: "ee-login",
      description: t("commands.loginDescription"),
      input: { hint: t("commands.setupCodeHint") },
      handler: (invocation) => login(invocation?.rawInput)
    });
    register({
      name: "ee-status",
      description: t("commands.statusDescription"),
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: t("commands.notLoggedIn") };
        let response;
        try {
          response = await fetch(`${config.base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(3e4) });
        } catch {
          return { kind: "error", text: t("commands.unreachable") };
        }
        if (!response.ok) {
          if (response.status === 401) return { kind: "success", text: t("commands.sessionExpired") };
          return { kind: "error", text: t("commands.httpError", { status: response.status }) };
        }
        const me = await response.json();
        const matches = await tokens.npmrcMatches(current.value);
        const expiry = me.expires_at ? t("commands.expiresOn", { date: new Date(me.expires_at).toLocaleDateString() }) : t("commands.neverExpires");
        return { kind: "success", text: t("commands.statusLine", { username: me.username, department: me.department, token: me.token_prefix, expiry, version: config.version, npmrc: matches ? t("commands.npmrcMatch") : t("commands.npmrcMismatch") }) };
      }
    });
    register({
      name: "ee-logout",
      description: t("commands.logoutDescription"),
      handler: async () => {
        const current = await tokens.resolve();
        if (!current) return { kind: "success", text: t("commands.noAccount") };
        const revoked = await fetch(`${config.base}/api/dsh/logout`, { method: "POST", headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(3e4) }).then((response) => response.ok, () => false);
        await tokens.unset(current.value);
        const left = await tokens.resolve();
        if (left?.value === current.value) {
          return {
            kind: "error",
            text: `${revoked ? t("commands.revokedPrefix") : ""}${t("commands.envToken", { ref: config.tokenRef })}`
          };
        }
        return { kind: "success", text: revoked ? t("commands.loggedOut") : t("commands.loggedOutOffline") };
      }
    });
  }
  return { login };
}

// src/plugins.ts
var installing = false;
async function installRequired(ctx, config, required) {
  if (!config.autoInstall || installing || required.length === 0) return;
  const manager = ctx.get("pluginManager");
  if (!manager) return;
  installing = true;
  try {
    const have = new Set((await manager.listBundles()).filter((b) => b.installed || b.enabled).map((b) => b.name));
    for (const plugin of required) {
      if (have.has(plugin.name)) continue;
      await manager.installBundle(plugin.name, { registry: `${config.base}/npm/` });
    }
  } finally {
    installing = false;
  }
}

// src/skills.ts
function centerData(catalog, base) {
  const detail = (id) => id ? `${base}/assets/${id}` : void 0;
  const withUrls = (app) => ({ ...app, web_url: appWebUrl(base, app), detail_url: detail(app.asset_id) });
  const experts = (catalog?.apps ?? []).filter((app) => app.form === "expert").map(withUrls);
  const availableExperts = (catalog?.available_apps ?? []).filter((app) => app.form === "expert").map(withUrls);
  const skill = (enabled) => (k) => ({
    name: k.name,
    title: k.title,
    kind: k.kind,
    description: k.description,
    asset_id: k.asset_id,
    delivery: k.delivery,
    category_id: k.category_id,
    featured: !!k.featured,
    enabled,
    detail_url: detail(k.asset_id)
  });
  const isSkill = (k) => (k?.kind === "skill" || k?.kind === "prompt") && k?.asset_id;
  const skills = [
    ...(catalog?.skills ?? []).filter(isSkill).map(skill(true)),
    ...(catalog?.available_skills ?? []).filter(isSkill).map(skill(false))
  ];
  const connections = (catalog?.connections ?? []).map((c) => ({ ...c, featured: !!c.featured, detail_url: detail(c.asset_id) }));
  return {
    experts,
    availableExperts,
    skills,
    connections,
    categories: Array.isArray(catalog?.categories) ? catalog.categories : [],
    legacy: !Array.isArray(catalog?.connections)
  };
}
async function registerSkills(ctx, config, http) {
  let catalog = { skills: [], required_plugins: [] };
  let etag = "";
  const refresh = async () => {
    try {
      const response = await http.fetch("/api/dsh/catalog", { headers: etag ? { "If-None-Match": etag } : {} });
      if (response.status === 304) return false;
      if (!response.ok) return false;
      catalog = await response.json();
      etag = response.headers.get("etag") ?? "loaded";
      return true;
    } catch {
      return false;
    }
  };
  const install = () => installRequired(ctx, config, catalog.required_plugins).catch(() => void 0);
  void (async () => {
    await refresh();
    await install();
  })();
  let invalidate = () => {
  };
  ctx.skills.registerProvider((control) => {
    invalidate = () => control.invalidate();
    let timer;
    const pollSeconds = Math.max(15, Number(config.pollSeconds) || 60) * 1e3;
    const tick = async () => {
      try {
        if (await refresh()) {
          control.invalidate();
          await install();
        }
      } finally {
        if (!control.signal.aborted) timer = setTimeout(tick, pollSeconds);
      }
    };
    timer = setTimeout(tick, 1e3);
    control.signal.addEventListener("abort", () => timer && clearTimeout(timer));
    return {
      name: "dsh-ee",
      async list() {
        if (!etag) await refresh();
        return catalog.skills.map((skill) => ({
          name: skill.name,
          description: skill.description || skill.title,
          invocation: { modelInvocable: !!skill.model_invocable, userInvocable: !!skill.user_invocable },
          provider: "dsh-ee",
          source: "dsh-ee",
          rank: 350,
          locator: skill.name,
          metadata: { title: skill.title, kind: skill.kind, version: skill.version },
          ...skill.when_to_use ? { whenToUse: skill.when_to_use } : {}
        }));
      },
      async get(candidate, options) {
        const response = await http.fetch(`/api/dsh/skills/${encodeURIComponent(String(candidate.locator))}`, { signal: options.signal }).catch(() => void 0);
        if (!response) return void 0;
        if (!response.ok) return void 0;
        const body = await response.json();
        const { rank: _rank, locator: _locator, ...summary } = candidate;
        return { ...summary, content: body.content };
      }
    };
  });
  return {
    readBrand: async () => {
      await refresh();
      return catalog.brand ?? { enabled: true, name: config.org, logo_url: "", revision: "" };
    },
    readApps: async () => {
      await refresh();
      return (catalog.apps ?? []).map((app) => ({ ...app, web_url: appWebUrl(config.base, app), detail_url: app.asset_id ? `${config.base}/assets/${app.asset_id}` : void 0 }));
    },
    readAvailableApps: async () => {
      await refresh();
      return (catalog.available_apps ?? []).map((app) => ({ ...app, web_url: appWebUrl(config.base, app), detail_url: app.asset_id ? `${config.base}/assets/${app.asset_id}` : void 0 }));
    },
    refreshNow: async () => {
      if (await refresh()) {
        invalidate();
        await install();
      }
    },
    readCenter: async () => {
      await refresh();
      return centerData(catalog, config.base);
    }
  };
}
function appWebUrl(base, app) {
  if (app.web) return `${base}/a/${encodeURIComponent(app.name)}`;
  return app.asset_id ? `${base}/assets/${app.asset_id}` : void 0;
}

// src/write-log.ts
import fs2 from "node:fs";
import path3 from "node:path";
import { createHash } from "node:crypto";
var MAX_PER_SESSION = 200;
function writeOf(tool, args) {
  if (!tool || tool.annotations?.readOnlyHint === true) return void 0;
  const meta = tool._meta?.dshee ?? {};
  const actions = meta.actions;
  if (actions && typeof actions === "object") {
    const action = actions[String(args?.action)];
    if (!action) return void 0;
    if (action.e === "danger" && !args?.confirm) return void 0;
    return { label: String(action.zh || args?.action) };
  }
  const name2 = String(tool.name ?? "");
  const short = name2.includes("__") ? name2.slice(name2.indexOf("__") + 2) : name2;
  const asset = Number(meta.asset);
  return {
    label: short,
    ...typeof meta.system === "string" && meta.system ? { system: meta.system } : systemOf(tool.description),
    ...Number.isInteger(asset) && asset > 0 ? { asset } : {}
  };
}
function systemOf(description) {
  const match = /^【([^】]{1,80})】/.exec(String(description ?? ""));
  return match ? { system: match[1] } : {};
}
function resultOk(result) {
  return !(result && typeof result === "object" && result.isError === true);
}
function writesBetween(records, from, to) {
  return records.filter((item) => item.at >= from && (to === void 0 || item.at <= to));
}
var safeName = /^[A-Za-z0-9_-]{1,80}$/;
var WriteLog = class {
  constructor(dir = path3.join(dshHome(), "dsh-ee", "writes")) {
    this.dir = dir;
  }
  sequence = 0;
  file(sessionId) {
    if (typeof sessionId !== "string" || !sessionId) throw new Error(t("writeLog.invalidSessionId"));
    const name2 = safeName.test(sessionId) ? sessionId : createHash("sha256").update(sessionId).digest("hex").slice(0, 32);
    return path3.join(this.dir, `${name2}.json`);
  }
  read(sessionId) {
    try {
      const value = JSON.parse(fs2.readFileSync(this.file(sessionId), "utf8"));
      if (value?.sessionId !== sessionId || !Array.isArray(value?.writes)) return [];
      return value.writes;
    } catch {
      return [];
    }
  }
  record(sessionId, item) {
    const writes = [...this.read(sessionId), item].slice(-MAX_PER_SESSION);
    const file = this.file(sessionId);
    fs2.mkdirSync(this.dir, { recursive: true, mode: 448 });
    const temp = `${file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs2.writeFileSync(temp, JSON.stringify({ sessionId, writes }) + "\n", { mode: 384 });
      fs2.renameSync(temp, file);
    } finally {
      if (fs2.existsSync(temp)) fs2.unlinkSync(temp);
    }
  }
  between(sessionId, from, to) {
    if (typeof sessionId !== "string" || !sessionId || !Number.isFinite(from)) return [];
    return writesBetween(this.read(sessionId), from, Number.isFinite(to) ? to : void 0);
  }
};

// src/bridge.ts
function renderResult(value) {
  if (value && Array.isArray(value.content)) {
    const text = value.content.filter((part) => part?.type === "text").map((part) => String(part.text)).join("\n");
    return value.isError ? t("bridge.error", { text }) : text;
  }
  return JSON.stringify(value);
}
function canonArgs(args) {
  const sort = (value) => {
    if (Array.isArray(value)) return value.map(sort);
    if (value && typeof value === "object") {
      return Object.fromEntries(Object.keys(value).filter((key) => key !== "confirm").sort().map((key) => [key, sort(value[key])]));
    }
    return value;
  };
  return JSON.stringify(sort(args ?? {}));
}
function listChangedCounter() {
  const decoder = new TextDecoder();
  let buffer = "";
  return {
    push(chunk) {
      if (!chunk) return 0;
      buffer += decoder.decode(chunk, { stream: true });
      let count = 0;
      for (; ; ) {
        const end = buffer.search(/\r?\n\r?\n/);
        if (end < 0) break;
        const event = buffer.slice(0, end);
        buffer = buffer.slice(end).replace(/^\r?\n\r?\n/, "");
        const data = event.split(/\r?\n/).filter((line) => line.startsWith("data:")).map((line) => line.slice(5).trim()).join("\n");
        if (!data) continue;
        try {
          if (JSON.parse(data)?.method === "notifications/tools/list_changed") count++;
        } catch {
        }
      }
      return count;
    }
  };
}
var McpBridge = class {
  constructor(ctx, config, http) {
    this.ctx = ctx;
    this.config = config;
    this.http = http;
  }
  disposers = /* @__PURE__ */ new Map();
  /** 已注册工具的定义指纹（DSH 里的工具全名 -> 定义的 JSON），用来发现「名字没变、定义变了」 */
  signatures = /* @__PURE__ */ new Map();
  session = "";
  controller;
  /** 结束长连接的退避等待，马上重连（刚登录时用）。 */
  wakeUp;
  /** DSH 里的工具全名 -> 服务端工具名 */
  serverNames = /* @__PURE__ */ new Map();
  metas = /* @__PURE__ */ new Map();
  previews = /* @__PURE__ */ new Map();
  local = /* @__PURE__ */ new Map();
  /** 服务端在 tools/list 里标了 readOnlyHint 的工具（服务端工具名）；工作台取数只能用它们 */
  readOnly = /* @__PURE__ */ new Set();
  status = { lastSyncAt: 0, tools: 0, error: "" };
  /** 有写操作完成时调用；index.ts 接到 WriteLog 上 */
  onWrite;
  async rpc(method, params, timeoutMs = 3e4, retried = false) {
    const headers = { "content-type": "application/json", accept: "application/json, text/event-stream" };
    if (this.session) headers["Mcp-Session-Id"] = this.session;
    const response = await this.http.fetch("/mcp", { method: "POST", headers, body: JSON.stringify({ jsonrpc: "2.0", id: 1, method, params }) }, timeoutMs);
    if (response.status === 404) {
      this.session = "";
      if (retried) throw new Error(`MCP ${method} HTTP 404`);
      return this.rpc(method, params, timeoutMs, true);
    }
    if (!response.ok) throw new Error(`MCP ${method} HTTP ${response.status}`);
    this.session = response.headers.get("mcp-session-id") || this.session;
    const body = await response.json();
    if (body.error) throw new Error(body.error.message);
    return body.result;
  }
  /** 注册由本机完成的 action（例如要读本机文件的上传）。键是 `<工具>.<action>`。 */
  registerLocal(key, fn) {
    this.local.set(key, fn);
  }
  /** 直接调服务端工具（本机诊断用）。 */
  async callTool(name2, args) {
    return this.rpc("tools/call", { name: name2, arguments: args });
  }
  /** 当前员工能用、并且服务端标为只读的工具；没权限的工具不在 tools/list 里，也就不在这里。 */
  isReadOnlyTool(serverName) {
    return this.readOnly.has(serverName);
  }
  /** 当前员工能不能用这个工具（服务端按权限列出的工具）。 */
  hasTool(serverName) {
    return [...this.serverNames.values()].includes(serverName);
  }
  /** 这次调用该不该先让用户批准；只读的返回 undefined。 */
  approval(fullName, args) {
    const metas = this.metas.get(fullName);
    const meta = metas?.[String(args?.action)];
    if (!meta) return void 0;
    if (meta.e === "danger") {
      if (!args?.confirm) return void 0;
      return { effect: "danger", zh: meta.zh, en: meta.en, preview: this.previews.get(`${fullName}
${canonArgs(args)}`) };
    }
    return { effect: "write", zh: meta.zh, en: meta.en };
  }
  remember(fullName, args, result) {
    try {
      const text = result?.content?.[0]?.text;
      if (typeof text !== "string" || !text.includes("confirm_required")) return;
      const body = JSON.parse(text);
      if (body?.confirm_required && typeof body.preview === "string") {
        if (this.previews.size > 50) this.previews.clear();
        this.previews.set(`${fullName}
${canonArgs(args)}`, body.preview);
      }
    } catch {
    }
  }
  /** 写操作记一笔；记录失败不能影响工具结果。exec.agent.session.id 是调用它的会话。 */
  noteWrite(tool, args, exec, at, ok) {
    const sessionId = exec?.agent?.session?.id;
    if (!this.onWrite || typeof sessionId !== "string" || !sessionId) return;
    const write2 = writeOf(tool, args);
    if (!write2) return;
    try {
      this.onWrite(sessionId, { at, tool: String(tool.name), ok, ...write2 });
    } catch {
    }
  }
  register(tool) {
    const name2 = `mcp__${this.config.serverName}__${tool.name}`;
    const actions = tool._meta?.dshee?.actions;
    if (actions && typeof actions === "object") this.metas.set(name2, actions);
    this.serverNames.set(name2, tool.name);
    if (tool.annotations?.readOnlyHint === true) this.readOnly.add(tool.name);
    const dispose = this.ctx.tools.register({
      name: name2,
      description: tool.description || tool.name,
      parameters: tool.inputSchema || { type: "object", properties: {} },
      output: { schema: {}, render: (_args, value) => [{ type: "text", text: renderResult(value) }] },
      execute: async (args, exec) => {
        const local = this.local.get(`${tool.name}.${args?.action}`);
        if (local) return local(args);
        const at = Date.now();
        let result;
        try {
          result = await this.rpc("tools/call", { name: tool.name, arguments: args }, 0);
        } catch (error) {
          this.noteWrite(tool, args, exec, at, false);
          throw error;
        }
        this.remember(name2, args, result);
        this.noteWrite(tool, args, exec, at, resultOk(result));
        return result;
      }
    });
    this.signatures.set(name2, JSON.stringify(tool));
    this.disposers.set(name2, () => {
      dispose();
      this.signatures.delete(name2);
      this.metas.delete(name2);
      this.serverNames.delete(name2);
      this.readOnly.delete(tool.name);
    });
  }
  async sync() {
    let result;
    try {
      if (!this.session) await this.rpc("initialize", { protocolVersion: "2025-06-18", capabilities: {}, clientInfo: { name: "dsh-ee-connect", version: this.config.version } });
      result = await this.rpc("tools/list");
    } catch (error) {
      this.status.error = error instanceof Error ? error.message : String(error);
      throw error;
    }
    this.status.error = "";
    this.status.lastSyncAt = Date.now();
    this.status.tools = (result.tools ?? []).length;
    const next = new Map((result.tools ?? []).map((tool) => [`mcp__${this.config.serverName}__${tool.name}`, tool]));
    for (const [name2, dispose] of this.disposers) {
      const tool = next.get(name2);
      if (!tool || JSON.stringify(tool) !== this.signatures.get(name2)) {
        dispose();
        this.disposers.delete(name2);
      }
    }
    for (const [name2, tool] of next) {
      if (!this.disposers.has(name2)) this.register(tool);
    }
  }
  async start() {
    this.controller = new AbortController();
    const signal = this.controller.signal;
    await this.sync().catch(() => void 0);
    void (async () => {
      let delay = 1e3;
      while (!signal.aborted) {
        try {
          const response = await this.http.fetch("/mcp", { headers: { accept: "text/event-stream" }, signal }, 0);
          const reader = response.ok ? response.body?.getReader() : void 0;
          if (reader) {
            delay = 1e3;
            await this.sync().catch(() => void 0);
            const events = listChangedCounter();
            while (true) {
              const { done, value } = await reader.read();
              if (done) break;
              if (events.push(value) > 0) await this.sync().catch(() => void 0);
            }
          }
        } catch {
        }
        if (signal.aborted) break;
        let onAbort;
        try {
          await new Promise((resolve) => {
            const timer = setTimeout(resolve, delay);
            onAbort = () => {
              clearTimeout(timer);
              resolve(void 0);
            };
            this.wakeUp = () => {
              clearTimeout(timer);
              delay = 1e3;
              resolve(void 0);
            };
            signal.addEventListener("abort", onAbort, { once: true });
          });
        } finally {
          if (onAbort) signal.removeEventListener("abort", onAbort);
          this.wakeUp = void 0;
        }
        delay = Math.min(delay * 2, 6e4);
      }
    })();
  }
  wake() {
    this.wakeUp?.();
  }
  /** 换了 dsh-ee 地址：断开旧的长连接和会话，连到新地址重新同步；已注册的工具留着，由同步增删。 */
  async restart() {
    this.controller?.abort();
    this.session = "";
    await this.start();
  }
  stop() {
    this.controller?.abort();
    for (const dispose of this.disposers.values()) dispose();
    this.disposers.clear();
  }
};

// src/approval.ts
function describeArgs(args, zh2 = true) {
  if (!args || typeof args !== "object") return "";
  const parts = [];
  for (const key of ["id", "slug", "kind", "name", "path", "username", "user_id", "department", "decision", "enabled", "reason"]) {
    const value = args[key];
    if (typeof value === "string" || typeof value === "number") {
      const text = String(value);
      parts.push(`${key}=${text.length > 60 ? `${text.slice(0, 60)}…` : text}`);
    }
  }
  const title = args.spec?.title;
  if (typeof title === "string") parts.push(`title=${title.length > 60 ? `${title.slice(0, 60)}…` : title}`);
  if (!parts.length) return "";
  return zh2 ? `（${parts.join("，")}）` : ` (${parts.join(", ")})`;
}
function registerApproval(ctx, bridge) {
  ctx.on("tools/pre-execute", async (exec, next) => {
    const need = bridge.approval(String(exec.name), exec.arguments);
    if (!need) return next();
    const preview = need.preview ? `
${need.preview}` : "";
    return {
      kind: "ask",
      reason: `dsh-ee ${need.effect}: ${exec.name}`,
      displayReason: {
        en: `dsh-ee: ${need.en}${describeArgs(exec.arguments, false)}${preview}`,
        zh: `dsh-ee：${need.zh}${describeArgs(exec.arguments)}${preview}`
      }
    };
  });
}

// src/doctor.ts
function compareVersions(a, b) {
  const parse = (v) => {
    const [core, pre] = v.replace(/^v/, "").split("-", 2);
    return { nums: core.split(".").map((n) => Number.parseInt(n, 10) || 0), pre };
  };
  const x = parse(a);
  const y = parse(b);
  for (let i = 0; i < Math.max(x.nums.length, y.nums.length); i++) {
    const d = (x.nums[i] ?? 0) - (y.nums[i] ?? 0);
    if (d) return d;
  }
  if (x.pre === y.pre) return 0;
  if (!x.pre) return 1;
  if (!y.pre) return -1;
  return x.pre < y.pre ? -1 : 1;
}
async function runLocalDoctor(ctx, config, tokens, http, bridge, bindings) {
  const checks = [];
  const add = (check) => checks.push(check);
  if (bindings?.warning()) add({ id: "app_bindings", status: "warn", title: bindings.warning(), fix: t("doctor.appBindingsFix") });
  const current = await tokens.resolve();
  if (!current?.value) {
    add({ id: "credential", status: "fail", title: t("doctor.noCredential"), fix: t("doctor.loginFix") });
    return { summary: summarize(checks), checks };
  }
  add({ id: "credential", status: "ok", title: t("doctor.hasCredential"), detail: t("doctor.source", { source: current.source }) });
  let reachable = true;
  try {
    const response = await http.fetch("/api/dsh/whoami");
    if (response.ok) {
      const me = await response.json();
      const days = me.expires_at ? (new Date(me.expires_at).getTime() - Date.now()) / 864e5 : Infinity;
      add(days < 7 ? { id: "token", status: "warn", title: t("doctor.expiringSoon", { days: Math.max(0, Math.ceil(days)) }), detail: me.username, fix: t("doctor.reLoginFix") } : { id: "token", status: "ok", title: t("doctor.signedInAs", { username: me.username, department: me.department }) });
    } else {
      const body = await response.json().catch(() => null);
      if (response.status === 401) {
        add({ id: "token", status: "fail", title: t("doctor.loginExpired"), detail: body?.code ?? "HTTP 401", fix: t("doctor.reLoginFix2") });
      } else {
        add({ id: "token", status: "fail", title: t("doctor.readAccountFailed", { status: response.status }), detail: t("doctor.serverRestarting"), fix: t("doctor.retryDiagnose") });
      }
    }
  } catch {
    reachable = false;
    add({ id: "token", status: "fail", title: t("doctor.unreachable", { base: config.base }), fix: t("doctor.unreachableFix") });
  }
  const npmrc = npmrcState(ctx, config, current.value);
  add(npmrc === "ok" ? { id: "npmrc", status: "ok", title: t("doctor.npmrcOk"), detail: npmrcPath(ctx) } : {
    id: "npmrc",
    status: "warn",
    title: npmrc === "missing" ? t("doctor.npmrcMissing") : t("doctor.npmrcMismatch"),
    detail: t("doctor.npmrcDetail", { path: npmrcPath(ctx) }),
    fix: t("doctor.npmrcFix")
  });
  if (!reachable) return { summary: summarize(checks), checks };
  let models = [];
  try {
    const response = await http.fetch("/v1/models");
    if (response.ok) models = (await response.json()).data.map((m) => m.id);
    add(models.length ? { id: "models", status: "ok", title: t("doctor.modelsAvailable", { models: models.join(t("common.listSeparator")) }) } : { id: "models", status: "fail", title: response.ok ? t("doctor.noModels") : t("doctor.readModelsHttp", { status: response.status }), fix: t("doctor.modelPermissionFix") });
  } catch {
    add({ id: "models", status: "fail", title: t("doctor.readModelsFailed") });
  }
  let server;
  try {
    const result = await bridge.callTool("ee_doctor", { action: "check" });
    if (!result?.isError) server = JSON.parse(result.content[0].text);
  } catch {
  }
  if (server) {
    for (const c of server.checks ?? []) add({ ...c, id: `server.${c.id}` });
    if (server.default_model && models.length && !models.includes(server.default_model)) {
      add({ id: "default_model", status: "warn", title: t("doctor.defaultModelMissing", { model: server.default_model }), fix: t("doctor.chooseModelFix") });
    }
    if (server.connect_version && config.version !== "dev") {
      add(compareVersions(server.connect_version, config.version) > 0 ? { id: "plugin_version", status: "warn", title: t("doctor.pluginNewer", { latest: server.connect_version, version: config.version }), fix: t("doctor.updateFix") } : { id: "plugin_version", status: "ok", title: t("doctor.pluginLatest", { version: config.version }) });
    }
    const manager = ctx.get("pluginManager");
    if (manager && (server.required_plugins ?? []).length) {
      try {
        const have = new Set((await manager.listBundles()).filter((b) => b.installed || b.enabled).map((b) => b.name));
        const missing = server.required_plugins.map((p) => p.name).filter((n) => !have.has(n));
        add(missing.length ? { id: "required_plugins", status: "warn", title: t("doctor.missingPlugins", { plugins: missing.join(t("common.listSeparator")) }), fix: t("doctor.missingPluginsFix") } : { id: "required_plugins", status: "ok", title: t("doctor.pluginsOk") });
      } catch {
      }
    }
  }
  add(bridge.status.error ? { id: "bridge", status: "fail", title: t("doctor.bridgeFailed"), detail: bridge.status.error, fix: t("doctor.bridgeFix") } : { id: "bridge", status: "ok", title: t("doctor.bridgeOk", { count: bridge.status.tools }) });
  return { summary: summarize(checks), checks };
}
function summarize(checks) {
  const fails = checks.filter((c) => c.status === "fail").length;
  const warns = checks.filter((c) => c.status === "warn").length;
  return fails ? t("doctor.summaryFail", { fails, warns }) : warns ? t("doctor.summaryWarn", { warns }) : t("doctor.summaryOk");
}
function registerDoctor(ctx, config, tokens, http, bridge, bindings) {
  const dispose = ctx.tools.register({
    name: "ee_local_doctor",
    description: t("doctor.description"),
    parameters: { type: "object", properties: {} },
    output: { schema: {}, render: (_args, value) => [{ type: "text", text: JSON.stringify(value) }] },
    execute: async () => runLocalDoctor(ctx, config, tokens, http, bridge, bindings)
  });
  ctx.effect(() => dispose);
}

// node_modules/@deepseek-ai/cosmokit/lib/index.js
function isNullable(value) {
  return value === null || value === void 0;
}
function defineProperty(object, key, value) {
  return Object.defineProperty(object, key, {
    writable: true,
    value,
    enumerable: false
  });
}
var write = Symbol.for("cosmokit.volatile.write");
function is(type, value) {
  if (arguments.length === 1) return (value2) => is(type, value2);
  return type in globalThis && value instanceof globalThis[type] || Object.prototype.toString.call(value).slice(8, -1) === type;
}
function isArrayBufferLike(value) {
  return is("ArrayBuffer", value) || is("SharedArrayBuffer", value);
}
function isArrayBufferSource(value) {
  return isArrayBufferLike(value) || ArrayBuffer.isView(value);
}
var Binary;
(function(Binary2) {
  Binary2.is = isArrayBufferLike;
  Binary2.isSource = isArrayBufferSource;
  function fromSource(source) {
    if (ArrayBuffer.isView(source)) return source.buffer.slice(source.byteOffset, source.byteOffset + source.byteLength);
    else return source;
  }
  Binary2.fromSource = fromSource;
  function toBase64(source) {
    source = fromSource(source);
    if (typeof Buffer !== "undefined") return Buffer.from(source).toString("base64");
    let binary = "";
    const bytes = new Uint8Array(source);
    for (let i = 0; i < bytes.byteLength; i++) binary += String.fromCharCode(bytes[i]);
    return btoa(binary);
  }
  Binary2.toBase64 = toBase64;
  function fromBase64(source) {
    if (typeof Buffer !== "undefined") return fromSource(Buffer.from(source, "base64"));
    return Uint8Array.from(atob(source), (c) => c.charCodeAt(0));
  }
  Binary2.fromBase64 = fromBase64;
  function toHex(source) {
    source = fromSource(source);
    if (typeof Buffer !== "undefined") return Buffer.from(source).toString("hex");
    return Array.from(new Uint8Array(source), (byte) => byte.toString(16).padStart(2, "0")).join("");
  }
  Binary2.toHex = toHex;
  function fromHex(source) {
    if (typeof Buffer !== "undefined") return fromSource(Buffer.from(source, "hex"));
    const hex = source.length % 2 === 0 ? source : source.slice(0, source.length - 1);
    const buffer = [];
    for (let i = 0; i < hex.length; i += 2) buffer.push(parseInt(`${hex[i]}${hex[i + 1]}`, 16));
    return Uint8Array.from(buffer).buffer;
  }
  Binary2.fromHex = fromHex;
})(Binary || (Binary = {}));
var base64ToArrayBuffer = Binary.fromBase64;
var arrayBufferToBase64 = Binary.toBase64;
var hexToArrayBuffer = Binary.fromHex;
var arrayBufferToHex = Binary.toHex;
function tokenize(source, delimiters, delimiter) {
  const output = [];
  let state = 0;
  for (let i = 0; i < source.length; i++) {
    const code = source.charCodeAt(i);
    if (code >= 65 && code <= 90) {
      if (state === 1) {
        const next = source.charCodeAt(i + 1);
        if (next >= 97 && next <= 122) output.push(delimiter);
        output.push(code + 32);
      } else {
        if (state !== 0) output.push(delimiter);
        output.push(code + 32);
      }
      state = 1;
    } else if (code >= 97 && code <= 122) {
      output.push(code);
      state = 2;
    } else if (delimiters.includes(code)) {
      if (state !== 0) output.push(delimiter);
      state = 0;
    } else output.push(code);
  }
  return String.fromCharCode(...output);
}
function paramCase(source) {
  return tokenize(source, [45, 95], 45);
}
var hyphenate = paramCase;
var Time;
(function(Time2) {
  Time2.millisecond = 1;
  Time2.second = 1e3;
  Time2.minute = Time2.second * 60;
  Time2.hour = Time2.minute * 60;
  Time2.day = Time2.hour * 24;
  Time2.week = Time2.day * 7;
  let timezoneOffset = (/* @__PURE__ */ new Date()).getTimezoneOffset();
  function setTimezoneOffset(offset) {
    timezoneOffset = offset;
  }
  Time2.setTimezoneOffset = setTimezoneOffset;
  function getTimezoneOffset() {
    return timezoneOffset;
  }
  Time2.getTimezoneOffset = getTimezoneOffset;
  function getDateNumber(date = /* @__PURE__ */ new Date(), offset) {
    if (typeof date === "number") date = new Date(date);
    if (offset === void 0) offset = timezoneOffset;
    return Math.floor((date.valueOf() / Time2.minute - offset) / 1440);
  }
  Time2.getDateNumber = getDateNumber;
  function fromDateNumber(value, offset) {
    const date = new Date(value * Time2.day);
    if (offset === void 0) offset = timezoneOffset;
    return new Date(+date + offset * Time2.minute);
  }
  Time2.fromDateNumber = fromDateNumber;
  const numeric = /\d+(?:\.\d+)?/.source;
  const timeRegExp = new RegExp(`^${[
    "w(?:eek(?:s)?)?",
    "d(?:ay(?:s)?)?",
    "h(?:our(?:s)?)?",
    "m(?:in(?:ute)?(?:s)?)?",
    "s(?:ec(?:ond)?(?:s)?)?"
  ].map((unit) => `(${numeric}${unit})?`).join("")}$`);
  function parseTime(source) {
    const capture = timeRegExp.exec(source);
    if (!capture) return 0;
    return (parseFloat(capture[1]) * Time2.week || 0) + (parseFloat(capture[2]) * Time2.day || 0) + (parseFloat(capture[3]) * Time2.hour || 0) + (parseFloat(capture[4]) * Time2.minute || 0) + (parseFloat(capture[5]) * Time2.second || 0);
  }
  Time2.parseTime = parseTime;
  function parseDate(date) {
    const parsed = parseTime(date);
    if (parsed) date = Date.now() + parsed;
    else if (/^\d{1,2}(:\d{1,2}){1,2}$/.test(date)) date = `${(/* @__PURE__ */ new Date()).toLocaleDateString()}-${date}`;
    else if (/^\d{1,2}-\d{1,2}-\d{1,2}(:\d{1,2}){1,2}$/.test(date)) date = `${(/* @__PURE__ */ new Date()).getFullYear()}-${date}`;
    return date ? new Date(date) : /* @__PURE__ */ new Date();
  }
  Time2.parseDate = parseDate;
  function format(ms) {
    const abs = Math.abs(ms);
    if (abs >= Time2.day - Time2.hour / 2) return Math.round(ms / Time2.day) + "d";
    else if (abs >= Time2.hour - Time2.minute / 2) return Math.round(ms / Time2.hour) + "h";
    else if (abs >= Time2.minute - Time2.second / 2) return Math.round(ms / Time2.minute) + "m";
    else if (abs >= Time2.second) return Math.round(ms / Time2.second) + "s";
    return ms + "ms";
  }
  Time2.format = format;
  function toDigits(source, length = 2) {
    return source.toString().padStart(length, "0");
  }
  Time2.toDigits = toDigits;
  function template(template2, time = /* @__PURE__ */ new Date()) {
    return template2.replace("yyyy", time.getFullYear().toString()).replace("yy", time.getFullYear().toString().slice(2)).replace("MM", toDigits(time.getMonth() + 1)).replace("dd", toDigits(time.getDate())).replace("hh", toDigits(time.getHours())).replace("mm", toDigits(time.getMinutes())).replace("ss", toDigits(time.getSeconds())).replace("SSS", toDigits(time.getMilliseconds(), 3));
  }
  Time2.template = template;
})(Time || (Time = {}));

// node_modules/@deepseek-ai/cordis/lib/index.js
var DisposableList = class {
  sn = 0;
  map = /* @__PURE__ */ new Map();
  weak = /* @__PURE__ */ new WeakMap();
  get length() {
    return this.map.size;
  }
  push(value) {
    const sn = ++this.sn;
    this.map.set(sn, value);
    this.weak.set(value, sn);
    return () => this.map.delete(sn);
  }
  delete(value) {
    const sn = this.weak.get(value);
    if (!sn) return false;
    return this.map.delete(sn);
  }
  clear() {
    const values = [...this.map.values()];
    this.map.clear();
    return values.reverse();
  }
  [Symbol.iterator]() {
    return this.map.values();
  }
  [Symbol.for("nodejs.util.inspect.custom")]() {
    return [...this];
  }
};
var symbols = {
  shadow: Symbol.for("cordis.shadow"),
  receiver: Symbol.for("cordis.receiver"),
  original: Symbol.for("cordis.original"),
  metadata: Symbol.for("cordis.metadata"),
  initHooks: Symbol.for("cordis.initHooks"),
  checkProto: Symbol.for("cordis.checkProto"),
  effect: Symbol.for("cordis.effect"),
  filter: Symbol.for("cordis.filter"),
  isolate: Symbol.for("cordis.isolate"),
  intercept: Symbol.for("cordis.intercept"),
  init: Symbol.for("cordis.init"),
  check: Symbol.for("cordis.check"),
  config: Symbol.for("cordis.config"),
  invoke: Symbol.for("cordis.invoke"),
  extend: Symbol.for("cordis.extend"),
  tracker: Symbol.for("cordis.tracker"),
  resolveConfig: Symbol.for("cordis.resolveConfig")
};
var GeneratorFunction = function* () {
}.constructor;
var AsyncGeneratorFunction = async function* () {
}.constructor;
function isConstructor(func) {
  if (!func.prototype) return false;
  if (func instanceof GeneratorFunction) return false;
  if (AsyncGeneratorFunction !== Function && func instanceof AsyncGeneratorFunction) return false;
  return true;
}
function joinPrototype(proto1, proto2) {
  if (proto1 === Object.prototype) return proto2;
  const result = Object.create(joinPrototype(Object.getPrototypeOf(proto1), proto2));
  for (const key of Reflect.ownKeys(proto1)) Object.defineProperty(result, key, Object.getOwnPropertyDescriptor(proto1, key));
  return result;
}
function isObject(value) {
  return value && (typeof value === "object" || typeof value === "function");
}
function getPropertyDescriptor(target, prop) {
  let proto = target;
  while (proto) {
    const desc = Reflect.getOwnPropertyDescriptor(proto, prop);
    if (desc) return desc;
    proto = Object.getPrototypeOf(proto);
  }
}
function getTraceable(ctx, value) {
  if (!isObject(value)) return value;
  if (Object.hasOwn(value, symbols.shadow)) return Object.getPrototypeOf(value);
  const tracker = value[symbols.tracker];
  if (!tracker) return value;
  return createTraceable(ctx, value, tracker);
}
function withProps(target, props) {
  if (!props) return target;
  return new Proxy(target, {
    get: (target2, prop, receiver) => {
      if (prop in props && prop !== "constructor") return Reflect.get(props, prop, receiver);
      return Reflect.get(target2, prop, receiver);
    },
    set: (target2, prop, value, receiver) => {
      if (prop in props && prop !== "constructor") return Reflect.set(props, prop, value, receiver);
      return Reflect.set(target2, prop, value, receiver);
    }
  });
}
function withProp(target, prop, value) {
  return withProps(target, Object.defineProperty(/* @__PURE__ */ Object.create(null), prop, {
    value,
    writable: false
  }));
}
function createShadow(ctx, target, property, receiver) {
  if (!property) return receiver;
  const origin = Reflect.getOwnPropertyDescriptor(target, property)?.value;
  if (!origin) return receiver;
  return withProp(receiver, property, ctx.extend({ [symbols.shadow]: origin }));
}
function createShadowMethod(ctx, value, outer, shadow) {
  return new Proxy(value, { apply: (target, thisArg, args) => {
    if (thisArg === outer) thisArg = shadow;
    return getTraceable(ctx, Reflect.apply(target, thisArg, args));
  } });
}
function createTraceable(ctx, value, tracker) {
  if (ctx[symbols.shadow] && !tracker.noShadow) ctx = Object.getPrototypeOf(ctx);
  const proxy = new Proxy(value, {
    get: (target, prop, receiver) => {
      if (prop === symbols.original) return target;
      if (prop === tracker.property) return ctx;
      if (typeof prop === "symbol") return Reflect.get(target, prop, receiver);
      if (tracker.associate && ctx.reflect.props[`${tracker.associate}.${prop}`]) return Reflect.get(ctx, `${tracker.associate}.${prop}`, withProp(ctx, symbols.receiver, receiver));
      let shadow, innerValue;
      const desc = getPropertyDescriptor(target, prop);
      if (desc && "value" in desc) innerValue = desc.value;
      else {
        shadow = createShadow(ctx, target, tracker.property, receiver);
        innerValue = Reflect.get(target, prop, shadow);
      }
      const innerTracker = innerValue?.[symbols.tracker];
      if (innerTracker) return createTraceable(ctx, innerValue, innerTracker);
      else if (!tracker.noShadow && typeof innerValue === "function") {
        shadow ??= createShadow(ctx, target, tracker.property, receiver);
        return createShadowMethod(ctx, innerValue, receiver, shadow);
      } else return innerValue;
    },
    set: (target, prop, value2, receiver) => {
      if (prop === symbols.original) return false;
      if (prop === tracker.property) return false;
      if (typeof prop === "symbol") return Reflect.set(target, prop, value2, receiver);
      if (tracker.associate && ctx.reflect.props[`${tracker.associate}.${prop}`]) return Reflect.set(ctx, `${tracker.associate}.${prop}`, value2, withProp(ctx, symbols.receiver, receiver));
      const shadow = createShadow(ctx, target, tracker.property, receiver);
      return Reflect.set(target, prop, value2, shadow);
    },
    apply: (target, thisArg, args) => {
      return applyTraceable(proxy, target, thisArg, args);
    }
  });
  return proxy;
}
function applyTraceable(proxy, value, thisArg, args) {
  if (!value[symbols.invoke]) return Reflect.apply(value, thisArg, args);
  return value[symbols.invoke].apply(proxy, args);
}
function createCallable(name2, proto, tracker) {
  const self = function(...args) {
    return applyTraceable(createTraceable(self["ctx"], self, tracker), self, this, args);
  };
  defineProperty(self, "name", name2);
  return Object.setPrototypeOf(self, proto);
}
function handleError(info, reason3, getOuterStack) {
  const innerLines = info.error.stack.split("\n");
  if (typeof reason3?.stack !== "string") {
    const outerError = new Error(reason3);
    const lines2 = outerError.stack.split("\n");
    lines2.splice(1, Infinity, ...getOuterStack());
    outerError.stack = lines2.join("\n");
    throw outerError;
  }
  const lines = reason3.stack.split("\n");
  let index = lines.indexOf(innerLines[2]);
  if (index === -1) throw reason3;
  index -= info.offset;
  while (index > 0) {
    if (!lines[index - 1].endsWith(" (<anonymous>)")) break;
    index -= 1;
  }
  lines.splice(index, Infinity, ...getOuterStack());
  reason3.stack = lines.join("\n");
  throw reason3;
}
function composeError(callback, getOuterStack = buildOuterStack()) {
  const info = {
    offset: 1,
    error: /* @__PURE__ */ new Error()
  };
  try {
    const result = callback(info);
    if (isObject(result) && "then" in result) return result.then(void 0, (reason3) => handleError(info, reason3, getOuterStack));
    else return result;
  } catch (reason3) {
    handleError(info, reason3, getOuterStack);
  }
}
function buildOuterStack(offset = 0) {
  const outerError = /* @__PURE__ */ new Error();
  return () => outerError.stack.split("\n").slice(3 + offset);
}
function isBailed(value) {
  return value !== null && value !== false && value !== void 0;
}
var EventsService = class {
  ctx;
  _hooks = {};
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
    this.on("internal/listener", function(name2, listener, options) {
      if (name2 === "internal/update" && !options.global) return (this.fiber._hooks["internal/update"] ??= new DisposableList())[options.prepend ? "unshift" : "push"](listener);
    });
    this.on("internal/update", function(config, noSave, next) {
      const cbs = [...this._hooks["internal/update"] || []];
      const _next = () => {
        return (cbs.shift() ?? next).call(this, config, noSave, _next);
      };
      return _next();
    }, {
      global: true,
      prepend: true
    });
  }
  /**
  * Resolve listeners for one dispatch and apply context filtering.
  *
  * @param type — the dispatch mode, reported on `internal/dispatch`.
  * @param args — the raw dispatch arguments; consumed up to the event name.
  * @returns the matching listener callbacks, bound to the dispatch `this`.
  */
  dispatch(type, args) {
    const thisArg = typeof args[0] === "object" || typeof args[0] === "function" ? args.shift() : null;
    const name2 = args.shift();
    if (!name2.startsWith("internal/")) this.emit("internal/dispatch", type, name2, args, thisArg);
    const filter = thisArg?.[Context.filter];
    return (this._hooks[name2] || []).filter((hook) => hook.global || !filter || filter.call(thisArg, hook.ctx)).map((hook) => hook.callback.bind(thisArg));
  }
  /**
  * Run listeners concurrently and wait for all of them.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns a promise resolving once every listener has settled.
  */
  async parallel(...args) {
    const errors = (await Promise.allSettled(this.dispatch("emit", args).map(async (cb) => cb(...args)))).filter((result) => result.status === "rejected");
    if (errors.length) throw new AggregateError(errors.map((error) => error.reason));
  }
  /**
  * Run listeners synchronously without waiting for returned promises.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  */
  emit(...args) {
    this.dispatch("emit", args).map((cb) => cb(...args));
  }
  /**
  * Run listeners in order, awaiting each, until one returns a bail value.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns the first bail value (see {@link isBailed}), if any.
  */
  async serial(...args) {
    for (const cb of this.dispatch("serial", args)) {
      const result = await cb(...args);
      if (isBailed(result)) return result;
    }
  }
  /**
  * Run listeners synchronously until one returns a bail value.
  *
  * @param args — optional `this`, the event name, then listener arguments.
  * @returns the first bail value (see {@link isBailed}), if any.
  */
  bail(...args) {
    for (const cb of this.dispatch("bail", args)) {
      const result = cb(...args);
      if (isBailed(result)) return result;
    }
  }
  /**
  * Compose listeners around the final `next` callback.
  *
  * The last dispatch argument is treated as the innermost `next`. Listeners
  * run outermost-first; a listener that does not call `next()` vetoes the
  * rest of the chain, including the built-in behavior.
  *
  * @param args — optional `this`, the event name, listener arguments, then `next`.
  * @returns the outermost listener's return value.
  */
  waterfall(...args) {
    const cbs = this.dispatch("waterfall", args);
    const inner = args.pop();
    const next = () => {
      return (cbs.shift() ?? inner)(...args);
    };
    args.push(next);
    return next();
  }
  /**
  * Store a listener record as an effect on the current fiber.
  *
  * @param label — effect label shown in fiber diagnostics.
  * @param hooks — the listener list for one event.
  * @param callback — the listener to store.
  * @param options — placement and filtering options.
  * @returns a disposer that unregisters the listener.
  */
  register(label, hooks, callback, options) {
    const method = options.prepend ? "unshift" : "push";
    return this.ctx.fiber.effect(() => {
      hooks[method]({
        ctx: this.ctx,
        callback,
        ...options
      });
      return () => this.unregister(hooks, callback);
    }, label);
  }
  /**
  * Remove a stored listener record.
  *
  * @param hooks — the listener list for one event.
  * @param callback — the listener to remove.
  * @returns `true` if the listener was found and removed.
  */
  unregister(hooks, callback) {
    const index = hooks.findIndex((hook) => hook.callback === callback);
    if (index >= 0) {
      hooks.splice(index, 1);
      return true;
    }
  }
  /**
  * Register an event listener owned by the current fiber.
  *
  * The listener is removed automatically when the fiber unloads. Throws
  * `CordisError('INACTIVE_EFFECT')` if the fiber is already disposed.
  *
  * @param name — the event name to listen for.
  * @param listener — called with the dispatch arguments.
  * @param options — listener options; a boolean is shorthand for `prepend`.
  * @returns a disposer removing the listener; `true` if it was still registered.
  */
  on(name2, listener, options) {
    if (typeof options !== "object") options = { prepend: options };
    this.ctx.fiber.assertActive();
    listener = this.ctx.reflect.bind(listener);
    const result = this.bail(this.ctx, "internal/listener", name2, listener, options);
    if (result) return result;
    const hooks = this._hooks[name2] ||= [];
    const label = `ctx.on(${typeof name2 === "string" ? JSON.stringify(name2) : name2.toString()})`;
    return this.register(label, hooks, listener, options);
  }
  /**
  * Register an event listener that disposes itself after the first call.
  *
  * @param name — the event name to listen for.
  * @param listener — called at most once with the dispatch arguments.
  * @param options — listener options; a boolean is shorthand for `prepend`.
  * @returns a disposer removing the listener; `true` if it was still registered.
  */
  once(name2, listener, options) {
    const dispose = this.on(name2, function(...args) {
      dispose();
      return listener.apply(this, args);
    }, options);
    return dispose;
  }
};
var defaultFormatters = {
  s: (value) => String(value),
  d: (value) => Math.trunc(Number(value)),
  i: (value) => Math.trunc(Number(value)),
  f: (value) => Number(value),
  o: (value) => JSON.stringify(value),
  O: (value) => JSON.stringify(value),
  c: () => "",
  C: (value, exporter, message) => {
    return Logger.color(exporter, Logger.code(message.name, exporter.colors), value);
  }
};
function isAggregateError(error) {
  return error instanceof Error && Array.isArray(error["errors"]);
}
var Logger = class {
  service;
  static color(exporter, code, value, decoration = "") {
    if (!exporter.colors) return "" + value;
    return `\x1B[3${code < 8 ? code : "8;5;" + code}${exporter.colors >= 2 ? decoration : ""}m${value}\x1B[0m`;
  }
  static code(name2, level) {
    let hash = 0;
    for (let i = 0; i < name2.length; i++) {
      hash = (hash << 3) - hash + name2.charCodeAt(i) + 13;
      hash |= 0;
    }
    const colors = !level ? [] : level >= 2 ? c256 : c16;
    return colors[Math.abs(hash) % colors.length];
  }
  static format(exporter, message) {
    const args = message.args.slice();
    if (args[0] instanceof Error) {
      args[0] = args[0].stack || args[0].message;
      args.unshift("%s");
    } else if (typeof args[0] !== "string") args.unshift("%o");
    let format = args.shift();
    format = format.replace(/%([a-zA-Z%])/g, (match, char) => {
      if (match === "%%") return "%";
      const formatter = exporter.formatters?.[char] ?? defaultFormatters[char];
      if (typeof formatter === "function") return formatter(args.shift(), exporter, message);
      return match;
    });
    const oFormatter = exporter.formatters?.o ?? defaultFormatters.o;
    for (let arg of args) {
      if (typeof arg === "object" && arg) arg = oFormatter(arg, exporter, message);
      format += " " + arg;
    }
    const { maxLength = 10240 } = exporter;
    return format.split(/\r?\n/g).map((line) => {
      return line.slice(0, maxLength) + (line.length > maxLength ? "..." : "");
    }).join("\n");
  }
  constructor(options, service) {
    this.service = service;
    Object.assign(this, options);
    this.error = this._method("error", 0);
    this.info = this._method("info", 1);
    this.warn = this._method("warn", 2);
    this.debug = this._method("debug", 3);
  }
  _method(type, level) {
    return (...args) => {
      if (args.length === 1 && args[0] instanceof Error) {
        if (args[0].cause) this[type](args[0].cause);
        else if (isAggregateError(args[0])) {
          args[0].errors.forEach((error) => this[type](error));
          return;
        }
      }
      const sn = ++this.service._snMessage;
      const ts = Date.now();
      for (const exporter of this.service.exporters.values()) {
        if ((exporter.levels?.[this.name] ?? exporter.levels?.default ?? this.level ?? 1) < level) continue;
        const message = {
          sn,
          ts,
          type,
          level,
          name: this.name,
          ...this.meta,
          args
        };
        exporter.export(message);
      }
    };
  }
};
var c16 = [
  6,
  2,
  3,
  4,
  5,
  1
];
var c256 = [
  20,
  21,
  26,
  27,
  32,
  33,
  38,
  39,
  40,
  41,
  42,
  43,
  44,
  45,
  56,
  57,
  62,
  63,
  68,
  69,
  74,
  75,
  76,
  77,
  78,
  79,
  80,
  81,
  92,
  93,
  98,
  99,
  112,
  113,
  129,
  134,
  135,
  148,
  149,
  160,
  161,
  162,
  163,
  164,
  165,
  166,
  167,
  168,
  169,
  170,
  171,
  172,
  173,
  178,
  179,
  184,
  185,
  196,
  197,
  198,
  199,
  200,
  201,
  202,
  203,
  204,
  205,
  206,
  207,
  208,
  209,
  214,
  215,
  220,
  221
];
var LoggerService = class LoggerService2 {
  bufferSize = 1e3;
  buffer = [];
  ctx;
  _snMessage = 0;
  _snExporter = 0;
  exporters = /* @__PURE__ */ new Map();
  constructor(ctx) {
    const tracker = {
      property: "ctx",
      noShadow: true
    };
    const self = createCallable("logger", joinPrototype(Object.getPrototypeOf(this), Function.prototype), tracker);
    Object.assign(self, this);
    self.ctx = ctx;
    defineProperty(self, symbols.tracker, tracker);
    self.exporter({
      colors: 3,
      export: (message) => {
        self.buffer.push(message);
        if (self.buffer.length > self.bufferSize) self.buffer = self.buffer.slice(-self.bufferSize);
      }
    });
    return self;
  }
  /**
  * Register an exporter and dispose it with the current fiber.
  *
  * @param exporter — the sink that receives structured log messages.
  * @returns a disposer that removes the exporter.
  */
  exporter(exporter) {
    return this.ctx.effect(() => {
      const id = ++this._snExporter;
      this.exporters.set(id, exporter);
      return () => this.exporters.delete(id);
    }, "ctx.logger.exporter()");
  }
  _resolveConfig() {
    let intercept = this.ctx[symbols.intercept];
    const configs = [];
    while ("logger" in intercept) {
      if (Object.hasOwn(intercept, "logger")) configs.unshift(intercept["logger"]);
      intercept = Object.getPrototypeOf(intercept);
    }
    return Object.assign({}, ...configs);
  }
  [symbols.invoke](name2) {
    const config = this._resolveConfig();
    const fiber = (this.ctx[symbols.shadow] ?? this.ctx).fiber;
    name2 ??= config.name;
    name2 ??= hyphenate(fiber.name);
    return new Logger({
      name: name2,
      level: config.level,
      meta: { fiber: new WeakRef(fiber) }
    }, this);
  }
  static {
    for (const type of [
      "error",
      "info",
      "warn",
      "debug"
    ]) LoggerService2.prototype[type] = function(...args) {
      return this()[type](...args);
    };
  }
};
function enhanceError(error) {
  const lines = error.stack.split("\n");
  lines.splice(0, 2, `Error: ${error.message}`);
  error.stack = lines.join("\n");
  return error;
}
var RESERVED_WORDS = ["prototype", "then"];
function isSpecialProperty(prop) {
  return typeof prop === "symbol" || RESERVED_WORDS.includes(prop) || parseInt(prop).toString() === prop || prop.startsWith("_");
}
var ReflectService = class {
  ctx;
  /** Proxy traps implementing service resolution for every context object. */
  static handler = {
    get: (target, prop, ctx) => {
      if (isSpecialProperty(prop)) return Reflect.get(target, prop, ctx);
      if (Reflect.has(target, prop)) return getTraceable(ctx, Reflect.get(target, prop, ctx));
      const error = /* @__PURE__ */ new Error(`cannot get property "${prop}" without inject`);
      try {
        const def = target.reflect.props[prop];
        if (def?.type === "accessor") return def.get.call(ctx, ctx[symbols.receiver], error);
        if (!ctx.fiber.runtime) return ctx.reflect.get(prop, false);
        return ctx.events.waterfall("internal/get", ctx, prop, error, () => {
          const key = target[symbols.isolate][prop];
          let fiber = (ctx[symbols.shadow] ?? ctx).fiber;
          while (true) {
            const impl = fiber.store?.[prop];
            if (impl) return getTraceable(ctx, impl.value);
            if (prop in fiber.inject) {
              error.message = `cannot get required service "${prop}" in inactive context`;
              throw error;
            }
            if (!fiber.runtime) throw error;
            if (fiber.parent[symbols.isolate][prop] !== key) throw error;
            fiber = fiber.parent.fiber;
          }
        });
      } catch (e) {
        throw e === error ? enhanceError(e) : e;
      }
    },
    set: (target, prop, value, ctx) => {
      if (isSpecialProperty(prop)) return Reflect.set(target, prop, value, ctx);
      const error = /* @__PURE__ */ new Error(`cannot set property "${prop}" without provide`);
      const def = target.reflect.props[prop];
      if (!def) {
        if (!ctx.fiber.runtime) return Reflect.set(target, prop, value, ctx);
        throw enhanceError(error);
      }
      try {
        if (def.type === "accessor") {
          if (!def.set) return false;
          return def.set.call(ctx, value, ctx[symbols.receiver], error);
        }
        return ctx.events.waterfall("internal/set", ctx, prop, value, error, () => {
          return ctx.reflect.set(prop, value, error);
        });
      } catch (e) {
        throw e === error ? enhanceError(e) : e;
      }
    },
    has: (target, prop) => {
      if (isSpecialProperty(prop)) return Reflect.has(target, prop);
      if (Reflect.has(target, prop)) return true;
      return !!target.reflect.props[prop];
    }
  };
  /** Service implementations, keyed by isolation label. */
  store = /* @__PURE__ */ Object.create(null);
  /** Declared context properties (services and accessors), by name. */
  props = /* @__PURE__ */ Object.create(null);
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
    this.mixin("reflect", [
      "get",
      "set",
      "provide",
      "accessor",
      "mixin"
    ]);
    this.mixin("fiber", ["runtime", "effect"]);
    this.mixin("registry", ["inject", "plugin"]);
    this.mixin("events", [
      "on",
      "once",
      "parallel",
      "emit",
      "serial",
      "bail",
      "waterfall"
    ]);
  }
  /**
  * Read a service from the store without the inject requirement.
  *
  * @param name — the service name.
  * @param strict — when `true`, only return implementations whose providing
  * fiber is currently active.
  * @returns the service value, or `undefined` when not (yet) provided.
  */
  get(name2, strict = true) {
    return getTraceable(this.ctx, this._getImpl(name2, strict)?.value);
  }
  _getImpl(name2, strict = true) {
    const key = this.ctx[symbols.isolate][name2];
    const impl = key && this.store[key];
    if (!impl) return;
    if (strict && impl.fiber.state !== 2) return;
    return impl;
  }
  /**
  * Overwrite a provided service's value.
  *
  * @param name — the service name.
  * @param value — the new service value.
  * @param error — carrier for the caller stack in diagnostics.
  * @returns `true` on success.
  * @throws when `name` was never provided, or was provided by another fiber.
  */
  set(name2, value, error) {
    const key = this.ctx[symbols.isolate][name2];
    const impl = this.store[key];
    if (!impl) throw new Error(`cannot set property "${name2}" without provide`);
    if (impl.fiber !== this.ctx.fiber) throw new Error(`cannot set property "${name2}" in multiple fibers`);
    impl.value = value;
    return true;
  }
  /**
  * Register a service implementation owned by the current fiber.
  *
  * See the `ctx.provide()` overload above for the full contract.
  *
  * @param name — the service name.
  * @param value — the service value.
  * @param check — optional availability predicate for dependents.
  * @returns a disposer that unregisters the service.
  */
  provide(name2, value, check) {
    return this.ctx.fiber.effect(() => {
      if (!this.props[name2]) this.props[name2] ??= { type: "service" };
      else if (this.props[name2].type !== "service") throw new Error(`property "${name2}" is already declared as ${this.props[name2].type}`);
      this.props[name2] = { type: "service" };
      this.ctx.root[symbols.isolate][name2] ??= Symbol(name2);
      const key = this.ctx[symbols.isolate][name2];
      const impl = {
        name: name2,
        value,
        fiber: this.ctx.fiber,
        check
      };
      if (this.store[key]) throw new Error(`service "${name2}" has been registered at <${this.store[key].fiber.name}>`);
      this.store[key] = impl;
      this.ctx.fiber.store[name2] = impl;
      if (this.ctx.fiber.state === 2) this.notify([name2]);
      return async () => {
        delete this.store[key];
        const fibers = this.notify([name2]);
        await Promise.allSettled(fibers.map((fiber) => fiber.await()));
        delete this.ctx.fiber.store[name2];
      };
    }, `ctx.provide(${JSON.stringify(name2)})`);
  }
  /**
  * Re-evaluate every fiber that requires one of the given services.
  *
  * @param names — the service names that changed.
  * @param filter — restricts notification to matching isolation scopes.
  * @returns the fibers whose dependency state was refreshed.
  */
  notify(names, filter = (ctx, name2) => ctx[symbols.isolate][name2] === this.ctx[symbols.isolate][name2]) {
    const fibers = [];
    for (const runtime of this.ctx.registry.values()) for (const fiber of runtime.fibers) {
      let hasUpdate = false;
      for (const name2 of names) {
        if (!(name2 in fiber.inject)) continue;
        if (!filter(fiber.ctx, name2)) continue;
        hasUpdate = true;
        fiber._checkImpl(name2);
      }
      if (!hasUpdate) continue;
      fiber._refresh();
      fibers.push(fiber);
    }
    for (const name2 of names) {
      const self = Object.create(this.ctx);
      self[symbols.filter] = (target) => filter(target, name2);
      this.ctx.events.emit(self, "internal/service", name2, this._getImpl(name2, false)?.value);
    }
    return fibers;
  }
  /**
  * Define a computed context property backed by get/set hooks.
  *
  * @param name — the context property name.
  * @param options — the `get` hook and optional `set` hook.
  * @returns a disposer that removes the accessor.
  */
  accessor(name2, options) {
    return this.ctx.fiber.effect(() => {
      if (name2 in this.props) throw new Error(`property "${name2}" is already declared as ${this.props[name2].type}`);
      this.props[name2] = {
        type: "accessor",
        ...options
      };
      return () => delete this.props[name2];
    }, `ctx.accessor(${JSON.stringify(name2)})`);
  }
  /**
  * Expose selected members of a service directly on `ctx`.
  *
  * See the `ctx.mixin()` overload above for the full contract.
  *
  * @param source — a context property name or a source object.
  * @param mixins — keys to forward, or a source-key → ctx-key map.
  * @returns a disposer that removes all created accessors.
  */
  mixin(source, mixins) {
    const self = this;
    return this.ctx.fiber.effect(function* () {
      const entries = Array.isArray(mixins) ? mixins.map((key) => [key, key]) : Object.entries(mixins);
      const getTarget = (ctx, error) => {
        return ctx[source];
      };
      for (const [key, value] of entries) yield self.accessor(value, {
        get(receiver, error) {
          const service = getTarget(this, error);
          if (isNullable(service)) return service;
          const mixin = receiver ? withProps(receiver, service) : service;
          const value2 = Reflect.get(service, key, mixin);
          if (typeof value2 !== "function") return value2;
          return value2.bind(mixin ?? service);
        },
        set(value2, receiver, error) {
          const service = getTarget(this, error);
          const mixin = receiver ? withProps(receiver, service) : service;
          return Reflect.set(service, key, value2, mixin);
        }
      });
    }, `ctx.mixin(${JSON.stringify(source)})`);
  }
  /**
  * Attach this context's tracing wrapper to a value.
  *
  * @param value — the value to wrap.
  * @returns the traceable wrapper (or the value itself when not applicable).
  */
  trace(value) {
    return getTraceable(this.ctx, value);
  }
  /**
  * Wrap a callback so calls trace `this` and arguments to this context.
  *
  * @param callback — the function to wrap.
  * @returns a proxy delegating to `callback` with traced values.
  */
  bind(callback) {
    return new Proxy(callback, {
      apply: (target, thisArg, args) => {
        return Reflect.apply(target, this.trace(thisArg), args.map((arg) => this.trace(arg)));
      },
      construct: (target, args, newTarget) => {
        return Reflect.construct(target, args.map((arg) => this.trace(arg)), newTarget);
      }
    });
  }
};
var kValidationError = Symbol.for("ValidationError");
var ValidationError = class extends TypeError {
  name = "ValidationError";
  /**
  * Build the aggregated message from schema issues.
  *
  * @param issues — the standard-schema issues, one message line each.
  */
  constructor(issues) {
    super(`invalid config:
` + issues.map((issue) => {
      if (issue.path) return `  - ${issue.message} (at ${issue.path.join(".")})`;
      else return `  - ${issue.message}`;
    }).join("\n"));
  }
};
Object.defineProperty(ValidationError.prototype, kValidationError, { value: true });
function resolveConfig(runtime, config) {
  if (!runtime.Config) return config;
  const result = runtime.Config["~standard"].validate(config);
  if ("then" in result) throw new TypeError("Async config validation is not supported");
  if (result.issues) throw new ValidationError(result.issues);
  else return result.value;
}
var effectInertia = /* @__PURE__ */ new WeakMap();
function runDisposable(dispose) {
  const result = dispose();
  return effectInertia.get(dispose)?.() ?? result;
}
function emitPluginDisposed(context, fiber) {
  const args = ["internal/plugin", fiber];
  let callbacks;
  try {
    callbacks = context.events.dispatch("emit", args);
  } catch (error) {
    context.logger.error(error);
    return;
  }
  for (const callback of callbacks) try {
    const returned = callback(...args);
    Promise.resolve(returned).catch((error) => context.logger.error(error));
  } catch (error) {
    context.logger.error(error);
  }
}
var CordisError = class CordisError2 extends Error {
  code;
  /**
  * @param code — the stable error code; also the default message.
  * @param message — optional human-readable override.
  */
  constructor(code, message) {
    super(message ?? CordisError2.Code[code]);
    this.code = code;
  }
};
(function(CordisError3) {
  CordisError3.Code = { INACTIVE_EFFECT: "cannot create effect on inactive context" };
})(CordisError || (CordisError = {}));
var INACTIVE = "__INACTIVE__";
var Fiber = class {
  parent;
  inject;
  runtime;
  /** Unique id within the registry; 0 for the root fiber, `null` once disposed. */
  uid;
  /** The context this fiber's plugin runs in (extends the parent context). */
  ctx;
  /** The validated plugin config (updated by `update()`); read through {@link effectiveConfig}. */
  config;
  /** The raw plugin config, re-resolved before activation; use {@link rawConfig} to read or retain it. */
  _config;
  /** Current lifecycle state; transitions emit `internal/status`. */
  state = 0;
  /** Dispose this fiber: unload the plugin, then settle once cleanup finished. */
  dispose;
  /** Snapshot of required service implementations while loaded; `undefined` otherwise. */
  store;
  /** The in-flight load/unload transition, if one is currently running. */
  inertia;
  _hooks = /* @__PURE__ */ Object.create(null);
  _disposables = new DisposableList();
  context;
  _error;
  _runner;
  _store = /* @__PURE__ */ Object.create(null);
  /**
  * Create a fiber. Plugin authors normally obtain fibers from `ctx.plugin()`
  * rather than constructing them directly.
  *
  * @param parent — the context the plugin was loaded from.
  * @param config — raw config, validated against the runtime's schema.
  * @param inject — resolved dependency map (service name → intercept config).
  * @param runtime — the shared plugin runtime, or `null` for the root fiber.
  * @param getOuterStack — captures the caller stack for effect diagnostics.
  */
  constructor(parent, config, inject2, runtime, getOuterStack) {
    this.parent = parent;
    this.inject = inject2;
    this.runtime = runtime;
    this._config = config;
    const collect = (dispose) => {
      this._disposables.push(dispose);
    };
    if (runtime) {
      this.uid = parent.registry.counter;
      this.ctx = this.context = parent.extend({ fiber: this });
      const injectEntries = Object.entries(this.inject);
      if (injectEntries.length) {
        this.ctx[Context.intercept] = Object.create(parent[Context.intercept]);
        for (const [name2, config2] of injectEntries) {
          if (isNullable(config2)) continue;
          this.ctx[Context.intercept][name2] = config2;
        }
      }
      this._runner = {
        epoch: INACTIVE,
        getOuterStack,
        execute: function() {
          if (isConstructor(runtime.callback)) {
            const instance = new runtime.callback(this.ctx, this.config);
            for (const hook of instance?.[symbols.initHooks] ?? []) hook();
            return instance?.[symbols.init]?.();
          } else return runtime.callback(this.ctx, this.config);
        },
        collect
      };
      this.dispose = parent.fiber.effect(() => {
        const remove = runtime.fibers.push(this);
        return async () => {
          this.uid = null;
          emitPluginDisposed(this.context, this);
          if (this.ctx.registry.has(runtime.callback)) {
            remove();
            if (!runtime.fibers.length) this.ctx.registry.delete(runtime.callback);
          }
          this._setEpoch(INACTIVE);
          if (!this.inertia) this._updateState(() => {
            this.inertia = this._unload();
            return 5;
          });
          while (this.inertia) await this.inertia;
        };
      }, "ctx.plugin()");
      try {
        this.context.emit("internal/plugin", this);
      } catch (error) {
        Promise.resolve(this.dispose()).catch((reason3) => this.ctx.logger.error(reason3));
        throw error;
      }
      if (this.uid !== null && parent.fiber.state !== 5) {
        for (const name2 of Object.keys(this.inject)) this._checkImpl(name2);
        this._refresh();
      }
    } else {
      this.uid = 0;
      this.ctx = this.context = parent;
      this.state = 2;
      this.store = /* @__PURE__ */ Object.create(null);
      this._runner = {
        epoch: "",
        getOuterStack,
        execute: () => {
        },
        collect
      };
      this.dispose = () => this.restart();
    }
  }
  /** The plugin's display name, inherited from the nearest named ancestor, else `'root'`. */
  get name() {
    let fiber = this;
    do {
      if (fiber.runtime?.name) return fiber.runtime.name;
      fiber = fiber.parent.fiber;
    } while (fiber !== fiber.parent.fiber);
    return "root";
  }
  /**
  * Throw if the fiber has already been disposed.
  *
  * @returns nothing when the fiber is still active.
  * @throws {CordisError} `INACTIVE_EFFECT` when the fiber's uid has been cleared.
  */
  assertActive() {
    if (this.uid !== null) return;
    throw new CordisError("INACTIVE_EFFECT");
  }
  _execute(runner) {
    const oldEpoch = runner.epoch;
    return composeError((info) => {
      const safeCollect = (dispose) => {
        if (typeof dispose === "function") runner.collect(dispose);
        else if (!isNullable(dispose)) throw new TypeError("Invalid effect");
      };
      const effect = runner.execute.call(this);
      if (typeof effect === "function") return runner.collect(effect);
      else if (isNullable(effect)) {
      } else if (!isObject(effect)) throw new TypeError("Invalid effect");
      else if ("then" in effect) return effect.then(safeCollect);
      else if (Symbol.iterator in effect) {
        info.error = /* @__PURE__ */ new Error();
        const iter = effect[Symbol.iterator]();
        while (true) {
          const result = iter.next();
          safeCollect(result.value);
          if (result.done) return;
        }
      } else if (Symbol.asyncIterator in effect) {
        const iter = effect[Symbol.asyncIterator]();
        return (async () => {
          await Promise.resolve();
          info.error = /* @__PURE__ */ new Error();
          while (true) {
            if (runner.epoch !== oldEpoch) return;
            const result = await iter.next();
            safeCollect(result.value);
            if (result.done) return;
          }
        })();
      } else throw new TypeError("Invalid effect");
    }, runner.getOuterStack);
  }
  effect(execute, label = "anonymous") {
    this.assertActive();
    if (this.state === 5) throw new CordisError("INACTIVE_EFFECT");
    const disposables = [];
    let disposing = false;
    let disposalTask;
    const dispose = () => {
      if (disposing) return disposalTask;
      disposing = true;
      let task2;
      for (const disposable of disposables.splice(0).reverse()) if (task2) task2 = task2.then(() => runDisposable(disposable));
      else {
        const result = runDisposable(disposable);
        if (isObject(result) && "then" in result) task2 = result;
      }
      return disposalTask = task2;
    };
    const meta = {
      label,
      children: []
    };
    const runner = {
      execute,
      epoch: true,
      collect: (dispose2) => {
        disposables.push(dispose2);
        this._disposables.delete(dispose2);
        if (dispose2[symbols.effect]) meta.children.push(dispose2[symbols.effect]);
      },
      getOuterStack: buildOuterStack()
    };
    let task;
    let executing = true;
    let resolveSetup;
    let rejectSetup;
    let setupBarrier;
    let setupFailed = false;
    let inFlight;
    let removeWrapper = () => false;
    const waitForSetup = () => {
      setupBarrier ??= new Promise((resolve, reject) => {
        resolveSetup = resolve;
        rejectSetup = reject;
      });
      return setupBarrier;
    };
    const disposeAfter = (setup) => {
      return Promise.resolve(setup).then(() => dispose(), async (reason3) => {
        await dispose();
        throw reason3;
      });
    };
    const finalizeDisposal = (callback) => {
      let result;
      try {
        result = callback();
      } catch (error) {
        removeWrapper();
        throw error;
      }
      if (isObject(result) && "then" in result) {
        const pending = Promise.resolve(result).finally(() => {
          removeWrapper();
          if (inFlight === pending) inFlight = void 0;
        });
        return inFlight = pending;
      }
      removeWrapper();
      return result;
    };
    const wrapper = defineProperty(() => {
      if (!runner.epoch) return setupFailed ? inFlight : void 0;
      runner.epoch = false;
      return finalizeDisposal(() => {
        if (executing) return disposeAfter(waitForSetup());
        return task ? disposeAfter(task) : dispose();
      });
    }, symbols.effect, meta);
    effectInertia.set(wrapper, () => inFlight);
    removeWrapper = this._disposables.push(wrapper);
    try {
      task = this._execute(runner);
    } catch (reason3) {
      executing = false;
      setupFailed = true;
      runner.epoch = false;
      let cleanup;
      try {
        cleanup = finalizeDisposal(dispose);
      } finally {
        rejectSetup?.(reason3);
      }
      if (isObject(cleanup) && "then" in cleanup) cleanup.catch((error) => this.ctx.logger.error(error));
      throw reason3;
    }
    executing = false;
    if (setupBarrier) Promise.resolve(task).then(resolveSetup, rejectSetup);
    task?.catch(() => {
      if (!runner.epoch) return dispose();
      return finalizeDisposal(dispose);
    }).catch((error) => this.ctx.logger.error(error));
    const disposeAsync = () => {
      if (!runner.epoch) return;
      runner.epoch = false;
      return finalizeDisposal(dispose);
    };
    wrapper.then = async (onFulfilled, onRejected) => {
      return Promise.resolve(task).then(() => disposeAsync).then(onFulfilled, onRejected);
    };
    return wrapper;
  }
  /**
  * Return metadata for currently registered effects.
  *
  * @returns one {@link EffectMeta} tree per labeled live effect.
  */
  getEffects() {
    return [...this._disposables].map((dispose) => dispose[symbols.effect]).filter(Boolean);
  }
  _getState() {
    if (this.uid === null) return 4;
    if (this._error) return 3;
    if (this._runner.epoch !== INACTIVE) return 2;
    return 0;
  }
  _updateState(callback) {
    const oldState = this.state;
    this.state = callback() ?? this._getState();
    if (oldState === this.state) return;
    this.context.emit("internal/status", this, oldState);
    if (oldState !== 2 && this.state !== 2) return;
    for (const key of Reflect.ownKeys(this.ctx.reflect.store)) {
      const impl = this.ctx.reflect.store[key];
      if (impl.fiber !== this) continue;
      this.ctx.reflect.notify([impl.name]);
    }
  }
  _checkImpl(name2) {
    const impl = this.ctx.reflect._getImpl(name2, true);
    if (!impl) return delete this._store[name2];
    try {
      if (impl.check && !impl.check.call(getTraceable(this.ctx, impl.value))) return delete this._store[name2];
    } catch (error) {
      impl.fiber.ctx.logger.error(error);
      return delete this._store[name2];
    }
    this._store[name2] = impl;
  }
  _refresh() {
    let epoch = false;
    epoch = "";
    for (const name2 of Object.keys(this.inject)) {
      const impl = this._store[name2];
      if (!impl) {
        epoch = INACTIVE;
        break;
      }
      epoch += ":" + impl.fiber.uid;
    }
    this._setEpoch(epoch);
  }
  _setEpoch(epoch) {
    const oldEpoch = this._runner.epoch;
    if (epoch === oldEpoch) return;
    this._runner.epoch = epoch;
    if (this.inertia) return;
    this._updateState(() => {
      if (epoch !== INACTIVE && oldEpoch === INACTIVE) {
        this.inertia = this._reload();
        return 1;
      } else {
        this.inertia = this._unload();
        return 5;
      }
    });
  }
  /** Raw input retained for the next activation, without evaluating expressions or validating it. */
  get rawConfig() {
    return this.ctx.fiber._config;
  }
  /** Retain raw input without changing the running config or triggering an update. */
  set rawConfig(config) {
    this.ctx.fiber._config = config;
  }
  /**
  * The config object published by the existing activation/update lifecycle, without cloning or re-parsing.
  * It is undefined before the first resolution and remains readable after unload; callers requiring a live instance check `state`.
  */
  get effectiveConfig() {
    return this.ctx.fiber.config;
  }
  /**
  * Evaluate and validate a candidate using this fiber's `internal/config` hooks and runtime schema.
  * Does not publish the result, replace raw input, or commit running volatile references.
  * Required injections must be available when a hook uses them.
  * @param config Raw candidate input.
  * @returns The candidate's resolved config, retaining the schema's result identity.
  * @throws {ValidationError} when the runtime schema rejects the candidate; hook errors propagate.
  */
  resolveConfig(config) {
    const fiber = this.ctx.fiber;
    config = fiber.context.waterfall(fiber, "internal/config", config, () => config);
    return fiber.runtime ? resolveConfig(fiber.runtime, config) : config;
  }
  async _reload() {
    this.store = { ...this._store };
    const oldEpoch = this._runner.epoch;
    try {
      await Promise.resolve();
      if (this._runner.epoch === oldEpoch) {
        this.config = this.resolveConfig(this._config);
        await this._execute(this._runner);
        this._error = void 0;
      }
    } catch (reason3) {
      this.ctx.logger.error(reason3);
      this._error = reason3;
      this._runner.epoch = INACTIVE;
    }
    this._updateState(() => {
      if (this._runner.epoch === oldEpoch) this.inertia = void 0;
      else {
        this.inertia = this._unload();
        return 5;
      }
    });
  }
  async _unload() {
    await Promise.all(this._disposables.clear().map(async (dispose) => {
      try {
        await composeError(async (info) => {
          await Promise.resolve();
          info.error = /* @__PURE__ */ new Error();
          await runDisposable(dispose);
        }, this._runner.getOuterStack);
      } catch (reason3) {
        this.ctx.logger.error(reason3);
      }
    }));
    this.store = void 0;
    this._updateState(() => {
      if (this._runner.epoch === INACTIVE) this.inertia = void 0;
      else {
        this.inertia = this._reload();
        return 1;
      }
    });
  }
  /**
  * Wait for current lifecycle work and rethrow startup errors.
  *
  * @returns this fiber, once it has settled into a stable state.
  * @throws the config-validation or plugin-startup error, if any.
  */
  async await() {
    while (this.inertia) await this.inertia;
    if (this._error) throw this._error;
    return this;
  }
  /**
  * Dispose and immediately reload this plugin with its current config.
  *
  * @returns a promise resolving once the reload settled.
  * @throws {CordisError} `INACTIVE_EFFECT` when the fiber is already disposed.
  */
  async restart() {
    const fiber = this.ctx.fiber;
    fiber.assertActive();
    fiber._setEpoch(INACTIVE);
    fiber._refresh();
    await fiber.await();
  }
  /**
  * Validate and apply new config, then restart the plugin.
  *
  * Runs the `internal/update` waterfall first, so update hooks (and HMR)
  * can veto or replace the restart.
  *
  * @param config — the new raw config; validated before anything restarts.
  * @param noSave — hint for persistence hooks not to write the change back.
  * @returns nothing; the restart runs behind the `internal/update` waterfall.
  * @throws {ValidationError} when the new config fails validation.
  */
  update(config, noSave = false) {
    const fiber = this.ctx.fiber;
    fiber.assertActive();
    fiber._config = config;
    if (fiber.state !== 2) {
      fiber._error = void 0;
      fiber._setEpoch(INACTIVE);
      fiber._refresh();
      return;
    }
    config = fiber.resolveConfig(config);
    fiber.context.waterfall(fiber, "internal/update", config, noSave, () => {
      fiber.config = config;
      fiber._error = void 0;
      return fiber.restart();
    });
  }
};
function isApplicable(object) {
  return object && typeof object === "object" && typeof object.apply === "function";
}
function Inject(name2, config) {
  return function(value, decorator) {
    if (decorator.kind === "class") {
      if (!Object.hasOwn(value, "inject")) {
        defineProperty(value, "inject", Object.create(Object.getPrototypeOf(value).inject ?? null));
        defineProperty(value.inject, symbols.checkProto, true);
      }
      value.inject[name2] = config;
    } else if (decorator.kind === "method") {
      const inject2 = (value[symbols.metadata] ??= {}).inject ??= /* @__PURE__ */ Object.create(null);
      inject2[name2] = config;
      decorator.addInitializer(function() {
        const property = this[symbols.tracker]?.property;
        (this[symbols.initHooks] ??= []).push(() => {
          this.ctx.inject(inject2, (ctx) => {
            return value.call(property ? withProps(this, { [property]: ctx }) : this);
          });
        });
      });
    } else throw new Error("@Inject() can only be used on class or class methods");
  };
}
(function(Inject2) {
  function resolve(inject2, result = /* @__PURE__ */ Object.create(null)) {
    if (!inject2) return result;
    if (Array.isArray(inject2)) for (const name2 of inject2) result[name2] = null;
    else if (Reflect.has(inject2, symbols.checkProto)) {
      Object.assign(result, resolve(Object.getPrototypeOf(inject2)));
      for (const name2 of Object.keys(inject2)) result[name2] = inject2[name2] ?? null;
    } else for (const name2 of Object.keys(inject2)) result[name2] = inject2[name2] ?? null;
    return result;
  }
  Inject2.resolve = resolve;
})(Inject || (Inject = {}));
var RegistryService = class {
  ctx;
  _counter = 0;
  _internal = /* @__PURE__ */ new Map();
  constructor(ctx) {
    this.ctx = ctx;
    defineProperty(this, symbols.tracker, {
      property: "ctx",
      noShadow: true
    });
  }
  /** Allocate the next fiber uid (increments on every read). */
  get counter() {
    return ++this._counter;
  }
  /** Number of registered plugin runtimes. */
  get size() {
    return this._internal.size;
  }
  /**
  * Resolve a supported plugin shape to its executable callback.
  *
  * @param plugin — a function, class, or `{ apply }` object plugin.
  * @returns the callback identifying the plugin, or `undefined` if invalid.
  */
  resolve(plugin) {
    try {
      if (typeof plugin === "function") return plugin;
      if (isApplicable(plugin)) return plugin.apply;
    } catch {
    }
  }
  /**
  * Look up the runtime record for a plugin.
  *
  * @param plugin — any supported plugin shape.
  * @returns the runtime, or `undefined` when the plugin is not registered.
  */
  get(plugin) {
    const key = this.resolve(plugin);
    return key && this._internal.get(key);
  }
  /**
  * Check whether a plugin has a registered runtime.
  *
  * @param plugin — any supported plugin shape.
  * @returns `true` when at least one fiber of the plugin exists.
  */
  has(plugin) {
    const key = this.resolve(plugin);
    return !!key && this._internal.has(key);
  }
  /**
  * Dispose every running fiber for a plugin and remove its runtime record.
  *
  * @param plugin — any supported plugin shape.
  * @returns the removed runtime, or `undefined` when none was registered.
  */
  delete(plugin) {
    const key = this.resolve(plugin);
    const runtime = key && this._internal.get(key);
    if (!runtime) return;
    this._internal.delete(key);
    for (const fiber of runtime.fibers) fiber.dispose();
    return runtime;
  }
  /** Iterate the registered plugin callbacks. */
  keys() {
    return this._internal.keys();
  }
  /** Iterate the registered plugin runtimes. */
  values() {
    return this._internal.values();
  }
  /** Iterate `[callback, runtime]` pairs. */
  entries() {
    return this._internal.entries();
  }
  /**
  * Visit every registered runtime.
  *
  * @param callback — receives each runtime and its identifying callback.
  */
  forEach(callback) {
    return this._internal.forEach(callback);
  }
  /**
  * Start a callback once the requested dependencies are available.
  *
  * @param inject — required services, as an array or a name → config map.
  * @param callback — plugin body called with `(ctx, config)`.
  * @returns the fiber; awaiting it settles once loading finished.
  */
  inject(inject2, callback) {
    return this.plugin({
      inject: inject2,
      apply: callback,
      name: callback.name
    });
  }
  /**
  * Start a plugin in the current context and return its fiber.
  *
  * Creates (or reuses) the plugin's runtime record, then starts a new fiber
  * under the current context. Throws if `plugin` is not a supported shape or
  * if the current fiber is already disposed.
  *
  * @param plugin — a function, class, or `{ apply }` object plugin.
  * @param config — the plugin config, validated against its `Config` schema.
  * @param getOuterStack — captures the caller stack for effect diagnostics.
  * @returns the fiber; awaiting it settles once loading finished.
  */
  plugin(plugin, config, getOuterStack = buildOuterStack()) {
    const callback = this.resolve(plugin);
    if (!callback) throw new Error('invalid plugin, expect function or object with an "apply" method, received ' + typeof plugin);
    this.ctx.fiber.assertActive();
    let runtime = this._internal.get(callback);
    if (!runtime) {
      let name2 = plugin.name;
      if (name2 === "apply") name2 = void 0;
      runtime = {
        name: name2,
        callback,
        fibers: new DisposableList(),
        Config: plugin.Config
      };
      this._internal.set(callback, runtime);
    }
    const fiber = new Fiber(this.ctx, config, Inject.resolve(plugin.inject), runtime, getOuterStack);
    const wrapped = Object.create(fiber);
    wrapped.then = (onFulfilled, onRejected) => {
      return fiber.await().then(onFulfilled, onRejected);
    };
    return wrapped;
  }
};
var Context = class Context2 {
  /** Symbol key under which a disposer exposes its {@link EffectMeta} diagnostics tree. */
  static effect = symbols.effect;
  /** Symbol key for a context's listener filter, consulted on every event dispatch. */
  static filter = symbols.filter;
  /** Symbol key of the isolation map (see the `Context[symbols.isolate]` property). */
  static isolate = symbols.isolate;
  /** Symbol key of the intercept map (see the `Context[symbols.intercept]` property). */
  static intercept = symbols.intercept;
  /**
  * Returns true for Cordis context proxies and context prototypes.
  *
  * Works across realms and across multiple copies of cordis, because the
  * brand is keyed by a global symbol rather than by `instanceof`.
  *
  * @param value — the value to test.
  * @returns `true` if `value` is a Cordis context, narrowing its type.
  */
  static is(value) {
    return !!value?.[Context2.is];
  }
  static {
    Context2.is[Symbol.toPrimitive] = () => Symbol.for("cordis.is");
    Context2.prototype[Context2.is] = true;
  }
  /** Create the root context and install the built-in services. */
  constructor() {
    this[symbols.isolate] = /* @__PURE__ */ Object.create(null);
    this[symbols.intercept] = /* @__PURE__ */ Object.create(null);
    const self = new Proxy(this, ReflectService.handler);
    this.root = self;
    this.baseUrl = void 0;
    this.fiber = new Fiber(self, {}, /* @__PURE__ */ Object.create(null), null, () => []);
    this.reflect = new ReflectService(self);
    this.registry = new RegistryService(self);
    this.events = new EventsService(self);
    this.logger = new LoggerService(self);
    this.fiber._disposables.clear();
    return self;
  }
  [Symbol.for("nodejs.util.inspect.custom")]() {
    return `Context <${this.fiber.name}>`;
  }
  /**
  * Create a child context with extra metadata on top of the current scope.
  *
  * The child prototypally inherits every property of this context; own
  * properties of `meta` shadow the inherited ones. The parent is not mutated.
  *
  * @param meta — own properties (including symbol keys) to define on the child.
  * @returns a child context inheriting from this one.
  */
  extend(meta = {}) {
    const shadow = Reflect.getOwnPropertyDescriptor(this, symbols.shadow)?.value;
    const self = Object.create(getTraceable(this, this));
    for (const prop of Reflect.ownKeys(meta)) Object.defineProperty(self, prop, Reflect.getOwnPropertyDescriptor(meta, prop));
    if (!shadow) return self;
    return Object.assign(Object.create(self), { [symbols.shadow]: shadow });
  }
  /**
  * Create a child context with an independent service scope for `name`.
  *
  * Below the returned context, reads and writes of the service `name`
  * resolve against the new label instead of the parent's, so a different
  * implementation can be provided without affecting the parent scope.
  * Passing the same `label` to two `isolate()` calls joins their scopes.
  *
  * @param name — the service name to isolate.
  * @param label — scope label to join; defaults to a fresh unique symbol.
  * @returns a child context whose `name` service resolves in the new scope.
  */
  isolate(name2, label) {
    const shadow = Object.create(this[symbols.isolate]);
    shadow[name2] = label ?? Symbol(name2);
    return this.extend({ [symbols.isolate]: shadow });
  }
  intercept(name2, config) {
    const intercept = Object.create(this[symbols.intercept]);
    intercept[name2] = config;
    return this.extend({ [symbols.intercept]: intercept });
  }
};
var Service = class Service2 {
  ctx;
  /** Symbol key of an instance method run after construction (class plugins). */
  static init = symbols.init;
  /** Symbol key of the availability predicate passed to `ctx.provide()`. */
  static check = symbols.check;
  /** Symbol key of the phantom intercept-config type parameter. */
  static config = symbols.config;
  /** Symbol key of the call body making a service callable (e.g. `ctx.logger()`). */
  static invoke = symbols.invoke;
  /** Symbol key of the helper deriving an extended service instance. */
  static extend = symbols.extend;
  /** Symbol key of the tracker metadata used for context tracing. */
  static tracker = symbols.tracker;
  /** Symbol key of the intercept-config resolution helper below. */
  static resolveConfig = symbols.resolveConfig;
  /** The service name this instance is registered under. */
  name;
  /**
  * Register this instance as `name` in the current context.
  *
  * Calls `ctx.reflect.provide(name, this, this[Service.check])`, so the
  * service is unregistered automatically when the owning fiber unloads.
  * Services with a `[Service.invoke]` body return a callable instance.
  *
  * @param ctx — the context to register in (stored as `this.ctx`).
  * @param name — the service name; defaults to the static `provide` field.
  */
  constructor(ctx, name2) {
    this.ctx = ctx;
    name2 ??= this.constructor["provide"];
    let self = this;
    const tracker = {
      associate: name2,
      property: "ctx"
    };
    if (self[symbols.invoke]) self = createCallable(name2, joinPrototype(Object.getPrototypeOf(this), Function.prototype), tracker);
    self.ctx = ctx;
    self.name = name2;
    defineProperty(self, symbols.tracker, tracker);
    self.ctx.reflect.provide(name2, self, this[symbols.check]);
    return self;
  }
  [symbols.filter](ctx) {
    return ctx[symbols.isolate][this.name] === this.ctx[symbols.isolate][this.name];
  }
  [symbols.extend](props) {
    let self;
    if (this[Service2.invoke]) self = createCallable(this.name, this, this[symbols.tracker]);
    else self = Object.create(this);
    return Object.assign(self, props);
  }
  /**
  * Merge intercept config from ancestors with optional base and head values.
  *
  * Entries added closer to the root apply first; `base` is prepended and
  * `head` appended. Uses `Config.merge` when the service declares one,
  * otherwise a shallow `Object.assign`.
  *
  * @param base — lowest-precedence config merged before all intercepts.
  * @param head — highest-precedence config merged after all intercepts.
  * @returns the merged config.
  */
  [symbols.resolveConfig](base, head) {
    let intercept = this.ctx[Context.intercept];
    const configs = [];
    while (this.name in intercept) {
      if (Object.hasOwn(intercept, this.name)) configs.unshift(intercept[this.name]);
      intercept = Object.getPrototypeOf(intercept);
    }
    if (base) configs.unshift(base);
    if (head) configs.push(head);
    if (this["Config"]?.merge) return this["Config"].merge(...configs);
    else return Object.assign({}, ...configs);
  }
  static [Symbol.hasInstance](instance) {
    if (!instance) return false;
    let constructor = instance.constructor;
    while (constructor) {
      constructor = constructor.prototype?.constructor;
      if (constructor === this) return true;
      constructor &&= Object.getPrototypeOf(constructor);
    }
    return false;
  }
};

// node_modules/@deepseek-ai/dsh-typert-protocol/lib/index.js
var TYPERT_OWNED_VALUE = Symbol.for("dsh.typert.owned-value");
var TYPERT_REMOTE_SEGMENT_PATTERN = /^[A-Za-z0-9_$.-]+$/;
function isTypertRemoteSegment(value) {
  return value !== "." && value !== ".." && TYPERT_REMOTE_SEGMENT_PATTERN.test(value);
}
var REMOTE_METHOD_DESCRIPTOR = "@deepseek-ai/dsh-typert-protocol/remote-methods";
function bindTypertRemote(service, serviceKey, options = {}) {
  validateName("service key", serviceKey);
  const namespace = options.namespace ?? serviceKey;
  validateName("namespace", namespace);
  const ctx = Reflect.get(service, "ctx");
  if (Context.is(ctx)) provideInvocationAccessor(ctx);
  return Object.freeze({
    service,
    serviceKey,
    namespace
  });
}
var TypertRemoteService = class extends Service {
  /** Visible binding consumed by the Gateway's source-mode discovery. */
  typertRemote;
  /**
  * Register the Service and bind the same key to Typert Gateway.
  * @param ctx - owning Cordis Context.
  * @param serviceKey - exact Cordis service key and default wire namespace.
  * @param options - optional wire namespace.
  */
  constructor(ctx, serviceKey, options = {}) {
    super(ctx, serviceKey);
    this.typertRemote = bindTypertRemote(this, this.name, options);
  }
};
function provideInvocationAccessor(ctx) {
  if (Object.hasOwn(ctx.root.reflect.props, "invocation")) return;
  ctx.root.accessor("invocation", { get: () => void 0 });
}
function Remote(methodExportOrOptions, context) {
  if (typeof methodExportOrOptions === "string") {
    validateName("Remote export name", methodExportOrOptions);
    return remoteDecorator({ kind: "direct" }, { exportName: methodExportOrOptions });
  }
  if (typeof methodExportOrOptions === "object") return remoteDecorator({ kind: "direct" }, markerOptions(methodExportOrOptions, REMOTE_OPTION_KEYS, 'Remote options require name, mode: "stream", or resources'));
  if (context === void 0) throw new TypeError("typert-protocol: Remote decorator context is missing");
  addMarkerInitializer(context, { kind: "direct" });
}
var REMOTE_OPTION_KEYS = /* @__PURE__ */ new Set([
  "name",
  "mode",
  "resources"
]);
function markerOptions(options, accepted, requirement) {
  const keys = Reflect.ownKeys(options);
  if (keys.length === 0 || keys.some((key) => typeof key !== "string" || !accepted.has(key)) || Object.hasOwn(options, "mode") && options.mode !== "stream") throw new TypeError(`typert-protocol: ${requirement}`);
  const { mode, name: name2, resources } = options;
  if (name2 !== void 0) validateName("Remote export name", name2);
  return {
    ...mode === void 0 ? {} : { mode },
    ...name2 === void 0 ? {} : { exportName: name2 },
    ...resources === void 0 ? {} : { resources: resourceReaders(resources) }
  };
}
function resourceReaders(resources) {
  const readers = {};
  for (const [kind, reader] of Object.entries(resources)) {
    if (typeof reader !== "function") throw new TypeError(`typert-protocol: Remote resource reader "${kind}" must be a function`);
    readers[kind] = reader;
  }
  if (Object.keys(readers).length === 0) throw new TypeError("typert-protocol: Remote resources require at least one reader");
  return Object.freeze(readers);
}
function remoteDecorator(invocation, options = {}) {
  return function(_method, context) {
    addMarkerInitializer(context, invocation, options);
  };
}
function readRemoteMethodDescriptor(prototype) {
  const property = Object.getOwnPropertyDescriptor(prototype, REMOTE_METHOD_DESCRIPTOR);
  if (property === void 0) return void 0;
  const descriptor = property.value;
  if (descriptor === null || typeof descriptor !== "object") throw new TypeError("typert-protocol: Remote method descriptor must be an object");
  const version = Reflect.get(descriptor, "version");
  if (version !== 1) throw new TypeError(`typert-protocol: unsupported Remote method descriptor version ${String(version)}`);
  const methods = Reflect.get(descriptor, "methods");
  if (!Array.isArray(methods)) throw new TypeError("typert-protocol: Remote method descriptor methods must be an array");
  return descriptor;
}
function addMarkerInitializer(context, invocation, options = {}) {
  if (context.private || context.static || typeof context.name !== "string") throw new TypeError("typert-protocol: Remote decorators require a public instance method with a string name");
  const method = context.name;
  context.addInitializer(function() {
    const prototype = Object.getPrototypeOf(this);
    if (prototype === null) throw new TypeError(`typert-protocol: cannot mark Remote method "${method}" on an object without a prototype`);
    mark(prototype, method, invocation, options);
  });
}
function mark(prototype, method, invocation, { mode, exportName, resources }) {
  const descriptor = readRemoteMethodDescriptor(prototype);
  const marker = Object.freeze({
    method,
    ...exportName === void 0 || exportName === method ? {} : { exportName },
    ...mode === void 0 ? {} : { mode },
    invocation: Object.freeze(invocation),
    ...resources === void 0 ? {} : { resources }
  });
  const current = descriptor?.methods.find((candidate) => candidate.method === method);
  if (current !== void 0) {
    if (current.exportName === marker.exportName && current.mode === marker.mode && current.resources === marker.resources && sameInvocation(current.invocation, invocation)) return;
    throw new Error(`typert-protocol: Remote method "${method}" has conflicting invocation markers`);
  }
  Object.defineProperty(prototype, REMOTE_METHOD_DESCRIPTOR, {
    configurable: true,
    value: Object.freeze({
      version: 1,
      methods: Object.freeze([...descriptor?.methods ?? [], marker])
    })
  });
}
function sameInvocation(left, right) {
  if (left.kind === "direct") return right.kind === "direct";
  if (right.kind === "direct") return false;
  return left.context === right.context;
}
function validateName(subject, value) {
  if (!isTypertRemoteSegment(value)) throw new TypeError(`typert-protocol: ${subject} must contain only RPC endpoint segment characters`);
}

// src/brand.ts
var _vote_dec, _writeTableRow_dec, _insertFormRow_dec, _webLink_dec, _updatePlugin_dec, _saveServer_dec, _testServer_dec, _serverSettings_dec, _doctor_dec, _login_dec, _overview_dec, _turnWrites_dec, _kbDoc_dec, _setChecklist_dec, _checklists_dec, _board_dec, _workbenchData_dec, _unbindApp_dec, _bindApp_dec, _appBindings_dec, _setSubscription_dec, _center_dec, _disableApp_dec, _enableApp_dec, _availableApps_dec, _apps_dec, _brand_dec, _a, _init;
var DshEEClientService = class extends (_a = TypertRemoteService, _brand_dec = [Remote], _apps_dec = [Remote], _availableApps_dec = [Remote], _enableApp_dec = [Remote], _disableApp_dec = [Remote], _center_dec = [Remote], _setSubscription_dec = [Remote], _appBindings_dec = [Remote], _bindApp_dec = [Remote], _unbindApp_dec = [Remote], _workbenchData_dec = [Remote], _board_dec = [Remote], _checklists_dec = [Remote], _setChecklist_dec = [Remote], _kbDoc_dec = [Remote], _turnWrites_dec = [Remote], _overview_dec = [Remote], _login_dec = [Remote], _doctor_dec = [Remote], _serverSettings_dec = [Remote], _testServer_dec = [Remote], _saveServer_dec = [Remote], _updatePlugin_dec = [Remote], _webLink_dec = [Remote], _insertFormRow_dec = [Remote], _writeTableRow_dec = [Remote], _vote_dec = [Remote], _a) {
  constructor(ctx, actions) {
    super(ctx, "dshEE");
    this.actions = actions;
    __runInitializers(_init, 5, this);
  }
  async brand() {
    return this.actions.readBrand();
  }
  async apps() {
    return this.actions.readApps();
  }
  async availableApps() {
    return this.actions.readAvailableApps();
  }
  async enableApp(request) {
    return this.actions.setAppSubscription(request?.name, true);
  }
  async disableApp(request) {
    return this.actions.setAppSubscription(request?.name, false);
  }
  async center() {
    return this.actions.readCenter();
  }
  async setSubscription(request) {
    return this.actions.setSubscription(request ?? {});
  }
  async appBindings() {
    return this.actions.readAppBindings();
  }
  async bindApp(request) {
    return this.actions.bindApp(request.name, request.sessionId);
  }
  async unbindApp(request) {
    return this.actions.unbindApp(request.name);
  }
  async workbenchData(request) {
    return this.actions.workbenchData(request);
  }
  async board(request) {
    return this.actions.readBoard(request);
  }
  async checklists(request) {
    return this.actions.readChecklists(request);
  }
  async setChecklist(request) {
    return this.actions.setChecklist(request);
  }
  async kbDoc(request) {
    return this.actions.readKBDoc(request);
  }
  async turnWrites(request) {
    return this.actions.turnWrites(request);
  }
  async overview() {
    return this.actions.readOverview();
  }
  async login() {
    return this.actions.login();
  }
  async doctor() {
    return this.actions.doctor();
  }
  async serverSettings() {
    return this.actions.serverSettings();
  }
  async testServer(request) {
    return this.actions.testServer(request ?? {});
  }
  async saveServer(request) {
    return this.actions.saveServer(request ?? {});
  }
  async updatePlugin() {
    return this.actions.updatePlugin();
  }
  async webLink(request) {
    return this.actions.webLink(request ?? {});
  }
  async insertFormRow(request) {
    return this.actions.insertFormRow(request);
  }
  async writeTableRow(request) {
    return this.actions.writeTableRow(request);
  }
  async vote(request) {
    return this.actions.vote(request);
  }
};
_init = __decoratorStart(_a);
__decorateElement(_init, 1, "brand", _brand_dec, DshEEClientService);
__decorateElement(_init, 1, "apps", _apps_dec, DshEEClientService);
__decorateElement(_init, 1, "availableApps", _availableApps_dec, DshEEClientService);
__decorateElement(_init, 1, "enableApp", _enableApp_dec, DshEEClientService);
__decorateElement(_init, 1, "disableApp", _disableApp_dec, DshEEClientService);
__decorateElement(_init, 1, "center", _center_dec, DshEEClientService);
__decorateElement(_init, 1, "setSubscription", _setSubscription_dec, DshEEClientService);
__decorateElement(_init, 1, "appBindings", _appBindings_dec, DshEEClientService);
__decorateElement(_init, 1, "bindApp", _bindApp_dec, DshEEClientService);
__decorateElement(_init, 1, "unbindApp", _unbindApp_dec, DshEEClientService);
__decorateElement(_init, 1, "workbenchData", _workbenchData_dec, DshEEClientService);
__decorateElement(_init, 1, "board", _board_dec, DshEEClientService);
__decorateElement(_init, 1, "checklists", _checklists_dec, DshEEClientService);
__decorateElement(_init, 1, "setChecklist", _setChecklist_dec, DshEEClientService);
__decorateElement(_init, 1, "kbDoc", _kbDoc_dec, DshEEClientService);
__decorateElement(_init, 1, "turnWrites", _turnWrites_dec, DshEEClientService);
__decorateElement(_init, 1, "overview", _overview_dec, DshEEClientService);
__decorateElement(_init, 1, "login", _login_dec, DshEEClientService);
__decorateElement(_init, 1, "doctor", _doctor_dec, DshEEClientService);
__decorateElement(_init, 1, "serverSettings", _serverSettings_dec, DshEEClientService);
__decorateElement(_init, 1, "testServer", _testServer_dec, DshEEClientService);
__decorateElement(_init, 1, "saveServer", _saveServer_dec, DshEEClientService);
__decorateElement(_init, 1, "updatePlugin", _updatePlugin_dec, DshEEClientService);
__decorateElement(_init, 1, "webLink", _webLink_dec, DshEEClientService);
__decorateElement(_init, 1, "insertFormRow", _insertFormRow_dec, DshEEClientService);
__decorateElement(_init, 1, "writeTableRow", _writeTableRow_dec, DshEEClientService);
__decorateElement(_init, 1, "vote", _vote_dec, DshEEClientService);
__decoratorMetadata(_init, DshEEClientService);
function registerBrand(ctx, actions) {
  return new DshEEClientService(ctx, actions);
}

// src/overview.ts
function overviewFailure(config, error) {
  const err = error;
  const message = String(err?.message ?? error);
  if (message === "HTTP 401") return { reason: "auth", base: config.base, detail: message };
  if (/^HTTP \d+$/.test(message)) return { reason: "server", base: config.base, detail: message };
  const detail = err?.name === "TimeoutError" ? t("overview.timeout") : err?.cause?.code || err?.cause?.message || message;
  return { reason: "unreachable", base: config.base, detail };
}
async function bridgeJson(bridge, name2, args) {
  const result = await bridge.callTool(name2, args);
  if (result?.isError) throw new Error(t("overview.toolFailed"));
  if (result?.structuredContent) return result.structuredContent;
  const text = result?.content?.find((part) => part.type === "text")?.text;
  if (typeof text !== "string") throw new Error(t("overview.toolNoJson"));
  return JSON.parse(text);
}
async function readEnterpriseOverview(config, http, bridge) {
  const me = await http.json("/api/dsh/whoami");
  const [usage, modelList, doctor] = await Promise.all([
    bridgeJson(bridge, "ee_me", { action: "usage" }).catch(() => void 0),
    http.json("/v1/models").catch(() => void 0),
    bridgeJson(bridge, "ee_doctor", { action: "check" }).catch(() => void 0)
  ]);
  return {
    me,
    usage,
    models: modelList?.data?.map((model) => model.id) ?? [],
    default_model: doctor?.default_model,
    version: config.version,
    latest_version: doctor?.connect_version
  };
}

// src/server-settings.ts
import fs3 from "node:fs";
import path4 from "node:path";
function serverSettingsPath() {
  return path4.join(dshHome(), "dsh-ee", "server.json");
}
function normalizeBase(input) {
  const raw = String(input ?? "").trim().replace(/\/+$/, "");
  try {
    const url = new URL(raw);
    if (url.protocol !== "http:" && url.protocol !== "https:" || !url.host || url.username || url.password || url.search || url.hash) return void 0;
    return raw;
  } catch {
    return void 0;
  }
}
function readBaseOverride() {
  try {
    const data = JSON.parse(fs3.readFileSync(serverSettingsPath(), "utf8"));
    return normalizeBase(data.base);
  } catch {
    return void 0;
  }
}
function writeBaseOverride(base) {
  const file = serverSettingsPath();
  if (!base) {
    fs3.rmSync(file, { force: true });
    return;
  }
  fs3.mkdirSync(path4.dirname(file), { recursive: true, mode: 448 });
  fs3.writeFileSync(file, `${JSON.stringify({ base }, null, 2)}
`, { mode: 384 });
}
function reason(error) {
  const err = error;
  if (err?.name === "TimeoutError") return t("serverSettings.reasonTimeout");
  const code = err?.cause?.code;
  if (code === "ECONNREFUSED") return t("serverSettings.reasonRefused");
  if (code === "ENOTFOUND" || code === "EAI_AGAIN") return t("serverSettings.reasonDns");
  if (code === "CERT_HAS_EXPIRED" || code?.startsWith?.("ERR_TLS") || code?.includes?.("CERT")) return t("serverSettings.reasonTls", { code });
  return code || err?.cause?.message || err?.message || String(error);
}
var ServerSettingsService = class {
  constructor(config, tokens, onChange, timeoutMs = 8e3) {
    this.config = config;
    this.tokens = tokens;
    this.onChange = onChange;
    this.timeoutMs = timeoutMs;
    this.packaged = config.base;
    const override = readBaseOverride();
    if (override) config.base = override;
  }
  packaged;
  read() {
    return { base: this.config.base, packaged: this.packaged, overridden: this.config.base !== this.packaged, org: this.config.org, version: this.config.version };
  }
  /** 按顺序测：地址格式、能不能连上、是不是 dsh-ee、本机的登录在这台 dsh-ee 上还有没有效。 */
  async test(input) {
    const base = input === void 0 || input === "" ? this.config.base : normalizeBase(input);
    const checks = [];
    const done = (b) => ({ base: b, ok: checks.every((c) => c.ok), checks });
    if (!base) {
      checks.push({ id: "address", ok: false, title: t("serverSettings.badAddress"), detail: t("serverSettings.badAddressDetail") });
      return done(String(input ?? ""));
    }
    const started = Date.now();
    let health;
    try {
      health = await fetch(`${base}/healthz`, { signal: AbortSignal.timeout(this.timeoutMs) });
    } catch (error) {
      checks.push({ id: "reach", ok: false, title: t("serverSettings.unreachable"), detail: reason(error) });
      return done(base);
    }
    checks.push({ id: "reach", ok: true, title: t("serverSettings.reachable", { ms: Date.now() - started }) });
    const body = await health.json().catch(() => null);
    if (!health.ok || body?.status !== "ok") {
      checks.push({ id: "server", ok: false, title: t("serverSettings.notDshEe"), detail: t("serverSettings.notDshEeDetail", { status: health.status }) });
      return done(base);
    }
    checks.push({ id: "server", ok: true, title: t("serverSettings.isDshEe", { version: body.version ? t("serverSettings.versionSuffix", { version: body.version }) : "" }) });
    const current = await this.tokens.resolve();
    if (!current?.value) {
      checks.push({ id: "login", ok: false, title: t("serverSettings.noLogin"), detail: t("serverSettings.noLoginDetail") });
      return done(base);
    }
    try {
      const me = await fetch(`${base}/api/dsh/whoami`, { headers: { Authorization: `Bearer ${current.value}` }, signal: AbortSignal.timeout(this.timeoutMs) });
      if (me.ok) {
        const who = await me.json();
        checks.push({ id: "login", ok: true, title: t("serverSettings.signedIn", { username: who.username ?? "", department: who.department ? t("serverSettings.departmentSuffix", { department: who.department }) : "" }) });
      } else {
        checks.push({ id: "login", ok: false, title: t("serverSettings.loginInvalid"), detail: t("serverSettings.loginInvalidDetail") });
      }
    } catch (error) {
      checks.push({ id: "login", ok: false, title: t("serverSettings.loginCheckFailed"), detail: reason(error) });
    }
    return done(base);
  }
  /** 保存地址并立即生效；传空或与插件包里的一样就恢复默认。 */
  async save(input) {
    const base = input === void 0 || input === null || input === "" ? this.packaged : normalizeBase(input);
    if (!base) throw new Error(t("serverSettings.saveBadAddress"));
    writeBaseOverride(base === this.packaged ? void 0 : base);
    if (base !== this.config.base) {
      this.config.base = base;
      await this.onChange();
    }
    return this.read();
  }
};

// src/connect-update.ts
var reason2 = (error) => error instanceof Error ? error.message : String(error);
async function updateConnect(ctx, config, http) {
  const manager = ctx.get("pluginManager");
  if (!manager || typeof manager.installBundle !== "function") {
    return { ok: false, version: config.version, message: t("connectUpdate.noManager") };
  }
  let info;
  try {
    info = await http.json("/api/connect");
  } catch (error) {
    const why = reason2(error);
    return { ok: false, version: config.version, message: t("connectUpdate.noLatest", { why, hint: why === "HTTP 401" ? t("connectUpdate.signInHint") : "" }) };
  }
  const latest = String(info?.version ?? "");
  const pkg = String(info?.package ?? "");
  if (!info?.ready || !latest || !pkg) {
    return { ok: false, version: config.version, message: info?.error || t("connectUpdate.notPublished") };
  }
  if (config.version !== "dev" && compareVersions(latest, config.version) <= 0) {
    return { ok: true, version: config.version, latest, package: pkg, message: t("connectUpdate.upToDate", { version: config.version }) };
  }
  const done = { ok: true, updated: true, version: config.version, latest, package: pkg, message: t("connectUpdate.installed", { pkg, latest }) };
  try {
    await manager.installBundle(info.tarball_url || `${config.base}/dl/connect/dsh-connect-${latest}.tgz`);
    return done;
  } catch (tarballError) {
    try {
      await manager.installBundle(`${pkg}@${latest}`, { registry: `${config.base}/npm/` });
      return done;
    } catch (registryError) {
      return { ok: false, version: config.version, latest, package: pkg, message: t("connectUpdate.installFailed", { error: reason2(registryError) || reason2(tarballError) }) };
    }
  }
}

// src/upload.ts
import fs4 from "node:fs";
import os3 from "node:os";
import path5 from "node:path";
var MAX_BYTES = 100 * 1024 * 1024;
function fail(text) {
  return { content: [{ type: "text", text }], isError: true };
}
async function uploadPackage(http, args) {
  const raw = String(args.path ?? "").trim();
  if (!raw) return fail(t("upload.missingPath"));
  const file = path5.resolve(raw.startsWith("~") ? path5.join(os3.homedir(), raw.slice(1)) : raw);
  if (!/\.(tgz|tar\.gz)$/i.test(file)) return fail(t("upload.notTarball"));
  let stat;
  try {
    stat = fs4.statSync(file);
  } catch {
    return fail(t("upload.fileNotFound", { file }));
  }
  if (!stat.isFile()) return fail(t("upload.notAFile", { file }));
  if (stat.size > MAX_BYTES) return fail(t("upload.tooLarge"));
  let response;
  try {
    response = await http.fetch("/api/agent/packages", {
      method: "POST",
      headers: { "content-type": "application/octet-stream" },
      body: fs4.readFileSync(file)
    }, 0);
  } catch (error) {
    if (error?.name === "TimeoutError") return fail(t("upload.timeout"));
    return fail(t("upload.unreachable"));
  }
  const body = await response.json().catch(() => null);
  if (!response.ok) return fail(body?.error ?? t("upload.httpFailed", { status: response.status }));
  return { content: [{ type: "text", text: JSON.stringify(body) }], isError: false };
}

// src/app-bindings.ts
import fs5 from "node:fs";
import path6 from "node:path";
var AppBindings = class {
  constructor(file = path6.join(dshHome(), "dsh-ee", "apps.json")) {
    this.file = file;
  }
  sequence = 0;
  warningMessage = "";
  list() {
    return this.read().bindings;
  }
  warning() {
    return this.warningMessage;
  }
  bind(name2, sessionId) {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name2) || name2.length > 48) throw new Error(t("appBindings.invalidAppName"));
    if (typeof sessionId !== "string" || !sessionId.trim() || sessionId.length > 256) throw new Error(t("appBindings.invalidSessionId"));
    const bindings = this.list();
    bindings[name2] = { sessionId: sessionId.trim(), updatedAt: (/* @__PURE__ */ new Date()).toISOString() };
    this.write(bindings);
    return bindings;
  }
  unbind(name2) {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name2) || name2.length > 48) throw new Error(t("appBindings.invalidAppName"));
    const bindings = this.list();
    delete bindings[name2];
    this.write(bindings);
    return bindings;
  }
  read() {
    this.warningMessage = "";
    if (!fs5.existsSync(this.file)) return { version: 1, bindings: {} };
    try {
      const value = JSON.parse(fs5.readFileSync(this.file, "utf8"));
      if (value?.version !== 1 || !value.bindings || typeof value.bindings !== "object" || Array.isArray(value.bindings)) throw new Error("invalid");
      const bindings = {};
      for (const [name2, binding] of Object.entries(value.bindings)) {
        const sessionId = binding?.sessionId;
        const updatedAt = binding?.updatedAt;
        if (typeof sessionId === "string" && sessionId && typeof updatedAt === "string") bindings[name2] = { sessionId, updatedAt };
      }
      return { version: 1, bindings };
    } catch {
      this.warningMessage = t("appBindings.corruptFile", { file: this.file });
      return { version: 1, bindings: {} };
    }
  }
  write(bindings) {
    fs5.mkdirSync(path6.dirname(this.file), { recursive: true, mode: 448 });
    const temp = `${this.file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs5.writeFileSync(temp, JSON.stringify({ version: 1, bindings }, null, 2) + "\n", { mode: 384 });
      fs5.renameSync(temp, this.file);
      fs5.chmodSync(this.file, 384);
      this.warningMessage = "";
    } finally {
      if (fs5.existsSync(temp)) fs5.unlinkSync(temp);
    }
  }
};

// src/app-subscription.ts
var nameRe = /^[a-z0-9]+(?:-[a-z0-9]+)*$/;
var invalidNameKey = {
  apps: "appSubscription.invalidAppName",
  skills: "appSubscription.invalidSkillName",
  connections: "appSubscription.invalidConnectionName"
};
async function setSubscription(http, kind, name2, enabled) {
  if (!(kind in invalidNameKey)) return { ok: false, error: t("appSubscription.invalidKind") };
  if (typeof name2 !== "string" || !nameRe.test(name2) || name2.length > 48) return { ok: false, error: t(invalidNameKey[kind]) };
  const response = await http.fetch(`/api/dsh/${kind}/${encodeURIComponent(name2)}/subscription`, { method: enabled ? "POST" : "DELETE" });
  if (response.ok) return { ok: true };
  let error = `HTTP ${response.status}`;
  try {
    const body = await response.json();
    if (body?.error) error = body.error;
  } catch {
  }
  return { ok: false, error };
}
function setAppSubscription(http, name2, enabled) {
  return setSubscription(http, "apps", name2, enabled);
}

// src/web-link.ts
async function exchangeWebLink(base, http, url) {
  if (!base || typeof url !== "string" || !url.startsWith(`${base}/`)) return url;
  const next = url.slice(base.length);
  if (next.startsWith("/api/")) return url;
  try {
    const response = await http.fetch("/api/dsh/web-link", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ next }) }, 1e4);
    if (!response.ok) return url;
    const value = await response.json();
    return typeof value?.url === "string" && /^https?:\/\//.test(value.url) ? value.url : url;
  } catch {
    return url;
  }
}

// src/workbench-host.ts
import fs6 from "node:fs";
import path7 from "node:path";
import { createHash as createHash2 } from "node:crypto";

// src/workbench.ts
var checklistKey = (pane, widget) => `${pane}:${widget}`;
function toolValue(result) {
  if (result?.isError) {
    const text2 = (result.content ?? []).filter((part) => part?.type === "text").map((part) => part.text).join("\n");
    throw new Error(text2 || t("overview.toolFailed"));
  }
  if (result?.structuredContent !== void 0) return result.structuredContent;
  const text = (result?.content ?? []).filter((part) => part?.type === "text").map((part) => String(part.text)).join("\n");
  try {
    return JSON.parse(text);
  } catch {
    return text;
  }
}
var maxBoardBytes = 32 << 10;
function checkBoardBlocks(blocks) {
  if (!Array.isArray(blocks) || blocks.length === 0) return { error: t("workbench.blocksRequired") };
  if (blocks.length > 20) return { error: t("workbench.blocksTooMany") };
  if (new TextEncoder().encode(JSON.stringify(blocks)).length > maxBoardBytes) return { error: t("workbench.boardTooLarge") };
  const out = [];
  const text = (value) => value === null || value === void 0 ? "" : String(value);
  for (const [index, block] of blocks.entries()) {
    const where = t("workbench.blockWhere", { index: index + 1 });
    switch (block?.type) {
      case "text":
        if (typeof block.text !== "string" || !block.text.trim()) return { error: t("workbench.textEmpty", { where }) };
        out.push({ type: "text", text: block.text });
        break;
      case "list":
        if (!Array.isArray(block.items) || block.items.length === 0 || block.items.length > 50) return { error: t("workbench.itemsRange", { where }) };
        out.push({ type: "list", items: block.items.map(text) });
        break;
      case "table":
        if (!Array.isArray(block.columns) || block.columns.length === 0 || block.columns.length > 12) return { error: t("workbench.columnsRange", { where }) };
        if (!Array.isArray(block.rows) || block.rows.length > 50 || !block.rows.every((row) => Array.isArray(row))) return { error: t("workbench.rowsRange", { where }) };
        out.push({ type: "table", columns: block.columns.map(text), rows: block.rows.map((row) => row.map(text)) });
        break;
      case "citations": {
        if (!Array.isArray(block.items) || block.items.length === 0 || block.items.length > 30) return { error: t("workbench.citationsRange", { where }) };
        const items = [];
        for (const [i, item] of block.items.entries()) {
          const at = t("workbench.citationAt", { where, index: i + 1 });
          const int = (value) => Number.isInteger(value) && value > 0;
          if (!int(item?.doc) || !int(item?.from) || !int(item?.to) || item.to < item.from) return { error: t("workbench.citationNumbers", { at }) };
          if (item.version !== void 0 && item.version !== null && !int(item.version)) return { error: t("workbench.citationVersion", { at }) };
          if (typeof item.quote !== "string" || !item.quote.trim() || item.quote.length > 500) return { error: t("workbench.citationQuote", { at }) };
          items.push({ doc: item.doc, version: item.version ?? void 0, from: item.from, to: item.to, quote: item.quote, title: item.title === void 0 ? void 0 : text(item.title).slice(0, 200) });
        }
        out.push({ type: "citations", items });
        break;
      }
      default:
        return { error: t("workbench.blockType", { where }) };
    }
  }
  return { blocks: out };
}
function rowWriteOf(action) {
  if (!action.table) return void 0;
  if (action.delete) return "delete";
  return action.set && Object.keys(action.set).length ? "set" : void 0;
}

// src/workbench-host.ts
function findWidget(apps, request) {
  const app = apps.find((item) => item.name === request?.app);
  const pane = app?.workbench?.panes.find((item) => item.id === request?.pane && item.type === "panel");
  return Number.isInteger(request?.widget) ? pane?.widgets?.[request.widget] : void 0;
}
async function readWidgetData(apps, tools, request) {
  const widget = findWidget(apps, request);
  if (!widget?.source) return { error: t("workbenchHost.noComponent") };
  const name2 = `${widget.source.connection}__${widget.source.tool}`;
  if (!tools.hasTool(name2)) return { forbidden: true };
  if (!tools.isReadOnlyTool(name2)) return { error: t("workbenchHost.notReadOnly") };
  try {
    return { value: toolValue(await tools.callTool(name2, widget.source.args ?? {})) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function insertFormRow(apps, tools, request) {
  const widget = findWidget(apps, request);
  const table = widget?.type === "form" ? widget.submit?.table : void 0;
  if (!table) return { error: t("workbenchHost.formNoTable") };
  const allowed = new Set((widget.fields ?? []).map((field) => field.name));
  const row = {};
  for (const [key, value] of Object.entries(request.values ?? {})) {
    if (allowed.has(key) && value !== "" && value !== void 0) row[key] = value;
  }
  try {
    toolValue(await tools.callTool("app__write", { action: "insert", app: request.app, table, rows: [row] }));
    return { ok: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function writeTableRow(apps, tools, request) {
  const widget = findWidget(apps, request);
  const action = widget?.type === "table" && Number.isInteger(request?.action) ? widget.row_actions?.[request.action] : void 0;
  const write2 = action ? rowWriteOf(action) : void 0;
  const table = widget?.source?.table;
  if (!action || !write2 || !table || action.table !== table) return { error: t("workbenchHost.rowNoAction") };
  if (!Number.isInteger(request.id) || request.id <= 0) return { error: t("workbenchHost.rowNoId") };
  try {
    if (write2 === "set") {
      toolValue(await tools.callTool("app__write", { action: "update", app: request.app, table, id: request.id, row: action.set }));
      return { ok: true };
    }
    const args = { action: "delete", app: request.app, table, ids: [request.id] };
    const preview = toolValue(await tools.callTool("app__write", args));
    if (preview?.confirm) toolValue(await tools.callTool("app__write", { ...args, confirm: preview.confirm }));
    return { ok: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
var kbTool = (name2) => `kb__${name2}`;
async function readKBDoc(tools, request) {
  const name2 = kbTool("read");
  if (!tools.hasTool(name2)) return { forbidden: true };
  const int = (value) => Number.isInteger(value) && value > 0 ? value : void 0;
  if (!int(request?.doc)) return { error: t("workbenchHost.badDoc") };
  try {
    return { value: toolValue(await tools.callTool(name2, { doc: request.doc, version: int(request.version), from: int(request.from) ?? 1, to: int(request.to) })) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}
async function verifyCitations(tools, blocks) {
  const all = [];
  for (const block of blocks) if (block.type === "citations") all.push(...block.items);
  if (!all.length) return { blocks, bad: [], stale: [] };
  let results;
  if (tools?.hasTool(kbTool("verify"))) {
    try {
      const value = toolValue(await tools.callTool(kbTool("verify"), { citations: all.map(({ doc, version, from, to, quote }) => ({ doc, version, from, to, quote })) }));
      if (Array.isArray(value?.results)) results = value.results;
    } catch {
    }
  }
  const bad = [];
  const stale = [];
  let index = 0;
  const out = blocks.map((block) => {
    if (block.type !== "citations") return block;
    return {
      ...block,
      items: block.items.map((item) => {
        const result = results?.find((r) => r?.index === index);
        index++;
        const status = ["ok", "stale", "mismatch", "missing"].includes(result?.status) ? result.status : "unchecked";
        if (status === "mismatch" || status === "missing") bad.push(index);
        if (status === "stale") stale.push(index);
        const near = Number.isInteger(result?.near_skipped) && result.near_skipped > 0 ? result.near_skipped : void 0;
        return { ...item, title: item.title || result?.title || void 0, status, near_skipped: near };
      })
    };
  });
  return { blocks: out, bad, stale };
}
var safeName2 = /^[A-Za-z0-9_-]{1,80}$/;
function sessionFile(dir, app, sessionId) {
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(app) || app.length > 48) throw new Error(t("workbenchHost.invalidAppName"));
  if (typeof sessionId !== "string" || !sessionId) throw new Error(t("workbenchHost.invalidSessionId"));
  const session = safeName2.test(sessionId) ? sessionId : createHash2("sha256").update(sessionId).digest("hex").slice(0, 32);
  return path7.join(dir, `${app}--${session}.json`);
}
function writeAtomic(dir, file, value, sequence) {
  fs6.mkdirSync(dir, { recursive: true, mode: 448 });
  const temp = `${file}.${process.pid}.${sequence}.tmp`;
  try {
    fs6.writeFileSync(temp, JSON.stringify(value) + "\n", { mode: 384 });
    fs6.renameSync(temp, file);
  } finally {
    if (fs6.existsSync(temp)) fs6.unlinkSync(temp);
  }
}
var BoardStore = class {
  constructor(dir = path7.join(dshHome(), "dsh-ee", "boards")) {
    this.dir = dir;
  }
  sequence = 0;
  file(app, sessionId) {
    return sessionFile(this.dir, app, sessionId);
  }
  read(app, sessionId) {
    try {
      const value = JSON.parse(fs6.readFileSync(this.file(app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || !Array.isArray(value?.blocks)) return void 0;
      return value;
    } catch {
      return void 0;
    }
  }
  write(app, sessionId, blocks) {
    const file = this.file(app, sessionId);
    const board = { app, sessionId, updatedAt: (/* @__PURE__ */ new Date()).toISOString(), blocks };
    writeAtomic(this.dir, file, board, ++this.sequence);
    return board;
  }
};
var maxChecked = 100;
var ChecklistStore = class {
  constructor(dir = path7.join(dshHome(), "dsh-ee", "checklists")) {
    this.dir = dir;
  }
  sequence = 0;
  read(app, sessionId) {
    try {
      const value = JSON.parse(fs6.readFileSync(sessionFile(this.dir, app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || typeof value?.lists !== "object" || !value.lists) return void 0;
      return value;
    } catch {
      return void 0;
    }
  }
  /** 把 done 里的条目勾上、undone 里的取消；返回这个清单现在勾了哪些。 */
  update(app, sessionId, key, done, undone) {
    if (!/^[a-z0-9-]{1,32}:\d{1,2}$/.test(key)) throw new Error(t("workbenchHost.invalidChecklistKey"));
    const file = sessionFile(this.dir, app, sessionId);
    const current = this.read(app, sessionId) ?? { app, sessionId, updatedAt: "", lists: {} };
    const set = new Set(current.lists[key] ?? []);
    for (const id of done) set.add(id);
    for (const id of undone) set.delete(id);
    const next = [...set].slice(0, maxChecked);
    const value = { app, sessionId, updatedAt: (/* @__PURE__ */ new Date()).toISOString(), lists: { ...current.lists, [key]: next } };
    writeAtomic(this.dir, file, value, ++this.sequence);
    return next;
  }
};
var workbenchToolName = "ee_workbench";
async function runWorkbenchTool(deps, args, exec) {
  if (args?.action !== "board" && args?.action !== "checklist") throw new Error(t("workbenchHost.actionInvalid"));
  const app = String(args?.app ?? "");
  const apps = await deps.readApps();
  const target = apps.find((item) => item.name === app);
  if (!target) throw new Error(t("workbenchHost.appNotFound", { app }));
  if (args.action === "checklist") return runChecklist(deps, target, args, exec);
  if (!target.workbench?.panes.some((pane) => pane.widgets?.some((widget) => widget.type === "board"))) throw new Error(t("workbenchHost.noBoard", { app }));
  const checked = checkBoardBlocks(args?.blocks);
  if (!checked.blocks) throw new Error(checked.error);
  const sessionId = exec?.agent?.session?.id ?? deps.boundSession(app);
  if (typeof sessionId !== "string" || !sessionId) throw new Error(t("workbenchHost.noBoardSession"));
  const verified = await verifyCitations(deps.tools, checked.blocks);
  deps.boards.write(app, sessionId, verified.blocks);
  const notes = [t("workbenchHost.boardUpdated")];
  if (verified.bad.length) notes.push(t("workbenchHost.badCitations", { numbers: verified.bad.map((n) => `[${n}]`).join(" ") }));
  if (verified.stale.length) notes.push(t("workbenchHost.staleCitations", { numbers: verified.stale.map((n) => `[${n}]`).join(" ") }));
  return notes.join("\n");
}
function runChecklist(deps, target, args, exec) {
  if (!deps.checklists) throw new Error(t("workbenchHost.checklistUnsupported"));
  const found = [];
  for (const pane of target.workbench?.panes ?? []) {
    (pane.widgets ?? []).forEach((widget2, index) => {
      if (widget2.type === "checklist") found.push({ key: checklistKey(pane.id, index), widget: widget2 });
    });
  }
  if (!found.length) throw new Error(t("workbenchHost.noChecklist", { app: target.name }));
  const title = typeof args?.title === "string" ? args.title.trim() : "";
  const picked = title ? found.filter((item) => item.widget.title === title) : found;
  if (picked.length !== 1) throw new Error(title ? t("workbenchHost.checklistNotFound", { title, options: found.map((item) => t("workbenchHost.checklistOption", { title: item.widget.title })).join("") }) : t("workbenchHost.checklistAmbiguous", { count: found.length, options: found.map((item) => t("workbenchHost.checklistOption", { title: item.widget.title })).join("") }));
  const ids = (value) => Array.isArray(value) ? value.map((x) => String(x).trim()).filter((x) => x && x.length <= 80) : [];
  const done = ids(args?.done);
  const undone = ids(args?.undone);
  if (!done.length && !undone.length) throw new Error(t("workbenchHost.doneOrUndone"));
  const { key, widget } = picked[0];
  if (widget.items?.length) {
    const known = widget.items.map((item) => item.id);
    const unknown = [...done, ...undone].filter((id) => !known.includes(id));
    if (unknown.length) throw new Error(t("workbenchHost.itemsNotFound", { title: widget.title, unknown: unknown.join(t("common.listSeparator")), known: known.join(t("common.listSeparator")) }));
  }
  const sessionId = exec?.agent?.session?.id ?? deps.boundSession(target.name);
  if (typeof sessionId !== "string" || !sessionId) throw new Error(t("workbenchHost.noChecklistSession"));
  const now = deps.checklists.update(target.name, sessionId, key, done, undone);
  const total = widget.items?.length;
  return total ? t("workbenchHost.checklistUpdated", { title: widget.title, done: widget.items.filter((item) => now.includes(item.id)).length, total }) : t("workbenchHost.checklistMarked", { title: widget.title, count: now.length });
}
function registerWorkbenchTool(ctx, deps) {
  return ctx.tools.register({
    name: workbenchToolName,
    description: t("workbenchHost.description"),
    parameters: {
      type: "object",
      required: ["action", "app"],
      properties: {
        action: { type: "string", enum: ["board", "checklist"] },
        app: { type: "string", description: t("workbenchHost.paramApp") },
        title: { type: "string", description: t("workbenchHost.paramTitle") },
        done: { type: "array", items: { type: "string" }, description: t("workbenchHost.paramDone") },
        undone: { type: "array", items: { type: "string" }, description: t("workbenchHost.paramUndone") },
        blocks: {
          type: "array",
          maxItems: 20,
          description: t("workbenchHost.paramBlocks"),
          items: {
            type: "object",
            required: ["type"],
            properties: {
              type: { type: "string", enum: ["text", "list", "table", "citations"] },
              text: { type: "string" },
              items: {
                type: "array",
                description: t("workbenchHost.paramItems"),
                items: {
                  anyOf: [
                    { type: "string" },
                    {
                      type: "object",
                      required: ["doc", "from", "to", "quote"],
                      properties: {
                        doc: { type: "integer" },
                        version: { type: "integer" },
                        from: { type: "integer" },
                        to: { type: "integer" },
                        quote: { type: "string" },
                        title: { type: "string" }
                      }
                    }
                  ]
                }
              },
              columns: { type: "array", items: { type: "string" } },
              rows: { type: "array", items: { type: "array", items: { type: "string" } } }
            }
          }
        }
      }
    },
    output: { schema: {}, render: (_args, value) => [{ type: "text", text: renderResult(value) }] },
    execute: async (args, exec) => ({ content: [{ type: "text", text: await runWorkbenchTool(deps, args, exec) }] })
  });
}

// src/local-mcp.ts
import fs7 from "node:fs";
import path8 from "node:path";
var PACKAGE_RE = /^(?:@[a-z0-9][a-z0-9._-]*\/)?[a-z0-9][a-z0-9._-]*$/;
var VERSION_RE = /^[0-9A-Za-z][0-9A-Za-z.+_-]*$/;
var EXTENSION_ID_RE = /^[a-z0-9][a-z0-9._-]{0,40}\/[a-z0-9][a-z0-9._-]{0,60}$/;
var SERVER_NAME_RE = /^[A-Za-z0-9_-]{1,32}$/;
function localMcpStorePath() {
  return path8.join(dshHome(), "dsh-ee", "local-mcp.json");
}
function parsePackageSpec(raw) {
  const text = String(raw ?? "").trim();
  if (!text) return { error: t("localMcp.missingPackage") };
  if (/[\s:]/.test(text)) return { error: t("localMcp.notNpmPackage", { text }) };
  const at = text.lastIndexOf("@");
  const hasVersion = at > 0;
  const pkg = hasVersion ? text.slice(0, at) : text;
  const version = hasVersion ? text.slice(at + 1) : "";
  if (!PACKAGE_RE.test(pkg)) return { error: t("localMcp.invalidPackage", { pkg }) };
  if (hasVersion && !VERSION_RE.test(version)) return { error: t("localMcp.invalidVersion", { version }) };
  return { package: pkg, version };
}
function parseExtensionId(raw) {
  const text = String(raw ?? "").trim();
  return EXTENSION_ID_RE.test(text) ? text : void 0;
}
function localServerName(extensionId, packageName) {
  const base = (extensionId ?? packageName.replace(/^@/, "")).replace(/[^A-Za-z0-9_-]+/g, "-").replace(/-+/g, "-").replace(/^-|-$/g, "");
  const clipped = base.slice(0, 32).replace(/-$/, "");
  const name2 = clipped || "ee-local";
  return SERVER_NAME_RE.test(name2) ? name2 : "ee-local";
}
function deriveExtensionId(packageName) {
  const scoped = /^@([a-z0-9][a-z0-9._-]*)\/([a-z0-9][a-z0-9._-]*)$/.exec(packageName);
  if (scoped) return `${scoped[1]}/${scoped[2]}`.slice(0, 102);
  return `local/${packageName}`.slice(0, 102);
}
function npxCommand(platform) {
  return platform === "win32" ? "npx.cmd" : "npx";
}
function localMcpEntry(config, extension, options = { npmrc: "" }) {
  const spec = extension.version ? `${extension.package}@${extension.version}` : extension.package;
  const env = {
    DSH_EE_URL: config.base,
    // 令牌留在 DSH 凭据库里，由 mcp-client 的 ${credential:…} 模板在连接时解析；注册项里不落明文。
    DSH_EE_TOKEN: `\${credential:${config.tokenRef}}`,
    DSH_EE_HARNESS: "dsh"
  };
  if (options.npmrc) env.NPM_CONFIG_USERCONFIG = options.npmrc;
  return {
    transport: "stdio",
    command: npxCommand(options.platform ?? process.platform),
    args: ["-y", spec],
    env,
    startupTimeoutMs: 6e4
  };
}
function localMcpSnippet(config, extension, npmrc = "") {
  const spec = extension.version ? `${extension.package}@${extension.version}` : extension.package;
  const q = (value) => `'${value.replace(/'/g, "''")}'`;
  const env = [
    `      DSH_EE_URL: ${q(config.base)}`,
    `      DSH_EE_TOKEN: ${q(`\${credential:${config.tokenRef}}`)}`,
    "      DSH_EE_HARNESS: dsh"
  ];
  if (npmrc) env.push(`      NPM_CONFIG_USERCONFIG: ${q(npmrc)}`);
  return [
    `- id: ${q(`dsh-ee-local-${extension.serverName}`)}`,
    `  name: '@deepseek-ai/dsh-mcp-client'`,
    `  config:`,
    `    serverName: ${q(extension.serverName)}`,
    `    transport: stdio`,
    `    command: ${q(npxCommand(process.platform))}`,
    `    args: [${q("-y")}, ${q(spec)}]`,
    `    env:`,
    ...env,
    `    expandTemplates: true`,
    `    startupTimeoutMs: 60000`
  ].join("\n");
}
function validExtension(value) {
  if (!value || typeof value !== "object") return false;
  const item = value;
  return typeof item.id === "string" && EXTENSION_ID_RE.test(item.id) && typeof item.package === "string" && PACKAGE_RE.test(item.package) && typeof item.version === "string" && (item.version === "" || VERSION_RE.test(item.version)) && typeof item.serverName === "string" && SERVER_NAME_RE.test(item.serverName) && typeof item.addedAt === "string";
}
function readLocalExtensions(file = localMcpStorePath()) {
  try {
    const data = JSON.parse(fs7.readFileSync(file, "utf8"));
    return Array.isArray(data.extensions) ? data.extensions.filter(validExtension) : [];
  } catch {
    return [];
  }
}
function writeLocalExtensions(extensions, file = localMcpStorePath()) {
  fs7.mkdirSync(path8.dirname(file), { recursive: true, mode: 448 });
  const tmp = `${file}.tmp`;
  fs7.writeFileSync(tmp, `${JSON.stringify({ version: 1, extensions }, null, 2)}
`, { mode: 384 });
  fs7.renameSync(tmp, file);
}
function parseLocalCommand(rawInput) {
  const tokens = String(rawInput ?? "").trim().split(/\s+/).filter(Boolean);
  const [action, ...rest] = tokens;
  const args = { action: "list", target: "", yes: false };
  if (!action) return args;
  if (action === "help") return { ...args, action: "help" };
  if (!["list", "add", "remove", "snippet"].includes(action)) {
    return { ...args, error: t("localMcp.unknownSubcommand", { action }) };
  }
  args.action = action;
  const positional = [];
  for (let i = 0; i < rest.length; i++) {
    const token = rest[i];
    if (token === "--yes" || token === "-y") args.yes = true;
    else if (token === "--id") {
      const value = rest[++i];
      if (value === void 0) return { ...args, error: t("localMcp.idHint") };
      const id = parseExtensionId(value);
      if (!id) return { ...args, error: t("localMcp.invalidId", { value }) };
      args.id = id;
    } else if (token.startsWith("-")) return { ...args, error: t("localMcp.unknownFlag", { token }) };
    else positional.push(token);
  }
  if ((args.action === "add" || args.action === "remove" || args.action === "snippet") && positional.length === 0) {
    return { ...args, error: t("localMcp.missingTarget", { action: args.action }) };
  }
  if (positional.length > 1) return { ...args, error: t("localMcp.extraArgs", { args: positional.slice(1).join(" ") }) };
  args.target = positional[0] ?? "";
  return args;
}
function createLocalMcp(ctx, config, options = {}) {
  const store = options.store ?? localMcpStorePath();
  const npmrc = options.npmrc ?? npmrcPath(ctx);
  const platform = options.platform ?? process.platform;
  const disposers = /* @__PURE__ */ new Map();
  const manager = () => {
    let service;
    try {
      service = ctx.get("mcpManager");
    } catch {
      return void 0;
    }
    return service && typeof service.register === "function" && typeof service.list === "function" ? service : void 0;
  };
  const names = async () => {
    const service = manager();
    if (!service) return /* @__PURE__ */ new Set();
    try {
      const list = await service.list();
      return new Set(list.map((item) => item.name));
    } catch {
      return /* @__PURE__ */ new Set();
    }
  };
  const register = (extension) => {
    const service = manager();
    if (!service) return;
    if (disposers.has(extension.id)) return;
    const dispose = service.register(extension.serverName, localMcpEntry(config, extension, { npmrc, platform }));
    disposers.set(extension.id, () => {
      try {
        dispose();
      } catch {
      }
    });
  };
  const unregister = (id) => {
    const dispose = disposers.get(id);
    if (!dispose) return;
    dispose();
    disposers.delete(id);
  };
  const describe = (extension, state) => `  ${extension.id}  ${extension.package}${extension.version ? `@${extension.version}` : ""}  serverName=${extension.serverName}  ${state}`;
  return {
    get available() {
      return manager() !== void 0;
    },
    async start() {
      const service = manager();
      if (!service) return { registered: [], skipped: [] };
      const extensions = readLocalExtensions(store);
      const taken = await names();
      const registered = [];
      const skipped = [];
      for (const extension of extensions) {
        if (taken.has(extension.serverName)) {
          skipped.push({ name: extension.id, reason: t("localMcp.duplicateService", { name: extension.serverName }) });
          continue;
        }
        try {
          register(extension);
          taken.add(extension.serverName);
          registered.push(extension.id);
        } catch (error) {
          skipped.push({ name: extension.id, reason: error instanceof Error ? error.message : String(error) });
        }
      }
      return { registered, skipped };
    },
    stop() {
      for (const dispose of disposers.values()) dispose();
      disposers.clear();
    },
    async refresh() {
      for (const id of [...disposers.keys()]) unregister(id);
      return this.start();
    },
    list() {
      const extensions = readLocalExtensions(store);
      const hint = this.available ? "" : t("localMcp.noManagerHint");
      if (!extensions.length) {
        return { kind: "success", text: t("localMcp.noExtensions", { hint }) };
      }
      const state = /* @__PURE__ */ new Map();
      const service = manager();
      if (service) {
        try {
          const rows = service.list();
          if (Array.isArray(rows)) for (const row of rows) state.set(row.name, row.status?.state ?? t("localMcp.registered"));
        } catch {
        }
      }
      const lines = extensions.map((extension) => describe(extension, state.get(extension.serverName) ?? (this.available ? t("localMcp.unregistered") : t("localMcp.unregisteredNoManager"))));
      return { kind: "success", text: t("localMcp.listHeader", { count: extensions.length, lines: lines.join("\n"), hint }) };
    },
    async add(parsed) {
      const spec = parsePackageSpec(parsed.target);
      if ("error" in spec) return { kind: "error", text: spec.error };
      const id = parsed.id ?? deriveExtensionId(spec.package);
      const extension = {
        id,
        package: spec.package,
        version: spec.version,
        serverName: localServerName(parsed.id ? id : void 0, spec.package),
        addedAt: (/* @__PURE__ */ new Date()).toISOString()
      };
      const stored = readLocalExtensions(store);
      const sameId = stored.find((item) => item.id === extension.id);
      const samePackage = stored.find((item) => item.package === extension.package);
      if (sameId ? sameId.package === extension.package && sameId.version === extension.version : samePackage && samePackage.version === extension.version) {
        return { kind: "success", text: t("localMcp.alreadyAdded", { spec: `${extension.package}${extension.version ? `@${extension.version}` : ""}` }) };
      }
      if (sameId && sameId.package !== extension.package) {
        return { kind: "error", text: t("localMcp.idTaken", { id: extension.id, pkg: sameId.package }) };
      }
      if (!sameId && samePackage) {
        return { kind: "error", text: t("localMcp.packageBound", { id: samePackage.id }) };
      }
      if (!this.available) {
        return {
          kind: "error",
          text: t("localMcp.noManagerAdd", { snippet: localMcpSnippet(config, extension, npmrc) })
        };
      }
      const taken = await names();
      const ours = sameId !== void 0 && disposers.has(sameId.id);
      if (!ours && taken.has(extension.serverName)) {
        return { kind: "error", text: t("localMcp.serviceConflict", { name: extension.serverName, spec: `${extension.package}${extension.version ? `@${extension.version}` : ""}`, scope: id.split("/")[0] }) };
      }
      if (!parsed.yes) {
        return {
          kind: "success",
          text: t("localMcp.confirmPlan", { update: sameId ? t("localMcp.updateSuffix") : "", id: extension.id, pkg: extension.package, latest: extension.version ? `@${extension.version}` : t("localMcp.latestVersion"), server: extension.serverName, spec: `${extension.package}${extension.version ? `@${extension.version}` : ""}`, idFlag: parsed.id ? ` --id ${parsed.id}` : "" })
        };
      }
      try {
        unregister(extension.id);
        register(extension);
      } catch (error) {
        return { kind: "error", text: t("localMcp.registerFailed", { error: error instanceof Error ? error.message : String(error) }) };
      }
      writeLocalExtensions([...stored.filter((item) => item.id !== extension.id), extension], store);
      return {
        kind: "success",
        text: t("localMcp.registeredText", { id: extension.id, spec: `${extension.package}${extension.version ? `@${extension.version}` : ""}`, server: extension.serverName })
      };
    },
    async remove(parsed) {
      const stored = readLocalExtensions(store);
      const target = parsed.target;
      const hit = stored.find((item) => item.package === target || item.id === target || item.serverName === target || `${item.package}@${item.version}` === target);
      if (!hit) return { kind: "error", text: t("localMcp.notFound", { target }) };
      unregister(hit.id);
      writeLocalExtensions(stored.filter((item) => item !== hit), store);
      return { kind: "success", text: t("localMcp.removed", { id: hit.id, pkg: hit.package, server: hit.serverName }) };
    },
    snippet(parsed) {
      const spec = parsePackageSpec(parsed.target);
      if ("error" in spec) return { kind: "error", text: spec.error };
      const id = parsed.id ?? deriveExtensionId(spec.package);
      const extension = {
        id,
        package: spec.package,
        version: spec.version,
        serverName: localServerName(parsed.id ? id : void 0, spec.package)
      };
      return {
        kind: "success",
        text: t("localMcp.snippetIntro", { snippet: localMcpSnippet(config, extension, npmrc), hint: this.available ? t("localMcp.snippetHint") : "" })
      };
    }
  };
}
function registerLocalMcpCommand(ctx, local) {
  const commands = ctx.get("commands");
  if (!commands) return;
  try {
    const dispose = commands.register({
      name: "ee-local",
      description: t("localMcp.description"),
      input: { hint: t("localMcp.inputHint") },
      handler: async (invocation) => {
        let parsed = parseLocalCommand(invocation?.rawInput);
        if (parsed.error) return { kind: "error", text: parsed.error };
        if (parsed.action === "help") parsed = { ...parsed, action: "list" };
        switch (parsed.action) {
          case "add":
            return local.add(parsed);
          case "remove":
            return local.remove(parsed);
          case "snippet":
            return local.snippet(parsed);
          default:
            return local.list();
        }
      }
    });
    ctx.effect(() => dispose);
  } catch {
  }
}

// src/index.ts
var name = "dsh-ee-connect";
var inject = {
  skills: { required: true },
  tools: { required: true },
  credentials: { required: false },
  commands: { required: false },
  pluginManager: { required: false }
  // 不要在这里声明 mcpManager（DSH 0.2.2 才有的 @deepseek-ai/dsh-mcp-manager）：旧版 DSH 的加载器
  // 对 required: false 的依赖也会一直等它出现，整个插件停在 pending，登录、目录全部 404。
  // local-mcp.ts 在运行时用 ctx.get("mcpManager") 取，没有就退回配置片段。
};
function apply(ctx, input) {
  initI18n(ctx);
  const config = normalizeConfig(input);
  const tokens = new TokenManager(ctx, config);
  const http = new HttpClient(tokens);
  const bridge = new McpBridge(ctx, config, http);
  const localMcp = createLocalMcp(ctx, config);
  const writes = new WriteLog();
  bridge.onWrite = (sessionId, record) => writes.record(sessionId, record);
  let refreshSkills;
  const server = new ServerSettingsService(config, tokens, async () => {
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    await bridge.restart();
    await localMcp.refresh().catch(() => void 0);
    await refreshSkills?.().catch(() => void 0);
  });
  bridge.registerLocal("ee_plugins.upload", (args) => uploadPackage(http, args));
  registerApproval(ctx, bridge);
  ctx.effect(async () => {
    await tokens.importPending().catch(() => void 0);
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    let skills;
    const commands = registerCommands(ctx, config, tokens, async () => {
      bridge.wake();
      await Promise.all([bridge.sync().catch(() => void 0), skills?.refreshNow()]);
    });
    registerLocalMcpCommand(ctx, localMcp);
    const appBindings = new AppBindings();
    const boards = new BoardStore();
    const checklists = new ChecklistStore();
    registerDoctor(ctx, config, tokens, http, bridge, appBindings);
    const skillsReady = registerSkills(ctx, config, http).then((loaded) => {
      refreshSkills = loaded.refreshNow;
      return skills = loaded;
    });
    registerBrand(ctx, {
      readBrand: async () => (await skillsReady).readBrand(),
      readApps: async () => (await skillsReady).readApps(),
      readAvailableApps: async () => (await skillsReady).readAvailableApps(),
      setAppSubscription: async (name2, enabled) => {
        const result = await setAppSubscription(http, name2, enabled);
        if (result.ok) await skills?.refreshNow();
        return result;
      },
      readCenter: async () => (await skillsReady).readCenter(),
      setSubscription: async (request) => {
        const kind = String(request?.kind ?? "");
        const result = await setSubscription(http, kind, String(request?.name ?? ""), !!request?.enabled);
        if (result.ok) {
          await skills?.refreshNow();
          if (kind === "connections") {
            bridge.wake();
            await bridge.sync().catch(() => void 0);
          }
        }
        return result;
      },
      readAppBindings: async () => appBindings.list(),
      bindApp: async (name2, sessionId) => {
        if (!(await (await skillsReady).readApps()).some((app) => app.name === name2)) throw new Error(t("serverIndex.microAppNotFound"));
        return appBindings.bind(name2, sessionId);
      },
      unbindApp: async (name2) => appBindings.unbind(name2),
      workbenchData: async (request) => readWidgetData(await (await skillsReady).readApps(), bridge, request),
      insertFormRow: async (request) => insertFormRow(await (await skillsReady).readApps(), bridge, request),
      writeTableRow: async (request) => writeTableRow(await (await skillsReady).readApps(), bridge, request),
      vote: async (request) => {
        const token = await tokens.resolve();
        if (!token) throw new Error(t("serverIndex.notLoggedIn"));
        const r = await fetch(`${config.base}/api/dsh/feedback`, {
          method: "POST",
          headers: { "content-type": "application/json", Authorization: `Bearer ${token.value}` },
          body: JSON.stringify(request),
          signal: AbortSignal.timeout(15e3)
        });
        if (!r.ok) throw new Error(t("serverIndex.feedbackFailed", { status: r.status }));
      },
      readBoard: async (request) => boards.read(String(request?.app ?? ""), String(request?.sessionId ?? "")) ?? null,
      readChecklists: async (request) => checklists.read(String(request?.app ?? ""), String(request?.sessionId ?? ""))?.lists ?? {},
      setChecklist: async (request) => {
        const app = String(request?.app ?? "");
        const id = String(request?.id ?? "").trim();
        if (!id || id.length > 80) throw new Error(t("serverIndex.invalidEntryId"));
        if (!(await (await skillsReady).readApps()).some((item) => item.name === app)) throw new Error(t("serverIndex.microAppNotFound"));
        return checklists.update(app, String(request?.sessionId ?? ""), String(request?.key ?? ""), request?.done ? [id] : [], request?.done ? [] : [id]);
      },
      readKBDoc: (request) => readKBDoc(bridge, request),
      turnWrites: async (request) => ({
        base: config.base,
        writes: writes.between(String(request?.sessionId ?? ""), Number(request?.from), request?.to == null ? void 0 : Number(request.to))
      }),
      readOverview: () => readEnterpriseOverview(config, http, bridge).catch((error) => ({ failure: overviewFailure(config, error) })),
      login: () => commands.login(),
      doctor: () => runLocalDoctor(ctx, config, tokens, http, bridge, appBindings),
      serverSettings: async () => server.read(),
      testServer: (request) => server.test(request?.base),
      saveServer: (request) => server.save(request?.base),
      updatePlugin: () => updateConnect(ctx, config, http),
      webLink: (request) => exchangeWebLink(config.base, http, String(request?.url ?? ""))
    });
    const disposeWorkbenchTool = registerWorkbenchTool(ctx, {
      boards,
      checklists,
      readApps: () => skills.readApps(),
      boundSession: (app) => appBindings.list()[app]?.sessionId,
      tools: bridge
    });
    await skillsReady;
    await bridge.start();
    await localMcp.start().catch(() => void 0);
    return () => {
      localMcp.stop();
      disposeWorkbenchTool();
      bridge.stop();
    };
  });
}
export {
  apply,
  inject,
  name
};
