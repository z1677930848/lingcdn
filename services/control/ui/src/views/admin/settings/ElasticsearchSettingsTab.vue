<template>
  <div class="settings-tab">
    <t-form layout="vertical" label-align="top" :disabled="loading || saving">
      <t-form-item label="ES 地址">
        <t-input v-model="settings.elasticsearch_url" placeholder="https://es.example.com:9200" />
        <div class="hint">留空表示禁用 ES 数据统计与日志检索</div>
      </t-form-item>

      <t-form-item label="ES 用户名">
        <t-input v-model="settings.elasticsearch_user" />
      </t-form-item>

      <t-form-item label="ES 密码">
        <t-input type="password" v-model="settings.elasticsearch_pass" />
        <div class="hint">留空则沿用当前已保存的密码</div>
      </t-form-item>

      <t-form-item v-if="settings.elasticsearch_url.trim() !== ''" label="日志采集">
        <div class="hint">
          节点安装时同步部署 Filebeat，分别将 <code>/var/log/lingcdn/access.log</code> 与 <code>/var/log/lingcdn/error.log</code> 直推 ES（索引：<code>cdn-access-*</code> / <code>cdn-error-*</code>）。“添加节点”生成的安装命令会自动携带 ES 参数；【已运行的老节点】需在机器上补跑：
          <pre class="hint-cmd">curl -fsSL https://auth.lingcdn.cloud/install-filebeat.sh | bash -s -- --es_host &lt;ES_IP&gt; --es_user elastic --es_pass &lt;PASSWORD&gt;</pre>
          保存设置或点击“测试连接”后，控制面会自动向 ES PUT <code>cdn-access</code> / <code>cdn-error</code> 索引模板。
        </div>
      </t-form-item>

      <div v-if="health.status !== 'idle'" class="health">
        <t-tag :theme="health.status === 'ok' ? 'success' : 'danger'" variant="light">
          {{ health.status === 'ok' ? 'ES 连接正常' : 'ES 连接失败' }}
        </t-tag>
        <span v-if="health.message" class="health-message">{{ health.message }}</span>
      </div>

      <div class="actions">
        <t-button theme="primary" @click="handleSave" :loading="saving">保存</t-button>
        <t-button variant="outline" @click="handleTest" :loading="testing" :disabled="saving">测试连接</t-button>
      </div>
    </t-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"

type ESSettings = {
  elasticsearch_url: string
  elasticsearch_user: string
  elasticsearch_pass: string
  elasticsearch_index: string
  elasticsearch_ts_field: string
  elasticsearch_domain_field: string
  elasticsearch_bytes_field: string
}

const settings = ref<ESSettings>({
  elasticsearch_url: "",
  elasticsearch_user: "",
  elasticsearch_pass: "",
  elasticsearch_index: "cdn-access",
  elasticsearch_ts_field: "@timestamp",
  elasticsearch_domain_field: "domain.keyword",
  elasticsearch_bytes_field: "bytes",
})

const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const health = ref<{ status: "idle" | "ok" | "error"; message?: string }>({ status: "idle" })

const loadSettings = async () => {
  try {
    loading.value = true
    const { settings: data } = await api.getSettings()
    if (data) {
      const pickNonEmpty = (value: any, fallback: string) => {
        const s = String(value ?? "").trim()
        return s ? String(value) : fallback
      }
      settings.value = {
        elasticsearch_url: String(data.elasticsearch_url ?? ""),
        elasticsearch_user: String(data.elasticsearch_user ?? ""),
        elasticsearch_pass: "",
        elasticsearch_index: pickNonEmpty(data.elasticsearch_index, "cdn-access"),
        elasticsearch_ts_field: pickNonEmpty(data.elasticsearch_ts_field, "@timestamp"),
        elasticsearch_domain_field: pickNonEmpty(data.elasticsearch_domain_field, "domain.keyword"),
        elasticsearch_bytes_field: pickNonEmpty(data.elasticsearch_bytes_field, "bytes"),
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
    const res: any = await api.updateSettings({
      elasticsearch_url: settings.value.elasticsearch_url,
      elasticsearch_user: settings.value.elasticsearch_user,
      elasticsearch_pass: settings.value.elasticsearch_pass,
      elasticsearch_index: settings.value.elasticsearch_index,
      elasticsearch_ts_field: settings.value.elasticsearch_ts_field,
      elasticsearch_domain_field: settings.value.elasticsearch_domain_field,
      elasticsearch_bytes_field: settings.value.elasticsearch_bytes_field,
    })
    const applied: string[] = Array.isArray(res?.templates_applied) ? res.templates_applied : []
    if (applied.length > 0) {
      MessagePlugin.success(`设置已保存；ES 索引模板已推送：${applied.join(", ")}`)
    } else {
      MessagePlugin.success("设置保存成功")
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存设置失败")
  } finally {
    saving.value = false
  }
}

const handleTest = async () => {
  testing.value = true
  health.value = { status: "idle" }
  try {
    const res: any = await api.checkESHealth()
    const esStatus = String(res.body?.status ?? "")
    const applied: string[] = Array.isArray(res?.templates_applied) ? res.templates_applied : []
    const parts: string[] = []
    if (esStatus) parts.push(`集群状态：${esStatus}`)
    if (applied.length > 0) parts.push(`模板已推送：${applied.join(", ")}`)
    const message = parts.length > 0 ? parts.join("、") : "连接正常"
    health.value = { status: "ok", message }
    MessagePlugin.success("ES 连接正常")
  } catch (err: any) {
    const message = err?.message || "连接失败"
    health.value = { status: "error", message }
    MessagePlugin.error(message)
  } finally {
    testing.value = false
  }
}

onMounted(() => {
  loadSettings()
})
</script>

<style scoped>
.hint {
  font-size: 12px;
  color: var(--app-text-faint);
  margin-top: 6px;
  line-height: 1.6;
}

.hint code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  background: rgba(99, 91, 255, 0.08);
  padding: 1px 4px;
  border-radius: 3px;
}

.hint-cmd {
  margin: 6px 0 0;
  padding: 6px 8px;
  background: rgba(15, 23, 42, 0.04);
  border-radius: 4px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: var(--app-text-muted);
  white-space: pre-wrap;
  word-break: break-all;
}

.actions {
  margin-top: 12px;
  display: flex;
  gap: 8px;
}

.health {
  margin-top: 12px;
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 12px;
  flex-wrap: wrap;
}

.health-message {
  color: var(--app-text-muted);
}

@media (max-width: 768px) {
  .actions {
    flex-direction: column;
  }

  .actions > * {
    width: 100%;
  }
}
</style>

