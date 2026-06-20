export type EpTagType = "success" | "warning" | "info" | "primary" | "danger"

/** Map TDesign tag theme names to Element Plus tag types */
export function epTagType(theme?: string | null): EpTagType {
  if (!theme || theme === "default") return "info"
  if (theme === "error") return "danger"
  if (theme === "success" || theme === "warning" || theme === "info" || theme === "primary" || theme === "danger") {
    return theme
  }
  return "info"
}
