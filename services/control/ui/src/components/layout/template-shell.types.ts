export type TemplateNavItem = {
  name: string
  href: string
  /** Optional permission key; item hidden when user lacks permission */
  perm?: string
}

/** Top-level sidebar entry — never rendered as an expandable group. */
export type TemplateStandaloneNavItem = {
  name: string
  href: string
  icon?: string
  perm?: string
}

export type TemplateModule = {
  id: string
  name: string
  /** TDesign icon name for the module group */
  icon?: string
  defaultHref: string
  items: TemplateNavItem[]
}

export type TemplateShellUser = {
  username: string
  email?: string
}

export type TemplateShellBrand = {
  title: string
  logo?: string
  /** Sidebar top-left: show system name or logo only */
  display?: "name" | "logo"
}

