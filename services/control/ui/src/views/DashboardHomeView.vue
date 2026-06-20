<template>
  <div v-if="loading" class="overview-loading">
    <SkeletonStatGrid :count="4" />
  </div>
  <div v-else class="page overview-page">
    <section class="console-head">
      <div class="console-head-main">
        <h1 class="console-head-title">
          欢迎回来<span v-if="auth.user?.username">，{{ auth.user.username }}</span>
        </h1>
        <p class="console-head-desc">
          {{ enabledDomainCount }} 个域名在线· {{ activeOrderCount }} 个套餐生效· 余额 {{ formattedBalance }}
        </p>
      </div>
      <RouterLink to="/dashboard/balance" class="console-head-action">充值 / 明细</RouterLink>
    </section>

    <ErrorState v-if="error" :message="error" @retry="loadData" />

    <!-- Stat cards grid: TCard with hover-shadow + lucide icon -->
    <div class="dh-stat-grid">
      <el-card
        v-for="(stat, idx) in statCards"
        :key="stat.key"
        :hover-shadow="true"
       
        class="dh-stat-card"
        :body-style="{ padding: '18px 20px' }"
      >
        <div class="dh-stat-row">
          <div class="dh-stat-info">
            <div class="dh-stat-title">{{ stat.title }}</div>
            <div class="dh-stat-value">{{ stat.value }}</div>
            <div class="dh-stat-sub" :class="stat.subClass">{{ stat.sub }}</div>
          </div>
          <div class="dh-stat-icon" :style="{ background: stat.bg, color: stat.fg }">
            <component :is="stat.icon" :size="22" />
          </div>
        </div>
      </el-card>
    </div>

    <!-- Current plan card -->
    <el-card
      class="dh-plan-card"
     
      :body-style="{ padding: '22px 24px' }"
    >
      <template v-if="currentPlan">
        <div class="dh-plan-head">
          <div class="dh-plan-head-left">
            <div class="dh-plan-icon"><Crown :size="22" /></div>
            <div>
              <div class="dh-plan-name">{{ currentPlan.product_name }}</div>
              <div class="dh-plan-meta">
                <el-tag type="success" effect="light" size="small" shape="round">
                  <template #icon><CheckCircle2 :size="12" /></template>
                  生效中                </el-tag>
                <span class="dh-plan-period">{{ periodLabel(currentPlan.period) }}</span>
                <span v-if="currentPlan.amount_cents != null" class="dh-plan-price">{{ formatMoney(currentPlan.amount_cents) }}</span>
              </div>
            </div>
          </div>
          <RouterLink to="/dashboard/products" class="dh-plan-cta">
            更换套餐
            <ArrowRight :size="14" />
          </RouterLink>
        </div>
        <div class="dh-plan-dates">
          <div class="dh-plan-date">
            <CalendarPlus :size="14" />
            <div>
              <div class="dh-plan-date-label">开始时间</div>
              <div class="dh-plan-date-value">{{ formatDate(currentPlan.starts_at) }}</div>
            </div>
          </div>
          <div class="dh-plan-date">
            <CalendarClock :size="14" />
            <div>
              <div class="dh-plan-date-label">到期时间</div>
              <div class="dh-plan-date-value">{{ formatDate(currentPlan.ends_at) }}</div>
            </div>
          </div>
        </div>
      </template>
      <template v-else>
        <div class="dh-plan-empty">
          <div class="dh-plan-empty-icon"><PackageOpen :size="22" /></div>
          <div class="dh-plan-empty-text">
            <div class="dh-plan-empty-title">暂无生效中的套餐</div>
            <div class="dh-plan-empty-sub">选购一款套餐即可开始使用{{ brand.title }} 服务。</div>
          </div>
          <RouterLink to="/dashboard/products" class="dh-plan-cta">
            去选购
            <ArrowRight :size="14" />
          </RouterLink>
        </div>
      </template>
    </el-card>

    <!-- Two-column: announcements + quick actions -->
    <div class="dh-two-col">
      <el-card
        class="dh-list-card"
       
        :body-style="{ padding: '0' }"
      >
        <template #header>
          <div class="dh-card-head">
            <Megaphone :size="16" class="dh-card-head-icon" />
            <span>系统公告</span>
            <RouterLink to="/dashboard/announcements" class="dh-card-head-link">查看全部</RouterLink>
          </div>
        </template>
        <div v-if="announcementsLoading && announcements.length === 0" class="dh-list-empty">
          加载中…        </div>
        <ul v-else-if="announcements.length" class="dh-anno-list">
          <li v-for="item in announcements.slice(0, 4)" :key="item.id" class="dh-anno-item">
            <div class="dh-anno-bullet" />
            <div class="dh-anno-body">
              <div class="dh-anno-title">
                <span>{{ item.title }}</span>
                <el-tag v-if="item.pinned" size="small" type="warning" effect="light" shape="round">置顶</el-tag>
              </div>
              <div class="dh-anno-time">{{ formatDateTime(item.updated_at) }}</div>
            </div>
          </li>
        </ul>
        <div v-else class="dh-list-empty">暂无公告</div>
      </el-card>

      <el-card
        class="dh-list-card"
       
        :body-style="{ padding: '16px' }"
      >
        <template #header>
          <div class="dh-card-head">
            <Zap :size="16" class="dh-card-head-icon" />
            <span>快捷入口</span>
          </div>
        </template>
        <div class="dh-quick-grid">
          <RouterLink
            v-for="link in quickLinks"
            :key="link.href"
            :to="link.href"
            class="dh-quick-tile"
            :style="{ '--tile-fg': link.fg }"
          >
            <div class="dh-quick-icon" :style="{ background: link.bg, color: link.fg }">
              <component :is="link.icon" :size="18" />
            </div>
            <div class="dh-quick-title">{{ link.title }}</div>
            <div class="dh-quick-desc">{{ link.desc }}</div>
          </RouterLink>
        </div>
      </el-card>
    </div>

    <!-- Recent orders + my domains -->
    <div class="dh-two-col">
      <el-card
        class="dh-list-card"
       
        :body-style="{ padding: '16px' }"
      >
        <template #header>
          <div class="dh-card-head">
            <Receipt :size="16" class="dh-card-head-icon" />
            <span>最近订单</span>
            <RouterLink v-if="orders.length" to="/dashboard/balance" class="dh-card-head-link">查看全部</RouterLink>
          </div>
        </template>
        <div v-if="orders.length === 0" class="dh-list-empty">
          暂无订单
          <RouterLink to="/dashboard/products" class="dh-anno-link">去选购</RouterLink>
        </div>
        <ul v-else class="dh-row-list">
          <li v-for="order in recentOrders" :key="order.id" class="dh-row-item">
            <div class="dh-row-left">
              <div class="dh-row-title">{{ order.product_name || order.product_id }}</div>
              <div class="dh-row-sub">{{ formatDate(order.created_at) }}{{ order.period ? ` · ${periodLabel(order.period)}` : "" }}</div>
            </div>
            <el-tag :type="orderTagType(order.status)" effect="light" size="small" shape="round">
              {{ orderStatusLabel(order.status) }}
            </el-tag>
          </li>
        </ul>
      </el-card>

      <el-card
        class="dh-list-card"
       
        :body-style="{ padding: '16px' }"
      >
        <template #header>
          <div class="dh-card-head">
            <Globe2 :size="16" class="dh-card-head-icon" />
            <span>我的域名</span>
            <RouterLink v-if="domains.length" to="/dashboard/domains" class="dh-card-head-link">查看全部</RouterLink>
          </div>
        </template>
        <div v-if="domains.length === 0" class="dh-list-empty">
          暂无域名
          <RouterLink to="/dashboard/domains" class="dh-anno-link">去添加</RouterLink>
        </div>
        <ul v-else class="dh-row-list">
          <li v-for="domain in recentDomains" :key="domain.id" class="dh-row-item">
            <div class="dh-row-left">
              <div class="dh-row-title">{{ domain.name }}</div>
              <div class="dh-row-sub dh-mono">{{ domain.cname || "-" }}</div>
            </div>
            <div class="dh-row-tags">
              <el-tag :type="domain.enabled !== false ? 'success' : 'info'" effect="light" size="small" shape="round">
                {{ domain.enabled !== false ? "已启用" : "已禁用" }}
              </el-tag>
              <el-tag v-if="domain.https_enabled" type="primary" effect="light" size="small" shape="round">
                <template #icon><ShieldCheck :size="12" /></template>
                HTTPS
              </el-tag>
            </div>
          </li>
        </ul>
      </el-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useAuthStore } from "@/stores/auth"
