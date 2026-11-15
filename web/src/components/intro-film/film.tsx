// 30 秒介绍短片。整部片子是时间 t（秒）的纯函数：没有 hook、没有定时器、不读界面主题，
// 所以同一份代码既能在首页用 requestAnimationFrame 播，也能被 promo/ 逐帧渲染成 MP4，两边一帧不差。
// 画布固定 1280×720，外面按容器宽度整体缩放。
// 风格：点阵底、超大标题、高亮词套一个设计工具式的选框，卡片快速弹入；片尾转深色，飘碎片。
import type { CSSProperties, ReactNode } from "react";
import { WhaleMark } from "../whale-mark";
import { filmCopy, type FilmCopy, type FilmLang } from "./copy";

export const FILM_SECONDS = 30;
export const FILM_WIDTH = 1280;
export const FILM_HEIGHT = 720;

const BRAND = "#2F6BFF";
const INK = "#0B0F17";
const MUTED = "#6B7686";
const LINE = "#E4E8EF";
const BG = "#F6F7FA";
const GREEN = "#10A37F";
const RED = "#E5484D";
const AMBER = "#F08C00";
const VIOLET = "#7C4DFF";
const FONT = '"Inter", "PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", system-ui, sans-serif';
const MONO = '"JetBrains Mono", "SFMono-Regular", Menlo, Consolas, monospace';
const SHADOW = "0 18px 50px rgba(11,15,23,0.10), 0 0 0 1px rgba(11,15,23,0.05)";

/** 场景起止（秒）。相邻场景交叠 FADE 秒。 */
const FADE = 0.22;
const CUTS = [0, 4, 10.5, 16, 21, 26.2, FILM_SECONDS] as const;

const clamp01 = (x: number) => Math.max(0, Math.min(1, x));
const easeOut = (x: number) => 1 - Math.pow(1 - clamp01(x), 4);
/** 带一点回弹的缓出，用在弹入的药丸和小方块上。 */
const easeBack = (x: number) => {
  const v = clamp01(x) - 1;
  return 1 + 2.4 * v * v * v + 1.4 * v * v;
};
/** 从 start 秒起，用 dur 秒从 0 走到 1。 */
const rise = (s: number, start: number, dur = 0.5) => easeOut((s - start) / dur);
const pop = (p: number, dy = 20): CSSProperties => ({ opacity: clamp01(p * 1.4), transform: `translateY(${(1 - p) * dy}px)` });
/** 大标题的入场：上浮、从模糊到清楚。 */
const headIn = (p: number): CSSProperties => ({ opacity: clamp01(p * 1.5), transform: `translateY(${(1 - p) * 46}px)`, filter: `blur(${(1 - p) * 10}px)` });
const typed = (text: string, s: number, start: number, cps: number) => Array.from(text).slice(0, Math.max(0, Math.floor((s - start) * cps))).join("");
/** 固定的伪随机数：同一个 (i, k) 永远得到同一个 0–1 的数。 */
const rnd = (i: number, k: number) => {
  const x = Math.sin(i * 127.1 + k * 311.7) * 43758.5453;
  return x - Math.floor(x);
};

type SceneProps = { c: FilmCopy; s: number; cjk: boolean };

export function IntroFilm({ t, lang }: { t: number; lang: FilmLang }) {
  const c = filmCopy[lang];
  const time = Math.max(0, Math.min(FILM_SECONDS, t));
  const scenes = [Scene1, Scene2, Scene3, Scene4, Scene5, Scene6];
  const mm = String(Math.floor(time)).padStart(2, "0");
  const ff = String(Math.floor((time % 1) * 30)).padStart(2, "0");
  return (
    <div style={{ position: "relative", width: FILM_WIDTH, height: FILM_HEIGHT, overflow: "hidden", background: BG, color: INK, fontFamily: FONT, lineHeight: 1.35, textAlign: "left" }}>
      {scenes.map((Scene, i) => {
        const from = CUTS[i];
        const to = CUTS[i + 1];
        if (time < from - FADE || time > to) return null;
        const last = i === scenes.length - 1;
        const fadeIn = i === 0 ? 1 : clamp01((time - (from - FADE)) / FADE);
        const fadeOut = last ? 1 : clamp01((to - time) / FADE);
        const s = time - from;
        return (
          <div key={i} style={{ position: "absolute", inset: 0, opacity: Math.min(fadeIn, fadeOut) }}>
            {last ? <DarkBackdrop s={s} /> : <Backdrop />}
            <Scene c={c} s={s} cjk={lang === "zh"} />
            <Corners dark={last} index={i} timecode={`00:${mm}:${ff}`} />
          </div>
        );
      })}
    </div>
  );
}

