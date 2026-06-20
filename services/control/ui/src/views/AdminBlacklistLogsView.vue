<template>
  <div class="admin-page">
    <ErrorState v-if="error && bans.length === 0" :message="error" @retry="loadBans" />

    <el-card class="admin-list-card">
      <el-tabs v-model="activeTab" class="admin-tabs" @tab-change="handleTabChange">
        <el-tab-pane v-for="tab in tabs" :key="tab.value" :name="tab.value" :label="tab.label">
          <template v-if="tab.value === 'stats'">
            <LoadingState v-if="loading && bans.length === 0" />
            <div v-else class="abl-stats-grid">
              <div class="abl-stat-card">
                <div class="abl-stat-label">当前拉黑</div>
                <div class="abl-stat-value">{{ stats.activeCount }}</div>
                <div class="abl-stat-sub">未过期 IP</div>
              </div>
              <div class="abl-stat-card">
                <div class="abl-stat-label">历史记录</div>
                <div class="abl-stat-value">{{ stats.expiredCount }}</div>
                <div class="abl-stat-sub">已过期 IP</div>
              </div>
              <div class="abl-stat-card">
                <div class="abl-stat-label">累计触发</div>
                <div class="abl-stat-value">{{ stats.totalStrikes }}</div>
                <div class="abl-stat-sub">拦截次数合计</div>
              </div>
              <div class="abl-stat-card">
                <div class="abl-stat-label">今日新增</div>
                <div class="abl-stat-value">{{ stats.todayCount }}</div>
                <div class="abl-stat-sub">24 小时内拉黑</div>
              </div>
            </div>
            <div v-if="stats.topReasons.length" class="abl-reason-panel">
              <div class="abl-reason-title">拦截原因 Top</div>
              <ul class="abl-reason-list">
                <li v-for="item in stats.topReasons" :key="item.reason" class="abl-reason-item">
                  <span class="abl-reason-name">{{ item.reason }}</span>
                  <span class="abl-reason-count">{{ item.count }}</span>
                </li>
              </ul>
            </div>
            <div v-else class="admin-empty">暂无统计数据</div>
          </template>

          <template v-else>
            <div class="admin-toolbar">
              <div class="admin-toolbar-left">
                <el-button
                  type="primary"
                  :disabled="!selectedRowKeys.length || unlocking"
                  :loading="unlocking"
                  @click="unlockSelected"
                >
                  解锁 IP
                </el-button>
                <el-button plain :disabled="!filteredRows.length" @click="exportBlacklist">
                  导出黑名单
                </el-button>
              </div>
              <div class="admin-toolbar-right">
                <el-button plain :loading="loading" @click="loadBans">刷新</el-button>
              </div>
            </div>

            <div class="admin-filters-row">
              <div class="admin-filters-left">
                <el-input
                  v-model="filters.ip"
                  class="admin-filter-input"
                  clearable
                  placeholder="请输入 IP 地址"
                />
                <el-input
                  v-model="filters.reason"
                  class="admin-filter-input"
                  clearable
                  placeholder="请输入拦截原因"
                />
                <el-input
                  v-model="filters.nodeId"
                  class="admin-filter-input"
                  clearable
                  placeholder="请输入来源节点"
                />
              </div>
              <el-button link type="primary" @click="clearFilters">清除</el-button>
            </div>

            <div class="admin-desktop-only">
              <LoadingState v-if="loading && bans.length === 0" />
              <EpDataTable
                v-else
                :data="filteredRows"
                :columns="columns"
                row-key="ip"
                hover
                stripe
                :loading="loading"
                :selected-row-keys="selectedRowKeys"
                :pagination="pagination"
                empty-text="暂无数据"
                @select-change="handleSelectChange"
              />
            </div>

            <div class="admin-mobile-only">
              <div v-if="loading && bans.length === 0" class="admin-loading">
                <div v-loading="true" style="min-height: 48px" />
              </div>
              <div v-else-if="filteredRows.length === 0" class="admin-mobile-card-empty">暂无数据</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="row in pagedMobileRows" :key="row.ip" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ row.ip }}</span>
                    <el-tag :type="isActive(row) ? 'danger' : 'info'" effect="light" size="small">
                      {{ isActive(row) ? "拉黑中" : "已过期" }}
                    </el-tag>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">拦截原因</span>
                      <span class="admin-mobile-card-value">{{ row.reason || "-" }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">触发次数</span>
                      <span class="admin-mobile-card-value">{{ row.strikes ?? 0 }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">来源节点</span>
                      <span class="admin-mobile-card-value">{{ row.node_id || "-" }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">拉黑时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(row.created_at) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">解锁时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(row.expires_at) }}</span>
                    </div>
                  </div>
                  <div v-if="isActive(row)" class="admin-mobile-card-actions">
                    <el-button size="small" type="primary" link @click="unlockOne(row.ip)">解锁</el-button>
                  </div>
                </div>
              </div>
              <div v-if="filteredRows.length > 0" class="admin-mobile-pagination">
                <el-pagination
                  :current-page="page"
                  :page-size="pageSize"
                  :total="filteredRows.length"
                  :page-sizes="[10, 20, 50, 100]"
                  layout="total, prev, pager, next"
                  size="small"
                  @current-change="(v: number) => { page = v }"
                  @size-change="(v: number) => { pageSize = v; page = 1 }"
                />
              </div>
            </div>
          </template>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, h, onMounted, ref, watch } from "vue"
