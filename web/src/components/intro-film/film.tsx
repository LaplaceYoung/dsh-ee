// 30 秒介绍短片。整部片子是时间 t（秒）的纯函数：没有 hook、没有定时器、不读界面主题，
// 所以同一份代码既能在首页用 requestAnimationFrame 播，也能被 promo/ 逐帧渲染成 MP4，两边一帧不差。
// 画布固定 1280×720，外面按容器宽度整体缩放。
import type { CSSProperties, ReactNode } from "react";
import { WhaleMark } from "../whale-mark";
import { filmCopy, type FilmCopy, type FilmLang } from "./copy";

export const FILM_SECONDS = 30;
export const FILM_WIDTH = 1280;
export const FILM_HEIGHT = 720;

const BRAND = "#4D6BFE";
const INK = "#0A2540";
const MUTED = "#5B6B82";
const LINE = "#E3E8EE";
const BG = "#F5F7FB";
const GREEN = "#12A150";
const RED = "#D92D20";
const AMBER = "#B7791F";
const FONT = '"PingFang SC", "Noto Sans CJK SC", "Microsoft YaHei", "Inter", system-ui, sans-serif';
const MONO = '"JetBrains Mono", "SFMono-Regular", Menlo, Consolas, monospace';
const SHADOW = "0 24px 60px rgba(10,37,64,0.14), 0 0 0 1px rgba(10,37,64,0.06)";

/** 场景起止（秒）。相邻场景交叠 FADE 秒做淡入淡出。 */
const FADE = 0.35;
const CUTS = [0, 4.5, 11.5, 16.5, 21, 26.5, FILM_SECONDS] as const;

const clamp01 = (x: number) => Math.max(0, Math.min(1, x));
const easeOut = (x: number) => 1 - Math.pow(1 - clamp01(x), 3);
/** 从 start 秒起，用 dur 秒从 0 走到 1（缓出）。 */
const rise = (s: number, start: number, dur = 0.45) => easeOut((s - start) / dur);
/** 淡入并上浮的通用样式。 */
const pop = (p: number, dy = 16): CSSProperties => ({ opacity: p, transform: `translateY(${(1 - p) * dy}px)` });
/** 打字机：从 start 秒起每秒 cps 个字。 */
const typed = (text: string, s: number, start: number, cps: number) => Array.from(text).slice(0, Math.max(0, Math.floor((s - start) * cps))).join("");

type SceneProps = { c: FilmCopy; s: number; cjk: boolean };

export function IntroFilm({ t, lang }: { t: number; lang: FilmLang }) {
  const c = filmCopy[lang];
  const time = Math.max(0, Math.min(FILM_SECONDS, t));
  const scenes = [Scene1, Scene2, Scene3, Scene4, Scene5, Scene6];
  return (
    <div style={{ position: "relative", width: FILM_WIDTH, height: FILM_HEIGHT, overflow: "hidden", background: BG, color: INK, fontFamily: FONT, lineHeight: 1.4, textAlign: "left" }}>
      {scenes.map((Scene, i) => {
        const from = CUTS[i];
        const to = CUTS[i + 1];
        if (time < from - FADE || time > to) return null;
        const fadeIn = i === 0 ? 1 : clamp01((time - (from - FADE)) / FADE);
        const fadeOut = i === scenes.length - 1 ? 1 : clamp01((to - time) / FADE);
        return (
          <div key={i} style={{ position: "absolute", inset: 0, background: BG, opacity: Math.min(fadeIn, fadeOut) }}>
            <Scene c={c} s={time - from} cjk={lang === "zh"} />
          </div>
        );
      })}
    </div>
  );
}

// ---- 公用 ----

function Caption({ text, s, cjk }: { text: string; s: number; cjk: boolean }) {
  return (
    <div style={{ position: "absolute", top: 44, left: 40, right: 40, textAlign: "center", fontSize: cjk ? 36 : 30, fontWeight: 700, letterSpacing: cjk ? 0 : -0.4, ...pop(rise(s, 0.1)) }}>
      {text}
    </div>
  );
}