// ---- 公用 ----

function Backdrop() {
  return (
    <div style={{ position: "absolute", inset: 0, background: BG }}>
      <div style={{ position: "absolute", inset: 0, backgroundImage: "radial-gradient(rgba(11,15,23,0.16) 1px, transparent 1.4px)", backgroundSize: "26px 26px" }} />
      <div style={{ position: "absolute", inset: 0, background: "radial-gradient(ellipse 70% 60% at 50% 50%, rgba(246,247,250,0.96), rgba(246,247,250,0.35) 70%, rgba(246,247,250,0))" }} />
      <div style={{ position: "absolute", left: "8%", right: "8%", bottom: -260, height: 420, borderRadius: "50%", background: "radial-gradient(ellipse at center, rgba(47,107,255,0.16), rgba(47,107,255,0))" }} />
    </div>
  );
}

function Corners({ dark, index, timecode }: { dark: boolean; index: number; timecode: string }) {
  const style: CSSProperties = { position: "absolute", fontFamily: MONO, fontSize: 10, letterSpacing: 1.2, color: dark ? "rgba(255,255,255,0.45)" : "rgba(11,15,23,0.38)" };
  return (
    <>
      <span style={{ ...style, left: 26, top: 20 }}>DSH-EE · INTRO</span>
      <span style={{ ...style, right: 26, top: 20 }}>
        0{index + 1} / 06
      </span>
      <span style={{ ...style, left: 26, bottom: 18 }}>{timecode}</span>
      <span style={{ ...style, right: 26, bottom: 18 }}>
        {FILM_WIDTH} × {FILM_HEIGHT}
      </span>
    </>
  );
}

/** 横竖两条细参考线，交在 (x, y)。 */
function Guides({ x, y, p }: { x: number; y: number; p: number }) {
  const color = "rgba(47,107,255,0.22)";
  return (
    <>
      <div style={{ position: "absolute", left: 0, top: y, height: 1, width: `${p * 100}%`, background: color }} />
      <div style={{ position: "absolute", top: 0, left: x, width: 1, height: `${p * 100}%`, background: color }} />
    </>
  );
}

/** 设计工具式的选框：细边、四个角的小方块、下面一个尺寸标签。 */
function Sel({ p, size, children }: { p: number; size: string; children: ReactNode }) {
  const handle = (pos: CSSProperties): CSSProperties => ({ position: "absolute", width: 8, height: 8, background: "#fff", border: `1.5px solid ${BRAND}`, boxSizing: "border-box", ...pos });
  return (
    <span style={{ position: "relative", display: "inline-block" }}>
      {children}
      <span style={{ position: "absolute", inset: -9, border: `1.5px solid ${BRAND}`, opacity: clamp01(p * 2), transform: `scale(${1.06 - 0.06 * p})` }}>
        <span style={handle({ left: -5, top: -5 })} />
        <span style={handle({ right: -5, top: -5 })} />
        <span style={handle({ left: -5, bottom: -5 })} />
        <span style={handle({ right: -5, bottom: -5 })} />
        <span style={{ position: "absolute", left: "50%", bottom: -26, transform: "translateX(-50%)", background: BRAND, color: "#fff", fontFamily: MONO, fontSize: 11, fontWeight: 600, letterSpacing: 0, lineHeight: 1, padding: "4px 7px", borderRadius: 3, whiteSpace: "nowrap" }}>{size}</span>
      </span>
    </span>
  );
}

function Pill({ color, children, style }: { color: string; children: ReactNode; style?: CSSProperties }) {
  return <span style={{ display: "inline-block", background: color, color: "#fff", borderRadius: 999, padding: "0.06em 0.42em 0.12em", whiteSpace: "nowrap", ...style }}>{children}</span>;
}

function Card({ style, children }: { style?: CSSProperties; children: ReactNode }) {
  return <div style={{ position: "absolute", background: "#fff", borderRadius: 16, boxShadow: SHADOW, boxSizing: "border-box", ...style }}>{children}</div>;
}

