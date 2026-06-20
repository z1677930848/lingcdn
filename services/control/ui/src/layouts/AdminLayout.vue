<template>
  <AppLoading v-if="auth.loading || licenseGate.loading.value" fullscreen :title="`${brand.title} 管理后台`" subtitle="正在加载管理控制台…" />
  <TemplateShell
    v-else-if="auth.user"
    :brand="brand"
    :standalone-items="standaloneItems"
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
        <el-button v-if="!isLicenseCenter" plain size="small" @click="router.push('/admin/dashboard/license-center')">
          前往授权中心
        </el-button>
      </div>
      <RouterView />
    </div>
  </TemplateShell>
</template>

<script setup lang="ts">
import { computed, onMounted, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import TemplateShell from "@/components/layout/TemplateShell.vue"
import type { TemplateModule, TemplateStandaloneNavItem } from "@/components/layout/template-shell.types"
import LicenseRequired from "@/components/license/LicenseRequired.vue"
import AppLoading from "@/components/atoms/AppLoading.vue"
import { useAuthStore } from "@/stores/auth"
import { useSystemSettings } from "@/lib/systemSettings"
import { useLicenseGate } from "@/composables/useLicenseGate"
import { resolveModuleIdFromPath } from "@/lib/templateMenu"
import { ADMIN_SETTINGS_NAV_ITEMS } from "@/lib/adminSettingsNav"

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { brand, footerLinks, footerCopyright } = useSystemSettings()

const licenseGate = useLicenseGate(() => !!auth.user && auth.user.role === "admin")

const standaloneItems: TemplateStandaloneNavItem[] = [
  { name: "管理概览", href: "/admin/dashboard", icon: "dashboard" },
]

const modules: TemplateModule[] = [
  {
    id: "nodes",
    name: "节点管理",
    icon: "server",
    defaultHref: "/admin/dashboard/nodes",
    items: [
      { name: "节点列表", href: "/admin/dashboard/nodes" },
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
      { name: "拉黑日志", href: "/admin/dashboard/blacklist-logs" },
    ],
  },
  {
    id: "domains",
    name: "域名管理",
    icon: "internet",
    defaultHref: "/admin/dashboard/domains",
    items: [
      { name: "域名列表", href: "/admin/dashboard/domains" },
      { name: "网站监控", href: "/admin/dashboard/domains/health" },
      { name: "访问日志", href: "/admin/dashboard/access-logs" },
      { name: "缓存规则", href: "/admin/dashboard/cache-rules" },
      { name: "证书管理", href: "/admin/dashboard/certificates" },
    ],
  },
  {
    id: "l4",
    name: "L4 管理",
    icon: "swap",
    defaultHref: "/admin/dashboard/l4",
    items: [{ name: "转发列表", href: "/admin/dashboard/l4" }],
  },
  {
    id: "global",
    name: "系统配置",
    icon: "setting",
    defaultHref: ADMIN_SETTINGS_NAV_ITEMS[0].href,
    items: [
      ...ADMIN_SETTINGS_NAV_ITEMS.map((item) => ({ name: item.label, href: item.href })),
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
      { name: "工单管理", href: "/admin/dashboard/tickets" },
      { name: "操作日志", href: "/admin/dashboard/logs" },
      { name: "系统升级", href: "/admin/dashboard/upgrade" },
    ],
  },
]

const activeModuleId = computed(() => resolveModuleIdFromPath(route.path, modules, standaloneItems))

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

