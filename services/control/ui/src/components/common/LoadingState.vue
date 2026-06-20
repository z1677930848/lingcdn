<template>
  <div v-loading="true" class="loading-state" role="status" aria-live="polite">
    <span class="loading-state__text">{{ text }}</span>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    size?: "small" | "medium" | "large"
    text?: string
  }>(),
  {
    size: "medium",
    text: "加载中...",
  }
)
</script>

<style scoped>
.loading-state {
  min-height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.loading-state__text {
  margin-top: 40px;
  color: var(--app-text-muted);
  font-size: 14px;
}
</style>
