<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">四层转发</h1>
        <p class="subtitle">TCP/UDP 端口转发配额与套餐信息</p>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="l4-notice">
          <div class="notice-icon">
            <svg width="20" height="20" viewBox="0 0 20 20" fill="none"><circle cx="10" cy="10" r="9" stroke="#635BFF" stroke-width="2"/><rect x="9" y="5" width="2" height="6" rx="1" fill="#635BFF"/><rect x="9" y="13" width="2" height="2" rx="1" fill="#635BFF"/></svg>
          </div>
          <div class="notice-text">
            <p><strong>四层转发功能</strong>用于配置 TCP/UDP 端口转发规则，将流量通过 CDN 节点转发到源站指定端口。</p>
            <p>需要套餐包含 L4 端口配额方可使用，请联系管理员开通对应套餐。</p>
          </div>
        </div>

        <div v-if="loading" style="text-align:center;padding:40px 0"><t-loading /></div>
        <div v-else>
          <div v-if="!hasL4Quota" class="empty-state">
            <p>您当前的套餐尚未包含 L4 端口配额</p>
            <RouterLink to="/dashboard/products">
              <t-button size="small" variant="outline">查看产品</t-button>
            </RouterLink>
          </div>
          <div v-else class="l4-info">
            <div class="info-item">
              <span class="info-label">可用端口数</span>
              <span class="info-value">{{ streamPortLimit }}</span>
            </div>
            <div class="info-item">
              <span class="info-label">套餐名称</span>
              <span class="info-value">{{ activeProductName }}</span>
            </div>
            <p class="muted">如需新增或修改 L4 转发规则，请前往域名详情页面进行配置。</p>
          </div>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { api, type UserOrder } from "@/lib/api"

const loading = ref(true)
const hasL4Quota = ref(false)
const streamPortLimit = ref(0)
const activeProductName = ref("-")

const loadData = async () => {
  try {
    loading.value = true
    const [ordersRes, productsRes] = await Promise.all([
      api.getUserOrders(),
      api.getProducts(),
    ])
    const activeOrders = (ordersRes.orders || []).filter((o: UserOrder) => o.status === "active")
    const products = productsRes.products || []
    const productMap: Record<string, any> = {}
    for (const p of products) {
      if (p?.id) productMap[p.id] = p
    }
    for (const order of activeOrders) {
      const product = productMap[order.product_id]
      if (product?.stream_port_limit && product.stream_port_limit > 0) {
        hasL4Quota.value = true
        streamPortLimit.value = product.stream_port_limit
        activeProductName.value = product.name || order.product_name || "-"
        break
      }
    }
  } catch {
    // ignore
  } finally {
    loading.value = false
  }
}

onMounted(() => { loadData() })
</script>

<style scoped>
.section-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.l4-notice {
  display: flex;
  gap: 12px;
  padding: 14px 16px;
  border-radius: 10px;
  background: linear-gradient(135deg, #F5F4FF 0%, #f0f9ff 100%);
  border: 1px solid #D5CFFF;
}

.notice-icon {
  flex-shrink: 0;
  margin-top: 2px;
}

.notice-text {
  font-size: 14px;
  color: #475569;
  line-height: 1.6;
}

.notice-text p {
  margin: 0 0 4px;
}

.empty-state {
  text-align: center;
  padding: 48px 0;
  color: #94a3b8;
  font-size: 14px;
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 12px;
}

.l4-info {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.info-item {
  display: flex;
  align-items: center;
  gap: 12px;
  font-size: 14px;
}

.info-label {
  color: #94a3b8;
  min-width: 120px;
}

.info-value {
  color: #0f172a;
  font-weight: 600;
}

.muted {
  font-size: 13px;
  color: #94a3b8;
  margin-top: 8px;
}
</style>
