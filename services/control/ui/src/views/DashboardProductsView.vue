<template>
  <LoadingState v-if="loading" />
  <ErrorState v-else-if="error" :message="error" @retry="loadProducts" />
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">产品套餐</h1>
        <p class="subtitle">共 {{ products.length }} 个可选套餐</p>
      </div>
    </div>

    <PermissionNotice :show="hasRestrictions && !canOrdersPurchase" />

    <el-card v-if="products.length === 0" class="section-card">
      <div class="empty">
        <p class="muted" style="margin:0">暂无可购买的套餐</p>
      </div>
    </el-card>

    <div v-else class="product-grid">
      <el-card v-for="product in products" :key="product.id" class="section-card">
        <div class="section-body">
          <div class="product-head">
            <div>
              <h2 class="product-name">{{ product.name }}</h2>
              <p class="product-desc">{{ product.description || '-' }}</p>
            </div>
            <span class="product-status">可购买</span>
          </div>

          <div class="product-meta">
            <div class="meta-row" v-if="product.monthly_traffic_bytes">
              <span class="meta-label">月流量</span>
              <span class="meta-value">{{ formatTraffic(product.monthly_traffic_bytes) }}</span>
            </div>
            <div class="meta-row" v-if="product.bandwidth_bps">
              <span class="meta-label">带宽峰值</span>
              <span class="meta-value">{{ formatBandwidth(product.bandwidth_bps) }}</span>
            </div>
            <div class="meta-row" v-if="product.domain_limit">
              <span class="meta-label">域名数</span>
              <span class="meta-value">{{ product.domain_limit }} 个</span>
            </div>
            <div class="meta-row">
              <span class="meta-label">月付价格</span>
              <span class="meta-value price">{{ formatMoney(product.price_month_cents, product.currency) }}</span>
            </div>
            <div class="meta-row" v-if="product.price_quarter_cents">
              <span class="meta-label">季付价格</span>
              <span class="meta-value">{{ formatMoney(product.price_quarter_cents, product.currency) }}</span>
            </div>
            <div class="meta-row" v-if="product.price_year_cents">
              <span class="meta-label">年付价格</span>
              <span class="meta-value">{{ formatMoney(product.price_year_cents, product.currency) }}</span>
            </div>
          </div>

          <div class="product-actions">
            <el-button size="small" plain @click="openDetail(product)">查看详情</el-button>
            <el-button v-if="canOrdersPurchase" size="small" type="primary" :disabled="Boolean(getRenewalBlockedReason(product))" @click="openBuy(product)">
              {{ getProductActionLabel(product) }}
            </el-button>
          </div>
          <div v-if="getRenewalBlockedReason(product)" class="renewal-hint">{{ getRenewalBlockedReason(product) }}</div>
        </div>
      </el-card>
    </div>

    <!-- Detail Dialog -->
    <EpDialog append-to-body v-model="detailVisible" :title="detailProduct?.name || '套餐详情'" :footer="false" width="540px">
      <div v-if="detailProduct" class="detail-content">
        <div class="detail-section">
          <div class="detail-row"><span class="detail-label">名称</span><span>{{ detailProduct.name }}</span></div>
          <div class="detail-row"><span class="detail-label">描述</span><span>{{ detailProduct.description || "-" }}</span></div>
          <div class="detail-row" v-if="detailProduct.slug"><span class="detail-label">标识</span><span class="mono">{{ detailProduct.slug }}</span></div>
          <div class="detail-row" v-if="detailProduct.region"><span class="detail-label">区域</span><span>{{ detailProduct.region }}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">规格参数</div>
          <div class="detail-row" v-if="detailProduct.monthly_traffic_bytes"><span class="detail-label">月流量</span><span>{{ formatTraffic(detailProduct.monthly_traffic_bytes) }}</span></div>
          <div class="detail-row" v-if="detailProduct.bandwidth_bps"><span class="detail-label">带宽峰值</span><span>{{ formatBandwidth(detailProduct.bandwidth_bps) }}</span></div>
          <div class="detail-row" v-if="detailProduct.conn_limit"><span class="detail-label">并发连接</span><span>{{ detailProduct.conn_limit }}</span></div>
          <div class="detail-row" v-if="detailProduct.domain_limit"><span class="detail-label">域名数</span><span>{{ detailProduct.domain_limit }}</span></div>
          <div class="detail-row" v-if="detailProduct.primary_domain_limit"><span class="detail-label">主域名数</span><span>{{ detailProduct.primary_domain_limit }}</span></div>
          <div class="detail-row" v-if="detailProduct.stream_port_limit"><span class="detail-label">L4 端口数</span><span>{{ detailProduct.stream_port_limit }}</span></div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">功能特性</div>
          <div class="detail-features">
            <el-tag v-if="detailProduct.websocket" type="primary" effect="light" size="small">WebSocket</el-tag>
            <el-tag v-if="detailProduct.http3" type="primary" effect="light" size="small">HTTP/3</el-tag>
            <el-tag v-if="detailProduct.custom_cc_rules" type="primary" effect="light" size="small">自定义 CC 规则</el-tag>
            <el-tag v-if="detailProduct.l2_origin" type="primary" effect="light" size="small">L2 回源</el-tag>
            <el-tag v-if="detailProduct.cc_protection" type="primary" effect="light" size="small">CC 防护: {{ detailProduct.cc_protection }}</el-tag>
            <el-tag v-if="detailProduct.ddos_protection" type="primary" effect="light" size="small">DDoS 防护: {{ detailProduct.ddos_protection }}</el-tag>
          </div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">价格</div>
          <div class="detail-row"><span class="detail-label">月付</span><span class="price">{{ formatMoney(detailProduct.price_month_cents, detailProduct.currency) }}</span></div>
          <div class="detail-row" v-if="detailProduct.price_quarter_cents"><span class="detail-label">季付</span><span>{{ formatMoney(detailProduct.price_quarter_cents, detailProduct.currency) }}</span></div>
          <div class="detail-row" v-if="detailProduct.price_year_cents"><span class="detail-label">年付</span><span>{{ formatMoney(detailProduct.price_year_cents, detailProduct.currency) }}</span></div>
        </div>
      </div>
    </EpDialog>

    <!-- Buy Dialog -->
    <EpDialog append-to-body v-model="buyVisible" header="购买套餐" :confirm-btn="{ content: '确认购买', loading: buying }" @confirm="handleBuy">
      <div v-if="buyProduct" class="buy-form">
        <div class="buy-product-name">{{ buyProduct.name }}</div>
        <div class="form-item">
          <div class="form-label">选择周期</div>
          <el-radio-group v-model="buyPeriod">
            <el-radio value="month">月付 {{ formatMoney(buyProduct.price_month_cents, buyProduct.currency) }}</el-radio>
            <el-radio v-if="buyProduct.price_quarter_cents" value="quarter">季付 {{ formatMoney(buyProduct.price_quarter_cents, buyProduct.currency) }}</el-radio>
            <el-radio v-if="buyProduct.price_year_cents" value="year">年付 {{ formatMoney(buyProduct.price_year_cents, buyProduct.currency) }}</el-radio>
          </el-radio-group>
        </div>
        <div class="buy-total">
          <span>应付总额</span>
          <span class="buy-price">{{ formatMoney(buyAmount, buyProduct.currency) }}</span>
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type Product, type UserOrder } from "@/lib/api"
import { useSystemSettings } from "@/lib/systemSettings"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"
import PermissionNotice from "@/components/common/PermissionNotice.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"

