<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">DNS 配置</h1>
        <p class="subtitle">配置 DNS 提供商并同步节点/域名解析</p>
      </div>
      <t-button :loading="loading" @click="refreshAll">刷新</t-button>
    </div>

    <t-card class="card" bordered>
      <t-tabs class="tabs" v-model="activeTab">
        <t-tab-panel value="config" label="DNS配置">
          <div class="form-grid">
            <t-form layout="vertical">
              <t-form-item label="DNS提供商">
                <t-select v-model="form.provider" :options="providersOptions" />
              </t-form-item>
              <t-form-item :label="fieldLabels.account_id">
                <t-input v-model="form.account_id" :placeholder="fieldPlaceholders.account_id" />
              </t-form-item>
              <t-form-item v-if="fieldVisibility.token" :label="fieldLabels.token">
                <t-input v-model="form.token" :placeholder="fieldPlaceholders.token" />
              </t-form-item>
              <t-form-item v-if="fieldVisibility.secret" :label="fieldLabels.secret">
                <t-input v-model="form.secret" type="password" :placeholder="fieldPlaceholders.secret" />
              </t-form-item>
              <t-form-item label="TTL">
                <t-input v-model="form.ttl" />
              </t-form-item>
              <t-form-item label="开启 IP 权重">
                <t-switch v-model="form.enable_ip_weight" />
              </t-form-item>
            </t-form>

            <div class="status-row">
              <span class="help-text">DNS错误：</span>
              <span :class="config?.last_error ? 'danger-text' : 'help-text'">
                {{ config?.last_error?.trim() || '没有错误' }}
              </span>
            </div>

            <div class="row-actions">
              <t-button theme="primary" :loading="saving" @click="handleSave">保存</t-button>
              <t-button :loading="recovering" @click="handleRecover">记录修复</t-button>
              <t-button variant="outline" :loading="syncing" @click="handleSync">同步解析</t-button>
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="provider-domains" label="DNS提供商域名">
          <div class="search-row">
            <t-input v-model="search" clearable class="w-260" placeholder="搜索域名" />
            <t-button :loading="loadingProviderDomains" @click="loadProviderDomains">查询</t-button>
            <span class="help-text">实时从 DNS 提供商获取的域名列表</span>
          </div>
          <div class="admin-desktop-only">
            <t-table :data="filteredProviderDomains" :columns="providerDomainColumns" row-key="name" size="small" />
          </div>
          <div class="admin-mobile-only">
            <div v-if="filteredProviderDomains.length === 0" style="text-align:center;padding:24px;color:#475569">暂无域名</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="row in filteredProviderDomains" :key="row.name" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">{{ row.name }}</div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">解析记录数</span>
                    <span class="admin-mobile-card-value">{{ row.record_count ?? '-' }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </t-tab-panel>

      </t-tabs>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api, type DNSConfig, type DNSProviderOption } from "@/lib/api"

const loading = ref(true)
const saving = ref(false)
const syncing = ref(false)
const recovering = ref(false)
const loadingProviderDomains = ref(false)

const config = ref<DNSConfig | null>(null)
const providers = ref<DNSProviderOption[]>([])
const providerDomains = ref<{ name: string; record_count: number }[]>([])
const search = ref("")
const activeTab = ref("config")

const form = ref({
  provider: "",
  account_id: "",
  token: "",
  secret: "",
  ttl: 600,
  enable_ip_weight: false,
})

const providersOptions = computed(() => providers.value.map((p) => ({ label: p.label, value: p.value })))

const fieldLabels = computed(() => {
  const p = form.value.provider
  switch (p) {
    case "alidns":
    case "aliyun":
      return { account_id: "AccessKey ID", token: "Token", secret: "AccessKey Secret" }
    case "cloudflare":
      return { account_id: "Account ID（可选）", token: "API Token", secret: "API Key（可选）" }
    case "dnspod":
    case "dnspod-global":
    case "dnspod_global":
    case "dnspod-intl":
    case "dnspod-int":
      return { account_id: "DNSPod ID", token: "DNSPod Token", secret: "Secret" }
    default:
      return { account_id: "ID", token: "Token", secret: "Secret" }
  }
})

const fieldPlaceholders = computed(() => {
  const p = form.value.provider
  switch (p) {
    case "alidns":
    case "aliyun":
      return { account_id: "请输入AccessKey ID", token: "", secret: "请输入AccessKey Secret" }
    case "cloudflare":
      return { account_id: "请输入Account ID", token: "请输入API Token", secret: "Global API Key（使用 Token 时可留空）" }
    case "dnspod":
    case "dnspod-global":
    case "dnspod_global":
    case "dnspod-intl":
    case "dnspod-int":
      return { account_id: "请输入DNSPod ID", token: "请输入DNSPod Token", secret: "" }
    default:
      return { account_id: "", token: "", secret: "" }
  }
})

const fieldVisibility = computed(() => {
  const p = form.value.provider
  switch (p) {
    case "alidns":
    case "aliyun":
      // 阿里云只需要 AccessKey ID + AccessKey Secret，不需要 Token 字段
      return { token: false, secret: true }
    case "cloudflare":
      return { token: true, secret: true }
    case "dnspod":
    case "dnspod-global":
    case "dnspod_global":
    case "dnspod-intl":
    case "dnspod-int":
      // DNSPod 只需要 ID + Token，不需要 Secret
      return { token: true, secret: false }
    default:
      return { token: true, secret: true }
  }
})

