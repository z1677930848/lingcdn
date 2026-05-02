
<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">DDoS 防护</h1>
        <p class="subtitle">
          XDP 统计面板（自动刷新：{{ autoRefresh ? '开' : '关' }}）
          <span v-if="data?.generated_at">，生成时间：{{ formatDateTime(data.generated_at) }}</span>
          <span v-if="summary.latestXdpAt">，最新上报：{{ formatDateTime(summary.latestXdpAt) }}</span>
        </p>
      </div>
      <div class="header-actions">
        <t-switch v-model="autoRefresh" />
        <t-button :loading="loading" @click="load">刷新</t-button>
      </div>
    </div>

    <div class="stat-grid">
      <t-card class="section-card" bordered>
        <div class="section-body">
          <div class="stat-label">节点数（总）</div>
          <div class="stat-value">{{ summary.nodesTotal.toLocaleString('zh-CN') }}</div>
        </div>
      </t-card>
      <t-card class="section-card" bordered>
        <div class="section-body">
          <div class="stat-label">含 XDP 上报节点</div>
          <div class="stat-value">{{ summary.nodesWithXdp.toLocaleString('zh-CN') }}</div>
        </div>
      </t-card>
      <t-card class="section-card" bordered>
        <div class="section-body">
          <div class="stat-label">启用 XDP 节点</div>
          <div class="stat-value">{{ summary.nodesXdpEnabled.toLocaleString('zh-CN') }}</div>
        </div>
      </t-card>
      <t-card class="section-card" bordered>
        <div class="section-body">
          <div class="stat-label">总丢弃（估算）</div>
          <div class="stat-value">{{ derived.dropped.toLocaleString('zh-CN') }}</div>
          <div class="muted" style="margin-top:6px">丢弃率：{{ (derived.dropRate * 100).toFixed(2) }}%</div>
        </div>
      </t-card>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="section-title">数据面板</div>
        <div class="panel-grid">
          <div v-for="item in panelItems" :key="item.label" class="panel-item">
            <span class="panel-label">{{ item.label }}</span>
            <span class="panel-value">{{ formatNumber(item.value) }}</span>
            <span class="panel-sub">{{ item.sub }}</span>
          </div>
        </div>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="map-title">
          <div class="section-title">世界地图</div>
          <div class="muted">气泡大小 = 丢弃包</div>
        </div>
        <div class="map-layout">
          <div class="map-col">
            <WorldMap :points="regionData.points" :selected-id="selectedRegion?.key" :height="320" @select="handleSelectRegion" />
            <div class="map-footer">
              <span>数据按节点区域 GeoIP 估算</span>
              <span>{{ regionData.unknownRegions > 0 ? `未定位区域：${regionData.unknownRegions} 个` : '区域已全部定位' }}</span>
            </div>
          </div>
          <div class="detail-col">
            <div class="detail-title">区域详情</div>
            <div v-if="selectedRegion" class="region-card">
              <div class="region-name">{{ selectedRegion.name }}</div>
              <div class="region-grid">
                <span>节点数：{{ selectedRegion.nodesTotal.toLocaleString('zh-CN') }}</span>
                <span>上报节点：{{ selectedRegion.nodesWithXdp.toLocaleString('zh-CN') }}</span>
                <span>启用 XDP：{{ selectedRegion.nodesXdpEnabled.toLocaleString('zh-CN') }}</span>
                <span>丢弃率：{{ (selectedRegion.dropRate * 100).toFixed(2) }}%</span>
                <span>总包：{{ selectedRegion.total.toLocaleString('zh-CN') }}</span>
                <span>丢弃包：{{ selectedRegion.dropped.toLocaleString('zh-CN') }}</span>
              </div>
              <div class="top-nodes">
                <div class="muted">Top 节点</div>
                <div v-if="selectedRegion.topNodes.length">
                  <div v-for="node in selectedRegion.topNodes" :key="node.id" class="top-node">
                    <span class="node-name">{{ node.hostname }}</span>
                    <span>丢弃 {{ node.dropped.toLocaleString('zh-CN') }}</span>
                  </div>
                </div>
                <div v-else class="muted">暂无节点数据</div>
              </div>
            </div>
            <div v-else class="muted">暂无区域数据</div>

            <div class="top-region">
              <div class="muted">Top 区域</div>
              <div v-if="topRegions.length" class="top-region-list">
                <button
                  v-for="item in topRegions"
                  :key="item.key"
                  type="button"
                  class="region-item"
                  :class="{ active: item.key === selectedRegion?.key }"
                  @click="() => (selectedRegionKey = item.key)"
                >
                  <div class="region-item-title">{{ item.name }}</div>
                  <div class="region-item-meta">
                    <span>节点 {{ item.nodesTotal.toLocaleString('zh-CN') }}</span>
                    <span>丢弃 {{ item.dropped.toLocaleString('zh-CN') }}</span>
                  </div>
                </button>
              </div>
              <div v-else class="muted">暂无区域数据</div>
            </div>
          </div>
        </div>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="admin-desktop-only">
          <t-table
            :data="totalsRows"
            row-key="key"
            :columns="totalsColumns"
            bordered
            size="small"
          />
        </div>
        <div class="admin-mobile-only">
          <div v-if="totalsRows.length" class="admin-mobile-cards">
            <div v-for="row in totalsRows" :key="row.key" class="admin-mobile-card">
              <div class="admin-mobile-card-header">
                <div class="admin-mobile-card-title">{{ row.name }}</div>
              </div>
              <div class="admin-mobile-card-rows">
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">总计</span>
                  <span class="admin-mobile-card-value">{{ toSafeNumber(row.value).toLocaleString('zh-CN') }}</span>
                </div>
              </div>
            </div>
          </div>
          <div v-else class="admin-mobile-card-empty">暂无数据</div>
        </div>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="admin-desktop-only">
          <div class="table-scroll">
            <t-table
              :data="data?.items || []"
              :columns="columns"
              :row-key="(row: DdosXdpStatsItem) => row.node.id"
              :loading="loading"
              bordered
              size="small"
              :max-height="560"
            />
          </div>
        </div>
        <div class="admin-mobile-only">
          <div v-if="loading" style="text-align:center;padding:32px 0"><t-loading /></div>
          <template v-else>
            <div v-if="(data?.items || []).length" class="admin-mobile-cards">
              <div v-for="item in (data?.items || [])" :key="item.node.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div style="min-width:0">
                    <div class="admin-mobile-card-title">{{ item.node.hostname || item.node.id }}</div>
                    <div class="admin-mobile-card-subtitle">{{ item.node.public_ip || '-' }}</div>
                  </div>
                  <div class="admin-mobile-card-tags">
                    <t-tag v-if="item.node.status" variant="light" size="small">{{ item.node.status }}</t-tag>
                    <t-tag v-if="item.xdp" :theme="item.xdp.enabled ? 'success' : 'default'" variant="light" size="small">
                      {{ item.xdp.enabled ? 'XDP 启用' : 'XDP 未启用' }}
                    </t-tag>
                    <t-tag v-else variant="light" size="small">未上报</t-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">上报时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(item.xdp?.updated_at) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">总包</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'packets_total').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">放行包</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'packets_passed').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">黑名单丢弃</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'packets_dropped_blacklist').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">限速丢弃</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'packets_dropped_rate_limit').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">SYN Flood 丢弃</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'packets_dropped_syn_flood').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">TCP 包</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'tcp_packets').toLocaleString('zh-CN') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">UDP 包</span>
                    <span class="admin-mobile-card-value">{{ getStat(item, 'udp_packets').toLocaleString('zh-CN') }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-else class="admin-mobile-card-empty">暂无数据</div>
          </template>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref, watch } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import WorldMap from "@/components/charts/WorldMap.vue"
