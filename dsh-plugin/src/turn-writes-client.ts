import { createElement, useEffect, useState } from "react";
import { groupTurnWrites, turnWritesView, type TurnWriteGroup, type TurnWrites } from "./turn-writes-view.js";
import { createWebOpener } from "./web-link.js";

/**
 * 在对话每一轮末尾（conversation.chat.turnTail）放「已在 dsh-ee 留痕」提示。
 * 写操作由接入插件后端在调用时按会话记下（write-log.ts），这里按这一轮的起止时间去取。
 * 只看已经结束的轮次：没结束的轮次写操作可能还没做完。
 */
export function registerTurnWrites(ctx: any, remote: any, unwrapRemote: (result: any) => any): void {
  const cache = new Map<string, Promise<TurnWrites | undefined>>();
  const openWeb = createWebOpener(remote, unwrapRemote);
  const load = (sessionId: string, from: number, to: number) => {
    const key = `${sessionId}\n${from}\n${to}`;
    let pending = cache.get(key);
    if (!pending) {
      pending = Promise.resolve()
        .then(async () => unwrapRemote(await remote.turnWrites({ sessionId, from, to })) as TurnWrites)
        .catch(() => {
          // 旧版接入插件后端没有这个方法，或一时取不到：下次渲染再试
          cache.delete(key);
          return undefined;
        });
      if (cache.size > 500) cache.clear();
      cache.set(key, pending);
    }
    return pending;
  };
  const Tail = (props: any) => {
    const sessionId = typeof props?.sessionId === "string" ? props.sessionId : "";
    const from = Number(props?.turn?.start?.time);
    const to = Number(props?.turn?.end?.time);
    const ready = !!sessionId && Number.isFinite(from) && Number.isFinite(to);
    const [groups, setGroups] = useState<TurnWriteGroup[]>([]);
    useEffect(() => {
      if (!ready) return;
      let live = true;
      void load(sessionId, from, to).then((data) => { if (live) setGroups(groupTurnWrites(data)); });
      return () => { live = false; };
    }, [ready, sessionId, from, to]);
    return ready ? turnWritesView(createElement, groups, (url) => void openWeb(url)) : null;
  };
  ctx.slots.inject("conversation.chat.turnTail", () => ctx.slots.register({ name: "conversation.chat.turnTail", id: "dsh-ee-writes", order: 900, label: "dsh-ee" }, Tail));
}
