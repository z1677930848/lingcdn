<template>
  <div class="app-loading" :class="{ 'is-fullscreen': fullscreen }" role="status" aria-live="polite">
    <div class="app-loading__spinner" aria-hidden="true" />
    <div class="app-loading__title">{{ title }}</div>
    <div v-if="subtitle" class="app-loading__subtitle">{{ subtitle }}</div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    subtitle?: string
    fullscreen?: boolean
  }>(),
  {
    title: "加载中",
    subtitle: "",
    fullscreen: false,
  },
)
</script>