import { api, type DdosXdpStatsItem, type DdosXdpStatsResponse } from "@/lib/api"

const statLabels: Record<string, string> = {
  packets_total: "总包",
  packets_passed: "放行包",
  packets_dropped_blacklist: "黑名单丢弃",
  packets_whitelisted: "白名单放行",
  packets_non_ip: "非 IP 包",
  packets_dropped_rate_limit: "限速丢弃",
  packets_dropped_syn_flood: "SYN Flood 丢弃",
  packets_dropped_invalid: "非法包丢弃",
  syn_packets: "SYN 包",
  udp_packets: "UDP 包",
  tcp_packets: "TCP 包",
  icmp_packets: "ICMP 包",
}

const statOrder = [
  "packets_total",
  "packets_passed",
  "packets_dropped_blacklist",
  "packets_dropped_rate_limit",
  "packets_dropped_syn_flood",
  "packets_dropped_invalid",
  "packets_whitelisted",
  "packets_non_ip",
  "syn_packets",
  "tcp_packets",
  "udp_packets",
  "icmp_packets",
]

type RegionHint = {
  key: string
  label: string
  lat: number
  lon: number
  keywords: string[]
}

type RegionNodeMetric = {
  id: string
  hostname: string
  ip?: string
  total: number
  dropped: number
}

