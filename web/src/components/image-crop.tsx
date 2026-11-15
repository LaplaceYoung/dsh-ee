import { useEffect, useState } from "react";
import Cropper, { type Area } from "react-easy-crop";
import "react-easy-crop/react-easy-crop.css";
import { useTranslation } from "react-i18next";
import i18n from "../i18n";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/shadcn/dialog";
import { Button } from "./ui";

/** SVG 是矢量图，不用裁；其余位图都走裁剪，导出的永远是正方形 PNG。 */
export function needsCrop(file: File): boolean {
  return file.type !== "image/svg+xml";
}

async function cropToPng(src: string, area: Area): Promise<Blob> {
  const img = await new Promise<HTMLImageElement>((resolve, reject) => {
    const el = new Image();
    el.onload = () => resolve(el);
    el.onerror = () => reject(new Error(i18n.t("brandAdmin.cropReadFailed")));
    el.src = src;
  });
  const out = document.createElement("canvas");
  out.width = 512;
  out.height = 512;
  const ctx = out.getContext("2d");
  if (!ctx) throw new Error(i18n.t("brandAdmin.cropCanvasUnavailable"));
  ctx.drawImage(img, area.x, area.y, area.width, area.height, 0, 0, 512, 512);
  return new Promise((resolve, reject) => out.toBlob((b) => (b ? resolve(b) : reject(new Error(i18n.t("brandAdmin.cropExportFailed")))), "image/png"));
}

/** 位图 logo 的正方形裁剪对话框：拖位置、滚轮/滑杆缩放，确定后回调 512×512 的 PNG File。 */
export function ImageCropDialog({ file, busy, onDone, onCancel }: {
  file: File | null;
  busy: boolean;
  onDone: (file: File) => void;
  onCancel: () => void;
}) {
  const { t } = useTranslation();
  const [url, setUrl] = useState("");
  const [crop, setCrop] = useState({ x: 0, y: 0 });
  const [zoom, setZoom] = useState(1);
  const [area, setArea] = useState<Area | null>(null);
  const [err, setErr] = useState("");

  useEffect(() => {
    setCrop({ x: 0, y: 0 });
    setZoom(1);
    setArea(null);
    setErr("");
    if (!file) {
      setUrl("");
      return;
    }
    const u = URL.createObjectURL(file);
    setUrl(u);
    return () => URL.revokeObjectURL(u);
  }, [file]);

  const confirm = async () => {
    if (!area || !url) return;
    try {
      const blob = await cropToPng(url, area);
      onDone(new File([blob], "logo.png", { type: "image/png" }));
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  };

  return (
    <Dialog open={!!file} onOpenChange={(open) => { if (!open) onCancel(); }}>
      <DialogContent className="sm:max-w-md">
        <DialogHeader>
          <DialogTitle>{t("brandAdmin.cropTitle")}</DialogTitle>
          <DialogDescription>{t("brandAdmin.cropHint")}</DialogDescription>
        </DialogHeader>
        <div className="relative h-72 w-full overflow-hidden rounded-lg bg-neutral-900">
          {url && (
            <Cropper
              image={url}
              crop={crop}
              zoom={zoom}
              aspect={1}
              cropShape="round"
              showGrid
              onCropChange={setCrop}
              onZoomChange={setZoom}
              onCropComplete={(_, pixels) => setArea(pixels)}
            />
          )}
        </div>
        <div className="flex items-center gap-3">
          <span className="shrink-0 text-xs text-neutral-500">{t("brandAdmin.cropZoom")}</span>
          <input type="range" min={1} max={3} step={0.01} value={zoom} onChange={(e) => setZoom(Number(e.target.value))} className="w-full accent-neutral-800" />
        </div>
        {err && <p className="text-sm text-red-600">{err}</p>}
        <DialogFooter>
          <Button size="sm" variant="ghost" onClick={onCancel} disabled={busy}>{t("brandAdmin.cropCancel")}</Button>
          <Button size="sm" onClick={() => void confirm()} disabled={!area || busy}>{busy ? t("brandAdmin.uploading") : t("brandAdmin.cropConfirm")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
