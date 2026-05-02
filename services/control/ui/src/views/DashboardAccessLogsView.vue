<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">访问日志</h1>
        <p class="subtitle">查询请求日志与请求明细</p>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="filters">
          <t-input v-model="domain" placeholder="域名" clearable style="width:200px" />
          <t-input v-model="clientIP" placeholder="客户端 IP" clearable style="width:160px" />
          <t-select v-model="statusCode" placeholder="状态码" clearable style="width:120px" :options="statusOptions" />
          <t-input v-model="keyword" placeholder="关键词搜索" clearable style="width:180px" />
          <div class="filter-time">
            <t-input v-model="timeFrom" placeholder="开始时间" style="width:180px" />
            <span class="muted">~</span>
            <t-input v-model="timeTo" placeholder="结束时间" style="width:180px" />
          </div>
          <t-button theme="primary" :loading="loading" @click="handleSearch">查询</t-button>
        </div>

        <div v-if="!esAvailable" class="notice-box">
          Elasticsearch 未配置，日志查询功能不可用。请联系管理员配置。
        </div>

        <template v-else>
          <div class="admin-desktop-only">
            <t-table
              :data="logs"
              :columns="columns"
              row-key="id"
              size="small"
              :loading="loading"
              empty="暂无日志"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" style="text-align:center;padding:32px 0"><t-loading /></div>
            <div v-else-if="logs.length === 0" class="admin-mobile-card-empty">暂无日志</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="(log, idx) in logs" :key="idx" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ log.domain || "-" }}</span>
                  <t-tag :theme="statusTheme(log.status)" variant="light" size="small">{{ log.status || "-" }}</t-tag>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">路径</span>
                    <span class="admin-mobile-card-value log-path">{{ log.path || "-" }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">IP</span>
                    <span class="admin-mobile-card-value">{{ log.client_ip || "-" }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">时间</span>
                    <span class="admin-mobile-card-value" :title="formatTimeFull(log.timestamp)">{{ formatTime(log.timestamp) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div v-if="total > 0" class="result-info">
            共 {{ total.toLocaleString("zh-CN") }} 条结果
          </div>
        </template>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { h, onMounted, ref } from "vue"
import { MessagePlugin, Tag } from "tdesign-vue-next"
import { api } from "@/lib/api"
import { formatTime, formatTimeFull } from "@/lib/time"

interface LogEntry {
  domain?: string
  path?: string
  method?: string
  status?: number
  client_ip?: string
  bytes?: number
  timestamp?: string
  ua?: string
}

const domain = ref("")
const clientIP = ref("")
const statusCode = ref("")
const keyword = ref("")
const timeFrom = ref("")
const timeTo = ref("")
const loading = ref(false)
const esAvailable = ref(true)
const logs = ref<LogEntry[]>([])
const total = ref(0)

const statusOptions = [
  { label: "2xx", value: "2xx" },
  { label: "3xx", value: "3xx" },
  { label: "4xx", value: "4xx" },
  { label: "5xx", value: "5xx" },
]

const statusTheme = (status?: number) => {
  if (!status) return "default"
  if (status >= 500) return "danger"
  if (status >= 400) return "warning"
  if (status >= 300) return "primary"
  return "success"
}

const checkES = async () => {
  try {
    const res = await api.checkESHealth()
    esAvailable.value = res.status === 200 || !!res.body
  } catch {
    esAvailable.value = false
  }
}

const handleSearch = async () => {
  if (!esAvailable.value) return
  loading.value = true
  try {
    const res = await api.searchLogs({
      domain: domain.value || undefined,
      ip: clientIP.value || undefined,
      status: statusCode.value || undefined,
      query: keyword.value || undefined,
      from: timeFrom.value || undefined,
      to: timeTo.value || undefined,
    })
    logs.value = res.hits || []
    total.value = res.total || 0
  } catch (err: any) {
    MessagePlugin.error(err.message || "查询日志失败")
  } finally {
    loading.value = false
  }
}

const columns = [
  { colKey: "timestamp", title: "时间", width: 170 },
  { colKey: "domain", title: "域名", minWidth: 160 },
  { colKey: "method", title: "方法", width: 80 },
  { colKey: "path", title: "路径", minWidth: 200, ellipsis: true },
  {
    colKey: "status",
    title: "状态",
    width: 90,
    cell: (_h: any, { row }: { row: LogEntry }) => {
      const s = row.status
      return h(Tag, { theme: statusTheme(s), variant: "light", size: "small" }, () => s || "-")
    },
  },
  { colKey: "client_ip", title: "客户端IP", width: 140 },
  { colKey: "bytes", title: "字节数", width: 100 },
]

onMounted(async () => {
  await checkES()
  if (esAvailable.value) {
    handleSearch()
  }
})
</script>

<style scoped>
.section-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  align-items: center;
}

.filter-time {
  display: flex;
  align-items: center;
  gap: 6px;
}

.notice-box {
  padding: 14px 16px;
  border-radius: 8px;
  background: #fff7e8;
  border: 1px solid #ffd591;
  color: #8c5412;
  font-size: 14px;
}

.result-info {
  font-size: 13px;
  color: #94a3b8;
  text-align: right;
}

.muted {
  color: #94a3b8;
}

.log-path {
  word-break: break-all;
}

@media (max-width: 768px) {
  .filters {
    flex-direction: column;
    align-items: stretch;
  }
  .filter-time {
    flex-direction: column;
    align-items: stretch;
  }
}
</style>
