import { createContext, useCallback, useContext, useEffect, useRef, useState, type ReactNode } from "react";
import { toast } from "sonner";
import { api, toPlugin } from "./api";
import { useAuth } from "./auth";
import type { Plugin, Settings } from "./types";
import i18n from "../i18n";

/** 与服务端 store.DefaultSettings 一致；登录前、或接口还没返回时用它。 */
export const defaultSettings: Settings = {
  orgName: "",
  baseUrl: "",
  defaultLanguage: "zh",
  scope: "@company",
  admissionMode: "allowlist",
  scoring: {
    enabled: true,
    showUpstreamScore: true,
    allowEmployeeRating: true,
    minPreloadScore: 60,
  },
  upstream: {
    mode: "online",
    dshfindFeed: true,
    registry: "npmmirror",
    customRegistry: "",
    autoSync: "review",
  },
  distribution: {
    enabled: true,
    autoSync: "notify",
    keepVersions: 3,
    offerDmg: false,
  },
  brand: {
    enabled: false,
    name: "",
    logoRev: "",
    title: "",
    logoShape: "circle",
    theme: { preset: "", colors: {}, tokens: {} },
  },
  auth: "local",
  connect: {
    enabled: true,
    model: true,
    defaultModel: "",
    market: true,
    installRequired: true,
    pollSeconds: 60,
    deviceTokenDays: 90,
    allowManualTokens: true,
    agentEnabled: true,
  },
  registry: {
    proxyMode: "cache",
    extraScopes: [],
    requireToken: true,
  },
  embed: { crossSite: null, allowedOrigins: [], apps: [], sessionMinutes: 0 },
};

interface Store {
  /** 系统设置（服务端）；修改后约 0.6 秒合并保存。 */
  settings: Settings;
  updateSettings: (patch: (s: Settings) => Settings) => void;
  resetSettings: () => void;
  /** 员工可见的已上架能力（/api/catalog）。 */
  plugins: Plugin[];
  pluginsLoading: boolean;
  reloadPlugins: () => void;
  /** 审核队列里的版本数（有审核权限时）。 */
  pendingCount: number;
  reloadPending: () => void;
  /** 员工访问 dsh-ee 的地址：设置里填了就用它，否则用当前页面地址。 */
  baseUrl: string;
  notify: (msg: string) => void;
}

const Ctx = createContext<Store | null>(null);

export function StoreProvider({ children }: { children: ReactNode }) {
  const { user, can } = useAuth();
  const [settings, setSettings] = useState<Settings>(defaultSettings);
  const [plugins, setPlugins] = useState<Plugin[]>([]);
  const [pluginsLoading, setPluginsLoading] = useState(true);
  const [pendingCount, setPendingCount] = useState(0);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latest = useRef(settings);
  latest.current = settings;

  const userId = user?.id ?? 0;
  const canPortal = can("portal.view");
  const canReview = can("plugins.manage");

  useEffect(() => {
    if (!userId) return;
    api.settings().then(setSettings, () => {});
  }, [userId]);

  const reloadPlugins = useCallback(() => {
    if (!userId || !canPortal) {
      setPlugins([]);
      setPluginsLoading(false);
      return;
    }
    setPluginsLoading(true);
    api
      .catalog()
      .then((cs) => setPlugins(cs.map(toPlugin)))
      .catch(() => setPlugins([]))
      .finally(() => setPluginsLoading(false));
  }, [userId, canPortal]);

  const reloadPending = useCallback(() => {
    if (!userId || !canReview) {
      setPendingCount(0);
      return;
    }
    Promise.all([api.reviewQueue(), api.hubReview().catch(() => ({ items: [], scopes: [] }))]).then(
      ([q, h]) => setPendingCount(q.length + h.items.length + h.scopes.length),
      () => {},
    );
  }, [userId, canReview]);

  useEffect(reloadPlugins, [reloadPlugins]);
  useEffect(reloadPending, [reloadPending]);

  const save = useCallback((next: Settings) => {
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      api.saveSettings(next).then(
        (saved) => {
          // 期间又改了别的就不覆盖，等下一次保存
          if (latest.current === next) setSettings(saved);
        },
        (e: Error) => toast(i18n.t("registry.settingsSaveFailed", { msg: e.message })),
      );
    }, 600);
  }, []);

  const store: Store = {
    settings,
    updateSettings: (patch) =>
      setSettings((s) => {
        const next = patch(s);
        save(next);
        return next;
      }),
    resetSettings: () => {
      setSettings(defaultSettings);
      save(defaultSettings);
    },
    plugins,
    pluginsLoading,
    reloadPlugins,
    pendingCount,
    reloadPending,
    baseUrl: (settings.baseUrl || window.location.origin).replace(/\/$/, ""),
    notify: (msg) => void toast(msg),
  };

  return <Ctx.Provider value={store}>{children}</Ctx.Provider>;
}

export function useStore() {
  const s = useContext(Ctx);
  if (!s) throw new Error("useStore must be used inside StoreProvider");
  return s;
}

/** 评分相关 UI 统一走这里判断，避免各页面各自读开关读出不一致。 */
export function useScoring() {
  const { scoring } = useStore().settings;
  return {
    upstream: scoring.enabled && scoring.showUpstreamScore,
    rating: scoring.enabled && scoring.allowEmployeeRating,
    any: scoring.enabled && (scoring.showUpstreamScore || scoring.allowEmployeeRating),
    minPreloadScore: scoring.minPreloadScore,
  };
}
