// Shared timestamp helpers.
//
// Historically each view inline-called `new Date(x).toLocaleString('zh-CN')`
// with slightly different options, which meant the same timestamp rendered
// differently across pages and never showed a timezone. Centralizing here
// gives us a single place to tune formatting (e.g. add a relative-time
// render, tweak locale) and a single place to grep when debugging a
// "wrong time" bug.
//
// All helpers accept a `string | number | Date | undefined` so they can be
// dropped into Vue templates without callers pre-stringifying.

type TimeInput = string | number | Date | null | undefined

const parse = (v: TimeInput): Date | null => {
  if (v == null || v === "") return null
  const d = v instanceof Date ? v : new Date(v)
  if (Number.isNaN(d.getTime())) return null
  return d
}

// "2026/04/18 20:34:34" — short form for table cells.
// Returns "-" for missing values so templates don't need v-if branches.
export const formatTime = (v: TimeInput): string => {
  const d = parse(v)
  if (!d) return "-"
  return d.toLocaleString("zh-CN", { hour12: false })
}

// Only the date portion — for columns where time-of-day is noise
// (e.g. an order's 创建日期).
export const formatDate = (v: TimeInput): string => {
  const d = parse(v)
  if (!d) return "-"
  return d.toLocaleDateString("zh-CN")
}

// Full precision + UTC offset. Surfaced in tooltips so operators working
// across regions can disambiguate a timestamp without guessing whether
// it's UTC or Asia/Shanghai.
export const formatTimeFull = (v: TimeInput): string => {
  const d = parse(v)
  if (!d) return "-"
  const local = d.toLocaleString("zh-CN", { hour12: false })
  const offsetMin = -d.getTimezoneOffset() // negate to match "UTC+08:00" convention
  const sign = offsetMin >= 0 ? "+" : "-"
  const abs = Math.abs(offsetMin)
  const hh = String(Math.floor(abs / 60)).padStart(2, "0")
  const mm = String(abs % 60).padStart(2, "0")
  return `${local} (UTC${sign}${hh}:${mm})`
}

// Relative time like "3 分钟前" / "2 天前". Bounded at 30 days beyond
// which we fall back to the absolute date, because "67 天前" communicates
// less than "2026/2/10".
export const formatRelative = (v: TimeInput, now: Date = new Date()): string => {
  const d = parse(v)
  if (!d) return "-"
  const diffMs = now.getTime() - d.getTime()
  const absMs = Math.abs(diffMs)
  const suffix = diffMs >= 0 ? "前" : "后"
  const SEC = 1000, MIN = 60 * SEC, HOUR = 60 * MIN, DAY = 24 * HOUR
  if (absMs < 45 * SEC) return "刚刚"
  if (absMs < 60 * MIN) return `${Math.round(absMs / MIN)} 分钟${suffix}`
  if (absMs < 24 * HOUR) return `${Math.round(absMs / HOUR)} 小时${suffix}`
  if (absMs < 30 * DAY) return `${Math.round(absMs / DAY)} 天${suffix}`
  return formatDate(v)
}