type RegionStat = {
  key: string
  name: string
  lat?: number
  lon?: number
  nodesTotal: number
  nodesWithXdp: number
  nodesXdpEnabled: number
  sum: Record<string, number>
  nodes: RegionNodeMetric[]
}

type RegionDerived = RegionStat & {
  total: number
  dropped: number
  passed: number
  dropRate: number
  topNodes: RegionNodeMetric[]
}

const REGION_HINTS: RegionHint[] = [
  { key: "北京", label: "北京", lat: 39.9042, lon: 116.4074, keywords: ["北京", "beijing", "bj"] },
  { key: "上海", label: "上海", lat: 31.2304, lon: 121.4737, keywords: ["上海", "shanghai", "sh"] },
  { key: "广州", label: "广州", lat: 23.1291, lon: 113.2644, keywords: ["广州", "guangzhou", "gz"] },
  { key: "深圳", label: "深圳", lat: 22.5431, lon: 114.0579, keywords: ["深圳", "shenzhen", "sz"] },
  { key: "杭州", label: "杭州", lat: 30.2741, lon: 120.1551, keywords: ["杭州", "hangzhou", "hz"] },
  { key: "成都", label: "成都", lat: 30.5728, lon: 104.0668, keywords: ["成都", "chengdu", "cd"] },
  { key: "重庆", label: "重庆", lat: 29.4316, lon: 106.9123, keywords: ["重庆", "chongqing", "cq"] },
  { key: "西安", label: "西安", lat: 34.3416, lon: 108.9398, keywords: ["西安", "xian", "xi'an"] },
  { key: "香港", label: "香港", lat: 22.3193, lon: 114.1694, keywords: ["香港", "hong kong", "hk"] },
  { key: "台湾", label: "台湾", lat: 23.6978, lon: 120.9605, keywords: ["台湾", "taiwan", "tw"] },
  { key: "澳门", label: "澳门", lat: 22.1987, lon: 113.5439, keywords: ["澳门", "macau", "mo"] },
  { key: "华北", label: "华北", lat: 39.5, lon: 115.5, keywords: ["华北"] },
  { key: "华东", label: "华东", lat: 31.2, lon: 121.0, keywords: ["华东"] },
  { key: "华南", label: "华南", lat: 23.0, lon: 113.0, keywords: ["华南"] },
  { key: "华中", label: "华中", lat: 30.6, lon: 114.3, keywords: ["华中"] },
  { key: "东北", label: "东北", lat: 43.9, lon: 125.3, keywords: ["东北"] },
  { key: "西南", label: "西南", lat: 29.5, lon: 104.0, keywords: ["西南"] },
  { key: "西北", label: "西北", lat: 36.1, lon: 103.8, keywords: ["西北"] },
  { key: "中国", label: "中国", lat: 35.8617, lon: 104.1954, keywords: ["中国", "china", "cn"] },
  { key: "日本", label: "日本", lat: 36.2048, lon: 138.2529, keywords: ["日本", "japan", "jp"] },
  { key: "韩国", label: "韩国", lat: 35.9078, lon: 127.7669, keywords: ["韩国", "korea", "kr"] },
  { key: "新加坡", label: "新加坡", lat: 1.3521, lon: 103.8198, keywords: ["新加坡", "singapore", "sg"] },
  { key: "印度", label: "印度", lat: 20.5937, lon: 78.9629, keywords: ["印度", "india", "in"] },
  { key: "越南", label: "越南", lat: 14.0583, lon: 108.2772, keywords: ["越南", "vietnam", "vn"] },
  { key: "泰国", label: "泰国", lat: 15.87, lon: 100.9925, keywords: ["泰国", "thailand", "th"] },
  { key: "印尼", label: "印尼", lat: -0.7893, lon: 113.9213, keywords: ["印尼", "indonesia", "id"] },
  { key: "菲律宾", label: "菲律宾", lat: 12.8797, lon: 121.774, keywords: ["菲律宾", "philippines", "ph"] },
  { key: "阿联酋", label: "阿联酋", lat: 23.4241, lon: 53.8478, keywords: ["阿联酋", "uae", "united arab emirates", "ae"] },
  { key: "土耳其", label: "土耳其", lat: 38.9637, lon: 35.2433, keywords: ["土耳其", "turkey", "tr"] },
  { key: "俄罗斯", label: "俄罗斯", lat: 61.524, lon: 105.3188, keywords: ["俄罗斯", "russia", "ru"] },
  { key: "澳大利亚", label: "澳大利亚", lat: -25.2744, lon: 133.7751, keywords: ["澳大利亚", "australia", "au"] },
  { key: "新西兰", label: "新西兰", lat: -40.9006, lon: 174.886, keywords: ["新西兰", "new zealand", "nz"] },
  { key: "美国", label: "美国", lat: 37.0902, lon: -95.7129, keywords: ["美国", "united states", "usa", "us"] },
  { key: "加拿大", label: "加拿大", lat: 56.1304, lon: -106.3468, keywords: ["加拿大", "canada", "ca"] },
  { key: "墨西哥", label: "墨西哥", lat: 23.6345, lon: -102.5528, keywords: ["墨西哥", "mexico", "mx"] },
  { key: "巴西", label: "巴西", lat: -14.235, lon: -51.9253, keywords: ["巴西", "brazil", "br"] },
  { key: "阿根廷", label: "阿根廷", lat: -38.4161, lon: -63.6167, keywords: ["阿根廷", "argentina", "ar"] },
  { key: "英国", label: "英国", lat: 55.3781, lon: -3.436, keywords: ["英国", "united kingdom", "uk", "gb"] },
  { key: "德国", label: "德国", lat: 51.1657, lon: 10.4515, keywords: ["德国", "germany", "de"] },
  { key: "法国", label: "法国", lat: 46.2276, lon: 2.2137, keywords: ["法国", "france", "fr"] },
  { key: "荷兰", label: "荷兰", lat: 52.1326, lon: 5.2913, keywords: ["荷兰", "netherlands", "nl"] },
  { key: "西班牙", label: "西班牙", lat: 40.4637, lon: -3.7492, keywords: ["西班牙", "spain", "es"] },
  { key: "意大利", label: "意大利", lat: 41.8719, lon: 12.5674, keywords: ["意大利", "italy", "it"] },
  { key: "南非", label: "南非", lat: -30.5595, lon: 22.9375, keywords: ["南非", "south africa", "za"] },
]

