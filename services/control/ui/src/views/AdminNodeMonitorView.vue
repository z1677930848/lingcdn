<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">实时监控</h1>
        <p class="subtitle">查看节点资源排行、监控指标趋势与节点流量（基于主控实时采集）。</p>
      </div>
      <t-button :loading="loading" @click="loadRank" class="header-refresh-btn">刷新</t-button>
      <t-tag v-if="sseConnected" theme="success" variant="light" size="small">实时</t-tag>
    </div>

    <t-card class="section-card" bordered>
      <t-tabs v-model="tab">
        <t-tab-panel value="rank" label="资源排行" />
        <t-tab-panel value="metrics" label="监控指标" />
        <t-tab-panel value="traffic" label="节点流量" />
      </t-tabs>

      <div class="panel-body">
        <div class="filters">
          <div class="filter-row">
            <span class="filter-label">指标</span>
            <t-space size="small">
              <t-button size="small" :theme="group === 'bandwidth' ? 'primary' : 'default'" :variant="group === 'bandwidth' ? 'base' : 'outline'" @click="group = 'bandwidth'">带宽</t-button>
              <t-button size="small" :theme="group === 'connect' ? 'primary' : 'default'" :variant="group === 'connect' ? 'base' : 'outline'" @click="group = 'connect'">连接</t-button>
              <t-button size="small" :theme="group === 'load' ? 'primary' : 'default'" :variant="group === 'load' ? 'base' : 'outline'" @click="group = 'load'">负载</t-button>
              <t-button size="small" :theme="group === 'disk' ? 'primary' : 'default'" :variant="group === 'disk' ? 'base' : 'outline'" @click="group = 'disk'">硬盘</t-button>
            </t-space>
          </div>
          <div class="filter-row">
            <span class="filter-label">时间</span>
            <t-space size="small">
              <t-button v-for="w in windows" :key="w.value" size="small" :theme="windowSeconds === w.value ? 'primary' : 'default'" :variant="windowSeconds === w.value ? 'base' : 'outline'" @click="windowSeconds = w.value">
                {{ w.label }}
              </t-button>
            </t-space>
          </div>
        </div>

        <div class="panel-title">{{ title }}</div>

        <div v-if="tab === 'metrics'" class="panel-grid">
          <div class="trend-header">
            <div class="trend-tip">趋势为汇总值：{{ trendLabel }}</div>
            <t-button variant="outline" size="small" :loading="seriesLoading" @click="loadSeries">刷新趋势</t-button>
          </div>
          <LineAreaChart v-if="chartValues.length" :values="chartValues" :labels="chartLabels" :height="260" />
          <div class="admin-desktop-only">
            <t-table :data="rows" :columns="columns" row-key="node_id" size="small" :loading="loading" empty="暂无数据" />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="mobile-loading"><t-loading /></div>
            <div v-else-if="rows.length === 0" class="admin-mobile-card-empty">暂无数据</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="(row, idx) in rows" :key="row.node_id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ row.hostname || row.node_id }}</span>
                  <span class="mobile-rank-badge">#{{ row.rank || idx + 1 }}</span>
                </div>
                <div class="admin-mobile-card-subtitle">{{ row.node_id }}</div>
                <div class="admin-mobile-card-rows">
                  <template v-if="group === 'bandwidth'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">网卡</span>
                      <span class="admin-mobile-card-value">{{ row.iface || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">出站带宽</span>
                      <span class="admin-mobile-card-value">{{ formatBps(row.out_bps) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">入站带宽</span>
                      <span class="admin-mobile-card-value">{{ formatBps(row.in_bps) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'connect'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">网卡</span>
                      <span class="admin-mobile-card-value">{{ row.iface || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">连接数</span>
                      <span class="admin-mobile-card-value mono">{{ (row.connections || 0).toFixed(0) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'load'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">CPU</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(row.cpu_usage) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">内存</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(row.mem_usage) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'disk'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">磁盘使用率</span>
                      <span class="admin-mobile-card-value" :class="row.disk_usage >= 90 ? 'val-danger' : row.disk_usage >= 80 ? 'val-warning' : 'val-success'">{{ formatPercent(row.disk_usage) }}</span>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-else-if="tab === 'traffic'">
          <div class="admin-desktop-only">
            <t-table :data="rows" :columns="trafficColumns" row-key="node_id" size="small" :loading="loading" empty="暂无数据" />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="mobile-loading"><t-loading /></div>
            <div v-else-if="rows.length === 0" class="admin-mobile-card-empty">暂无数据</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="(row, idx) in rows" :key="row.node_id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ row.hostname || row.node_id }}</span>
                  <span class="mobile-rank-badge">#{{ row.rank || idx + 1 }}</span>
                </div>
                <div class="admin-mobile-card-subtitle">{{ row.node_id }}</div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">出站带宽</span>
                    <span class="admin-mobile-card-value">{{ formatBps(row.out_bps) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">入站带宽</span>
                    <span class="admin-mobile-card-value">{{ formatBps(row.in_bps) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">出站流量({{ windowLabel }})</span>
                    <span class="admin-mobile-card-value">{{ formatBytes(row.delta_bytes_sent) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">入站流量({{ windowLabel }})</span>
                    <span class="admin-mobile-card-value">{{ formatBytes(row.delta_bytes_recv) }}</span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </div>
        <div v-else>
          <div class="admin-desktop-only">
            <t-table :data="rows" :columns="columns" row-key="node_id" size="small" :loading="loading" empty="暂无数据" />
          </div>
          <div class="admin-mobile-only">
            <div v-if="loading" class="mobile-loading"><t-loading /></div>
            <div v-else-if="rows.length === 0" class="admin-mobile-card-empty">暂无数据</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="(row, idx) in rows" :key="row.node_id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ row.hostname || row.node_id }}</span>
                  <span class="mobile-rank-badge">#{{ row.rank || idx + 1 }}</span>
                </div>
                <div class="admin-mobile-card-subtitle">{{ row.node_id }}</div>
                <div class="admin-mobile-card-rows">
                  <template v-if="group === 'bandwidth'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">网卡</span>
                      <span class="admin-mobile-card-value">{{ row.iface || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">出站带宽</span>
                      <span class="admin-mobile-card-value">{{ formatBps(row.out_bps) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">入站带宽</span>
                      <span class="admin-mobile-card-value">{{ formatBps(row.in_bps) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'connect'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">网卡</span>
                      <span class="admin-mobile-card-value">{{ row.iface || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">连接数</span>
                      <span class="admin-mobile-card-value mono">{{ (row.connections || 0).toFixed(0) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'load'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">CPU</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(row.cpu_usage) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">内存</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(row.mem_usage) }}</span>
                    </div>
                  </template>
                  <template v-else-if="group === 'disk'">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">磁盘使用率</span>
                      <span class="admin-mobile-card-value" :class="row.disk_usage >= 90 ? 'val-danger' : row.disk_usage >= 80 ? 'val-warning' : 'val-success'">{{ formatPercent(row.disk_usage) }}</span>
                    </div>
                  </template>
                </div>
              </div>
            </div>
          </div>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue"
import { MessagePlugin, Tag } from "tdesign-vue-next"
import { api, type NodeMonitorRankEntry, type NodeMonitorSeriesResponse } from "@/lib/api"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"
import { useMonitorSSE } from "@/composables/useMonitorSSE"

const tab = ref<"rank" | "metrics" | "traffic">("rank")
const group = ref<"bandwidth" | "connect" | "load" | "disk">("bandwidth")
const windowSeconds = ref(60)
const loading = ref(false)
const rows = ref<NodeMonitorRankEntry[]>([])
const seriesLoading = ref(false)
const series = ref<NodeMonitorSeriesResponse | null>(null)

const { data: sseData, connected: sseConnected, connect: sseConnect, disconnect: sseDisconnect } = useMonitorSSE()

const windows = [
  { label: "1分钟", value: 60 },
  { label: "5分钟", value: 300 },
  { label: "30分钟", value: 1800 },
  { label: "1小时", value: 3600 },
]

const formatBps = (bytesPerSec: number) => {
  const v = Number(bytesPerSec || 0) * 8
  if (!Number.isFinite(v) || v <= 0) return "0.0 Kbps"
  const kb = v / 1000
  if (kb < 1000) return `${kb.toFixed(1)} Kbps`
  const mb = kb / 1000
  if (mb < 1000) return `${mb.toFixed(2)} Mbps`
  const gb = mb / 1000
  return `${gb.toFixed(2)} Gbps`
}

const formatPercent = (v: number) => {
  const n = Number(v || 0)
  if (!Number.isFinite(n)) return "-"
  return `${n.toFixed(2)}%`
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

const loadRank = async () => {
  if (loading.value) return
  try {
    loading.value = true
    const res = await api.getNodeMonitorRank({ group: group.value, windowSeconds: windowSeconds.value, limit: 100 })
    rows.value = res.nodes || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载监控排行失败")
  } finally {
    loading.value = false
  }
}

const loadSeries = async () => {
  if (seriesLoading.value) return
  try {
    seriesLoading.value = true
    const metric =
      group.value === "connect"
        ? "connections"
        : group.value === "load"
        ? "cpu"
        : group.value === "disk"
        ? "disk"
        : "bandwidth_out"
    const res = await api.getNodeMonitorSeries({ metric, windowSeconds: windowSeconds.value, points: 30 })
    series.value = res
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载监控趋势失败")
  } finally {
    seriesLoading.value = false
  }
}

const nodeCell = (row: NodeMonitorRankEntry) =>
  h("div", { style: "display:flex;flex-direction:column" }, [
    h("span", { style: "font-weight:600" }, row.hostname || row.node_id),
    h("span", { style: "font-size:12px;color:#94a3b8" }, row.node_id),
  ])

const columns = computed(() => {
  if (group.value === "connect") {
    return [
      { colKey: "rank", title: "排行", width: 80 },
      { colKey: "node", title: "节点", minWidth: 240, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => nodeCell(row) },
      { colKey: "iface", title: "网卡", width: 120, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => row.iface || "-" },
      {
        colKey: "connections",
        title: "连接数",
        width: 160,
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => h("span", { class: "mono" }, (row.connections || 0).toFixed(0)),
      },
    ]
  }
  if (group.value === "load") {
    return [
      { colKey: "rank", title: "排行", width: 80 },
      { colKey: "node", title: "节点", minWidth: 240, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => nodeCell(row) },
      {
        colKey: "cpu",
        title: "CPU",
        width: 160,
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => h(Tag, { theme: "primary", variant: "light" }, () => formatPercent(row.cpu_usage)),
      },
      {
        colKey: "mem",
        title: "内存",
        width: 160,
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => h(Tag, { theme: "default", variant: "light" }, () => formatPercent(row.mem_usage)),
      },
    ]
  }
  if (group.value === "disk") {
    return [
      { colKey: "rank", title: "排行", width: 80 },
      { colKey: "node", title: "节点", minWidth: 240, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => nodeCell(row) },
      {
        colKey: "disk",
        title: "磁盘使用率",
        width: 180,
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) =>
          h(
            Tag,
            { theme: row.disk_usage >= 90 ? "danger" : row.disk_usage >= 80 ? "warning" : "success", variant: "light" },
            () => formatPercent(row.disk_usage)
          ),
      },
    ]
  }
  return [
    { colKey: "rank", title: "排行", width: 80 },
    { colKey: "node", title: "节点", minWidth: 240, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => nodeCell(row) },
    { colKey: "iface", title: "网卡", width: 120, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => row.iface || "-" },
    { colKey: "out", title: "出站带宽", width: 200, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBps(row.out_bps) },
    { colKey: "in", title: "入站带宽", width: 200, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBps(row.in_bps) },
  ]
})

const trafficColumns = computed(() => [
  { colKey: "rank", title: "排行", width: 80 },
  { colKey: "node", title: "节点", minWidth: 240, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => nodeCell(row) },
  { colKey: "out", title: "出站带宽", width: 200, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBps(row.out_bps) },
  { colKey: "in", title: "入站带宽", width: 200, cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBps(row.in_bps) },
  {
    colKey: "out_bytes",
    title: `出站流量(${windows.find((w) => w.value === windowSeconds.value)?.label || ""})`,
    width: 200,
    cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBytes(row.delta_bytes_sent),
  },
  {
    colKey: "in_bytes",
    title: `入站流量(${windows.find((w) => w.value === windowSeconds.value)?.label || ""})`,
    width: 200,
    cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBytes(row.delta_bytes_recv),
  },
])

const title = computed(() => (tab.value === "rank" ? "资源排行" : tab.value === "metrics" ? "监控指标" : "节点流量"))

const trendLabel = computed(() =>
  group.value === "bandwidth" ? "出站带宽" : group.value === "connect" ? "连接数" : group.value === "load" ? "CPU 使用率" : "磁盘使用率"
)

const windowLabel = computed(() => windows.find((w) => w.value === windowSeconds.value)?.label || "")

const chartValues = computed(() => series.value?.points?.map((p) => p.value || 0) || [])
const chartLabels = computed(() => {
  const pts = series.value?.points || []
  if (pts.length < 2) return []
  const first = pts[0]
  const last = pts[pts.length - 1]
  return [
    new Date(first.ts * 1000).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
    new Date(last.ts * 1000).toLocaleTimeString("zh-CN", { hour: "2-digit", minute: "2-digit" }),
  ]
})

watch([group, windowSeconds], () => {
  loadRank()
  if (tab.value === "metrics") loadSeries()
})

watch(tab, (next) => {
  if (next === "metrics") loadSeries()
})

onMounted(() => {
  loadRank()
  sseConnect()
})

onUnmounted(() => {
  sseDisconnect()
})

watch(sseData, (d) => {
  if (!d) return
  if (d.rank?.nodes) {
    const sorted = [...d.rank.nodes]
    sorted.sort((a, b) => {
      switch (group.value) {
        case "connect": return (b.connections || 0) - (a.connections || 0)
        case "load": return (b.cpu_usage || 0) - (a.cpu_usage || 0)
        case "disk": return (b.disk_usage || 0) - (a.disk_usage || 0)
        default: return (b.out_bps || 0) - (a.out_bps || 0)
      }
    })
    sorted.forEach((n, i) => { n.rank = i + 1 })
    rows.value = sorted
  }
  if (d.series) {
    const metricKey = group.value === "connect" ? "connections"
      : group.value === "load" ? "cpu"
      : group.value === "disk" ? "disk"
      : "bandwidth_out"
    if (d.series[metricKey]) {
      series.value = d.series[metricKey]
    }
  }
})
</script>

<style scoped>
.panel-body {
  margin-top: 12px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.filters {
  display: flex;
  align-items: center;
  gap: 16px;
  flex-wrap: wrap;
}

.filter-row {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.filter-label {
  font-size: 14px;
  color: #475569;
}

.panel-title {
  font-size: 16px;
  font-weight: 700;
}

.panel-grid {
  display: grid;
  gap: 12px;
}

.trend-header {
  display: flex;
  justify-content: space-between;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
}

.trend-tip {
  font-size: 12px;
  color: #94a3b8;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.mobile-rank-badge {
  font-size: 12px;
  font-weight: 600;
  color: #94a3b8;
  flex-shrink: 0;
}

.mobile-loading {
  text-align: center;
  padding: 32px 0;
}

.val-danger {
  color: #ef4444;
  font-weight: 600;
}

.val-warning {
  color: #f59e0b;
  font-weight: 600;
}

.val-success {
  color: #2ba471;
  font-weight: 600;
}

@media (max-width: 768px) {
  .header {
    flex-direction: column;
    align-items: flex-start;
    gap: 12px;
  }

  .header-refresh-btn {
    align-self: flex-start;
  }

  .subtitle {
    font-size: 13px;
  }

  .panel-title {
    font-size: 14px;
  }

  .filters {
    flex-direction: column;
    align-items: flex-start;
    gap: 10px;
  }

  .filter-row {
    width: 100%;
  }

  .filter-row :deep(.t-space) {
    flex-wrap: wrap;
  }
}
</style>
