import { t } from "./i18n/index.js";

export interface BrandColorPair {
  light: string;
  dark: string;
}

export interface BrandViewData {
  enabled: boolean;
  name: string;
  logo_url: string;
  revision: string;
  /** 首屏大标题；空时保留 DSH 自带的标题。 */
  title?: string;
  /** circle 圆形裁切 / rounded 圆角方形 / plain 原样。 */
  logo_shape?: string;
  /** DSH 设计变量（--dsw-*）→ 浅色 / 深色取值；空串表示该模式用默认。 */
  theme?: Record<string, BrandColorPair>;
}

function shapeStyle(shape: string | undefined, size: number) {
  if (shape === "plain") return { borderRadius: 0, objectFit: "contain" as const };
  if (shape === "rounded") return { borderRadius: Math.round(size * 0.22), objectFit: "cover" as const };
  return { borderRadius: "50%", objectFit: "cover" as const };
}

export function brandMark(createElement: (...args: any[]) => any, brand: BrandViewData, props: { size?: number } = {}) {
  const size = props.size ?? 32;
  const shape = shapeStyle(brand.logo_shape, size);
  if (brand.logo_url) {
    return createElement("img", {
      src: brand.logo_url,
      alt: brand.name,
      width: size,
      height: size,
      style: { width: size, height: size, ...shape, display: "block" },
    });
  }
  const letter = (brand.name.trim().charAt(0) || t("brandView.fallbackLetter")).toUpperCase();
  return createElement("span", {
    "aria-label": brand.name,
    style: {
      width: size,
      height: size,
      borderRadius: brand.logo_shape === "circle" || !brand.logo_shape ? "50%" : Math.round(size * 0.22),
      display: "inline-flex",
      alignItems: "center",
      justifyContent: "center",
      background: "#eef1f5",
      color: "#344054",
      fontSize: Math.max(12, Math.round(size * 0.42)),
      fontWeight: 600,
    },
  }, letter);
}

export function brandName(createElement: (...args: any[]) => any, brand: BrandViewData) {
  return createElement("span", { "data-dsh-ee-brand": brand.revision || "neutral" }, brand.name);
}

/**
 * 首屏 logo 位置。设了标题时把 logo 和标题画在一起，再由 {@link HERO_TITLE_CSS} 藏起 DSH 自带的标题
 * （DSH 的标题文案来自它自己的词典，插件不能覆盖同一个词条）。
 */
export function heroBrand(createElement: (...args: any[]) => any, brand: BrandViewData, props: { size?: number } = {}) {
  const title = (brand.title ?? "").trim();
  if (!title) return brandMark(createElement, brand, props);
  const size = Math.round((props.size ?? 34) * 1.4);
  return createElement("span", {
    "data-dsh-ee-hero-title": brand.revision || "neutral",
    style: { display: "inline-flex", alignItems: "center", gap: 14, fontWeight: 600, letterSpacing: "0.02em" },
  }, brandMark(createElement, brand, { size }), createElement("span", null, title));
}

/**
 * 藏起首屏里紧跟在品牌 logo 后面的 DSH 标题组（含「预览版」角标）。
 * DSH 的首屏是 `<span 鱼形热区>{logo 位置}</span><span 标题组>…</span>`；只认 span，
 * 不会碰到外层的 div。浏览器不支持 :has 时两段标题都会显示，不影响使用。
 */
export const HERO_TITLE_CSS = "span:has([data-dsh-ee-hero-title]) + span:not(:has([data-dsh-ee-hero-title])) { display: none !important; }";

const TOKEN_NAME = /^--dsw-[a-z0-9-]{1,80}$/;

/** 把目录里的配色整理成 DSH 主题服务要的形状；没有可用的变量时返回 undefined。 */
export function themeOverrides(brand: BrandViewData | undefined): Record<string, BrandColorPair> | undefined {
  if (!brand?.enabled || !brand.theme || typeof brand.theme !== "object") return undefined;
  const out: Record<string, BrandColorPair> = {};
  for (const [name, pair] of Object.entries(brand.theme)) {
    if (!TOKEN_NAME.test(name) || !pair || typeof pair !== "object") continue;
    const light = typeof pair.light === "string" ? pair.light : "";
    const dark = typeof pair.dark === "string" ? pair.dark : "";
    if (light || dark) out[name] = { light, dark };
  }
  return Object.keys(out).length ? out : undefined;
}

/** 品牌内容的指纹：没变时不重绘、不重设主题。 */
export function brandKey(brand: BrandViewData | undefined): string {
  return JSON.stringify(brand ?? null);
}

/** 管理员在网页上改了品牌后，DSH 不用重启：每分钟（以及窗口回到前台时）重读一次，有变化就重绘。 */
export const BRAND_POLL_MS = 60_000;

/**
 * 品牌状态。read 可以晚点再给：插件一启动就先用上次缓存的品牌注册界面，连上 dsh-ee 以后再读最新的。
 * 每次读到新内容都通知订阅者（界面重绘、主题重设、写缓存）。
 */
