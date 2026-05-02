<template>
  <div v-if="loading" class="overview-loading">
    <t-loading size="medium" />
  </div>
  <div v-else class="page overview-page">
    <!-- Hero: system pulse banner with live indicators -->
    <section class="adh-hero" v-motion="{ initial: { opacity: 0, y: 16 }, enter: { opacity: 1, y: 0, transition: { duration: 360 } } }">
      <div class="adh-hero-glow" aria-hidden="true"></div>
      <div class="adh-hero-grid" aria-hidden="true"></div>
      <div class="adh-hero-content">
        <div class="adh-hero-left">
          <div class="adh-hero-tag">
            <Activity :size="14" />
            <span>系统脉搏 · 实时</span>
          </div>
          <h1 class="adh-hero-title">管理员控制台</h1>
          <p class="adh-hero-subtitle">系统概览与核心指标，掌握 {{ brand.title }} 全局运行状态</p>
          <div class="adh-hero-pulse">
            <div class="adh-pulse-dot" :class="overallHealthClass">
              <span class="adh-pulse-ring"></span>
            </div>
            <span class="adh-pulse-text">{{ overallHealthText }}</span>
            <span class="adh-pulse-detail">在线率 {{ onlineRate }}% · {{ onlineNodes }}/{{ totalNodes }} 节点</span>
          </div>
        </div>
        <div class="adh-hero-right">
          <button type="button" class="adh-refresh-btn" :disabled="loading" @click="refreshAll">
            <RefreshCw :size="14" :class="{ 'spin': loading }" />
            <span>刷新数据</span>
          </button>
        </div>
      </div>
    </section>

    <div v-if="error" class="error-box">{{ error }}</div>

    <!-- Stat cards -->
    <div class="adh-stat-grid">
      <t-card
        v-for="(stat, idx) in adminStats"
        :key="stat.key"
        :hover-shadow="true"
        :bordered="true"
        class="adh-stat-card"
        :class="{ 'adh-stat-primary': stat.primary }"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 80 + idx * 60, duration: 320 } } }"
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
      </t-card>
    </div>

    <!-- System status pills -->
    <t-card
      v-if="systemStatusItems.length"
      class="adh-card"
      :bordered="true"
      :body-style="{ padding: '18px 20px' }"
      v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 320, duration: 320 } } }"
    >
      <template #header>
        <div class="adh-card-head">
          <ServerCog :size="16" class="adh-card-head-icon" />
          <span>系统状态</span>
        </div>
      </template>
      <div class="adh-status-row">
        <div v-for="item in systemStatusItems" :key="item.name" class="adh-status-pill" :class="`adh-pill--${item.color}`">
          <span class="adh-status-dot"></span>
          <span class="adh-status-name">{{ item.name }}</span>
          <span v-if="item.detail" class="adh-status-detail">{{ item.detail }}</span>
        </div>
      </div>
    </t-card>

    <!-- Trend chart + Network overview -->
    <div class="adh-chart-grid">
      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 380, duration: 320 } } }"
      >
        <template #header>
          <div class="adh-card-head">
            <TrendingUp :size="16" class="adh-card-head-icon" />
            <span>{{ activeTrendSeries?.name || '统计数据' }}</span>
            <span class="adh-muted adh-card-head-sub">{{ trendRangeLabel }}</span>
            <t-select
              v-if="availableTrendKeys.length > 1"
              v-model="selectedTrendKey"
              :options="availableTrendKeys"
              class="adh-trend-selector"
              size="small"
            />
          </div>
        </template>
        <LineAreaChart :values="trendValues" :labels="trendLabels" />
      </t-card>

      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 440, duration: 320 } } }"
      >
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
        <ul v-if="networkRegions.length" class="adh-region-list">
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
      </t-card>
    </div>

    <!-- Global traffic heatmap -->
    <t-card
      class="adh-card"
      :bordered="true"
      :body-style="{ padding: '18px 20px' }"
      v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 500, duration: 320 } } }"
    >
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
      <TrafficMap :regions="trafficMapData" :height="360" />
    </t-card>

    <!-- Top domains -->
    <t-card
      v-if="topDomains.length"
      class="adh-card"
      :bordered="true"
      :body-style="{ padding: '18px 20px' }"
      v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 560, duration: 320 } } }"
    >
      <template #header>
        <div class="adh-card-head">
          <Flame :size="16" class="adh-card-head-icon" />
          <span>热门域名</span>
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
    </t-card>

    <!-- Node health + System info -->
    <div class="adh-info-grid">
      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 620, duration: 320 } } }"
      >
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
        <ul class="adh-info-list">
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
      </t-card>

      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 680, duration: 320 } } }"
      >
        <template #header>
          <div class="adh-card-head">
            <Cpu :size="16" class="adh-card-head-icon" />
            <span>系统信息</span>
          </div>
        </template>
        <ul class="adh-info-list">
          <li class="adh-info-row">
            <span class="adh-info-label"><Clock :size="12" /> 运行时长</span>
            <span class="adh-info-value">{{ systemInfo ? formatUptime(systemInfo.uptime_seconds) : systemPlaceholder }}</span>
          </li>
          <li class="adh-info-row">
            <span class="adh-info-label"><Hash :size="12" /> 编译信息</span>
            <span class="adh-info-value adh-mono">{{ systemInfo ? formatBuildInfo() : systemPlaceholder }}</span>
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
              <t-tag :theme="systemInfo?.debug_mode ? 'warning' : 'default'" variant="light" size="small" shape="round">
                {{ systemInfo ? (systemInfo.debug_mode ? '已开启' : '已关闭') : systemPlaceholder }}
              </t-tag>
            </span>
          </li>
          <li class="adh-info-row">
            <span class="adh-info-label"><Database :size="12" /> 数据库调试</span>
            <span class="adh-info-value">
              <t-tag :theme="systemInfo?.db_debug_mode ? 'warning' : 'default'" variant="light" size="small" shape="round">
                {{ systemInfo ? (systemInfo.db_debug_mode ? '已开启' : '已关闭') : systemPlaceholder }}
              </t-tag>
            </span>
          </li>
        </ul>
      </t-card>
    </div>

    <!-- Announcements + Quick actions -->
    <div class="adh-chart-grid">
      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '0' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 740, duration: 320 } } }"
      >
        <template #header>
          <div class="adh-card-head">
            <Megaphone :size="16" class="adh-card-head-icon" />
            <span>系统公告</span>
            <RouterLink to="/admin/dashboard/announcements" class="adh-card-head-link">查看全部</RouterLink>
          </div>
        </template>
        <div v-if="announcementsLoading && announcements.length === 0" class="adh-empty">加载中…</div>
        <ul v-else-if="announcements.length" class="adh-anno-list">
          <li v-for="item in announcements.slice(0, 3)" :key="item.id" class="adh-anno-item">
            <div class="adh-anno-bullet"></div>
            <div class="adh-anno-body">
              <div class="adh-anno-title">
                <span>{{ item.title }}</span>
                <t-tag v-if="item.pinned" theme="warning" size="small" variant="light" shape="round">置顶</t-tag>
              </div>
              <div class="adh-anno-time">{{ formatDateTime(item.updated_at) }}</div>
            </div>
          </li>
        </ul>
        <div v-else class="adh-empty">暂无公告</div>
      </t-card>

      <t-card
        class="adh-card"
        :bordered="true"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 800, duration: 320 } } }"
      >
        <template #header>
          <div class="adh-card-head">
            <Zap :size="16" class="adh-card-head-icon" />
            <span>快捷入口</span>
          </div>
        </template>
        <div class="adh-quick-grid">
          <RouterLink
            v-for="link in adminQuickLinks"
            :key="link.href"
            :to="link.href"
            class="adh-quick-tile"
          >
            <div class="adh-quick-icon" :style="{ background: link.bg, color: link.fg }">
              <component :is="link.icon" :size="18" />
            </div>
            <div class="adh-quick-title">{{ link.title }}</div>
            <div class="adh-quick-desc">{{ link.desc }}</div>
          </RouterLink>
        </div>
      </t-card>
    </div>

    <!-- Node install -->
    <t-card
      class="adh-card adh-install-card"
      :bordered="true"
      :body-style="{ padding: '18px 20px' }"
      v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 860, duration: 320 } } }"
    >
      <template #header>
        <div class="adh-card-head">
          <Terminal :size="16" class="adh-card-head-icon" />
          <span>节点安装维护</span>
          <button type="button" class="adh-mini-btn" :disabled="installLoading" @click="loadInstallInfo">
            <RefreshCw :size="12" :class="{ 'spin': installLoading }" />
            <span>刷新</span>
          </button>
        </div>
      </template>
      <div v-if="installError" class="error-box">{{ installError }}</div>
      <div v-else-if="installLoading && !installInfo" class="adh-muted adh-empty">加载中…</div>
      <div v-else class="adh-install-wrap">
        <div class="adh-install-section-title">
          <Link2 :size="13" />
          Master 通信信息
        </div>
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
            复制
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
            复制
          </button>
        </div>

        <div class="adh-install-section-title">
          <Terminal :size="13" />
          节点安装脚本
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
            复制脚本
          </button>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import {
  Activity,
  RefreshCw,
  ServerCog,
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
  Hash,
  Tag,
  Code,
  Bug,
  Database,
  Megaphone,
  Zap,
  Terminal,
  Copy,
  Link2,
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
import "@/styles/overview.css"

const overview = ref<OverviewResponse | null>(null)
const { brand } = useSystemSettings()
const loading = ref(true)
const error = ref("")
const systemInfo = ref<SystemInfo | null>(null)
const systemInfoLoading = ref(false)
const announcements = ref<Announcement[]>([])
const announcementsLoading = ref(false)
const selectedTrendKey = ref("requests")
const FIXED_PORTAL_BASE = "https://auh.lingcdn.cloud"

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
    MessagePlugin.error(message)
  } finally {
    loading.value = false
  }
}

