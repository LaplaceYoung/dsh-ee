import { createElement, useState } from "react";
import type { DshContext } from "./dsh.js";
import { currentSessionOf } from "./host-session.js";

// 反馈（M8）：每条助手消息尾的 👍👎，经 remote 的 dshEE/vote 打到 POST /api/dsh/feedback。

type VoteRemote = { vote: (request: { app: string; session: string; message: string; vote: 1 | -1; note: string }) => Promise<unknown> };
type VoteBindings = { appBindings: () => Promise<unknown> };

/** 在 assistant-actions 槽位（0.2.2 起有；旧宿主没有就直接不出按钮）注册 👍👎。 */
export function registerVotes(ctx: DshContext, remote: VoteRemote & VoteBindings, unwrapRemote: (value: unknown) => any): void {
  if (!ctx?.slots?.inject) return;
  const buttonStyle = { border: 0, background: "transparent", cursor: "pointer", fontSize: 13, padding: "1px 4px", opacity: 0.65 };
  const VoteButtons = (props: any) => {
    const messageId = String(props?.messageId ?? props?.message_id ?? props?.node?.id ?? props?.id ?? "");
    const [voted, setVoted] = useState(0);
    const mark = async (value: 1 | -1) => {
      if (voted) return;
      const sessionId = currentSessionOf(ctx.sessions?.list?.getSnapshot?.());
      let note = "";
      if (value < 0 && typeof window !== "undefined" && typeof window.prompt === "function") {
        note = window.prompt("这次回答哪里没帮上忙？（可留空，会按问题聚合给应用作者）") ?? "";
      }
      let app = "";
      try {
        const bindings = unwrapRemote(await remote.appBindings()) as Record<string, { sessionId?: string }> | undefined;
        app = Object.entries(bindings ?? {}).find(([, b]) => b?.sessionId === sessionId)?.[0] ?? "";
      } catch { /* 取不到绑定就只记消息 */ }
      try {
        // 服务端拒绝（401/5xx）时 remote 也会 resolve 成 { ok: false }，必须解包后再标记已投票。
        unwrapRemote(await remote.vote({ app, session: sessionId, message: messageId, vote: value, note }));
        setVoted(value);
      } catch {
        // 保持按钮可点，让用户能重试
      }
    };
    return createElement("span", { "data-dsh-ee-ui": "votes", style: { display: "inline-flex", gap: 2 } },
      createElement("button", { type: "button", title: "有帮助", "aria-label": "有帮助", disabled: voted !== 0, style: { ...buttonStyle, opacity: voted === 1 ? 1 : buttonStyle.opacity }, onClick: () => void mark(1) }, "👍"),
      createElement("button", { type: "button", title: "没帮上忙", "aria-label": "没帮上忙", disabled: voted !== 0, style: { ...buttonStyle, opacity: voted === -1 ? 1 : buttonStyle.opacity }, onClick: () => void mark(-1) }, "👎"));
  };
  try {
    ctx.slots.inject("conversation.chat.assistant-actions", () =>
      ctx.slots.register({ name: "conversation.chat.assistant-actions", id: "dsh-ee-vote", order: 100, label: "评价这次回答" }, VoteButtons));
  } catch {
    // 旧版宿主没有这个槽位：先不上按钮（计划里的降级方案是看板上的文字反馈，见 docs）
  }
}
