import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router"
// Auth-flow views are kept eager — they're on the critical path before the
// user logs in and any lazy delay here would feel like a blank flash.
import LoginView from "@/views/LoginView.vue"
import RegisterView from "@/views/RegisterView.vue"
import ForgotPasswordView from "@/views/ForgotPasswordView.vue"
import AdminLoginView from "@/views/AdminLoginView.vue"
// Layouts are also eager — they wrap every authenticated route and are tiny.
import UserLayout from "@/layouts/UserLayout.vue"
import AdminLayout from "@/layouts/AdminLayout.vue"
import { getCachedSystemName } from "@/lib/systemSettings"
import PlaceholderView from "@/views/PlaceholderView.vue"
import { useAuthStore } from "@/stores/auth"

// All other views are lazy-loaded so the auth bundle doesn't ship admin
// code, and admins don't ship every page on first paint.
const routes: RouteRecordRaw[] = [
  { path: "/", redirect: "/login" },
  { path: "/login", name: "login", component: LoginView, meta: { title: "用户登录" } },
  { path: "/register", name: "register", component: RegisterView, meta: { title: "用户注册" } },
  { path: "/forgot-password", name: "forgot-password", component: ForgotPasswordView, meta: { title: "找回密码" } },
  { path: "/nimda", name: "admin-login", component: AdminLoginView, meta: { title: "管理员登录" } },
  {
    path: "/dashboard",
    component: UserLayout,
    meta: { requiresAuth: true },
    children: [
      { path: "", name: "dashboard", component: () => import("@/views/DashboardHomeView.vue"), meta: { title: "控制台" } },
      { path: "announcements", component: () => import("@/views/DashboardAnnouncementsView.vue"), meta: { title: "公告" } },
      { path: "balance", component: () => import("@/views/DashboardBalanceView.vue"), meta: { title: "余额" } },
      { path: "domains", component: () => import("@/views/DashboardDomainsView.vue"), meta: { title: "域名管理" } },
      { path: "domains/:id", component: () => import("@/views/DomainDetailView.vue"), meta: { title: "域名详情" } },
      { path: "certs", component: () => import("@/views/DashboardCertsView.vue"), meta: { title: "证书" } },
      { path: "rules", component: () => import("@/views/DashboardRulesView.vue"), meta: { title: "我的规则" } },
      { path: "l4", component: () => import("@/views/DashboardL4View.vue"), meta: { title: "L4 配置" } },
      { path: "monitor", component: () => import("@/views/DashboardMonitorView.vue"), meta: { title: "监控" } },
      { path: "blacklist-logs", component: () => import("@/views/DashboardBlacklistLogsView.vue"), meta: { title: "黑名单日志" } },
      { path: "access-logs", component: () => import("@/views/DashboardAccessLogsView.vue"), meta: { title: "访问日志" } },
      { path: "products", component: () => import("@/views/DashboardProductsView.vue"), meta: { title: "产品" } },
      { path: "profile", component: () => import("@/views/DashboardProfileView.vue"), meta: { title: "个人资料" } },
      { path: "purge", component: () => import("@/views/DashboardPurgeView.vue"), meta: { title: "缓存刷新" } },
    ],
  },
  {
    path: "/admin/dashboard",
    component: AdminLayout,
    meta: { requiresAuth: true, requiresAdmin: true },
    children: [
      { path: "", name: "admin-dashboard", component: () => import("@/views/AdminDashboardHomeView.vue"), meta: { title: "管理员控制台" } },
      { path: "announcements", component: () => import("@/views/AdminAnnouncementsView.vue"), meta: { title: "公告管理" } },
      { path: "balance", component: () => import("@/views/AdminBalanceView.vue"), meta: { title: "余额管理" } },
      { path: "domains", component: () => import("@/views/AdminDomainsView.vue"), meta: { title: "域名管理" } },
      { path: "domains/health", component: () => import("@/views/AdminDomainHealthView.vue"), meta: { title: "域名健康" } },
      { path: "domains/:id", component: () => import("@/views/DomainDetailView.vue"), meta: { title: "域名详情" } },
      { path: "cache-rules", component: () => import("@/views/AdminCacheRulesView.vue"), meta: { title: "缓存规则" } },
      { path: "waf", component: () => import("@/views/AdminWAFPoliciesView.vue"), meta: { title: "WAF 策略" } },
      { path: "license-center", component: () => import("@/views/AdminLicenseCenterView.vue"), meta: { title: "授权中心" } },
      { path: "logs", component: () => import("@/views/AdminLogsView.vue"), meta: { title: "系统日志" } },
      { path: "clusters", component: () => import("@/views/AdminClustersView.vue"), meta: { title: "集群管理" } },
      { path: "nodes", component: () => import("@/views/AdminNodesView.vue"), meta: { title: "节点管理" } },
      { path: "nodes/monitor", component: () => import("@/views/AdminNodeMonitorView.vue"), meta: { title: "节点监控" } },
      { path: "nodes/dns", component: () => import("@/views/AdminDNSConfigView.vue"), meta: { title: "DNS 配置" } },
      { path: "orders", component: () => import("@/views/AdminOrdersView.vue"), meta: { title: "订单管理" } },
      { path: "products", component: () => import("@/views/AdminProductsView.vue"), meta: { title: "产品管理" } },
      { path: "finance/stats", component: () => import("@/views/AdminFinanceStatsView.vue"), meta: { title: "财务统计" } },
      { path: "profile", component: () => import("@/views/AdminProfileView.vue"), meta: { title: "管理员资料" } },
      { path: "settings", component: () => import("@/views/AdminSettingsView.vue"), meta: { title: "系统设置" } },
      { path: "tasks", component: () => import("@/views/AdminTasksView.vue"), meta: { title: "系统任务" } },
      { path: "tasks/publish/:id", component: () => import("@/views/AdminPublishTaskView.vue"), meta: { title: "发布任务详情" } },
      { path: "templates", component: () => import("@/views/AdminTemplatesView.vue"), meta: { title: "模板管理" } },
      { path: "upgrade", component: () => import("@/views/AdminUpgradeView.vue"), meta: { title: "系统升级" } },
      { path: "upgrade/:id", component: () => import("@/views/AdminUpgradeTaskView.vue"), meta: { title: "升级详情" } },
      { path: "users", component: () => import("@/views/AdminUsersView.vue"), meta: { title: "用户管理" } },
      { path: "ddos", component: () => import("@/views/AdminDdosView.vue"), meta: { title: "DDoS 防护" } },
      { path: "l4", component: () => import("@/views/AdminL4View.vue"), meta: { title: "L4 管理" } },
      { path: "certificates", component: () => import("@/views/AdminCertsView.vue"), meta: { title: "证书管理" } },
    ],
  },
  { path: "/:pathMatch(.*)*", component: PlaceholderView, meta: { title: "页面不存在" } },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
  scrollBehavior() {
    return { top: 0 }
  },
})

const guestPages = new Set(["login", "register", "forgot-password", "admin-login"])

router.beforeEach(async (to) => {
  const auth = useAuthStore()
  if (auth.loading) {
    await auth.checkAuth()
  }
  if (to.meta.requiresAuth) {
    if (!auth.user) {
      return to.meta.requiresAdmin ? "/nimda" : "/login"
    }
    if (to.meta.requiresAdmin && auth.user.role !== "admin") {
      return "/dashboard"
    }
  }
  if ((guestPages.has(to.name as string) || to.path === "/") && auth.user) {
    return auth.user.role === "admin" ? "/admin/dashboard" : "/dashboard"
  }
  return true
})

router.afterEach((to) => {
  const brandName = getCachedSystemName()
  const title = typeof to.meta.title === "string" ? to.meta.title : `${brandName} 控制台`
  if (typeof document !== "undefined") {
    document.title = title
  }
})

export default router
