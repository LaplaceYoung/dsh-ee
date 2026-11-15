// 业务工作台的插件后端：面板取数（只读工具）、AI 看板的存取与 ee_workbench 工具。
import fs from "node:fs";
import path from "node:path";
import { createHash } from "node:crypto";
import { dshHome } from "./config.js";
import { t } from "./i18n/index.js";
import { renderResult } from "./bridge.js";
import type { CatalogApp } from "./skills.js";
import type { DshContext } from "./dsh.js";
import { checkBoardBlocks, checklistKey, rowWriteOf, toolValue, type Board, type BoardBlock, type Citation, type WorkbenchWidget } from "./workbench.js";

export interface ToolCaller {
  hasTool(serverName: string): boolean;
  isReadOnlyTool(serverName: string): boolean;
  callTool(serverName: string, args: unknown): Promise<any>;
}

export interface WidgetRequest {
  app: string;
  pane: string;
  widget: number;
}

/** 取数结果：forbidden 表示员工没有这个工具的权限，前端显示「无权限」。 */
export type WidgetData = { value: any } | { forbidden: true } | { error: string };

export function findWidget(apps: CatalogApp[], request: WidgetRequest): WorkbenchWidget | undefined {
  const app = apps.find((item) => item.name === request?.app);
  const pane = app?.workbench?.panes.find((item) => item.id === request?.pane && item.type === "panel");
  return Number.isInteger(request?.widget) ? pane?.widgets?.[request.widget] : undefined;
}

/**
 * 按目录里的配置给一个组件取数。服务端保存时已经校验过只读，这里再按当前员工的工具列表校验一次：
 * 员工没权限的工具返回 forbidden；不是只读的工具一律拒绝（宁可不显示，也不能让面板触发写操作）。
 */
export async function readWidgetData(apps: CatalogApp[], tools: ToolCaller, request: WidgetRequest): Promise<WidgetData> {
  const widget = findWidget(apps, request);
  if (!widget?.source) return { error: t("workbenchHost.noComponent") };
  const name = `${widget.source.connection}__${widget.source.tool}`;
  if (!tools.hasTool(name)) return { forbidden: true };
  if (!tools.isReadOnlyTool(name)) return { error: t("workbenchHost.notReadOnly") };
  try {
    return { value: toolValue(await tools.callTool(name, widget.source.args ?? {})) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}

/** 表单提交到应用自带的表：按目录里的配置找到表单，经 app__write 插入一行（服务端校验字段、记审计）。 */
export async function insertFormRow(apps: CatalogApp[], tools: ToolCaller, request: WidgetRequest & { values?: Record<string, unknown> }): Promise<{ ok: true } | { error: string }> {
  const widget = findWidget(apps, request);
  const table = widget?.type === "form" ? widget.submit?.table : undefined;
  if (!table) return { error: t("workbenchHost.formNoTable") };
  const allowed = new Set((widget!.fields ?? []).map((field) => field.name));
  const row: Record<string, unknown> = {};
  for (const [key, value] of Object.entries(request.values ?? {})) {
    if (allowed.has(key) && value !== "" && value !== undefined) row[key] = value;
  }
  try {
    toolValue(await tools.callTool("app__write", { action: "insert", app: request.app, table, rows: [row] }));
    return { ok: true };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}

/**
 * 表格的行按钮写表：按目录里的配置找到那个按钮（不信任面板传来的表名和字段），经 app__write 改这一行或删掉它。
 * 删除要服务端的确认码：员工在面板上已经点过两次确认，这里先拿确认码再带着它删。
 */
export async function writeTableRow(apps: CatalogApp[], tools: ToolCaller, request: WidgetRequest & { action: number; id: number }): Promise<{ ok: true } | { error: string }> {
  const widget = findWidget(apps, request);
  const action = widget?.type === "table" && Number.isInteger(request?.action) ? widget.row_actions?.[request.action] : undefined;
  const write = action ? rowWriteOf(action) : undefined;
  const table = widget?.source?.table;
  if (!action || !write || !table || action.table !== table) return { error: t("workbenchHost.rowNoAction") };
  if (!Number.isInteger(request.id) || request.id <= 0) return { error: t("workbenchHost.rowNoId") };
  try {
    if (write === "set") {
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

/** 知识库工具在网关上的名字（kb__<工具>） */
export const kbTool = (name: string) => `kb__${name}`;

export interface KBDocRequest {
  doc: number;
  version?: number;
  from: number;
  to: number;
}

/** 文档窗格读原文：经 kb__read，服务端按员工权限过滤，读的记录也会留下。 */
export async function readKBDoc(tools: ToolCaller, request: KBDocRequest): Promise<WidgetData> {
  const name = kbTool("read");
  if (!tools.hasTool(name)) return { forbidden: true };
  const int = (value: any) => (Number.isInteger(value) && value > 0 ? value : undefined);
  if (!int(request?.doc)) return { error: t("workbenchHost.badDoc") };
  try {
    return { value: toolValue(await tools.callTool(name, { doc: request.doc, version: int(request.version), from: int(request.from) ?? 1, to: int(request.to) })) };
  } catch (error) {
    return { error: error instanceof Error ? error.message : String(error) };
  }
}

/**
 * 把看板里的引用逐条拿到服务端核对，结果写进每条引用的 status。核对不了（没有 kb__verify、网络错误）时标 unchecked。
 * 返回对不上的引用编号（从 1 开始，按看板里出现的顺序）。
 */
export async function verifyCitations(tools: ToolCaller | undefined, blocks: BoardBlock[]): Promise<{ blocks: BoardBlock[]; bad: number[]; stale: number[] }> {
  const all: Citation[] = [];
  for (const block of blocks) if (block.type === "citations") all.push(...block.items);
  if (!all.length) return { blocks, bad: [], stale: [] };
  let results: any[] | undefined;
  if (tools?.hasTool(kbTool("verify"))) {
    try {
      const value = toolValue(await tools.callTool(kbTool("verify"), { citations: all.map(({ doc, version, from, to, quote }) => ({ doc, version, from, to, quote })) }));
      if (Array.isArray(value?.results)) results = value.results;
    } catch {
      // 核对失败不影响写看板，标成没核对
    }
  }
  const bad: number[] = [];
  const stale: number[] = [];
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
        const near = Number.isInteger(result?.near_skipped) && result.near_skipped > 0 ? result.near_skipped : undefined;
        return { ...item, title: item.title || result?.title || undefined, status, near_skipped: near } as Citation;
      }),
    };
  });
  return { blocks: out, bad, stale };
}

const safeName = /^[A-Za-z0-9_-]{1,80}$/;

/** 每个应用每个会话一个文件：<dir>/<应用>--<会话>.json。 */
function sessionFile(dir: string, app: string, sessionId: string): string {
  if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(app) || app.length > 48) throw new Error(t("workbenchHost.invalidAppName"));
  if (typeof sessionId !== "string" || !sessionId) throw new Error(t("workbenchHost.invalidSessionId"));
  // 会话 id 原样做文件名不安全时，改用它的哈希
  const session = safeName.test(sessionId) ? sessionId : createHash("sha256").update(sessionId).digest("hex").slice(0, 32);
  return path.join(dir, `${app}--${session}.json`);
}

