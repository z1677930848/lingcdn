<template>
  <header class="page-header">
    <div class="page-header-text">
      <component :is="headingTag" class="page-title">
        <slot name="title">{{ title }}</slot>
      </component>
      <p v-if="subtitle || $slots.subtitle" class="page-subtitle">
        <slot name="subtitle">{{ subtitle }}</slot>
      </p>
    </div>
    <div v-if="$slots.actions" class="page-header-actions">
      <slot name="actions" />
    </div>
  </header>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    title?: string
    subtitle?: string
    headingTag?: "h1" | "h2" | "h3"
  }>(),
  {
    headingTag: "h1",
  }
)
</script>

<script lang="ts">
export default { name: "PageHeader" }
</script>

<style scoped>
.page-header {
  display: flex;
  align-items: flex-end;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.page-header-text {
  min-width: 0;
}

.page-title {
  font-size: 20px;
  font-weight: 700;
  color: var(--app-text-strong);
  margin: 0;
  line-height: 1.3;
  letter-spacing: -0.01em;
}

.page-subtitle {
  font-size: 13px;
  color: var(--app-text-muted);
  margin: 4px 0 0;
  line-height: 1.4;
}

.page-header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

@media (max-width: 640px) {
  .page-header {
    align-items: flex-start;
  }

  .page-title {
    font-size: 18px;
  }
}
</style>
