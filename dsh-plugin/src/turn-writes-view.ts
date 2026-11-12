import type { WriteRecord } from "./write-log.js";

export interface TurnWrites {
  /** dsh-ee 网页地址，拼详情链接用 */
  base: string;
  writes: WriteRecord[];
}

export interface TurnWriteGroup {
  /** 企业系统名；ee_* 工具归到「dsh-ee」 */
  system: string;
  /** 网页详情，没有资产 id 时为空 */
  url?: string;
  /** 每种操作的次数，按第一次出现的顺序 */
  ops: { label: string; ok: number; failed: number }[];
}

/** 把一轮里的写操作按系统归组，操作按名字合并计数。 */
export function groupTurnWrites(data: TurnWrites | undefined): TurnWriteGroup[] {
  const groups = new Map<string, TurnWriteGroup>();
  for (const item of data?.writes ?? []) {
    const system = item.system || "dsh-ee";
    const key = `${item.asset ?? ""}\n${system}`;
    let group = groups.get(key);
    if (!group) {
      group = { system, ops: [], ...(item.asset && data?.base ? { url: `${data.base}/assets/${item.asset}` } : {}) };
      groups.set(key, group);
    }
    let op = group.ops.find((entry) => entry.label === item.label);
    if (!op) {
      op = { label: item.label, ok: 0, failed: 0 };
      group.ops.push(op);
    }
    if (item.ok) op.ok++;
    else op.failed++;
  }
  return [...groups.values()];
}

function opText(op: TurnWriteGroup["ops"][number]): string {
  const parts: string[] = [];
  if (op.ok) parts.push(op.ok > 1 ? `${op.label} ×${op.ok}` : op.label);
  if (op.failed) parts.push(`${op.label} 未成功${op.failed > 1 ? ` ×${op.failed}` : ""}`);
  return parts.join("、");
}

/**
 * 一轮末尾的「已留痕」提示：这一轮经 dsh-ee 改了哪些企业系统、做了什么。
 * 没有写操作时返回 null，不占位置。
 */
export function turnWritesView(createElement: (...args: any[]) => any, groups: TurnWriteGroup[], openWeb: (url: string) => void) {
  if (!groups.length) return null;
  const failed = groups.some((group) => group.ops.some((op) => op.failed));
  return createElement("div", {
    "data-dsh-ee-ui": "turn-writes",
    role: "note",
    style: { display: "grid", gap: 3, margin: "6px 0 2px", padding: "7px 10px", borderRadius: 8, border: "1px solid rgba(127,127,127,.25)", fontSize: 12, lineHeight: 1.5, opacity: 0.9 },
  },
  createElement("div", { style: { fontWeight: 600 } }, failed ? "这一轮在企业系统里的写操作（有未成功的），已在 dsh-ee 留痕" : "这一轮改了企业系统里的数据，已在 dsh-ee 留痕"),
  ...groups.map((group, index) => createElement("div", { key: index, style: { display: "flex", flexWrap: "wrap", alignItems: "baseline", gap: 6 } },
    createElement("span", null, `「${group.system}」`),
    createElement("span", { style: { opacity: 0.8 } }, group.ops.map(opText).join("、")),
    group.url ? createElement("a", {
      href: group.url,
      target: "_blank",
      rel: "noopener noreferrer",
      onClick: (event: any) => { event.preventDefault(); openWeb(group.url!); },
      style: { color: "inherit", textDecoration: "underline" },
    }, "在网页查看") : null)));
}
