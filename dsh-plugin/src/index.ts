import { normalizeConfig } from "./config.js";
import { TokenManager } from "./token.js";
import { HttpClient } from "./http.js";
import { registerCommands } from "./commands.js";
import { registerSkills } from "./skills.js";
import { McpBridge } from "./bridge.js";
import { registerApproval } from "./approval.js";
import { registerDoctor, runLocalDoctor } from "./doctor.js";
import { registerBrand } from "./brand.js";
import { overviewFailure, readEnterpriseOverview } from "./overview.js";
import { ServerSettingsService } from "./server-settings.js";
import { updateConnect } from "./connect-update.js";
import { uploadPackage } from "./upload.js";
import { AppBindings } from "./app-bindings.js";
import { setAppSubscription } from "./app-subscription.js";
import { exchangeWebLink } from "./web-link.js";
import { BoardStore, ChecklistStore, insertFormRow, writeTableRow, readKBDoc, readWidgetData, registerWorkbenchTool } from "./workbench-host.js";
import { registerVotes } from "./votes.js";
import { WriteLog } from "./write-log.js";
import type { DshContext, PluginConfig } from "./dsh.js";

export const name = "dsh-ee-connect";
export const inject = {
  skills: { required: true },
  tools: { required: true },
  credentials: { required: false },
  commands: { required: false },
  pluginManager: { required: false },
};

export function apply(ctx: DshContext, input: Partial<PluginConfig>): void {
  const config = normalizeConfig(input);
  const tokens = new TokenManager(ctx, config);
  const http = new HttpClient(tokens);
  const bridge = new McpBridge(ctx, config, http);
  const writes = new WriteLog();
  bridge.onWrite = (sessionId, record) => writes.record(sessionId, record);
  let refreshSkills: (() => Promise<void>) | undefined;
  // 员工在 DSH 设置里改过地址的话，从这里开始就用改过的地址
  const server = new ServerSettingsService(config, tokens, async () => {
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    await bridge.restart();
    await refreshSkills?.().catch(() => undefined);
  });
  bridge.registerLocal("ee_plugins.upload", (args) => uploadPackage(http, args));
  registerApproval(ctx, bridge);

  ctx.effect(async () => {
    await tokens.importPending().catch(() => undefined);
    // 每次启动都把令牌同步进 profile 的 .npmrc：覆盖从 tarball 地址安装、令牌轮换等情况
    const current = await tokens.resolve();
    if (current?.value) await tokens.syncNpmrc(current.value).catch(() => false);
    let skills: Awaited<ReturnType<typeof registerSkills>> | undefined;
    const commands = registerCommands(ctx, config, tokens, async () => {
      bridge.wake();
      await Promise.all([bridge.sync().catch(() => undefined), skills?.refreshNow()]);
    });
    const appBindings = new AppBindings();
    const boards = new BoardStore();
    const checklists = new ChecklistStore();
    registerDoctor(ctx, config, tokens, http, bridge, appBindings);
    const skillsReady = registerSkills(ctx, config, http).then((loaded) => {
      refreshSkills = loaded.refreshNow;
      return (skills = loaded);
    });
    registerBrand(ctx, {
      readBrand: async () => (await skillsReady).readBrand(),
      readApps: async () => (await skillsReady).readApps(),
      readAvailableApps: async () => (await skillsReady).readAvailableApps(),
      setAppSubscription: async (name, enabled) => {
        const result = await setAppSubscription(http, name, enabled);
        if (result.ok) await skills?.refreshNow();
        return result;
      },
      readAppBindings: async () => appBindings.list(),
      bindApp: async (name, sessionId) => {
        if (!(await (await skillsReady).readApps()).some((app) => app.name === name)) throw new Error("目录里没有这个微应用");
        return appBindings.bind(name, sessionId);
      },
      unbindApp: async (name) => appBindings.unbind(name),
      workbenchData: async (request) => readWidgetData(await (await skillsReady).readApps(), bridge, request),
      insertFormRow: async (request) => insertFormRow(await (await skillsReady).readApps(), bridge, request),
      writeTableRow: async (request) => writeTableRow(await (await skillsReady).readApps(), bridge, request),
      vote: async (request) => {
        const token = await tokens.resolve();
        if (!token) throw new Error("没有登录企业账号");
        const r = await fetch(`${config.base}/api/dsh/feedback`, {
          method: "POST",
          headers: { "content-type": "application/json", Authorization: `Bearer ${token.value}` },
          body: JSON.stringify(request),
          signal: AbortSignal.timeout(15_000),
        });
        if (!r.ok) throw new Error(`反馈失败：HTTP ${r.status}`);
      },
      readBoard: async (request) => boards.read(String(request?.app ?? ""), String(request?.sessionId ?? "")) ?? null,
      readChecklists: async (request) => checklists.read(String(request?.app ?? ""), String(request?.sessionId ?? ""))?.lists ?? {},
      setChecklist: async (request) => {
        const app = String(request?.app ?? "");
        const id = String(request?.id ?? "").trim();
        if (!id || id.length > 80) throw new Error("条目编号不合法");
        if (!(await (await skillsReady).readApps()).some((item) => item.name === app)) throw new Error("目录里没有这个微应用");
        return checklists.update(app, String(request?.sessionId ?? ""), String(request?.key ?? ""), request?.done ? [id] : [], request?.done ? [] : [id]);
      },
      readKBDoc: (request) => readKBDoc(bridge, request),
      turnWrites: async (request) => ({
        base: config.base,
        writes: writes.between(String(request?.sessionId ?? ""), Number(request?.from), request?.to == null ? undefined : Number(request.to)),
      }),
      readOverview: () => readEnterpriseOverview(config, http, bridge).catch((error) => ({ failure: overviewFailure(config, error) })),
      login: () => commands.login(),
      doctor: () => runLocalDoctor(ctx, config, tokens, http, bridge, appBindings),
      serverSettings: async () => server.read(),
      testServer: (request) => server.test(request?.base),
      saveServer: (request) => server.save(request?.base),
      updatePlugin: () => updateConnect(ctx, config, http),
      webLink: (request) => exchangeWebLink(config.base, http, String(request?.url ?? "")),
    });
    const disposeWorkbenchTool = registerWorkbenchTool(ctx, {
      boards,
      checklists,
      readApps: () => skills!.readApps(),
      boundSession: (app) => appBindings.list()[app]?.sessionId,
      tools: bridge,
    });
    await skillsReady;
    await bridge.start();
    return () => {
      disposeWorkbenchTool();
      bridge.stop();
    };
  });
}