const loadSystemInfo = async () => {
  try {
    systemInfoLoading.value = true
    const { info } = await api.getSystemInfo()
    systemInfo.value = info
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载系统信息失败")
  } finally {
    systemInfoLoading.value = false
  }
}

const loadAnnouncements = async () => {
  try {
    announcementsLoading.value = true
    const res = await api.getAdminAnnouncements({ page: 1, pageSize: 5, status: "published" })
    announcements.value = res.announcements || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载公告失败")
  } finally {
    announcementsLoading.value = false
  }
}

const loadInstallInfo = async () => {
  try {
    installLoading.value = true
    const res = await api.getNodeInstallCommand({ portalBase: FIXED_PORTAL_BASE })
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
    bg: "rgba(99, 91, 255, 0.12)",
    fg: "#4D44E0",
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

// System status — same OK/degraded/error mapping as before, just rendered
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

// Overall pulse — derived from the worst severity in system_status; falls
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
const formatBuildInfo = () => {
  const timeText = systemInfo.value?.build_time?.trim() || "-"
  const hashText = systemInfo.value?.build_hash?.trim()
  if (!hashText) return timeText
  return `${timeText} - ${hashText}`
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
  { title: "节点管理", desc: "添加 / 监控节点", href: "/admin/dashboard/nodes", icon: Server, bg: "rgba(99, 91, 255, 0.12)", fg: "#4D44E0" },
  { title: "域名健康", desc: "网站监控状态", href: "/admin/dashboard/domains/health", icon: HeartPulse, bg: "rgba(239, 68, 68, 0.12)", fg: "#b91c1c" },
  { title: "WAF 策略", desc: "安全防护规则", href: "/admin/dashboard/waf", icon: ShieldCheck, bg: "rgba(16, 185, 129, 0.12)", fg: "#047857" },
  { title: "系统设置", desc: "全局配置项", href: "/admin/dashboard/settings", icon: SettingsIcon, bg: "rgba(100, 116, 139, 0.14)", fg: "#475569" },
  { title: "升级中心", desc: "版本与发布", href: "/admin/dashboard/upgrade", icon: ArrowUpCircle, bg: "rgba(168, 85, 247, 0.12)", fg: "#7c3aed" },
  { title: "操作日志", desc: "审计与追踪", href: "/admin/dashboard/logs", icon: ListChecks, bg: "rgba(245, 158, 11, 0.12)", fg: "#b45309" },
]

onMounted(() => {
  loadOverview()
  loadSystemInfo()
  loadAnnouncements()
  loadInstallInfo()
})
</script>

<style scoped>
.overview-loading {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

/* ─── Hero ─────────────────────────────────────────────────────────── */
.adh-hero {
  position: relative;
  overflow: hidden;
  border-radius: 16px;
  padding: 26px 30px;
  background: linear-gradient(135deg, #1A1A1A 0%, #2D2899 50%, #4D44E0 100%);
  color: #fff;
  box-shadow: 0 6px 24px rgba(15, 23, 42, 0.18);
  isolation: isolate;
}
.adh-hero-glow {
  position: absolute;
  inset: -40% -10%;
  background:
    radial-gradient(40% 60% at 80% 20%, rgba(125, 211, 252, 0.4), transparent 60%),
    radial-gradient(40% 50% at 15% 90%, rgba(99, 102, 241, 0.45), transparent 60%);
  filter: blur(24px);
  z-index: 0;
  pointer-events: none;
}
.adh-hero-grid {
  position: absolute;
  inset: 0;
  background-image:
    linear-gradient(to right, rgba(255, 255, 255, 0.06) 1px, transparent 1px),
    linear-gradient(to bottom, rgba(255, 255, 255, 0.06) 1px, transparent 1px);
  background-size: 32px 32px;
  mask-image: radial-gradient(ellipse at top right, #000 25%, transparent 75%);
  -webkit-mask-image: radial-gradient(ellipse at top right, #000 25%, transparent 75%);
  z-index: 0;
  pointer-events: none;
}
.adh-hero-content {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 20px;
  flex-wrap: wrap;
}
.adh-hero-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(16, 185, 129, 0.18);
  border: 1px solid rgba(110, 231, 183, 0.4);
  color: #d1fae5;
  font-size: 12px;
  letter-spacing: 0.4px;
  margin-bottom: 12px;
}
.adh-hero-title {
  font-size: 24px;
  font-weight: 700;
  margin: 0 0 6px;
  letter-spacing: -0.01em;
}
.adh-hero-subtitle {
  margin: 0 0 14px;
  color: rgba(255, 255, 255, 0.78);
  font-size: 13px;
}
.adh-hero-pulse {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  padding: 6px 14px;
  background: rgba(15, 23, 42, 0.42);
  border: 1px solid rgba(255, 255, 255, 0.12);
  border-radius: 999px;
  font-size: 13px;
  backdrop-filter: blur(8px);
}
.adh-pulse-dot {
  position: relative;
  width: 9px;
  height: 9px;
  border-radius: 50%;
  background: #10b981;
  flex: 0 0 auto;
}
.adh-pulse-dot::before {
  content: "";
  position: absolute;
  inset: -3px;
  border-radius: 50%;
  border: 2px solid currentColor;
  opacity: 0.5;
  animation: adh-ping 1.6s var(--app-easing-standard, cubic-bezier(0.4, 0, 0.2, 1)) infinite;
}
.adh-pulse--ok { background: #10b981; color: #10b981; }
.adh-pulse--warn { background: #f59e0b; color: #f59e0b; }
.adh-pulse--err { background: #ef4444; color: #ef4444; }
@keyframes adh-ping {
  0% { transform: scale(0.8); opacity: 0.7; }
  80%, 100% { transform: scale(2.2); opacity: 0; }
}
.adh-pulse-text { color: #fff; font-weight: 600; }
.adh-pulse-detail { color: rgba(255, 255, 255, 0.7); font-size: 12px; }
.adh-refresh-btn {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 14px;
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.12);
  border: 1px solid rgba(255, 255, 255, 0.2);
  color: #fff;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.15s;
}
.adh-refresh-btn:hover:not(:disabled) { background: rgba(255, 255, 255, 0.2); }
.adh-refresh-btn:disabled { opacity: 0.5; cursor: not-allowed; }

/* spin keyframe shared across icons (origin list, refresh) */
.spin {
  animation: spin 1s linear infinite;
}
@keyframes spin {
  to { transform: rotate(360deg); }
}

/* ─── Stat cards ───────────────────────────────────────────────────── */
.adh-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--app-page-gap-tight, 16px);
}
@media (max-width: 1200px) { .adh-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
@media (max-width: 600px) { .adh-stat-grid { grid-template-columns: 1fr; } }

.adh-stat-card {
  transition: transform 0.18s var(--app-easing-standard), box-shadow 0.18s;
}
.adh-stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--app-shadow-card-hover);
}
.adh-stat-primary {
  background: var(--app-brand-gradient) !important;
  border: 0 !important;
  color: #fff;
  box-shadow: var(--app-brand-shadow-md);
}
.adh-stat-primary :deep(.t-card__body) { color: #fff; }
.adh-stat-primary .adh-stat-title { color: rgba(255, 255, 255, 0.85); }
.adh-stat-primary .adh-stat-value { color: #fff; }
.adh-stat-primary .adh-stat-sub { color: rgba(255, 255, 255, 0.85); }
.adh-stat-primary .adh-stat-icon {
  background: rgba(255, 255, 255, 0.18);
  color: #fff;
  border-color: rgba(255, 255, 255, 0.22);
}
.adh-stat-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.adh-stat-info { min-width: 0; }
.adh-stat-title {
  font-size: 12px;
  color: var(--app-text-muted);
  letter-spacing: 0.4px;
  margin-bottom: 6px;
}
.adh-stat-value {
  font-size: 28px;
  font-weight: 800;
  color: var(--app-text-strong);
  letter-spacing: -0.02em;
  line-height: 1.1;
}
.adh-stat-sub {
  margin-top: 8px;
  font-size: 12px;
  color: var(--app-text-muted);
  font-weight: 500;
}
.adh-stat-icon {
  flex: 0 0 auto;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(15, 23, 42, 0.06);
}
.adh-stat-spark {
  margin-top: 12px;
  height: 32px;
}

/* ─── Generic card head ────────────────────────────────────────────── */
.adh-card { overflow: hidden; }
.adh-card-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  width: 100%;
  font-weight: 600;
  font-size: 14px;
  color: var(--app-text-strong);
}
.adh-card-head--split {
  width: 100%;
  justify-content: space-between;
  flex-wrap: wrap;
  gap: 12px;
}
.adh-card-head-block { display: inline-flex; align-items: center; gap: 8px; }
.adh-card-head-icon { color: var(--app-brand); }
.adh-card-head-sub {
  font-size: 11px;
  font-weight: 400;
  color: var(--app-text-muted);
  margin-left: 4px;
}
.adh-card-head-link {
  margin-left: auto;
  font-size: 12px;
  font-weight: 500;
  color: var(--app-brand-strong);
  text-decoration: none;
}
.adh-card-head-link:hover { text-decoration: underline; }

.adh-mini-btn {
  margin-left: auto;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 4px 10px;
  border-radius: 8px;
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  color: var(--app-text);
  font-size: 12px;
  cursor: pointer;
  transition: background 0.15s, border-color 0.15s;
}
.adh-mini-btn:hover:not(:disabled) {
  background: var(--app-surface-hover);
  border-color: var(--app-border-strong);
}
.adh-mini-btn:disabled { opacity: 0.5; cursor: not-allowed; }
.adh-mini-btn--primary {
  background: var(--app-brand-soft-bg);
  color: var(--app-brand-strong);
  border-color: var(--app-brand-soft-border);
}
.adh-mini-btn--primary:hover:not(:disabled) {
  background: rgba(99, 91, 255, 0.16);
}

.adh-trend-selector {
  margin-left: auto;
  width: 140px;
}

/* ─── System status pills ──────────────────────────────────────────── */
.adh-status-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.adh-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 6px 14px;
  border-radius: 999px;
  font-size: 13px;
  border: 1px solid var(--app-border);
  background: var(--app-surface-muted);
}
.adh-status-dot {
  width: 8px;
  height: 8px;
  border-radius: 50%;
  background: var(--app-text-faint);
  flex: 0 0 auto;
}
.adh-pill--ok { background: var(--app-success-soft-bg); border-color: rgba(16, 185, 129, 0.3); color: var(--app-success-strong); }
.adh-pill--ok .adh-status-dot { background: var(--app-success); box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18); }
.adh-pill--warn { background: var(--app-warning-soft-bg); border-color: rgba(245, 158, 11, 0.3); color: var(--app-warning-strong); }
.adh-pill--warn .adh-status-dot { background: var(--app-warning); box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.18); }
.adh-pill--err { background: var(--app-danger-soft-bg); border-color: rgba(239, 68, 68, 0.3); color: var(--app-danger-strong); }
.adh-pill--err .adh-status-dot { background: var(--app-danger); box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.2); }
.adh-status-name { font-weight: 600; }
.adh-status-detail { color: inherit; opacity: 0.78; font-size: 12px; }

/* ─── Two-column charts grid ───────────────────────────────────────── */
.adh-chart-grid {
  display: grid;
  grid-template-columns: 2fr 1fr;
  gap: var(--app-page-gap, 20px);
}
@media (max-width: 980px) {
  .adh-chart-grid { grid-template-columns: 1fr; }
}

/* Network breakdown */
.adh-net-breakdown {
  display: grid;
  grid-template-columns: repeat(3, 1fr);
  gap: 8px;
  margin-bottom: 12px;
}
.adh-net-item {
  text-align: center;
  padding: 12px 8px;
  border-radius: 10px;
  background: var(--app-surface-muted);
  border: 1px solid var(--app-border);
}
.adh-net--ok { background: var(--app-success-soft-bg); border-color: rgba(16, 185, 129, 0.25); }
.adh-net--off { background: var(--app-danger-soft-bg); border-color: rgba(239, 68, 68, 0.22); }
.adh-net-label { display: block; font-size: 11px; color: var(--app-text-muted); margin-bottom: 4px; }
.adh-net-value { font-size: 22px; font-weight: 700; color: var(--app-text-strong); }
.adh-net--ok .adh-net-value { color: var(--app-success-strong); }
.adh-net--off .adh-net-value { color: var(--app-danger-strong); }

.adh-region-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 4px;
  max-height: 220px;
  overflow-y: auto;
}
.adh-region-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 6px 10px;
  border-radius: 8px;
  font-size: 13px;
  transition: background 0.15s;
}
.adh-region-item:hover { background: var(--app-surface-hover); }
.adh-region-name {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--app-text);
  font-weight: 500;
}
.adh-region-meta { display: inline-flex; align-items: center; gap: 12px; }
.adh-region-nodes { color: var(--app-text-muted); font-size: 12px; }
.adh-region-latency-good { color: var(--app-success-strong); font-weight: 600; font-size: 12px; }
.adh-region-latency-ok { color: var(--app-warning-strong); font-weight: 600; font-size: 12px; }
.adh-region-latency-warn { color: var(--app-danger-strong); font-weight: 600; font-size: 12px; }