export function createBrandStore(initial: BrandViewData, read?: () => Promise<BrandViewData>) {
  let current = initial;
  let key = brandKey(initial);
  let inflight = false;
  let reader = read;
  const listeners = new Set<() => void>();
  const set = (next: BrandViewData) => {
    const nextKey = brandKey(next);
    if (!next || nextKey === key) return;
    current = next;
    key = nextKey;
    for (const listener of [...listeners]) {
      try {
        listener();
      } catch {
        // 一个订阅者出错不影响其它
      }
    }
  };
  const refresh = async () => {
    if (inflight || !reader) return;
    inflight = true;
    try {
      set(await reader());
    } catch {
      // 读不到时保持上一次的品牌
    } finally {
      inflight = false;
    }
  };
  const subscribe = (listener: () => void) => {
    listeners.add(listener);
    return () => { listeners.delete(listener); };
  };
  return {
    get: () => current,
    set,
    refresh,
    subscribe,
    /** 连上 dsh-ee 以后接上读取函数。 */
    setReader: (next: () => Promise<BrandViewData>) => { reader = next; },
    /** 在组件里读品牌，并在品牌变化时重绘。 */
    use: (useState: (v: number) => [number, (fn: (n: number) => number) => void], useEffect: (fn: () => () => void, deps: unknown[]) => void) => {
      const [, setTick] = useState(0);
      useEffect(() => subscribe(() => setTick((n) => n + 1)), []);
      return current;
    },
    start: () => {
      const timer = setInterval(() => void refresh(), BRAND_POLL_MS);
      const onVisible = () => { if (typeof document !== "undefined" && !document.hidden) void refresh(); };
      if (typeof document !== "undefined") document.addEventListener("visibilitychange", onVisible);
      return () => {
        clearInterval(timer);
        if (typeof document !== "undefined") document.removeEventListener("visibilitychange", onVisible);
      };
    },
  };
}

export type BrandStore = ReturnType<typeof createBrandStore>;

const CACHE_KEY = "dsh-ee:brand";

/** 上次读到的品牌（存在浏览器本地）：DSH 一打开就显示企业 logo，不先闪一下 DSH 的鱼。 */
export function loadCachedBrand(storage: Pick<Storage, "getItem"> | undefined = globalThis.localStorage): BrandViewData | undefined {
  try {
    const raw = storage?.getItem(CACHE_KEY);
    if (!raw) return undefined;
    const brand = JSON.parse(raw);
    if (!brand || typeof brand !== "object" || typeof brand.enabled !== "boolean" || typeof brand.name !== "string") return undefined;
    return brand as BrandViewData;
  } catch {
    return undefined;
  }
}

export function saveCachedBrand(brand: BrandViewData, storage: Pick<Storage, "setItem"> | undefined = globalThis.localStorage): void {
  try {
    storage?.setItem(CACHE_KEY, JSON.stringify(brand));
  } catch {
    // 本地存储不可用（隐私模式、配额满）时不缓存
  }
}

/**
 * 包一层防护：品牌组件渲染出错时显示企业名称首字母，而不是抛出去——
 * 宿主会撤下出错的组件、退回 DSH 自带的鱼，企业 logo 就「悄悄换不掉」了。
 */
export function safeRender(render: () => any, fallback: () => any): any {
  try {
    return render();
  } catch (error) {
    if (typeof console !== "undefined") console.warn("[dsh-ee] 品牌组件渲染失败，改用首字母", error);
    try {
      return fallback();
    } catch {
      return null;
    }
  }
}

/** 品牌开启时浏览器标签页（Desktop 是窗口标题）显示企业名称，图标换成企业 logo。 */
export function pageBranding(doc: Document, brand: BrandViewData): () => void {
  if (!brand.enabled || !brand.name) return () => {};
  const restoreTitle = doc.title;
  const apply = () => {
    if (doc.title !== brand.name) doc.title = brand.name;
  };
  apply();
  // DSH 切换会话时会改标题，改回来
  const titleNode = doc.querySelector("title");
  const MO = (doc.defaultView as any)?.MutationObserver;
  const observer = titleNode && MO ? new MO(apply) : undefined;
  observer?.observe(titleNode, { childList: true, characterData: true, subtree: true });
  let icon: HTMLLinkElement | undefined;
  const hidden: { el: Element; rel: string }[] = [];
  if (brand.logo_url) {
    for (const el of doc.querySelectorAll('link[rel~="icon"]')) {
      hidden.push({ el, rel: el.getAttribute("rel") || "icon" });
      el.setAttribute("rel", "dsh-ee-hidden-icon");
    }
    icon = doc.createElement("link");
    icon.rel = "icon";
    icon.href = brand.logo_url;
    icon.dataset.dshEe = "brand";
    doc.head.appendChild(icon);
  }
  return () => {
    observer?.disconnect();
    icon?.remove();
    for (const h of hidden) h.el.setAttribute("rel", h.rel);
    if (doc.title === brand.name) doc.title = restoreTitle;
  };
}
