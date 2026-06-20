<template>
  <div class="admin-page">
    <ErrorState v-if="error && logs.length === 0" :message="error" @retry="loadData" />

    <el-card class="admin-list-card">
      <el-tabs v-model="activeTab" class="admin-tabs" @tab-change="handleTabChange">
        <el-tab-pane v-for="tabItem in tabs" :key="tabItem.value" :name="tabItem.value" :label="tabItem.label">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <EpSelect v-model="status" :options="statusOptions" class="admin-filter-select-large" />
            </div>
            <div class="admin-toolbar-right">
              <el-input v-model="query" class="admin-search-input" clearable placeholder="搜索" />
              <el-button plain @click="loadData">刷新</el-button>
            </div>
          </div>

          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <LoadingState v-if="loading && logs.length === 0" />
            <el-empty v-else-if="logs.length === 0" description="暂无日志" class="admin-empty" />
            <EpDataTable
              v-else
              :data="logs"
              :columns="columns"
              row-key="id"
              hover
              stripe
              :loading="loading"
              :pagination="pagination"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="logs.length === 0 && !loading" class="admin-mobile-card-empty">
              暂无日志
            </div>
            <div v-else class="admin-mobile-cards">
              <div v-for="row in logs" :key="row.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">{{ row.username || row.user_id || '-' }}</div>
                  <div class="admin-mobile-card-tags">
                    <el-tag
                      :type="row.status === 'failed' ? 'danger' : 'success'"
                      effect="light"
                      size="small"
                    >
                      {{ row.status === 'failed' ? '失败' : '成功' }}
                    </el-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-subtitle">{{ row.message || '-' }}</div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">IP</span>
                    <span class="admin-mobile-card-value">{{ row.ip || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">位置</span>
                    <span class="admin-mobile-card-value">{{ row.location || '--' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.created_at) }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="total > 0" class="admin-mobile-pagination">
              <el-pagination
                :current="page"
                :page-size="pageSize"
                :total="total"
                :page-size-options="[10, 20, 50, 100]"
                show-page-size
                size="small"
                @current-change="(v: number) => { page = v; loadData() }"
                @page-size-change="(v: number) => { pageSize = v; page = 1; loadData() }"
              />
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
import { ElTag } from "element-plus"
import { api, type SystemLog } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const tabs = [
  { label: "登录日志", value: "login" },
  { label: "操作日志", value: "action" },
  { label: "备份日志", value: "backup" },
  { label: "邮件日志", value: "email" },
]

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "成功", value: "success" },
  { label: "失败", value: "failed" },
]

const logs = ref<SystemLog[]>([])
const loading = ref(true)
const error = ref("")
const activeTab = ref("login")
const status = ref("all")
const query = ref("")
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

const loadData = async () => {
  try {
    loading.value = true
    const res = await api.getSystemLogs({
      type: activeTab.value,
      status: status.value === "all" ? undefined : status.value,
      q: query.value.trim() || undefined,
      page: page.value,
      pageSize: pageSize.value,
    })
    logs.value = res.logs || []
    total.value = res.total || 0
    error.value = ""
  } catch (err: any) {
    const msg = err.message || "加载日志失败"
    error.value = msg
    MessagePlugin.error(msg)
  } finally {
    loading.value = false
  }
}

const renderStatusTag = (value: string) => {
  if (value === "failed") return h(ElTag, { type: "danger" }, () => "失败")
  return h(ElTag, { type: "success" }, () => "成功")
}

const columns = computed(() => [
  {
    colKey: "username",
    title: "用户",
    minWidth: 140,
    cell: (_h: any, { row }: { row: SystemLog }) => h("span", { class: "admin-table-muted" }, row.username || row.user_id || "-"),
  },
  { colKey: "ip", title: "IP", width: 140, cell: (_h: any, { row }: { row: SystemLog }) => h("span", { class: "admin-table-muted" }, row.ip || "-") },
  { colKey: "location", title: "位置", minWidth: 200, cell: (_h: any, { row }: { row: SystemLog }) => h("span", { class: "admin-table-muted" }, row.location || "--") },
  { colKey: "message", title: "内容", minWidth: 200, cell: (_h: any, { row }: { row: SystemLog }) => h("span", { class: "admin-table-muted" }, row.message || "-") },
  {
    colKey: "created_at",
    title: "时间",
    width: 180,
    cell: (_h: any, { row }: { row: SystemLog }) => h("span", { class: "admin-table-muted" }, formatDateTime(row.created_at)),
  },
  { colKey: "status", title: "状态", width: 120, cell: (_h: any, { row }: { row: SystemLog }) => renderStatusTag(row.status) },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pi: { current: number; pageSize: number }) => {
    page.value = pi.current
    pageSize.value = pi.pageSize
    loadData()
  },
}))

const handleTabChange = (value: string) => {
  activeTab.value = value
  page.value = 1
  loadData()
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

onMounted(() => {
  loadData()
})
</script>