.adh-traffic-total {
  display: flex;
  flex-direction: column;
  align-items: flex-end;
  gap: 0;
}
.adh-traffic-total-value {
  font-size: 18px;
  font-weight: 700;
  color: var(--app-brand-strong);
}

/* ─── Top domains table ────────────────────────────────────────────── */
.adh-table-wrap { overflow-x: auto; }
.adh-table {
  width: 100%;
  border-collapse: collapse;
  font-size: 13px;
}
.adh-table th {
  text-align: left;
  font-weight: 500;
  color: var(--app-text-muted);
  padding: 10px 12px;
  background: var(--app-surface-muted);
  border-bottom: 1px solid var(--app-border);
  font-size: 12px;
  letter-spacing: 0.3px;
}
.adh-table td {
  padding: 10px 12px;
  border-bottom: 1px solid var(--app-divider-soft);
  color: var(--app-text);
}
.adh-table tr:last-child td { border-bottom: none; }
.adh-table tbody tr { transition: background 0.15s; }
.adh-table tbody tr:hover { background: var(--app-surface-hover); }
.adh-rank {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 24px;
  height: 24px;
  padding: 0 8px;
  border-radius: 999px;
  background: var(--app-brand-soft-bg);
  color: var(--app-brand-strong);
  font-size: 12px;
  font-weight: 600;
}
.adh-domain-name { color: var(--app-text-strong); font-weight: 500; }
.adh-rate-good { color: var(--app-success-strong); font-weight: 600; }
.adh-rate-medium { color: var(--app-warning-strong); font-weight: 600; }
.adh-rate-poor { color: var(--app-danger-strong); font-weight: 600; }

