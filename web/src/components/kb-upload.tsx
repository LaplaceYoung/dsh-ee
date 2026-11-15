import { useEffect, useRef, useState } from "react";
import { Link } from "react-router";
import { useTranslation } from "react-i18next";
import { FileText, FolderUp, Upload, X } from "lucide-react";
import { api, type KBase, type KDoc, type KBUploadResult } from "../lib/api";
import { useStore } from "../lib/store";
import { ErrorNote } from "./api-ui";
import { Badge, Button, cx, Field, inputCls } from "./ui";
import { ConvertNotes } from "./kb-convert";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";

/** 能上传的格式（跟服务端 kb.Formats 一致）。 */
export const KB_ACCEPT = ".pdf,.docx,.xlsx,.csv,.pptx,.md,.markdown,.txt";
const MAX_FILE = 50 << 20;

/** 一个要上传的文件；dir 是拖进来的文件夹里的相对目录。 */
export interface QueuedFile {
  file: File;
  dir: string;
}

function joinFolder(base: string, dir: string): string {
  return [base, dir].map((x) => x.replace(/^\/+|\/+$/g, "")).filter(Boolean).join("/");
}

const skipName = (name: string) => name.startsWith(".") || name.startsWith("~$") || name === "Thumbs.db";

/** 从拖放里取文件；拖的是文件夹时一层层读进去，记下相对目录。 */
export async function filesFromDrop(dt: DataTransfer): Promise<QueuedFile[]> {
  const out: QueuedFile[] = [];
  const entries = [...dt.items].map((it) => (typeof it.webkitGetAsEntry === "function" ? it.webkitGetAsEntry() : null));
  if (entries.every((e) => !e)) {
    return [...dt.files].filter((f) => !skipName(f.name)).map((file) => ({ file, dir: "" }));
  }
  const walk = async (entry: FileSystemEntry, dir: string): Promise<void> => {
    if (skipName(entry.name)) return;
    if (entry.isFile) {
      const file = await new Promise<File>((resolve, reject) => (entry as FileSystemFileEntry).file(resolve, reject));
      out.push({ file, dir });
      return;
    }
    if (entry.isDirectory) {
      const reader = (entry as FileSystemDirectoryEntry).createReader();
      const sub = joinFolder(dir, entry.name);
      // readEntries 一次只给一部分，要读到空为止
      for (;;) {
        const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject));
        if (batch.length === 0) break;
        for (const e of batch) await walk(e, sub);
      }
    }
  };
  for (const e of entries) if (e) await walk(e, "");
  return out;
}

/** 选文件夹（<input webkitdirectory>）时，文件带着相对路径。 */
function filesFromInput(list: FileList | null): QueuedFile[] {
  return [...(list ?? [])]
    .filter((f) => !skipName(f.name))
    .map((file) => {
      const rel = (file as any).webkitRelativePath as string | undefined;
      const dir = rel && rel.includes("/") ? rel.slice(0, rel.lastIndexOf("/")) : "";
      return { file, dir };
    });
}

function sizeText(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1 << 20) return `${(n / 1024).toFixed(0)} KB`;
  return `${(n / (1 << 20)).toFixed(1)} MB`;
}

type Row = QueuedFile & {
  folder: string;
  /** 同目录下的同名文档 */
  same?: KDoc;
  /** 同名时另存为新文档 */
  asNew: boolean;
  result?: KBUploadResult;
};

/**
 * 上传文档：选文件或拖进来（可以是文件夹）→ 看哪些是同名的（默认更新成新版本，可以改成另存一篇）→ 上传 →
 * 每个文件一张小卡，写着转换得怎么样（收录了几页、哪几页没收录、补回了多少条款编号……），点「预览」看阅读视图。
 */
