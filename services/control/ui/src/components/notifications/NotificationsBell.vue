<template>
  <t-popup placement="bottom-right" show-arrow>
    <template #content>
      <div class="panel">
        <div class="panel-title">站内通知</div>
        <EmptyState compact title="暂无通知" />
      </div>
    </template>
    <t-badge :count="0" shape="circle" :offset="[-2, 6]" :max-count="99">
      <t-button variant="text" aria-label="站内通知">通知</t-button>
    </t-badge>
  </t-popup>
</template>

<script setup lang="ts">
import EmptyState from "@/components/atoms/EmptyState.vue"
</script>

<style scoped>
.panel {
  width: 260px;
  padding: 12px;
  color: var(--app-text);
}

.panel-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 8px;
  color: var(--app-text-strong);
}

:deep(.t-button) {
  min-height: 44px;
}
</style>