/* ─── Info grid ────────────────────────────────────────────────────── */
.adh-info-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: var(--app-page-gap, 20px);
}
@media (max-width: 980px) { .adh-info-grid { grid-template-columns: 1fr; } }

.adh-progress-block {
  padding: 12px 14px;
  border-radius: 10px;
  background: var(--app-surface-muted);
  margin-bottom: 14px;
}
.adh-progress-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 13px;
  color: var(--app-text-muted);
  margin-bottom: 8px;
}
.adh-progress-pct {
  font-size: 18px;
  font-weight: 700;
}
.adh-progress-pct.ok { color: var(--app-success-strong); }
.adh-progress-pct.warn { color: var(--app-warning-strong); }
.adh-progress-pct.err { color: var(--app-danger-strong); }
.adh-progress-bar {
  height: 8px;
  border-radius: 999px;
  background: var(--app-divider);
  overflow: hidden;
}
.adh-progress-fill {
  height: 100%;
  border-radius: 999px;
  transition: width 0.4s var(--app-easing-standard);
}
.adh-progress-fill.ok { background: linear-gradient(90deg, #34d399, #10b981); }
.adh-progress-fill.warn { background: linear-gradient(90deg, #fcd34d, #f59e0b); }
.adh-progress-fill.err { background: linear-gradient(90deg, #fca5a5, #ef4444); }
.adh-progress-foot {
  margin-top: 6px;
  font-size: 11px;
  color: var(--app-text-faint);
}

.adh-info-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.adh-info-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  border-radius: 8px;
  font-size: 13px;
  transition: background 0.15s;
}
.adh-info-row:hover { background: var(--app-surface-hover); }
.adh-info-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  color: var(--app-text-muted);
}
.adh-info-value { color: var(--app-text-strong); font-weight: 500; }
.adh-mono { font-family: "JetBrains Mono", "Fira Code", Consolas, monospace; font-size: 12px; }

/* ─── Announcements + quick actions ────────────────────────────────── */
.adh-anno-list {
  list-style: none;
  margin: 0;
  padding: 8px 0;
}
.adh-anno-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--app-divider-soft);
}
.adh-anno-item:last-child { border-bottom: none; }
.adh-anno-bullet {
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  margin-top: 7px;
  border-radius: 50%;
  background: linear-gradient(135deg, #9385FE, #635BFF);
  box-shadow: 0 0 0 3px rgba(99, 91, 255, 0.12);
}
.adh-anno-body { flex: 1; min-width: 0; }
.adh-anno-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--app-text-strong);
  font-weight: 500;
}
.adh-anno-time { font-size: 11px; color: var(--app-text-faint); margin-top: 2px; }

.adh-quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
@media (max-width: 720px) { .adh-quick-grid { grid-template-columns: 1fr; } }

.adh-quick-tile {
  display: block;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--app-border);
  background: var(--app-surface);
  text-decoration: none;
  color: var(--app-text-strong);
  transition: transform 0.15s, border-color 0.15s, box-shadow 0.15s;
}
.adh-quick-tile:hover {
  transform: translateY(-2px);
  border-color: var(--app-border-strong);
  box-shadow: var(--app-shadow-card-hover);
}
.adh-quick-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}
.adh-quick-title { font-size: 14px; font-weight: 600; color: var(--app-text-strong); }
.adh-quick-desc { font-size: 12px; color: var(--app-text-muted); margin-top: 2px; }

