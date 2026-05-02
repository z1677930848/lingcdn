import { computed, onMounted, ref, watchEffect } from "vue"
import { api } from "@/lib/api"

export type FooterLink = {
  label: string
  href: string
}

export type SystemSettings = {
  system_name: string
  footer_links: string
  footer_copyright: string
  favicon: string
  logo: string
  register_enabled: boolean
  register_email_verification: boolean
}

const DEFAULT_SETTINGS: SystemSettings = {
  system_name: "LingCDN",
  footer_links: "",
  footer_copyright: `(c) ${new Date().getFullYear()} LingCDN. All rights reserved.`,
  favicon: "",
  logo: "",
  register_enabled: true,
  register_email_verification: false,
}

let cachedSettings: SystemSettings | null = null
let pendingSettings: Promise<SystemSettings> | null = null

const SYSTEM_NAME_STORAGE_KEY = "lingcdn:system-name"

export function getCachedSystemName(): string {
  if (cachedSettings?.system_name) return cachedSettings.system_name
  if (typeof localStorage !== "undefined") {
    const stored = localStorage.getItem(SYSTEM_NAME_STORAGE_KEY)
    if (stored) return stored
  }
  return DEFAULT_SETTINGS.system_name
}

function persistSystemName(name: string) {
  if (typeof localStorage === "undefined") return
  try {
    if (name) localStorage.setItem(SYSTEM_NAME_STORAGE_KEY, name)
  } catch {
    /* ignore quota errors */
  }
}

function normalizeSettings(input: any): SystemSettings {
  const source = input || {}
  return {
    system_name: String(source.system_name || DEFAULT_SETTINGS.system_name),
    footer_links: String(source.footer_links || ""),
    footer_copyright: String(source.footer_copyright || DEFAULT_SETTINGS.footer_copyright),
    favicon: String(source.favicon || ""),
    logo: String(source.logo || ""),
    register_enabled: Boolean(source.register_enabled ?? DEFAULT_SETTINGS.register_enabled),
    register_email_verification: Boolean(
      source.register_email_verification ?? DEFAULT_SETTINGS.register_email_verification
    ),
  }
}

async function loadSettings(): Promise<SystemSettings> {
  if (cachedSettings) return cachedSettings
  if (!pendingSettings) {
    pendingSettings = api
      .getPublicSettings()
      .then(({ settings }) => {
        cachedSettings = normalizeSettings(settings)
        persistSystemName(cachedSettings.system_name)
        return cachedSettings
      })
      .catch(() => {
        cachedSettings = DEFAULT_SETTINGS
        return cachedSettings
      })
  }
  return pendingSettings
}

export function resolveFooterCopyright(template: string, name: string): string {
  const year = String(new Date().getFullYear())
  const safe = String(template || "").trim()
  if (!safe) {
    return `(c) ${year} ${name}. All rights reserved.`
  }
  return safe.replaceAll("{year}", year).replaceAll("{name}", name)
}

export function parseFooterLinks(value: string): FooterLink[] {
  if (!value) return []
  return value
    .split(/\r?\n/)
    .map((line) => line.trim())
    .filter(Boolean)
    .map((line) => {
      const [label, href] = line.split("|").map((part) => part.trim())
      if (!label || !href) return null
      return { label, href }
    })
    .filter((item): item is FooterLink => Boolean(item))
}

function applyFavicon(href: string) {
  if (!href || typeof document === "undefined") return
  const existing = document.querySelector<HTMLLinkElement>("link[rel='icon']")
  if (existing) {
    existing.href = href
    return
  }
  const link = document.createElement("link")
  link.rel = "icon"
  link.href = href
  document.head.appendChild(link)
}

export function useSystemSettings() {
  const settings = ref<SystemSettings>(cachedSettings ?? DEFAULT_SETTINGS)
  const loading = ref(!cachedSettings)

  onMounted(() => {
    let active = true
    loadSettings().then((next) => {
      if (!active) return
      settings.value = next
      loading.value = false
    })
    return () => {
      active = false
    }
  })

  const footerLinks = computed(() => parseFooterLinks(settings.value.footer_links))

  const brand = computed(() => {
    const title = settings.value.system_name || DEFAULT_SETTINGS.system_name
    return { title, logo: settings.value.logo }
  })

  const footerCopyright = computed(() =>
    resolveFooterCopyright(settings.value.footer_copyright, brand.value.title)
  )

  watchEffect(() => {
    applyFavicon(settings.value.favicon)
  })

  return { settings, brand, footerLinks, footerCopyright, loading }
}


