<template>
  <div class="placeholder">
    <t-card class="card" bordered>
      <template #title>{{ title }}</template>
      <p>该页面正在迁移到 Vue 版本，请稍后继续。</p>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"
import { useRoute } from "vue-router"

const route = useRoute()
const title = computed(() => {
  const raw = route.meta.title
  return typeof raw === "string" && raw.trim().length > 0 ? raw : "页面"
})
</script>

<style scoped>
.placeholder {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 48px 16px;
  color: #1f2937;
}

.card {
  width: min(640px, 100%);
}

.card p {
  margin: 0;
  color: #475569;
}
</style>