function Chip({ children, tone = "plain" }: { children: ReactNode; tone?: "plain" | "brand" | "ok" | "bad" | "wait" }) {
  const tones = {
    plain: { background: "#EEF1F5", color: "#3A4556" },
    brand: { background: "rgba(47,107,255,0.11)", color: BRAND },
    ok: { background: "rgba(16,163,127,0.12)", color: GREEN },
    bad: { background: "rgba(229,72,77,0.11)", color: RED },
    wait: { background: "rgba(240,140,0,0.13)", color: "#B26A00" },
  } as const;
  return <span style={{ display: "inline-block", padding: "3px 10px", borderRadius: 999, fontSize: 13, fontWeight: 600, whiteSpace: "nowrap", ...tones[tone] }}>{children}</span>;
}

function Check({ ok = true }: { ok?: boolean }) {
  return (
    <span style={{ display: "inline-flex", width: 18, height: 18, borderRadius: 9, alignItems: "center", justifyContent: "center", background: ok ? GREEN : RED, color: "#fff", fontSize: 11, fontWeight: 800, flexShrink: 0 }}>
      {ok ? "✓" : "✕"}
    </span>
  );
}

const headline = (cjk: boolean, size: number): CSSProperties => ({ fontSize: size, fontWeight: 700, lineHeight: 1.12, letterSpacing: cjk ? "-0.01em" : "-0.045em", whiteSpace: "nowrap" });

// ---- 1：AI 不认识你的公司 ----

function Scene1({ c, s, cjk }: SceneProps) {
  const starts = [0.3, 1.5, 2.7];
  const colors = [GREEN, AMBER, BRAND];
  const i = s < starts[1] ? 0 : s < starts[2] ? 1 : 2;
  const p = easeBack((s - starts[i]) / 0.42);
  const word = c.s1.words[i];
  const size = cjk ? 92 : 70;
  const width = Math.round(Array.from(word).length * (cjk ? 92 : 36) + 78);
  return (
    <>
      <Guides x={300} y={302} p={rise(s, 0, 0.9)} />
      <div style={{ position: "absolute", left: 0, right: 0, top: 302, height: 116, display: "flex", alignItems: "center", justifyContent: "center", gap: 22, ...headline(cjk, size) }}>
        <span style={headIn(rise(s, 0.05, 0.6))}>{c.s1.lead}</span>
        <span style={{ display: "inline-block", opacity: clamp01((s - starts[i]) / 0.12), transform: `scale(${0.82 + 0.18 * p})`, transformOrigin: "left center" }}>
          <Sel p={rise(s, starts[i] + 0.12, 0.35)} size={`${width} × ${Math.round(size * 1.26)}`}>
            <Pill color={colors[i]}>{word}</Pill>
          </Sel>
        </span>
      </div>
    </>
  );
}

// ---- 2：接上 dsh-ee 之后 ----

