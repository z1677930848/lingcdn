<template>
  <div class="settings-tab">
    <ErrorState v-if="error" :message="error" @retry="loadSettings" />
    <LoadingState v-else-if="loading" />
    <el-form v-else label-position="top" :disabled="saving">
      <el-form-item label="ES 地址">
        <el-input v-model="settings.elasticsearch_url" placeholder="https://es.example.com:9200" />
        <div class="hint">留空表示禁用 ES 数据统计与日志检索</div>
      </el-form-item>

      <el-form-item label="ES 用户名">
        <el-input v-model="settings.elasticsearch_user" />
      </el-form-item>

      <el-form-item label="ES 密码">
        <el-input type="password" v-model="settings.elasticsearch_pass" />
        <div class="hint">留空则沿用当前已保存的密码</div>
      </el-form-item>

      <el-form-item v-if="settings.elasticsearch_url.trim() !== ''" label="日志采集">
        <div class="hint">
          节点安装时同步部署 Filebeat，分别将 <code>/var/log/lingcdn/access.log</code> 与 <code>/var/log/lingcdn/error.log</code> 直推 ES（索引：<code>cdn-access-*</code> / <code>cdn-error-*</code>）。“添加节点”生成的安装命令会自动携带 ES 参数；【已运行的老节点】需在机器上补跑：
          <pre class="hint-cmd">curl -fsSL https://auth.lingcdn.cloud/install-filebeat.sh | bash -s -- --es_host &lt;ES_IP&gt; --es_user elastic --es_pass &lt;PASSWORD&gt;</pre>
          保存设置或点击“测试连接”后，控制面会自动向 ES PUT <code>cdn-access</code> / <code>cdn-error</code> 索引模板。
        </div>
      </el-form-item>

      <div v-if="health.status !== 'idle'" class="health-row">
        <el-tag :type="health.status === 'ok' ? 'success' : 'danger'" effect="light">
          {{ health.status === 'ok' ? 'ES 连接正常' : 'ES 连接失败' }}
        </el-tag>
        <span v-if="health.message" class="health-message">{{ health.message }}</span>
      </div>

      <div class="settings-actions">
        <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
        <el-button plain @click="handleTest" :loading="testing" :disabled="saving">测试连接</el-button>
      </div>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

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
  elasticsearch_domain_field: "domain",
  elasticsearch_bytes_field: "bytes",
})

const loading = ref(false)
const error = ref("")
const saving = ref(false)
const testing = ref(false)
const health = ref<{ status: "idle" | "ok" | "error"; message?: string }>({ status: "idle" })

const loadSettings = async () => {
  try {
    loading.value = true
    error.value = ""
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
        elasticsearch_domain_field: pickNonEmpty(data.elasticsearch_domain_field, "domain"),
        elasticsearch_bytes_field: pickNonEmpty(data.elasticsearch_bytes_field, "bytes"),
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