function Window({ title, style, children }: { title?: string; style?: CSSProperties; children: ReactNode }) {
  return (
    <div style={{ position: "absolute", background: "#fff", borderRadius: 14, boxShadow: SHADOW, overflow: "hidden", ...style }}>
      <div style={{ height: 32, display: "flex", alignItems: "center", gap: 6, padding: "0 14px", borderBottom: `1px solid ${LINE}`, background: "#FAFBFD" }}>
        {["#FF5F57", "#FEBC2E", "#28C840"].map((x) => (
          <span key={x} style={{ width: 9, height: 9, borderRadius: 5, background: x }} />
        ))}
        {title && <span style={{ marginLeft: 8, fontSize: 13, color: MUTED }}>{title}</span>}
      </div>
      {children}
    </div>
  );
}

function Chip({ children, tone = "plain" }: { children: ReactNode; tone?: "plain" | "brand" | "ok" | "bad" | "wait" | "idle" }) {
  const tones = {
    plain: { background: "#EEF2F7", color: "#34465B" },
    brand: { background: "rgba(77,107,254,0.12)", color: BRAND },
    ok: { background: "rgba(18,161,80,0.12)", color: GREEN },
    bad: { background: "rgba(217,45,32,0.10)", color: RED },
    wait: { background: "rgba(183,121,31,0.13)", color: AMBER },
    idle: { background: "#EEF2F7", color: MUTED },
  } as const;
  return <span style={{ display: "inline-block", padding: "3px 10px", borderRadius: 999, fontSize: 13, fontWeight: 600, whiteSpace: "nowrap", ...tones[tone] }}>{children}</span>;
}

function Check({ ok = true }: { ok?: boolean }) {
  return (
    <span style={{ display: "inline-flex", width: 18, height: 18, borderRadius: 9, alignItems: "center", justifyContent: "center", background: ok ? GREEN : RED, color: "#fff", fontSize: 12, fontWeight: 700, flexShrink: 0 }}>
      {ok ? "✓" : "✕"}
    </span>
  );
}

function Bubble({ me, children, style }: { me?: boolean; children: ReactNode; style?: CSSProperties }) {
  return (
    <div style={{ display: "flex", justifyContent: me ? "flex-end" : "flex-start", ...style }}>
      <div style={{ maxWidth: "78%", padding: "12px 16px", borderRadius: 14, fontSize: 19, background: me ? BRAND : "#EEF2F7", color: me ? "#fff" : INK, minHeight: 27 }}>{children}</div>
    </div>
  );
}

// ---- 1：AI 不认识你的公司 ----

function Scene1({ c, s, cjk }: SceneProps) {
  const q = typed(c.s1.q, s, 0.5, cjk ? 22 : 44);
  return (
    <>
      <Caption text={c.s1.caption} s={s} cjk={cjk} />
      <Window title="AI" style={{ left: 240, top: 170, width: 800, height: 400, ...pop(rise(s, 0.2), 24) }}>
        <div style={{ padding: 28, display: "flex", flexDirection: "column", gap: 18 }}>
          <Bubble me>{q || " "}</Bubble>
          <Bubble style={pop(rise(s, 2.3))}>
            <span style={{ color: MUTED }}>{c.s1.a}</span>
          </Bubble>
        </div>
      </Window>
    </>
  );
}

// ---- 2：接上 dsh-ee 之后 ----

