/**
 * 解析登录后的安全跳转目标：仅接受同站绝对路径，
 * 并排除会把已登录用户又打回登录/注册页的路径。
 */
export function resolveRedirect(
  fallback: string,
  raw: string | string[] | null | undefined,
  forbidden: string[] = ["/login", "/nimda", "/register", "/forgot-password"],
): string {
  const target = Array.isArray(raw) ? raw[0] : raw
  if (typeof target !== "string" || target === "") return fallback
  if (!target.startsWith("/") || target.startsWith("//")) return fallback
  if (forbidden.some((p) => target === p || target.startsWith(p + "/") || target.startsWith(p + "?") || target.startsWith(p + "#"))) {
    return fallback
  }
  return target
}
