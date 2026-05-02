<template>
  <div v-if="loading" class="overview-loading">
    <t-loading size="medium" />
  </div>
  <div v-else class="page overview-page">
    <!-- Hero greeting: gradient + radial glow + welcome chip + quick stats -->
    <section class="dh-hero" v-motion="{ initial: { opacity: 0, y: 16 }, enter: { opacity: 1, y: 0, transition: { duration: 360 } } }">
      <div class="dh-hero-glow" aria-hidden="true"></div>
      <div class="dh-hero-grid" aria-hidden="true"></div>
      <div class="dh-hero-content">
        <div class="dh-hero-left">
          <div class="dh-hero-tag">
            <Sparkles :size="14" />
            <span>用户控制台</span>
          </div>
          <h1 class="dh-hero-title">
            欢迎回来<span v-if="auth.user?.username">，{{ auth.user.username }}</span>
            <span class="dh-hero-wave" aria-hidden="true">👋</span>
          </h1>
          <p class="dh-hero-subtitle">
            这里是您的服务总览。今天有 <b>{{ enabledDomainCount }}</b> 个域名在线，
            <b>{{ activeOrderCount }}</b> 个套餐生效中。
          </p>
          <div class="dh-hero-meta">
            <span class="dh-meta-chip">
              <UserRound :size="13" />
              ID {{ auth.user?.numeric_id ?? '-' }}
            </span>
            <span class="dh-meta-chip">
              <Wallet :size="13" />
              余额 {{ formattedBalance }}
            </span>
            <span class="dh-meta-chip">
              <Globe2 :size="13" />
              {{ domains.length }} 个域名
            </span>
          </div>
        </div>
        <div class="dh-hero-right">
          <div class="dh-hero-balance">
            <div class="dh-bal-label">账户余额</div>
            <div class="dh-bal-value">{{ formattedBalance }}</div>
            <RouterLink to="/dashboard/balance" class="dh-bal-link">
              充值 / 明细
              <ArrowRight :size="14" />
            </RouterLink>
          </div>
        </div>
      </div>
    </section>

    <div v-if="error" class="error-box">
      <span>{{ error }}</span>
      <t-button size="small" theme="primary" @click="loadData">重试</t-button>
    </div>

    <!-- Stat cards grid: TCard with hover-shadow + lucide icon -->
    <div class="dh-stat-grid">
      <t-card
        v-for="(stat, idx) in statCards"
        :key="stat.key"
        :hover-shadow="true"
        :bordered="true"
        class="dh-stat-card"
        :body-style="{ padding: '18px 20px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 80 + idx * 60, duration: 320 } } }"
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
      </t-card>
    </div>

    <!-- Current plan card -->
    <t-card
      class="dh-plan-card"
      :bordered="true"
      :body-style="{ padding: '22px 24px' }"
      v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 320, duration: 360 } } }"
    >
      <template v-if="currentPlan">
        <div class="dh-plan-head">
          <div class="dh-plan-head-left">
            <div class="dh-plan-icon"><Crown :size="22" /></div>
            <div>
              <div class="dh-plan-name">{{ currentPlan.product_name }}</div>
              <div class="dh-plan-meta">
                <t-tag theme="success" variant="light" size="small" shape="round">
                  <template #icon><CheckCircle2 :size="12" /></template>
                  生效中
                </t-tag>
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
            <div class="dh-plan-empty-sub">选购一款套餐即可开始使用 {{ brand.title }} 服务。</div>
          </div>
          <RouterLink to="/dashboard/products" class="dh-plan-cta">
            去选购
            <ArrowRight :size="14" />
          </RouterLink>
        </div>
      </template>
    </t-card>

    <!-- Two-column: announcements + quick actions -->
    <div class="dh-two-col">
      <t-card
        class="dh-list-card"
        :bordered="true"
        :body-style="{ padding: '0' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 380, duration: 360 } } }"
      >
        <template #header>
          <div class="dh-card-head">
            <Megaphone :size="16" class="dh-card-head-icon" />
            <span>系统公告</span>
            <RouterLink to="/dashboard/announcements" class="dh-card-head-link">查看全部</RouterLink>
          </div>
        </template>
        <div v-if="announcementsLoading && announcements.length === 0" class="dh-list-empty">
          加载中…
        </div>
        <ul v-else-if="announcements.length" class="dh-anno-list">
          <li v-for="item in announcements.slice(0, 4)" :key="item.id" class="dh-anno-item">
            <div class="dh-anno-bullet" />
            <div class="dh-anno-body">
              <div class="dh-anno-title">
                <span>{{ item.title }}</span>
                <t-tag v-if="item.pinned" size="small" theme="warning" variant="light" shape="round">置顶</t-tag>
              </div>
              <div class="dh-anno-time">{{ formatDateTime(item.updated_at) }}</div>
            </div>
          </li>
        </ul>
        <div v-else class="dh-list-empty">暂无公告</div>
      </t-card>

      <t-card
        class="dh-list-card"
        :bordered="true"
        :body-style="{ padding: '16px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 440, duration: 360 } } }"
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
      </t-card>
    </div>

    <!-- Recent orders + my domains -->
    <div class="dh-two-col">
      <t-card
        class="dh-list-card"
        :bordered="true"
        :body-style="{ padding: '16px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 500, duration: 360 } } }"
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
            <t-tag :theme="orderTagType(order.status)" variant="light" size="small" shape="round">
              {{ orderStatusLabel(order.status) }}
            </t-tag>
          </li>
        </ul>
      </t-card>

      <t-card
        class="dh-list-card"
        :bordered="true"
        :body-style="{ padding: '16px' }"
        v-motion="{ initial: { opacity: 0, y: 12 }, enter: { opacity: 1, y: 0, transition: { delay: 560, duration: 360 } } }"
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
              <t-tag :theme="domain.enabled !== false ? 'success' : 'default'" variant="light" size="small" shape="round">
                {{ domain.enabled !== false ? "已启用" : "已禁用" }}
              </t-tag>
              <t-tag v-if="domain.https_enabled" theme="primary" variant="light" size="small" shape="round">
                <template #icon><ShieldCheck :size="12" /></template>
                HTTPS
              </t-tag>
            </div>
          </li>
        </ul>
      </t-card>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { useAuthStore } from "@/stores/auth"
