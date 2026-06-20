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

