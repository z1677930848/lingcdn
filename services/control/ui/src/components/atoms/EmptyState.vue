<template>
  <div class="empty-state" :class="{ compact }">
    <div class="empty-icon" aria-hidden="true">
      <svg viewBox="0 0 64 64" fill="none" stroke="currentColor" stroke-width="1.4" stroke-linecap="round" stroke-linejoin="round">
        <rect x="10" y="14" width="44" height="36" rx="4" />
        <path d="M10 24 L54 24" />
        <circle cx="16" cy="19" r="1" />
        <circle cx="20" cy="19" r="1" />
        <circle cx="24" cy="19" r="1" />
        <path d="M22 36 L42 36" opacity="0.45" />
        <path d="M22 42 L36 42" opacity="0.3" />
      </svg>
    </div>
    <div class="empty-title">{{ title }}</div>
    <div v-if="description" class="empty-description">{{ description }}</div>
    <div v-if="$slots.actions" class="empty-actions">
      <slot name="actions" />
    </div>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    description?: string
    compact?: boolean
  }>(),
  {
    title: "暂无数据",
    compact: false,
  }
)
</script>

<script lang="ts">
export default { name: "EmptyState" }
</script>

