<template>
  <div class="license-gate">
    <el-card class="license-gate__card">
      <div class="license-gate__title">{{ title }}</div>
      <div class="license-gate__desc" role="alert">{{ message }}</div>
      <div class="actions">
        <el-button v-if="canManage" type="primary" @click="onGoLicense">前往系统授权</el-button>
        <el-button v-if="onRefresh" plain @click="onRefresh">刷新状态</el-button>
      </div>
    </el-card>
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