import { api, type Announcement, type Product, type Domain, type Certificate, type UserOrder, type BalanceAccount } from "@/lib/api"
import { useSystemSettings } from "@/lib/systemSettings"
import ErrorState from "@/components/common/ErrorState.vue"
import SkeletonStatGrid from "@/components/common/SkeletonStatGrid.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"
import {
  PermDomainsWrite,
  PermCertificatesWrite,
  PermPurgeExecute,
} from "@/lib/permissions"
import {
  Globe2,
  ArrowRight,
  Crown,
  CheckCircle2,
  CalendarPlus,
  CalendarClock,
  PackageOpen,
  Megaphone,
  Zap,
  Receipt,
  ShieldCheck,
  FileBadge2,
  RefreshCw,
  ShoppingBag,
  UserCog,
} from "lucide-vue-next"
const auth = useAuthStore()
const { can } = useUserPermissions()
const { brand } = useSystemSettings()
const products = ref<Record<string, Product>>({})
const announcements = ref<Announcement[]>([])
const domains = ref<Domain[]>([])
const certificates = ref<Certificate[]>([])
const orders = ref<UserOrder[]>([])
const balanceAccount = ref<BalanceAccount | null>(null)
const loading = ref(true)
const error = ref("")
const announcementsLoading = ref(false)

