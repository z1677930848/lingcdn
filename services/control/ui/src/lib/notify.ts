import { MessagePlugin } from "tdesign-vue-next"

export type NotifyVariant = "default" | "success" | "warning" | "destructive" | "info"

export interface NotifyOptions {
  variant?: NotifyVariant
  title: string
  description?: string
}

export function notify({ variant = "default", title, description }: NotifyOptions) {
  const content = description ? `${title}：${description}` : title

  if (variant === "success") return MessagePlugin.success(content)
  if (variant === "warning") return MessagePlugin.warning(content)
  if (variant === "destructive") return MessagePlugin.error(content)
  if (variant === "info") return MessagePlugin.info(content)
  return MessagePlugin.info(content)
}
