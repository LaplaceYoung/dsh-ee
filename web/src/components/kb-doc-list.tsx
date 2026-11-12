import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { ChevronRight, Folder, FolderOpen, SearchX, FileText } from "lucide-react";
import { api, formatTime, type KBase, type KBHit, type KDoc } from "../lib/api";
import { useStore } from "../lib/store";
import { useConfirm } from "../lib/confirm";
import { Badge, Button, cx, Field, inputBase, inputCls, Segmented, Empty } from "./ui";
import { Snippet } from "./kb-access";
import { convertWarns } from "./kb-convert";
import { ErrorNote } from "./api-ui";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";

const fileKinds: Record<string, { label: string; cls: string }> = {
  pdf: { label: "PDF", cls: "bg-red-50 text-red-700 dark:bg-red-500/15 dark:text-red-300" },
  docx: { label: "W", cls: "bg-blue-50 text-blue-700 dark:bg-blue-500/15 dark:text-blue-300" },
  xlsx: { label: "X", cls: "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300" },
  csv: { label: "CSV", cls: "bg-emerald-50 text-emerald-700 dark:bg-emerald-500/15 dark:text-emerald-300" },
  pptx: { label: "P", cls: "bg-orange-50 text-orange-700 dark:bg-orange-500/15 dark:text-orange-300" },
  txt: { label: "TXT", cls: "bg-neutral-100 text-neutral-600" },
  md: { label: "MD", cls: "bg-neutral-100 text-neutral-600" },
};

/** 文件类型的小方块：一眼看出是 PDF、Word 还是 Excel。 */
export function FileBadge({ doc }: { doc: KDoc }) {
  const ext = doc.convert_info?.format || doc.source_name.split(".").pop()?.toLowerCase() || "md";
  const k = fileKinds[ext] ?? fileKinds.md;
  return (
    <span aria-hidden className={cx("mt-0.5 inline-flex h-6 w-7 shrink-0 items-center justify-center rounded text-[10px] font-bold tracking-tight", k.cls)}>
      {k.label}
    </span>
  );
}

const inFolder = (doc: KDoc, folder: string | null) => folder === null || doc.folder === folder || doc.folder.startsWith(folder + "/");

type TreeNode = { name: string; path: string; count: number; children: TreeNode[] };

/** 由文档的目录（「行政/差旅」）拼出目录树，每个目录带上它和子目录里的文档数。 */
function buildTree(docs: KDoc[]): TreeNode[] {
  const root: TreeNode = { name: "", path: "", count: 0, children: [] };
  for (const d of docs) {
    if (!d.folder) continue;
    let node = root;
    const parts = d.folder.split("/");
    parts.forEach((part, i) => {
      const path = parts.slice(0, i + 1).join("/");
      let child = node.children.find((c) => c.path === path);
      if (!child) {
        child = { name: part, path, count: 0, children: [] };
        node.children.push(child);
      }
      child.count++;
      node = child;
    });
  }
  const sort = (n: TreeNode) => {
    n.children.sort((a, b) => a.name.localeCompare(b.name, "zh"));
    n.children.forEach(sort);
  };
  sort(root);
  return root.children;
}

function TreeItem({ node, depth, folder, setFolder }: { node: TreeNode; depth: number; folder: string | null; setFolder: (f: string | null) => void }) {
  const active = folder === node.path;
  const open = folder !== null && (folder === node.path || folder.startsWith(node.path + "/"));
  const Icon = open ? FolderOpen : Folder;
  return (
    <li>
      <button
        onClick={() => setFolder(node.path)}
        aria-current={active ? "true" : undefined}
        className={cx(
          "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm transition",
          active ? "bg-brand-50 font-medium text-brand-700 dark:bg-brand-500/15 dark:text-brand-200" : "text-neutral-700 hover:bg-neutral-100",
        )}
        style={{ paddingLeft: 8 + depth * 14 }}
      >
        {node.children.length > 0 ? <ChevronRight size={12} className={cx("shrink-0 transition", open && "rotate-90")} /> : <span className="w-3 shrink-0" />}
        <Icon size={14} className="shrink-0 text-neutral-400" aria-hidden />
        <span className="min-w-0 flex-1 truncate">{node.name}</span>
        <span className="text-xs text-neutral-400 tabular-nums">{node.count}</span>
      </button>
      {open && node.children.length > 0 && (
        <ul>
          {node.children.map((c) => (
            <TreeItem key={c.path} node={c} depth={depth + 1} folder={folder} setFolder={setFolder} />
          ))}
        </ul>
      )}
    </li>
  );
}