const { canOrdersPurchase, hasRestrictions } = useUserPermissions()

const products = ref<Product[]>([])
const orders = ref<UserOrder[]>([])
const loading = ref(true)
const error = ref("")
const { settings } = useSystemSettings()

const detailVisible = ref(false)
const detailProduct = ref<Product | null>(null)

const buyVisible = ref(false)
const buyProduct = ref<Product | null>(null)
const buyPeriod = ref("month")
const buying = ref(false)

const loadProducts = async () => {
  try {
    loading.value = true
    const [productsRes, ordersRes] = await Promise.all([
      api.getProducts(),
      api.getUserOrders().catch(() => ({ orders: [], total: 0 })),
    ])
    products.value = (productsRes.products || []).filter((p) => p.enabled !== false)
    orders.value = ordersRes.orders || []
    error.value = ""
  } catch (err: any) {
    error.value = err.message || "加载产品失败"
  } finally {
    loading.value = false
  }
}

const openDetail = (product: Product) => {
  detailProduct.value = product
  detailVisible.value = true
}

const openBuy = (product: Product) => {
  const blockedReason = getRenewalBlockedReason(product)
  if (blockedReason) {
    MessagePlugin.warning(blockedReason)
    return
  }
  buyProduct.value = product
  buyPeriod.value = "month"
  buyVisible.value = true
}

