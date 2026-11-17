// 首页上播放一分钟产品短片：按容器宽度缩放，循环播放，点一下暂停。
// 短片是代码画的，不是视频文件：画面和时间轴由 promo/openmotion/build-film.mjs 生成在 generated/ 里，
// 同一份内容也渲染成 MP4（promo/out）。画面放在影子 DOM 里，短片的样式不会影响页面，页面的样式也进不去。
// 看不见的时候不走时钟；系统开了「减少动态效果」就停在片尾那一帧，不自动播。
import { useEffect, useRef, useState } from "react";
import inter400 from "./fonts/Inter-400.woff2?url";
import inter700 from "./fonts/Inter-700.woff2?url";
import { createFilm, FILM_HEIGHT, FILM_SECONDS, FILM_WIDTH } from "./generated/film-runtime";
import { css } from "./generated/film-style";

export type ProductFilmLang = "en" | "zh" | "ja";

/** 各语言的画面按需加载，首页只下载用到的那一份。 */
const markup: Record<ProductFilmLang, () => Promise<{ html: string }>> = {
  en: () => import("./generated/film.en"),
  zh: () => import("./generated/film.zh"),
  ja: () => import("./generated/film.ja"),
};

/** 片尾那一帧：标志和网址都已出齐。 */
const END_FRAME = FILM_SECONDS - 0.5;

let fonts: Promise<unknown> | null = null;
/** 影子 DOM 里的 @font-face 不生效，短片的字体在页面上注册一次。加载失败就用系统字体。 */
function loadFonts() {
  fonts ??= Promise.all(
    [
      [inter400, "400"],
      [inter700, "700"],
    ].map(async ([url, weight]) => {
      const face = new FontFace("FilmSans", `url(${url}) format("woff2")`, { weight });
      document.fonts.add(await face.load());
    }),
  ).catch(() => undefined);
  return fonts;
}

export function ProductFilmPlayer({ lang, label, className }: { lang: ProductFilmLang; label: string; className?: string }) {
  const box = useRef<HTMLDivElement>(null);
  const stage = useRef<HTMLDivElement>(null);
  const bar = useRef<HTMLDivElement>(null);
  const draw = useRef<((t: number) => void) | null>(null);
  const time = useRef(0);
  const [scale, setScale] = useState(0);
  const [ready, setReady] = useState(false);
  const [paused, setPaused] = useState(false);
  const [visible, setVisible] = useState(true);
  const [still] = useState(() => typeof window !== "undefined" && window.matchMedia("(prefers-reduced-motion: reduce)").matches);

  useEffect(() => {
    const el = box.current;
    if (!el) return;
    const fit = () => setScale(el.clientWidth / FILM_WIDTH);
    fit();
    const sizes = new ResizeObserver(fit);
    sizes.observe(el);
    const seen = new IntersectionObserver(([entry]) => setVisible(entry.isIntersecting));
    seen.observe(el);
    return () => {
      sizes.disconnect();
      seen.disconnect();
    };
  }, []);

  // 换语言：换画面，时间不变，接着播
  useEffect(() => {
    let stale = false;
    setReady(false);
    void Promise.all([markup[lang](), loadFonts()]).then(([film]) => {
      const host = stage.current;
      if (stale || !host) return;
      const root = host.shadowRoot ?? host.attachShadow({ mode: "open" });
      // 内容是构建时生成的固定画面，不含用户输入
      root.innerHTML = `<style>${css}</style>${film.html}`;
      draw.current = createFilm(root);
      draw.current(still ? END_FRAME : time.current);
      setReady(true);
    });
    return () => {
      stale = true;
    };
  }, [lang, still]);

  const running = ready && !paused && visible && !still;
  useEffect(() => {
    if (!running) return;
    let raf = 0;
    let last = performance.now();
    const tick = (now: number) => {
      // 标签页切到后台再回来，间隔会很大：一帧最多走 0.1 秒，不跳片
      const dt = Math.min(0.1, (now - last) / 1000);
      last = now;
      time.current = (time.current + dt) % FILM_SECONDS;
      draw.current?.(time.current);
      if (bar.current) bar.current.style.width = `${(time.current / FILM_SECONDS) * 100}%`;
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [running]);

  return (
    <div
      ref={box}
      role="img"
      aria-label={label}
      data-intro-film
      data-film-lang={lang}
      data-paused={paused || still ? "true" : "false"}
      onClick={() => !still && setPaused((p) => !p)}
      className={className}
      style={{ position: "relative", aspectRatio: `${FILM_WIDTH} / ${FILM_HEIGHT}`, overflow: "hidden", cursor: still ? "default" : "pointer", background: "#f5f3e9" }}
    >
      <div aria-hidden style={{ width: FILM_WIDTH, height: FILM_HEIGHT, transform: `scale(${scale})`, transformOrigin: "0 0", visibility: scale > 0 && ready ? "visible" : "hidden" }}>
        <div ref={stage} style={{ width: FILM_WIDTH, height: FILM_HEIGHT }} />
      </div>
      {!still && <div ref={bar} aria-hidden style={{ position: "absolute", left: 0, bottom: 0, height: 3, width: 0, background: "#4D6BFE", opacity: 0.85 }} />}
      {paused && (
        <div aria-hidden style={{ position: "absolute", inset: 0, display: "flex", alignItems: "center", justifyContent: "center", background: "rgba(10,37,64,0.18)" }}>
          <span style={{ width: 64, height: 64, borderRadius: 32, background: "#fff", display: "flex", alignItems: "center", justifyContent: "center", boxShadow: "0 8px 24px rgba(10,37,64,0.25)" }}>
            <svg width="24" height="24" viewBox="0 0 24 24">
              <path d="M8 5v14l11-7z" fill="#0A2540" />
            </svg>
          </span>
        </div>
      )}
    </div>
  );
}
