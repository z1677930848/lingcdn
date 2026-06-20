import { ElMessageBox } from "element-plus"

type ConfirmBtn = string | { content?: string; theme?: string; loading?: boolean }

export type EpConfirmOptions = {
  header?: string
  body?: string
  theme?: "warning" | "danger" | "info"
  confirmBtn?: ConfirmBtn
  cancelBtn?: string
  onConfirm?: () => void | Promise<void>
  onClose?: () => void
}

type ConfirmDialogInstance = {
  destroy: () => void
}

function confirmLabel(btn: ConfirmBtn | undefined, fallback: string): string {
  if (!btn) return fallback
  if (typeof btn === "string") return btn
  return btn.content || fallback
}

/** Drop-in replacement for TDesign DialogPlugin.confirm */
export function confirmDialog(options: EpConfirmOptions): ConfirmDialogInstance {
  const type = options.theme === "danger" || options.theme === "warning" ? "warning" : "info"
  const promise = ElMessageBox.confirm(options.body || "", options.header || "确认", {
    type,
    confirmButtonText: confirmLabel(options.confirmBtn, "确定"),
    cancelButtonText: options.cancelBtn || "取消",
    distinguishCancelAndClose: true,
  })
    .then(async () => {
      await options.onConfirm?.()
    })
    .catch(() => {
      options.onClose?.()
    })

  return {
    destroy: () => {
      ElMessageBox.close()
      void promise
    },
  }
}

/** Alias for gradual migration from DialogPlugin.confirm */
export const DialogPlugin = {
  confirm: confirmDialog,
}