const loadConfig = async () => {
  try {
    loading.value = true
    const res = await api.getDNSConfig()
    config.value = res.config
    providers.value = res.providers || []
    form.value = {
      provider: res.config?.provider || "",
      account_id: res.config?.account_id || "",
      token: res.config?.token || "",
      secret: res.config?.secret || "",
      ttl: res.config?.ttl || 600,
      enable_ip_weight: Boolean(res.config?.enable_ip_weight),
    }
    // 阿里云兼容：如果 secret 为空但 token 有值，把 token 回填到 secret 显示
    const pv = form.value.provider
    if ((pv === "alidns" || pv === "aliyun") && !form.value.secret && form.value.token) {
      form.value.secret = form.value.token
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载 DNS 配置失败")
  } finally {
    loading.value = false
  }
}

const loadProviderDomains = async () => {
  loadingProviderDomains.value = true
  try {
    const res = await api.getProviderDomains()
    providerDomains.value = res.domains || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "获取 DNS 提供商域名失败")
  } finally {
    loadingProviderDomains.value = false
  }
}

const refreshAll = () => {
  loadConfig()
  loadProviderDomains()
}

onMounted(() => {
  refreshAll()
})

const filteredProviderDomains = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return providerDomains.value
  return (providerDomains.value || []).filter((d) => d.name.toLowerCase().includes(q))
})

const handleSave = async () => {
  if (saving.value) return
  if (!form.value.provider) {
    MessagePlugin.warning("请选择 DNS 提供商")
    return
  }
  const p = form.value.provider
  const isAli = p === "alidns" || p === "aliyun"
  if (isAli) {
    if (!form.value.account_id.trim()) {
      MessagePlugin.warning("请输入 AccessKey ID")
      return
    }
    if (!form.value.secret.trim()) {
      MessagePlugin.warning("请输入 AccessKey Secret")
      return
    }
  } else {
    if (!form.value.token.trim()) {
      MessagePlugin.warning("请输入 Token")
      return
    }
  }
  saving.value = true
  try {
    const p = form.value.provider
    const isAliProvider = p === "alidns" || p === "aliyun"
    // 阿里云：AccessKey Secret 同时写入 token 和 secret 保证后端兼容
    const tokenVal = isAliProvider ? form.value.secret.trim() : form.value.token.trim()
    const secretVal = isAliProvider ? form.value.secret.trim() : form.value.secret.trim()
    const res = await api.saveDNSConfig({
      provider: form.value.provider,
      account_id: form.value.account_id.trim(),
      token: tokenVal,
      secret: secretVal,
      ttl: Number(form.value.ttl) || 600,
      enable_ip_weight: Boolean(form.value.enable_ip_weight),
    })
    config.value = res.config
    MessagePlugin.success("DNS 配置已保存")
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存失败")
  } finally {
    saving.value = false
  }
}

const handleRecover = async () => {
  if (recovering.value) return
  recovering.value = true
  try {
    const res = await api.recoverDNSConfig()
    MessagePlugin.success(res.message || "已提交记录修复")
  } catch (err: any) {
    MessagePlugin.error(err.message || "记录修复失败")
  } finally {
    recovering.value = false
  }
}

const handleSync = async () => {
  if (syncing.value) return
  syncing.value = true
  try {
    const res = await api.syncDNS()
    if (res.ok === false) {
      MessagePlugin.error(res.message || "同步失败")
    } else {
      MessagePlugin.success(res.message || "同步成功")
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "同步失败")
  } finally {
    syncing.value = false
  }
}

const providerDomainColumns = [
  {
    colKey: "name",
    title: "域名",
    minWidth: 260,
    cell: (_h: any, { row }: { row: { name: string } }) => h("span", { style: "font-weight:600" }, row.name),
  },
  {
    colKey: "record_count",
    title: "解析记录数",
    width: 140,
    cell: (_h: any, { row }: { row: { record_count: number } }) => row.record_count ?? "-",
  },
]

</script>

<style scoped>
.page {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.card {
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
}

.tabs :deep(.t-tabs__nav) {
  padding: 0 16px;
  border-bottom: 1px solid #e2e8f0;
}

.tabs :deep(.t-tabs__content) {
  padding: 16px;
}

.form-grid {
  display: grid;
  gap: 16px;
  max-width: 720px;
}

.row-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin-top: 8px;
}

.help-text {
  font-size: 12px;
  color: #475569;
}

.danger-text {
  color: #ef4444;
  font-size: 12px;
}

.status-row {
  display: flex;
  gap: 12px;
  align-items: center;
  flex-wrap: wrap;
}

.table-muted {
  font-size: 12px;
  color: #475569;
}

.search-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.w-260 {
  width: 260px;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace;
}

@media (max-width: 768px) {
  .tabs :deep(.t-tabs__content) {
    padding: 12px;
  }

  .form-grid {
    max-width: 100%;
  }

  .search-row {
    flex-direction: column;
    align-items: stretch;
  }

  .w-260 {
    width: 100%;
  }

  .row-actions {
    flex-direction: column;
  }

  .row-actions .t-button {
    width: 100%;
  }
}
</style>