function Scene2({ c, s, cjk }: SceneProps) {
  const ask = 1 - rise(s, 2.0, 0.35); // 提问框 2 秒后收起
  const q = typed(c.s2.q, s, 0.45, cjk ? 20 : 46);
  const dots = [BRAND, GREEN, AMBER];
  return (
    <>
      {ask > 0.01 && (
        <div style={{ position: "absolute", left: 250, top: 250, width: 780, opacity: ask, transform: `translateY(${(1 - ask) * -30}px) scale(${0.96 + 0.04 * ask})` }}>
          <div style={{ display: "flex", gap: 10, marginBottom: 14 }}>
            {c.s2.chips.map((chip, i) => (
              <span key={chip} style={{ display: "inline-flex", alignItems: "center", gap: 8, background: "#fff", borderRadius: 999, boxShadow: SHADOW, padding: "7px 14px", fontSize: 15, fontWeight: 600, fontFamily: i === 0 ? MONO : FONT, ...pop(easeBack((s - 0.1 - i * 0.08) / 0.4), 14) }}>
                <span style={{ width: 8, height: 8, borderRadius: 4, background: dots[i] }} />
                {chip}
              </span>
            ))}
          </div>
          <div style={{ background: "#fff", borderRadius: 20, boxShadow: SHADOW, padding: "22px 24px 16px", ...pop(rise(s, 0.2), 26) }}>
            <div style={{ fontSize: 27, fontWeight: 500, minHeight: 38 }}>
              {q}
              <span style={{ display: "inline-block", width: 2, height: 30, marginLeft: 3, verticalAlign: -5, background: BRAND, opacity: Math.floor(s * 2.5) % 2 ? 0.2 : 1 }} />
            </div>
            <div style={{ marginTop: 18, display: "flex", alignItems: "center", gap: 14 }}>
              {[0, 1, 2, 3].map((i) => (
                <span key={i} style={{ width: 18, height: 18, borderRadius: 6, border: `1.5px solid ${LINE}` }} />
              ))}
              <span style={{ marginLeft: "auto", width: 36, height: 36, borderRadius: 18, background: BRAND, color: "#fff", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 18, fontWeight: 700, transform: `scale(${s > 1.75 && s < 1.95 ? 0.86 : 1})` }}>↑</span>
            </div>
          </div>
        </div>
      )}
      <div style={{ position: "absolute", left: 70, top: 78, ...headline(cjk, 60) }}>
        <div style={headIn(rise(s, 2.15))}>{c.s2.headline[0]}</div>
        <div style={{ color: BRAND, ...headIn(rise(s, 2.35)) }}>{c.s2.headline[1]}</div>
      </div>
      <div style={{ position: "absolute", right: 70, top: 100, ...pop(easeBack((s - 2.4) / 0.45), 16) }}>
        <Sel p={rise(s, 2.7, 0.35)} size="268 × 52">
          <Pill color={BRAND} style={{ fontSize: 22, fontWeight: 600, fontFamily: MONO, padding: "10px 22px" }}>
            {c.s2.chips[0]}
          </Pill>
        </Sel>
      </div>
      {c.s2.sources.map((src, i) => {
        const start = 2.6 + i * 0.25;
        const done = s > start + 1.0;
        return (
          <Card key={src.tool} style={{ left: 70 + i * 350, top: 262, width: 330, height: 186, padding: "18px 20px", ...pop(easeBack((s - start) / 0.5), 40) }}>
            <div style={{ display: "flex", alignItems: "center", gap: 9, fontSize: 14, fontWeight: 600, fontFamily: MONO, color: MUTED }}>
              <span style={{ width: 9, height: 9, borderRadius: 5, background: i === 0 ? GREEN : AMBER }} />
              {src.tool}
              <span style={{ marginLeft: "auto", fontSize: 12 }}>{done ? "0.4s" : "…"}</span>
            </div>
            <div style={{ marginTop: 16, fontSize: 21, fontWeight: 600, lineHeight: 1.4 }}>
              {typed(src.fact, s, start + 0.35, cjk ? 26 : 56)}
              {!done && <span style={{ display: "inline-block", width: 9, height: 9, borderRadius: 5, marginLeft: 5, background: INK }} />}
            </div>
          </Card>
        );
      })}
      <Card style={{ left: 770, top: 262, width: 440, height: 364, padding: "22px 24px", ...pop(easeBack((s - 3.5) / 0.5), 40) }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 24, fontWeight: 700 }}>
          {c.s2.title}
          <Chip tone="bad">{c.s2.risk}</Chip>
        </div>
        <div style={{ marginTop: 16, display: "flex", flexDirection: "column", gap: 12 }}>
          {c.s2.lines.map((line, i) => (
            <div key={line} style={{ display: "flex", gap: 12, fontSize: 18, lineHeight: 1.4, ...pop(rise(s, 3.9 + i * 0.3), 12) }}>
              <span style={{ color: BRAND, fontWeight: 800, fontFamily: MONO }}>{i + 1}</span>
              {line}
            </div>
          ))}
        </div>
        <div style={{ position: "absolute", left: 24, right: 24, bottom: 20, paddingTop: 12, borderTop: `1px dashed ${LINE}`, fontSize: 13.5, color: MUTED, ...pop(rise(s, 4.9), 6) }}>{c.s2.source}</div>
      </Card>
      <div style={{ position: "absolute", left: 70, top: 476, ...pop(rise(s, 3.2), 10) }}>
        <Chip>{c.s2.badge}</Chip>
      </div>
    </>
  );
}

// ---- 3：专家、技能、连接器 ----

