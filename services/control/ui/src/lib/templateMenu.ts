import type { TemplateModule, TemplateStandaloneNavItem } from "@/components/layout/template-shell.types"

/** Dashboard roots must not prefix-match every child route. */
const EXACT_ONLY_HREFS = new Set(["/admin/dashboard", "/dashboard"])

/** Path matches menu href exactly or as a nested detail route. */
export function pathMatchesMenuHref(path: string, href: string): boolean {
  const normalizedPath = path.split("?")[0].split("#")[0]
  const normalizedHref = href.split("?")[0].split("#")[0]
  if (normalizedPath === normalizedHref) return true
  if (EXACT_ONLY_HREFS.has(normalizedHref)) return false
  return normalizedPath.startsWith(`${normalizedHref}/`)
}

function allMenuHrefs(modules: TemplateModule[], standalone: TemplateStandaloneNavItem[] = []): string[] {
  return [
    ...standalone.map((item) => item.href),
    ...modules.flatMap((mod) => mod.items.map((item) => item.href)),
  ]
}

export function resolveStandaloneFromPath(
  path: string,
  standalone: TemplateStandaloneNavItem[] = []
): TemplateStandaloneNavItem | undefined {
  let best: { item: TemplateStandaloneNavItem; hrefLen: number } | undefined
  for (const item of standalone) {
    if (pathMatchesMenuHref(path, item.href)) {
      const hrefLen = item.href.length
      if (!best || hrefLen > best.hrefLen) {
        best = { item, hrefLen }
      }
    }
  }
  return best?.item
}

/** Longest-prefix match wins (detail pages inherit parent menu highlight). */
export function resolveMenuActivePath(
  path: string,
  modules: TemplateModule[],
  standalone: TemplateStandaloneNavItem[] = []
): string {
  const hrefs = allMenuHrefs(modules, standalone).sort((a, b) => b.length - a.length)
  const match = hrefs.find((href) => pathMatchesMenuHref(path, href))
  if (match) return match

  const mod = resolveModuleFromPath(path, modules, standalone)
  return mod?.defaultHref || standalone[0]?.href || modules[0]?.items[0]?.href || "/"
}

/** Which grouped module owns the current path (standalone entries return undefined). */
export function resolveModuleFromPath(
  path: string,
  modules: TemplateModule[],
  standalone: TemplateStandaloneNavItem[] = []
): TemplateModule | undefined {
  if (resolveStandaloneFromPath(path, standalone)) return undefined

  let best: { module: TemplateModule; hrefLen: number } | undefined

  for (const mod of modules) {
    for (const item of mod.items) {
      if (pathMatchesMenuHref(path, item.href)) {
        const hrefLen = item.href.length
        if (!best || hrefLen > best.hrefLen) {
          best = { module: mod, hrefLen }
        }
      }
    }
    if (pathMatchesMenuHref(path, mod.defaultHref)) {
      const hrefLen = mod.defaultHref.length
      if (!best || hrefLen > best.hrefLen) {
        best = { module: mod, hrefLen }
      }
    }
  }

  return best?.module
}

export function resolveModuleIdFromPath(
  path: string,
  modules: TemplateModule[],
  standalone: TemplateStandaloneNavItem[] = []
): string {
  if (resolveStandaloneFromPath(path, standalone)) return ""
  return resolveModuleFromPath(path, modules, standalone)?.id || modules[0]?.id || ""
}

/** One row when label matches the only child; otherwise use a submenu (e.g. L4 管理 → 转发列表). */
export function isFlatModule(group: TemplateModule): boolean {
  return group.items.length === 1 && group.items[0].name === group.name
}

/**
 * Secondary items under a primary group.
 * Drop a child whose label duplicates the group title when other siblings exist
 * (e.g. 节点管理 / 节点管理 → keep only the submenu title + other pages).
 */
export function getSubmenuItems(group: TemplateModule) {
  if (group.items.length <= 1) return group.items
  const deduped = group.items.filter((item) => item.name !== group.name)
  return deduped.length > 0 ? deduped : group.items
}

export function buildTitleMap(
  modules: TemplateModule[],
  standalone: TemplateStandaloneNavItem[] = []
): Map<string, string> {
  const map = new Map<string, string>()
  standalone.forEach((item) => map.set(item.href, item.name))
  modules.forEach((mod) => mod.items.forEach((item) => map.set(item.href, item.name)))
  return map
}
