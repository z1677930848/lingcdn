<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">DNS 配置</h1>
        <p class="subtitle">配置 DNS 提供商并同步节点/域名解析</p>
      </div>
      <el-button :loading="loading" @click="refreshAll">刷新</el-button>
    </div>

    <ErrorState v-if="error" :message="error" @retry="refreshAll" />

    <el-card class="dns-card">
      <el-tabs class="dns-tabs" v-model="activeTab">
        <el-tab-pane name="config" label="DNS配置">
          <div class="dns-form-grid">
            <el-form label-position="top">
              <el-form-item label="DNS提供商">
                <EpSelect v-model="form.provider" :options="providersOptions" />
              </el-form-item>
              <el-form-item :label="fieldLabels.account_id">
                <el-input v-model="form.account_id" :placeholder="fieldPlaceholders.account_id" />
              </el-form-item>
              <el-form-item v-if="fieldVisibility.token" :label="fieldLabels.token">
                <el-input v-model="form.token" :placeholder="fieldPlaceholders.token" />
              </el-form-item>
              <el-form-item v-if="fieldVisibility.secret" :label="fieldLabels.secret">
                <el-input v-model="form.secret" type="password" :placeholder="fieldPlaceholders.secret" />
              </el-form-item>
              <el-form-item label="TTL">
                <el-input v-model="form.ttl" />
              </el-form-item>
              <el-form-item label="开启 IP 权重">
                <el-switch v-model="form.enable_ip_weight" />
              </el-form-item>
            </el-form>

            <div class="status-row">
              <span class="help-text">DNS错误：</span>
              <span :class="config?.last_error ? 'danger-text' : 'help-text'">
                {{ config?.last_error?.trim() || '没有错误' }}
              </span>
            </div>

            <div class="row-actions">
              <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
              <el-button :loading="recovering" :disabled="!providerSyncReady" @click="handleRecover">记录修复</el-button>
              <el-button plain :loading="syncing" :disabled="!providerSyncReady" @click="handleSync">同步解析</el-button>
            </div>
            <el-alert v-if="!providerSyncReady && form.provider" type="warning" class="sync-alert">
              当前 DNS 提供商仅支持凭证校验，自动解析同步尚未开放，请改用 AliDNS / DNSPod / Cloudflare。
            </el-alert>
          </div>
        </el-tab-pane>

        <el-tab-pane name="provider-domains" label="DNS提供商域名">
          <div class="search-row">
            <el-input v-model="search" clearable class="w-260" placeholder="搜索域名" />
            <el-button :loading="loadingProviderDomains" @click="loadProviderDomains">查询</el-button>
            <span class="help-text">实时从 DNS 提供商获取的域名列表</span>
          </div>
          <div class="admin-desktop-only">
            <EpDataTable :data="filteredProviderDomains" :columns="providerDomainColumns" row-key="name" size="small" />
          </div>
          <div class="admin-mobile-only">
            <div v-if="filteredProviderDomains.length === 0" style="text-align:center;padding:24px;color:var(--app-text-muted)">暂无域名</div>
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
        </el-tab-pane>

      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type DNSConfig, type DNSProviderOption } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(true)
const error = ref("")
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

const providersOptions = computed(() =>
  providers.value.map((p) => ({
    label: p.sync_ready === false ? `${p.label}（同步开发中）` : p.label,
    value: p.value,
  }))
)

const selectedProvider = computed(() => providers.value.find((p) => p.value === form.value.provider))

const providerSyncReady = computed(() => selectedProvider.value?.sync_ready !== false)

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
    case "route53":
    case "aws":
      return { account_id: "Access Key ID", token: "Token", secret: "Secret Access Key" }
    case "huawei":
      return { account_id: "Access Key", token: "Token", secret: "Secret Key" }
    case "google":
      return { account_id: "Project ID", token: "Access Token / JSON", secret: "Service Account JSON（可选）" }
    case "51dns":
      return { account_id: "Account ID", token: "API Token", secret: "API Secret" }
    case "dnsla":
      return { account_id: "Account ID", token: "API Token", secret: "API Secret" }
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
    error.value = ""
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
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载 DNS 配置失败"
    error.value = msg
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