const TILE_SPOTS: [number, number][] = [
  [-440, -236],
  [-150, -250],
  [150, -250],
  [440, -236],
  [-500, -92],
  [500, -92],
  [-505, 72],
  [505, 72],
  [-440, 226],
  [-150, 244],
  [150, 244],
  [440, 226],
];

function Scene3({ c, s, cjk }: SceneProps) {
  const wall = 1 - rise(s, 1.75, 0.4);
  const hues = [BRAND, GREEN, AMBER, VIOLET, RED];
  return (
    <>
      {wall > 0.01 && (
        <>
          <div style={{ position: "absolute", inset: -30, display: "grid", gridTemplateColumns: "repeat(5, 1fr)", gap: 22, padding: 30, opacity: 0.8 * wall, filter: "blur(5px)", transform: `scale(${1.08 - 0.06 * rise(s, 0, 1.8)})` }}>
            {Array.from({ length: 15 }, (_, i) => (
              <div key={i} style={{ background: "#fff", borderRadius: 14, boxShadow: SHADOW, padding: 18 }}>
                <div style={{ display: "flex", gap: 10, alignItems: "center" }}>
                  <span style={{ width: 26, height: 26, borderRadius: 8, background: hues[i % hues.length] }} />
                  <span style={{ height: 12, width: `${40 + rnd(i, 1) * 40}%`, borderRadius: 6, background: "#C9D1DC" }} />
                </div>
                {[0, 1, 2].map((k) => (
                  <div key={k} style={{ marginTop: 14, height: 9, width: `${45 + rnd(i, k + 2) * 50}%`, borderRadius: 5, background: "#E4E8EF" }} />
                ))}
                <div style={{ marginTop: 18, height: 9, width: 54, borderRadius: 5, background: hues[(i + 2) % hues.length], opacity: 0.55 }} />
              </div>
            ))}
          </div>
          <div style={{ position: "absolute", left: 0, right: 0, top: 300, textAlign: "center", opacity: wall, ...headline(cjk, cjk ? 88 : 84) }}>
            <span style={{ display: "inline-block", ...headIn(rise(s, 0.1, 0.55)) }}>{c.s3.lead}</span>
            <span style={{ display: "inline-block", marginLeft: cjk ? 0 : 22, color: BRAND, ...headIn(rise(s, 0.4, 0.55)) }}>{c.s3.word}</span>
          </div>
        </>
      )}
      {s > 1.7 && (
        <>
          {c.s3.tiles.map((tile, i) => {
            const p = easeBack((s - 1.95 - i * 0.06) / 0.6);
            const [x, y] = TILE_SPOTS[i];
            const dx = Math.sin(s * 0.9 + i * 1.7) * 7;
            const dy = Math.cos(s * 0.8 + i * 2.3) * 6;
            const look: CSSProperties =
              tile.kind === "dark"
                ? { background: INK, color: "#fff" }
                : tile.kind === "skill"
                  ? { background: BRAND, color: "#fff" }
                  : { background: "#fff", color: INK };
            return (
              <div key={tile.text} style={{ position: "absolute", left: 640 + x * p + dx, top: 360 + y * p + dy, transform: `translate(-50%, -50%) scale(${0.4 + 0.6 * p}) rotate(${(1 - p) * (i % 2 ? 14 : -14)}deg)`, opacity: clamp01(p * 2) }}>
                <div style={{ display: "flex", alignItems: "center", gap: 10, borderRadius: 14, boxShadow: SHADOW, padding: tile.icon ? "13px 18px 13px 13px" : "13px 18px", fontSize: 17, fontWeight: 700, whiteSpace: "nowrap", ...look }}>
                  {tile.icon && <span style={{ width: 36, height: 36, borderRadius: 10, background: "rgba(47,107,255,0.10)", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 20 }}>{tile.icon}</span>}
                  {tile.kind === "conn" && <span style={{ width: 9, height: 9, borderRadius: 5, background: GREEN }} />}
                  {tile.text}
                </div>
              </div>
            );
          })}
          <div style={{ position: "absolute", left: 0, right: 0, top: 306, textAlign: "center" }}>
            <div style={{ display: "inline-flex", alignItems: "center", gap: 12, color: BRAND, ...headIn(rise(s, 1.9, 0.55)) }}>
              <WhaleMark size={62} />
              <span style={{ fontSize: 60, fontWeight: 800, letterSpacing: "-0.04em", color: INK }}>dsh-ee</span>
            </div>
            <div style={{ marginTop: 6, fontSize: 21, fontWeight: 600, color: MUTED, ...pop(rise(s, 2.3), 10) }}>{c.s3.center}</div>
          </div>
        </>
      )}
    </>
  );
}

