<template>
  <div class="chart">
    <svg
      width="100%"
      :height="height"
      :viewBox="`0 0 ${width} ${height}`"
      preserveAspectRatio="none"
      role="img"
      aria-label=""
    >
      <line
        v-for="(g, idx) in grid"
        :key="idx"
        :x1="padding"
        :y1="gridY(idx)"
        :x2="width - padding"
        :y2="gridY(idx)"
        :stroke="gridColor"
        stroke-width="1"
      />
      <path :d="areaD" :fill="fill" />
      <path :d="lineD" fill="none" :stroke="stroke" stroke-width="2.5" stroke-linejoin="round" stroke-linecap="round" />
      <circle
        v-for="(p, idx) in points"
        :key="idx"
        :cx="p.x"
        :cy="p.y"
        :r="idx === points.length - 1 ? 4.5 : 3"
        :fill="stroke"
        :opacity="idx === points.length - 1 ? 1 : 0.75"
      />
      <circle
        v-if="points.length"
        :cx="points[points.length - 1].x"
        :cy="points[points.length - 1].y"
        r="10"
        :fill="haloColor"
      />
    </svg>

    <div class="labels">
      <span
        v-for="(label, idx) in labels"
        :key="idx"
        :class="{ start: idx === 0, end: idx === labels.length - 1 }"
      >
        {{ label }}
      </span>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  values: number[]
  labels: string[]
  height?: number
  stroke?: string
  fill?: string
  gridColor?: string
  haloColor?: string
}>()

const width = 860
const padding = 36
const height = computed(() => props.height ?? 260)
const stroke = computed(() => props.stroke ?? "var(--td-brand-color)")
const fill = computed(() => props.fill ?? "var(--app-brand-soft-bg)")
const gridColor = computed(() => props.gridColor ?? "var(--app-divider)")
const haloColor = computed(() => props.haloColor ?? "var(--app-brand-ring)")

const points = computed(() => {
  const safe = props.values.length ? props.values : [0]
  const minV = Math.min(...safe)
  const maxV = Math.max(...safe)
  const range = maxV - minV || 1
  const innerW = Math.max(1, width - padding * 2)
  const innerH = Math.max(1, height.value - padding * 2)
  return safe.map((v, i) => {
    const x = padding + innerW * (safe.length === 1 ? 0.5 : i / (safe.length - 1))
    const y = padding + innerH * (1 - (v - minV) / range)
    return { x, y }
  })
})

const areaD = computed(() => {
  if (!points.value.length) return ""
  const maxY = height.value - padding
  const d = points.value.map((p, i) => `${i === 0 ? "M" : "L"}${p.x},${p.y}`).join(" ")
  return `${d} L${points.value[points.value.length - 1].x},${maxY} L${points.value[0].x},${maxY} Z`
})

const lineD = computed(() => points.value.map((p, i) => `${i === 0 ? "M" : "L"}${p.x},${p.y}`).join(" "))

const grid = computed(() => Array.from({ length: 5 }, (_, i) => i))
const gridY = (idx: number) => {
  const minY = padding
  const maxY = height.value - padding
  return minY + ((maxY - minY) * idx) / (grid.value.length - 1)
}
</script>

<script lang="ts">
export default { name: "LineAreaChart" }
</script>

<style scoped>
.chart {
  width: 100%;
}

.labels {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-top: 10px;
  color: var(--app-text-faint);
  font-size: 12px;
}

.labels span {
  flex: 1;
  text-align: center;
}

.labels span.start {
  text-align: left;
}

.labels span.end {
  text-align: right;
}
</style>
