<template>
  <div class="settings-tab">
    <ErrorState v-if="error" :message="error" @retry="loadSettings" />
    <LoadingState v-else-if="loading" />
    <template v-else>
    <p class="tab-desc">设置各类数据的保留天数，超过保留期的数据将被自动清理。设置为 <code>0</code> 表示不自动清理。</p>

    <el-form label-position="top" :disabled="loading || saving">
      <el-form-item v-for="item in retentionItems" :key="item.key" :label="item.label">
        <div class="input-with-unit">
          <el-input
            :value="retentionText(item.key)"
            @change="(v: string) => setRetention(item.key, v)"
            type="number"
            placeholder="0-9999"
          />
          <span class="unit">天</span>
        </div>
        <div class="hint">{{ item.hint }}</div>
      </el-form-item>

      <div class="actions">
        <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
      </div>
    </el-form>
    </template>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

type RetentionSettings = {
  retention_system_logs: number
  retention_es_logs: number
  retention_waf_bans: number
  retention_upgrade_logs: number
}

const settings = ref<RetentionSettings>({
  retention_system_logs: 90,
  retention_es_logs: 7,
  retention_waf_bans: 7,
  retention_upgrade_logs: 30,
})

const retentionText = (key: keyof RetentionSettings): string => {
  const v = settings.value[key]
  return v == null ? "" : String(v)
}

const setRetention = (key: keyof RetentionSettings, v: string) => {
  const n = Number(String(v || "").trim())
  if (!Number.isFinite(n)) {
    settings.value[key] = 0
    return
  }
  settings.value[key] = Math.max(0, Math.min(9999, Math.round(n)))
}

const retentionItems = [
  {
    key: "retention_system_logs",
    label: "系统操作日志",
    hint: "管理员操作、登录记录等系统审计日志",
  },
  {
    key: "retention_es_logs",
    label: "网站访问日志 (ES)",
    hint: "存储在 Elasticsearch 中的 HTTP 访问日志索引。节点 ERROR/WARN 日志同样存于 ES (cdn-error-*)，请在 ES ILM 中管理其保留期",
  },
  {
    key: "retention_waf_bans",
    label: "过期封禁记录",
    hint: "WAF / DDoS 防护产生的已过期 IP 封禁记录",
  },
  {
    key: "retention_upgrade_logs",
    label: "升级任务日志",
    hint: "主控与节点升级任务及其详细日志",
  },
] as const

const loading = ref(false)
const error = ref("")
const saving = ref(false)

const loadSettings = async () => {
  try {
    loading.value = true
    error.value = ""
    const { settings: data } = await api.getSettings()
    if (data) {
      settings.value = {
        retention_system_logs: Number(data.retention_system_logs ?? 90),
        retention_es_logs: Number(data.retention_es_logs ?? 7),
        retention_waf_bans: Number(data.retention_waf_bans ?? 7),
        retention_upgrade_logs: Number(data.retention_upgrade_logs ?? 30),
      }
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载设置失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    await api.updateSettings(settings.value)
    MessagePlugin.success("数据保留设置已保存")
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存设置失败")
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadSettings()
})
</script>

