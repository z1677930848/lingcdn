<template>
  <div class="donut-wrap" :style="{ width: `${size}px`, height: `${size}px` }">
    <svg :width="size" :height="size" :viewBox="`0 0 ${size} ${size}`" role="img" aria-label="">
      <circle :cx="size / 2" :cy="size / 2" :r="radius" :stroke="trackColor" :stroke-width="strokeWidth" fill="none" />
      <circle
        :cx="size / 2"
        :cy="size / 2"
        :r="radius"
        :stroke="color"
        :stroke-width="strokeWidth"
        fill="none"
        stroke-linecap="round"
        :stroke-dasharray="dashArray"
        :transform="`rotate(-90 ${size / 2} ${size / 2})`"
      />
    </svg>
    <div class="donut-center">
      <div class="donut-percent">{{ percentText }}</div>
      <div class="donut-label">{{ label }}</div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  percent: number
  size?: number
  strokeWidth?: number
  trackColor?: string
  color?: string
  label?: string
}>()

const size = computed(() => props.size ?? 160)
const strokeWidth = computed(() => props.strokeWidth ?? 16)
const trackColor = computed(() => props.trackColor ?? "var(--app-divider)")
const color = computed(() => props.color ?? "var(--td-brand-color)")
const label = computed(() => props.label ?? "使用占比")

const clamped = computed(() => Math.max(0, Math.min(1, Number(props.percent || 0))))
const radius = computed(() => (size.value - strokeWidth.value) / 2)
const circumference = computed(() => 2 * Math.PI * radius.value)

const dashArray = computed(() => {
  const dash = circumference.value * clamped.value
  const gap = circumference.value - dash
  return `${dash} ${gap}`
})

const percentText = computed(() => `${(clamped.value * 100).toFixed(2)}%`)
</script>

<script lang="ts">
export default { name: "DonutChart" }
</script>

