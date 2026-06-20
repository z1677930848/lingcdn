<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统授权</h1>
        <p class="subtitle">管理与查看系统授权信息</p>
      </div>
      <el-button type="warning" plain @click="openPortal">前往官方授权中心</el-button>
    </div>

    <ErrorState v-if="error" :message="error" @retry="loadData" />

    <el-card class="section-card">
      <div class="section-body">
        <div class="lc-section-title">授权码</div>
        <form class="license-form" @submit.prevent="handleUpdate">
          <el-input v-model="licenseKeyInput" placeholder="请输入授权码" :disabled="saving" />
          <el-button type="primary" native-type="submit" :loading="saving">{{ saving ? "更新中..." : "激活 / 更新" }}</el-button>
        </form>
      </div>
    </el-card>

    <el-card class="section-card">
      <div class="section-body">
        <div class="lc-section-title">授权信息</div>
        <div class="lc-info-grid">
          <div v-for="item in infoRows" :key="item.label" class="lc-info-item">
            <div class="lc-info-label">{{ item.label }}</div>
            <div class="lc-info-value">{{ item.value }}</div>
          </div>
          <div class="lc-info-item">
            <div class="lc-info-label">授权状态</div>
            <div class="lc-info-value">
              <el-tag :type="statusConfig.theme" effect="light">{{ statusConfig.text }}</el-tag>
            </div>
          </div>
        </div>
        <div class="lc-meta">
          当前在线节点：{{ status?.active_nodes ?? 0 }}，最近校验时间：{{ formatDate(status?.license?.last_checked) }}
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type SystemLicenseStatusResponse } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(true)
const error = ref("")
const saving = ref(false)
const licenseKeyInput = ref("")
const status = ref<SystemLicenseStatusResponse | null>(null)
const portalBase = computed(() => status.value?.portal_base || "https://auth.lingcdn.cloud")

const loadData = async () => {
  try {
    loading.value = true
    const res = await api.getSystemLicenseStatus()
    status.value = res
    licenseKeyInput.value = res.license?.license_key || ""
    error.value = ""
  } catch (err: any) {
    error.value = err.message || "加载系统授权信息失败"
  } finally {
    loading.value = false
  }
}

const handleUpdate = async () => {
  const key = licenseKeyInput.value.trim()
  if (!key) {
    MessagePlugin.warning("请输入授权码")
    return
  }
  saving.value = true
  try {
    await api.activateSystemLicense(key)
    MessagePlugin.success("授权更新成功")
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "授权更新失败")
  } finally {
    saving.value = false
  }
}

const licenseType = computed(() => {
  const lic = status.value?.license
  if (!lic) return "-"
  if (lic.license_key) return "在线授权"
  return "未授权"
})

const statusConfig = computed(() => {
  const st = String(status.value?.license?.status || "").toLowerCase()
  if (!st || st === "unlicensed") return { theme: "info" as const, text: "未授权" }
  if (st === "active") return { theme: "success" as const, text: "已激活" }
  if (st === "expired") return { theme: "danger" as const, text: "已过期" }
  if (st === "revoked") return { theme: "danger" as const, text: "已吊销" }
  if (st === "limited") return { theme: "warning" as const, text: "受限" }
  if (st === "paused" || st === "suspended") return { theme: "danger" as const, text: "已暂停（失效）" }
  return { theme: "info" as const, text: st }
})

const isZeroTime = (value?: string) => {
  if (!value) return true
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return true
  // Go zero time: 0001-01-01T00:00:00Z
  return date.getFullYear() <= 1
}

const formatDate = (value?: string) => {
  if (isZeroTime(value)) return "-"
  const date = new Date(value!)
  return date.toLocaleString("zh-CN")
}

const infoRows = computed(() => [
  { label: "System TOKEN", value: status.value?.control_id || "-" },
  { label: "许可证", value: status.value?.license?.license_key || "-" },
  { label: "授权 IP", value: status.value?.license_ip || "-" },
  { label: "到期时间", value: formatDate(status.value?.license?.expires_at) },
  {
    label: "最大节点数",
    value: (status.value?.license?.max_nodes || 0) > 0 ? String(status.value?.license?.max_nodes) : "不限",
  },
  { label: "授权类型", value: licenseType.value },
])

const openPortal = () => {
  window.open(portalBase.value, "_blank", "noopener,noreferrer")
}

onMounted(() => {
  loadData()
})
</script>

