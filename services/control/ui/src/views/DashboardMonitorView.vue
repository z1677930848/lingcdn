<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">实时监控</h1>
        <p class="subtitle">流量、命中率与健康状态实时查看</p>
      </div>
      <div class="header-actions">
        <t-button :loading="metricsLoading" @click="handleRefresh">查询</t-button>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <t-tabs v-model="tab">
        <t-tab-panel value="basic" label="基础数据" />
        <t-tab-panel value="quality" label="质量监控" />
        <t-tab-panel value="origin" label="回源监控" />
      </t-tabs>

      <div class="section-body">
        <div class="filters">
          <div class="filter-item">
            <t-input v-model="domainInput" placeholder="输入域名，多个空格分隔" clearable style="min-width:220px" />
          </div>
          <div class="filter-item">
            <t-input v-model="portInput" placeholder="输入监听端口" clearable style="width:140px" />
          </div>
          <div class="filter-item time-buttons">
            <t-button
              v-for="item in timeRanges"
              :key="item.value"
              size="small"
              :theme="activeTimeRange === item.value ? 'primary' : 'default'"
              :variant="activeTimeRange === item.value ? 'base' : 'outline'"
              @click="selectTimeRange(item.value)"
            >
              {{ item.label }}
            </t-button>
          </div>
          <div v-if="activeTimeRange === 'custom'" class="filter-item">
            <t-input v-model="customFrom" placeholder="开始 YYYY-MM-DD HH:mm:ss" style="width:200px" />
            <span class="muted">~</span>
            <t-input v-model="customTo" placeholder="结束 YYYY-MM-DD HH:mm:ss" style="width:200px" />
          </div>
        </div>

        <div v-if="metricsLoading" style="text-align:center;padding:40px 0"><t-loading /></div>
        <div v-else class="series-grid">
          <div v-if="seriesList.length === 0" class="muted" style="padding:20px 0;text-align:center">暂无数据</div>
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
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api, type DomainHealthMetricsResponse, type DomainHealthSeries } from "@/lib/api"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"

const tab = ref<"basic" | "quality" | "origin">("basic")
const domainInput = ref("")
const portInput = ref("")
const metricsLoading = ref(false)
const metrics = ref<DomainHealthMetricsResponse | null>(null)
const activeTimeRange = ref("1h")
const customFrom = ref("")
const customTo = ref("")

const timeRanges = [
  { label: "近1小时", value: "1h" },
  { label: "近6小时", value: "6h" },
  { label: "近12小时", value: "12h" },
  { label: "自定义", value: "custom" },
]

const selectTimeRange = (value: string) => {
  activeTimeRange.value = value
}

const resolveRange = () => {
  if (activeTimeRange.value === "custom") {
    const start = new Date(customFrom.value)
    const end = new Date(customTo.value)
    if (!Number.isNaN(start.getTime()) && !Number.isNaN(end.getTime()) && end > start) {
      return { fromUnix: Math.floor(start.getTime() / 1000), toUnix: Math.floor(end.getTime() / 1000) }
    }
  }
  const now = Math.floor(Date.now() / 1000)
  const hours: Record<string, number> = { "1h": 1, "6h": 6, "12h": 12 }
  const h = hours[activeTimeRange.value] || 1
  return { fromUnix: now - h * 3600, toUnix: now }
}

const parseDomains = () => {
  const raw = domainInput.value.trim()
  if (!raw) return []
  return raw.split(/[\s,;，；]+/).filter(Boolean)
}

const loadMetrics = async () => {
  if (metricsLoading.value) return
  try {
    metricsLoading.value = true
    const { fromUnix, toUnix } = resolveRange()
    const domains = parseDomains()
    const port = parseInt(portInput.value) || undefined
    const res = await api.getDomainHealthMetrics({
      group: tab.value,
      fromUnix,
      toUnix,
      domains: domains.length > 0 ? domains : undefined,
      port,
      points: 60,
    })
    metrics.value = res
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载监控数据失败")
  } finally {
    metricsLoading.value = false
  }
}

const handleRefresh = () => {
  loadMetrics()
}

const formatBps = (bps: number) => {
  const v = Number(bps || 0)
  if (!Number.isFinite(v) || v <= 0) return "0.00 Kbps"
  const kb = v / 1000
  if (kb < 1000) return `${kb.toFixed(2)} Kbps`
  const mb = kb / 1000
  if (mb < 1000) return `${mb.toFixed(2)} Mbps`
  const gb = mb / 1000
  return `${gb.toFixed(2)} Gbps`
}

const formatBytes = (b: number) => {
  const v = Number(b || 0)
  if (!Number.isFinite(v) || v <= 0) return "0.00 KB"
  const units = ["B", "KB", "MB", "GB", "TB"]
  let n = v
  let i = 0
  while (n >= 1024 && i < units.length - 1) { n /= 1024; i++ }
  return `${n.toFixed(2)} ${units[i]}`
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

const rangeLabel = computed(() => {
  const { fromUnix, toUnix } = resolveRange()
  const from = new Date(fromUnix * 1000).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
  const to = new Date(toUnix * 1000).toLocaleString("zh-CN", { month: "2-digit", day: "2-digit", hour: "2-digit", minute: "2-digit" })
  return `${from} ~ ${to}`
})

const seriesList = computed(() => metrics.value?.series || [])

onMounted(() => { loadMetrics() })
watch(tab, () => { loadMetrics() })
</script>

<style scoped>
.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.section-body {
  padding: 16px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  align-items: center;
}

.filter-item {
  display: flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: #475569;
}

.time-buttons {
  display: flex;
  gap: 0;
}

.series-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 16px;
}

.series-card {
  border: 1px solid #e2e8f0;
  border-radius: 12px;
  padding: 18px;
  background: #fff;
  transition: all 0.2s ease;
}

.series-card:hover {
  border-color: #cbd5e1;
  box-shadow: 0 4px 12px rgba(0, 0, 0, 0.05);
}

.series-head {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  margin-bottom: 12px;
  align-items: center;
}

.series-title {
  font-size: 14px;
  font-weight: 600;
  color: #475569;
}

.series-value {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  letter-spacing: -0.02em;
}

.muted {
  font-size: 12px;
  color: #94a3b8;
}

@media (max-width: 768px) {
  .filters {
    flex-direction: column;
    align-items: stretch;
  }
  .series-grid {
    grid-template-columns: 1fr;
  }
}
</style>
