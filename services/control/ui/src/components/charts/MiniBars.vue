<template>
  <svg
    :width="width"
    :height="height"
    :viewBox="`0 0 ${width} ${height}`"
    role="img"
    aria-label=""
  >
    <rect
      v-for="(bar, idx) in bars"
      :key="idx"
      :x="bar.x"
      :y="bar.y"
      :width="bar.w"
      :height="bar.h"
      :fill="color"
      :opacity="0.7"
      rx="1.5"
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

const bars = computed(() => {
  const padding = 4
  const innerW = width - padding * 2
  const innerH = height.value - padding * 2
  const safe = props.values.length ? props.values : [0]
  const maxV = Math.max(...safe) || 1
  const barW = innerW / safe.length
  return safe.map((v, i) => {
    const h = (innerH * v) / maxV
    const x = padding + i * barW + barW * 0.2
    const y = padding + (innerH - h)
    return { x, y, w: barW * 0.6, h }
  })
})
</script>

<script lang="ts">
export default { name: "MiniBars" }
</script>

