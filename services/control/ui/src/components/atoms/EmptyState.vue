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

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 10px;
  padding: 40px 16px;
  text-align: center;
  color: var(--app-text-muted);
}

.empty-state.compact {
  padding: 20px 12px;
  gap: 6px;
}

.empty-icon {
  width: 64px;
  height: 64px;
  color: var(--app-text-faint);
  opacity: 0.7;
}

.empty-state.compact .empty-icon {
  width: 44px;
  height: 44px;
}

.empty-icon svg {
  width: 100%;
  height: 100%;
}

.empty-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text);
}

.empty-state.compact .empty-title {
  font-size: 13px;
}

.empty-description {
  font-size: 12px;
  color: var(--app-text-faint);
  max-width: 280px;
  line-height: 1.5;
}

.empty-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
}
</style>
