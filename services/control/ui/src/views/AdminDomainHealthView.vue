<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">网站监控</h1>
        <p class="subtitle">按域名查看基础数据、质量与回源指标，并支持排行筛选。</p>
      </div>
      <div class="header-actions">
        <el-button :loading="tab === 'rank' ? rankLoading : metricsLoading" @click="handleRefresh">刷新</el-button>
      </div>
    </div>

    <el-card class="section-card">
      <el-tabs v-model="tab">
        <el-tab-pane name="basic" label="基础数据" />
        <el-tab-pane name="quality" label="质量监控" />
        <el-tab-pane name="origin" label="回源监控" />
        <el-tab-pane name="rank" label="数据排行" />
      </el-tabs>

      <div class="section-body">
        <div class="filters">
          <div class="filter-item">
            <span>域名</span>
            <EpSelect
              v-model="selectedDomains"
              :options="domainOptions"
              multiple
              filterable
              clearable
              :loading="domainLoading"
              style="min-width:240px"
            />
          </div>
          <div class="filter-item">
            <span>时间</span>
            <div class="range-inputs">
              <el-input v-model="dateRange[0]" placeholder="开始时间 YYYY-MM-DD HH:mm:ss" />
              <span class="muted">~</span>
              <el-input v-model="dateRange[1]" placeholder="结束时间 YYYY-MM-DD HH:mm:ss" />
            </div>
          </div>
          <div v-if="tab === 'rank'" class="filter-item">
            <span>排行指标</span>
            <div class="rank-buttons">
              <el-button
                v-for="item in rankMetrics"
                :key="item.value"
                size="small"
                :type="rankMetric === item.value ? 'primary' : 'info'"
                :variant="rankMetric === item.value ? 'base' : 'outline'"
                @click="rankMetric = item.value"
              >
                {{ item.label }}
              </el-button>
            </div>
          </div>
        </div>

        <el-alert v-if="domainError" type="warning" :message="domainError" />

        <template v-if="tab === 'rank'">
          <LoadingState v-if="rankLoading && rankItems.length === 0" />
          <ErrorState v-else-if="rankError" :message="rankError" @retry="loadRank" />
          <template v-else>
          <div class="admin-desktop-only">
            <EpDataTable
              :data="rankItems"
              :columns="rankColumns"
              row-key="domain"
              size="small"
              :loading="rankLoading"
              empty-text="暂无数据"
            />
          </div>
          <div class="admin-mobile-only">
            <div v-if="rankLoading" style="text-align:center;padding:32px 0"><div v-loading="true" style="min-height:48px" /></div>
            <div v-else-if="rankItems.length === 0" class="admin-mobile-card-empty">暂无数据</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="item in rankItems" :key="item.domain" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">#{{ item.rank }} {{ item.domain }}</div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">带宽</span>
                    <span class="admin-mobile-card-value">{{ formatBps(item.bandwidth_bps) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">流量</span>
                    <span class="admin-mobile-card-value">{{ formatBytes(item.bytes) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">请求数</span>
                    <span class="admin-mobile-card-value">{{ (item.requests || 0).toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">QPS</span>
                    <span class="admin-mobile-card-value">{{ Number(item.qps || 0).toFixed(2) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">4xx</span>
                    <span class="admin-mobile-card-value">{{ (item.http_4xx || 0).toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">5xx</span>
                    <span class="admin-mobile-card-value">{{ (item.http_5xx || 0).toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">错误率</span>
                    <span class="admin-mobile-card-value">{{ formatPercent(item.error_rate) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
          </template>
        </template>
        <template v-else>
          <LoadingState v-if="metricsLoading && seriesList.length === 0" />
          <ErrorState v-else-if="metricsError" :message="metricsError" @retry="loadMetrics" />
          <div v-else class="series-grid">
          <div v-if="seriesList.length === 0" class="muted">暂无数据</div>
          <div v-for="series in seriesList" :key="series.key" class="series-card">
            <div class="series-head">
              <div>
                <div class="series-title">{{ series.name }}</div>
                <div class="series-value">{{ formatMetricValue(latestValue(series), series.unit) }}</div>
              </div>
              <div class="muted">{{ rangeLabel }}</div>
            </div>
            <LineAreaChart :values="seriesValues(series)" :labels="seriesLabels(series)" :height="220" />
          </div>
          </div>
        </template>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, onMounted, ref, watch } from "vue"
import { api, type Domain, type DomainHealthMetricsResponse, type DomainHealthRankEntry, type DomainHealthSeries } from "@/lib/api"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const tab = ref<"basic" | "quality" | "origin" | "rank">("basic")
const domains = ref<Domain[]>([])
const domainLoading = ref(false)
const selectedDomains = ref<string[]>([])
const dateRange = ref<[string, string]>((() => {
  const end = new Date()
  const start = new Date(end.getTime() - 60 * 60 * 1000)
  return [formatDateTime(start), formatDateTime(end)]
})())
const metricsLoading = ref(false)
const metricsError = ref("")
const metrics = ref<DomainHealthMetricsResponse | null>(null)
const rankLoading = ref(false)
const rankError = ref("")
const rankItems = ref<DomainHealthRankEntry[]>([])
const domainError = ref("")
const rankMetric = ref("bandwidth")

const rankMetrics = [
  { label: "带宽", value: "bandwidth" },
  { label: "流量", value: "traffic" },
  { label: "请求数", value: "requests" },
  { label: "QPS", value: "qps" },
  { label: "4xx", value: "4xx" },
  { label: "5xx", value: "5xx" },
  { label: "错误率", value: "error_rate" },
]

const domainOptions = computed(() => (domains.value || []).map((d) => ({ label: d.name, value: d.name })))

const rangeLabel = computed(() => `${dateRange.value[0] || "-"} ~ ${dateRange.value[1] || "-"}`)

const parseDate = (value: string) => {
  const d = new Date(value)
  return Number.isNaN(d.getTime()) ? null : d
}

const resolveRange = () => {
  const start = parseDate(dateRange.value[0])
  const end = parseDate(dateRange.value[1])
  if (!start || !end || end < start) {
    const fallbackEnd = new Date()
    const fallbackStart = new Date(fallbackEnd.getTime() - 60 * 60 * 1000)
    return { fromUnix: Math.floor(fallbackStart.getTime() / 1000), toUnix: Math.floor(fallbackEnd.getTime() / 1000) }
  }
  return { fromUnix: Math.floor(start.getTime() / 1000), toUnix: Math.floor(end.getTime() / 1000) }
}

const loadDomains = async () => {
  try {
    domainLoading.value = true
    domainError.value = ""
    const res = await api.listDomains()
    domains.value = res.domains || []
  } catch (err: unknown) {
    domainError.value = err instanceof Error ? err.message : "加载网站列表失败"
  } finally {
    domainLoading.value = false
  }
}

const loadMetrics = async () => {
  if (metricsLoading.value || tab.value === "rank") return
  try {
    metricsLoading.value = true
    metricsError.value = ""
    const { fromUnix, toUnix } = resolveRange()
    const res = await api.getDomainHealthMetrics({
      group: tab.value,
      fromUnix,
      toUnix,
      domains: selectedDomains.value,
      points: 60,
    })
    metrics.value = res
  } catch (err: unknown) {
    metricsError.value = err instanceof Error ? err.message : "加载健康指标失败"
  } finally {
    metricsLoading.value = false
  }
}

const loadRank = async () => {
  if (rankLoading.value || tab.value !== "rank") return
  try {
    rankLoading.value = true
    rankError.value = ""
    const { fromUnix, toUnix } = resolveRange()
    const res = await api.getDomainHealthRank({
      metric: rankMetric.value,
      fromUnix,
      toUnix,
      domains: selectedDomains.value,
      limit: 100,
    })
    rankItems.value = res.items || []
  } catch (err: unknown) {
    rankError.value = err instanceof Error ? err.message : "加载数据排行失败"
  } finally {
    rankLoading.value = false
  }
}

const handleRefresh = () => {
  if (tab.value === "rank") {
    loadRank()
  } else {
    loadMetrics()
  }
}

const formatBps = (bps: number) => {
  const v = Number(bps || 0)
  if (!Number.isFinite(v) || v <= 0) return "0 Kbps"
  const kb = v / 1000
  if (kb < 1000) return `${kb.toFixed(1)} Kbps`
  const mb = kb / 1000
  if (mb < 1000) return `${mb.toFixed(2)} Mbps`
  const gb = mb / 1000
  return `${gb.toFixed(2)} Gbps`
}

const formatBytes = (b: number) => {
  const v = Number(b || 0)
  if (!Number.isFinite(v) || v <= 0) return "0 B"
  const units = ["B", "KB", "MB", "GB", "TB"]
  let n = v
  let i = 0
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024
    i++
  }
  return `${n.toFixed(i === 0 ? 0 : 2)} ${units[i]}`
}

const formatPercent = (ratio: number) => {
  const v = Number(ratio || 0)
  if (!Number.isFinite(v)) return "-"
  return `${(v * 100).toFixed(2)}%`
}

const formatMetricValue = (value: number, unit: string) => {
  if (!Number.isFinite(value)) return "-"
  if (unit === "bps") return formatBps(value)
  if (unit === "bytes") return formatBytes(value)
  if (unit === "ratio") return formatPercent(value)
  if (unit === "qps") return value.toFixed(2)
  if (unit === "count") return Math.round(value).toLocaleString("zh-CN")
  return value.toFixed(2)
}

const latestValue = (series: DomainHealthSeries) => {
  const points = series.points || []
  const last = points[points.length - 1]
  return last ? Number(last.value || 0) : 0
}

const seriesValues = (series: DomainHealthSeries) => (series.points || []).map((p) => Number(p.value || 0))

const seriesLabels = (series: DomainHealthSeries) => {
  const points = series.points || []
  const first = points[0]
  const last = points[points.length - 1]
  return first && last
    ? [
        new Date(first.ts * 1000).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }),
        new Date(last.ts * 1000).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" }),
      ]
    : ["-", "-"]
}

const seriesList = computed(() => metrics.value?.series || [])

const rankColumns = computed(() => [
  { colKey: "rank", title: "排行", width: 80 },
  { colKey: "domain", title: "域名", minWidth: 240 },
  { colKey: "bandwidth_bps", title: "带宽", width: 140, cell: (_h: any, { row }: { row: any }) => formatBps(row.bandwidth_bps) },
  { colKey: "bytes", title: "流量", width: 140, cell: (_h: any, { row }: { row: any }) => formatBytes(row.bytes) },
  { colKey: "requests", title: "请求数", width: 120, cell: (_h: any, { row }: { row: any }) => (row.requests || 0).toLocaleString("zh-CN") },
  { colKey: "qps", title: "QPS", width: 120, cell: (_h: any, { row }: { row: any }) => Number(row.qps || 0).toFixed(2) },
  { colKey: "http_4xx", title: "4xx", width: 100, cell: (_h: any, { row }: { row: any }) => (row.http_4xx || 0).toLocaleString("zh-CN") },
  { colKey: "http_5xx", title: "5xx", width: 100, cell: (_h: any, { row }: { row: any }) => (row.http_5xx || 0).toLocaleString("zh-CN") },
  { colKey: "error_rate", title: "错误率", width: 120, cell: (_h: any, { row }: { row: any }) => formatPercent(row.error_rate) },
])

function formatDateTime(date: Date) {
  return date.toLocaleString("zh-CN")
}

onMounted(() => {
  loadDomains()
  handleRefresh()
})

watch([tab, selectedDomains, () => dateRange.value[0], () => dateRange.value[1]], () => {
  handleRefresh()
})

watch(rankMetric, () => {
  if (tab.value === "rank") loadRank()
})
</script>

