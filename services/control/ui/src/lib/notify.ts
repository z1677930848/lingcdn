import { ElMessage } from "element-plus"

export type NotifyVariant = "default" | "success" | "warning" | "destructive" | "info"

export interface NotifyOptions {
  variant?: NotifyVariant
  title: string
  description?: string
}

export function notify({ variant = "default", title, description }: NotifyOptions) {
  const content = description ? `${title}：${description}` : title
  if (variant === "success") return ElMessage.success(content)
  if (variant === "warning") return ElMessage.warning(content)
  if (variant === "destructive") return ElMessage.error(content)
  return ElMessage.info(content)
}
