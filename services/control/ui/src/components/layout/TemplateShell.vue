<template>
  <el-container class="shell-root">
    <el-container class="shell-body">
      <el-drawer v-if="isMobile" v-model="drawerOpen" direction="ltr" size="280px" :with-header="true" append-to-body>
        <template #header>
          <button type="button" class="drawer-brand" :class="brandModeClass" @click="handleBrandClick">
            <template v-if="showSidebarLogo">
              <div class="sidebar-brand-logo" :class="{ 'sidebar-brand-logo--solo': brandDisplay === 'logo' }">
                <img :src="brand.logo" :alt="brand.title" />
              </div>
            </template>
            <div v-else-if="showSidebarAvatar" class="sidebar-brand-avatar">{{ brandInitial }}</div>
            <span v-if="showSidebarTitle" class="sidebar-brand-text">{{ brand.title }}</span>
          </button>
        </template>
        <el-menu
          class="shell-menu sidebar-nav-menu"
          :default-active="menuActivePath"
          :collapse="false"
          :default-openeds="expanded"
          unique-opened
          @select="handleMenuSelect"
          @open="handleMenuOpen"
        >
          <el-menu-item v-for="item in standaloneItems" :key="item.href" :index="item.href">
            <EpIcon v-if="item.icon" :name="item.icon" />
            <span>{{ item.name }}</span>
          </el-menu-item>
          <template v-for="group in modules" :key="group.id">
            <el-menu-item v-if="isFlatModule(group)" :index="group.items[0].href">
              <EpIcon v-if="group.icon" :name="group.icon" />
              <span>{{ group.name }}</span>
            </el-menu-item>
            <el-sub-menu v-else :index="group.id">
              <template #title>
                <EpIcon v-if="group.icon" :name="group.icon" />
                <span>{{ group.name }}</span>
              </template>
              <el-menu-item v-for="item in getSubmenuItems(group)" :key="item.href" :index="item.href">
                {{ item.name }}
              </el-menu-item>
            </el-sub-menu>
          </template>
        </el-menu>
      </el-drawer>

      <el-aside v-else class="shell-aside" :width="asideWidth">
        <button
          type="button"
          class="sidebar-brand"
          :class="[{ collapsed }, brandModeClass]"
          :aria-label="`${brand.title || '控制台'} 首页`"
          @click="navigate(activeModule.defaultHref)"
        >
          <template v-if="showSidebarLogo">
            <div class="sidebar-brand-logo" :class="{ 'sidebar-brand-logo--solo': brandDisplay === 'logo' && !collapsed }">
              <img :src="brand.logo" :alt="brand.title" />
            </div>
          </template>
          <div v-else-if="showSidebarAvatar" class="sidebar-brand-avatar">{{ brandInitial }}</div>
          <transition name="brand-fade">
            <span v-if="showSidebarTitle" class="sidebar-brand-text">{{ brand.title }}</span>
          </transition>
        </button>
        <el-menu
          class="shell-menu sidebar-nav-menu"
          :default-active="menuActivePath"
          :collapse="menuCollapsed"
          :default-openeds="expanded"
          unique-opened
          @select="handleMenuSelect"
          @open="handleMenuOpen"
        >
          <el-menu-item v-for="item in standaloneItems" :key="item.href" :index="item.href">
            <EpIcon v-if="item.icon" :name="item.icon" />
            <span>{{ item.name }}</span>
          </el-menu-item>
          <template v-for="group in modules" :key="group.id">
            <el-menu-item v-if="isFlatModule(group)" :index="group.items[0].href">
              <EpIcon v-if="group.icon" :name="group.icon" />
              <span>{{ group.name }}</span>
            </el-menu-item>
            <el-sub-menu v-else :index="group.id">
              <template #title>
                <EpIcon v-if="group.icon" :name="group.icon" />
                <span>{{ group.name }}</span>
              </template>
              <el-menu-item v-for="item in getSubmenuItems(group)" :key="item.href" :index="item.href">
                {{ item.name }}
              </el-menu-item>
            </el-sub-menu>
          </template>
        </el-menu>
      </el-aside>

      <el-container class="main-column">
        <el-header class="shell-header">
          <div class="header-content">
            <div class="header-nav">
              <button type="button" class="hdr-icon-btn" :aria-label="toggleAriaLabel" @click.stop="toggleSide">
                <component :is="toggleIcon" :size="20" />
              </button>
              <nav class="header-breadcrumb" aria-label="breadcrumb">
                <span
                  v-for="(item, idx) in breadcrumb"
                  :key="`${item}-${idx}`"
                  class="breadcrumb-item"
                  :class="{ current: idx === breadcrumb.length - 1 }"
                >
                  <ChevronRight v-if="idx > 0" :size="12" class="breadcrumb-sep" aria-hidden="true" />
                  {{ item }}
                </span>
              </nav>
            </div>

            <div class="shell-header-actions">
              <button type="button" class="hdr-icon-btn" aria-label="公告" @click="openNotifications">
                <Bell :size="20" />
              </button>
              <button
                type="button"
                class="hdr-icon-btn"
                :aria-label="isDark ? '切换到浅色模式' : '切换到深色模式'"
                @click="toggleTheme"
              >
                <Sun v-if="isDark" :size="20" />
                <Moon v-else :size="20" />
              </button>
              <el-dropdown trigger="click" @command="handleUserCommand">
                <button type="button" class="user-trigger" :aria-label="`${user.username || '用户'} 菜单`">
                  <el-avatar :size="32" class="avatar">{{ userInitial }}</el-avatar>
                  <span class="username">{{ user.username }}</span>
                  <ChevronDown :size="14" class="chevron-icon" />
                </button>
                <template #dropdown>
                  <el-dropdown-menu>
                    <el-dropdown-item command="profile">个人中心</el-dropdown-item>
                    <el-dropdown-item command="logout">退出登录</el-dropdown-item>
                  </el-dropdown-menu>
                </template>
              </el-dropdown>
            </div>
          </div>
        </el-header>

        <el-container class="content-area">
          <el-main class="content-scroll">
            <transition name="route-fade" mode="out-in">
              <div :key="activePath" class="content-inner">
                <slot />
                <div class="shell-footer">
                  <div class="footer-inner">
                    <span>{{ footerText }}</span>
                    <span v-if="footerLinks.length" class="footer-links">
                      <a
                        v-for="(link, idx) in footerLinks"
                        :key="`${link.href}-${idx}`"
                        :href="link.href"
                        target="_blank"
                        rel="noreferrer"
                      >
                        {{ link.label }}
                      </a>
                    </span>
                  </div>
                </div>
              </div>
            </transition>
          </el-main>
        </el-container>
      </el-container>
    </el-container>
  </el-container>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import {
  Menu as MenuIcon,
  PanelLeftClose,
  PanelLeftOpen,
  Sun,
  Moon,
  Bell,
  ChevronDown,
  ChevronRight,
} from "lucide-vue-next"
import EpIcon from "@/components/ep/EpIcon.vue"
import type { FooterLink } from "@/lib/systemSettings"
import type { TemplateModule, TemplateShellBrand, TemplateShellUser, TemplateStandaloneNavItem } from "@/components/layout/template-shell.types"
import {
  buildTitleMap,
  getSubmenuItems,
  isFlatModule,
  resolveMenuActivePath,
  resolveModuleFromPath,
  resolveModuleIdFromPath,
  resolveStandaloneFromPath,
} from "@/lib/templateMenu"
import { useTheme } from "@/composables/useTheme"