/* ─── Install card ─────────────────────────────────────────────────── */
.adh-install-wrap {
  display: flex;
  flex-direction: column;
  gap: 10px;
}
.adh-install-section-title {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 13px;
  font-weight: 600;
  color: var(--app-text-strong);
  margin-top: 8px;
}
.adh-install-section-title:first-child { margin-top: 0; }
.adh-install-row,
.adh-install-script-row {
  display: grid;
  grid-template-columns: 130px minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  padding: 12px 14px;
  border: 1px solid var(--app-border);
  border-radius: 10px;
  background: var(--app-surface-muted);
  transition: border-color 0.15s;
}
.adh-install-row:hover,
.adh-install-script-row:hover { border-color: var(--app-border-strong); }
.adh-install-script-row {
  grid-template-columns: minmax(0, 1fr) auto;
}
.adh-install-label { font-size: 12px; font-weight: 600; color: var(--app-text-strong); }
.adh-install-value { font-size: 13px; color: var(--app-text); word-break: break-all; }
.adh-install-script {
  display: block;
  padding: 12px 14px;
  border: 1px dashed var(--app-border-strong);
  border-radius: 8px;
  background: var(--app-surface);
  color: var(--app-text);
  font-size: 12px;
  line-height: 1.6;
  font-family: "JetBrains Mono", "Fira Code", Consolas, monospace;
  white-space: pre-wrap;
  word-break: break-all;
}

@media (max-width: 720px) {
  .adh-install-row { grid-template-columns: 1fr; }
  .adh-install-script-row { grid-template-columns: 1fr; }
}

.adh-muted { color: var(--app-text-muted); }
.adh-empty {
  padding: 24px 18px;
  color: var(--app-text-faint);
  font-size: 13px;
  text-align: center;
}

.error-box {
  padding: 12px 16px;
  border-radius: 10px;
  background: var(--app-danger-soft-bg);
  color: var(--app-danger-strong);
  border: 1px solid rgba(239, 68, 68, 0.2);
}

/* admin-shared.css already provides .admin-desktop-only / .admin-mobile-only;
 * we keep those classes intact so the responsive table swap still works. */
</style>
