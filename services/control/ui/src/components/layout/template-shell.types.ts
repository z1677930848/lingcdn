export type TemplateNavItem = {
  name: string
  href: string
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
}