import { ElButton, ElMessageBox } from "element-plus"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type WAFBan } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const tabs = [
  { label: "当前拉黑", value: "active" },
  { label: "拉黑统计", value: "stats" },
  { label: "历史拉黑", value: "history" },
]

const loading = ref(false)
const unlocking = ref(false)
const error = ref("")
const bans = ref<WAFBan[]>([])
const activeTab = ref("active")
const selectedRowKeys = ref<string[]>([])
const page = ref(1)
const pageSize = ref(10)

const filters = ref({
  ip: "",
  reason: "",
  nodeId: "",
})

const formatTime = (value?: string) => {
  if (!value) return "-"
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return "-"
  return d.toLocaleString("zh-CN")
}

const isActive = (row: WAFBan) => {
  if (!row.expires_at) return true
  const expires = new Date(row.expires_at)
  return !Number.isNaN(expires.getTime()) && expires.getTime() > Date.now()
}

const tabFilteredRows = computed(() => {
  if (activeTab.value === "history") {
    return bans.value.filter((row) => !isActive(row))
  }
  if (activeTab.value === "active") {
    return bans.value.filter((row) => isActive(row))
  }
  return bans.value
})

const filteredRows = computed(() => {
  const ipQ = filters.value.ip.trim().toLowerCase()
  const reasonQ = filters.value.reason.trim().toLowerCase()
  const nodeQ = filters.value.nodeId.trim().toLowerCase()
  return tabFilteredRows.value.filter((row) => {
    if (ipQ && !row.ip.toLowerCase().includes(ipQ)) return false
    if (reasonQ && !(row.reason || "").toLowerCase().includes(reasonQ)) return false
    if (nodeQ && !(row.node_id || "").toLowerCase().includes(nodeQ)) return false
    return true
  })
})

const pagedMobileRows = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredRows.value.slice(start, start + pageSize.value)
})

const pagination = computed(() => ({
  defaultCurrent: page.value,
  defaultPageSize: pageSize.value,
  pageSizeOptions: [10, 20, 50, 100],
  showJumper: true,
  total: filteredRows.value.length,
}))

const stats = computed(() => {
  const active = bans.value.filter((row) => isActive(row))
  const expired = bans.value.filter((row) => !isActive(row))
  const todayStart = new Date()
  todayStart.setHours(0, 0, 0, 0)
  const todayCount = bans.value.filter((row) => {
    if (!row.created_at) return false
    const created = new Date(row.created_at)
    return !Number.isNaN(created.getTime()) && created >= todayStart
  }).length
  const reasonMap = new Map<string, number>()
  for (const row of bans.value) {
    const reason = (row.reason || "未知").trim() || "未知"
    reasonMap.set(reason, (reasonMap.get(reason) || 0) + 1)
  }
  const topReasons = [...reasonMap.entries()]
    .map(([reason, count]) => ({ reason, count }))
    .sort((a, b) => b.count - a.count)
    .slice(0, 8)
  return {
    activeCount: active.length,
    expiredCount: expired.length,
    totalStrikes: bans.value.reduce((sum, row) => sum + (row.strikes || 0), 0),
    todayCount,
    topReasons,
  }
})