const readNumber = (value: any) => {
  if (typeof value === "number" && !Number.isNaN(value)) return value
  if (typeof value === "string" && value.trim()) {
    const n = Number(value)
    if (!Number.isNaN(n)) return n
  }
  return null
}

const toSafeNumber = (value: any) => {
  const n = readNumber(value)
  return n == null ? 0 : n
}

const sanitizeDateTime = (value: any) => {
  const text = String(value || "").trim()
  if (!text || text.startsWith("0001-01-01")) return ""
  return Number.isNaN(Date.parse(text)) ? "" : text
}

const matchRegion = (raw: string) => {
  if (!raw) return null
  const hay = raw.toLowerCase()
  for (const hint of REGION_HINTS) {
    for (const kw of hint.keywords) {
      if (kw && hay.includes(kw.toLowerCase())) return hint
    }
  }
  return null
}

const pickNodeGeo = (node: any) => {
  const lat =
    readNumber(node?.lat) ??
    readNumber(node?.latitude) ??
    readNumber(node?.geo?.lat) ??
    readNumber(node?.geo?.latitude) ??
    readNumber(node?.location?.lat) ??
    readNumber(node?.location?.latitude)
  const lon =
    readNumber(node?.lon) ??
    readNumber(node?.lng) ??
    readNumber(node?.longitude) ??
    readNumber(node?.geo?.lon) ??
    readNumber(node?.geo?.lng) ??
    readNumber(node?.geo?.longitude) ??
    readNumber(node?.location?.lon) ??
    readNumber(node?.location?.lng) ??
    readNumber(node?.location?.longitude)
  if (lat == null || lon == null) return null
  const name = node?.geo?.name || node?.location?.name || node?.city || node?.region || node?.country || node?.country_code || ""
  return { lat, lon, name: String(name || "") }
}