import { api, type Announcement, type Product, type Domain, type Certificate, type UserOrder, type BalanceAccount } from "@/lib/api"
import { useSystemSettings } from "@/lib/systemSettings"
import {
  Sparkles,
  UserRound,
  Wallet,
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
import "@/styles/overview.css"

const auth = useAuthStore()
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
const orderTagType = (status: string): "success" | "primary" | "warning" | "default" | "danger" => {
  switch (status) {
    case "active": return "success"
    case "paid": return "primary"
    case "pending": return "warning"
    case "expired":
    case "cancelled": return "default"
    default: return "default"
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
    bg: "rgba(99, 91, 255, 0.12)",
    fg: "#4D44E0",
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
    fg: "#b45309",
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

const quickLinks = [
  { title: "域名管理", desc: "添加 / 编辑站点", href: "/dashboard/domains", icon: Globe2, bg: "rgba(99, 91, 255, 0.12)", fg: "#4D44E0" },
  { title: "证书管理", desc: "上传 / 申请证书", href: "/dashboard/certs", icon: FileBadge2, bg: "rgba(16, 185, 129, 0.12)", fg: "#047857" },
  { title: "刷新预热", desc: "缓存刷新与预热", href: "/dashboard/purge", icon: RefreshCw, bg: "rgba(168, 85, 247, 0.12)", fg: "#7c3aed" },
  { title: "账户信息", desc: "个人资料与安全", href: "/dashboard/profile", icon: UserCog, bg: "rgba(100, 116, 139, 0.14)", fg: "#475569" },
]

onMounted(() => {
  loadData()
  loadAnnouncements()
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
.dh-hero {
  position: relative;
  overflow: hidden;
  border-radius: 16px;
  padding: 28px 32px;
  background: linear-gradient(135deg, #2D2899 0%, #4D44E0 50%, #8E5BFF 100%);
  color: #fff;
  box-shadow: var(--app-brand-shadow-md);
  isolation: isolate;
}
.dh-hero-glow {
  position: absolute;
  inset: -40% -10%;
  background:
    radial-gradient(40% 60% at 80% 20%, rgba(125, 211, 252, 0.45), transparent 60%),
    radial-gradient(40% 50% at 15% 90%, rgba(99, 102, 241, 0.45), transparent 60%);
  filter: blur(24px);
  z-index: 0;
  pointer-events: none;
}
.dh-hero-grid {
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
.dh-hero-content {
  position: relative;
  z-index: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 24px;
  flex-wrap: wrap;
}
.dh-hero-tag {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(255, 255, 255, 0.16);
  border: 1px solid rgba(255, 255, 255, 0.22);
  font-size: 12px;
  letter-spacing: 0.4px;
  color: #e0f2fe;
  margin-bottom: 12px;
}
.dh-hero-title {
  font-size: 26px;
  font-weight: 700;
  line-height: 1.2;
  margin: 0 0 8px;
}
.dh-hero-wave {
  display: inline-block;
  margin-left: 6px;
  animation: dh-wave 1.6s ease-in-out infinite;
  transform-origin: 70% 70%;
}
@keyframes dh-wave {
  0%, 60%, 100% { transform: rotate(0deg); }
  10% { transform: rotate(14deg); }
  20% { transform: rotate(-8deg); }
  30% { transform: rotate(14deg); }
  40% { transform: rotate(-4deg); }
  50% { transform: rotate(10deg); }
}
.dh-hero-subtitle {
  margin: 0 0 14px;
  color: rgba(255, 255, 255, 0.84);
  font-size: 14px;
  line-height: 1.6;
}
.dh-hero-subtitle b {
  color: #fff;
  font-weight: 700;
}
.dh-hero-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.dh-meta-chip {
  display: inline-flex;
  align-items: center;
  gap: 5px;
  padding: 4px 10px;
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.32);
  border: 1px solid rgba(255, 255, 255, 0.14);
  color: rgba(255, 255, 255, 0.92);
  font-size: 12px;
  backdrop-filter: blur(6px);
}
.dh-hero-right {
  flex: 0 0 auto;
}
.dh-hero-balance {
  padding: 18px 22px;
  border-radius: 14px;
  background: rgba(15, 23, 42, 0.32);
  border: 1px solid rgba(255, 255, 255, 0.18);
  backdrop-filter: blur(8px);
  text-align: right;
  min-width: 200px;
}
.dh-bal-label {
  font-size: 12px;
  color: rgba(255, 255, 255, 0.7);
  margin-bottom: 4px;
}
.dh-bal-value {
  font-size: 28px;
  font-weight: 700;
  color: #fff;
  letter-spacing: -0.02em;
  margin-bottom: 8px;
}
.dh-bal-link {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  font-size: 12px;
  color: #B8AFFE;
  text-decoration: none;
  transition: color 0.15s;
}
.dh-bal-link:hover {
  color: #bae6fd;
}

/* ─── Stat cards ───────────────────────────────────────────────────── */
.dh-stat-grid {
  display: grid;
  grid-template-columns: repeat(4, minmax(0, 1fr));
  gap: var(--app-page-gap-tight, 16px);
}
@media (max-width: 1200px) {
  .dh-stat-grid { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
@media (max-width: 600px) {
  .dh-stat-grid { grid-template-columns: 1fr; }
}
.dh-stat-card {
  transition: transform 0.18s var(--app-easing-standard), box-shadow 0.18s;
}
.dh-stat-card:hover {
  transform: translateY(-2px);
  box-shadow: var(--app-shadow-card-hover);
}
.dh-stat-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.dh-stat-info {
  min-width: 0;
}
.dh-stat-title {
  font-size: 12px;
  color: var(--app-text-muted);
  letter-spacing: 0.4px;
  margin-bottom: 6px;
}
.dh-stat-value {
  font-size: 28px;
  font-weight: 800;
  color: var(--app-text-strong);
  letter-spacing: -0.02em;
  line-height: 1.1;
}
.dh-stat-sub {
  margin-top: 8px;
  font-size: 12px;
  font-weight: 500;
}
.dh-stat-sub--muted { color: var(--app-text-muted); }
.dh-stat-sub--warn { color: var(--app-warning-strong); }
.dh-stat-icon {
  flex: 0 0 auto;
  width: 48px;
  height: 48px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid rgba(15, 23, 42, 0.06);
}

/* ─── Plan card ────────────────────────────────────────────────────── */
.dh-plan-card {
  position: relative;
  overflow: hidden;
}
.dh-plan-card::before {
  content: "";
  position: absolute;
  top: 0;
  left: 0;
  right: 0;
  height: 3px;
  background: linear-gradient(90deg, #635BFF, #8E5BFF 60%, #FF6BCB);
}
.dh-plan-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
}
.dh-plan-head-left {
  display: flex;
  align-items: center;
  gap: 14px;
}
.dh-plan-icon {
  width: 44px;
  height: 44px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: linear-gradient(135deg, #fde68a, #f59e0b);
  color: #92400e;
  box-shadow: 0 4px 14px rgba(245, 158, 11, 0.32);
}
.dh-plan-name {
  font-size: 18px;
  font-weight: 700;
  color: var(--app-text-strong);
  margin-bottom: 6px;
}
.dh-plan-meta {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.dh-plan-period { color: var(--app-text-muted); font-size: 13px; }
.dh-plan-price { font-weight: 700; color: var(--app-brand-strong); font-size: 15px; }
.dh-plan-cta {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  padding: 7px 14px;
  background: var(--app-brand-soft-bg);
  color: var(--app-brand-strong);
  border-radius: 999px;
  font-size: 13px;
  font-weight: 500;
  text-decoration: none;
  transition: background 0.15s;
}
.dh-plan-cta:hover { background: rgba(99, 91, 255, 0.16); }
.dh-plan-dates {
  display: flex;
  gap: 24px;
  margin-top: 18px;
  padding-top: 14px;
  border-top: 1px dashed var(--app-divider);
  flex-wrap: wrap;
}
.dh-plan-date {
  display: flex;
  align-items: center;
  gap: 8px;
  color: var(--app-text-muted);
}
.dh-plan-date-label { font-size: 11px; letter-spacing: 0.3px; }
.dh-plan-date-value { font-size: 14px; color: var(--app-text-strong); font-weight: 500; }
.dh-plan-empty {
  display: flex;
  align-items: center;
  gap: 14px;
}
.dh-plan-empty-icon {
  width: 44px;
  height: 44px;
  border-radius: 14px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--app-neutral-soft-bg);
  color: var(--app-text-muted);
}
.dh-plan-empty-text { flex: 1; }
.dh-plan-empty-title { font-weight: 600; color: var(--app-text-strong); }
.dh-plan-empty-sub { font-size: 12px; color: var(--app-text-muted); margin-top: 2px; }

/* ─── Two-column grids ─────────────────────────────────────────────── */
.dh-two-col {
  display: grid;
  grid-template-columns: 1.2fr 1fr;
  gap: var(--app-page-gap, 20px);
}
@media (max-width: 980px) {
  .dh-two-col { grid-template-columns: 1fr; }
}

.dh-list-card { overflow: hidden; }
.dh-card-head {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-weight: 600;
  font-size: 14px;
  color: var(--app-text-strong);
  width: 100%;
}
.dh-card-head-icon { color: var(--app-brand); }
.dh-card-head-link {
  margin-left: auto;
  font-size: 12px;
  font-weight: 500;
  color: var(--app-brand-strong);
  text-decoration: none;
}
.dh-card-head-link:hover { text-decoration: underline; }

/* Announcements */
.dh-anno-list {
  list-style: none;
  margin: 0;
  padding: 8px 0;
}
.dh-anno-item {
  display: flex;
  align-items: flex-start;
  gap: 12px;
  padding: 10px 18px;
  border-bottom: 1px solid var(--app-divider-soft);
}
.dh-anno-item:last-child { border-bottom: none; }
.dh-anno-bullet {
  flex: 0 0 auto;
  width: 8px;
  height: 8px;
  margin-top: 7px;
  border-radius: 50%;
  background: linear-gradient(135deg, #9385FE, #635BFF);
  box-shadow: 0 0 0 3px rgba(99, 91, 255, 0.12);
}
.dh-anno-body { flex: 1; min-width: 0; }
.dh-anno-title {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  font-size: 14px;
  color: var(--app-text-strong);
  font-weight: 500;
}
.dh-anno-time {
  font-size: 11px;
  color: var(--app-text-faint);
  margin-top: 2px;
}
.dh-list-empty {
  padding: 24px 18px;
  color: var(--app-text-faint);
  font-size: 13px;
  text-align: center;
}
.dh-anno-link {
  margin-left: 8px;
  color: var(--app-brand-strong);
  text-decoration: none;
  font-weight: 500;
}
.dh-anno-link:hover { text-decoration: underline; }

/* Quick action tiles */
.dh-quick-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}
.dh-quick-tile {
  display: block;
  padding: 14px;
  border-radius: 12px;
  border: 1px solid var(--app-border);
  background: var(--app-surface);
  text-decoration: none;
  color: var(--app-text-strong);
  transition: transform 0.15s, border-color 0.15s, box-shadow 0.15s;
}
.dh-quick-tile:hover {
  transform: translateY(-2px);
  border-color: var(--tile-fg, var(--app-brand));
  box-shadow: var(--app-shadow-card-hover);
}
.dh-quick-icon {
  width: 36px;
  height: 36px;
  border-radius: 10px;
  display: flex;
  align-items: center;
  justify-content: center;
  margin-bottom: 8px;
}
.dh-quick-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
}
.dh-quick-desc {
  font-size: 12px;
  color: var(--app-text-muted);
  margin-top: 2px;
}

/* Order / domain rows */
.dh-row-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: 8px;
}
.dh-row-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 10px;
  background: var(--app-surface-muted);
  transition: background 0.15s;
}
.dh-row-item:hover { background: var(--app-surface-hover); }
.dh-row-left { min-width: 0; flex: 1; }
.dh-row-title {
  font-size: 14px;
  font-weight: 500;
  color: var(--app-text-strong);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-row-sub {
  font-size: 12px;
  color: var(--app-text-muted);
  margin-top: 2px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}
.dh-row-tags {
  display: inline-flex;
  align-items: center;
  gap: 4px;
  flex-shrink: 0;
}
.dh-mono {
  font-family: "JetBrains Mono", "Fira Code", Consolas, monospace;
}

.error-box {
  padding: 12px 16px;
  border-radius: 10px;
  background: var(--app-danger-soft-bg);
  color: var(--app-danger-strong);
  border: 1px solid rgba(239, 68, 68, 0.2);
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

@media (max-width: 720px) {
  .dh-hero { padding: 22px 18px; }
  .dh-hero-title { font-size: 22px; }
  .dh-hero-content { flex-direction: column; align-items: stretch; }
  .dh-hero-balance { text-align: left; }
  .dh-plan-head { flex-direction: column; align-items: flex-start; }
  .dh-plan-dates { gap: 16px; }
}
</style>
