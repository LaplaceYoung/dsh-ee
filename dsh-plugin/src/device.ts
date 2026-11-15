import { execFile } from "node:child_process";
import os from "node:os";
import { t } from "./i18n/index.js";
import type { PluginConfig } from "./dsh.js";

export interface DeviceStart {
  device_code: string;
  user_code: string;
  verification_uri: string;
  verification_uri_complete: string;
  interval: number;
}

export class DeviceFlow {
  constructor(private config: PluginConfig, private fetchImpl: typeof fetch = fetch) {}

  /** setupCode：网页「开始使用」页给的接入码，带上后这次登录会出现在那个页面上，可以就地确认。 */
  async start(setupCode = ""): Promise<DeviceStart> {
    const response = await this.fetchImpl(`${this.config.base}/api/device/code`, {
      method: "POST",
      headers: { "content-type": "application/json" },
      signal: AbortSignal.timeout(30_000),
      body: JSON.stringify({ client_name: os.hostname(), client_os: `${process.platform} ${os.release()}`, client_version: this.config.version, ...(setupCode ? { setup_code: setupCode } : {}) }),
    });
    if (!response.ok) {
      const body = await response.json().catch(() => null) as { error?: string } | null;
      throw new Error(body?.error ? t("device.startFailed", { error: body.error }) : t("device.startHttpFailed", { status: response.status }));
    }
    return response.json() as Promise<DeviceStart>;
  }

  async wait(deviceCode: string, intervalSeconds = 5, timeoutMs = 600_000, signal?: AbortSignal): Promise<{ access_token: string; scopes: string[]; user: { username: string; department: string }; expires_at: string }> {
    const deadline = Date.now() + timeoutMs;
    let interval = Math.max(1, intervalSeconds) * 1000;
    while (Date.now() < deadline) {
      if (signal?.aborted) throw new Error(t("device.cancelled"));
      const response = await this.fetchImpl(`${this.config.base}/api/device/token`, {
        method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ device_code: deviceCode }),
        signal: signal ? AbortSignal.any([signal, AbortSignal.timeout(30_000)]) : AbortSignal.timeout(30_000),
      });
      const body = await response.json() as { error?: string; access_token?: string; scopes?: string[]; user?: { username: string; department: string }; expires_at?: string };
      if (body.access_token) return body as any;
      // slow_down / rate_limited 都是「慢一点再来」，不算失败
      if (body.error === "slow_down" || body.error === "rate_limited") {
        interval += 5000;
      } else if (body.error !== "authorization_pending") {
        throw new Error(body.error || t("device.failed", { status: response.status }));
      }
      let onAbort: (() => void) | undefined;
      try {
        await new Promise<void>((resolve, reject) => {
          const timer = setTimeout(resolve, interval);
          onAbort = () => { clearTimeout(timer); reject(new Error(t("device.cancelled"))); };
          signal?.addEventListener("abort", onAbort, { once: true });
        });
      } finally {
        if (onAbort) signal?.removeEventListener("abort", onAbort);
      }
    }
    throw new Error(t("device.timeout"));
  }
}

export function openBrowser(url: string): void {
  // start 是 cmd 的内置命令，不能直接 execFile；空字符串占位窗口标题
  if (process.platform === "win32") execFile("cmd", ["/c", "start", "", url], () => {});
  else execFile(process.platform === "darwin" ? "open" : "xdg-open", [url], () => {});
}