// ---- 4：管得住 ----

function Scene4({ c, s, cjk }: SceneProps) {
  const clickAt = 2.5;
  const allowed = s > clickAt;
  const move = easeOut((s - 1.6) / 0.8);
  const cursorX = 430 - move * 300;
  const cursorY = 690 - move * 92;
  const colors = [BRAND, GREEN, VIOLET];
  return (
    <>
      <div style={{ position: "absolute", left: 70, top: 96, ...headline(cjk, cjk ? 74 : 78) }}>
        {c.s4.lines.map((line, i) => (
          <div key={line.b} style={headIn(rise(s, 0.1 + i * 0.32, 0.5))}>
            {line.a}
            {cjk ? "" : " "}
            <span style={{ color: colors[i] }}>{line.b}</span>
          </div>
        ))}
      </div>
      <Card style={{ left: 70, top: 440, width: 440, padding: "20px 22px", ...pop(easeBack((s - 1.2) / 0.5), 40) }}>
        <div style={{ fontSize: 18, fontWeight: 700 }}>{c.s4.cardTitle}</div>
        <div style={{ marginTop: 6, fontSize: 15, color: MUTED }}>{c.s4.cardSub}</div>
        <div style={{ marginTop: 16, display: "flex", gap: 10, alignItems: "center", height: 42 }}>
          {allowed ? (
            <div style={{ display: "flex", alignItems: "center", gap: 9, fontSize: 17, fontWeight: 700, color: GREEN }}>
              <Check />
              {c.s4.allowed}
            </div>
          ) : (
            <>
              <span style={{ padding: "9px 28px", borderRadius: 10, background: BRAND, color: "#fff", fontSize: 16, fontWeight: 700, transform: `scale(${s > clickAt - 0.12 ? 0.93 : 1})` }}>{c.s4.allow}</span>
              <span style={{ padding: "9px 28px", borderRadius: 10, border: `1px solid ${LINE}`, fontSize: 16, fontWeight: 600, color: MUTED }}>{c.s4.deny}</span>
            </>
          )}
        </div>
      </Card>
      {s > 1.5 && s < clickAt + 0.5 && (
        <svg width="26" height="26" viewBox="0 0 24 24" style={{ position: "absolute", left: cursorX, top: cursorY, opacity: clamp01((clickAt + 0.5 - s) / 0.3), filter: "drop-shadow(0 2px 3px rgba(11,15,23,0.35))" }}>
          <path d="M4 2l15 9-6.5 1.6L9.6 19z" fill="#fff" stroke={INK} strokeWidth="1.4" strokeLinejoin="round" />
        </svg>
      )}
      <Card style={{ left: 570, top: 110, width: 640, overflow: "hidden", ...pop(easeBack((s - 0.5) / 0.55), 50) }}>
        <div style={{ height: 40, display: "flex", alignItems: "center", gap: 7, padding: "0 16px", borderBottom: `1px solid ${LINE}`, background: "#FAFBFC" }}>
          {["#FF5F57", "#FEBC2E", "#28C840"].map((x) => (
            <span key={x} style={{ width: 10, height: 10, borderRadius: 5, background: x }} />
          ))}
          <span style={{ marginLeft: 10, fontSize: 14, fontWeight: 600, color: MUTED }}>{c.s4.logTitle}</span>
        </div>
        {c.s4.log.map((row, i) => {
          // 最后一条是刚才点「允许」的那次调用，点完才出现
          const start = i === c.s4.log.length - 1 ? clickAt + 0.25 : 0.9 + i * 0.25;
          return (
            <div key={i} style={{ display: "flex", alignItems: "center", gap: 14, padding: "17px 20px", borderBottom: `1px solid ${LINE}`, background: row.ok ? "#fff" : "rgba(229,72,77,0.06)", ...pop(rise(s, start, 0.4), 14) }}>
              <Check ok={row.ok} />
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={{ fontSize: 18, fontWeight: 700 }}>{row.who}</div>
                <div style={{ fontSize: 14, color: MUTED, fontFamily: MONO, marginTop: 2 }}>{row.what}</div>
              </div>
              <span style={{ fontSize: 15, fontWeight: 700, color: row.ok ? GREEN : RED, textAlign: "right", maxWidth: 260 }}>{row.result}</span>
            </div>
          );
        })}
      </Card>
    </>
  );
}

