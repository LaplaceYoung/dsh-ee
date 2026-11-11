/**
 * 确认对话框：const confirm = useConfirm(); if (!(await confirm({ ... }))) return;
 * 替代 window.confirm —— 样式统一、跟随界面语言、可键盘操作（Esc 取消，焦点留在对话框内）。
 */
import { createContext, useCallback, useContext, useRef, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "@/components/shadcn/alert-dialog";

export type ConfirmKind = "delete" | "revoke" | "disable" | "reset" | "restore" | "unbind" | "rotate";

export interface ConfirmOptions {
  /** 决定标题与确认按钮的文字。 */
  kind: ConfirmKind;
  /** 说明这次操作的后果。 */
  description: string;
}

type Confirm = (opts: ConfirmOptions) => Promise<boolean>;

const Ctx = createContext<Confirm | null>(null);

export function ConfirmProvider({ children }: { children: ReactNode }) {
  const { t } = useTranslation();
  const [opts, setOpts] = useState<ConfirmOptions | null>(null);
  const resolver = useRef<((ok: boolean) => void) | null>(null);

  const confirm = useCallback<Confirm>((o) => {
    // 上一个还没答复就又弹：先按取消处理
    resolver.current?.(false);
    setOpts(o);
    return new Promise<boolean>((resolve) => {
      resolver.current = resolve;
    });
  }, []);

  const close = (ok: boolean) => {
    resolver.current?.(ok);
    resolver.current = null;
    setOpts(null);
  };

  return (
    <Ctx.Provider value={confirm}>
      {children}
      <AlertDialog open={opts !== null} onOpenChange={(open) => !open && close(false)}>
        <AlertDialogContent>
          {opts && (
            <>
              <AlertDialogHeader>
                <AlertDialogTitle>{t(`confirm.${opts.kind}.title`)}</AlertDialogTitle>
                <AlertDialogDescription>{opts.description}</AlertDialogDescription>
              </AlertDialogHeader>
              <AlertDialogFooter>
                <AlertDialogCancel onClick={() => close(false)}>{t("common.cancel")}</AlertDialogCancel>
                <AlertDialogAction variant={opts.kind === "reset" || opts.kind === "restore" || opts.kind === "rotate" ? "default" : "destructive"} onClick={() => close(true)}>
                  {t(`confirm.${opts.kind}.action`)}
                </AlertDialogAction>
              </AlertDialogFooter>
            </>
          )}
        </AlertDialogContent>
      </AlertDialog>
    </Ctx.Provider>
  );
}

export function useConfirm() {
  const c = useContext(Ctx);
  if (!c) throw new Error("useConfirm must be used inside ConfirmProvider");
  return c;
}
