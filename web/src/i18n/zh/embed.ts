/** 同站 iframe 嵌入模式（?embed=1）的文案。 */
export default {
  embed: {
    loggedOut: "已退出本次嵌入会话。宿主可以重新打开这个应用，或在浏览器中登录。",
    authRequired: "登录已过期或尚未登录。请在宿主页面重新打开这个应用，或在浏览器里打开后登录。",
    connecting: "正在通过宿主页面连接…",
    connectHint: "宿主会自动完成授权；如果一直停在这里，请在浏览器顶层打开这个应用。",
    openInBrowser: "在浏览器打开",
    failureHint: "嵌入模式下不会在这里跳转登录页：宿主可以监听 dsh-ee:error 消息，在顶层窗口完成登录后重新打开。",
    mobileHint: "当前是移动端嵌入视图，内容可正常使用；需要完整操作时请在浏览器顶层打开。",
    exchangeNetwork: "无法连接 dsh-ee",
    exchangeInvalidCode: "登录码无效",
  },
};
