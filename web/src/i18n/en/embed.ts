/** Embed mode (?embed=1) copy. */
export default {
  embed: {
    loggedOut: "This embed session has been signed out. The host can reopen the app, or sign in with a browser.",
    authRequired: "Your session has expired or you're not signed in. Reopen this app from the host page, or open it in a browser and sign in there.",
    connecting: "Connecting through the host page…",
    connectHint: "The host will complete authorization automatically. If this persists, open the app at the top level in a browser.",
    openInBrowser: "Open in browser",
    failureHint: "Embed mode never redirects to the sign-in page inside the iframe. The host can listen for dsh-ee:error, sign the user in at the top level and reopen the app.",
    mobileHint: "You are viewing this app in a mobile embed. Content still works; open it at the top level in a browser for the full experience.",
    exchangeNetwork: "Can't reach dsh-ee",
    exchangeInvalidCode: "Invalid sign-in code",
  },
};