function Scene2({ c, s, cjk }: SceneProps) {
  const q = typed(c.s2.q, s, 0.8, cjk ? 24 : 60);
  return (
    <>
      <Caption text={c.s2.caption} s={s} cjk={cjk} />
      <Window title="DSH" style={{ left: 150, top: 136, width: 980, height: 480, ...pop(rise(s, 0.15), 24) }}>
        <div style={{ position: "absolute", top: 5, right: 12 }}>
          <Chip>{c.s2.badge}</Chip>
        </div>
        <div style={{ padding: "22px 28px", display: "flex", flexDirection: "column", gap: 14 }}>
          <div style={{ display: "flex", justifyContent: "flex-end", alignItems: "center", gap: 10, ...pop(rise(s, 0.5)) }}>
            <span style={{ fontFamily: MONO, fontSize: 16, color: BRAND, background: "rgba(77,107,254,0.10)", padding: "4px 10px", borderRadius: 8 }}>{c.s2.cmd}</span>
            <div style={{ padding: "10px 16px", borderRadius: 14, fontSize: 18, background: BRAND, color: "#fff", minHeight: 25, minWidth: 20 }}>{q || " "}</div>
          </div>
          {c.s2.tools.map((tool, i) => {
            const start = 1.9 + i * 0.6;
            const done = s > start + 0.5;
            return (
              <div key={tool} style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 16, color: MUTED, ...pop(rise(s, start)) }}>
                {done ? <Check /> : <span style={{ width: 18, height: 18, borderRadius: 9, border: `2px solid ${LINE}`, borderTopColor: BRAND, boxSizing: "border-box", transform: `rotate(${s * 540}deg)` }} />}
                <span style={{ fontFamily: MONO }}>{tool}</span>
              </div>
            );
          })}
          <div style={{ border: `1px solid ${LINE}`, borderRadius: 14, padding: "18px 22px", ...pop(rise(s, 3.3), 20) }}>
            <div style={{ display: "flex", alignItems: "center", gap: 12, fontSize: 22, fontWeight: 700 }}>
              {c.s2.title}
              <Chip tone="bad">{c.s2.risk}</Chip>
            </div>
            <div style={{ marginTop: 12, display: "flex", flexDirection: "column", gap: 9 }}>
              {c.s2.lines.map((line, i) => (
                <div key={line} style={{ display: "flex", gap: 10, fontSize: 18, ...pop(rise(s, 3.7 + i * 0.35), 10) }}>
                  <span style={{ color: BRAND, fontWeight: 700 }}>{i + 1}</span>
                  {line}
                </div>
              ))}
            </div>
            <div style={{ marginTop: 14, paddingTop: 12, borderTop: `1px dashed ${LINE}`, fontSize: 15, color: MUTED, ...pop(rise(s, 4.9), 6) }}>{c.s2.source}</div>
          </div>
        </div>
      </Window>
    </>
  );
}

// ---- 3：专家、技能、连接器 ----

function Scene3({ c, s, cjk }: SceneProps) {
  return (
    <>
      <Caption text={c.s3.caption} s={s} cjk={cjk} />
      <div style={{ position: "absolute", left: 70, top: 150, width: 690, display: "grid", gridTemplateColumns: "1fr 1fr", gap: 18 }}>
        {c.s3.experts.map((e, i) => (
          <div key={e.name} style={{ background: "#fff", borderRadius: 14, boxShadow: SHADOW, padding: "18px 20px", height: 172, boxSizing: "border-box", ...pop(rise(s, 0.4 + i * 0.22), 22) }}>
            <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
              <span style={{ width: 44, height: 44, borderRadius: 12, background: "rgba(77,107,254,0.10)", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 24 }}>{e.icon}</span>
              <span style={{ fontSize: 21, fontWeight: 700 }}>{e.name}</span>
            </div>
            <div style={{ marginTop: 14, display: "flex", gap: 8, flexWrap: "wrap" }}>
              {e.tags.map((tag) => (
                <Chip key={tag}>{tag}</Chip>
              ))}
            </div>
            <div style={{ marginTop: 14, fontSize: 14, color: MUTED }}>
              {c.s3.usesLabel} · <span style={{ color: INK, fontWeight: 600 }}>{e.uses}</span>
            </div>
          </div>
        ))}
      </div>
      <div style={{ position: "absolute", left: 800, top: 150, width: 410, display: "flex", flexDirection: "column", gap: 14 }}>
        {c.s3.rows.map((row, i) => (
          <div key={row.label} style={{ background: "#fff", borderRadius: 14, boxShadow: SHADOW, padding: "14px 18px", ...pop(rise(s, 1.5 + i * 0.3), 18) }}>
            <div style={{ fontSize: 14, fontWeight: 700, color: BRAND, letterSpacing: 0.4 }}>{row.label}</div>
            <div style={{ marginTop: 9, display: "flex", gap: 8, flexWrap: "wrap" }}>
              {row.items.map((item) => (
                <Chip key={item}>{item}</Chip>
              ))}
            </div>
          </div>
        ))}
      </div>
      <div style={{ position: "absolute", left: 0, right: 0, top: 572, display: "flex", justifyContent: "center", ...pop(rise(s, 2.9), 18) }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12, background: INK, color: "#fff", borderRadius: 999, padding: "13px 26px", fontSize: 19, fontWeight: 600 }}>
          <Check />
          {c.s3.deliver}
        </div>
      </div>
    </>
  );
}

