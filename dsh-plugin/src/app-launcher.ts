import { startText } from "./apps-view.js";
import { t } from "./i18n/index.js";
import { createSession, currentSessionOf, openSession, sendToSession } from "./host-session.js";
import type { CatalogApp } from "./skills.js";

/** 发送这一步出错：消息可能已经到了宿主，调用方不能再填输入框或重发。 */
export class SendUnknownError extends Error {
  constructor(cause: unknown) {
    super(t("appLauncher.sendUnknown"));
    this.cause = cause;
  }
}

/** 发送结果不确定时的提示（按当前界面语言在调用时取）。 */
export function sendUnknownNotice(): string {
  return t("appLauncher.sendUnknown");
}

export interface LauncherDeps {
  /** 记下应用当前对应的会话；失败不影响启动。 */
  bind: (app: CatalogApp, sessionId: string) => Promise<void>;
  /** 会话已经切过去、消息发出之后调用（打开工作台等）；失败不影响启动。 */
  afterLaunch?: (app: CatalogApp, sessionId: string) => Promise<void> | void;
}

async function sendOnce(ctx: any, id: string, text: string): Promise<void> {
  try {
    await sendToSession(ctx, id, text);
  } catch (error) {
    throw new SendUnknownError(error);
  }
}

export function createLauncher(ctx: any, deps: LauncherDeps) {
  const after = async (app: CatalogApp, id: string) => {
    try {
      await deps.afterLaunch?.(app, id);
    } catch {
      // 工作台等附加动作失败时，会话已经开始，不回退
    }
  };
  const bind = (app: CatalogApp, id: string) => deps.bind(app, id).catch(() => undefined);

  /** 新建会话、切过去、绑定，再发一条启动消息。 */
  const launchNew = async (app: CatalogApp, starter?: string) => {
    const id = await createSession(ctx);
    await openSession(ctx, id);
    await bind(app, id);
    await sendOnce(ctx, id, startText(app, starter));
    await after(app, id);
  };

  /** 首屏：在当前空白会话里直接开始，并记下绑定；宿主把首屏会话做成懒创建的、取不到时退化为新建。 */
  const launchHero = async (app: CatalogApp) => {
    const id = currentSessionOf(ctx.sessions?.list?.getSnapshot?.());
    if (!id) {
      await launchNew(app);
      return;
    }
    await sendOnce(ctx, id, startText(app, app.starters?.[0]));
    await bind(app, id);
    await after(app, id);
  };

  /** 回到已绑定的会话，只切换不发消息；回不去就新建。 */
  const resume = async (app: CatalogApp, id: string) => {
    try {
      await openSession(ctx, id);
    } catch {
      await launchNew(app);
      return;
    }
    await after(app, id);
  };

  /**
   * 执行一次启动；发送之前的步骤出错时退回 fallback（填入或复制），
   * 发送这一步出错时只提示，不再填入，避免员工回车后发两遍。
   */
  const run = async (launch: () => Promise<void>, fallback: () => Promise<string>): Promise<string> => {
    try {
      await launch();
      return "";
    } catch (error) {
      try { console.error("dsh-ee launch failed:", error); } catch { /* console 不可用时忽略 */ }
      if (error instanceof SendUnknownError) return sendUnknownNotice();
      return fallback();
    }
  };

  return { launchNew, launchHero, resume, run };
}

/** 同一时间只跑一个：并发调用复用正在进行的那一次。 */
export function singleFlight<T>(fn: () => Promise<T>): () => Promise<T> {
  let running: Promise<T> | undefined;
  return () => {
    if (!running) running = fn().finally(() => { running = undefined; });
    return running;
  };
}
