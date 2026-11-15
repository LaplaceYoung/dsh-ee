import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { dshHome } from "./config.js";
import { t } from "./i18n/index.js";

/**
 * 一次经接入插件调到 dsh-ee 的写操作。dsh-ee 那边已经记下了这次调用（连接调用日志 / 审计日志），
 * 这里只在本机按会话留一份，让对话里那一轮的末尾能告诉员工「这一轮改了企业系统，已经留痕」。
 */
export interface WriteRecord {
  /** 调用开始的时间（Unix 毫秒），用来对上是哪一轮 */
  at: number;
  /** 服务端工具名，例如 demo-itsm__create_ticket、ee_hub */
  tool: string;
  /** 给人看的操作名：连接工具是工具名，ee_* 工具是 action 的中文说明 */
  label: string;
  /** 企业系统名（连接标题）；ee_* 工具为空 */
  system?: string;
  /** 连接的资产 id，网页详情是 /assets/<id> */
  asset?: number;
  ok: boolean;
}

/** 一轮之内写操作的最大条数；再多只计数不展示。 */
const MAX_PER_SESSION = 200;

/**
 * 这次调用算不算写操作，以及给人看的名字。
 * - 服务端标了 readOnlyHint 的：不算；
 * - 带 _meta.dshee.actions 的 ee_* 工具：只有标注过的 action 才算，危险操作的第一步（只拿预览，没带 confirm）不算；
 * - 其余工具（连接里没标只读的）：都算。
 */
export function writeOf(tool: any, args: any): { label: string; system?: string; asset?: number } | undefined {
  if (!tool || tool.annotations?.readOnlyHint === true) return undefined;
  const meta = tool._meta?.dshee ?? {};
  const actions = meta.actions;
  if (actions && typeof actions === "object") {
    const action = actions[String(args?.action)];
    if (!action) return undefined;
    if (action.e === "danger" && !args?.confirm) return undefined;
    return { label: String(action.zh || args?.action) };
  }
  const name = String(tool.name ?? "");
  const short = name.includes("__") ? name.slice(name.indexOf("__") + 2) : name;
  const asset = Number(meta.asset);
  return {
    label: short,
    ...(typeof meta.system === "string" && meta.system ? { system: meta.system } : systemOf(tool.description)),
    ...(Number.isInteger(asset) && asset > 0 ? { asset } : {}),
  };
}

/** 旧服务端没有 _meta.dshee.system：从说明开头的「【系统名】」里取。 */
function systemOf(description: unknown): { system?: string } {
  const match = /^【([^】]{1,80})】/.exec(String(description ?? ""));
  return match ? { system: match[1] } : {};
}

/** 工具结果是不是成功：MCP 的 isError 或抛错都算失败。 */
export function resultOk(result: any): boolean {
  return !(result && typeof result === "object" && result.isError === true);
}

/** 落在 [from, to] 之间的写操作；to 为空表示这一轮还没结束，算到现在。 */
export function writesBetween(records: readonly WriteRecord[], from: number, to?: number): WriteRecord[] {
  return records.filter((item) => item.at >= from && (to === undefined || item.at <= to));
}

const safeName = /^[A-Za-z0-9_-]{1,80}$/;

/** 按会话保存：$DSH_HOME/dsh-ee/writes/<会话>.json。只追加、最多留最近 200 条。 */
export class WriteLog {
  private sequence = 0;

  constructor(readonly dir = path.join(dshHome(), "dsh-ee", "writes")) {}

  private file(sessionId: string): string {
    if (typeof sessionId !== "string" || !sessionId) throw new Error(t("writeLog.invalidSessionId"));
    const name = safeName.test(sessionId) ? sessionId : createHash("sha256").update(sessionId).digest("hex").slice(0, 32);
    return path.join(this.dir, `${name}.json`);
  }

  read(sessionId: string): WriteRecord[] {
    try {
      const value = JSON.parse(fs.readFileSync(this.file(sessionId), "utf8"));
      if (value?.sessionId !== sessionId || !Array.isArray(value?.writes)) return [];
      return value.writes;
    } catch {
      return [];
    }
  }

  record(sessionId: string, item: WriteRecord): void {
    const writes = [...this.read(sessionId), item].slice(-MAX_PER_SESSION);
    const file = this.file(sessionId);
    fs.mkdirSync(this.dir, { recursive: true, mode: 0o700 });
    const temp = `${file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs.writeFileSync(temp, JSON.stringify({ sessionId, writes }) + "\n", { mode: 0o600 });
      fs.renameSync(temp, file);
    } finally {
      if (fs.existsSync(temp)) fs.unlinkSync(temp);
    }
  }

  between(sessionId: string, from: number, to?: number): WriteRecord[] {
    if (typeof sessionId !== "string" || !sessionId || !Number.isFinite(from)) return [];
    return writesBetween(this.read(sessionId), from, Number.isFinite(to) ? to : undefined);
  }
}