const getStat = (item: DdosXdpStatsItem, key: string) => {
  const v = item.xdp?.stats?.[key]
  return toSafeNumber(v)
}

const getDroppedFromSum = (sum: Record<string, number>) => {
  return (
    (sum.packets_dropped_blacklist || 0) +
    (sum.packets_dropped_rate_limit || 0) +
    (sum.packets_dropped_syn_flood || 0) +
    (sum.packets_dropped_invalid || 0)
  )
}

const getDroppedFromItem = (item: DdosXdpStatsItem) => {
  return (
    getStat(item, "packets_dropped_blacklist") +
    getStat(item, "packets_dropped_rate_limit") +
    getStat(item, "packets_dropped_syn_flood") +
    getStat(item, "packets_dropped_invalid")
  )
}

const loading = ref(false)
const autoRefresh = ref(true)
const data = ref<DdosXdpStatsResponse | null>(null)
const selectedRegionKey = ref("")

const load = async () => {
  try {
    loading.value = true
    const res = await api.getDdosXdpStats()
    data.value = res
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载 XDP 数据失败")
  } finally {
    loading.value = false
  }
}

const summary = computed(() => {
  if (data.value?.summary) {
    const totals = data.value.summary.totals || {}
    const sum: Record<string, number> = {}
    for (const key of statOrder) sum[key] = toSafeNumber((totals as Record<string, any>)[key])
    return {
      nodesTotal: Math.max(0, Math.trunc(toSafeNumber(data.value.summary.nodes_total))),
      nodesWithXdp: Math.max(0, Math.trunc(toSafeNumber(data.value.summary.nodes_with_xdp))),
      nodesXdpEnabled: Math.max(0, Math.trunc(toSafeNumber(data.value.summary.nodes_xdp_enabled))),
      latestXdpAt: sanitizeDateTime(data.value.summary.latest_xdp_at),
      sum,
    }
  }
  const items = data.value?.items || []
  const sum: Record<string, number> = {}
  for (const key of statOrder) sum[key] = 0
  for (const it of items) {
    for (const key of statOrder) sum[key] += getStat(it, key)
  }
  const nodesXdpEnabled = items.filter((x) => x.xdp?.enabled).length
  const nodesWithXdp = items.filter((x) => !!x.xdp).length
  return { nodesTotal: items.length, nodesWithXdp, nodesXdpEnabled, latestXdpAt: "", sum }
})

const totalsRows = computed(() =>
  statOrder.map((key) => ({
    key,
    name: statLabels[key] || key,
    value: summary.value.sum[key] || 0,
  }))
)

const derived = computed(() => {
  const total = summary.value.sum.packets_total || 0
  const dropped = getDroppedFromSum(summary.value.sum)
  const dropRate = total > 0 ? dropped / total : 0
  return { dropped, dropRate }
})

const panelItems = computed(() => {
  const total = summary.value.sum.packets_total || 0
  const passed = summary.value.sum.packets_passed || 0
  const dropRate = `${(derived.value.dropRate * 100).toFixed(2)}%`
  return [
    { label: "总包", value: total, sub: "全站流量" },
    { label: "放行包", value: passed, sub: "XDP 放行" },
    { label: "丢弃包", value: derived.value.dropped, sub: `丢弃率：${dropRate}` },
    { label: "黑名单丢弃", value: summary.value.sum.packets_dropped_blacklist || 0, sub: "名单拦截" },
    { label: "限速丢弃", value: summary.value.sum.packets_dropped_rate_limit || 0, sub: "速率限制" },
    { label: "SYN Flood 丢弃", value: summary.value.sum.packets_dropped_syn_flood || 0, sub: "SYN 异常" },
    { label: "TCP 包", value: summary.value.sum.tcp_packets || 0, sub: "协议分布" },
    { label: "UDP 包", value: summary.value.sum.udp_packets || 0, sub: "协议分布" },
  ]
})

