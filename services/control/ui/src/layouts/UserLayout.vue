<template>
  <AppLoading v-if="auth.loading || licenseGate.loading.value" fullscreen :title="`${brand.title} 控制台`" subtitle="正在为您准备工作台…" />
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
    <div class="content-wrap">
      <LicenseRequired
        v-if="licenseGate.blocked.value && !isAccountPage"
        :status="licenseGate.statusCode.value"
        :reason="licenseGate.notice.value"
        :can-manage="false"
        :on-refresh="licenseGate.fetchStatus"
      />
      <template v-else>
        <div v-if="licenseGate.blocked.value && isAccountPage" class="license-warning" role="alert">
          系统授权异常，部分功能暂不可用。如有疑问请联系管理员。
        </div>
        <div v-else-if="licenseGate.warning.value" class="license-warning" role="alert">
          {{ licenseGate.notice.value }}
        </div>
        <RouterView />
      </template>
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

const licenseGate = useLicenseGate(() => !!auth.user && auth.user.role !== "admin")

const modules: TemplateModule[] = [
  {
    id: "overview",
    name: "服务概览",
    icon: "dashboard",
    defaultHref: "/dashboard",
    items: [{ name: "服务概览", href: "/dashboard" }],
  },
  {
    id: "sites",
    name: "网站管理",
    icon: "internet",
    defaultHref: "/dashboard/domains",
    items: [
      { name: "网站列表", href: "/dashboard/domains" },
      { name: "证书管理", href: "/dashboard/certs" },
      { name: "我的规则", href: "/dashboard/rules" },
      { name: "刷新预热", href: "/dashboard/purge" },
      { name: "实时监控", href: "/dashboard/monitor" },
      { name: "拉黑日志", href: "/dashboard/blacklist-logs" },
      { name: "访问日志", href: "/dashboard/access-logs" },
    ],
  },
  {
    id: "l4",
    name: "四层转发",
    icon: "swap",
    defaultHref: "/dashboard/l4",
    items: [{ name: "四层转发", href: "/dashboard/l4" }],
  },
  {
    id: "plans",
    name: "套餐管理",
    icon: "app",
    defaultHref: "/dashboard/products",
    items: [
      { name: "产品列表", href: "/dashboard/products" },
      { name: "账户余额", href: "/dashboard/balance" },
    ],
  },
  {
    id: "account",
    name: "账户中心",
    icon: "user",
    defaultHref: "/dashboard/profile",
    items: [
      { name: "账户信息", href: "/dashboard/profile" },
      { name: "系统公告", href: "/dashboard/announcements" },
    ],
  },
]

const activeModuleId = computed(() => {
  const path = route.path
  if (path.startsWith("/dashboard/profile") || path.startsWith("/dashboard/announcements")) {
    return "account"
  }
  if (path.startsWith("/dashboard/products") || path.startsWith("/dashboard/balance")) return "plans"
  if (path.startsWith("/dashboard/l4")) return "l4"
  if (
    path.startsWith("/dashboard/domains") ||
    path.startsWith("/dashboard/certs") ||
    path.startsWith("/dashboard/rules") ||
    path.startsWith("/dashboard/purge") ||
    path.startsWith("/dashboard/monitor") ||
    path.startsWith("/dashboard/blacklist-logs") ||
    path.startsWith("/dashboard/access-logs")
  ) {
    return "sites"
  }
  return "overview"
})

const isAccountPage = computed(() => {
  const path = route.path
  return (
    path === "/dashboard" ||
    path.startsWith("/dashboard/profile") ||
    path.startsWith("/dashboard/announcements")
  )
})

watch(
  () => auth.user,
  () => {
    if (!auth.loading && !auth.user) {
      router.push("/login")
    }
    if (auth.user?.role === "admin") {
      router.push("/admin/dashboard")
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
  await router.push("/login")
}
</script>

<style scoped>
.content-wrap {
  /* Was `display: grid` with no `grid-template-columns` — the implicit
   * single column collapsed to content width (≈1280px from each page's
   * `.page { max-width: 1280px }`), leaving a wide empty band on the
   * right of any 1600+px monitor. Flex column keeps the same vertical
   * spacing while letting children stretch to the full content area.
   *
   * `flex: 1` continues the height-fill chain started in TemplateShell
   * so the routed view stretches to fill the visible content area
   * instead of leaving empty space below short pages. */
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
 * 撑满 .content-wrap 的剩余高度，短页面（空表格、空状态）才不会留
 * 大片灰底空白。`:last-child` 排除上面的 .license-warning 横幅。
 *
 * `max-width: none` + `margin: 0` 覆盖各视图 `.page` 自己设置的
 * `max-width: 1280px; margin: 0 auto`，让仪表盘真正全屏铺开，不再被
 * 1280 锁住中间一条。 */
.content-wrap > :last-child {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  max-width: none !important;
  margin: 0 !important;
}

/* 禁用子项 flex-shrink —— 仪表盘等长页面 (9+ 段卡片) 总高远超 viewport，
 * flex column 默认会等比缩压把所有子项硬塞进容器，hero 会被挤成一条线。
 * 加 flex-shrink: 0 让子项保持自然高度，溢出交给 .content-area 滚动。 */
.content-wrap > :last-child > * {
  flex-shrink: 0;
}

.license-warning {
  padding: 12px 16px;
  border-radius: var(--app-card-radius-sm);
  background: var(--app-warning-soft-bg);
  border: 1px solid var(--td-warning-color-2);
  color: var(--app-warning-soft-fg);
  font-size: 13px;
}
</style>
