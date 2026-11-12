import { Remote, TypertRemoteService } from "@deepseek-ai/dsh-typert-protocol";
import type { DshContext } from "./dsh.js";

export interface ClientServiceActions {
  readBrand: () => Promise<any>;
  readApps: () => Promise<any>;
  readAvailableApps: () => Promise<any>;
  setAppSubscription: (name: string, enabled: boolean) => Promise<any>;
  readOverview: () => Promise<any>;
  readAppBindings: () => Promise<any>;
  bindApp: (name: string, sessionId: string) => Promise<any>;
  unbindApp: (name: string) => Promise<any>;
  workbenchData: (request: { app: string; pane: string; widget: number }) => Promise<any>;
  readBoard: (request: { app: string; sessionId: string }) => Promise<any>;
  readChecklists: (request: { app: string; sessionId: string }) => Promise<any>;
  setChecklist: (request: { app: string; sessionId: string; key: string; id: string; done: boolean }) => Promise<any>;
  readKBDoc: (request: { doc: number; version?: number; from: number; to: number }) => Promise<any>;
  /** 某个会话在 [from, to] 之间经接入插件做的写操作，对话里那一轮末尾展示 */
  turnWrites: (request: { sessionId: string; from: number; to?: number }) => Promise<any>;
  login: () => Promise<any>;
  doctor: () => Promise<any>;
  serverSettings: () => Promise<any>;
  testServer: (request: { base?: string }) => Promise<any>;
  saveServer: (request: { base?: string | null }) => Promise<any>;
  updatePlugin: () => Promise<any>;
  webLink: (request: { url?: string }) => Promise<string>;
  insertFormRow: (request: { app: string; pane: string; widget: number; values?: Record<string, unknown> }) => Promise<any>;
  writeTableRow: (request: { app: string; pane: string; widget: number; action: number; id: number }) => Promise<any>;
  vote: (request: { app: string; session: string; message: string; vote: 1 | -1; note: string }) => Promise<any>;
}

export class DshEEClientService extends TypertRemoteService {
  constructor(ctx: DshContext, private readonly actions: ClientServiceActions) {
    super(ctx as any, "dshEE");
  }

  @Remote
  async brand(): Promise<any> {
    return this.actions.readBrand();
  }

  @Remote
  async apps(): Promise<any> {
    return this.actions.readApps();
  }

  @Remote
  async availableApps(): Promise<any> {
    return this.actions.readAvailableApps();
  }

  @Remote
  async enableApp(request: { name: string }): Promise<any> {
    return this.actions.setAppSubscription(request?.name, true);
  }

  @Remote
  async disableApp(request: { name: string }): Promise<any> {
    return this.actions.setAppSubscription(request?.name, false);
  }

  @Remote
  async appBindings(): Promise<any> {
    return this.actions.readAppBindings();
  }

  @Remote
  async bindApp(request: { name: string; sessionId: string }): Promise<any> {
    return this.actions.bindApp(request.name, request.sessionId);
  }

  @Remote
  async unbindApp(request: { name: string }): Promise<any> {
    return this.actions.unbindApp(request.name);
  }

  @Remote
  async workbenchData(request: { app: string; pane: string; widget: number }): Promise<any> {
    return this.actions.workbenchData(request);
  }

  @Remote
  async board(request: { app: string; sessionId: string }): Promise<any> {
    return this.actions.readBoard(request);
  }

  @Remote
  async checklists(request: { app: string; sessionId: string }): Promise<any> {
    return this.actions.readChecklists(request);
  }

  @Remote
  async setChecklist(request: { app: string; sessionId: string; key: string; id: string; done: boolean }): Promise<any> {
    return this.actions.setChecklist(request);
  }

  @Remote
  async kbDoc(request: { doc: number; version?: number; from: number; to: number }): Promise<any> {
    return this.actions.readKBDoc(request);
  }

  @Remote
  async turnWrites(request: { sessionId: string; from: number; to?: number }): Promise<any> {
    return this.actions.turnWrites(request);
  }

  @Remote
  async overview(): Promise<any> {
    return this.actions.readOverview();
  }

  @Remote
  async login(): Promise<any> {
    return this.actions.login();
  }

  @Remote
  async doctor(): Promise<any> {
    return this.actions.doctor();
  }

  @Remote
  async serverSettings(): Promise<any> {
    return this.actions.serverSettings();
  }

  @Remote
  async testServer(request: { base?: string }): Promise<any> {
    return this.actions.testServer(request ?? {});
  }

  @Remote
  async saveServer(request: { base?: string | null }): Promise<any> {
    return this.actions.saveServer(request ?? {});
  }

  @Remote
  async updatePlugin(): Promise<any> {
    return this.actions.updatePlugin();
  }

  @Remote
  async webLink(request: { url?: string }): Promise<string> {
    return this.actions.webLink(request ?? {});
  }

  @Remote
  async insertFormRow(request: { app: string; pane: string; widget: number; values?: Record<string, unknown> }): Promise<any> {
    return this.actions.insertFormRow(request);
  }

  @Remote
  async writeTableRow(request: { app: string; pane: string; widget: number; action: number; id: number }): Promise<any> {
    return this.actions.writeTableRow(request);
  }

  @Remote
  async vote(request: { app: string; session: string; message: string; vote: 1 | -1; note: string }): Promise<any> {
    return this.actions.vote(request);
  }
}

export function registerBrand(ctx: DshContext, actions: ClientServiceActions) {
  return new DshEEClientService(ctx, actions);
}
