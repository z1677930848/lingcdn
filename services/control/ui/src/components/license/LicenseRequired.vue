<template>
  <div class="wrap">
    <t-card bordered class="card">
      <div class="title">{{ title }}</div>
      <div class="desc" role="alert">{{ message }}</div>
      <div class="actions">
        <t-button v-if="canManage" theme="primary" @click="onGoLicense">前往系统授权</t-button>
        <t-button v-if="onRefresh" variant="outline" @click="onRefresh">刷新状态</t-button>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed } from "vue"

const props = defineProps<{
  reason?: string
  status?: string
  canManage: boolean
  onGoLicense?: () => void
  onRefresh?: () => void
}>()

const statusCode = computed(() => String(props.status || "").trim().toLowerCase())
const isInvalidated = computed(() => ["paused", "suspended"].includes(statusCode.value))
const title = computed(() => (isInvalidated.value ? "系统授权已失效" : "系统未授权"))
const defaultMessage = computed(() => (isInvalidated.value ? "系统授权已失效，请重新授权" : "系统未授权"))
const message = computed(() => props.reason?.trim() || defaultMessage.value)
</script>

<style scoped>
.wrap {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 60vh;
  padding: 32px 16px;
}

.card {
  width: min(560px, 100%);
  text-align: center;
  padding: 48px 32px;
}

.title {
  font-size: 22px;
  font-weight: 600;
  margin-bottom: 12px;
}

.desc {
  font-size: 15px;
  color: #8a8f8d;
}

.actions {
  margin-top: 24px;
  display: flex;
  gap: 12px;
  justify-content: center;
}
</style>
