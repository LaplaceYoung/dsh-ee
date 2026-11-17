// 给一分钟产品短片配声音：全部用代码合成（不用任何音频素材），再混进 promo/out 里的三个 MP4。
//
// 声音跟着画面走（时间点取自模板的时间轴，见 build-film.mjs 生成的工程）：
//   0–3.7 秒      铺底和弦渐起，开场三个词各一声轻拨，标志出现时一声钟
//   3.7 秒起      鼓进来（每拍一下底鼓，铺底随底鼓起伏），之后依次加上贝斯、踩镲、拍手和琶音
//   每次切场      一声「呼」
//   两次打字      每出一个字一声键盘响（按各语言的字数算），点发送时一声「啵」
//   52.4–55.5 秒  管控那一段鼓停，只留铺底，滤波慢慢打开、噪声上扬
//   58.7 秒       片尾重拍加镲，落在 D 大和弦上，标志出现时再一声钟，最后淡出
//
// 用法：node promo/openmotion/build-audio.mjs [en zh ja]
// 需要 ffmpeg（带 aac 编码器）：默认用 PATH 里的，可用 FFMPEG 指定。
// 第一次运行会把无声的 <name>.mp4 改名成 <name>.silent.mp4，以后都从它重新混，可以反复运行。
import { execFileSync } from "node:child_process";
import { existsSync, readFileSync, renameSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const outDir = join(here, "..", "out");
const FFMPEG = process.env.FFMPEG || "ffmpeg";
const LANGS = process.argv.length > 2 ? process.argv.slice(2) : ["en", "zh", "ja"];

const SR = 48000;
const DUR = 61;
const N = SR * DUR;
const TAU = Math.PI * 2;

// ---------- 画面时间轴 ----------
const CUTS = [3.7, 7.8, 12.2, 493 / 30, 694 / 30, 914 / 30, 34.13, 37.2, 40.3, 1313 / 30, 50.2, 52.4, 55.5, 58.7];
const TYPING = [
  { id: "dailyPrompt", start: 19.75, dur: 1.6, send: 22.7 },
  { id: "opportunityPrompt", start: 47.05, dur: 1.45, send: 49.9 },
];
const BEAT = 0.5; // 120 BPM，一小节 2 秒
const GRID = 3.7; // 鼓和和弦从第一次切场开始数拍
const QUIET = [52.4, 55.5]; // 管控那一段：鼓停
const OUTRO = 58.7;

// ---------- 工具 ----------
function rng(seed) {
  let s = seed >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = Math.imul(s ^ (s >>> 15), s | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}
let seed = 1;
const noiseSource = () => { const r = rng(seed++ * 7919); return () => r() * 2 - 1; };
const mtof = (m) => 440 * 2 ** ((m - 69) / 12);
const coef = (fc) => 1 - Math.exp((-TAU * fc) / SR);
// 打字的时候踩镲、拍手和琶音让开，只留底鼓、贝斯和铺底，好听清键盘声
const whileTyping = (t) => TYPING.some(({ start, dur }) => t >= start - 0.25 && t < start + dur + 0.2);

class Bus {
  constructor() { this.L = new Float32Array(N); this.R = new Float32Array(N); }
  // fn(秒) 按时间顺序调用，所以里面可以带滤波器的状态
  add(t0, len, fn, gain = 1, pan = 0) {
    const s0 = Math.round(t0 * SR), n = Math.round(len * SR);
    const gl = gain * Math.cos(((pan + 1) * Math.PI) / 4), gr = gain * Math.sin(((pan + 1) * Math.PI) / 4);
    for (let i = 0; i < n; i++) {
      const k = s0 + i;
      if (k < 0) continue;
      if (k >= N) break;
      const v = fn(i / SR);
      this.L[k] += v * gl;
      this.R[k] += v * gr;
    }
  }
}

// 带限锯齿波表：铺底和贝斯用
const TL = 4096;
const SAW = new Float32Array(TL + 1);
for (let i = 0; i <= TL; i++) {
  let v = 0;
  for (let h = 1; h <= 24; h++) v += (Math.sin((TAU * h * i) / TL) / h) * (1 - h / 26);
  SAW[i] = v * 0.55;
}
const saw = (ph) => { const x = (ph - Math.floor(ph)) * TL, i = x | 0; return SAW[i] + (SAW[i + 1] - SAW[i]) * (x - i); };

// ---------- 和弦 ----------
// D 大调：Dmaj9 – Bm11 – Gmaj9 – A，一小节一个
const CHORDS = {
  D: { pad: [54, 57, 61, 64], bass: 38 },
  Bm: { pad: [54, 57, 62, 64], bass: 35 },
  G: { pad: [54, 57, 59, 62], bass: 31 },
  A: { pad: [52, 57, 61, 64], bass: 33 },
  Dend: { pad: [50, 54, 57, 61, 64, 69], bass: 38 },
};
const PROG = ["D", "Bm", "G", "A"];
const bars = [{ t: 0, len: GRID, chord: "D" }];
for (let k = 0; GRID + k * 2 < OUTRO; k++) bars.push({ t: GRID + k * 2, len: Math.min(2, OUTRO - (GRID + k * 2)), chord: PROG[k % 4] });
bars.push({ t: OUTRO, len: DUR - OUTRO, chord: "Dend" });
const chordAt = (t) => CHORDS[[...bars].reverse().find((b) => b.t <= t).chord];

// 拍点
const beatsIn = (from, to, origin, step = BEAT, offset = 0) => {
  const out = [];
  for (let n = Math.ceil((from - origin - offset) / step - 1e-9); origin + offset + n * step < to - 1e-9; n++) out.push({ t: origin + offset + n * step, n });
  return out;
};
const sections = [[GRID, QUIET[0], GRID], [QUIET[1], OUTRO - 0.25, QUIET[1]]];

// ---------- 乐器 ----------
function padNote(bus, t0, len, midi, gain, pan) {
  const f = mtof(midi), fs = [f * 2 ** (-8 / 1200), f, f * 2 ** (7 / 1200)];
  const p = [0.13, 0.51, 0.77].map((x) => x + midi * 0.071);
  const atk = t0 === 0 ? 2.4 : 0.35, rel = 0.9;
  bus.add(t0, len + rel, (t) => {
    const env = Math.min(1, t / atk) * (t > len ? Math.exp(-(t - len) / (rel / 4)) : 1);
    return (env * (saw(fs[0] * t + p[0]) + saw(fs[1] * t + p[1]) + saw(fs[2] * t + p[2]))) / 3;
  }, gain, pan);
}

function bassNote(bus, t0, len, midi, gain) {
  const f = mtof(midi);
  bus.add(t0, len + 0.05, (t) => {
    const env = Math.min(1, t / 0.006) * Math.exp(-t / 0.55) * (t > len ? Math.max(0, 1 - (t - len) / 0.05) : 1);
    const x = Math.sin(TAU * f * t) + 0.35 * saw(f * t * 2) * 0.6;
    return env * Math.tanh(1.6 * x) * 0.8;
  }, gain);
}

function kick(bus, t0, gain) {
  const n = noiseSource();
  bus.add(t0, 0.6, (t) => {
    const ph = 45 * t + 85 * 0.035 * (1 - Math.exp(-t / 0.035));
    const body = Math.sin(TAU * ph) * Math.exp(-t / 0.28) * Math.min(1, t / 0.002);
    return body + n() * 0.25 * Math.exp(-t / 0.002);
  }, gain);
}

function hat(bus, t0, gain, pan, decay = 0.035) {
  const n = noiseSource(), c = coef(7000);
  let lp = 0;
  bus.add(t0, decay * 5, (t) => { const x = n(); lp += c * (x - lp); return (x - lp) * Math.exp(-t / decay); }, gain, pan);
}

function clap(bus, t0, gain) {
  const n = noiseSource(), cHi = coef(2600), cLo = coef(900);
  let a = 0, b = 0;
  bus.add(t0, 0.35, (t) => {
    const x = n();
    a += cHi * (x - a); b += cLo * (a - b);
    const bursts = [0, 0.011, 0.022].reduce((s, o) => s + (t >= o ? Math.exp(-(t - o) / (o < 0.02 ? 0.006 : 0.09)) : 0), 0);
    return (a - b) * bursts * 1.6 + Math.sin(TAU * 185 * t) * Math.exp(-t / 0.04) * 0.35;
  }, gain);
}

function pluck(bus, t0, midi, gain, pan) {
  const f = mtof(midi);
  bus.add(t0, 0.7, (t) => {
    const env = Math.min(1, t / 0.002) * Math.exp(-t / 0.11);
    return env * (Math.sin(TAU * f * t) + 0.45 * Math.sin(TAU * 2 * f * t) * Math.exp(-t / 0.05) + 0.15 * Math.sin(TAU * 3 * f * t) * Math.exp(-t / 0.03));
  }, gain, pan);
}

function bell(bus, t0, midi, gain, pan) {
  const f = mtof(midi);
  const parts = [[1, 1, 1.6], [2.76, 0.45, 0.7], [5.4, 0.22, 0.35], [8.93, 0.1, 0.2]];
  bus.add(t0, 3, (t) => parts.reduce((s, [m, a, d]) => s + a * Math.sin(TAU * f * m * t) * Math.exp(-t / d), 0) * Math.min(1, t / 0.003) * 0.6, gain, pan);
}

function whoosh(bus, cut, gain) {
  const n = noiseSource(), pre = 0.4, len = 0.75;
  let lp = 0, lp2 = 0;
  bus.add(cut - pre, len, (t) => {
    const u = t - pre; // 0 = 切场那一刻
    const fc = u < 0 ? 500 + 4500 * (1 + u / pre) ** 2 : 5000 * Math.exp(-u / 0.12) + 900;
    const c = coef(fc);
    const x = n();
    lp += c * (x - lp); lp2 += coef(fc * 0.25) * (lp - lp2);
    const env = u < 0 ? (1 + u / pre) ** 2.2 : Math.exp(-u / 0.14);
    return (lp - lp2) * env * 2.2;
  }, gain, 0);
}

function boom(bus, t0, gain) {
  bus.add(t0, 2.5, (t) => Math.sin(TAU * (42 * t + 30 * 0.08 * (1 - Math.exp(-t / 0.08)))) * Math.exp(-t / 0.7) * Math.min(1, t / 0.004), gain);
}

function crash(bus, t0, gain) {
  const n = noiseSource(), c = coef(3500);
  let lp = 0;
  bus.add(t0, 4, (t) => { const x = n(); lp += c * (x - lp); return (x - lp) * Math.exp(-t / 1.1) * Math.min(1, t / 0.002); }, gain, 0);
}

function riser(bus, t0, t1, gain) {
  const n = noiseSource();
  let lp = 0, lp2 = 0;
  bus.add(t0, t1 - t0, (t) => {
    const u = t / (t1 - t0), fc = 300 + 6000 * u * u;
    const x = n(); lp += coef(fc) * (x - lp); lp2 += coef(fc * 0.3) * (lp - lp2);
    return (lp - lp2) * u ** 2.5 * 2.5;
  }, gain, 0);
}

function keyClick(bus, t0, gain, pan, r) {
  const n = noiseSource(), c = coef(4500), tone = 1800 + r() * 900;
  let lp = 0;
  bus.add(t0, 0.06, (t) => {
    const x = n(); lp += c * (x - lp);
    return ((x - lp) * 0.8 + Math.sin(TAU * tone * t) * 0.4) * Math.exp(-t / 0.0045) + Math.sin(TAU * 230 * t) * Math.exp(-t / 0.012) * 0.35;
  }, gain * (0.65 + 0.35 * r()), pan);
}

function sendPop(bus, t0, gain) {
  bus.add(t0, 0.25, (t) => Math.sin(TAU * (600 * t + 600 * 0.04 * Math.min(1, t / 0.04) * (t / 0.04 < 1 ? t / 0.08 : 1))) * Math.exp(-t / 0.06) * Math.min(1, t / 0.002), gain);
}

// ---------- 处理 ----------
function lowpass2(bus, fcAt) {
  for (const buf of [bus.L, bus.R]) {
    let a = 0, b = 0;
    for (let k = 0; k < N; k++) { const c = coef(fcAt(k / SR)); a += c * (buf[k] - a); b += c * (a - b); buf[k] = b; }
  }
}

function duck(bus, kicks, depth, tau) {
  let j = 0;
  for (let k = 0; k < N; k++) {
    const t = k / SR;
    while (j + 1 < kicks.length && kicks[j + 1] <= t) j++;
    if (!kicks.length || kicks[j] > t) continue;
    const d = t - kicks[j], g = 1 - depth * Math.min(1, d / 0.005) * Math.exp(-d / tau);
    bus.L[k] *= g; bus.R[k] *= g;
  }
}

// Freeverb 式的混响：一路发送
function reverb(send) {
  const wet = new Bus(), s = SR / 44100;
  const combs = [1116, 1188, 1277, 1356, 1422, 1491, 1557, 1617], aps = [556, 441, 341, 225];
  [[send.L, wet.L, 0], [send.R, wet.R, 23]].forEach(([inp, out, spread]) => {
    const cs = combs.map((d) => ({ buf: new Float32Array(Math.round((d + spread) * s)), i: 0, store: 0 }));
    const as = aps.map((d) => ({ buf: new Float32Array(Math.round((d + spread) * s)), i: 0 }));
    for (let k = 0; k < N; k++) {
      const x = inp[k] * 0.015;
      let y = 0;
      for (const c of cs) {
        const o = c.buf[c.i];
        c.store = o * 0.7 + c.store * 0.3;
        c.buf[c.i] = x + c.store * 0.84;
        c.i = (c.i + 1) % c.buf.length;
        y += o;
      }
      for (const a of as) {
        const o = a.buf[a.i];
        a.buf[a.i] = y + o * 0.5;
        a.i = (a.i + 1) % a.buf.length;
        y = o - y;
      }
      out[k] = y;
    }
  });
  return wet;
}

// ---------- 编排 ----------
function renderBase() {
  const pad = new Bus(), bass = new Bus(), drums = new Bus(), arp = new Bus(), sfx = new Bus(), send = new Bus();
  const sendTo = (bus, amount) => { for (let k = 0; k < N; k++) { send.L[k] += bus.L[k] * amount; send.R[k] += bus.R[k] * amount; } };

  // 铺底
  for (const b of bars) CHORDS[b.chord].pad.forEach((m, i) => padNote(pad, b.t, b.len, m, 0.085, (i / (CHORDS[b.chord].pad.length - 1)) * 1.2 - 0.6));
  lowpass2(pad, (t) => {
    if (t < GRID) return 450 + 950 * (t / GRID);
    if (t < QUIET[0]) return 1500;
    if (t < QUIET[1]) return 900 + 2600 * ((t - QUIET[0]) / (QUIET[1] - QUIET[0])) ** 2;
    if (t < OUTRO) return 2400;
    return 900 + 1400 * Math.exp(-(t - OUTRO) / 0.9);
  });

  // 鼓、贝斯、琶音
  const kicks = [];
  for (const [from, to, origin] of sections) {
    for (const { t, n } of beatsIn(from, to, origin)) {
      kick(drums, t, 0.85);
      kicks.push(t);
      if (t >= 7.8) bassNote(bass, t, BEAT * 0.8, chordAt(t + 0.01).bass, 0.42);
      if (t >= 12.2 && n % 2 === 1 && !whileTyping(t)) clap(drums, t, 0.3);
    }
    for (const { t } of beatsIn(Math.max(from, 7.8), to, origin, BEAT, BEAT / 2)) if (!whileTyping(t)) hat(drums, t, 0.16, 0.25);
    for (const { t, n } of beatsIn(Math.max(from, 694 / 30), Math.min(to, 34.13), origin, BEAT / 4)) if (n % 2 === 1) hat(drums, t, 0.06, -0.3, 0.02);
    const shape = [0, 1, 2, 3, 2, 1, 2, 3];
    for (const { t, n } of beatsIn(Math.max(from, 12.2), to, origin, BEAT / 2)) {
      if (whileTyping(t)) continue;
      const tones = chordAt(t + 0.01).pad;
      pluck(arp, t, tones[shape[n % shape.length]] + 12, 0.075, n % 2 ? 0.35 : -0.35);
    }
  }
  duck(pad, kicks, 0.45, 0.14);
  duck(bass, kicks, 0.6, 0.1);

  // 开场：三个词各一声，标志出现一声钟
  [[0.12, 69], [0.49, 74], [1.04, 78]].forEach(([t, m], i) => pluck(arp, t, m, 0.09, [-0.3, 0, 0.3][i]));
  riser(sfx, 2.6, GRID, 0.1);
  bell(sfx, 2.42, 81, 0.11, -0.15);
  bell(sfx, 2.5, 86, 0.08, 0.15);
  boom(drums, GRID, 0.5);

  // 切场
  for (const c of CUTS) if (c !== GRID && c !== OUTRO) whoosh(sfx, c, 0.16);

  // 回答出来、管控那一段
  bell(sfx, 50.25, 81, 0.07, -0.2);
  bell(sfx, 50.35, 88, 0.05, 0.2);
  boom(drums, QUIET[0], 0.35);
  bell(sfx, QUIET[0] + 0.05, 74, 0.08, 0);
  riser(sfx, QUIET[0] + 0.6, QUIET[1], 0.14);
  boom(drums, QUIET[1], 0.4);

  // 片尾
  kick(drums, OUTRO, 1);
  boom(drums, OUTRO, 0.55);
  crash(drums, OUTRO, 0.13);
  bell(sfx, 58.95, 86, 0.1, -0.15);
  bell(sfx, 59.05, 93, 0.06, 0.15);

  // 两次点发送
  for (const { send: t } of TYPING) sendPop(sfx, t, 0.2);

  sendTo(pad, 0.5); sendTo(arp, 1.4); sendTo(sfx, 1.2); sendTo(drums, 0.15);
  const wet = reverb(send);

  const mix = new Bus();
  // 各声部的比例：整段平均下来鼓约 -19、贝斯和铺底约 -21、琶音约 -25 dBFS（母带前），音效稀疏但听得清
  const stems = { pad: [pad, 3.5], bass: [bass, 1], drums: [drums, 0.5], arp: [arp, 3.5], sfx: [sfx, 2.5], reverb: [wet, 2] };
  for (const [bus, g] of Object.values(stems)) for (let k = 0; k < N; k++) { mix.L[k] += bus.L[k] * g; mix.R[k] += bus.R[k] * g; }
  const rms = (b) => Math.sqrt(b.L.reduce((s, x) => s + x * x, 0) / N);
  console.log("stem RMS (dBFS, pre-master):", Object.fromEntries(Object.entries(stems).map(([k, [b, g]]) => [k, +(20 * Math.log10(rms(b) * g + 1e-12)).toFixed(1)])));
  return mix;
}

function typingClicks(mix, lang) {
  const html = readFileSync(join(outDir, `dsh-ee-openmotion-film-${lang}/index.html`), "utf8");
  const vars = JSON.parse(html.match(/data-composition-variables='([^']*)'/)[1].replace(/&#39;/g, "'").replace(/&amp;/g, "&"));
  const keys = new Bus(), r = rng(lang.charCodeAt(0) * 31 + lang.charCodeAt(1));
  for (const { id, start, dur } of TYPING) {
    const text = vars.find((v) => v.id === id).default;
    // 和画面一样：第 i 个字在 start + dur * i / 总字数 出现
    [...text].forEach((ch, i) => { if (ch.trim()) keyClick(keys, start + (dur * (i + 1)) / text.length, 0.4, (r() - 0.5) * 0.4, r); });
  }
  const out = new Bus();
  out.L.set(mix.L); out.R.set(mix.R);
  for (let k = 0; k < N; k++) { out.L[k] += keys.L[k]; out.R[k] += keys.R[k]; }
  return out;
}

// 母带：去直流、按基准混音定响度，软削波，片尾淡出
function master(bus, gain) {
  const out = [bus.L, bus.R].map((buf) => {
    const y = new Float32Array(N);
    let prevX = 0, prevY = 0;
    const r = 1 - TAU * 20 / SR;
    for (let k = 0; k < N; k++) {
      const hp = buf[k] - prevX + r * prevY; prevX = buf[k]; prevY = hp;
      const t = k / SR, fade = Math.min(1, t / 0.02) * Math.min(1, (DUR - t) / 0.7);
      y[k] = (Math.tanh(1.3 * hp * gain) / Math.tanh(1.3)) * fade;
    }
    return y;
  });
  let peak = 0;
  for (const y of out) for (const v of y) peak = Math.max(peak, Math.abs(v));
  const lim = Math.min(1, 0.891 / peak); // 峰值不超过 -1 dBFS
  for (const y of out) for (let k = 0; k < N; k++) y[k] *= lim;
  return out;
}

function writeWav(path, [L, R]) {
  const buf = Buffer.alloc(44 + N * 4);
  buf.write("RIFF", 0); buf.writeUInt32LE(36 + N * 4, 4); buf.write("WAVE", 8);
  buf.write("fmt ", 12); buf.writeUInt32LE(16, 16); buf.writeUInt16LE(1, 20); buf.writeUInt16LE(2, 22);
  buf.writeUInt32LE(SR, 24); buf.writeUInt32LE(SR * 4, 28); buf.writeUInt16LE(4, 32); buf.writeUInt16LE(16, 34);
  buf.write("data", 36); buf.writeUInt32LE(N * 4, 40);
  const d = rng(99);
  for (let k = 0; k < N; k++) {
    for (const [c, ch] of [[0, L], [1, R]]) {
      const v = Math.max(-32768, Math.min(32767, Math.round(ch[k] * 32767 + (d() - d()))));
      buf.writeInt16LE(v, 44 + k * 4 + c * 2);
    }
  }
  writeFileSync(path, buf);
}

const base = renderBase();
// 响度以不带打字声的基准混音为准，三种语言一样响
const baseRms = Math.sqrt([base.L, base.R].reduce((s, b) => s + b.reduce((a, x) => a + x * x, 0), 0) / (2 * N));
const gain = 10 ** (-17 / 20) / baseRms;
for (const lang of LANGS) {
  const name = `dsh-ee-openmotion-film-${lang}`;
  const [L, R] = master(typingClicks(base, lang), gain);
  const wav = join(outDir, `${name}.wav`);
  writeWav(wav, [L, R]);
  const video = join(outDir, `${name}.mp4`), silent = join(outDir, `${name}.silent.mp4`);
  if (!existsSync(silent)) renameSync(video, silent);
  execFileSync(FFMPEG, ["-hide_banner", "-loglevel", "error", "-y", "-i", silent, "-i", wav, "-map", "0:v:0", "-map", "1:a:0", "-c:v", "copy", "-c:a", "aac", "-b:a", "192k", "-shortest", "-movflags", "+faststart", video]);
  let peak = 0, sum = 0;
  for (const ch of [L, R]) for (const v of ch) { peak = Math.max(peak, Math.abs(v)); sum += v * v; }
  console.log(`${lang}: ${video} (RMS ${(10 * Math.log10(sum / (2 * N))).toFixed(1)} dBFS, peak ${(20 * Math.log10(peak)).toFixed(1)} dBFS)`);
}
