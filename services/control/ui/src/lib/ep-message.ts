import { ElMessage } from "element-plus"

type MessageOptions = string | { content: string; duration?: number }

function normalizeMessage(input: MessageOptions): string {
  if (typeof input === "string") return input
  return input.content
}

function show(type: "success" | "warning" | "error" | "info", input: MessageOptions) {
  const message = normalizeMessage(input)
  const duration = typeof input === "object" ? input.duration : undefined
  ElMessage({ type, message, duration })
}

/** Drop-in replacement for TDesign MessagePlugin */
export const MessagePlugin = {
  success: (input: MessageOptions) => show("success", input),
  warning: (input: MessageOptions) => show("warning", input),
  error: (input: MessageOptions) => show("error", input),
  info: (input: MessageOptions) => show("info", input),
}
