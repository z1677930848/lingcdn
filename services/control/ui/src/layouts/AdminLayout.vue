<template>
  <AppLoading v-if="auth.loading || licenseGate.loading.value" fullscreen :title="`${brand.title} 管理后台`" subtitle="正在加载管理控制台…" />
  <TemplateShell
    v-else-if="auth.user"
    :brand="{ title: brand.title, logo: brand.logo }"
    :modules="modules"
    :active-module-id="activeModuleId"
    :active-path="route.path"
    :user="{ username: auth.user.username, email: auth.user.email }"
    :footer-links="footerLinks"
    :footer-copyright="footerCopyright"
    @navigate="(href) => router.push(href)"
    @logout="handleLogout"
  >
    <LicenseRequired
      v-if="licenseBlocked"
      :status="licenseGate.statusCode.value"
      :reason="licenseGate.notice.value"
      :can-manage="true"
      :on-go-license="() => router.push('/admin/dashboard/license-center')"
      :on-refresh="licenseGate.fetchStatus"
    />
    <div v-else class="content-wrap">
      <div v-if="licenseGate.warning.value" class="license-warning" role="alert">
        <span>{{ licenseGate.notice.value }}</span>
        <t-button v-if="!isLicenseCenter" variant="outline" size="small" @click="router.push('/admin/dashboard/license-center')">
          前往授权中心
        </t-button>
      </div>
      <RouterView />
    </div>
  </TemplateShell>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import TemplateShell from "@/components/layout/TemplateShell.vue"
import type { TemplateModule } from "@/components/layout/template-shell.types"
import LicenseRequired from "@/components/license/LicenseRequired.vue"
import AppLoading from "@/components/atoms/AppLoading.vue"
import { useAuthStore } from "@/stores/auth"
import { useSystemSettings } from "@/lib/systemSettings"
import { useLicenseGate } from "@/composables/useLicenseGate"

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { brand, footerLinks, footerCopyright } = useSystemSettings()

const licenseGate = useLicenseGate(() => !!auth.user && auth.user.role === "admin")

const modules: TemplateModule[] = [
  {
    id: "overview",
    name: "管理概览",
    icon: "dashboard",
    defaultHref: "/admin/dashboard",
    items: [{ name: "管理概览", href: "/admin/dashboard" }],
  },
  {
    id: "nodes",
    name: "节点管理",
    icon: "server",
    defaultHref: "/admin/dashboard/nodes",
    items: [
      { name: "节点管理", href: "/admin/dashboard/nodes" },
      { name: "集群管理", href: "/admin/dashboard/clusters" },
      { name: "实时监控", href: "/admin/dashboard/nodes/monitor" },
      { name: "DNS 配置", href: "/admin/dashboard/nodes/dns" },
    ],
  },
  {
    id: "security",
    name: "安全防护",
    icon: "secured",
    defaultHref: "/admin/dashboard/ddos",
    items: [
      { name: "DDoS 防护", href: "/admin/dashboard/ddos" },
      { name: "WAF 策略", href: "/admin/dashboard/waf" },
    ],
  },
  {
    id: "domains",
    name: "域名管理",
    icon: "internet",
    defaultHref: "/admin/dashboard/domains",
    items: [
      { name: "域名管理", href: "/admin/dashboard/domains" },
      { name: "网站监控", href: "/admin/dashboard/domains/health" },
      { name: "缓存规则", href: "/admin/dashboard/cache-rules" },
      { name: "证书管理", href: "/admin/dashboard/certificates" },
    ],
  },
  {
    id: "l4",
    name: "L4 管理",
    icon: "swap",
    defaultHref: "/admin/dashboard/l4",
    items: [{ name: "L4 管理", href: "/admin/dashboard/l4" }],
  },
  {
    id: "global",
    name: "系统配置",
    icon: "setting",
    defaultHref: "/admin/dashboard/settings",
    items: [
      { name: "系统配置", href: "/admin/dashboard/settings" },
      { name: "模板管理", href: "/admin/dashboard/templates" },
    ],
  },
  {
    id: "plans",
    name: "套餐管理",
    icon: "app",
    defaultHref: "/admin/dashboard/products",
    items: [
      { name: "基础套餐", href: "/admin/dashboard/products" },
      { name: "已售套餐", href: "/admin/dashboard/orders" },
    ],
  },
  {
    id: "finance",
    name: "财务管理",
    icon: "wallet",
    defaultHref: "/admin/dashboard/balance",
    items: [
      { name: "余额管理", href: "/admin/dashboard/balance" },
      { name: "财务统计", href: "/admin/dashboard/finance/stats" },
    ],
  },
  {
    id: "system",
    name: "系统管理",
    icon: "tools",
    defaultHref: "/admin/dashboard/users",
    items: [
      { name: "用户管理", href: "/admin/dashboard/users" },
      { name: "管理员资料", href: "/admin/dashboard/profile" },
      { name: "授权中心", href: "/admin/dashboard/license-center" },
      { name: "任务列表", href: "/admin/dashboard/tasks" },
      { name: "系统公告", href: "/admin/dashboard/announcements" },
      { name: "操作日志", href: "/admin/dashboard/logs" },
      { name: "系统升级", href: "/admin/dashboard/upgrade" },
    ],
  },
]

