<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">实时监控</h1>
        <p class="subtitle">流量、命中率与健康状态实时查看</p>
      </div>
      <div class="header-actions">
        <el-button :loading="metricsLoading" @click="handleRefresh">查询</el-button>
      </div>
    </div>

    <el-alert v-if="metrics?.demo_data" theme="info" class="monitor-alert">
      Elasticsearch 未配置或暂不可用，图表为零值占位。配置 ES 并采集 access.log 后将展示真实监控数据。
    </el-alert>

    <el-card class="section-card">
      <el-tabs v-model="tab">
        <el-tab-pane name="basic" label="基础数据" />
        <el-tab-pane name="quality" label="质量监控" />
        <el-tab-pane name="origin" label="回源监控" />
      </el-tabs>

      <div class="section-body">
        <div class="filters">
          <div class="filter-item">
            <el-input v-model="domainInput" placeholder="输入域名，多个空格分隔" clearable style="min-width:220px" />
          </div>
          <div class="filter-item">
            <el-input v-model="portInput" placeholder="输入监听端口" clearable style="width:140px" />
          </div>
          <div class="filter-item time-buttons">
            <el-button
              v-for="item in timeRanges"
              :key="item.value"
              size="small"
              :type="activeTimeRange === item.value ? 'primary' : 'info'"
              :variant="activeTimeRange === item.value ? 'base' : 'outline'"
              @click="selectTimeRange(item.value)"
            >
              {{ item.label }}
            </el-button>
          </div>
          <div v-if="activeTimeRange === 'custom'" class="filter-item">
            <el-input v-model="customFrom" placeholder="开始 YYYY-MM-DD HH:mm:ss" style="width:200px" />
            <span class="muted">~</span>
            <el-input v-model="customTo" placeholder="结束 YYYY-MM-DD HH:mm:ss" style="width:200px" />
          </div>
        </div>

        <LoadingState v-if="metricsLoading && seriesList.length === 0" />
        <ErrorState v-else-if="metricsError" :message="metricsError" @retry="loadMetrics" />
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
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref, watch } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type DomainHealthMetricsResponse, type DomainHealthSeries } from "@/lib/api"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const tab = ref<"basic" | "quality" | "origin">("basic")
const domainInput = ref("")
const portInput = ref("")
const metricsLoading = ref(false)
const metricsError = ref("")
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
    metricsError.value = ""
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
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载监控数据失败"
    metricsError.value = msg
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