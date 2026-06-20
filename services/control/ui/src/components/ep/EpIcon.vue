<template>
  <el-icon v-if="iconComp" :size="normalizedSize" :class="iconClass">
    <component :is="iconComp" />
  </el-icon>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { resolveEpIcon } from "@/lib/ep-icons"

const props = defineProps<{
  name?: string
  size?: string | number
  class?: string
}>()

const iconComp = computed(() => resolveEpIcon(props.name))

const normalizedSize = computed(() => {
  if (props.size == null || props.size === "") return undefined
  if (typeof props.size === "number") return props.size
  const n = Number.parseInt(String(props.size).replace("px", ""), 10)
  return Number.isFinite(n) ? n : undefined
})

const iconClass = computed(() => props.class)
</script>