const activeModuleId = computed(() => {
  const path = route.path
  if (path.startsWith("/admin/dashboard/nodes")) return "nodes"
  if (path.startsWith("/admin/dashboard/clusters")) return "nodes"
  if (path.startsWith("/admin/dashboard/ddos")) return "security"
  if (path.startsWith("/admin/dashboard/waf")) return "security"
  if (path.startsWith("/admin/dashboard/domains")) return "domains"
  if (path.startsWith("/admin/dashboard/cache-rules")) return "domains"
  if (path.startsWith("/admin/dashboard/certificates")) return "domains"
  if (path.startsWith("/admin/dashboard/l4")) return "l4"
  if (path.startsWith("/admin/dashboard/settings") || path.startsWith("/admin/dashboard/templates")) return "global"
  if (path.startsWith("/admin/dashboard/products") || path.startsWith("/admin/dashboard/orders")) return "plans"
  if (path.startsWith("/admin/dashboard/balance") || path.startsWith("/admin/dashboard/finance")) return "finance"
  if (
    path.startsWith("/admin/dashboard/users") ||
    path.startsWith("/admin/dashboard/profile") ||
    path.startsWith("/admin/dashboard/license-center") ||
    path.startsWith("/admin/dashboard/tasks") ||
    path.startsWith("/admin/dashboard/announcements") ||
    path.startsWith("/admin/dashboard/logs") ||
    path.startsWith("/admin/dashboard/upgrade")
  ) {
    return "system"
  }
  return "overview"
})

const isLicenseCenter = computed(() => route.path.startsWith("/admin/dashboard/license-center"))
const licenseBlocked = computed(() => licenseGate.blocked.value && !isLicenseCenter.value)

watch(
  () => auth.user,
  () => {
    if (!auth.loading) {
      if (!auth.user) {
        router.push("/nimda")
      } else if (auth.user.role !== "admin") {
        router.push("/dashboard")
      }
    }
  },
  { immediate: true }
)

onMounted(async () => {
  if (!auth.user && auth.loading) {
    await auth.checkAuth()
  }
  await licenseGate.fetchStatus()
})

const handleLogout = async () => {
  await auth.logout()
  await router.push("/nimda")
}
</script>

<style scoped>
.content-wrap {
  /* Was `display: grid` with no `grid-template-columns` — that defaults
   * the single column to `auto` = content width. Pages whose root
   * `.page` has `max-width: 1280px` collapsed the whole content area to
   * 1280px on the left, leaving a wide empty band on the right of any
   * 1600+px monitor. Switched to flex column so children stretch to the
   * full available width while still getting the same vertical gap.
   *
   * `flex: 1` continues the height-fill chain started in TemplateShell
   * (.content-inner) so the routed view stretches to the full visible
   * content area instead of leaving empty space below short pages. */
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
  gap: var(--app-page-gap-tight);
  min-width: 0;
}

.content-wrap > * {
  min-width: 0;
}

/* 链式高度的最后一节：让 RouterView 渲染出来的视图根（一般是 .page）
 * 撑满 .content-wrap 的剩余高度，避免短页面（空表格、空状态）下面
 * 出现一大片灰底空白，footer 被顶到视口最底。:last-child 只挑路由
 * 视图本身，不会让上面的 .license-warning 也被强行撑开。
 *
 * `max-width: none` + `margin: 0` 覆盖各视图 `.page` 自身可能设的
 * `max-width: 1280px; margin: 0 auto`（DomainDetailView 等），让仪表盘
 * 真正全屏铺满，不再被 1280 锁住中间一块。 */
.content-wrap > :last-child {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  max-width: none !important;
  margin: 0 !important;
}

/* 关键：禁用直接子项的 flex-shrink。
 * 长页面（仪表盘 9+ 段卡片）总高远超 viewport，flex column 默认会等比
 * 缩压所有子项把它们硬塞进容器 —— hero 会被挤成一条线，统计卡变窄条。
 * 加 flex-shrink: 0 后子项保持自然高度，溢出交由 .content-area 的
 * overflow-y: auto 滚动接管。 */
.content-wrap > :last-child > * {
  flex-shrink: 0;
}

/* 短页面（如空集群表）下让最后一段卡片自动 grow 撑满剩余空间，
 * 避免 footer 上方留一大片灰底。长页面里这条没影响 —— flex-grow 只在
 * 容器有富余空间时才生效。 */
.content-wrap > :last-child > :last-child {
  flex-grow: 1;
}

.license-warning {
  padding: 12px 16px;
  border-radius: var(--app-card-radius-sm);
  background: var(--app-warning-soft-bg);
  border: 1px solid var(--td-warning-color-2);
  color: var(--app-warning-soft-fg);
  font-size: 13px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}
</style>
