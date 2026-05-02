<template>
  <svg
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    role="img"
    aria-label=""
  >
    <path
      :d="pathD"
      fill="none"
      :stroke="color"
      stroke-width="2"
      stroke-linejoin="round"
      stroke-linecap="round"
    />
  </svg>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  values: number[]
  color?: string
  height?: number
}>()

const width = 120
const height = computed(() => props.height ?? 28)
const color = computed(() => props.color ?? "currentColor")

const toPoints = (values: number[], w: number, h: number, padding = 4) => {
  const safe = values.length ? values : [0]
  const minV = Math.min(...safe)
  const maxV = Math.max(...safe)
  const range = maxV - minV || 1
  const innerW = Math.max(1, w - padding * 2)
  const innerH = Math.max(1, h - padding * 2)
  return safe.map((v, i) => {
    const x = padding + innerW * (safe.length === 1 ? 0.5 : i / (safe.length - 1))
    const y = padding + innerH * (1 - (v - minV) / range)
    return { x, y }
  })
}

const points = computed(() => toPoints(props.values || [], width, height.value, 4))

const pathD = computed(() => points.value.map((p, i) => `${i === 0 ? "M" : "L"}${p.x},${p.y}`).join(" "))
</script>

<script lang="ts">
export default { name: "Sparkline" }
</script>