export function UploadDialog({ kb, open, onOpenChange, onDone, initial, initialFolder }: {
  kb: KBase;
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onDone: () => void;
  /** 从文档列表拖进来的文件 */
  initial?: QueuedFile[];
  initialFolder?: string;
}) {
  const { t } = useTranslation();
  const { notify } = useStore();
  const [folder, setFolder] = useState("");
  const [rows, setRows] = useState<Row[]>([]);
  const [checking, setChecking] = useState(false);
  const [busy, setBusy] = useState(false);
  const [done, setDone] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [dragging, setDragging] = useState(false);
  const fileInput = useRef<HTMLInputElement>(null);
  const dirInput = useRef<HTMLInputElement>(null);
  const checkSeq = useRef(0);
  const folderTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // 按目录分组查同名文档
  const check = async (queued: QueuedFile[], base: string) => {
    const seq = ++checkSeq.current;
    const previous = new Map(rows.map((r) => [`${r.dir}/${r.file.name}`, r.asNew]));
    const next: Row[] = queued.map((q) => ({ ...q, folder: joinFolder(base, q.dir), asNew: previous.get(`${q.dir}/${q.file.name}`) ?? false }));
    setRows(next);
    if (next.length === 0) return;
    setChecking(true);
    try {
      const groups = new Map<string, Row[]>();
      for (const r of next) groups.set(r.folder, [...(groups.get(r.folder) ?? []), r]);
      for (const [f, list] of groups) {
        // 服务端一次最多 50 个；和上传一样分批，否则同目录 51 个文件预检直接 400。
        for (let i = 0; i < list.length; i += 50) {
          const part = list.slice(i, i + 50);
          const res = await api.checkKBUpload(kb.id, f, part.map((r) => r.file.name));
          for (const it of res.items) {
            const row = part.find((r) => r.file.name === it.file);
            if (row) row.same = it.doc;
          }
        }
      }
      if (seq !== checkSeq.current) return;
      setRows([...next]);
    } catch (e) {
      if (seq === checkSeq.current) setError(e instanceof Error ? e.message : String(e));
    } finally {
      if (seq === checkSeq.current) setChecking(false);
    }
  };

  useEffect(() => {
    if (!open) return;
    setError(null);
    setDone(false);
    const base = initialFolder ?? "";
    setFolder(base);
    void check(initial ?? [], base);
  }, [open]);

  useEffect(() => () => {
    if (folderTimer.current) clearTimeout(folderTimer.current);
    checkSeq.current++;
  }, []);

  // 改了目标目录：防抖后重新查同名；旧请求由 checkSeq 丢弃
  const changeFolder = (v: string) => {
    setFolder(v);
    if (folderTimer.current) clearTimeout(folderTimer.current);
    folderTimer.current = setTimeout(() => {
      if (rows.length && !done) void check(rows, v);
    }, 250);
  };
  const add = (more: QueuedFile[]) => {
    if (done) return void check(more, folder);
    const seen = new Set(rows.map((r) => `${r.dir}/${r.file.name}`));
    void check([...rows, ...more.filter((m) => !seen.has(`${m.dir}/${m.file.name}`))], folder);
  };

  const upload = async () => {
    setBusy(true);
    setError(null);
    try {
      const groups = new Map<string, Row[]>();
      for (const r of rows) if (r.file.size <= MAX_FILE) groups.set(r.folder, [...(groups.get(r.folder) ?? []), r]);
      for (const [f, list] of groups) {
        // 一次最多 50 个文件
        for (let i = 0; i < list.length; i += 50) {
          const part = list.slice(i, i + 50);
          try {
            const res = await api.uploadKBDocs(kb.id, part.map((r) => r.file), f, { asNew: part.filter((r) => r.asNew).map((r) => r.file.name) });
            part.forEach((r, k) => (r.result = res.results[k] ?? { file: r.file.name, error: t("kupload.noResult") }));
          } catch (e) {
            const msg = e instanceof Error ? e.message : String(e);
            part.forEach((r) => (r.result = { file: r.file.name, error: msg }));
          }
          setRows((cur) => [...cur]);
        }
      }
      for (const r of rows) if (r.file.size > MAX_FILE) r.result = { file: r.file.name, error: t("kupload.tooBig") };
      setRows([...rows]);
      const ok = rows.filter((r) => r.result?.doc).length;
      const failed = rows.length - ok;
      notify(failed ? `${t("kbase.uploadDone", { ok })}，${t("kbase.uploadSomeFailed", { failed })}` : t("kbase.uploadDone", { ok }));
      setDone(true);
      onDone();
    } finally {
      setBusy(false);
    }
  };

  const updates = rows.filter((r) => r.same && !r.asNew).length;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle>{t("kbase.uploadTitle")}</DialogTitle>
          <DialogDescription>{kb.title}</DialogDescription>
        </DialogHeader>
        <div className="space-y-4">
          <Field label={t("kbase.uploadFolder")} hint={t("kbase.uploadFolderHint")}>
            <input className={inputCls} value={folder} disabled={busy || done} onChange={(e) => changeFolder(e.target.value)} />
          </Field>
          {!done && (
            <div
              onDragOver={(e) => {
                e.preventDefault();
                setDragging(true);
              }}
              onDragLeave={() => setDragging(false)}
              onDrop={async (e) => {
                e.preventDefault();
                setDragging(false);
                add(await filesFromDrop(e.dataTransfer));
              }}
              className={cx(
                "rounded-lg border-2 border-dashed px-4 py-6 text-center transition",
                dragging ? "border-brand-400 bg-brand-50/60 dark:bg-brand-500/10" : "border-neutral-200",
              )}
            >
              <Upload size={20} className="mx-auto mb-2 text-neutral-400" aria-hidden />
              <p className="text-sm text-neutral-700">{t("kupload.drop")}</p>
              <p className="mt-1 text-xs text-neutral-500">{t("kupload.formats")}</p>
              <div className="mt-3 flex justify-center gap-2">
                <Button size="sm" variant="secondary" onClick={() => fileInput.current?.click()}>
                  <FileText size={14} /> {t("kupload.pickFiles")}
                </Button>
                <Button size="sm" variant="secondary" onClick={() => dirInput.current?.click()}>
                  <FolderUp size={14} /> {t("kupload.pickFolder")}
                </Button>
              </div>
              <input ref={fileInput} type="file" multiple accept={KB_ACCEPT} className="hidden" onChange={(e) => (add(filesFromInput(e.target.files)), (e.target.value = ""))} />
              <input ref={dirInput} type="file" multiple className="hidden" {...{ webkitdirectory: "" }} onChange={(e) => (add(filesFromInput(e.target.files)), (e.target.value = ""))} />
            </div>
          )}
          {rows.length > 0 && (
            <ul className="max-h-[46vh] divide-y divide-neutral-100 overflow-auto rounded-lg border border-neutral-200">
              {rows.map((r, i) => (
                <li key={`${r.dir}/${r.file.name}/${i}`} className="px-3 py-2.5 text-sm">
                  <div className="flex items-start justify-between gap-3">
                    <div className="min-w-0">
                      <div className="truncate font-medium text-neutral-900">{r.file.name}</div>
                      <div className="text-xs text-neutral-500">
                        {sizeText(r.file.size)}
                        {r.folder && ` · ${r.folder}`}
                      </div>
                    </div>
                    <div className="flex shrink-0 items-center gap-2">
                      {!r.result && r.file.size > MAX_FILE && <Badge tone="red">{t("kupload.tooBig")}</Badge>}
                      {!r.result && r.file.size <= MAX_FILE && r.same && !r.asNew && <Badge tone="brand">{t("kupload.willUpdate", { version: r.same.version + 1 })}</Badge>}
                      {!r.result && r.file.size <= MAX_FILE && (!r.same || r.asNew) && <Badge>{t("kupload.willCreate")}</Badge>}
                      {!r.result && r.same && (
                        <button className="text-xs text-neutral-600 underline-offset-2 hover:underline" disabled={busy} onClick={() => setRows(rows.map((x) => (x === r ? { ...x, asNew: !x.asNew } : x)))}>
                          {r.asNew ? t("kupload.backToUpdate") : t("kupload.saveAsNew")}
                        </button>
                      )}
                      {!r.result && !busy && (
                        <button aria-label={t("kupload.remove")} className="text-neutral-400 hover:text-neutral-700" onClick={() => setRows(rows.filter((x) => x !== r))}>
                          <X size={14} />
                        </button>
                      )}
                      {r.result?.doc && <Badge tone="green">{r.result.updated ? t("kupload.updated", { version: r.result.doc.version }) : t("kupload.created")}</Badge>}
                      {r.result?.doc && (
                        <Link to={`/knowledge/docs/${r.result.doc.id}`} className="text-xs font-medium text-brand-600 hover:underline dark:text-brand-300" onClick={() => onOpenChange(false)}>
                          {t("kupload.preview")}
                        </Link>
                      )}
                    </div>
                  </div>
                  {!r.result && r.same && !r.asNew && (
                    <p className="mt-1 text-xs text-neutral-500">{t("kupload.sameHint", { title: r.same.title })}</p>
                  )}
                  {r.result?.doc && <ConvertNotes info={r.result.doc.convert_info} className="mt-2 text-[13px]" />}
                  {r.result?.error && <p className="mt-1 text-[13px] text-red-600 dark:text-red-400">{r.result.error}</p>}
                </li>
              ))}
            </ul>
          )}
          {checking && <p className="text-xs text-neutral-500">{t("kupload.checking")}</p>}
        </div>
        <ErrorNote>{error}</ErrorNote>
        <DialogFooter>
          {done ? (
            <Button onClick={() => onOpenChange(false)}>{t("kupload.finish")}</Button>
          ) : (
            <>
              <Button variant="secondary" onClick={() => onOpenChange(false)}>
                {t("common.cancel")}
              </Button>
              <Button disabled={busy || checking || rows.length === 0} onClick={() => void upload()}>
                {busy ? t("kupload.uploading") : updates ? t("kupload.submitWithUpdates", { count: rows.length, updates }) : t("kupload.submit", { count: rows.length })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
