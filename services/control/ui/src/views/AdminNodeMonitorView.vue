<template>
  <div class="page node-monitor-page">
    <div class="header">
      <div>
        <h1 class="title">实时监控</h1>
      </div>
      <div class="header-actions node-monitor-header-actions">
        <el-tag v-if="sseConnected && livePushEnabled" type="success" effect="light" size="small">实时推送</el-tag>
        <el-tag v-else-if="sseConnected" type="info" effect="light" size="small">SSE 已连接</el-tag>
        <el-tag v-else type="info" effect="plain" size="small">轮询模式</el-tag>
        <span v-if="lastUpdatedAt" class="node-monitor-updated">更新于 {{ lastUpdatedAt }}</span>
        <el-button plain :loading="loading" @click="refreshAll">刷新</el-button>
      </div>
    </div>

    <el-card class="section-card node-monitor-card" shadow="never">
      <div class="node-monitor-toolbar">
        <div class="filter-row">
          <span class="filter-label">指标</span>
          <el-radio-group v-model="group" size="small">
            <el-radio-button label="bandwidth">带宽</el-radio-button>
            <el-radio-button label="connect">连接</el-radio-button>
            <el-radio-button label="load">负载</el-radio-button>
            <el-radio-button label="disk">硬盘</el-radio-button>
          </el-radio-group>
        </div>
        <div class="filter-row">
          <span class="filter-label">窗口</span>
          <el-radio-group v-model="windowSeconds" size="small">
            <el-radio-button v-for="w in windows" :key="w.value" :label="w.value">{{ w.label }}</el-radio-button>
          </el-radio-group>
        </div>
      </div>

      <el-tabs v-model="tab" class="admin-tabs node-monitor-tabs">
        <el-tab-pane name="rank" label="资源排行">
          <div class="panel-body">
            <ErrorState v-if="rankError" :message="rankError" @retry="loadRank" />
            <template v-else>
              <RankTableBlock
                :rows="rows"
                :columns="columns"
                :loading="loading"
                :group="group"
                empty-text="暂无节点数据。请确认边缘节点已安装、在线并正常上报心跳。"
              />
            </template>
          </div>
        </el-tab-pane>

        <el-tab-pane name="metrics" label="监控指标">
          <div class="panel-body panel-grid">
            <ErrorState v-if="seriesError" :message="seriesError" @retry="loadSeries" />
            <template v-else>
              <div class="trend-header">
                <div class="trend-tip">趋势为全部节点汇总：{{ trendLabel }}（{{ windowLabel }}）</div>
                <el-button plain size="small" :loading="seriesLoading" @click="loadSeries">刷新趋势</el-button>
              </div>
              <LineAreaChart v-if="chartValues.length" :values="chartValues" :labels="chartLabels" :height="260" />
              <el-empty v-else description="暂无趋势数据，请等待节点上报或切换时间窗口" />
              <RankTableBlock :rows="rows" :columns="columns" :loading="loading" :group="group" empty-text="暂无节点数据" />
            </template>
          </div>
        </el-tab-pane>

        <el-tab-pane name="traffic" label="节点流量">
          <div class="panel-body">
            <ErrorState v-if="rankError" :message="rankError" @retry="loadRank" />
            <template v-else>
              <RankTableBlock
                :rows="rows"
                :columns="trafficColumns"
                :loading="loading"
                :window-label="windowLabel"
                traffic-mode
                empty-text="暂无流量数据"
              />
            </template>
          </div>
        </el-tab-pane>
      </el-tabs>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, defineComponent, h, onMounted, onUnmounted, ref, watch, type PropType } from "vue"
import { api, type NodeMonitorRankEntry, type NodeMonitorSeriesResponse } from "@/lib/api"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"
import { useMonitorSSE } from "@/composables/useMonitorSSE"
import ErrorState from "@/components/common/ErrorState.vue"
import { ElTag } from "element-plus"