// ---- 5：小团队也一样 ----

function Scene5({ c, s, cjk }: SceneProps) {
  const flag = rise(s, 2.3, 0.5);
  return (
    <>
      <div style={{ position: "absolute", left: 70, top: 70, ...headline(cjk, 60) }}>
        <div style={headIn(rise(s, 0.1))}>{c.s5.lead}</div>
        <div style={{ color: BRAND, ...headIn(rise(s, 0.4)) }}>{c.s5.word}</div>
      </div>
      <div style={{ position: "absolute", right: 80, top: 104, ...pop(easeBack((s - 0.6) / 0.45), 16) }}>
        <Sel p={rise(s, 0.95, 0.35)} size="team × 4">
          <Pill color={INK} style={{ fontSize: 19, fontWeight: 600, padding: "10px 20px" }}>
            {c.s5.team}
          </Pill>
        </Sel>
      </div>
      <Card style={{ left: 70, top: 262, width: 560, overflow: "hidden", ...pop(easeBack((s - 0.8) / 0.5), 40) }}>
        <div style={{ padding: "14px 22px", fontSize: 17, fontWeight: 700, borderBottom: `1px solid ${LINE}`, background: "#FAFBFC" }}>🗂 {c.s5.tableTitle}</div>
        <div style={{ display: "grid", gridTemplateColumns: "64px 1fr 150px", padding: "10px 22px", fontSize: 13, fontWeight: 700, color: MUTED, borderBottom: `1px solid ${LINE}` }}>
          {c.s5.cols.map((col) => (
            <span key={col}>{col}</span>
          ))}
        </div>
        {c.s5.rows.map((row, i) => {
          const hot = row.tone === "bad";
          return (
            <div
              key={row.no}
              style={{
                display: "grid",
                gridTemplateColumns: "64px 1fr 150px",
                alignItems: "center",
                padding: "15px 22px",
                fontSize: 19,
                borderBottom: `1px solid ${LINE}`,
                background: hot ? `rgba(229,72,77,${0.08 * flag})` : "#fff",
                boxShadow: hot ? `inset 4px 0 0 rgba(229,72,77,${flag})` : "none",
                ...pop(rise(s, 1.1 + i * 0.12, 0.4), 10),
              }}
            >
              <span style={{ color: MUTED, fontFamily: MONO }}>{row.no}</span>
              <span style={{ fontWeight: 700 }}>{row.title}</span>
              <span>
                <Chip tone={row.tone as "ok" | "bad" | "wait"}>{row.status}</Chip>
              </span>
            </div>
          );
        })}
      </Card>
      <Card style={{ left: 670, top: 290, width: 540, padding: 26, ...pop(easeBack((s - 2.0) / 0.5), 40) }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 44, height: 44, borderRadius: 12, background: "rgba(47,107,255,0.10)", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 24 }}>🧭</span>
          <span style={{ fontSize: 22, fontWeight: 700 }}>{c.s5.expert}</span>
        </div>
        <div style={{ marginTop: 18, fontSize: 18, ...pop(rise(s, 2.4), 8) }}>{c.s5.finding}</div>
        <div style={{ marginTop: 12, padding: "12px 16px", borderLeft: `4px solid ${RED}`, background: "rgba(229,72,77,0.07)", borderRadius: "0 10px 10px 0", fontSize: 18, fontWeight: 700, ...pop(rise(s, 2.8), 8) }}>{c.s5.rule}</div>
        <div style={{ marginTop: 16, paddingTop: 12, borderTop: `1px dashed ${LINE}`, fontSize: 14, color: MUTED, ...pop(rise(s, 3.2), 6) }}>{c.s5.source}</div>
      </Card>
    </>
  );
}

// ---- 6：片尾（深色） ----

