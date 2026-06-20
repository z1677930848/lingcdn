<template>
  <AppLoading v-if="auth.loading || licenseGate.loading.value" fullscreen :title="`${brand.title} 控制台`" subtitle="正在为您准备工作台…" />
  <TemplateShell
    v-else-if="auth.user"
    :brand="brand"
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
import { resolveModuleIdFromPath } from "@/lib/templateMenu"
import { useLicenseGate } from "@/composables/useLicenseGate"
import {
  PermBalanceWithdraw,
  PermCertificatesWrite,
  PermDomainsWrite,
  PermOrdersPurchase,
  PermPurgeExecute,
  PermStreamForwardsWrite,
  PermWAFEdit,
  userPermissionAllowed,
} from "@/lib/permissions"

const route = useRoute()
const router = useRouter()
const auth = useAuthStore()
const { brand, footerLinks, footerCopyright } = useSystemSettings()

const licenseGate = useLicenseGate(() => !!auth.user && auth.user.role !== "admin")

const perms = computed(() => auth.user?.permissions)
const can = (perm: string) => userPermissionAllowed(perms.value, perm)

const allModules: TemplateModule[] = [
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
      { name: "网站列表", href: "/dashboard/domains", perm: PermDomainsWrite },
      { name: "证书管理", href: "/dashboard/certs", perm: PermCertificatesWrite },
      { name: "我的规则", href: "/dashboard/rules", perm: PermWAFEdit },
      { name: "刷新预热", href: "/dashboard/purge", perm: PermPurgeExecute },
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
    items: [{ name: "转发列表", href: "/dashboard/l4", perm: PermStreamForwardsWrite }],
  },
  {
    id: "plans",
    name: "套餐管理",
    icon: "app",
    defaultHref: "/dashboard/products",
    items: [
      { name: "产品列表", href: "/dashboard/products", perm: PermOrdersPurchase },
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
      { name: "工单支持", href: "/dashboard/tickets" },
    ],
  },
]

type ModuleItem = TemplateModule["items"][number] & { perm?: string }

const modules = computed<TemplateModule[]>(() => {
  return allModules
    .map((mod) => {
      const items = (mod.items as ModuleItem[]).filter((item) => !item.perm || can(item.perm))
      if (items.length === 0) return null
      return { ...mod, items }
    })
    .filter(Boolean) as TemplateModule[]
})

const activeModuleId = computed(() => resolveModuleIdFromPath(route.path, modules.value))

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

