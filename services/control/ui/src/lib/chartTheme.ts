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
  textPrimary: "#303133",
  textSecondary: "#606266",
  textFaint: "#A8ABB2",
  border: "#EBEEF5",
  divider: "#F2F6FC",
  surface: "#FFFFFF",
  surfaceMuted: "#F5F7FA",
  brand: "#409EFF",
  brandStrong: "#337ECC",
  brandSoft: "rgba(64, 158, 255, 0.18)",
  brandRing: "rgba(64, 158, 255, 0.25)",
  mapArea: "#F5F7FA",
  mapAreaHover: "#ECF5FF",
  mapBorder: "#E4E7ED",
  mapGradient: ["#F5F7FA", "#D9ECFF", "#A0CFFF", "#409EFF", "#337ECC"],
  shadow: "rgba(64, 158, 255, 0.25)",
}

const darkPalette: ChartPalette = {
  isDark: true,
  textPrimary: "#E5EAF3",
  textSecondary: "#A3A6AD",
  textFaint: "#6C6E72",
  border: "#303030",
  divider: "#262626",
  surface: "#1D1D1D",
  surfaceMuted: "#141414",
  brand: "#79BBFF",
  brandStrong: "#A0CFFF",
  brandSoft: "rgba(64, 158, 255, 0.22)",
  brandRing: "rgba(64, 158, 255, 0.32)",
  mapArea: "#1D1D1D",
  mapAreaHover: "#2A6DB5",
  mapBorder: "#303030",
  mapGradient: ["#141414", "#1D4E89", "#337ECC", "#409EFF", "#79BBFF"],
  shadow: "rgba(64, 158, 255, 0.35)",
}

export function getChartPalette(isDark: boolean): ChartPalette {
  return isDark ? darkPalette : lightPalette
}
