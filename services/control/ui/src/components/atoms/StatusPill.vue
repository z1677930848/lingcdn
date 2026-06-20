<template>
  <span class="status-pill" :class="`status-pill--${tone}`">
    <span v-if="dot" class="status-pill-dot" aria-hidden="true" />
    <slot>{{ label }}</slot>
  </span>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    tone?: "neutral" | "success" | "warning" | "danger" | "info"
    label?: string
    dot?: boolean
  }>(),
  {
    tone: "neutral",
    dot: false,
  }
)
</script>

<script lang="ts">
export default { name: "StatusPill" }
</script>

