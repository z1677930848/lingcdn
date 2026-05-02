<template>
  <div class="settings-tab">
    <p class="desc">设置各类数据的保留天数，超过保留期的数据将被自动清理。设置为 <code>0</code> 表示不自动清理。</p>

    <t-form layout="vertical" label-align="top" :disabled="loading || saving">
      <t-form-item v-for="item in retentionItems" :key="item.key" :label="item.label">
        <div class="input-with-unit">
          <t-input
            :value="retentionText(item.key)"
            @change="(v: string) => setRetention(item.key, v)"
            type="number"
            placeholder="0-9999"
          />
          <span class="unit">天</span>
        </div>
        <div class="hint">{{ item.hint }}</div>
      </t-form-item>

      <div class="actions">
        <t-button theme="primary" @click="handleSave" :loading="saving">保存</t-button>
      </div>
    </t-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"

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
const saving = ref(false)

const loadSettings = async () => {
  try {
    loading.value = true
    const { settings: data } = await api.getSettings()
    if (data) {
      settings.value = {
        retention_system_logs: Number(data.retention_system_logs ?? 90),
        retention_es_logs: Number(data.retention_es_logs ?? 7),
        retention_waf_bans: Number(data.retention_waf_bans ?? 7),
        retention_upgrade_logs: Number(data.retention_upgrade_logs ?? 30),
      }
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载设置失败")
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

<style scoped>
.desc {
  color: var(--app-text-muted);
  font-size: 13px;
  margin: 0 0 16px;
  line-height: 1.6;
}

.desc code {
  background: var(--app-surface-muted);
  border: 1px solid var(--app-border);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--app-text-strong);
}

.input-with-unit {
  display: flex;
  align-items: center;
  gap: 8px;
}

.unit {
  font-size: 14px;
  color: var(--app-text);
  flex-shrink: 0;
}

.hint {
  font-size: 12px;
  color: var(--app-text-faint);
  margin-top: 6px;
}

.actions {
  margin-top: 12px;
}

@media (max-width: 768px) {
  .input-with-unit {
    width: 100%;
  }

  .input-with-unit :deep(.t-input-number) {
    flex: 1;
  }
}
</style>
