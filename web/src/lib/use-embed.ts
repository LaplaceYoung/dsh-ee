import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router";
import {
  EMBED_MOBILE_MAX_WIDTH,
  applyEmbedTheme,
  embedContentHeight,
  embedViewport,
  isEmbedMessage,
  isMobileEmbedViewport,
  isTrustedParentMessage,
  parseEmbedOptions,
  postToHost,
  resolvedDocumentTheme,
  type EmbedError,
  type EmbedOptions,
  type EmbedTheme,
} from "./embed";

/** 解析嵌入参数（?embed=1&host=…&fit=auto&theme=…&lang=…）；非嵌入模式全部为默认值。 */
export function useEmbedOptions(): EmbedOptions {
  const { search } = useLocation();
  return useMemo(() => parseEmbedOptions(search), [search]);
}

/**
 * 主题双向：`?theme=light|dark|system` 是初始偏好，宿主还可以随时发 `dsh-ee:theme` 覆盖；
 * 应用只改 DOM 不写 localStorage。`system` 跟随 prefers-color-scheme 实时变化，
 * 一旦宿主显式指定 light/dark，就以宿主为准（系统主题变化不再覆盖它）。
 */
export function useEmbedTheme(options: EmbedOptions): void {
  const hostOverride = useRef<EmbedTheme | null>(null);
  const { embed, host, theme } = options;

  useEffect(() => {
    if (!embed) return;
    // URL 偏好变化（或重新进入嵌入页）时，宿主覆盖失效，回到 URL/系统偏好。
    hostOverride.current = null;
    const media = window.matchMedia("(prefers-color-scheme: dark)");
    const applyPreference = () => {
      if (hostOverride.current) return;
      if (theme === "system") applyEmbedTheme(media.matches ? "dark" : "light");
      else if (theme === "light" || theme === "dark") applyEmbedTheme(theme);
    };
    applyPreference();
    // 先告诉宿主当前实际主题；宿主在 ready 之前收到也要容忍（协议里 ready 也带 theme）。
    postToHost(options, "dsh-ee:theme", { theme: resolvedDocumentTheme() });
    if (theme !== "system") return;
    const onSystemChange = () => {
      if (hostOverride.current) return;
      applyPreference();
      postToHost(options, "dsh-ee:theme", { theme: resolvedDocumentTheme() });
    };
    media.addEventListener("change", onSystemChange);
    return () => media.removeEventListener("change", onSystemChange);
    // 只用嵌入相关的原始参数做依赖：站内切页签时 options 对象会变，但不应重置宿主覆盖。
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [embed, host, theme]);

  useEffect(() => {
    if (!embed || !host) return;
    const onMessage = (event: MessageEvent) => {
      if (!isTrustedParentMessage(event.origin, event.source, host, window.parent)) return;
      if (!isEmbedMessage(event.data, "dsh-ee:theme")) return;
      const next = (event.data as { theme?: unknown }).theme;
      if (next !== "light" && next !== "dark") return;
      hostOverride.current = next;
      applyEmbedTheme(next);
      postToHost(options, "dsh-ee:theme", { theme: next });
    };
    window.addEventListener("message", onMessage);
    return () => window.removeEventListener("message", onMessage);
  }, [embed, host, options]);
}

/**
 * `fit=auto`：用 ResizeObserver 上报**内容高度**，节流到一帧，高度上限 EMBED_MAX_HEIGHT。
 * 观察页面根节点而不是 `documentElement`：宿主按内容撑高后视口高度会变，
 * 但页面根节点只在内容真的变化时才变，才能既撑开又缩回。
 * 宿主收到 `dsh-ee:resize` 后自己决定是否按高度设置 iframe；默认 fill 不主动上报。
 */
export function useEmbedResize(options: EmbedOptions): void {
  const { embed, fit, host } = options;
  useEffect(() => {
    if (!embed || fit !== "auto" || !host) return;
    let frame = 0;
    let last = "";
    const report = () => {
      frame = 0;
      const viewport = embedViewport();
      const height = embedContentHeight();
      const width = document.documentElement.clientWidth || viewport.width;
      const key = `${height}x${width}`;
      if (key === last) return;
      last = key;
      postToHost(options, "dsh-ee:resize", { height, width, mobile: viewport.mobile });
    };
    const schedule = () => {
      if (!frame) frame = requestAnimationFrame(report);
    };
    schedule();
    const root = document.getElementById("root");
    const observer = new ResizeObserver(schedule);
    observer.observe(root ?? document.body);
    // 视口变化（宿主改 iframe 尺寸/手机旋转）与字体、图片加载完成后再测一轮。
    window.addEventListener("resize", schedule);
    window.addEventListener("load", schedule);
    void document.fonts?.ready.then(() => schedule()).catch(() => {});
    return () => {
      observer.disconnect();
      window.removeEventListener("resize", schedule);
      window.removeEventListener("load", schedule);
      if (frame) cancelAnimationFrame(frame);
    };
  }, [embed, fit, host, options]);
}

/**
 * 移动端判定（窄屏 <640px 或移动 UA）：只用于显示「在浏览器打开」常驻兜底，
 * 不在 iframe 里自动顶层跳转（会被弹窗拦截，也丢宿主上下文）。内容照常显示。
 */
export function useEmbedMobile(options: EmbedOptions): boolean {
  const { embed } = options;
  const [mobile, setMobile] = useState(false);
  useEffect(() => {
    if (!embed) {
      setMobile(false);
      return;
    }
    const query = window.matchMedia(`(max-width: ${EMBED_MOBILE_MAX_WIDTH - 1}px)`);
    const update = () => setMobile(isMobileEmbedViewport(window.innerWidth, navigator.userAgent));
    update();
    query.addEventListener("change", update);
    return () => query.removeEventListener("change", update);
  }, [embed]);
  return mobile;
}

/** 加载失败 / 未登录 / 无权限 / 握手失败：给宿主发 `dsh-ee:error`（带 stage）；同一条错误只发一次。 */
export function useEmbedError(options: EmbedOptions, error: EmbedError | null): void {
  const sent = useRef("");
  useEffect(() => {
    if (!options.embed || !error) return;
    const key = `${error.stage}\n${error.code}\n${error.message}`;
    if (sent.current === key) return;
    sent.current = key;
    postToHost(options, "dsh-ee:error", {
      code: error.code,
      message: error.message,
      retryable: !!error.retryable,
      stage: error.stage,
    });
  }, [options, error, error?.code, error?.message, error?.retryable, error?.stage]);
}
