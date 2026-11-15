import { t } from "./i18n/index.js";

// 来源：dsh-worktable v0.5.1（f98246e，MIT）；仅保留本计划用到的宿主会话兼容写法。
export function currentSessionOf(snapshot: any): string {
  if (typeof snapshot?.current === "string") return snapshot.current;
  const rows = Object.values(snapshot?.byId ?? {}) as any[];
  return rows.find((row) => (row?.retainedBy?.mainView ?? 0) > 0)?.id ?? "";
}

export async function openSession(ctx: any, id: string): Promise<void> {
  const ui = typeof ctx?.get === "function" ? ctx.get("uiWorkspace") : undefined;
  if (typeof ui?.openSession === "function") {
    await ui.openSession(id);
    return;
  }
  if (typeof ctx?.sessions?.open === "function") {
    await ctx.sessions.open(id);
    return;
  }
  throw new Error(t("hostSession.switchUnsupported"));
}

/** 新会话放进哪个工作区：当前会话所在的；当前没有会话时用列表里的第一个。 */
function currentWorkspaceId(ctx: any): string | undefined {
  const items = ctx?.workspaces?.list?.getSnapshot?.()?.items;
  if (!Array.isArray(items) || items.length === 0) return undefined;
  const snapshot = ctx.sessions?.list?.getSnapshot?.();
  const id = currentSessionOf(snapshot);
  const cwd = snapshot?.byId?.[id]?.cwd;
  const hit = items.find((w: any) => id && Array.isArray(w?.sessionIds) && w.sessionIds.includes(id))
    ?? items.find((w: any) => cwd && w?.path === cwd)
    ?? items[0];
  return typeof hit?.workspaceId === "string" && hit.workspaceId ? hit.workspaceId : undefined;
}

// 0.2.2 起 initializeDefault 只在首次使用时建默认工作区，已有工作区或找不到「文稿」目录时返回空，
// 所以先用已有的工作区，实在没有才让宿主建默认的。
export async function createSession(ctx: any): Promise<string> {
  let workspaceId = currentWorkspaceId(ctx);
  if (!workspaceId) {
    let workspace: any;
    if (typeof ctx?.workspaces?.initializeDefault === "function") {
      workspace = await ctx.workspaces.initializeDefault();
    } else if (typeof ctx?.workspaces?.create === "function") {
      const snapshot = ctx.sessions?.list?.getSnapshot?.();
      const id = currentSessionOf(snapshot);
      const cwd = snapshot?.byId?.[id]?.cwd;
      if (!cwd) throw new Error(t("hostSession.noCwd"));
      workspace = await ctx.workspaces.create({ path: cwd });
    } else {
      throw new Error(t("hostSession.createWorkspaceUnsupported"));
    }
    workspaceId = typeof workspace?.workspaceId === "string" && workspace.workspaceId ? workspace.workspaceId : undefined;
  }
  if (!workspaceId) throw new Error(t("hostSession.noWorkspace"));
  // uiWorkspace 会复用这个工作区里还没发过消息的空白会话，和点「New Session」一样
  const ui = typeof ctx?.get === "function" ? ctx.get("uiWorkspace") : undefined;
  if (typeof ui?.connectWorkspace === "function") return ui.connectWorkspace(workspaceId);
  if (typeof ctx?.sessions?.create !== "function") throw new Error(t("hostSession.createSessionUnsupported"));
  return ctx.sessions.create({ workspaceId });
}

function checkSendResult(result: any) {
  if (result?.kind === "error" || result?.ok === false) {
    throw new Error(result.text || result.error?.message || result.error?.code || t("hostSession.rejected"));
  }
}

export async function sendToSession(ctx: any, id: string, text: string): Promise<void> {
  const sessions = ctx?.sessions;
  const send = async (session: any) => {
    if (session && typeof ctx?.conversation?.sendSession === "function") {
      checkSendResult(await ctx.conversation.sendSession(session, text, [], "queue"));
      return;
    }
    if (session && typeof session.prompt === "function") {
      checkSendResult(await session.prompt([{ type: "text", text }], "queue"));
      return;
    }
    throw new Error(t("hostSession.sendUnsupported"));
  };
  if (typeof sessions?.using === "function") {
    await sessions.using(id, { source: "dshEEApps" }, async (ref: any) => {
      const binding = await ref.ready;
      await send(binding.session);
    });
    return;
  }
  let session: any;
  for (let i = 0; i < 10; i++) {
    session = sessions?.binding?.(id)?.session;
    if (session) break;
    await new Promise((resolve) => setTimeout(resolve, 200));
  }
  await send(session);
}

export function fillInput(ctx: any, text: string): boolean {
  try {
    const resolver = ctx?.conversation?.input;
    if (typeof resolver?.for !== "function") return false;
    const snapshot = ctx.sessions?.list?.getSnapshot?.();
    const id = currentSessionOf(snapshot);
    const scope = id ? ctx.sessions?.scope?.(id) : null;
    if (!scope) return false;
    const input = resolver.for(scope);
    const state = input?.state?.getSnapshot?.();
    if (!state || typeof state.draft !== "string" || !Number.isInteger(state.draftRev) || !["plain", "claimed"].includes(state.phase) || !Array.isArray(state.occurrences)) return false;
    const compactReferences = typeof snapshot?.current !== "string";
    let end = state.draft.length;
    let previousEnd = 0;
    for (const item of state.occurrences) {
      if (!Number.isInteger(item.offset) || !Number.isInteger(item.length) || item.length < 0 || item.offset < previousEnd || item.offset + item.length > state.draft.length) return false;
      previousEnd = item.offset + item.length;
      if (compactReferences) end += 1 - item.length;
    }
    const span = { start: end, end, draftRev: state.draftRev };
    const addition = (state.draft.trim() ? "\n\n" : "") + text;
    const inserted = typeof input.actions?.insertText === "function"
      ? input.actions.insertText(addition, span) === true
      : scope.bail?.("slash/input-insert-text", { text: addition, span }) === true;
    if (inserted) {
      try { input.focus?.(); } catch {}
    }
    return inserted;
  } catch {
    return false;
  }
}