const loadBans = async () => {
  loading.value = true
  error.value = ""
  try {
    const res = await api.listWAFBans()
    bans.value = res.bans || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载拉黑日志失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleTabChange = () => {
  selectedRowKeys.value = []
  page.value = 1
}

const clearFilters = () => {
  filters.value = { ip: "", reason: "", nodeId: "" }
  page.value = 1
}

watch(
  () => [filters.value.ip, filters.value.reason, filters.value.nodeId] as const,
  () => {
    page.value = 1
  },
)

const handleSelectChange = (keys: Array<string | number>) => {
  selectedRowKeys.value = keys.map((key) => String(key))
}

const unlockOne = async (ip: string) => {
  try {
    await ElMessageBox.confirm(`确定解锁 IP ${ip} 吗？`, "解锁确认", {
      type: "warning",
      confirmButtonText: "解锁",
      cancelButtonText: "取消",
    })
  } catch {
    return
  }
  unlocking.value = true
  try {
    await api.deleteWAFBan(ip)
    MessagePlugin.success("已解锁")
    selectedRowKeys.value = selectedRowKeys.value.filter((key) => key !== ip)
    await loadBans()
  } catch (err: unknown) {
    MessagePlugin.error(err instanceof Error ? err.message : "解锁失败")
  } finally {
    unlocking.value = false
  }
}

const unlockSelected = async () => {
  if (!selectedRowKeys.value.length) return
  try {
    await ElMessageBox.confirm(
      `确定解锁选中的 ${selectedRowKeys.value.length} 个 IP 吗？`,
      "批量解锁",
      { type: "warning", confirmButtonText: "解锁", cancelButtonText: "取消" },
    )
  } catch {
    return
  }
  unlocking.value = true
  const targets = [...selectedRowKeys.value]
  let failed = 0
  try {
    for (const ip of targets) {
      try {
        await api.deleteWAFBan(ip)
      } catch {
        failed += 1
      }
    }
    if (failed > 0) {
      MessagePlugin.warning(`部分解锁失败（${failed}/${targets.length}）`)
    } else {
      MessagePlugin.success("批量解锁完成")
    }
    selectedRowKeys.value = []
    await loadBans()
  } finally {
    unlocking.value = false
  }
}

const exportBlacklist = () => {
  const rows = filteredRows.value
  if (!rows.length) {
    MessagePlugin.warning("没有可导出的数据")
    return
  }
  const header = ["IP", "拦截原因", "触发次数", "来源节点", "拉黑时间", "解锁时间", "状态"]
  const lines = rows.map((row) => [
    row.ip,
    row.reason || "",
    String(row.strikes ?? 0),
    row.node_id || "",
    formatTime(row.created_at),
    formatTime(row.expires_at),
    isActive(row) ? "拉黑中" : "已过期",
  ])
  const csv = [header, ...lines]
    .map((line) => line.map((cell) => `"${String(cell).replace(/"/g, '""')}"`).join(","))
    .join("\n")
  const blob = new Blob(["\uFEFF" + csv], { type: "text/csv;charset=utf-8;" })
  const url = URL.createObjectURL(blob)
  const link = document.createElement("a")
  link.href = url
  link.download = `blacklist-${activeTab.value}-${new Date().toISOString().slice(0, 10)}.csv`
  link.click()
  URL.revokeObjectURL(url)
  MessagePlugin.success("导出完成")
}

const columns = [
  { colKey: "row-select", type: "multiple", width: 48 },
  { colKey: "ip", title: "IP", minWidth: 140 },
  {
    colKey: "reason",
    title: "拦截原因",
    minWidth: 180,
    cell: (_h: unknown, { row }: { row: WAFBan }) => row.reason || "-",
  },
  {
    colKey: "strikes",
    title: "触发次数",
    width: 100,
    cell: (_h: unknown, { row }: { row: WAFBan }) => row.strikes ?? 0,
  },
  {
    colKey: "node_id",
    title: "来源节点",
    width: 140,
    cell: (_h: unknown, { row }: { row: WAFBan }) => row.node_id || "-",
  },
  {
    colKey: "created_at",
    title: "拉黑时间",
    width: 170,
    cell: (_h: unknown, { row }: { row: WAFBan }) => formatTime(row.created_at),
  },
  {
    colKey: "expires_at",
    title: "解锁时间",
    width: 170,
    cell: (_h: unknown, { row }: { row: WAFBan }) => formatTime(row.expires_at),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 90,
    fixed: "right" as const,
    cell: (_h: unknown, { row }: { row: WAFBan }) =>
      isActive(row)
        ? h(
            ElButton,
            { type: "primary", link: true, size: "small", onClick: () => unlockOne(row.ip) },
            () => "解锁",
          )
        : h("span", { class: "admin-table-muted" }, "-"),
  },
]

onMounted(() => {
  loadBans()
})
</script>

<style scoped>
.abl-stats-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 16px;
}

.abl-stat-card {
  padding: 16px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-card-radius-sm);
  background: var(--app-surface-muted);
}

.abl-stat-label {
  font-size: 12px;
  color: var(--app-text-muted);
}

.abl-stat-value {
  margin-top: 8px;
  font-size: 28px;
  font-weight: 600;
  color: var(--app-text-strong);
  line-height: 1.1;
}

.abl-stat-sub {
  margin-top: 6px;
  font-size: 12px;
  color: var(--app-text-faint);
}

.abl-reason-panel {
  border: 1px solid var(--app-border);
  border-radius: var(--app-card-radius-sm);
  padding: 14px 16px;
  background: var(--app-surface);
}

.abl-reason-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
  margin-bottom: 10px;
}

.abl-reason-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.abl-reason-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  font-size: 13px;
  padding: 8px 10px;
  border-radius: var(--app-card-radius-sm);
  background: var(--app-surface-muted);
}

.abl-reason-name {
  color: var(--app-text);
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.abl-reason-count {
  color: var(--app-brand-strong);
  font-weight: 600;
}

.admin-filter-input {
  width: 180px;
}

@media (max-width: 1100px) {
  .abl-stats-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .abl-stats-grid {
    grid-template-columns: 1fr;
  }

  .admin-filter-input {
    width: 100%;
  }
}
</style>