// ---- 4：管得住 ----

function Scene4({ c, s, cjk }: SceneProps) {
  const clickAt = 1.9;
  const allowed = s > clickAt;
  // 鼠标从右下角移到「允许」按钮上
  const move = easeOut((s - 0.9) / 0.9);
  const cursorX = 520 - move * 318;
  const cursorY = 560 - move * 172;
  const press = s > clickAt - 0.12 && s < clickAt + 0.12 ? 0.94 : 1;
  return (
    <>
      <Caption text={c.s4.caption} s={s} cjk={cjk} />
      <div style={{ position: "absolute", left: 70, top: 190, width: 470, background: "#fff", borderRadius: 16, boxShadow: SHADOW, padding: 26, boxSizing: "border-box", ...pop(rise(s, 0.3), 22) }}>
        <div style={{ fontSize: 21, fontWeight: 700 }}>{c.s4.cardTitle}</div>
        <div style={{ marginTop: 10, fontSize: 17, color: MUTED }}>{c.s4.cardSub}</div>
        <div style={{ marginTop: 24, display: "flex", gap: 12, alignItems: "center", height: 46 }}>
          {allowed ? (
            <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 18, fontWeight: 700, color: GREEN }}>
              <Check />
              {c.s4.allowed}
            </div>
          ) : (
            <>
              <span style={{ padding: "10px 30px", borderRadius: 10, background: BRAND, color: "#fff", fontSize: 18, fontWeight: 700, transform: `scale(${press})` }}>{c.s4.allow}</span>
              <span style={{ padding: "10px 30px", borderRadius: 10, border: `1px solid ${LINE}`, fontSize: 18, fontWeight: 600, color: MUTED }}>{c.s4.deny}</span>
            </>
          )}
        </div>
      </div>
      {s > 0.8 && s < clickAt + 0.5 && (
        <svg width="26" height="26" viewBox="0 0 24 24" style={{ position: "absolute", left: cursorX, top: cursorY, opacity: clamp01((clickAt + 0.5 - s) / 0.3), filter: "drop-shadow(0 2px 3px rgba(10,37,64,0.35))" }}>
          <path d="M4 2l15 9-6.5 1.6L9.6 19z" fill="#fff" stroke={INK} strokeWidth="1.4" strokeLinejoin="round" />
        </svg>
      )}
      <div style={{ position: "absolute", left: 580, top: 150, width: 630, background: "#fff", borderRadius: 16, boxShadow: SHADOW, overflow: "hidden", ...pop(rise(s, 0.5), 22) }}>
        <div style={{ padding: "14px 22px", fontSize: 16, fontWeight: 700, borderBottom: `1px solid ${LINE}`, background: "#FAFBFD" }}>{c.s4.logTitle}</div>
        {c.s4.log.map((row, i) => {
          // 最后一条是刚才点「允许」的那次调用，点完才出现
          const start = i === c.s4.log.length - 1 ? clickAt + 0.3 : 0.9 + i * 0.3;
          return (
            <div key={i} style={{ display: "flex", alignItems: "center", gap: 14, padding: "15px 22px", borderBottom: `1px solid ${LINE}`, background: row.ok ? "#fff" : "rgba(217,45,32,0.05)", ...pop(rise(s, start), 10) }}>
              <Check ok={row.ok} />
              <div style={{ minWidth: 0, flex: 1 }}>
                <div style={{ fontSize: 17, fontWeight: 600 }}>{row.who}</div>
                <div style={{ fontSize: 14, color: MUTED, fontFamily: MONO, marginTop: 2 }}>{row.what}</div>
              </div>
              <span style={{ fontSize: 15, fontWeight: 600, color: row.ok ? GREEN : RED, textAlign: "right", maxWidth: 270 }}>{row.result}</span>
            </div>
          );
        })}
      </div>
    </>
  );
}

// ---- 5：小团队也一样 ----