function DarkBackdrop({ s }: { s: number }) {
  const hues = ["#FFFFFF", "#FFD27A", "#FF6B6B", "#8FB4FF", "#C9B6FF", "#F3E3C3"];
  return (
    <div style={{ position: "absolute", inset: 0, overflow: "hidden", background: "linear-gradient(120deg, #2A1038 0%, #101A3A 48%, #0B1F4A 100%)" }}>
      <div style={{ position: "absolute", inset: 0, background: "radial-gradient(ellipse 55% 60% at 12% 8%, rgba(214,64,110,0.55), rgba(214,64,110,0) 70%), radial-gradient(ellipse 50% 55% at 58% 0%, rgba(230,150,60,0.42), rgba(230,150,60,0) 70%), radial-gradient(ellipse 60% 70% at 95% 85%, rgba(47,107,255,0.55), rgba(47,107,255,0) 70%)" }} />
      {Array.from({ length: 30 }, (_, i) => {
        const size = 18 + rnd(i, 1) * 46;
        const x = rnd(i, 2) * FILM_WIDTH;
        // 开头一下子散开，之后慢慢往下飘
        const burst = easeOut(s / 1.1);
        const y = ((rnd(i, 3) * 900 + s * (30 + rnd(i, 4) * 70)) % 900) - 90;
        // 正中间是标志和文字，飘到那一带的碎片调淡
        const mid = Math.abs(x - 640) < 430 && Math.abs(y - 360) < 150 ? 0.18 : 1;
        const shape = `polygon(${rnd(i, 5) * 40}% 0%, 100% ${30 + rnd(i, 6) * 50}%, ${20 + rnd(i, 7) * 40}% 100%)`;
        return (
          <div
            key={i}
            style={{
              position: "absolute",
              left: 640 + (x - 640) * burst,
              top: 360 + (y - 360) * burst,
              width: size,
              height: size,
              background: hues[i % hues.length],
              clipPath: shape,
              opacity: (0.35 + rnd(i, 8) * 0.6) * clamp01(s / 0.4) * mid,
              transform: `rotate(${rnd(i, 9) * 360 + s * (40 + rnd(i, 10) * 90) * (i % 2 ? 1 : -1)}deg)`,
              filter: i % 4 === 0 ? "blur(3px)" : "none",
            }}
          />
        );
      })}
    </div>
  );
}

function Scene6({ c, s, cjk }: SceneProps) {
  const pill: CSSProperties = { display: "inline-flex", alignItems: "center", gap: 12, borderRadius: 999, padding: "12px 22px", background: "rgba(255,255,255,0.10)", border: "1px solid rgba(255,255,255,0.20)", color: "#fff", fontFamily: MONO, fontSize: 19, fontWeight: 600 };
  const tag: CSSProperties = { fontFamily: MONO, fontSize: 11, letterSpacing: 1.2, color: "rgba(255,255,255,0.55)" };
  return (
    <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center", color: "#fff", padding: "0 60px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 18, transform: `scale(${0.7 + 0.3 * easeBack((s - 0.25) / 0.6)})`, opacity: clamp01((s - 0.25) / 0.25) }}>
        <WhaleMark size={104} />
        <span style={{ fontSize: 96, fontWeight: 800, letterSpacing: "-0.045em" }}>dsh-ee</span>
      </div>
      <div style={{ marginTop: 14, fontSize: cjk ? 34 : 32, fontWeight: 600, color: "rgba(255,255,255,0.92)", ...headIn(rise(s, 0.7)) }}>{c.s6.slogan}</div>
      <div style={{ marginTop: 10, fontSize: 18, color: "rgba(255,255,255,0.6)", ...pop(rise(s, 1.0), 10) }}>{c.s6.sub}</div>
      <div style={{ marginTop: 34, display: "flex", gap: 14 }}>
        <span style={{ ...pill, ...pop(easeBack((s - 1.3) / 0.45), 18) }}>
          <span style={{ width: 8, height: 8, borderRadius: 4, background: "#4ADE80" }} />
          {c.s6.demo}
          <span style={tag}>{c.s6.demoTag}</span>
        </span>
        <span style={{ ...pill, ...pop(easeBack((s - 1.45) / 0.45), 18) }}>
          <span style={{ width: 8, height: 8, borderRadius: 4, background: "#8FB4FF" }} />
          {c.s6.repo}
          <span style={tag}>{c.s6.repoTag}</span>
        </span>
      </div>
    </div>
  );
}