const props = defineProps<{
  brand: TemplateShellBrand
  standaloneItems?: TemplateStandaloneNavItem[]
  modules: TemplateModule[]
  activeModuleId: string
  activePath: string
  user: TemplateShellUser
  footerLinks?: FooterLink[]
  footerCopyright?: string
}>()

const emit = defineEmits<{
  (e: "navigate", href: string): void
  (e: "logout"): void
}>()

const collapsed = ref(false)
const desktopCollapsed = ref(false)
const isMobile = ref(false)
const drawerOpen = ref(false)
const expanded = ref<string[]>([])

const route = useRoute()
const { isDark, toggle: toggleTheme } = useTheme()

const MOBILE_BREAKPOINT = 768
const RAIL_BREAKPOINT = 1080

const standaloneItems = computed(() => props.standaloneItems || [])

const activeModule = computed(() => {
  const standalone = resolveStandaloneFromPath(props.activePath, standaloneItems.value)
  if (standalone) {
    return {
      id: standalone.href,
      name: standalone.name,
      defaultHref: standalone.href,
      items: [{ name: standalone.name, href: standalone.href }],
    }
  }

  const id = resolveModuleIdFromPath(props.activePath, props.modules, standaloneItems.value)
  return (
    props.modules.find((m) => m.id === id) ||
    props.modules.find((m) => m.id === props.activeModuleId) || {
      id: "info",
      name: "控制台",
      defaultHref: standaloneItems.value[0]?.href || props.modules[0]?.defaultHref || "/dashboard",
      items: [],
    }
  )
})

const titleMap = computed(() => buildTitleMap(props.modules, standaloneItems.value))

const menuActivePath = computed(() =>
  resolveMenuActivePath(props.activePath, props.modules, standaloneItems.value)
)

const footerLinks = computed(() => props.footerLinks || [])
const footerText = computed(() => props.footerCopyright || `© ${new Date().getFullYear()} ${props.brand.title}. All rights reserved.`)

const brandDisplay = computed(() => (props.brand.display === "logo" ? "logo" : "name"))
const brandInitial = computed(() => props.brand.title?.charAt(0) || "C")
const brandModeClass = computed(() =>
  brandDisplay.value === "logo" ? "sidebar-brand--logo-only" : "sidebar-brand--name-only"
)
const showSidebarLogo = computed(() => brandDisplay.value === "logo" && Boolean(props.brand.logo))
const showSidebarTitle = computed(() => brandDisplay.value === "name" && !collapsed.value)
const showSidebarAvatar = computed(() => {
  if (brandDisplay.value === "logo") return !props.brand.logo
  return collapsed.value
})

