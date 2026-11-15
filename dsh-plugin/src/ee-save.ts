import type { DshContext } from "./dsh.js";
import { fillInput, sendToSession } from "./host-session.js";
import { t } from "./i18n/index.js";

// /ee-save 提炼请求的正文；e2e 场景按这段文本比对（e2e/dsh/scenarios/run-m6-9.mjs）。
export const EE_SAVE_PROMPT =
  "刚才这段对话里可能沉淀了值得复用的做法。请把它提炼成一个企业能力：\n" +
  "1. 先告诉我你建议做成什么——提示词（反复用的一段话）、技能（带工具的操作流程）还是应用（带工作台的助手），以及理由；等我确认后再做，也可以我直接指定类型。\n" +
  "2. 确认后用 ee_hub 的 create（for_me: true）建好：起一个清晰的标识（slug）、标题和说明，说明第一段用两三句话总结这段对话的来源（写明「来自一次对话」）；正文 / 人设按我们刚才的做法写。做成应用时先读技能 dsh-ee-app-design，工作台按里面的仪表盘版式和响应式宽度来排。\n" +
  "3. 建好后给我网页上的详情链接（<dsh-ee 地址>/assets/<id>），我想继续改范围或内容就去网页。\n" +
  "注意：这个能力会把我们对话的摘要发到 dsh-ee 保存。";

/** 把提炼请求发进会话；宿主拒收时退回填进输入框，员工按回车就能发。返回是否发出去了。 */
export async function deliverEeSave(ctx: DshContext, sessionId: string): Promise<"sent" | "filled" | "failed"> {
  try {
    await sendToSession(ctx, sessionId, EE_SAVE_PROMPT);
    return "sent";
  } catch {
    // 发送失败的原因宿主已记在会话的 promptError 里；这里只换一条能走通的路
    return fillInput(ctx, EE_SAVE_PROMPT) ? "filled" : "failed";
  }
}

/**
 * 在渲染端注册 /ee-save。
 *
 * 用客户端的 commandUi（`ui.kind: "action"`），不用宿主的 commands：宿主命令在 command/run 与
 * command/done 之间执行，期间用 sendSession 排进去的消息在 0.2.2 上不会开启模型回合（M9 验证时发现）。
 * action 在客户端直接跑，等于员工自己发了一条消息。commandUi 不存在的旧宿主上没有 /ee-save。
 */
export function registerEeSave(ctx: DshContext): void {
  ctx.inject?.(["commandUi"], (scoped: any) => {
    const commandUi = scoped.commandUi ?? scoped.get?.("commandUi");
    if (typeof commandUi?.register !== "function") return;
    try {
      const dispose = commandUi.register({
        name: "ee-save",
        label: () => "ee-save",
        description: () => t("eeSave.description"),
        available: (session: any) => typeof session?.sessionId === "string" && session.sessionId !== "",
        ui: {
          kind: "action",
          run: (session: any) => {
            void deliverEeSave(ctx, session.sessionId);
          },
        },
      });
      scoped.effect?.(() => dispose);
    } catch {
      // 与宿主命令撞名时 commandUi 会抛错；/ee-save 注册失败不能拖死后面的品牌接管
    }
  });
}