const loadData = async () => {
  try {
    loading.value = true
    error.value = ""

    const results = await Promise.allSettled([
      api.getProducts(),
      api.listDomains(),
      api.listCertificates(),
      api.getUserOrders(),
      api.getBalanceAccount(),
    ])

    if (results[0].status === "fulfilled") {
      const map: Record<string, Product> = {}
      for (const p of results[0].value.products || []) {
        if (p?.id) map[p.id] = p
      }
      products.value = map
    }
    if (results[1].status === "fulfilled") domains.value = results[1].value.domains || []
    if (results[2].status === "fulfilled") certificates.value = results[2].value.certificates || []
    if (results[3].status === "fulfilled") orders.value = results[3].value.orders || []
    if (results[4].status === "fulfilled") balanceAccount.value = results[4].value.account || null

    const rejected = results.filter((r) => r.status === "rejected") as PromiseRejectedResult[]
    if (rejected.length === results.length) {
      error.value = rejected[0]?.reason?.message || "加载数据失败"
    }
  } catch (err: any) {
    error.value = err.message || "加载数据失败"
  } finally {
    loading.value = false
  }
}

const loadAnnouncements = async () => {
  try {
    announcementsLoading.value = true
    const res = await api.getPublicAnnouncements({ page: 1, pageSize: 5 })
    announcements.value = res.announcements || []
  } catch {
    announcements.value = []
  } finally {
    announcementsLoading.value = false
  }
}

const totalProducts = computed(() => Object.keys(products.value).length)
const enabledDomainCount = computed(() => domains.value.filter((d) => d.enabled !== false).length)
const activeOrderCount = computed(() => {
  const now = Date.now()
  return orders.value.filter((o) => {
    if (o.status !== "active" && o.status !== "paid") return false
    if (o.ends_at) {
      const end = new Date(o.ends_at).getTime()
      if (!Number.isNaN(end) && end < now) return false
    }
    return true
  }).length
})
const expiringCertCount = computed(() => {
  const now = Date.now()
  const thirtyDays = 30 * 24 * 60 * 60 * 1000
  return certificates.value.filter((c) => {
    if (!c.expires_at) return false
    const expireAt = new Date(c.expires_at).getTime()
    if (Number.isNaN(expireAt)) return false
    return expireAt > now && expireAt - now < thirtyDays
  }).length
})

const formattedBalance = computed(() => {
  if (!balanceAccount.value) return "¥0.00"
  const cents = balanceAccount.value.balance_cents || 0
  return `¥${(cents / 100).toFixed(2)}`
})

const currentPlan = computed(() => {
  const now = Date.now()
  const active = orders.value.filter((o) => {
    if (o.status !== "paid" && o.status !== "active") return false
    if (o.ends_at) {
      const end = new Date(o.ends_at).getTime()
      if (!Number.isNaN(end) && end < now) return false
    }
    return true
  })
  if (active.length === 0) return null
  return active.sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())[0]
})

