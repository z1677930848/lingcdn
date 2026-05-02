<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">我的规则</h1>
        <p class="subtitle">按您名下的域名维度管理缓存规则（只读）与 WAF 策略（自助维护）。</p>
      </div>
      <div class="header-actions">
        <t-button variant="text" :loading="loading" @click="load">
          <template #icon><t-icon name="refresh" /></template>
          刷新
        </t-button>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <t-tabs v-model="activeTab">
        <t-tab-panel value="cache" label="缓存规则">
          <t-alert theme="info" class="tab-alert">
            <template #message>
              {{ brand.title }} 的缓存规则按 <code>host_pattern</code> 匹配生效。下表仅包含与您任一域名匹配的规则（含通配）。
              如需新建/修改规则，请在「域名详情 → 缓存设置」里按域名申请，管理员会审阅后发布。
            </template>
          </t-alert>
          <div class="admin-desktop-only">
            <t-table
              :data="matchedCacheRules"
              :columns="cacheColumns"
              row-key="id"
              size="small"
              bordered
              :loading="loading"
              empty="您的域名暂无缓存规则命中"
              :pagination="{
                defaultCurrent: 1,
                defaultPageSize: 10,
                pageSizeOptions: [10, 20, 50, 100],
                total: matchedCacheRules.length,
              }"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="loading-row"><t-loading /></div>
            <div v-else-if="matchedCacheRules.length === 0" class="admin-mobile-card-empty">您的域名暂无缓存规则命中</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="rule in matchedCacheRules" :key="rule.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ rule.name || rule.id }}</span>
                  <div class="admin-mobile-card-tags">
                    <t-tag size="small" :theme="rule.enabled ? 'success' : 'default'" variant="light">
                      {{ rule.enabled ? '启用' : '停用' }}
                    </t-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">Host</span>
                    <span class="admin-mobile-card-value"><code>{{ rule.host_pattern || '*' }}</code></span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">路径</span>
                    <span class="admin-mobile-card-value"><code>{{ rule.path_pattern || '/*' }}</code></span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">TTL</span>
                    <span class="admin-mobile-card-value">{{ rule.ttl_seconds }}s</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">更新时间</span>
                    <span class="admin-mobile-card-value">{{ formatTime(rule.updated_at) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="waf" label="WAF 策略">
          <t-alert theme="info" class="tab-alert">
            <template #message>
              仅展示「按域名绑定」到您名下域名的 WAF 策略。您可以自助创建、启停和删除。
              全局 / 集群级策略由平台管理员维护，请联系管理员。
            </template>
          </t-alert>
          <div class="waf-toolbar">
            <t-button theme="primary" :disabled="ownDomains.length === 0" @click="openWafDialog()">
              <template #icon><t-icon name="add" /></template>
              新建域名策略
            </t-button>
            <span v-if="ownDomains.length === 0" class="waf-toolbar-hint">
              您当前还没有域名，无法创建 WAF 策略。
            </span>
          </div>
          <div class="admin-desktop-only">
            <t-table
              :data="ownPolicies"
              :columns="wafColumns"
              row-key="id"
              size="small"
              bordered
              :loading="loading"
              empty="暂无绑定到您域名的 WAF 策略"
              :pagination="{
                defaultCurrent: 1,
                defaultPageSize: 10,
                pageSizeOptions: [10, 20, 50, 100],
                total: ownPolicies.length,
              }"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="loading-row"><t-loading /></div>
            <div v-else-if="ownPolicies.length === 0" class="admin-mobile-card-empty">暂无绑定到您域名的 WAF 策略</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="p in ownPolicies" :key="p.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ p.name || p.id }}</span>
                  <div class="admin-mobile-card-tags">
                    <t-tag size="small" :theme="p.enabled ? 'success' : 'default'" variant="light">
                      {{ p.enabled ? '启用' : '停用' }}
                    </t-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">域名</span>
                    <span class="admin-mobile-card-value">{{ domainNameOf(p.scope_id) }}</span>
                  </div>
                  <div v-if="p.description" class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">描述</span>
                    <span class="admin-mobile-card-value">{{ p.description }}</span>
                  </div>
                </div>
                <div class="admin-mobile-card-actions">
                  <t-switch :model-value="p.enabled" size="small" @change="(v: boolean) => toggleWaf(p, v)" />
                  <span class="switch-text">{{ p.enabled ? '已启用' : '已停用' }}</span>
                  <div class="action-spacer" />
                  <t-button size="small" theme="primary" variant="text" @click="openWafDialog(p)">编辑</t-button>
                  <t-button size="small" theme="danger" variant="text" @click="confirmDeleteWaf(p)">删除</t-button>
                </div>
              </div>
            </div>
          </div>
        </t-tab-panel>
      </t-tabs>
    </t-card>

    <t-dialog
      v-model:visible="wafDialogOpen"
      :header="wafEditing ? '编辑 WAF 策略' : '新建 WAF 策略'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitWaf"
    >
      <div class="form-grid">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <t-input v-model="wafForm.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">目标域名</label>
          <t-select
            v-model="wafForm.scope_id"
            :options="ownDomainOptions"
            filterable
            class="form-input-v"
            :disabled="!!wafEditing"
          />
          <div class="form-hint-v">
            仅能选择您名下的域名；创建后域名不可改动，若需切换请重建策略。
          </div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">描述</label>
          <t-textarea v-model="wafForm.description" :autosize="{ minRows: 2 }" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <t-switch v-model="wafForm.enabled" />
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from "vue"
import { DialogPlugin, MessagePlugin, Switch, Button } from "tdesign-vue-next"
import { api, type CacheRule, type Domain, type WAFPolicy } from "@/lib/api"
import { useSystemSettings } from "@/lib/systemSettings"

const { brand } = useSystemSettings()

const loading = ref(false)
const activeTab = ref<"cache" | "waf">("cache")
const rules = ref<CacheRule[]>([])
const domains = ref<Domain[]>([])
const policies = ref<WAFPolicy[]>([])

const load = async () => {
  loading.value = true
  try {
    // listDomains is already user-scoped by the backend, so `domains`
    // contains only the current user's sites. We then filter cache rules
    // by host_pattern and WAF policies by (scope=domain, scope_id ∈ own).
    const [r, d, p] = await Promise.all([
      api.listCacheRules(),
      api.listDomains(),
      api.listWAFPolicies({ scope: "domain" }),
    ])
    rules.value = r.cache_rules || []
    domains.value = d.domains || []
    policies.value = p.policies || []
  } catch (err: any) {
    MessagePlugin.error(err?.message || "加载失败")
  } finally {
    loading.value = false
  }
}

// --- 缓存规则 ---
const ownDomains = computed<Domain[]>(() => domains.value || [])
const ownIds = computed(() => new Set(ownDomains.value.map((d) => d.id)))
const ownNames = computed(() => new Set(ownDomains.value.map((d) => d.name)))

const matchedCacheRules = computed<CacheRule[]>(() =>
  rules.value.filter((r) => {
    const p = (r.host_pattern || "").trim()
    if (!p) return false
    if (p === "*") return true
    if (ownNames.value.has(p)) return true
    const stripped = p.startsWith("*.") ? p.slice(1) : p.startsWith(".") ? p : ""
    if (stripped) {
      for (const n of ownNames.value) {
        if (n.endsWith(stripped)) return true
      }
    }
    return false
  }),
)

// --- WAF 策略 ---
const ownPolicies = computed<WAFPolicy[]>(() =>
  policies.value.filter((p) => p.scope === "domain" && !!p.scope_id && ownIds.value.has(p.scope_id)),
)

const ownDomainOptions = computed(() =>
  ownDomains.value.map((d) => ({ label: d.name, value: d.id })),
)

const wafDialogOpen = ref(false)
const wafEditing = ref<WAFPolicy | null>(null)
const wafForm = reactive<WAFPolicy>({
  id: "",
  name: "",
  scope: "domain",
  scope_id: "",
  description: "",
  enabled: true,
})

const openWafDialog = (p?: WAFPolicy) => {
  wafEditing.value = p || null
  if (p) {
    Object.assign(wafForm, p)
  } else {
    wafForm.id = ""
    wafForm.name = "新策略"
    wafForm.scope = "domain"
    // Preselect the first own domain so the required scope_id is never
    // accidentally empty on submit.
    wafForm.scope_id = ownDomains.value[0]?.id || ""
    wafForm.description = ""
    wafForm.enabled = true
  }
  wafDialogOpen.value = true
}

const submitWaf = async () => {
  if (!wafForm.scope_id) {
    MessagePlugin.error("请选择目标域名")
    return
  }
  if (!ownIds.value.has(wafForm.scope_id)) {
    MessagePlugin.error("只能为您名下的域名创建策略")
    return
  }
  try {
    if (wafEditing.value) {
      await api.updateWAFPolicy(wafEditing.value.id, { ...wafForm, scope: "domain" })
      MessagePlugin.success("策略已更新")
    } else {
      await api.createWAFPolicy({ ...wafForm, scope: "domain" })
      MessagePlugin.success("策略已创建")
    }
    wafDialogOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  }
}

const toggleWaf = async (p: WAFPolicy, enabled: boolean) => {
  try {
    await api.updateWAFPolicy(p.id, { ...p, enabled })
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新失败")
  }
}

const confirmDeleteWaf = (p: WAFPolicy) => {
  const inst = DialogPlugin.confirm({
    header: "删除 WAF 策略",
    body: `确认删除策略「${p.name || p.id}」吗？`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    onConfirm: async () => {
      try {
        await api.deleteWAFPolicy(p.id)
        MessagePlugin.success("策略已删除")
        await load()
      } catch (err: any) {
        MessagePlugin.error(err?.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const domainNameOf = (id?: string) => {
  const d = ownDomains.value.find((x) => x.id === id)
  return d?.name || id || "-"
}

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

const cacheColumns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 160 },
  { colKey: "host_pattern", title: "Host 模式", width: 180 },
  { colKey: "path_pattern", title: "路径模式", minWidth: 180 },
  {
    colKey: "ttl_seconds",
    title: "TTL (秒)",
    width: 110,
    cell: (_h: any, { row }: { row: CacheRule }) => String(row.ttl_seconds || 0),
  },
  {
    colKey: "enabled",
    title: "状态",
    width: 100,
    cell: (_h: any, { row }: { row: CacheRule }) => (row.enabled ? "启用" : "停用"),
  },
  {
    colKey: "updated_at",
    title: "更新时间",
    width: 180,
    cell: (_h: any, { row }: { row: CacheRule }) => formatTime(row.updated_at),
  },
])

const wafColumns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 160 },
  {
    colKey: "scope_id",
    title: "域名",
    width: 220,
    cell: (_h: any, { row }: { row: WAFPolicy }) => domainNameOf(row.scope_id),
  },
  { colKey: "description", title: "描述", minWidth: 200 },
  {
    colKey: "enabled",
    title: "启用",
    width: 80,
    cell: (_h: any, { row }: { row: WAFPolicy }) =>
      h(Switch, { size: "small", modelValue: row.enabled, "onUpdate:modelValue": (v: boolean) => toggleWaf(row, v) }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    fixed: "right",
    cell: (_h: any, { row }: { row: WAFPolicy }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(Button, { size: "small", theme: "primary", variant: "text", onClick: () => openWafDialog(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => confirmDeleteWaf(row) }, () => "删除"),
      ]),
  },
])

onMounted(load)
</script>

<style scoped>
.page {
  padding: 24px;
}
.header {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  margin-bottom: 16px;
  flex-wrap: wrap;
  gap: 16px;
}
.header-actions {
  display: flex;
  gap: 10px;
  align-items: center;
}
.title {
  font-size: 20px;
  font-weight: 600;
  margin: 0 0 4px;
  color: var(--app-text-strong);
}
.subtitle {
  color: var(--app-text-faint);
  margin: 0;
  font-size: 13px;
}

.tab-alert {
  margin: 12px 0;
}

.waf-toolbar {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.waf-toolbar-hint {
  color: var(--app-text-faint);
  font-size: 12px;
}

.loading-row {
  text-align: center;
  padding: 32px 0;
}

.admin-mobile-card-actions {
  align-items: center;
}

.action-spacer {
  flex: 1;
}

.switch-text {
  font-size: 12px;
  color: var(--app-text-muted);
}

.form-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
}
.form-row-v {
  display: grid;
  grid-template-columns: 100px minmax(280px, 1fr);
  gap: 6px 16px;
  align-items: center;
}
.form-label-v {
  color: var(--app-text-faint);
  font-weight: 500;
  text-align: right;
}
.form-input-v {
  width: 100%;
}
.form-hint-v {
  grid-column: 2;
  color: var(--app-text-faint);
  font-size: 12px;
  margin-top: 2px;
}
.section-card {
  min-width: 0;
}
.section-card :deep(.t-table) {
  min-width: 100%;
}

@media (max-width: 768px) {
  .page {
    padding: 12px;
  }

  .header {
    flex-direction: column;
    align-items: stretch;
  }

  .header-actions {
    flex-wrap: wrap;
  }

  .title {
    font-size: 16px;
  }

  .form-row-v {
    grid-template-columns: 1fr;
    gap: 4px;
  }

  .form-label-v {
    text-align: left;
  }

  .form-hint-v {
    grid-column: 1;
  }
}
</style>
