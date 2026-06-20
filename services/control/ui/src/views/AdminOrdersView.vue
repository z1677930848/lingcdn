<template>
  <div class="admin-page">
    <ErrorState v-if="error" :message="error" @retry="loadOrders" />

    <el-card class="admin-list-card">
      <div class="admin-toolbar">
        <div class="admin-toolbar-left">
          <el-button type="primary" @click="openCreate">添加已售套餐</el-button>
        </div>
        <div class="admin-toolbar-right">
          <EpSelect v-model="status" :options="statusOptions" class="admin-search-select" />
          <el-input v-model="userId" class="admin-search-input" clearable placeholder="用户ID" />
          <el-button plain @click="loadOrders">刷新</el-button>
        </div>
      </div>

      <div class="admin-desktop-only">
        <EpDataTable
          :data="orders"
          :columns="columns"
          row-key="id"
          hover
          stripe
          size="default"
          :loading="loading"
          :pagination="pagination"
        />
      </div>

      <div class="admin-mobile-only">
        <div v-if="loading" style="text-align:center;padding:32px 0">
          <div v-loading="true" style="min-height:48px" />
        </div>
        <div v-else-if="orders.length === 0" class="admin-mobile-card-empty">暂无订单数据</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="order in orders" :key="order.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <span class="admin-mobile-card-title">{{ order.product_name || '-' }}</span>
              <div class="admin-mobile-card-tags">
                <el-tag :type="statusTheme(order.status)" size="small">{{ statusLabel(order.status) }}</el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-subtitle">{{ order.product_id || '-' }}</div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">订单ID</span>
                <span class="admin-mobile-card-value">{{ order.id }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">用户ID</span>
                <span class="admin-mobile-card-value">{{ order.user_id || '-' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">金额</span>
                <span class="admin-mobile-card-value">{{ formatMoney(order.amount_cents, order.currency) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">周期/数量</span>
                <span class="admin-mobile-card-value">{{ periodLabel(order.period) }} / {{ order.quantity || 1 }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">创建时间</span>
                <span class="admin-mobile-card-value">{{ formatDateTime(order.created_at) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">更新时间</span>
                <span class="admin-mobile-card-value">{{ formatDateTime(order.updated_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button
                v-if="String(order.status || '').toLowerCase() !== 'paid'"
                size="small"
                type="primary"
                link
                @click="quickUpdateStatus(order.id, 'paid')"
              >标记已支付</el-button>
              <el-button
                v-if="String(order.status || '').toLowerCase() !== 'cancelled'"
                size="small"
                type="danger"
                link
                @click="quickUpdateStatus(order.id, 'cancelled')"
              >取消</el-button>
            </div>
          </div>
        </div>
        <div v-if="total > 0" class="admin-mobile-pagination">
          <el-pagination
            :current="page"
            :page-size="pageSize"
            :total="total"
            :page-size-options="[10, 20, 50, 100]"
            show-page-size
            size="small"
            @current-change="(v: number) => { page = v }"
            @page-size-change="(v: number) => { pageSize = v; page = 1 }"
          />
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body
      v-model="createDialogOpen"
      header="添加已售套餐"
      :confirm-btn="{ content: creating ? '创建中...' : '创建', loading: creating, theme: 'primary' }"
      cancel-btn="取消"
      width="560"
      @confirm="createOrder"
      @close="() => (createDialogOpen = false)"
    >
      <el-form label-position="top">
        <el-form-item label="用户ID">
          <el-input v-model="createUserId" />
        </el-form-item>
        <el-form-item label="套餐">
          <EpSelect v-model="createProductId" :options="productOptions" />
        </el-form-item>
        <el-form-item label="周期">
          <EpSelect v-model="createPeriod" :options="periodOptions" />
        </el-form-item>
        <el-form-item label="数量">
          <el-input-number v-model="createQuantity" :min="1" :step="1" />
        </el-form-item>
        <el-form-item label="金额（元）">
          <div class="amount-stack">
            <el-input-number v-model="amountInput" :min="0" :step="1" />
            <span class="admin-table-muted">建议金额：{{ suggestedAmountYuan.toFixed(2) }} 元</span>
          </div>
        </el-form-item>
        <el-form-item label="状态">
          <EpSelect v-model="createStatus" :options="orderStatusOptions" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="createNote" :autosize="{ minRows: 3, maxRows: 6 }" />
        </el-form-item>
      </el-form>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, h, onMounted, ref, watch } from "vue"
import { epTagType } from "@/lib/ep-tag"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag, ElButton } from "element-plus"
import { api, type Order, type Product } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"
import "@/styles/admin-shared.css"

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "待支付", value: "pending" },
  { label: "已支付", value: "paid" },
  { label: "已过期", value: "expired" },
  { label: "已取消", value: "cancelled" },
]

const periodOptions = [
  { label: "月付", value: "month" },
  { label: "季度付", value: "quarter" },
  { label: "年付", value: "year" },
]

const orderStatusOptions = [
  { label: "已支付", value: "paid" },
  { label: "待支付", value: "pending" },
  { label: "已取消", value: "cancelled" },
  { label: "已过期", value: "expired" },
]

const orders = ref<Order[]>([])
const products = ref<Product[]>([])
const loading = ref(true)
const error = ref("")
const status = ref("all")
const userId = ref("")
const page = ref(1)
const pageSize = ref(10)
const total = ref(0)

const createDialogOpen = ref(false)
const creating = ref(false)
const createUserId = ref("")
const createProductId = ref("")
const createPeriod = ref("month")
const createQuantity = ref(1)
const createStatus = ref("paid")
const createAmount = ref<number | null>(null)
const createNote = ref("")

const productOptions = computed(() => products.value.map((p) => ({ label: p.name, value: p.id })))

const selectedProduct = computed(() => products.value.find((p) => p.id === createProductId.value) || null)

const suggestedAmountYuan = computed(() => {
  if (!selectedProduct.value) return 0
  const qty = Math.max(1, Number(createQuantity.value) || 1)
  const month = selectedProduct.value.price_month_cents ?? selectedProduct.value.price_cents ?? 0
  const quarter = selectedProduct.value.price_quarter_cents ?? 0
  const year = selectedProduct.value.price_year_cents ?? 0
  let cents = 0
  if (createPeriod.value === "quarter") cents = quarter
  else if (createPeriod.value === "year") cents = year
  else cents = month
  return Math.round((cents * qty) / 100 * 100) / 100
})

const amountInput = computed({
  get() {
    return createAmount.value === null ? suggestedAmountYuan.value : createAmount.value
  },
  set(val: number) {
    createAmount.value = Number(val) || 0
  },
})

const formatMoney = (cents?: number, currency = "CNY") => {
  const value = typeof cents === "number" ? cents / 100 : 0
  return `${value.toFixed(2)} ${currency}`
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const statusLabel = (s?: string) => {
  const v = String(s || "").toLowerCase()
  if (v === "paid") return "已支付"
  if (v === "pending") return "待支付"
  if (v === "expired") return "已过期"
  if (v === "cancelled") return "已取消"
  return s || "-"
}

const statusTheme = (s?: string) => epTagType(
  (() => {
    const v = String(s || "").toLowerCase()
    if (v === "paid") return "success"
    if (v === "pending") return "warning"
    return "info"
  })()
)

const periodLabel = (p?: string) => {
  const v = String(p || "month").toLowerCase()
  if (v === "year") return "年"
  if (v === "quarter") return "季"
  return "月"
}

const statusTag = (s?: string) => {
  const v = String(s || "").toLowerCase()
  if (v === "paid") return h(ElTag, { type: "success" }, () => "已支付")
  if (v === "pending") return h(ElTag, { type: "warning" }, () => "待支付")
  if (v === "expired") return h(ElTag, { type: "info" }, () => "已过期")
  if (v === "cancelled") return h(ElTag, { type: "info" }, () => "已取消")
  return h(ElTag, { type: "info" }, () => s || "-")
}

const loadOrders = async () => {
  try {
    loading.value = true
    const res = await api.getAdminOrders({
      userId: userId.value.trim() || undefined,
      status: status.value === "all" ? undefined : status.value,
      page: page.value,
      pageSize: pageSize.value,
    })
    orders.value = res.orders || []
    total.value = res.total || 0
    error.value = ""
  } catch (err: any) {
    const msg = err?.message || "加载已售套餐失败"
    error.value = msg
    MessagePlugin.error(msg)
  } finally {
    loading.value = false
  }
}

const loadProducts = async () => {
  try {
    const res = await api.getProducts()
    products.value = res.products || []
  } catch {
    products.value = []
  }
}

const toCents = (yuan?: number | null) => Math.round((yuan || 0) * 100)

const openCreate = () => {
  createDialogOpen.value = true
  createUserId.value = userId.value.trim()
  createProductId.value = ""
  createPeriod.value = "month"
  createQuantity.value = 1
  createStatus.value = "paid"
  createAmount.value = null
  createNote.value = ""
}

const createOrder = async () => {
  if (creating.value) return
  const uid = createUserId.value.trim()
  const pid = createProductId.value.trim()
  if (!uid || !pid) {
    MessagePlugin.error("请输入用户ID并选择套餐")
    return
  }
  creating.value = true
  try {
    const amountYuan = createAmount.value === null ? suggestedAmountYuan.value : createAmount.value
    await api.createAdminOrder({
      user_id: uid,
      product_id: pid,
      period: createPeriod.value,
      quantity: createQuantity.value,
      status: createStatus.value,
      note: createNote.value,
      amount_cents: toCents(amountYuan),
    })
    MessagePlugin.success("已创建订单")
    createDialogOpen.value = false
    await loadOrders()
  } catch (err: any) {
    MessagePlugin.error(err.message || "创建失败")
  } finally {
    creating.value = false
  }
}

const quickUpdateStatus = async (id: string, nextStatus: string) => {
  try {
    await api.updateAdminOrder(id, { status: nextStatus } as any)
    MessagePlugin.success("状态已更新")
    await loadOrders()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新失败")
  }
}

const columns = computed(() => [
  { colKey: "id", title: "订单ID", minWidth: 160 },
  { colKey: "user_id", title: "用户ID", minWidth: 140, cell: (_h: any, { row }: { row: Order }) => h("span", { class: "admin-table-muted" }, row.user_id || "-") },
  {
    colKey: "product_name",
    title: "套餐",
    minWidth: 200,
    cell: (_h: any, { row }: { row: Order }) =>
      h("div", { style: "display:flex;flex-direction:column;gap:4px" }, [
        h("span", { style: "font-weight:600;color:var(--app-text-strong)" }, row.product_name || "-"),
        h("span", { class: "admin-table-muted", style: "font-size:12px" }, row.product_id || "-"),
      ]),
  },
  { colKey: "amount", title: "金额", width: 140, cell: (_h: any, { row }: { row: Order }) => h("span", { class: "admin-table-muted" }, formatMoney(row.amount_cents, row.currency)) },
  { colKey: "status", title: "状态", width: 120, cell: (_h: any, { row }: { row: Order }) => statusTag(row.status) },
  {
    colKey: "period",
    title: "周期/数量",
    width: 160,
    cell: (_h: any, { row }: { row: Order }) => {
      const p = String(row.period || "month").toLowerCase()
      const label = p === "year" ? "年" : p === "quarter" ? "季" : "月"
      const q = row.quantity || 1
      return h("span", { class: "admin-table-muted" }, `${label} / ${q}`)
    },
  },
  { colKey: "created_at", title: "创建时间", width: 180, cell: (_h: any, { row }: { row: Order }) => h("span", { class: "admin-table-muted" }, formatDateTime(row.created_at)) },
  { colKey: "updated_at", title: "更新时间", width: 180, cell: (_h: any, { row }: { row: Order }) => h("span", { class: "admin-table-muted" }, formatDateTime(row.updated_at)) },
  {
    colKey: "actions",
    title: "操作",
    width: 200,
    fixed: "right",
    cell: (_h: any, { row }: { row: Order }) =>
      h("div", { style: "display:flex;gap:8px;flex-wrap:wrap" }, [
        String(row.status || "").toLowerCase() !== "paid"
          ? h(ElButton, { size: "small", type: "primary", link: true, onClick: () => quickUpdateStatus(row.id, "paid") }, () => "标记已支付")
          : null,
        String(row.status || "").toLowerCase() !== "cancelled"
          ? h(ElButton, { size: "small", type: "danger", link: true, onClick: () => quickUpdateStatus(row.id, "cancelled") }, () => "取消")
          : null,
      ]),
  },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pi: { current: number; pageSize: number }) => {
    page.value = pi.current
    pageSize.value = pi.pageSize
  },
}))

watch([status, userId, page, pageSize], () => {
  loadOrders()
})

watch([status, userId], () => {
  page.value = 1
})

onMounted(() => {
  loadOrders()
  loadProducts()
})
</script>


