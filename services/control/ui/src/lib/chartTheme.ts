/*
 * Theme-aware color palette for ECharts charts.
 *
 * ECharts is canvas-based — it cannot read CSS variables directly. This
 * helper returns the resolved palette for the current `theme-mode` so the
 * chart looks coherent in both light and dark mode. Pair with `useTheme()`
 * and a watcher that re-renders the chart when the mode flips.
 */

export interface ChartPalette {
  isDark: boolean
  textPrimary: string
  textSecondary: string
  textFaint: string
  border: string
  divider: string
  surface: string
  surfaceMuted: string
  brand: string
  brandStrong: string
  brandSoft: string
  brandRing: string
  mapArea: string
  mapAreaHover: string
  mapBorder: string
  mapGradient: [string, string, string, string, string]
  shadow: string
}

const lightPalette: ChartPalette = {
  isDark: false,
  textPrimary: "#191919",
  textSecondary: "#5A5A5A",
  textFaint: "#9B9B9B",
  border: "#EBEBEC",
  divider: "#F0F0F1",
  surface: "#FFFFFF",
  surfaceMuted: "#F7F7F8",
  brand: "#635BFF",
  brandStrong: "#4D44E0",
  brandSoft: "rgba(99, 91, 255, 0.18)",
  brandRing: "rgba(99, 91, 255, 0.25)",
  mapArea: "#F0F0F1",
  mapAreaHover: "#ECE9FF",
  mapBorder: "#DDDDDE",
  mapGradient: ["#F0F0F1", "#D5CFFF", "#9385FE", "#635BFF", "#4D44E0"],
  shadow: "rgba(99, 91, 255, 0.25)",
}

const darkPalette: ChartPalette = {
  isDark: true,
  textPrimary: "#FAFAFA",
  textSecondary: "#A8A8A8",
  textFaint: "#6E6E6E",
  border: "#2A2A2A",
  divider: "#232323",
  surface: "#1A1A1A",
  surfaceMuted: "#141414",
  brand: "#9385FE",
  brandStrong: "#B8AFFE",
  brandSoft: "rgba(147, 133, 254, 0.22)",
  brandRing: "rgba(147, 133, 254, 0.32)",
  mapArea: "#1A1A1A",
  mapAreaHover: "#2D2899",
  mapBorder: "#2A2A2A",
  mapGradient: ["#1A1A1A", "#2D2899", "#4D44E0", "#635BFF", "#9385FE"],
  shadow: "rgba(147, 133, 254, 0.35)",
}

export function getChartPalette(isDark: boolean): ChartPalette {
  return isDark ? darkPalette : lightPalette
}