function MoveDialog({ open, count, onOpenChange, onMove }: { open: boolean; count: number; onOpenChange: (v: boolean) => void; onMove: (folder: string) => Promise<string | null> }) {
  const { t } = useTranslation();
  const [folder, setFolder] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  useEffect(() => {
    if (open) {
      setFolder("");
      setError(null);
    }
  }, [open]);
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{t("kdocs.moveTitle", { count })}</DialogTitle>
        </DialogHeader>
        <Field label={t("kbase.uploadFolder")} hint={t("kbase.uploadFolderHint")}>
          <input className={inputCls} value={folder} autoFocus onChange={(e) => setFolder(e.target.value)} />
        </Field>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          <Button variant="secondary" onClick={() => onOpenChange(false)}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={busy}
            onClick={async () => {
              setBusy(true);
              const err = await onMove(folder);
              setBusy(false);
              if (err) setError(err);
              else onOpenChange(false);
            }}
          >
            {t("kdocs.move")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/**
 * 知识库的文档列表：左边目录树，上面筛选（搜标题或搜内容），每篇带文件类型和状态；维护者可以批量移动、删除。
 */
export function DocList({ kb, docs, folder, setFolder, onChanged }: { kb: KBase; docs: KDoc[]; folder: string | null; setFolder: (f: string | null) => void; onChanged: () => void }) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const confirm = useConfirm();
  const tree = useMemo(() => buildTree(docs), [docs]);
  const rootCount = docs.filter((d) => !d.folder).length;
  const [mode, setMode] = useState<"title" | "content">("title");
  const [filter, setFilter] = useState("");
  const [hits, setHits] = useState<Map<number, KBHit> | null>(null);
  const searchSeq = useRef(0);
  const [selected, setSelected] = useState<Set<number>>(new Set());
  const [moving, setMoving] = useState(false);

  // 搜内容：停手 300 毫秒后查，结果按文档聚合（每篇留第一处命中）
  useEffect(() => {
    const q = filter.trim();
    const seq = ++searchSeq.current;
    if (mode !== "content" || !q) {
      setHits(null);
      return;
    }
    const timer = setTimeout(async () => {
      try {
        const r = await api.searchKB({ query: q, kb_ids: [kb.id], folder: folder ?? "" });
        // 输入变化后，慢的旧响应不能覆盖新查询的结果。
        if (seq !== searchSeq.current) return;
        const m = new Map<number, KBHit>();
        for (const h of r.hits) if (!m.has(h.doc)) m.set(h.doc, h);
        for (const d of r.docs ?? []) if (!m.has(d.doc)) m.set(d.doc, { doc: d.doc, title: d.title, snippet: "", heading: "", line: 1, from: 1, to: 1 } as KBHit);
        setHits(m);
      } catch {
        if (seq === searchSeq.current) setHits(new Map());
      }
    }, 300);
    return () => {
      clearTimeout(timer);
      // 卸载/依赖变化时让在途请求作废。
      if (seq === searchSeq.current) searchSeq.current++;
    };
  }, [mode, filter, folder, kb.id]);

  const q = filter.trim().toLowerCase();
  const shown = docs.filter((d) => {
    if (!inFolder(d, folder)) return false;
    if (!q) return true;
    if (mode === "content") return hits?.has(d.id) ?? false;
    return d.title.toLowerCase().includes(q) || d.source_name.toLowerCase().includes(q);
  });
  const allChecked = shown.length > 0 && shown.every((d) => selected.has(d.id));
  const toggle = (id: number) => setSelected((s) => {
    const n = new Set(s);
    if (n.has(id)) n.delete(id);
    else n.add(id);
    return n;
  });
  const chosen = docs.filter((d) => selected.has(d.id));

  const moveTo = async (target: string): Promise<string | null> => {
    try {
      for (const d of chosen) await api.patchKBDoc(d.id, { folder: target });
      notify(t("kdocs.moved", { count: chosen.length }));
      setSelected(new Set());
      onChanged();
      return null;
    } catch (e) {
      onChanged();
      return e instanceof Error ? e.message : String(e);
    }
  };
  const removeAll = async () => {
    if (!(await confirm({ kind: "delete", description: t("kdocs.deleteConfirm", { count: chosen.length }) }))) return;
    try {
      for (const d of chosen) await api.deleteKBDoc(d.id);
      notify(t("kdocs.deleted", { count: chosen.length }));
    } catch (e) {
      notify(e instanceof Error ? e.message : String(e));
    }
    setSelected(new Set());
    onChanged();
  };

  return (
    <div className="grid md:grid-cols-[220px_1fr]">
      <nav aria-label={t("kdocs.folders")} className="border-b border-neutral-100 p-3 md:border-r md:border-b-0">
        <ul className="space-y-0.5">
          <li>
            <button
              onClick={() => setFolder(null)}
              aria-current={folder === null ? "true" : undefined}
              className={cx(
                "flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm transition",
                folder === null ? "bg-brand-50 font-medium text-brand-700 dark:bg-brand-500/15 dark:text-brand-200" : "text-neutral-700 hover:bg-neutral-100",
              )}
            >
              <span className="min-w-0 flex-1">{t("kbase.folderAll")}</span>
              <span className="text-xs text-neutral-400 tabular-nums">{docs.length}</span>
            </button>
          </li>
          {tree.map((n) => (
            <TreeItem key={n.path} node={n} depth={0} folder={folder} setFolder={setFolder} />
          ))}
          {rootCount > 0 && tree.length > 0 && (
            <li>
              <button
                onClick={() => setFolder("")}
                aria-current={folder === "" ? "true" : undefined}
                className={cx("flex w-full items-center gap-1.5 rounded px-2 py-1 text-left text-sm transition", folder === "" ? "bg-brand-50 font-medium text-brand-700 dark:bg-brand-500/15 dark:text-brand-200" : "text-neutral-500 hover:bg-neutral-100")}
              >
                <span className="min-w-0 flex-1">{t("kdocs.unfiled")}</span>
                <span className="text-xs text-neutral-400 tabular-nums">{rootCount}</span>
              </button>
            </li>
          )}
        </ul>
      </nav>
      <div className="min-w-0">
        <div className="flex flex-wrap items-center gap-2 px-5 pt-4 pb-3">
          <Segmented
            value={mode}
            onChange={setMode}
            options={[
              { value: "title", label: t("kdocs.byTitle") },
              { value: "content", label: t("kdocs.byContent") },
            ]}
          />
          <input
            className={cx(inputBase, "h-9 w-72 max-w-full")}
            value={filter}
            placeholder={mode === "title" ? t("kbase.filter") : t("kdocs.contentPlaceholder")}
            aria-label={mode === "title" ? t("kbase.filter") : t("kdocs.contentPlaceholder")}
            onChange={(e) => setFilter(e.target.value)}
          />
          {kb.can_edit && chosen.length > 0 && (
            <div className="ml-auto flex items-center gap-2 text-sm">
              <span className="text-neutral-600">{t("kdocs.selected", { count: chosen.length })}</span>
              <Button size="sm" variant="secondary" onClick={() => setMoving(true)}>
                {t("kdocs.moveTo")}
              </Button>
              <Button size="sm" variant="secondary" className="text-red-600" onClick={() => void removeAll()}>
                {t("kdocs.delete")}
              </Button>
            </div>
          )}
        </div>
        {shown.length === 0 ? (
          <Empty icon={q ? SearchX : FileText}>
            {q ? t("kdocs.noMatch") : kb.can_edit && docs.length === 0 ? t("kbase.docsEmptyEdit") : t("kbase.docsEmpty")}
          </Empty>
        ) : (
          <div className="overflow-x-auto">
            <table className="tbl max-sm:min-w-[34rem]">
              <thead>
                <tr className="border-y border-neutral-100 text-left text-xs text-neutral-500">
                  {kb.can_edit && (
                    <th className="w-8 py-2 pl-5">
                      <input
                        type="checkbox"
                        aria-label={t("kdocs.selectAll")}
                        checked={allChecked}
                        onChange={() => setSelected(allChecked ? new Set() : new Set(shown.map((d) => d.id)))}
                      />
                    </th>
                  )}
                  <th className={cx("py-2 pr-3", kb.can_edit ? "pl-2" : "pl-5")}>{t("kbase.colTitle")}</th>
                  <th className="px-3 py-2 text-right">{t("kbase.colLines")}</th>
                  <th className="px-3 py-2 text-right">{t("kbase.colVersion")}</th>
                  <th className="px-5 py-2">{t("kbase.colUpdated")}</th>
                </tr>
              </thead>
              <tbody>
                {shown.map((d) => {
                  const hit = mode === "content" ? hits?.get(d.id) : undefined;
                  return (
                    <tr key={d.id} className={cx("border-b border-neutral-50 last:border-0", selected.has(d.id) && "bg-brand-50/40 dark:bg-brand-500/5")}>
                      {kb.can_edit && (
                        <td className="py-2 pl-5 align-top">
                          <input type="checkbox" className="mt-1.5" aria-label={t("kdocs.select", { title: d.title })} checked={selected.has(d.id)} onChange={() => toggle(d.id)} />
                        </td>
                      )}
                      <td className={cx("min-w-44 py-2 pr-3", kb.can_edit ? "pl-2" : "pl-5")}>
                        <div className="flex items-start gap-2.5">
                          <FileBadge doc={d} />
                          <div className="min-w-0">
                            <div className="flex flex-wrap items-center gap-1.5">
                              <Link
                                to={`/knowledge/docs/${d.id}${hit && hit.snippet ? `#L${hit.line}` : ""}`}
                                className="font-medium text-brand-600 hover:text-brand-700 dark:text-brand-300 dark:hover:text-brand-200"
                              >
                                {d.title}
                              </Link>
                              {convertWarns(d.convert_info) > 0 && <Badge tone="amber">{t("kupload.skippedBadge", { count: convertWarns(d.convert_info) })}</Badge>}
                              {d.convert_info?.edited && <Badge>{t("kupload.editedBadge")}</Badge>}
                            </div>
                            <div className="text-xs text-neutral-500">
                              {[d.folder, d.source_name].filter(Boolean).join(" · ")}
                            </div>
                            {hit?.snippet && (
                              <div className="mt-1 text-[13px] text-neutral-700">
                                <Snippet text={hit.snippet} />
                              </div>
                            )}
                          </div>
                        </div>
                      </td>
                      <td className="px-3 py-2 text-right align-top text-neutral-700 tabular-nums">{d.lines}</td>
                      <td className="px-3 py-2 text-right align-top text-neutral-700">v{d.version}</td>
                      <td className="px-5 py-2 align-top whitespace-nowrap text-neutral-600">
                        {formatTime(d.updated_at)} {d.updated_by_name && `· ${d.updated_by_name}`}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          </div>
        )}
      </div>
      <MoveDialog open={moving} count={chosen.length} onOpenChange={setMoving} onMove={moveTo} />
    </div>
  );
}
