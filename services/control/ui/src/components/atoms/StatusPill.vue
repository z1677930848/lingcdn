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

<style scoped>
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 2px 10px;
  border-radius: var(--app-pill-radius);
  font-size: 12px;
  font-weight: 500;
  line-height: 1.5;
  white-space: nowrap;
  vertical-align: middle;
}

.status-pill-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
  opacity: 0.85;
}

.status-pill--neutral {
  background: var(--app-neutral-soft-bg);
  color: var(--app-neutral-soft-fg);
}

.status-pill--success {
  background: var(--app-success-soft-bg);
  color: var(--app-success-soft-fg);
}

.status-pill--warning {
  background: var(--app-warning-soft-bg);
  color: var(--app-warning-soft-fg);
}

.status-pill--danger {
  background: var(--app-danger-soft-bg);
  color: var(--app-danger-soft-fg);
}

.status-pill--info {
  background: var(--app-info-soft-bg);
  color: var(--app-info-soft-fg);
}
</style>