const periodLabel = (period?: string) => {
  switch (period) {
    case "month": return "月付"
    case "quarter": return "季付"
    case "year": return "年付"
    default: return period || ""
  }
}

const formatMoney = (cents?: number) => {
  const value = typeof cents === "number" ? cents / 100 : 0
  return `¥${value.toFixed(2)}`
}

const recentOrders = computed(() =>
  [...orders.value]
    .sort((a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime())
    .slice(0, 5)
)

const recentDomains = computed(() =>
  [...domains.value]
    .sort((a, b) => new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime())
    .slice(0, 5)
)

const orderStatusLabel = (status: string) => {
  switch (status) {
    case "active": return "生效中"
    case "paid": return "已支付"
    case "pending": return "待支付"
    case "expired": return "已过期"
    case "cancelled": return "已取消"
    default: return status
  }
}

// Map order status onto TDesign tag themes so the same status uses the
// same colour everywhere on the page.
const orderTagType = (status: string): "success" | "primary" | "warning" | "info" | "danger" => {
  switch (status) {
    case "active": return "success"
    case "paid": return "primary"
    case "pending": return "warning"
    case "expired":
    case "cancelled": return "info"
    default: return "info"
  }
}

const formatDate = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleDateString("zh-CN")
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

// Stat tile config drives the four-card grid. Centralising icon/colour
// choice here keeps the template short and lets us reorder cards by
// editing one array.
const statCards = computed(() => [
  {
    key: "domains",
    title: "活跃域名",
    value: enabledDomainCount.value.toLocaleString("zh-CN"),
    sub: `域名总数 ${domains.value.length}`,
    subClass: "dh-stat-sub--muted",
    icon: Globe2,
    bg: "rgba(64, 158, 255, 0.12)",
    fg: "#337ECC",
  },
  {
    key: "certs",
    title: "证书",
    value: certificates.value.length.toLocaleString("zh-CN"),
    sub: expiringCertCount.value > 0 ? `即将到期 ${expiringCertCount.value}` : "无即将到期",
    subClass: expiringCertCount.value > 0 ? "dh-stat-sub--warn" : "dh-stat-sub--muted",
    icon: FileBadge2,
    bg: "rgba(16, 185, 129, 0.12)",
    fg: "#047857",
  },
  {
    key: "orders",
    title: "活跃订单",
    value: activeOrderCount.value.toLocaleString("zh-CN"),
    sub: `订单总数 ${orders.value.length}`,
    subClass: "dh-stat-sub--muted",
    icon: ShoppingBag,
    bg: "rgba(245, 158, 11, 0.12)",
    fg: "var(--app-warning-strong)",
  },
  {
    key: "products",
    title: "可用产品",
    value: totalProducts.value.toLocaleString("zh-CN"),
    sub: "最新已更新",
    subClass: "dh-stat-sub--muted",
    icon: PackageOpen,
    bg: "rgba(168, 85, 247, 0.12)",
    fg: "#7c3aed",
  },
])

const quickLinks = computed(() =>
  [
    { title: "域名管理", desc: "添加 / 编辑站点", href: "/dashboard/domains", icon: Globe2, bg: "rgba(64, 158, 255, 0.12)", fg: "#337ECC", perm: PermDomainsWrite },
    { title: "证书管理", desc: "上传 / 申请证书", href: "/dashboard/certs", icon: FileBadge2, bg: "rgba(16, 185, 129, 0.12)", fg: "#047857", perm: PermCertificatesWrite },
    { title: "刷新预热", desc: "缓存刷新与预热", href: "/dashboard/purge", icon: RefreshCw, bg: "rgba(168, 85, 247, 0.12)", fg: "#7c3aed", perm: PermPurgeExecute },
    { title: "账户信息", desc: "个人资料与安全", href: "/dashboard/profile", icon: UserCog, bg: "rgba(100, 116, 139, 0.14)", fg: "var(--app-text-muted)" },
  ].filter((item) => !item.perm || can(item.perm)),
)

onMounted(() => {
  loadData()
  loadAnnouncements()
})
</script>
