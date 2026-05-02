// navigator.clipboard 仅在安全上下文（HTTPS / localhost）下可用，
// CDN 管理面板常常通过 HTTP + IP 访问，需要回退到 execCommand 方案。
export async function copyText(text: string): Promise<boolean> {
  if (text == null) return false

  if (typeof navigator !== "undefined" && navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text)
      return true
    } catch {
      // 降级到 execCommand
    }
  }

  if (typeof document === "undefined") return false

  const textarea = document.createElement("textarea")
  textarea.value = text
  textarea.setAttribute("readonly", "")
  textarea.style.position = "fixed"
  textarea.style.top = "0"
  textarea.style.left = "0"
  textarea.style.width = "1px"
  textarea.style.height = "1px"
  textarea.style.padding = "0"
  textarea.style.border = "none"
  textarea.style.outline = "none"
  textarea.style.boxShadow = "none"
  textarea.style.background = "transparent"
  textarea.style.opacity = "0"
  document.body.appendChild(textarea)

  const selection = document.getSelection()
  const previousRange = selection && selection.rangeCount > 0 ? selection.getRangeAt(0) : null

  try {
    textarea.focus()
    textarea.select()
    textarea.setSelectionRange(0, textarea.value.length)
    const ok = document.execCommand("copy")
    return ok
  } catch {
    return false
  } finally {
    document.body.removeChild(textarea)
    if (previousRange && selection) {
      selection.removeAllRanges()
      selection.addRange(previousRange)
    }
  }
}
