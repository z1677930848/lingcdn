<template>
  <div v-if="loading" class="overview-loading">
    <SkeletonStatGrid :count="4" />
  </div>
  <div v-else class="page overview-page adh-overview">
    <section class="console-head adh-console-head">
      <div class="console-head-main">
        <div class="adh-head-row">
          <h1 class="console-head-title">管理概览</h1>
          <span class="adh-health-badge" :class="overallHealthClass">{{ overallHealthText }}</span>
        </div>
        <div class="adh-head-meta">
          <span class="adh-meta-chip">
            <Wifi :size="13" />
            在线 {{ onlineNodes }}/{{ totalNodes }}
          </span>
          <span class="adh-meta-chip" :class="`adh-meta-chip--${onlineRateClass}`">
            在线率 {{ onlineRate }}%
          </span>
          <span v-if="systemStatusItems.length" class="adh-meta-chip adh-meta-chip--muted">
            {{ systemStatusItems.length }} 项服务检测
          </span>
        </div>
      </div>
      <el-button plain size="small" :loading="loading" @click="refreshAll">
        <RefreshCw :size="14" class="console-head-refresh-icon" />
        刷新
      </el-button>
    </section>

    <ErrorState v-if="error" :message="error" @retry="refreshAll" />

    <el-alert v-if="overview?.demo_data" theme="info" class="adh-demo-alert">
      流量趋势与域名排行来自节点遥测估算；配置 Elasticsearch 后将展示真实访问日志统计。
    </el-alert>

    <div class="adh-stat-grid">
      <el-card
        v-for="(stat, idx) in adminStats"
        :key="stat.key"
        :hover-shadow="true"
       
        class="adh-stat-card"
        :class="{ 'adh-stat-primary': stat.primary }"
        :body-style="{ padding: '18px 20px' }"
      >
        <div class="adh-stat-row">
          <div class="adh-stat-info">
            <div class="adh-stat-title">{{ stat.title }}</div>
            <div class="adh-stat-value">{{ stat.value }}</div>
            <div class="adh-stat-sub">{{ stat.sub }}</div>
          </div>
          <div class="adh-stat-icon" :style="stat.primary ? undefined : { background: stat.bg, color: stat.fg }">
            <component :is="stat.icon" :size="22" />
          </div>
        </div>
        <div v-if="stat.spark && hasTrendData" class="adh-stat-spark">
          <Sparkline :values="trendValues" :color="stat.primary ? 'rgba(255,255,255,0.85)' : 'var(--td-brand-color)'" />
        </div>
      </el-card>
    </div>

    <div v-if="systemStatusItems.length" class="adh-status-bar">
      <div v-for="item in systemStatusItems" :key="item.name" class="adh-status-pill" :class="`adh-pill--${item.color}`">
        <span class="adh-status-dot"></span>
        <span class="adh-status-name">{{ item.name }}</span>
        <span v-if="item.detail" class="adh-status-detail">{{ item.detail }}</span>
      </div>
    </div>

    <div class="adh-dashboard-grid">
      <div class="adh-main-col">
        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <TrendingUp :size="16" class="adh-card-head-icon" />
              <span>{{ activeTrendSeries?.name || '统计数据' }}</span>
              <span class="adh-muted adh-card-head-sub">{{ trendRangeLabel }}</span>
              <el-select
                v-if="availableTrendKeys.length > 1"
                v-model="selectedTrendKey"
                :options="availableTrendKeys"
                class="adh-trend-selector"
                size="small"
              />
            </div>
          </template>
          <LineAreaChart :values="trendValues" :labels="trendLabels" />
        </el-card>

        <el-card v-if="topDomains.length" class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <Flame :size="16" class="adh-card-head-icon" />
              <span>热门域名</span>
              <span class="adh-muted adh-card-head-sub">Top {{ topDomains.length }}</span>
            </div>
          </template>
          <div class="admin-desktop-only">
            <div class="adh-table-wrap">
              <table class="adh-table">
                <thead>
                  <tr>
                    <th>#</th>
                    <th>域名</th>
                    <th>请求数</th>
                    <th>带宽 (Mbps)</th>
                    <th class="hide-mobile">缓存命中率</th>
                    <th class="hide-mobile">源站错误率</th>
                  </tr>
                </thead>
                <tbody>
                  <tr v-for="(item, idx) in topDomains" :key="item.domain">
                    <td class="adh-rank">{{ idx + 1 }}</td>
                    <td class="adh-domain-name">{{ item.domain }}</td>
                    <td>{{ item.requests?.toLocaleString("zh-CN") ?? "-" }}</td>
                    <td>{{ item.bandwidth_mbps?.toFixed(1) ?? "-" }}</td>
                    <td class="hide-mobile">
                      <span :class="cacheRateClass(item.cache_hit_rate)">
                        {{ item.cache_hit_rate != null ? (item.cache_hit_rate * 100).toFixed(1) + "%" : "-" }}
                      </span>
                    </td>
                    <td class="hide-mobile">
                      <span :class="errorRateClass(item.origin_error_pct)">
                        {{ item.origin_error_pct != null ? (item.origin_error_pct * 100).toFixed(1) + "%" : "-" }}
                      </span>
                    </td>
                  </tr>
                </tbody>
              </table>
            </div>
          </div>
          <div class="admin-mobile-only">
            <div class="admin-mobile-cards">
              <div v-for="(item, idx) in topDomains" :key="item.domain" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div style="min-width:0">
                    <div class="admin-mobile-card-title">#{{ idx + 1 }} {{ item.domain }}</div>
                  </div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">请求数</span>
                    <span class="admin-mobile-card-value">{{ item.requests?.toLocaleString("zh-CN") ?? "-" }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">带宽 (Mbps)</span>
                    <span class="admin-mobile-card-value">{{ item.bandwidth_mbps?.toFixed(1) ?? "-" }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">缓存命中率</span>
                    <span class="admin-mobile-card-value" :class="cacheRateClass(item.cache_hit_rate)">
                      {{ item.cache_hit_rate != null ? (item.cache_hit_rate * 100).toFixed(1) + "%" : "-" }}
                    </span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">源站错误率</span>
                    <span class="admin-mobile-card-value" :class="errorRateClass(item.origin_error_pct)">
                      {{ item.origin_error_pct != null ? (item.origin_error_pct * 100).toFixed(1) + "%" : "-" }}
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </el-card>

        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head adh-card-head--split">
              <div class="adh-card-head-block">
                <Globe :size="16" class="adh-card-head-icon" />
                <span>全球访问分布</span>
                <span class="adh-muted adh-card-head-sub">按节点出流量聚合</span>
              </div>
              <div class="adh-traffic-total">
                <span class="adh-muted">累计出流量</span>
                <span class="adh-traffic-total-value">{{ formatTrafficBytes(trafficMapTotal) }}</span>
              </div>
            </div>
          </template>
          <TrafficMap :regions="trafficMapData" :height="320" />
        </el-card>
      </div>

      <aside class="adh-side-col">
        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <Zap :size="16" class="adh-card-head-icon" />
              <span>快捷入口</span>
            </div>
          </template>
          <div class="adh-quick-grid adh-quick-grid--compact">
            <RouterLink
              v-for="link in adminQuickLinks"
              :key="link.href"
              :to="link.href"
              class="adh-quick-tile"
            >
              <div class="adh-quick-icon" :style="{ background: link.bg, color: link.fg }">
                <component :is="link.icon" :size="16" />
              </div>
              <div class="adh-quick-text">
                <div class="adh-quick-title">{{ link.title }}</div>
                <div class="adh-quick-desc">{{ link.desc }}</div>
              </div>
            </RouterLink>
          </div>
        </el-card>

        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <Network :size="16" class="adh-card-head-icon" />
              <span>网络概览</span>
            </div>
          </template>
          <div class="adh-net-breakdown">
            <div class="adh-net-item adh-net--ok">
              <span class="adh-net-label">在线</span>
              <span class="adh-net-value">{{ networkData.online_nodes }}</span>
            </div>
            <div class="adh-net-item">
              <span class="adh-net-label">已连接</span>
              <span class="adh-net-value">{{ networkData.connected_nodes }}</span>
            </div>
            <div class="adh-net-item adh-net--off">
              <span class="adh-net-label">离线</span>
              <span class="adh-net-value">{{ networkData.offline_nodes }}</span>
            </div>
          </div>
          <ul v-if="networkRegions.length" class="adh-region-list adh-region-list--compact">
            <li v-for="region in networkRegions" :key="region.name" class="adh-region-item">
              <span class="adh-region-name">
                <MapPin :size="12" />
                {{ region.name }}
              </span>
              <span class="adh-region-meta">
                <span class="adh-region-nodes">{{ region.nodes }} 节点</span>
                <span :class="latencyClass(region.latency_ms)">{{ region.latency_ms }}ms</span>
              </span>
            </li>
          </ul>
          <div v-else class="adh-muted adh-empty">暂无区域数据</div>
        </el-card>

        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <HeartPulse :size="16" class="adh-card-head-icon" />
              <span>节点健康</span>
            </div>
          </template>
          <div class="adh-progress-block">
            <div class="adh-progress-head">
              <span>在线率</span>
              <span class="adh-progress-pct" :class="onlineRateClass">{{ onlineRate }}%</span>
            </div>
            <div class="adh-progress-bar">
              <div class="adh-progress-fill" :class="onlineRateClass" :style="{ width: onlineRate + '%' }"></div>
            </div>
            <div class="adh-progress-foot">{{ onlineNodes }} / {{ totalNodes }} 节点在线</div>
          </div>
          <ul class="adh-info-list adh-info-list--compact">
            <li class="adh-info-row">
              <span class="adh-info-label"><Server :size="12" /> 节点总数</span>
              <span class="adh-info-value">{{ totalNodes }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><Globe2 :size="12" /> 域名数</span>
              <span class="adh-info-value">{{ usageData.domains }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><FileBadge2 :size="12" /> 证书数</span>
              <span class="adh-info-value">{{ usageData.certificates }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><HardDrive :size="12" /> 缓存规则</span>
              <span class="adh-info-value">{{ usageData.cache_rules }}</span>
            </li>
          </ul>
        </el-card>

        <el-card class="adh-card" :body-style="{ padding: '0' }">
          <template #header>
            <div class="adh-card-head">
              <Megaphone :size="16" class="adh-card-head-icon" />
              <span>系统公告</span>
              <RouterLink to="/admin/dashboard/announcements" class="adh-card-head-link">查看全部</RouterLink>
            </div>
          </template>
          <LoadingState v-if="announcementsLoading && announcements.length === 0" size="small" text="加载公告…" />
          <ErrorState v-else-if="announcementsError" :message="announcementsError" @retry="loadAnnouncements" />
          <ul v-else-if="announcements.length" class="adh-anno-list">
            <li v-for="item in announcements.slice(0, 3)" :key="item.id" class="adh-anno-item">
              <div class="adh-anno-bullet"></div>
              <div class="adh-anno-body">
                <div class="adh-anno-title">
                  <span>{{ item.title }}</span>
                  <el-tag v-if="item.pinned" type="warning" size="small" effect="light" shape="round">置顶</el-tag>
                </div>
                <div class="adh-anno-time">{{ formatDateTime(item.updated_at) }}</div>
              </div>
            </li>
          </ul>
          <div v-else class="adh-empty">暂无公告</div>
        </el-card>

        <el-card class="adh-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <Cpu :size="16" class="adh-card-head-icon" />
              <span>系统信息</span>
            </div>
          </template>
          <LoadingState v-if="systemInfoLoading && !systemInfo" size="small" text="加载系统信息…" />
          <ErrorState v-else-if="systemInfoError" :message="systemInfoError" @retry="loadSystemInfo" />
          <ul v-else class="adh-info-list adh-info-list--compact">
            <li class="adh-info-row">
              <span class="adh-info-label"><Clock :size="12" /> 运行时长</span>
              <span class="adh-info-value">{{ systemInfo ? formatUptime(systemInfo.uptime_seconds) : systemPlaceholder }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><Tag :size="12" /> 应用版本</span>
              <span class="adh-info-value adh-mono">{{ systemInfo?.app_version || systemPlaceholder }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><Code :size="12" /> Go 版本</span>
              <span class="adh-info-value adh-mono">{{ systemInfo?.go_version || systemPlaceholder }}</span>
            </li>
            <li class="adh-info-row">
              <span class="adh-info-label"><Bug :size="12" /> 调试模式</span>
              <span class="adh-info-value">
                <el-tag :type="systemInfo?.debug_mode ? 'warning' : 'info'" effect="light" size="small" shape="round">
                  {{ systemInfo ? (systemInfo.debug_mode ? '已开启' : '已关闭') : systemPlaceholder }}
                </el-tag>
              </span>
            </li>
          </ul>
        </el-card>

        <el-card class="adh-card adh-install-card" :body-style="{ padding: '18px 20px' }">
          <template #header>
            <div class="adh-card-head">
              <Terminal :size="16" class="adh-card-head-icon" />
              <span>节点安装</span>
              <button type="button" class="adh-mini-btn" :disabled="installLoading" @click="loadInstallInfo">
                <RefreshCw :size="12" :class="{ spin: installLoading }" />
                <span>刷新</span>
              </button>
            </div>
          </template>
          <ErrorState v-if="installError" :message="installError" @retry="loadInstallInfo" />
          <LoadingState v-else-if="installLoading && !installInfo" size="small" text="加载安装信息…" />
          <div v-else class="adh-install-wrap adh-install-wrap--compact">
            <div class="adh-install-row">
              <span class="adh-install-label">Master Host</span>
              <span class="adh-install-value adh-mono">{{ installInfo?.master_host || "-" }}</span>
              <button
                type="button"
                class="adh-mini-btn"
                :disabled="!installInfo?.master_host"
                @click="copyText(installInfo?.master_host || '', 'Master Host')"
              >
                <Copy :size="12" />
              </button>
            </div>
            <div class="adh-install-row">
              <span class="adh-install-label">Master Token</span>
              <span class="adh-install-value adh-mono">{{ installInfo?.master_token || "-" }}</span>
              <button
                type="button"
                class="adh-mini-btn"
                :disabled="!installInfo?.master_token"
                @click="copyText(installInfo?.master_token || '', 'Master Token')"
              >
                <Copy :size="12" />
              </button>
            </div>
            <div class="adh-install-script-row">
              <code class="adh-install-script">{{ installInfo?.command || "-" }}</code>
              <button
                type="button"
                class="adh-mini-btn adh-mini-btn--primary"
                :disabled="!installInfo?.command"
                @click="copyText(installInfo?.command || '', '安装命令')"
              >
                <Copy :size="12" />
                复制
              </button>
            </div>
          </div>
        </el-card>
      </aside>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import {
  Activity,
  RefreshCw,
  TrendingUp,
  Network,
  MapPin,
  Globe,
  Globe2,
  Flame,
  HeartPulse,
  Server,
  FileBadge2,
  HardDrive,
  Cpu,
  Clock,
  Tag,
  Code,
  Bug,
  Megaphone,
  Zap,
  Terminal,
  Copy,
  Wifi,
  ShieldCheck,
  ListChecks,
  Settings as SettingsIcon,
  ArrowUpCircle,
  Users,
} from "lucide-vue-next"
import { api, type Announcement, type OverviewResponse, type SystemInfo } from "@/lib/api"
import { copyText as copyToClipboard } from "@/lib/clipboard"
import { useSystemSettings } from "@/lib/systemSettings"
import LineAreaChart from "@/components/charts/LineAreaChart.vue"
import Sparkline from "@/components/charts/Sparkline.vue"
import TrafficMap from "@/components/charts/TrafficMap.vue"
import SkeletonStatGrid from "@/components/common/SkeletonStatGrid.vue"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const overview = ref<OverviewResponse | null>(null)
const { brand } = useSystemSettings()
const loading = ref(true)
const error = ref("")
const systemInfo = ref<SystemInfo | null>(null)
const systemInfoLoading = ref(false)
const systemInfoError = ref("")
const announcements = ref<Announcement[]>([])
const announcementsLoading = ref(false)
const announcementsError = ref("")
const selectedTrendKey = ref("requests")

type NodeInstallCommandInfo = {
  command: string
  master_host: string
  master_token: string
  master_version: string
  expires_at: string
  portal_base: string
  script_url: string
  style: string
}

const installInfo = ref<NodeInstallCommandInfo | null>(null)
const installLoading = ref(false)
const installError = ref("")

const loadOverview = async () => {
  try {
    loading.value = true
    const data = await api.getOverview("7d")
    overview.value = data
    error.value = ""
    const keys = data.trends?.map((t) => t.key) || []
    if (keys.length && !keys.includes(selectedTrendKey.value)) {
      selectedTrendKey.value = keys[0]
    }
  } catch (err: any) {
    const message = err.message || "加载仪表盘数据失败"
    error.value = message
  } finally {
    loading.value = false
  }
}

const loadSystemInfo = async () => {
  try {
    systemInfoLoading.value = true
    systemInfoError.value = ""
    const { info } = await api.getSystemInfo()
    systemInfo.value = info
  } catch (err: any) {
    systemInfoError.value = err.message || "加载系统信息失败"
  } finally {
    systemInfoLoading.value = false
  }
}

const loadAnnouncements = async () => {
  try {
    announcementsLoading.value = true
    announcementsError.value = ""
    const res = await api.getAdminAnnouncements({ page: 1, pageSize: 5, status: "published" })
    announcements.value = res.announcements || []
  } catch (err: any) {
    announcementsError.value = err.message || "加载公告失败"
  } finally {
    announcementsLoading.value = false
  }
}

const loadInstallInfo = async () => {
  try {
    installLoading.value = true
    const res = await api.getNodeInstallCommand()
    installInfo.value = res
    installError.value = ""
  } catch (err: any) {
    installInfo.value = null
    installError.value = err.message || "加载安装命令失败"
  } finally {
    installLoading.value = false
  }
}

const refreshAll = () => {
  loadOverview()
  loadSystemInfo()
  loadAnnouncements()
  loadInstallInfo()
}

// Summary computeds
const summary = computed(() => overview.value?.summary)
const totalUsers = computed(() => summary.value?.registered_users ?? 0)
const totalNodes = computed(() => summary.value?.total_nodes ?? 0)
const onlineNodes = computed(() => summary.value?.online_nodes ?? 0)
const totalDomains = computed(() => summary.value?.domains ?? 0)
const totalCerts = computed(() => summary.value?.certificates ?? 0)
const onlineRate = computed(() => (totalNodes.value > 0 ? Math.round((onlineNodes.value / totalNodes.value) * 100) : 0))

// Trend series
const availableTrendKeys = computed(() =>
  (overview.value?.trends || []).map((t) => ({ label: t.name, value: t.key }))
)
const activeTrendSeries = computed(() => {
  const series = overview.value?.trends || []
  return series.find((s) => s.key === selectedTrendKey.value) || series[0]
})
const trendPoints = computed(() => activeTrendSeries.value?.points || [])
const trendValues = computed(() => trendPoints.value.map((p) => Number(p.value || 0)))
// 全 0 数据时 Sparkline 仍会画出一条平直基线，看起来像渲染残缺。
// 用这个 flag 在主卡里隐藏 spark 区域，等真正有流量再显示。
const hasTrendData = computed(() => trendValues.value.some((v) => Number.isFinite(v) && v > 0))
const trendLabels = computed(() =>
  trendPoints.value.map((p) => {
    const date = new Date(p.timestamp)
    if (Number.isNaN(date.getTime())) return "-"
    return `${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`
  })
)
const trendRangeLabel = computed(() => {
  const points = trendPoints.value
  const start = points[0]?.timestamp
  const end = points[points.length - 1]?.timestamp
  return start && end ? `${formatDateTime(start)} - ${formatDateTime(end)}` : "近 7 天"
})

// Bandwidth
const bandwidthSeries = computed(() => {
  const series = overview.value?.trends || []
  return series.find((s) => s.key === "bandwidth") || series[0]
})
const bandwidthValues = computed(() => (bandwidthSeries.value?.points || []).map((p) => Number(p.value || 0)))
const formattedBandwidth = computed(() => {
  const values = bandwidthValues.value
  if (!values.length) return "-"
  const latest = values[values.length - 1]
  if (latest >= 1000) return (latest / 1000).toFixed(1) + " Gbps"
  if (latest >= 1) return latest.toFixed(1) + " Mbps"
  if (latest >= 0.001) return (latest * 1000).toFixed(1) + " Kbps"
  return (latest * 1e6).toFixed(0) + " bps"
})

// Stat cards drive the four-tile grid; centralising config keeps the
// template short and makes reordering / re-theming a one-line change.
const adminStats = computed(() => [
  {
    key: "online",
    primary: true,
    title: "在线节点",
    value: onlineNodes.value.toLocaleString("zh-CN"),
    sub: `在线率 ${onlineRate.value}%`,
    icon: Wifi,
    spark: true,
    bg: "",
    fg: "",
  },
  {
    key: "bandwidth",
    primary: false,
    title: "总带宽",
    value: formattedBandwidth.value,
    sub: `节点总数 ${totalNodes.value}`,
    icon: Activity,
    spark: false,
    bg: "rgba(64, 158, 255, 0.12)",
    fg: "#337ECC",
  },
  {
    key: "domains",
    primary: false,
    title: "网站数",
    value: totalDomains.value.toLocaleString("zh-CN"),
    sub: `证书数 ${totalCerts.value}`,
    icon: Globe2,
    spark: false,
    bg: "rgba(16, 185, 129, 0.12)",
    fg: "#047857",
  },
  {
    key: "users",
    primary: false,
    title: "用户数",
    value: totalUsers.value.toLocaleString("zh-CN"),
    sub: `配置版本 ${usageData.value.config_versions}`,
    icon: Users,
    spark: false,
    bg: "rgba(168, 85, 247, 0.12)",
    fg: "#7c3aed",
  },
])

// System status �?same OK/degraded/error mapping as before, just rendered
// via the new pill style.
const systemStatusItems = computed(() => {
  const statuses = overview.value?.system_status || []
  return statuses.map((s) => {
    let color: string
    const status = (s.status || "").toLowerCase()
    if (status === "ok" || status === "running" || status === "healthy" || status === "active") color = "ok"
    else if (status === "degraded" || status === "warning" || status === "slow") color = "warn"
    else color = "err"
    return { name: s.name, status: s.status, detail: s.detail || "", color }
  })
})

// Overall pulse �?derived from the worst severity in system_status; falls
// back to onlineRate when there are no checks reporting yet.
const overallHealthClass = computed(() => {
  const items = systemStatusItems.value
  if (items.some((i) => i.color === "err")) return "adh-pulse--err"
  if (items.some((i) => i.color === "warn")) return "adh-pulse--warn"
  if (onlineRate.value < 60) return "adh-pulse--warn"
  return "adh-pulse--ok"
})
const overallHealthText = computed(() => {
  switch (overallHealthClass.value) {
    case "adh-pulse--err": return "存在故障"
    case "adh-pulse--warn": return "运行中（注意）"
    default: return "运行正常"
  }
})

// Network
const networkData = computed(() => overview.value?.network || { total_nodes: 0, online_nodes: 0, connected_nodes: 0, offline_nodes: 0, regions: [] })
const networkRegions = computed(() => networkData.value.regions || [])

// Top domains
const topDomains = computed(() => (overview.value?.top_domains || []).slice(0, 10))

// Traffic map
const trafficMapData = computed(() => overview.value?.traffic_map || [])
const trafficMapTotal = computed(() => trafficMapData.value.reduce((acc, cur) => acc + Number(cur.bytes_sent || 0), 0))
const formatTrafficBytes = (bytes: number) => {
  if (bytes >= 1e12) return (bytes / 1e12).toFixed(1) + " TB"
  if (bytes >= 1e9) return (bytes / 1e9).toFixed(1) + " GB"
  if (bytes >= 1e6) return (bytes / 1e6).toFixed(1) + " MB"
  if (bytes >= 1e3) return (bytes / 1e3).toFixed(1) + " KB"
  return bytes.toString() + " B"
}

// Usage
const usageData = computed(() => overview.value?.usage || { domains: 0, certificates: 0, cache_rules: 0, config_versions: 0 })

const cacheRateClass = (rate?: number) => {
  if (rate == null) return ""
  if (rate >= 0.8) return "adh-rate-good"
  if (rate >= 0.5) return "adh-rate-medium"
  return "adh-rate-poor"
}
const errorRateClass = (rate?: number) => {
  if (rate == null) return ""
  if (rate <= 0.01) return "adh-rate-good"
  if (rate <= 0.05) return "adh-rate-medium"
  return "adh-rate-poor"
}
const latencyClass = (ms?: number) => {
  if (ms == null) return ""
  if (ms < 30) return "adh-region-latency-good"
  if (ms <= 80) return "adh-region-latency-ok"
  return "adh-region-latency-warn"
}
const onlineRateClass = computed(() => {
  if (onlineRate.value >= 90) return "ok"
  if (onlineRate.value >= 60) return "warn"
  return "err"
})

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}
const formatUptime = (seconds?: number) => {
  if (seconds === undefined || seconds === null || Number.isNaN(seconds)) return "-"
  const total = Math.max(0, Math.floor(seconds))
  const days = Math.floor(total / 86400)
  const hours = Math.floor((total % 86400) / 3600)
  const mins = Math.floor((total % 3600) / 60)
  const parts: string[] = []
  if (days > 0) parts.push(`${days}d`)
  if (hours > 0 || days > 0) parts.push(`${hours}h`)
  parts.push(`${mins}m`)
  return parts.join(" ")
}

const systemPlaceholder = computed(() => (systemInfoLoading.value && !systemInfo.value ? "加载中…" : "-"))

const copyText = async (text: string, label: string) => {
  if (!text.trim()) {
    MessagePlugin.warning(`${label}为空`)
    return
  }
  const ok = await copyToClipboard(text)
  if (ok) {
    MessagePlugin.success(`${label}已复制`)
  } else {
    MessagePlugin.error("复制失败，请手动复制")
  }
}

const adminQuickLinks = [
  { title: "节点管理", desc: "添加 / 监控节点", href: "/admin/dashboard/nodes", icon: Server, bg: "rgba(64, 158, 255, 0.12)", fg: "#337ECC" },
  { title: "域名健康", desc: "网站监控状态", href: "/admin/dashboard/domains/health", icon: HeartPulse, bg: "rgba(239, 68, 68, 0.12)", fg: "#b91c1c" },
  { title: "WAF 策略", desc: "安全防护规则", href: "/admin/dashboard/waf", icon: ShieldCheck, bg: "rgba(16, 185, 129, 0.12)", fg: "#047857" },
  { title: "网站设置", desc: "全局配置项", href: "/admin/dashboard/settings/site", icon: SettingsIcon, bg: "rgba(100, 116, 139, 0.14)", fg: "var(--app-text-muted)" },
  { title: "升级中心", desc: "版本与发布", href: "/admin/dashboard/upgrade", icon: ArrowUpCircle, bg: "rgba(168, 85, 247, 0.12)", fg: "#7c3aed" },
  { title: "操作日志", desc: "审计与追踪", href: "/admin/dashboard/logs", icon: ListChecks, bg: "rgba(245, 158, 11, 0.12)", fg: "var(--app-warning-strong)" },
]

onMounted(() => {
  loadOverview()
  loadSystemInfo()
  loadAnnouncements()
  loadInstallInfo()
})
</script>