const regionData = computed(() => {
  const items = data.value?.items || []
  const map = new Map<string, RegionStat>()

  for (const item of items) {
    const nodeAny = item.node as any
    const directGeo = pickNodeGeo(nodeAny)
    let key = ""
    let name = ""
    let lat: number | undefined
    let lon: number | undefined

    if (directGeo) {
      key = directGeo.name || nodeAny.region || nodeAny.cluster || nodeAny.public_ip || nodeAny.id || "未知"
      name = directGeo.name || key
      lat = directGeo.lat
      lon = directGeo.lon
    } else {
      const raw = String(nodeAny.region || nodeAny.cluster || nodeAny.country || nodeAny.city || "").trim()
      const match = matchRegion(raw)
      if (match) {
        key = match.key
        name = match.label
        lat = match.lat
        lon = match.lon
      } else {
        key = raw || "未知"
        name = raw || "未知"
      }
    }

    const entry = map.get(key) || {
      key,
      name,
      lat,
      lon,
      nodesTotal: 0,
      nodesWithXdp: 0,
      nodesXdpEnabled: 0,
      sum: {},
      nodes: [],
    }

    if (lat != null && lon != null && (entry.lat == null || entry.lon == null)) {
      entry.lat = lat
      entry.lon = lon
    }

    entry.nodesTotal += 1
    if (item.xdp) entry.nodesWithXdp += 1
    if (item.xdp?.enabled) entry.nodesXdpEnabled += 1
    for (const statKey of statOrder) {
      entry.sum[statKey] = (entry.sum[statKey] || 0) + getStat(item, statKey)
    }
    entry.nodes.push({
      id: item.node.id,
      hostname: item.node.hostname || item.node.id,
      ip: item.node.public_ip,
      total: getStat(item, "packets_total"),
      dropped: getDroppedFromItem(item),
    })

    map.set(key, entry)
  }

  const list: RegionDerived[] = Array.from(map.values()).map((entry) => {
    const total = entry.sum.packets_total || 0
    const dropped = getDroppedFromSum(entry.sum)
    const passed = entry.sum.packets_passed || 0
    const dropRate = total > 0 ? dropped / total : 0
    const topNodes = [...entry.nodes].sort((a, b) => b.dropped - a.dropped).slice(0, 3)
    return { ...entry, total, dropped, passed, dropRate, topNodes }
  })

  const points = list
    .filter((item) => typeof item.lat === "number" && typeof item.lon === "number")
    .map((item) => ({
      id: item.key,
      name: item.name,
      lat: item.lat as number,
      lon: item.lon as number,
      value: item.dropped || item.total || 0,
      total: item.total,
      dropped: item.dropped,
      nodes: item.nodesTotal,
    }))

  const unknownRegions = list.filter((item) => typeof item.lat !== "number" || typeof item.lon !== "number").length

  return { list, points, unknownRegions }
})

const regionList = computed(() => {
  return [...regionData.value.list].sort((a, b) => {
    if (b.dropped !== a.dropped) return b.dropped - a.dropped
    if (b.total !== a.total) return b.total - a.total
    return b.nodesTotal - a.nodesTotal
  })
})

const topRegions = computed(() => regionList.value.slice(0, 6))

const selectedRegion = computed(() => {
  if (!regionList.value.length) return null
  return regionList.value.find((r) => r.key === selectedRegionKey.value) || regionList.value[0]
})

watch([regionList, selectedRegionKey], () => {
  if (regionList.value.length === 0) {
    if (selectedRegionKey.value) selectedRegionKey.value = ""
    return
  }
  if (!selectedRegionKey.value || !regionList.value.find((r) => r.key === selectedRegionKey.value)) {
    selectedRegionKey.value = regionList.value[0].key
  }
})

const handleSelectRegion = (id: string) => {
  selectedRegionKey.value = id
}

