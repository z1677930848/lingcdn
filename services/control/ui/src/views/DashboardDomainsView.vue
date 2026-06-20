<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">网站管理</h1>
        <p class="subtitle">管理网站域名、源站、缓存与同步状态</p>
      </div>
      <div class="header-actions">
        <el-button v-if="canDomainsWrite" type="primary" @click="openCreate">
          <template #icon><EpIcon name="add" /></template>
          添加网站
        </el-button>
        <el-input v-model="search" clearable placeholder="搜索域名 / CNAME / 源站" class="search-input" />
        <EpSelect v-model="enabledFilter" :options="enabledOptions" class="filter-select" />
        <EpSelect v-model="syncFilter" :options="syncOptions" class="filter-select filter-select-wide" />
        <div class="header-actions-extra">
          <EpDropdown
            v-if="canDomainsWrite"
            :options="bulkActionOptions"
            trigger="click"
            @click="onBulkAction"
          >
            <el-button plain :disabled="selectedIds.length === 0">
              批量操作（{{ selectedIds.length }}）
              <template #suffix><EpIcon name="chevron-down" /></template>
            </el-button>
          </EpDropdown>
          <el-button plain @click="exportCSV">
            <template #icon><EpIcon name="download" /></template>
            导出
          </el-button>
          <el-button link :loading="loading" @click="load">
            <template #icon><EpIcon name="refresh" /></template>
            刷新
          </el-button>
        </div>
        <EpDropdown
          v-if="canDomainsWrite"
          class="header-actions-more"
          :options="mobileMoreOptions"
          trigger="click"
          @click="onMobileMore"
        >
          <el-button plain block>
            更多操作
            <template #suffix><EpIcon name="chevron-down" /></template>
          </el-button>
        </EpDropdown>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <PermissionNotice :show="hasRestrictions && !canDomainsWrite" />

    <div v-if="metaError" class="meta-error-banner">
      <el-alert type="warning" :message="metaError" />
      <el-button link size="small" type="primary" @click="loadMeta">重试</el-button>
    </div>

    <el-card class="section-card">
      <div class="admin-desktop-only">
        <EpDataTable
          :data="filtered"
          :columns="columns"
          row-key="id"
          size="small"
          :loading="loading"
          empty-text="暂无网站"
          :selected-row-keys="selectedIds"
          @select-change="onSelectChange"
          :pagination="{
            defaultCurrent: 1,
            defaultPageSize: 10,
            pageSizeOptions: [10, 20, 50, 100],
            showJumper: true,
            total: filtered.length,
          }"
        />
      </div>
      <div class="admin-mobile-only">
        <div v-if="loading" style="text-align:center;padding:32px 0"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="filtered.length === 0" class="admin-mobile-card-empty">暂无网站</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="item in filtered" :key="item.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ item.name }}</div>
              <div class="admin-mobile-card-tags">
                <el-tag v-if="item.https_enabled" size="small" type="primary">HTTPS</el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-subtitle">
              <template v-if="syncTasks.isSyncing(`domain:${item.id}`)">
                <span class="sync-dot sync-dot-running"></span> 同步中
              </template>
              <template v-else-if="syncTasks.isFailed(`domain:${item.id}`)">
                <span class="sync-dot sync-dot-failed"></span>
                <span :title="syncTasks.bySubject(`domain:${item.id}`)?.message || ''">同步失败</span>
                <el-link type="primary" hover="color" style="margin-left:4px;cursor:pointer" @click="retrySync(item)">重试</el-link>
              </template>
              <template v-else>{{ item.cname || '-' }}</template>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">源站</span>
                <span class="admin-mobile-card-value">{{ (item.origin_addresses || []).join(', ') || item.origin_id || '-' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">套餐</span>
                <span class="admin-mobile-card-value">{{ formatLineGroup(item) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">HTTPS</span>
                <span class="admin-mobile-card-value" :style="{ color: item.https_enabled ? 'var(--app-success)' : 'var(--app-danger)' }">
                  {{ item.https_enabled ? '已启用' : '未启用' }}
                </span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">更新时间</span>
                <span class="admin-mobile-card-value">{{ formatTime(item.updated_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button size="small" type="primary" link @click="router.push(`/dashboard/domains/${item.id}`)">{{ canDomainsWrite ? '管理' : '查看' }}</el-button>
              <el-button v-if="canDomainsWrite" size="small" type="danger" link :disabled="Boolean(saving[`${item.id}-delete`])" @click="handleDelete(item)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body
      v-model="createOpen"
      header="添加网站"
      :confirm-btn="{ content: '提交', loading: createLoading }"
      cancel-btn="取消"
      @confirm="submitCreate"
    >
      <el-alert v-if="metaError" type="warning" :message="metaError" style="margin-bottom: 12px" />
      <el-tabs v-model="createTab">
        <el-tab-pane name="single" label="单个添加">
          <div class="form-stack-compact">
            <div class="form-row-compact">
              <div class="form-label">套餐</div>
              <EpSelect v-model="singleForm.orderID" :options="orderOptions" placeholder="请选择套餐" />
            </div>
            <div v-if="selectedSingleOrder" class="form-tip">
              集群：{{ selectedSingleOrder.line_group_id || '-' }} ·
              域名上限：{{ selectedSingleOrder.domain_limit || '不限' }} ·
              已使用：{{ selectedSingleOrder.domain_count || 0 }}
            </div>
            <div class="form-row-compact">
              <div class="form-label">网站域名</div>
              <el-input v-model="singleForm.name" />
            </div>
            <div class="form-row-compact align-top">
              <div class="form-label">源站地址</div>
              <el-input v-model="singleForm.originAddresses" :autosize="true" placeholder="IP 或域名，多个用空格 / 逗号 / 换行分隔" />
            </div>
          </div>
        </el-tab-pane>
        <el-tab-pane name="batch" label="批量添加">
          <div class="form-stack-compact">
            <div class="form-row-compact">
              <div class="form-label">套餐</div>
              <EpSelect v-model="batchForm.orderID" :options="orderOptions" placeholder="请选择套餐" />
            </div>
            <div v-if="selectedBatchOrder" class="form-tip">
              集群：{{ selectedBatchOrder.line_group_id || '-' }} ·
              域名上限：{{ selectedBatchOrder.domain_limit || '不限' }} ·
              已使用：{{ selectedBatchOrder.domain_count || 0 }}
            </div>
            <div class="form-row-compact align-top">
              <div class="form-label">网站域名</div>
              <el-input v-model="batchForm.names" :autosize="true" />
            </div>
            <div class="form-row-compact align-top">
              <div class="form-label">源站地址</div>
              <el-input v-model="batchForm.originAddresses" :autosize="true" />
            </div>
          </div>
        </el-tab-pane>
      </el-tabs>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpDropdown from "@/components/ep/EpDropdown.vue"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElLink } from "element-plus"
import { api, type Domain, type UserOrder } from "@/lib/api"
import { useSyncTasks } from "@/composables/useSyncTasks"
import ErrorState from "@/components/common/ErrorState.vue"
import PermissionNotice from "@/components/common/PermissionNotice.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"

const router = useRouter()
const { canDomainsWrite, hasRestrictions } = useUserPermissions()

const syncTasks = useSyncTasks("domain:")

const loading = ref(false)
const error = ref("")
const domains = ref<Domain[]>([])
const search = ref("")
const enabledFilter = ref("all")
const syncFilter = ref<"all" | "syncing" | "failed" | "idle">("all")
const saving = ref<Record<string, boolean>>({})

const selectedIds = ref<string[]>([])
const onSelectChange = (keys: (string | number)[]) => {
  selectedIds.value = keys.map(String)
}

const bulkActionOptions = [
  { content: "启用", value: "enable" },
  { content: "禁用", value: "disable" },
  { content: "开启缓存", value: "cache_on" },
  { content: "关闭缓存", value: "cache_off" },
  { content: "申请 Let's Encrypt 证书", value: "request_cert" },
  { content: "重置 CNAME", value: "regen_cname" },
  { content: "删除", value: "delete", theme: "danger" },
]

const mobileMoreOptions = computed(() => [
  ...bulkActionOptions.map((item) => ({
    ...item,
    value: `bulk:${item.value}`,
    disabled: selectedIds.value.length === 0,
  })),
  { content: "导出 CSV", value: "export" },
  { content: "刷新列表", value: "refresh" },
])

const onMobileMore = (ctx: { value: string }) => {
  if (ctx.value === "export") {
    exportCSV()
    return
  }
  if (ctx.value === "refresh") {
    void load()
    return
  }
  if (ctx.value.startsWith("bulk:")) {
    void onBulkAction({ value: ctx.value.slice(5) })
  }
}

const onBulkAction = async (ctx: { value: string }) => {
  if (selectedIds.value.length === 0) return
  const ids = [...selectedIds.value]
  const selected = domains.value.filter((d) => ids.includes(d.id))
  const verb: Record<string, string> = {
    enable: "启用", disable: "禁用", cache_on: "开启缓存",
    cache_off: "关闭缓存", delete: "删除", request_cert: "申请证书",
    regen_cname: "重置 CNAME",
  }
  const label = verb[ctx.value] || ctx.value
  let ok = 0
  const errors: string[] = []
  for (const d of selected) {
    try {
      if (ctx.value === "delete") {
        await api.deleteDomain(d.id)
      } else if (ctx.value === "enable") {
        await api.updateDomain(d.id, { ...d, enabled: true })
      } else if (ctx.value === "disable") {
        await api.updateDomain(d.id, { ...d, enabled: false })
      } else if (ctx.value === "cache_on") {
        await api.updateDomain(d.id, { ...d, cache_enabled: true })
      } else if (ctx.value === "cache_off") {
        await api.updateDomain(d.id, { ...d, cache_enabled: false })
      } else if (ctx.value === "request_cert") {
        await api.requestACMECertificate({ domain: d.name })
      } else if (ctx.value === "regen_cname") {
        await api.updateDomain(d.id, { ...d, cname: "" })
      }
      ok++
    } catch (e: any) {
      errors.push(`${d.name}: ${e?.message || "失败"}`)
    }
  }
  if (errors.length === 0) {
    MessagePlugin.success(`已${label} ${ok} 个`)
  } else {
    MessagePlugin.warning({
      content: `${label}：成功 ${ok} 个，失败 ${errors.length} 个\n${errors.join("\n")}`,
      duration: 8000,
    })
  }
  selectedIds.value = []
  await load()
}

const exportCSV = () => {
  const rows = filtered.value
  const header = ["id", "name", "cname", "origin", "listen_port", "https_enabled", "enabled", "updated_at"]
  const lines = [header.join(",")]
  for (const d of rows) {
    const origin = (d.origin_addresses || []).join("|") || d.origin_name || d.origin_id || ""
    const cells = [
      d.id, d.name, d.cname || "", origin,
      String(d.listen_port || 80),
      d.https_enabled ? "1" : "0",
      d.enabled !== false ? "1" : "0",
      d.updated_at || "",
    ].map((v) => {
      const s = String(v).replace(/"/g, '""')
      return /[",\n]/.test(s) ? `"${s}"` : s
    })
    lines.push(cells.join(","))
  }
  const blob = new Blob(["\ufeff" + lines.join("\n")], { type: "text/csv;charset=utf-8" })
  const url = URL.createObjectURL(blob)
  const a = document.createElement("a")
  a.href = url
  a.download = `domains-${new Date().toISOString().slice(0, 10)}.csv`
  document.body.appendChild(a)
  a.click()
  document.body.removeChild(a)
  URL.revokeObjectURL(url)
  MessagePlugin.success(`已导出 ${rows.length} 条记录`)
}

const createOpen = ref(false)
const createTab = ref<"single" | "batch">("single")
const createLoading = ref(false)
const metaError = ref("")
const orders = ref<UserOrder[]>([])

const singleForm = ref({
  name: "",
  orderID: "",
  originAddresses: "",
  cacheEnabled: true,
  http2Enabled: true,
  enabled: true,
})

const batchForm = ref({
  names: "",
  orderID: "",
  originAddresses: "",
  cacheEnabled: true,
  http2Enabled: true,
  enabled: true,
})

const enabledOptions = [
  { label: "全部", value: "all" },
  { label: "已启用", value: "on" },
  { label: "已禁用", value: "off" },
]

const syncOptions = [
  { label: "全部", value: "all" },
  { label: "同步中", value: "syncing" },
  { label: "同步失败", value: "failed" },
  { label: "空闲", value: "idle" },
]

const parseList = (raw: string) => {
  return Array.from(
    new Set(
      String(raw || "")
        .split(/[\s,，]+/g)
        .map((s) => s.trim())
        .filter(Boolean)
    )
  )
}

const load = async () => {
  try {
    loading.value = true
    error.value = ""
    const res = await api.listDomains()
    domains.value = res.domains || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载网站列表失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const loadMeta = async () => {
  try {
    metaError.value = ""
    const ordersRes = await api.getUserOrders()
    orders.value = (ordersRes.orders || []).filter(
      (o) => o.status === "paid" && (!o.ends_at || new Date(o.ends_at) > new Date())
    )
    if (orders.value.length > 0) {
      if (!singleForm.value.orderID) singleForm.value.orderID = orders.value[0].id
      if (!batchForm.value.orderID) batchForm.value.orderID = orders.value[0].id
    }
  } catch (err: unknown) {
    metaError.value = err instanceof Error ? err.message : "加载套餐列表失败"
  }
}

const selectedSingleOrder = computed(() => orders.value.find((o) => o.id === singleForm.value.orderID))
const selectedBatchOrder = computed(() => orders.value.find((o) => o.id === batchForm.value.orderID))

const orderOptions = computed(() =>
  orders.value.map((o) => ({
    label: `${o.product_name}（${o.period === "month" ? "月付" : o.period === "quarter" ? "季付" : o.period === "year" ? "年付" : o.period}）`,
    value: o.id,
  }))
)

const openCreate = () => {
  createTab.value = "single"
  createOpen.value = true
  singleForm.value = {
    ...singleForm.value,
    name: "",
    originAddresses: "",
    cacheEnabled: true,
    http2Enabled: true,
    enabled: true,
  }
  batchForm.value = {
    ...batchForm.value,
    names: "",
    originAddresses: "",
    cacheEnabled: true,
    http2Enabled: true,
    enabled: true,
  }
  loadMeta()
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  return domains.value.filter((d) => {
    if (enabledFilter.value !== "all") {
      const enabled = Boolean(d.enabled)
      if (enabledFilter.value === "on" && !enabled) return false
      if (enabledFilter.value === "off" && enabled) return false
    }
    if (syncFilter.value !== "all") {
      const subj = `domain:${d.id}`
      const running = syncTasks.isSyncing(subj)
      const failed = syncTasks.isFailed(subj)
      if (syncFilter.value === "syncing" && !running) return false
      if (syncFilter.value === "failed" && !failed) return false
      if (syncFilter.value === "idle" && (running || failed)) return false
    }
    if (!q) return true
    const hay = [d.name, d.cname, d.origin_name, d.origin_id, d.line_group_name, d.line_group_id]
      .filter(Boolean)
      .join(" ")
      .toLowerCase()
    return hay.includes(q)
  })
})

const handleDelete = async (domain: Domain) => {
  const inst = DialogPlugin.confirm({
    header: "删除网站",
    body: `确认删除网站 ${domain.name}？该操作会同时清理 CNAME 与边缘配置，无法恢复。`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      const key = `${domain.id}-delete`
      try {
        saving.value[key] = true
        await api.deleteDomain(domain.id)
        domains.value = domains.value.filter((item) => item.id !== domain.id)
        MessagePlugin.success("删除成功")
      } catch (err: any) {
        MessagePlugin.error(err.message || "删除失败")
      } finally {
        saving.value[key] = false
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const submitCreate = async () => {
  if (createLoading.value) return
  try {
    createLoading.value = true
    const order = createTab.value === "single" ? selectedSingleOrder.value : selectedBatchOrder.value
    if (!order) {
      MessagePlugin.error("请选择套餐")
      return
    }
    const lgID = order.line_group_id
    if (!lgID) {
      MessagePlugin.error("套餐未关联可用集群")
      return
    }

    if (createTab.value === "single") {
      const names = parseList(singleForm.value.name)
      if (names.length === 0) {
        MessagePlugin.error("请输入网站域名")
        return
      }
      const originAddresses = parseList(singleForm.value.originAddresses)
      if (originAddresses.length === 0) {
        MessagePlugin.error("请输入源站地址")
        return
      }

      let ok = 0
      const errors: string[] = []
      for (const name of names) {
        try {
          const created = await api.createDomain({
            id: "",
            name,
            line_group_id: lgID,
            origin_id: "",
            cache_enabled: Boolean(singleForm.value.cacheEnabled),
            http2_enabled: Boolean(singleForm.value.http2Enabled),
          })
          await api.replaceDomainOrigins(
            created.id,
            originAddresses.map((addr) => ({ address: addr, weight: 1, enabled: true })),
          )
          if (!singleForm.value.enabled) {
            await api.updateDomain(created.id, { ...created, enabled: false })
          }
          ok++
        } catch (e: any) {
          errors.push(`${name}: ${e?.message || "创建失败"}`)
        }
      }
      if (errors.length > 0) {
        const detail = errors.join('\n')
        MessagePlugin.warning({
          content: `创建结果：成功 ${ok} 个，失败 ${errors.length} 个\n${detail}`,
          duration: 10000,
        })
        console.warn('创建网站失败:', errors)
      } else {
        MessagePlugin.success(`已创建 ${ok} 个网站`)
      }
    } else {
      const names = parseList(batchForm.value.names)
      const addresses = parseList(batchForm.value.originAddresses)
      if (names.length === 0) {
        MessagePlugin.error("请输入网站域名")
        return
      }
      if (addresses.length === 0) {
        MessagePlugin.error("请输入源站地址")
        return
      }
      const originEntries = addresses.map((addr) => ({ address: addr, weight: 1, enabled: true }))
      let ok = 0
      const errors: string[] = []
      for (const name of names) {
        try {
          const created = await api.createDomain({
            id: "",
            name,
            line_group_id: lgID,
            origin_id: "",
            cache_enabled: Boolean(batchForm.value.cacheEnabled),
            http2_enabled: Boolean(batchForm.value.http2Enabled),
          })
          await api.replaceDomainOrigins(created.id, originEntries)
          if (!batchForm.value.enabled) {
            await api.updateDomain(created.id, { ...created, enabled: false })
          }
          ok++
        } catch (e: any) {
          errors.push(`${name}: ${e?.message || "创建失败"}`)
        }
      }
      if (errors.length > 0) {
        const detail = errors.join('\n')
        MessagePlugin.warning({
          content: `批量创建结果：成功 ${ok} 个，失败 ${errors.length} 个\n${detail}`,
          duration: 10000,
        })
        console.warn('批量创建网站失败:', errors)
      } else {
        MessagePlugin.success(`已批量创建 ${ok} 个网站`)
      }
    }

    createOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err.message || "创建失败")
  } finally {
    createLoading.value = false
  }
}

const formatLineGroup = (d: Domain) => {
  const name = d.line_group_name || d.line_group_id || "-"
  const lgid = d.line_group_id ? String(d.line_group_id).slice(0, 8) : ""
  return lgid ? `${name} (id: ${lgid})` : name
}

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

const retrySync = async (row: Domain) => {
  const sync = syncTasks.bySubject(`domain:${row.id}`)
  if (!sync) return
  try {
    await api.retrySystemTask(sync.id)
    MessagePlugin.success("已重新提交同步任务")
  } catch (e: any) {
    MessagePlugin.error(e?.message || "重试失败")
  }
}

const renderCNAME = (row: Domain) => {
  const sync = syncTasks.bySubject(`domain:${row.id}`)
  if (sync?.status === "running") {
    return h("span", { class: "mono sync-pending", title: "同步中" }, [
      h("span", { class: "sync-dot sync-dot-running" }),
      " 同步中",
    ])
  }
  if (sync?.status === "failed") {
    return h("span", { class: "mono sync-failed", title: sync.message || "同步失败" }, [
      h("span", { class: "sync-dot sync-dot-failed" }),
      " 同步失败 ",
      h(ElLink,
        {
          theme: "primary",
          hover: "color",
          style: "cursor:pointer;margin-left:4px;",
          onClick: (e: Event) => {
            e.stopPropagation()
            retrySync(row)
          },
        },
        () => "重试",
      ),
    ])
  }
  return h("span", { class: "mono" }, row.cname || "-")
}

const columns = computed(() => [
  { colKey: "row-select", type: "multiple", width: 40, fixed: "left" },
  {
    colKey: "id",
    title: "ID",
    width: 60,
    cell: (_h: any, { rowIndex }: { rowIndex: number }) =>
      h("span", { class: "mono muted" }, String((rowIndex ?? 0) + 1)),
  },
  {
    colKey: "name",
    title: "域名",
    minWidth: 220,
    cell: (_h: any, { row }: { row: Domain }) =>
      h("div", { class: "stack" }, [
        h("div", { class: "title-row" }, [
          h(ElLink,
            {
              theme: "primary",
              hover: "color",
              style: "font-weight:600;cursor:pointer",
              onClick: () => router.push(`/dashboard/domains/${row.id}`),
            },
            () => row.name,
          ),
          row.https_enabled ? h("span", { class: "badge" }, "HTTPS") : null,
        ]),
        renderCNAME(row),
      ]),
  },
  {
    colKey: "origin",
    title: "源站",
    minWidth: 180,
    cell: (_h: any, { row }: { row: Domain }) =>
      h("span", { class: "mono" }, (row.origin_addresses || []).join(", ") || row.origin_id || "-"),
  },
  {
    colKey: "listen_port",
    title: "监听端口",
    width: 100,
    cell: (_h: any, { row }: { row: Domain }) =>
      h("span", { class: "mono" }, String(row.listen_port || 80)),
  },
  {
    colKey: "line_group",
    title: "套餐 / 集群",
    width: 180,
    cell: (_h: any, { row }: { row: Domain }) => {
      const name = row.line_group_name || row.line_group_id || "-"
      const lgid = row.line_group_id ? String(row.line_group_id).slice(0, 8) : ""
      return lgid ? `${name} (id: ${lgid})` : name
    },
  },
  {
    colKey: "https_enabled",
    title: "HTTPS",
    width: 90,
    cell: (_h: any, { row }: { row: Domain }) =>
      h(
        "span",
        { style: row.https_enabled ? "color:var(--app-success)" : "color:var(--app-danger)" },
        row.https_enabled ? "已启用" : "未启用",
      ),
  },
  { colKey: "updated_at", title: "更新时间", width: 180, cell: (_h: any, { row }: { row: Domain }) => h("span", { class: "mono" }, formatTime(row.updated_at)) },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    fixed: "right",
    cell: (_h: any, { row }: { row: Domain }) => {
      const delKey = `${row.id}-delete`
      const actions = [
        h(ElButton, { size: "small",
            type: "primary",
            link: true,
            onClick: () => router.push(`/dashboard/domains/${row.id}`),
          },
          () => (canDomainsWrite.value ? "管理" : "查看"),
        ),
      ]
      if (canDomainsWrite.value) {
        actions.push(
          h(ElButton, { size: "small",
              type: "danger",
              link: true,
              disabled: Boolean(saving.value[delKey]),
              onClick: () => handleDelete(row),
            },
            () => "删除",
          ),
        )
      }
      return h("div", { class: "row-actions" }, actions)
    },
  },
])

onMounted(() => {
  load()
  syncTasks.bootstrap()
  syncTasks.connect()
})
</script>