const formatBps = (bytesPerSec: number) => {
  const v = Number(bytesPerSec || 0) * 8
  if (!Number.isFinite(v) || v <= 0) return "0.0 Kbps"
  const kb = v / 1000
  if (kb < 1000) return `${kb.toFixed(1)} Kbps`
  const mb = kb / 1000
  if (mb < 1000) return `${mb.toFixed(2)} Mbps`
  return `${(mb / 1000).toFixed(2)} Gbps`
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

const RankTableBlock = defineComponent({
  name: "RankTableBlock",
  props: {
    rows: { type: Array as PropType<NodeMonitorRankEntry[]>, default: () => [] },
    columns: { type: Array as PropType<any[]>, required: true },
    loading: { type: Boolean, default: false },
    emptyText: { type: String, default: "暂无数据" },
    group: { type: String as PropType<"bandwidth" | "connect" | "load" | "disk">, default: "bandwidth" },
    windowLabel: { type: String, default: "" },
    trafficMode: { type: Boolean, default: false },
  },
  setup(props) {
    const mobileFields = (row: NodeMonitorRankEntry) => {
      if (props.trafficMode) {
        return [
          { label: "出站带宽", value: formatBps(row.out_bps) },
          { label: "入站带宽", value: formatBps(row.in_bps) },
          { label: `出站流量(${props.windowLabel})`, value: formatBytes(row.delta_bytes_sent) },
          { label: `入站流量(${props.windowLabel})`, value: formatBytes(row.delta_bytes_recv) },
        ]
      }
      if (props.group === "connect") {
        return [
          { label: "网卡", value: row.iface || "-" },
          { label: "连接数", value: (row.connections || 0).toFixed(0), mono: true },
        ]
      }
      if (props.group === "load") {
        return [
          { label: "CPU", value: formatPercent(row.cpu_usage) },
          { label: "内存", value: formatPercent(row.mem_usage) },
        ]
      }
      if (props.group === "disk") {
        return [{ label: "磁盘使用率", value: formatPercent(row.disk_usage) }]
      }
      return [
        { label: "网卡", value: row.iface || "-" },
        { label: "出站", value: formatBps(row.out_bps) },
        { label: "入站", value: formatBps(row.in_bps) },
      ]
    }

    return () =>
      h("div", { class: "rank-table-block" }, [
        h(EpDataTable, {
          class: "admin-desktop-only",
          data: props.rows,
          columns: props.columns,
          rowKey: "node_id",
          size: "small",
          loading: props.loading,
          emptyText: props.emptyText,
        }),
        props.loading
          ? h("div", { class: "admin-mobile-only mobile-loading" }, "加载中…")
          : props.rows.length === 0
            ? h("div", { class: "admin-mobile-only admin-mobile-card-empty" }, props.emptyText)
            : h(
                "div",
                { class: "admin-mobile-only admin-mobile-cards" },
                props.rows.map((row, idx) =>
                  h("div", { key: row.node_id, class: "admin-mobile-card" }, [
                    h("div", { class: "admin-mobile-card-header" }, [
                      h("span", { class: "admin-mobile-card-title" }, row.hostname || row.node_id),
                      h("span", { class: "mobile-rank-badge" }, `#${row.rank || idx + 1}`),
                    ]),
                    h("div", { class: "admin-mobile-card-subtitle mono" }, row.node_id),
                    h(
                      "div",
                      { class: "admin-mobile-card-body" },
                      mobileFields(row).map((f) =>
                        h("div", { key: f.label, class: "admin-mobile-card-row" }, [
                          h("span", { class: "admin-mobile-card-label" }, f.label),
                          h("span", { class: ["admin-mobile-card-value", f.mono ? "mono" : ""] }, f.value),
                        ])
                      )
                    ),
                  ])
                )
              ),
      ])
  },
})

const tab = ref<"rank" | "metrics" | "traffic">("rank")
const group = ref<"bandwidth" | "connect" | "load" | "disk">("bandwidth")
const windowSeconds = ref(60)
const loading = ref(false)
const rankError = ref("")
const rows = ref<NodeMonitorRankEntry[]>([])
const seriesLoading = ref(false)
const seriesError = ref("")
const series = ref<NodeMonitorSeriesResponse | null>(null)
const lastUpdatedAt = ref("")

const { data: sseData, connected: sseConnected, connect: sseConnect, disconnect: sseDisconnect } = useMonitorSSE()

const windows = [
  { label: "1分钟", value: 60 },
  { label: "5分钟", value: 300 },
  { label: "30分钟", value: 1800 },
  { label: "1小时", value: 3600 },
]

const livePushEnabled = computed(() => windowSeconds.value === 60)

const touchUpdatedAt = () => {
  lastUpdatedAt.value = new Date().toLocaleTimeString("zh-CN")
}

const sortRows = (list: NodeMonitorRankEntry[]) => {
  const sorted = [...list]
  sorted.sort((a, b) => {
    switch (group.value) {
      case "connect":
        return (b.connections || 0) - (a.connections || 0)
      case "load":
        return (b.cpu_usage || 0) - (a.cpu_usage || 0)
      case "disk":
        return (b.disk_usage || 0) - (a.disk_usage || 0)
      default:
        return (b.out_bps || 0) - (a.out_bps || 0)
    }
  })
  sorted.forEach((n, i) => {
    n.rank = i + 1
  })
  return sorted
}

const loadRank = async () => {
  if (loading.value) return
  try {
    loading.value = true
    rankError.value = ""
    const res = await api.getNodeMonitorRank({ group: group.value, windowSeconds: windowSeconds.value, limit: 100 })
    rows.value = sortRows(res.nodes || [])
    touchUpdatedAt()
  } catch (err: unknown) {
    rankError.value = err instanceof Error ? err.message : "加载监控排行失败"
  } finally {
    loading.value = false
  }
}

const loadSeries = async () => {
  if (seriesLoading.value) return
  try {
    seriesLoading.value = true
    seriesError.value = ""
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
    touchUpdatedAt()
  } catch (err: unknown) {
    seriesError.value = err instanceof Error ? err.message : "加载监控趋势失败"
  } finally {
    seriesLoading.value = false
  }
}

const refreshAll = async () => {
  await loadRank()
  if (tab.value === "metrics") await loadSeries()
}

const nodeCell = (row: NodeMonitorRankEntry) =>
  h("div", { style: "display:flex;flex-direction:column" }, [
    h("span", { style: "font-weight:600" }, row.hostname || row.node_id),
    h("span", { style: "font-size:12px;color:var(--app-text-faint)" }, row.node_id),
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
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => h(ElTag, { type: "primary", effect: "light" }, () => formatPercent(row.cpu_usage)),
      },
      {
        colKey: "mem",
        title: "内存",
        width: 160,
        cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => h(ElTag, { type: "info", effect: "light" }, () => formatPercent(row.mem_usage)),
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
            ElTag,
            { type: row.disk_usage >= 90 ? "danger" : row.disk_usage >= 80 ? "warning" : "success", effect: "light" },
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
    title: `出站流量(${windowLabel.value})`,
    width: 200,
    cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBytes(row.delta_bytes_sent),
  },
  {
    colKey: "in_bytes",
    title: `入站流量(${windowLabel.value})`,
    width: 200,
    cell: (_h: any, { row }: { row: NodeMonitorRankEntry }) => formatBytes(row.delta_bytes_recv),
  },
])

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