const columns = computed(() => {
  const base = [
    {
      colKey: "hostname",
      title: "节点",
      width: 180,
      cell: (_h: any, { row }: { row: DdosXdpStatsItem }) =>
        h("div", { style: "display:flex;flex-direction:column" }, [
          h("span", { style: "font-weight:600" }, row.node.hostname || row.node.id),
          h("span", { class: "muted" }, row.node.public_ip || "-"),
        ]),
    },
    {
      colKey: "status",
      title: "状态",
      width: 90,
      cell: (_h: any, { row }: { row: DdosXdpStatsItem }) => row.node.status || "-",
    },
    {
      colKey: "xdp",
      title: "XDP",
      width: 140,
      cell: (_h: any, { row }: { row: DdosXdpStatsItem }) => {
        if (!row.xdp) return h("span", { class: "muted" }, "未上报")
        return h("div", { style: "display:flex;flex-direction:column" }, [
          h("span", { style: "font-weight:600" }, row.xdp.enabled ? "已启用" : "未启用"),
          h("span", { class: "muted" }, row.xdp.interface || "-"),
        ])
      },
    },
    {
      colKey: "updated_at",
      title: "上报时间",
      width: 180,
      cell: (_h: any, { row }: { row: DdosXdpStatsItem }) =>
        formatDateTime(row.xdp?.updated_at),
    },
  ]

  const statCols = statOrder.map((key) => ({
    colKey: key,
    title: statLabels[key] || key,
    width: 120,
    align: "right" as const,
    cell: (_h: any, { row }: { row: DdosXdpStatsItem }) => getStat(row, key).toLocaleString("zh-CN"),
  }))

  return [...base, ...statCols]
})

const totalsColumns = [
  { colKey: "name", title: "指标", width: 160 },
  {
    colKey: "value",
    title: "总计",
    align: "right" as const,
    cell: (ctx: { row?: { value?: number } }) => toSafeNumber(ctx?.row?.value).toLocaleString("zh-CN"),
  },
]

const formatDateTime = (value?: string) => {
  const clean = sanitizeDateTime(value)
  if (!clean) return "-"
  return new Date(clean).toLocaleString("zh-CN")
}

const formatNumber = (value: number) => {
  return typeof value === "number" ? value.toLocaleString("zh-CN") : String(value || 0)
}

let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (autoRefresh.value) load()
  }, 8000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.header-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: 12px;
}

@media (max-width: 1200px) {
  .stat-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

@media (max-width: 640px) {
  .stat-grid {
    grid-template-columns: 1fr;
  }
}

.stat-label {
  font-size: 12px;
  color: #94a3b8;
}

.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}

.section-title {
  font-size: 14px;
  font-weight: 600;
  margin-bottom: 12px;
}

.panel-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 12px;
}

.panel-item {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 12px 14px;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.panel-label {
  font-size: 12px;
  color: #94a3b8;
}

.panel-value {
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
}

.panel-sub {
  font-size: 12px;
  color: #475569;
}

.map-title {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 12px;
}

.map-col {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.map-footer {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  color: #94a3b8;
  flex-wrap: wrap;
}

.detail-col {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-title {
  font-size: 13px;
  font-weight: 600;
}

.region-card {
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 12px;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.region-name {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.region-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px;
  font-size: 12px;
  color: #475569;
}

.top-nodes {
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.top-node {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  color: #475569;
}

.node-name {
  font-weight: 600;
  color: #0f172a;
}

.top-region {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.top-region-list {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.region-item {
  border: 1px solid #e2e8f0;
  background: #fff;
  border-radius: 8px;
  padding: 8px 10px;
  text-align: left;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.region-item.active {
  border-color: #7aa2ff;
  background: #eef3ff;
}

.region-item-title {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
}

.region-item-meta {
  display: flex;
  justify-content: space-between;
  gap: 8px;
  font-size: 12px;
  color: #475569;
}

.table-scroll {
  width: 100%;
  overflow-x: auto;
}

.table-scroll :deep(.t-table) {
  min-width: 1600px;
}

@media (max-width: 640px) {
  .region-grid {
    grid-template-columns: 1fr;
  }
}
</style>