/** 先写临时文件再改名，读的一方不会读到写了一半的内容。 */
function writeAtomic(dir: string, file: string, value: unknown, sequence: number) {
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 });
  const temp = `${file}.${process.pid}.${sequence}.tmp`;
  try {
    fs.writeFileSync(temp, JSON.stringify(value) + "\n", { mode: 0o600 });
    fs.renameSync(temp, file);
  } finally {
    if (fs.existsSync(temp)) fs.unlinkSync(temp);
  }
}

/** AI 看板，每个应用每个会话一个文件：$DSH_HOME/dsh-ee/boards/<应用>-<会话>.json，写法与 apps.json 相同。 */
export class BoardStore {
  private sequence = 0;

  constructor(readonly dir = path.join(dshHome(), "dsh-ee", "boards")) {}

  private file(app: string, sessionId: string): string {
    return sessionFile(this.dir, app, sessionId);
  }

  read(app: string, sessionId: string): Board | undefined {
    try {
      const value = JSON.parse(fs.readFileSync(this.file(app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || !Array.isArray(value?.blocks)) return undefined;
      return value as Board;
    } catch {
      return undefined;
    }
  }

  write(app: string, sessionId: string, blocks: Board["blocks"]): Board {
    const file = this.file(app, sessionId);
    const board: Board = { app, sessionId, updatedAt: new Date().toISOString(), blocks };
    writeAtomic(this.dir, file, board, ++this.sequence);
    return board;
  }
}

/** 清单的勾选状态：键是「窗格:组件序号」，值是勾了的条目编号。 */
export interface Checklists {
  app: string;
  sessionId: string;
  updatedAt: string;
  lists: Record<string, string[]>;
}

const maxChecked = 100;

/** 清单勾选状态，按会话保存：$DSH_HOME/dsh-ee/checklists/<应用>--<会话>.json。员工勾选和 AI 更新写的是同一个文件。 */
export class ChecklistStore {
  private sequence = 0;

  constructor(readonly dir = path.join(dshHome(), "dsh-ee", "checklists")) {}

  read(app: string, sessionId: string): Checklists | undefined {
    try {
      const value = JSON.parse(fs.readFileSync(sessionFile(this.dir, app, sessionId), "utf8"));
      if (value?.app !== app || value?.sessionId !== sessionId || typeof value?.lists !== "object" || !value.lists) return undefined;
      return value as Checklists;
    } catch {
      return undefined;
    }
  }

  /** 把 done 里的条目勾上、undone 里的取消；返回这个清单现在勾了哪些。 */
  update(app: string, sessionId: string, key: string, done: string[], undone: string[]): string[] {
    if (!/^[a-z0-9-]{1,32}:\d{1,2}$/.test(key)) throw new Error(t("workbenchHost.invalidChecklistKey"));
    const file = sessionFile(this.dir, app, sessionId);
    const current = this.read(app, sessionId) ?? { app, sessionId, updatedAt: "", lists: {} };
    const set = new Set(current.lists[key] ?? []);
    for (const id of done) set.add(id);
    for (const id of undone) set.delete(id);
    const next = [...set].slice(0, maxChecked);
    const value: Checklists = { app, sessionId, updatedAt: new Date().toISOString(), lists: { ...current.lists, [key]: next } };
    writeAtomic(this.dir, file, value, ++this.sequence);
    return next;
  }
}

export const workbenchToolName = "ee_workbench";

export interface WorkbenchToolDeps {
  boards: BoardStore;
  /** 清单勾选状态；没有时 checklist 动作不可用 */
  checklists?: ChecklistStore;
  readApps: () => Promise<CatalogApp[]>;
  /** 拿不到调用方会话时，退回这个应用当前绑定的会话 */
  boundSession: (app: string) => string | undefined;
  /** 核对看板引用用的企业工具（kb__verify）；没有时引用标成没核对 */
  tools?: ToolCaller;
}

/** 执行一次 ee_workbench 调用；exec 是 DSH 传给工具的执行上下文，exec.agent.session 是调用它的会话。 */
export async function runWorkbenchTool(deps: WorkbenchToolDeps, args: any, exec?: any): Promise<string> {
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

/** ee_workbench 的 checklist 动作：按标题找到清单（只有一个时可以不写），勾上 done、取消 undone。 */
function runChecklist(deps: WorkbenchToolDeps, target: CatalogApp, args: any, exec?: any): string {
  if (!deps.checklists) throw new Error(t("workbenchHost.checklistUnsupported"));
  const found: { key: string; widget: WorkbenchWidget }[] = [];
  for (const pane of target.workbench?.panes ?? []) {
    (pane.widgets ?? []).forEach((widget, index) => {
      if (widget.type === "checklist") found.push({ key: checklistKey(pane.id, index), widget });
    });
  }
  if (!found.length) throw new Error(t("workbenchHost.noChecklist", { app: target.name }));
  const title = typeof args?.title === "string" ? args.title.trim() : "";
  const picked = title ? found.filter((item) => item.widget.title === title) : found;
  if (picked.length !== 1) throw new Error(title ? t("workbenchHost.checklistNotFound", { title, options: found.map((item) => t("workbenchHost.checklistOption", { title: item.widget.title })).join("") }) : t("workbenchHost.checklistAmbiguous", { count: found.length, options: found.map((item) => t("workbenchHost.checklistOption", { title: item.widget.title })).join("") }));
  const ids = (value: any) => (Array.isArray(value) ? value.map((x) => String(x).trim()).filter((x) => x && x.length <= 80) : []);
  const done = ids(args?.done);
  const undone = ids(args?.undone);
  if (!done.length && !undone.length) throw new Error(t("workbenchHost.doneOrUndone"));
  const { key, widget } = picked[0];
  // 配置里写好的条目：编号必须存在；从数据源取的条目，编号由数据决定，这里不查
  if (widget.items?.length) {
    const known = widget.items.map((item) => item.id);
    const unknown = [...done, ...undone].filter((id) => !known.includes(id));
    if (unknown.length) throw new Error(t("workbenchHost.itemsNotFound", { title: widget.title, unknown: unknown.join(t("common.listSeparator")), known: known.join(t("common.listSeparator")) }));
  }
  const sessionId = exec?.agent?.session?.id ?? deps.boundSession(target.name);
  if (typeof sessionId !== "string" || !sessionId) throw new Error(t("workbenchHost.noChecklistSession"));
  const now = deps.checklists.update(target.name, sessionId, key, done, undone);
  const total = widget.items?.length;
  return total ? t("workbenchHost.checklistUpdated", { title: widget.title, done: widget.items!.filter((item) => now.includes(item.id)).length, total }) : t("workbenchHost.checklistMarked", { title: widget.title, count: now.length });
}

/** 注册 ee_workbench 工具，返回撤销函数。 */
export function registerWorkbenchTool(ctx: DshContext, deps: WorkbenchToolDeps): () => void {
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
                        title: { type: "string" },
                      },
                    },
                  ],
                },
              },
              columns: { type: "array", items: { type: "string" } },
              rows: { type: "array", items: { type: "array", items: { type: "string" } } },
            },
          },
        },
      },
    },
    output: { schema: {}, render: (_args: unknown, value: unknown) => [{ type: "text", text: renderResult(value) }] },
    execute: async (args: any, exec: any) => ({ content: [{ type: "text", text: await runWorkbenchTool(deps, args, exec) }] }),
  });
}
