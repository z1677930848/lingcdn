<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">网站管理</h1>
        <p class="subtitle">管理员视角下的所有用户网站，可批量执行启用 / 禁用 / 删除等操作</p>
      </div>
      <div class="header-actions">
        <t-button theme="primary" @click="openCreate">
          <template #icon><t-icon name="add" /></template>
          添加网站
        </t-button>
        <t-dropdown
          :options="bulkActionOptions"
          trigger="click"
          @click="onBulkAction"
        >
          <t-button variant="outline" :disabled="selectedIds.length === 0">
            批量操作（{{ selectedIds.length }}）
            <template #suffix><t-icon name="chevron-down" /></template>
          </t-button>
        </t-dropdown>
        <t-button variant="outline" @click="exportCSV">
          <template #icon><t-icon name="download" /></template>
          导出
        </t-button>
        <t-input v-model="search" clearable placeholder="搜索域名 / CNAME / 源站" style="width:240px" />
        <t-select v-model="enabledFilter" :options="enabledOptions" style="width:120px" />
        <t-select v-model="syncFilter" :options="syncOptions" style="width:140px" />
        <t-button variant="text" :loading="loading" @click="load">
          <template #icon><t-icon name="refresh" /></template>
          刷新
        </t-button>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <div class="admin-desktop-only">
        <t-table
          :data="filtered"
          :columns="columns"
          row-key="id"
          size="small"
          bordered
          :loading="loading"
          empty="暂无网站"
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
        <div v-if="loading" style="text-align:center;padding:32px 0"><t-loading /></div>
        <div v-else-if="filtered.length === 0" class="admin-mobile-card-empty">暂无网站</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="item in filtered" :key="item.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ item.name }}</div>
              <div class="admin-mobile-card-tags">
                <t-tag v-if="item.https_enabled" size="small" theme="primary">HTTPS</t-tag>
              </div>
            </div>
            <div class="admin-mobile-card-subtitle">
              <template v-if="syncTasks.isSyncing(`domain:${item.id}`)">
                <span class="sync-dot sync-dot-running"></span> 同步中
              </template>
              <template v-else-if="syncTasks.isFailed(`domain:${item.id}`)">
                <span class="sync-dot sync-dot-failed"></span>
                <span :title="syncTasks.bySubject(`domain:${item.id}`)?.message || ''">同步失败</span>
                <t-link theme="primary" hover="color" style="margin-left:4px;cursor:pointer" @click="retrySync(item)">重试</t-link>
              </template>
              <template v-else>{{ item.cname || '-' }}</template>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">源站</span>
                <span class="admin-mobile-card-value">{{ (item.origin_addresses || []).join(', ') || item.origin_id || '-' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">套餐 / 集群</span>
                <span class="admin-mobile-card-value">{{ formatLineGroup(item) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">HTTPS</span>
                <span class="admin-mobile-card-value" :style="{ color: item.https_enabled ? '#00a870' : '#ef4444' }">
                  {{ item.https_enabled ? '已启用' : '未启用' }}
                </span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">更新时间</span>
                <span class="admin-mobile-card-value">{{ formatTime(item.updated_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <t-button size="small" theme="primary" variant="text" @click="router.push(`/admin/dashboard/domains/${item.id}`)">管理</t-button>
              <t-button size="small" theme="danger" variant="text" :disabled="Boolean(saving[`${item.id}-delete`])" @click="handleDelete(item)">删除</t-button>
            </div>
          </div>
        </div>
      </div>
    </t-card>

    <t-dialog
      v-model:visible="createOpen"
      header="添加网站"
      :confirm-btn="{ content: '提交', loading: createLoading }"
      cancel-btn="取消"
      @confirm="submitCreate"
    >
      <t-tabs v-model="createTab">
        <t-tab-panel value="single" label="单个添加">
          <div class="form-stack">
            <div class="form-row">
              <div class="form-label">产品套餐</div>
              <t-select v-model="singleForm.productID" :options="productOptions" />
            </div>
            <div class="form-row">
              <div class="form-label">网站域名</div>
              <t-input v-model="singleForm.name" />
            </div>
            <div class="form-row align-top">
              <div class="form-label">源站地址</div>
              <t-textarea v-model="singleForm.originAddresses" :autosize="true" placeholder="IP 或域名，多个用空格 / 逗号 / 换行分隔" />
            </div>
          </div>
        </t-tab-panel>
        <t-tab-panel value="batch" label="批量添加">
          <div class="form-stack">
            <div class="form-row">
              <div class="form-label">产品套餐</div>
              <t-select v-model="batchForm.productID" :options="productOptions" />
            </div>
            <div class="form-row align-top">
              <div class="form-label">网站域名</div>
              <t-textarea v-model="batchForm.names" :autosize="true" />
            </div>
            <div class="form-row align-top">
              <div class="form-label">源站地址</div>
              <t-textarea v-model="batchForm.originAddresses" :autosize="true" />
            </div>
          </div>
        </t-tab-panel>
      </t-tabs>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { DialogPlugin, MessagePlugin, Button, Link } from "tdesign-vue-next"
import { api, type Domain, type Cluster, type Product } from "@/lib/api"
import { useSyncTasks } from "@/composables/useSyncTasks"

const router = useRouter()

const syncTasks = useSyncTasks("domain:")

const loading = ref(false)
const domains = ref<Domain[]>([])
const search = ref("")
const enabledFilter = ref("all")
const syncFilter = ref<"all" | "syncing" | "failed" | "idle">("all")
const saving = ref<Record<string, boolean>>({})

// --- Bulk selection + bulk actions (mirrors the CDNfly list toolbar) ---
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
        // ACME 默认走 CNAME + DNS-01/HTTP-01 流程
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

// Export the current (post-filter) list as CSV so operators can feed it
// into spreadsheets / audit tools without a dedicated backend endpoint.
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
const lineGroups = ref<Cluster[]>([])
const products = ref<Product[]>([])

const singleForm = ref({
  name: "",
  productID: "",
  originAddresses: "",
  cacheEnabled: true,
  http2Enabled: true,
  enabled: true,
})

const batchForm = ref({
  names: "",
  productID: "",
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
    const res = await api.listDomains()
    domains.value = res.domains || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载网站列表失败")
  } finally {
    loading.value = false
  }
}

const loadMeta = async () => {
  try {
    const [lgRes, prodRes] = await Promise.all([api.listClusters(), api.getProducts()])
    lineGroups.value = lgRes.clusters || []
    products.value = (prodRes.products || []).filter(p => p.enabled)
    if (products.value.length > 0) {
      if (!singleForm.value.productID) singleForm.value.productID = products.value[0].id
      if (!batchForm.value.productID) batchForm.value.productID = products.value[0].id
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载产品列表失败")
  }
}

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
  // Admin-side delete is even more dangerous than the user-side one:
  // it can remove another operator's domain, so confirm + name match
  // is the floor for safety.
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
    const productID = createTab.value === "single" ? singleForm.value.productID : batchForm.value.productID
    if (!productID) {
      MessagePlugin.error("请选择产品套餐")
      return
    }
    // Resolve line_group_id from selected product
    const selectedProduct = products.value.find(p => p.id === productID)
    const lgID = selectedProduct?.line_group_id || selectedProduct?.cluster_id || ""
    if (!lgID) {
      MessagePlugin.error("所选产品未关联可用集群")
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
      // In batch mode every domain gets the same origin list (what the
      // operator typed). We PUT domain_origins after the domain is
      // created rather than creating a shared global origin —— each
      // row now owns its own copy per the new design.
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
      h(
        Link,
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

const productOptions = computed(() => (products.value || []).map((p) => ({ label: p.name || p.id, value: p.id })))

const columns = computed(() => [
  { colKey: "row-select", type: "multiple", width: 40, fixed: "left" },
  {
    colKey: "id",
    title: "ID",
    width: 60,
    cell: (_h: any, { rowIndex }: { rowIndex: number }) =>
      h("span", { class: "mono", style: "color:#888" }, String((rowIndex ?? 0) + 1)),
  },
  {
    colKey: "name",
    title: "域名",
    minWidth: 220,
    cell: (_h: any, { row }: { row: Domain }) =>
      h("div", { class: "stack" }, [
        h("div", { class: "title-row" }, [
          h(
            Link,
            {
              theme: "primary",
              hover: "color",
              style: "font-weight:600;cursor:pointer",
              onClick: () => router.push(`/admin/dashboard/domains/${row.id}`),
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
        { style: row.https_enabled ? "color:#00a870" : "color:#ef4444" },
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
      return h("div", { class: "row-actions" }, [
        h(
          Button,
          {
            size: "small",
            theme: "primary",
            variant: "text",
            onClick: () => router.push(`/admin/dashboard/domains/${row.id}`),
          },
          () => "管理",
        ),
        h(
          Button,
          {
            size: "small",
            theme: "danger",
            variant: "text",
            disabled: Boolean(saving.value[delKey]),
            onClick: () => handleDelete(row),
          },
          () => "删除",
        ),
      ])
    },
  },
])

onMounted(() => {
  load()
  syncTasks.bootstrap()
  syncTasks.connect()
})
</script>

<style scoped>
.header-actions {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}

.page {
  min-width: 0;
}

.sync-pending {
  color: #94a3b8;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.sync-failed {
  color: #ef4444;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}

.sync-dot {
  display: inline-block;
  width: 8px;
  height: 8px;
  border-radius: 50%;
  vertical-align: middle;
}

.sync-dot-running {
  background: #635BFF;
  animation: sync-pulse 1.2s ease-in-out infinite;
}

.sync-dot-failed {
  background: #ef4444;
}

@keyframes sync-pulse {
  0%, 100% {
    opacity: 0.35;
    transform: scale(0.9);
  }
  50% {
    opacity: 1;
    transform: scale(1.1);
  }
}

.section-card {
  min-width: 0;
}

.section-card :deep(.t-table) {
  min-width: 100%;
}

.row-actions {
  display: flex;
  gap: 4px;
  align-items: center;
}

.form-stack {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-top: 12px;
}

.form-row {
  display: grid;
  grid-template-columns: 120px 1fr;
  gap: 10px;
  align-items: center;
}

.form-row.align-top {
  align-items: start;
}

.form-label {
  color: #475569;
}

.form-inline {
  display: flex;
  gap: 8px;
  align-items: center;
  flex-wrap: wrap;
}

.form-switches {
  display: flex;
  gap: 18px;
  align-items: center;
  flex-wrap: wrap;
}

.switch-item {
  display: flex;
  align-items: center;
  gap: 8px;
}

.form-tip {
  font-size: 12px;
  color: #94a3b8;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  color: #94a3b8;
}

.stack {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.badge {
  display: inline-flex;
  align-items: center;
  padding: 2px 8px;
  border-radius: 8px;
  font-size: 12px;
  background: rgba(99, 91, 255, 0.12);
  color: #635BFF;
}

@media (max-width: 768px) {
  .header-actions {
    width: 100%;
  }

  .header-actions > * {
    width: 100% !important;
    min-width: 0 !important;
  }

  .header-actions :deep(.t-button) {
    min-height: 44px;
  }

  .form-row {
    grid-template-columns: 1fr;
    gap: 6px;
  }

  .form-inline {
    flex-direction: column;
    align-items: stretch;
  }

  .form-inline > * {
    width: 100% !important;
    min-width: 0 !important;
  }
}
</style>