const renewalWindowDays = computed(() => {
  const raw = Number(settings.value.renewal_before_expiry_days)
  if (!Number.isFinite(raw)) return 30
  return Math.max(0, Math.min(3650, Math.floor(raw)))
})

const latestProductOrderEnd = (productID: string): Date | null => {
  let latest: Date | null = null
  for (const order of orders.value) {
    const status = String(order.status || "").toLowerCase()
    if (order.product_id !== productID || (status !== "paid" && status !== "active") || !order.ends_at) continue
    const end = new Date(order.ends_at)
    if (Number.isNaN(end.getTime())) continue
    if (!latest || end > latest) latest = end
  }
  return latest
}

const getRenewalBlockedReason = (product: Product) => {
  const end = latestProductOrderEnd(product.id)
  if (!end) return ""
  const now = new Date()
  const allowAt = new Date(end.getTime() - renewalWindowDays.value * 24 * 60 * 60 * 1000)
  if (allowAt <= now) return ""
  const days = Math.max(1, Math.ceil((allowAt.getTime() - now.getTime()) / (24 * 60 * 60 * 1000)))
  return `距当前套餐到期超过 ${renewalWindowDays.value} 天，约 ${days} 天后可续费`
}

const getProductActionLabel = (product: Product) => {
  return latestProductOrderEnd(product.id) ? "立即续费" : "立即购买"
}

const buyAmount = computed(() => {
  if (!buyProduct.value) return 0
  const p = buyProduct.value
  switch (buyPeriod.value) {
    case "quarter": return p.price_quarter_cents || 0
    case "year": return p.price_year_cents || 0
    default: return p.price_month_cents || p.price_cents || 0
  }
})

const handleBuy = async () => {
  if (!buyProduct.value) return
  buying.value = true
  try {
    await api.createUserOrder({
      product_id: buyProduct.value.id,
      period: buyPeriod.value,
    })
    MessagePlugin.success("下单成功")
    buyVisible.value = false
    await loadProducts()
  } catch (err: any) {
    MessagePlugin.error(err.message || "下单失败")
  } finally {
    buying.value = false
  }
}

const formatMoney = (cents?: number, currency?: string) => {
  const value = typeof cents === "number" ? cents / 100 : 0
  const cur = currency || "CNY"
  return cur === "CNY" ? `¥${value.toFixed(2)}` : `${value.toFixed(2)} ${cur}`
}

const formatTraffic = (bytes?: number | null) => {
  if (!bytes) return "-"
  const gb = bytes / (1024 * 1024 * 1024)
  if (gb >= 1024) return `${(gb / 1024).toFixed(0)} TB`
  return `${gb.toFixed(0)} GB`
}

const formatBandwidth = (bps?: number | null) => {
  if (!bps) return "-"
  const mbps = bps / 1000000
  if (mbps >= 1000) return `${(mbps / 1000).toFixed(0)} Gbps`
  return `${mbps.toFixed(0)} Mbps`
}

onMounted(() => { loadProducts() })
</script>

