import { computed, ref, watch } from "vue"

export type ThemeMode = "light" | "dark"

const STORAGE_KEY = "lingcdn:theme-mode"
const HTML_ATTR = "theme-mode"

function readInitial(): ThemeMode {
  if (typeof window === "undefined") return "light"
  try {
    const stored = window.localStorage.getItem(STORAGE_KEY)
    if (stored === "light" || stored === "dark") return stored
  } catch {
    // localStorage unavailable (private mode etc.) — fall through
  }
  if (typeof window.matchMedia === "function") {
    const prefersDark = window.matchMedia("(prefers-color-scheme: dark)").matches
    return prefersDark ? "dark" : "light"
  }
  return "light"
}

const mode = ref<ThemeMode>(readInitial())

function apply(value: ThemeMode) {
  if (typeof document === "undefined") return
  if (value === "dark") {
    document.documentElement.setAttribute(HTML_ATTR, "dark")
    document.body.setAttribute(HTML_ATTR, "dark")
  } else {
    document.documentElement.removeAttribute(HTML_ATTR)
    document.body.removeAttribute(HTML_ATTR)
  }
}

apply(mode.value)

watch(mode, (next) => {
  apply(next)
  try {
    window.localStorage.setItem(STORAGE_KEY, next)
  } catch {
    // ignore — preference is non-essential
  }
})

if (typeof window !== "undefined" && typeof window.matchMedia === "function") {
  const media = window.matchMedia("(prefers-color-scheme: dark)")
  const onSystemChange = (event: MediaQueryListEvent) => {
    let stored: string | null = null
    try {
      stored = window.localStorage.getItem(STORAGE_KEY)
    } catch {
      // ignore
    }
    if (stored !== "light" && stored !== "dark") {
      mode.value = event.matches ? "dark" : "light"
    }
  }
  if (typeof media.addEventListener === "function") {
    media.addEventListener("change", onSystemChange)
  } else if (typeof (media as MediaQueryList & { addListener?: (cb: (e: MediaQueryListEvent) => void) => void }).addListener === "function") {
    (media as MediaQueryList & { addListener: (cb: (e: MediaQueryListEvent) => void) => void }).addListener(onSystemChange)
  }
}

export function useTheme() {
  const isDark = computed(() => mode.value === "dark")

  const setMode = (next: ThemeMode) => {
    mode.value = next
  }

  const toggle = () => {
    mode.value = mode.value === "dark" ? "light" : "dark"
  }

  return {
    mode,
    isDark,
    setMode,
    toggle,
  }
}
