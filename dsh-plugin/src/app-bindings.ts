import fs from "node:fs";
import path from "node:path";
import { dshHome } from "./config.js";
import { t } from "./i18n/index.js";

export interface AppBinding {
  sessionId: string;
  updatedAt: string;
}

export type AppBindingMap = Record<string, AppBinding>;

export class AppBindings {
  private sequence = 0;
  private warningMessage = "";

  constructor(readonly file = path.join(dshHome(), "dsh-ee", "apps.json")) {}

  list(): AppBindingMap {
    return this.read().bindings;
  }

  warning(): string {
    return this.warningMessage;
  }

  bind(name: string, sessionId: string): AppBindingMap {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name) || name.length > 48) throw new Error(t("appBindings.invalidAppName"));
    if (typeof sessionId !== "string" || !sessionId.trim() || sessionId.length > 256) throw new Error(t("appBindings.invalidSessionId"));
    const bindings = this.list();
    bindings[name] = { sessionId: sessionId.trim(), updatedAt: new Date().toISOString() };
    this.write(bindings);
    return bindings;
  }

  unbind(name: string): AppBindingMap {
    if (!/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(name) || name.length > 48) throw new Error(t("appBindings.invalidAppName"));
    const bindings = this.list();
    delete bindings[name];
    this.write(bindings);
    return bindings;
  }

  private read(): { version: 1; bindings: AppBindingMap } {
    this.warningMessage = "";
    if (!fs.existsSync(this.file)) return { version: 1, bindings: {} };
    try {
      const value = JSON.parse(fs.readFileSync(this.file, "utf8"));
      if (value?.version !== 1 || !value.bindings || typeof value.bindings !== "object" || Array.isArray(value.bindings)) throw new Error("invalid");
      const bindings: AppBindingMap = {};
      for (const [name, binding] of Object.entries(value.bindings)) {
        const sessionId = (binding as any)?.sessionId;
        const updatedAt = (binding as any)?.updatedAt;
        if (typeof sessionId === "string" && sessionId && typeof updatedAt === "string") bindings[name] = { sessionId, updatedAt };
      }
      return { version: 1, bindings };
    } catch {
      this.warningMessage = t("appBindings.corruptFile", { file: this.file });
      return { version: 1, bindings: {} };
    }
  }

  private write(bindings: AppBindingMap) {
    fs.mkdirSync(path.dirname(this.file), { recursive: true, mode: 0o700 });
    const temp = `${this.file}.${process.pid}.${++this.sequence}.tmp`;
    try {
      fs.writeFileSync(temp, JSON.stringify({ version: 1, bindings }, null, 2) + "\n", { mode: 0o600 });
      fs.renameSync(temp, this.file);
      fs.chmodSync(this.file, 0o600);
      this.warningMessage = "";
    } finally {
      if (fs.existsSync(temp)) fs.unlinkSync(temp);
    }
  }
}