function Scene5({ c, s, cjk }: SceneProps) {
  const flag = rise(s, 2.2, 0.5);
  return (
    <>
      <Caption text={c.s5.caption} s={s} cjk={cjk} />
      <div style={{ position: "absolute", left: 0, right: 0, top: 108, textAlign: "center", ...pop(rise(s, 0.3), 8) }}>
        <Chip tone="brand">{c.s5.team}</Chip>
      </div>
      <div style={{ position: "absolute", left: 70, top: 172, width: 560, background: "#fff", borderRadius: 16, boxShadow: SHADOW, overflow: "hidden", ...pop(rise(s, 0.4), 22) }}>
        <div style={{ padding: "14px 22px", fontSize: 17, fontWeight: 700, borderBottom: `1px solid ${LINE}`, background: "#FAFBFD" }}>🗂 {c.s5.tableTitle}</div>
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
                padding: "13px 22px",
                fontSize: 18,
                borderBottom: `1px solid ${LINE}`,
                background: hot ? `rgba(217,45,32,${0.07 * flag})` : "#fff",
                boxShadow: hot ? `inset 4px 0 0 rgba(217,45,32,${flag})` : "none",
                ...pop(rise(s, 0.7 + i * 0.14), 8),
              }}
            >
              <span style={{ color: MUTED, fontFamily: MONO }}>{row.no}</span>
              <span style={{ fontWeight: 600 }}>{row.title}</span>
              <span>
                <Chip tone={row.tone as "ok" | "bad" | "wait" | "idle"}>{row.status}</Chip>
              </span>
            </div>
          );
        })}
      </div>
      <div style={{ position: "absolute", left: 670, top: 200, width: 540, background: "#fff", borderRadius: 16, boxShadow: SHADOW, padding: 26, boxSizing: "border-box", ...pop(rise(s, 2.0), 22) }}>
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <span style={{ width: 44, height: 44, borderRadius: 12, background: "rgba(77,107,254,0.10)", display: "inline-flex", alignItems: "center", justifyContent: "center", fontSize: 24 }}>🧭</span>
          <span style={{ fontSize: 21, fontWeight: 700 }}>{c.s5.expert}</span>
        </div>
        <div style={{ marginTop: 18, fontSize: 18, ...pop(rise(s, 2.5), 8) }}>{c.s5.finding}</div>
        <div style={{ marginTop: 12, padding: "12px 16px", borderLeft: `4px solid ${RED}`, background: "rgba(217,45,32,0.06)", borderRadius: "0 10px 10px 0", fontSize: 18, fontWeight: 600, ...pop(rise(s, 3.0), 8) }}>{c.s5.rule}</div>
        <div style={{ marginTop: 16, paddingTop: 12, borderTop: `1px dashed ${LINE}`, fontSize: 15, color: MUTED, ...pop(rise(s, 3.5), 6) }}>{c.s5.source}</div>
      </div>
    </>
  );
}

// ---- 6：片尾 ----

function Scene6({ c, s, cjk }: SceneProps) {
  return (
    <div style={{ position: "absolute", inset: 0, display: "flex", flexDirection: "column", alignItems: "center", justifyContent: "center", textAlign: "center", padding: "0 60px" }}>
      <div style={{ display: "flex", alignItems: "center", gap: 14, color: BRAND, ...pop(rise(s, 0.1), 18) }}>
        <WhaleMark size={72} />
        <span style={{ fontSize: 52, fontWeight: 800, letterSpacing: -1, color: INK }}>dsh-ee</span>
      </div>
      <div style={{ marginTop: 26, fontSize: cjk ? 46 : 42, fontWeight: 800, letterSpacing: cjk ? 0 : -0.8, ...pop(rise(s, 0.4), 18) }}>{c.s6.slogan}</div>
      <div style={{ marginTop: 16, fontSize: 21, color: MUTED, ...pop(rise(s, 0.75), 12) }}>{c.s6.sub}</div>
      <div style={{ marginTop: 38, display: "flex", gap: 16, alignItems: "center", ...pop(rise(s, 1.1), 12) }}>
        <span style={{ background: BRAND, color: "#fff", borderRadius: 999, padding: "13px 28px", fontSize: 21, fontWeight: 700 }}>{c.s6.demo}</span>
        <span style={{ fontFamily: MONO, fontSize: 19, color: INK, border: `1px solid ${LINE}`, background: "#fff", borderRadius: 999, padding: "13px 24px" }}>{c.s6.repo}</span>
      </div>
    </div>
  );
}