watch(windowSeconds, () => {
  if (!livePushEnabled.value && tab.value === "metrics") loadSeries()
})

onMounted(() => {
  loadRank()
  sseConnect()
})

onUnmounted(() => {
  sseDisconnect()
})

watch(sseData, (d) => {
  if (!d || !livePushEnabled.value) return
  if (d.rank?.nodes?.length) {
    rows.value = sortRows(d.rank.nodes)
    touchUpdatedAt()
  }
  if (tab.value === "metrics" && d.series) {
    const metricKey =
      group.value === "connect" ? "connections" : group.value === "load" ? "cpu" : group.value === "disk" ? "disk" : "bandwidth_out"
    if (d.series[metricKey]) {
      series.value = d.series[metricKey]
    }
  }
})
</script>

<style scoped>
.node-monitor-header-actions {
  display: flex;
  align-items: center;
  flex-wrap: wrap;
  gap: 10px;
}

.node-monitor-updated {
  font-size: 12px;
  color: var(--app-text-faint);
}

.node-monitor-card :deep(.el-card__body) {
  padding-top: 16px;
}

.node-monitor-toolbar {
  display: flex;
  flex-direction: column;
  gap: 12px;
  margin-bottom: 4px;
  padding: 0 4px 12px;
  border-bottom: 1px solid var(--app-border);
}

.node-monitor-tabs :deep(.el-tabs__header) {
  margin-bottom: 0;
}

.node-monitor-tabs :deep(.el-tabs__item) {
  height: 44px;
  font-weight: 500;
}

.node-monitor-tabs :deep(.el-tabs__item.is-active) {
  font-weight: 600;
  color: var(--app-brand-strong);
}

.rank-table-block {
  width: 100%;
}
</style>
