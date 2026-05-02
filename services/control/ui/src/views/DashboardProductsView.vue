<template>
  <div v-if="loading" class="loading">
    <t-loading size="medium" />
  </div>
  <div v-else-if="error" class="page">
    <div class="error-box">
      <span>{{ error }}</span>
      <t-button size="small" theme="primary" @click="loadProducts">重试</t-button>
    </div>
  </div>
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">产品套餐</h1>
        <p class="subtitle">共 {{ products.length }} 个可选套餐</p>
      </div>
    </div>

    <t-card v-if="products.length === 0" class="section-card" bordered>
      <div class="empty">
        <p class="muted" style="margin:0">暂无可购买的套餐</p>
      </div>
    </t-card>

    <div v-else class="product-grid">
      <t-card v-for="product in products" :key="product.id" class="section-card" bordered>
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
            <t-button size="small" variant="outline" @click="openDetail(product)">查看详情</t-button>
            <t-button size="small" theme="primary" @click="openBuy(product)">立即购买</t-button>
          </div>
        </div>
      </t-card>
    </div>

    <!-- Detail Dialog -->
    <t-dialog v-model:visible="detailVisible" :header="detailProduct?.name || '套餐详情'" :footer="false" width="540px">
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
            <t-tag v-if="detailProduct.websocket" theme="primary" variant="light" size="small">WebSocket</t-tag>
            <t-tag v-if="detailProduct.http3" theme="primary" variant="light" size="small">HTTP/3</t-tag>
            <t-tag v-if="detailProduct.custom_cc_rules" theme="primary" variant="light" size="small">自定义 CC 规则</t-tag>
            <t-tag v-if="detailProduct.l2_origin" theme="primary" variant="light" size="small">L2 回源</t-tag>
            <t-tag v-if="detailProduct.cc_protection" theme="primary" variant="light" size="small">CC 防护: {{ detailProduct.cc_protection }}</t-tag>
            <t-tag v-if="detailProduct.ddos_protection" theme="primary" variant="light" size="small">DDoS 防护: {{ detailProduct.ddos_protection }}</t-tag>
          </div>
        </div>
        <div class="detail-section">
          <div class="detail-section-title">价格</div>
          <div class="detail-row"><span class="detail-label">月付</span><span class="price">{{ formatMoney(detailProduct.price_month_cents, detailProduct.currency) }}</span></div>
          <div class="detail-row" v-if="detailProduct.price_quarter_cents"><span class="detail-label">季付</span><span>{{ formatMoney(detailProduct.price_quarter_cents, detailProduct.currency) }}</span></div>
          <div class="detail-row" v-if="detailProduct.price_year_cents"><span class="detail-label">年付</span><span>{{ formatMoney(detailProduct.price_year_cents, detailProduct.currency) }}</span></div>
        </div>
      </div>
    </t-dialog>

    <!-- Buy Dialog -->
    <t-dialog v-model:visible="buyVisible" header="购买套餐" :confirm-btn="{ content: '确认购买', loading: buying }" @confirm="handleBuy">
      <div v-if="buyProduct" class="buy-form">
        <div class="buy-product-name">{{ buyProduct.name }}</div>
        <div class="form-item">
          <div class="form-label">选择周期</div>
          <t-radio-group v-model="buyPeriod">
            <t-radio value="month">月付 {{ formatMoney(buyProduct.price_month_cents, buyProduct.currency) }}</t-radio>
            <t-radio v-if="buyProduct.price_quarter_cents" value="quarter">季付 {{ formatMoney(buyProduct.price_quarter_cents, buyProduct.currency) }}</t-radio>
            <t-radio v-if="buyProduct.price_year_cents" value="year">年付 {{ formatMoney(buyProduct.price_year_cents, buyProduct.currency) }}</t-radio>
          </t-radio-group>
        </div>
        <div class="buy-total">
          <span>应付总额</span>
          <span class="buy-price">{{ formatMoney(buyAmount, buyProduct.currency) }}</span>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api, type Product } from "@/lib/api"

const products = ref<Product[]>([])
const loading = ref(true)
const error = ref("")

const detailVisible = ref(false)
const detailProduct = ref<Product | null>(null)

const buyVisible = ref(false)
const buyProduct = ref<Product | null>(null)
const buyPeriod = ref("month")
const buying = ref(false)

const loadProducts = async () => {
  try {
    loading.value = true
    const res = await api.getProducts()
    products.value = (res.products || []).filter((p) => p.enabled !== false)
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
  buyProduct.value = product
  buyPeriod.value = "month"
  buyVisible.value = true
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

<style scoped>
.loading {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

.empty {
  padding: 24px 0;
  text-align: center;
}

.product-grid {
  display: grid;
  gap: 20px;
  grid-template-columns: repeat(auto-fit, minmax(280px, 1fr));
}

.product-head {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  margin-bottom: 16px;
}

.product-name {
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
  margin: 0 0 4px;
}

.product-desc {
  font-size: 14px;
  color: #94a3b8;
  margin: 0;
}

.product-status {
  padding: 4px 10px;
  font-size: 12px;
  font-weight: 500;
  border-radius: 6px;
  background: rgba(99, 91, 255, 0.1);
  color: #635BFF;
  white-space: nowrap;
}

.product-meta {
  display: flex;
  flex-direction: column;
  gap: 8px;
  margin-bottom: 20px;
}

.meta-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 14px;
}

.meta-label { color: #94a3b8; }
.meta-value { color: #0f172a; font-size: 13px; font-weight: 500; }
.meta-value.price { font-weight: 700; color: #635BFF; font-size: 16px; }

.mono { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; }

.product-actions {
  display: flex;
  gap: 12px;
  padding-top: 16px;
  border-top: 1px solid #f1f5f9;
}

/* Detail dialog */
.detail-content {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.detail-section-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  padding-bottom: 6px;
  border-bottom: 1px solid #f1f5f9;
}

.detail-row {
  display: flex;
  justify-content: space-between;
  font-size: 14px;
  padding: 2px 0;
}

.detail-label {
  color: #94a3b8;
  min-width: 80px;
}

.detail-features {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.price { font-weight: 700; color: #635BFF; }

/* Buy dialog */
.buy-form {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.buy-product-name {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.form-item {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.form-label {
  font-size: 14px;
  color: #475569;
  font-weight: 500;
}

.buy-total {
  display: flex;
  align-items: center;
  gap: 8px;
  padding-top: 12px;
  border-top: 1px solid #f1f5f9;
  font-size: 14px;
}

.buy-price {
  font-size: 20px;
  font-weight: 700;
  color: #635BFF;
}

.muted { color: #94a3b8; }
</style>
