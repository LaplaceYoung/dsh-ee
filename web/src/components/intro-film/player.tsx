// 首页上播放介绍短片：按容器宽度缩放，循环播放，点一下暂停。
// 看不见的时候不走时钟；系统开了「减少动态效果」就停在片尾那一帧，不自动播。
import { useEffect, useRef, useState } from "react";
import { FILM_HEIGHT, FILM_SECONDS, FILM_WIDTH, IntroFilm } from "./film";
import type { FilmLang } from "./copy";

export function IntroFilmPlayer({ lang, label, className }: { lang: FilmLang; label: string; className?: string }) {
  const box = useRef<HTMLDivElement>(null);
  const [scale, setScale] = useState(0);
  const [t, setT] = useState(0);
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

  const running = !paused && visible && !still;
  useEffect(() => {
    if (!running) return;
    let raf = 0;
    let last = performance.now();
    const tick = (now: number) => {
      // 标签页切到后台再回来，间隔会很大：一帧最多走 0.1 秒，不跳片
      const dt = Math.min(0.1, (now - last) / 1000);
      last = now;
      setT((prev) => (prev + dt) % FILM_SECONDS);
      raf = requestAnimationFrame(tick);
    };
    raf = requestAnimationFrame(tick);
    return () => cancelAnimationFrame(raf);
  }, [running]);

  const time = still ? FILM_SECONDS : t;
  return (
    <div
      ref={box}
      role="img"
      aria-label={label}
      data-intro-film
      data-paused={paused || still ? "true" : "false"}
      onClick={() => !still && setPaused((p) => !p)}
      className={className}
      style={{ position: "relative", aspectRatio: `${FILM_WIDTH} / ${FILM_HEIGHT}`, overflow: "hidden", cursor: still ? "default" : "pointer" }}
    >
      {scale > 0 && (
        <div aria-hidden style={{ width: FILM_WIDTH, height: FILM_HEIGHT, transform: `scale(${scale})`, transformOrigin: "0 0" }}>
          <IntroFilm t={time} lang={lang} />
        </div>
      )}
      {!still && (
        <div aria-hidden style={{ position: "absolute", left: 0, bottom: 0, height: 3, width: `${(time / FILM_SECONDS) * 100}%`, background: "#4D6BFE", opacity: 0.85 }} />
      )}
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
