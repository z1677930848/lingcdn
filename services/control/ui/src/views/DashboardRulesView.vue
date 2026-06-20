<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">我的规则</h1>
        <p class="subtitle">按您名下的域名维度管理缓存规则（只读）与 WAF 策略（自助维护）。</p>
      </div>
      <div class="header-actions">
        <el-button link :loading="loading" @click="load">
          <template #icon><EpIcon name="refresh" /></template>
          刷新
        </el-button>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <el-card class="section-card">
      <el-tabs v-model="activeTab">
        <el-tab-pane name="cache" label="缓存规则">
          <el-alert theme="info" class="tab-alert">
            <template #message>
              {{ brand.title }} 的缓存规则按 <code>host_pattern</code> 匹配生效。下表仅包含与您任一域名匹配的规则（含通配）。
              如需新建/修改规则，请在「域名详情 → 缓存设置」里按域名申请，管理员会审阅后发布。
            </template>
          </el-alert>
          <div class="admin-desktop-only">
            <EpDataTable
              :data="matchedCacheRules"
              :columns="cacheColumns"
              row-key="id"
              size="small"
              :loading="loading"
              empty-text="您的域名暂无缓存规则命中"
              :pagination="{
                defaultCurrent: 1,
                defaultPageSize: 10,
                pageSizeOptions: [10, 20, 50, 100],
                total: matchedCacheRules.length,
              }"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="loading-row"><div v-loading="true" style="min-height:48px" /></div>
            <div v-else-if="matchedCacheRules.length === 0" class="admin-mobile-card-empty">您的域名暂无缓存规则命中</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="rule in matchedCacheRules" :key="rule.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ rule.name || rule.id }}</span>
                  <div class="admin-mobile-card-tags">
                    <el-tag size="small" :type="rule.enabled ? 'success' : 'info'" effect="light">
                      {{ rule.enabled ? '启用' : '停用' }}
                    </el-tag>
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
        </el-tab-pane>

        <el-tab-pane name="waf" label="WAF 策略">
          <PermissionNotice :show="hasRestrictions && !canWAFEdit" />
          <el-alert theme="info" class="tab-alert">
            <template #message>
              仅展示「按域名绑定」到您名下域名的 WAF 策略。您可以自助创建、启停和删除。
              全局 / 集群级策略由平台管理员维护，请联系管理员。
            </template>
          </el-alert>
          <div v-if="canWAFEdit" class="waf-toolbar">
            <el-button type="primary" :disabled="ownDomains.length === 0" @click="openWafDialog()">
              <template #icon><EpIcon name="add" /></template>
              新建域名策略
            </el-button>
            <span v-if="ownDomains.length === 0" class="waf-toolbar-hint">
              您当前还没有域名，无法创建 WAF 策略。
            </span>
          </div>
          <div class="admin-desktop-only">
            <EpDataTable
              :data="ownPolicies"
              :columns="wafColumns"
              row-key="id"
              size="small"
              :loading="loading"
              empty-text="暂无绑定到您域名的 WAF 策略"
              :pagination="{
                defaultCurrent: 1,
                defaultPageSize: 10,
                pageSizeOptions: [10, 20, 50, 100],
                total: ownPolicies.length,
              }"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="loading-row"><div v-loading="true" style="min-height:48px" /></div>
            <div v-else-if="ownPolicies.length === 0" class="admin-mobile-card-empty">暂无绑定到您域名的 WAF 策略</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="p in ownPolicies" :key="p.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ p.name || p.id }}</span>
                  <div class="admin-mobile-card-tags">
                    <el-tag size="small" :type="p.enabled ? 'success' : 'info'" effect="light">
                      {{ p.enabled ? '启用' : '停用' }}
                    </el-tag>
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
                  <el-switch :model-value="p.enabled" size="small" @change="(v: boolean) => toggleWaf(p, v)" />
                  <span class="switch-text">{{ p.enabled ? '已启用' : '已停用' }}</span>
                  <div class="action-spacer" />
                  <el-button size="small" type="primary" link @click="openWafDialog(p)">编辑</el-button>
                  <el-button size="small" type="danger" link @click="confirmDeleteWaf(p)">删除</el-button>
                </div>
              </div>
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <EpDialog append-to-body
      v-model="wafDialogOpen"
      :title="wafEditing ? '编辑 WAF 策略' : '新建 WAF 策略'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submitWaf"
    >
      <div class="form-grid-vertical">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <el-input v-model="wafForm.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">目标域名</label>
          <EpSelect
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
          <el-input v-model="wafForm.description" :autosize="{ minRows: 2 }" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="wafForm.enabled" />
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, reactive, ref } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElSwitch } from "element-plus"
import { api, type CacheRule, type Domain, type WAFPolicy } from "@/lib/api"
import { useSystemSettings } from "@/lib/systemSettings"
import ErrorState from "@/components/common/ErrorState.vue"
import PermissionNotice from "@/components/common/PermissionNotice.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"

const { canWAFEdit, hasRestrictions } = useUserPermissions()

const { brand } = useSystemSettings()

const loading = ref(false)
const error = ref("")
const activeTab = ref<"cache" | "waf">("cache")
const rules = ref<CacheRule[]>([])
const domains = ref<Domain[]>([])
const policies = ref<WAFPolicy[]>([])

const load = async () => {
  loading.value = true
  error.value = ""
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
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载失败"
    error.value = msg
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
      canWAFEdit.value
        ? h(ElSwitch, { size: "small", modelValue: row.enabled, "onUpdate:modelValue": (v: boolean) => toggleWaf(row, v) })
        : h("span", null, row.enabled ? "是" : "否"),
  },
  ...(canWAFEdit.value
    ? [{
        colKey: "actions",
        title: "操作",
        width: 140,
        fixed: "right" as const,
        cell: (_h: any, { row }: { row: WAFPolicy }) =>
          h("div", { style: "display:flex;gap:4px" }, [
            h(ElButton, { size: "small", type: "primary", link: true, onClick: () => openWafDialog(row) }, () => "编辑"),
            h(ElButton, { size: "small", type: "danger", link: true, onClick: () => confirmDeleteWaf(row) }, () => "删除"),
          ]),
      }]
    : []),
])

onMounted(load)
</script>