const menuCollapsed = computed(() => (isMobile.value ? false : collapsed.value))
const asideWidth = computed(() =>
  collapsed.value ? `var(--app-sidebar-width-collapsed)` : `var(--app-sidebar-width-expanded)`
)
const toggleIcon = computed(() => {
  if (isMobile.value) return MenuIcon
  return collapsed.value ? PanelLeftOpen : PanelLeftClose
})
const toggleAriaLabel = computed(() => {
  if (isMobile.value) return drawerOpen.value ? "关闭菜单" : "打开菜单"
  return collapsed.value ? "展开侧边栏" : "收起侧边栏"
})

const resolvedModuleId = computed(() =>
  resolveModuleIdFromPath(props.activePath, props.modules, standaloneItems.value)
)

const resolveTitle = computed(() => {
  const entries = [
    ...standaloneItems.value.map((item) => ({ href: item.href, title: item.name })),
    ...props.modules.flatMap((m) => m.items.map((item) => ({ href: item.href, title: item.name }))),
  ]
  entries.sort((a, b) => b.href.length - a.href.length)
  return (href: string) => {
    const exact = titleMap.value.get(href)
    if (exact) return exact
    const best = entries.find((x) => href === x.href || href.startsWith(`${x.href}/`))
    if (best) return best.title
    const metaTitle = typeof route.meta?.title === "string" ? route.meta.title : ""
    if (metaTitle) return metaTitle
    const seg = href.split("?")[0].split("#")[0].split("/").filter(Boolean).pop()
    return seg ? decodeURIComponent(seg) : "页面"
  }
})

const breadcrumb = computed(() => {
  const standalone = resolveStandaloneFromPath(props.activePath, standaloneItems.value)
  const moduleName = activeModule.value?.name || ""
  const pageTitle = resolveTitle.value(props.activePath)
  const metaTitle = typeof route.meta?.title === "string" ? route.meta.title : ""
  const crumbs = ["首页"]

  if (standalone) {
    if (pageTitle) crumbs.push(pageTitle)
    return crumbs
  }

  const mod = resolveModuleFromPath(props.activePath, props.modules, standaloneItems.value)
  if (mod && !isFlatModule(mod)) {
    crumbs.push(moduleName)
  }
  if (pageTitle && pageTitle !== moduleName) {
    crumbs.push(pageTitle)
  } else if (mod && isFlatModule(mod) && moduleName) {
    crumbs.push(moduleName)
  }
  if (metaTitle && metaTitle !== pageTitle && metaTitle !== moduleName) {
    crumbs.push(metaTitle)
  }
  return crumbs
})

const userInitial = computed(() => props.user.username?.charAt(0).toUpperCase() || "U")

const handleResize = () => {
  const width = window.innerWidth
  const mobile = width < MOBILE_BREAKPOINT
  const inRailRange = !mobile && width < RAIL_BREAKPOINT

  if (mobile) {
    isMobile.value = true
    collapsed.value = true
    drawerOpen.value = false
    return
  }

  isMobile.value = false
  drawerOpen.value = false
  collapsed.value = inRailRange ? true : desktopCollapsed.value
}

const toggleSide = () => {
  if (isMobile.value) {
    drawerOpen.value = !drawerOpen.value
    return
  }
  const next = !collapsed.value
  collapsed.value = next
  if (window.innerWidth >= RAIL_BREAKPOINT) {
    desktopCollapsed.value = next
  }
}

const navigate = (href: string) => {
  emit("navigate", href)
}

const openNotifications = () => {
  const href = props.activePath.startsWith("/admin") ? "/admin/dashboard/announcements" : "/dashboard/announcements"
  emit("navigate", href)
}

const handleBrandClick = () => {
  emit("navigate", activeModule.value.defaultHref)
  if (isMobile.value) {
    drawerOpen.value = false
  }
}

const handleUserCommand = (command: string) => {
  if (command === "logout") {
    emit("logout")
  } else if (command === "profile") {
    const profileHref = props.activePath.startsWith("/admin") ? "/admin/dashboard/profile" : "/dashboard/profile"
    emit("navigate", profileHref)
  }
}

const handleMenuSelect = (index: string) => {
  if (!index.startsWith("/")) return
  emit("navigate", index)
  if (isMobile.value) {
    drawerOpen.value = false
  }
}

const handleMenuOpen = (index: string) => {
  expanded.value = [index]
}

watch(
  () => [resolvedModuleId.value, collapsed.value],
  () => {
    if (collapsed.value) return
    expanded.value = resolvedModuleId.value ? [resolvedModuleId.value] : []
  },
  { immediate: true }
)

onMounted(() => {
  handleResize()
  window.addEventListener("resize", handleResize)
})

onUnmounted(() => {
  window.removeEventListener("resize", handleResize)
})
</script>

<script lang="ts">
export default { name: "TemplateShell" }
</script>
