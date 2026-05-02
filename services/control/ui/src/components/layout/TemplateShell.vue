<template>
  <t-layout class="shell-root">
    <t-header class="shell-header">
      <!-- Brand zone: same width as sidebar, acts as the visual anchor -->
      <button
        type="button"
        class="header-brand"
        :class="{ collapsed: !isMobile && collapsed }"
        :aria-label="`${brand.title || '控制台'} 首页`"
        @click="navigate(activeModule.defaultHref)"
      >
        <div v-if="brand.logo" class="header-brand-logo">
          <img :src="brand.logo" :alt="brand.title" />
        </div>
        <div v-else class="header-brand-avatar">{{ brand.title?.charAt(0) || 'C' }}</div>
        <transition name="brand-fade">
          <span v-if="isMobile || !collapsed" class="header-brand-text">{{ brand.title }}</span>
        </transition>
      </button>

      <!-- Subtle divider between brand zone and content zone -->
      <div class="header-divider" aria-hidden="true" />

      <!-- Content zone: collapse toggle + breadcrumb ... user actions -->
      <div class="header-content">
        <div class="header-nav">
          <button type="button" class="hdr-icon-btn" :aria-label="toggleAriaLabel" @click.stop="toggleSide">
            <component :is="toggleIcon" :size="18" />
          </button>
          <nav class="header-breadcrumb" aria-label="breadcrumb">
            <span v-for="(item, idx) in breadcrumb" :key="`${item}-${idx}`" class="breadcrumb-item" :class="{ current: idx === breadcrumb.length - 1 }">
              <ChevronRight v-if="idx > 0" :size="12" class="breadcrumb-sep" aria-hidden="true" />
              {{ item }}
            </span>
          </nav>
        </div>

        <div class="header-actions">
          <button type="button" class="hdr-icon-btn" aria-label="通知" @click="openNotifications">
            <Bell :size="18" />
            <span class="bell-dot" aria-hidden="true" />
          </button>
          <button
            type="button"
            class="hdr-icon-btn"
            :aria-label="isDark ? '切换到浅色模式' : '切换到深色模式'"
            @click="toggleTheme"
          >
            <Sun v-if="isDark" :size="18" />
            <Moon v-else :size="18" />
          </button>
          <t-dropdown :options="userOptions" @click="handleUserAction">
            <button type="button" class="user-trigger" :aria-label="`${user.username || '用户'} 菜单`">
              <t-avatar size="small" class="avatar">{{ userInitial }}</t-avatar>
              <span class="username">{{ user.username }}</span>
              <ChevronDown :size="14" class="chevron-icon" />
            </button>
          </t-dropdown>
        </div>
      </div>
    </t-header>

    <t-layout class="shell-body">
      <t-drawer v-if="isMobile" v-model:visible="drawerOpen" placement="left" size="260px">
        <template #header>
          <button type="button" class="drawer-brand" @click="handleBrandClick">
            <div v-if="brand.logo" class="header-brand-logo">
              <img :src="brand.logo" :alt="brand.title" />
            </div>
            <span class="header-brand-text">{{ brand.title }}</span>
          </button>
        </template>
        <template #body>
          <t-menu
            :value="menuActivePath"
            :collapsed="menuCollapsed"
            :expanded="expanded"
            :expand-mutex="true"
            @change="handleMenuChange"
            @expand="handleExpand"
          >
            <template v-for="group in modules" :key="group.id">
              <t-menu-item v-if="isFlatModule(group)" :value="group.items[0].href">
                <template v-if="group.icon" #icon>
                  <t-icon :name="group.icon" :aria-label="group.name" :title="group.name" />
                </template>
                {{ group.name }}
              </t-menu-item>
              <t-submenu v-else :value="group.id" :title="group.name">
                <template v-if="group.icon" #icon>
                  <t-icon :name="group.icon" :aria-label="group.name" :title="group.name" />
                </template>
                <t-menu-item v-for="item in group.items" :key="item.href" :value="item.href">
                  {{ item.name }}
                </t-menu-item>
              </t-submenu>
            </template>
          </t-menu>
        </template>
      </t-drawer>

      <t-aside v-else class="shell-aside" :width="asideWidth">
        <t-menu
          class="shell-menu"
          :value="menuActivePath"
          :collapsed="menuCollapsed"
          :expanded="expanded"
          :expand-mutex="true"
          @change="handleMenuChange"
          @expand="handleExpand"
        >
          <template v-for="group in modules" :key="group.id">
            <t-menu-item v-if="isFlatModule(group)" :value="group.items[0].href">
              <template v-if="group.icon" #icon>
                <t-icon :name="group.icon" :aria-label="group.name" :title="group.name" />
              </template>
              {{ group.name }}
            </t-menu-item>
            <t-submenu v-else :value="group.id" :title="group.name">
              <template v-if="group.icon" #icon>
                <t-icon :name="group.icon" :aria-label="group.name" :title="group.name" />
              </template>
              <t-menu-item v-for="item in group.items" :key="item.href" :value="item.href">
                {{ item.name }}
              </t-menu-item>
            </t-submenu>
          </template>
        </t-menu>
      </t-aside>

      <t-layout class="content-area">
        <t-content>
          <transition name="route-fade" mode="out-in">
            <div :key="activePath" class="content-inner">
              <slot />
            </div>
          </transition>
        </t-content>
        <t-footer class="shell-footer">
          <div class="footer-inner">
            <span>{{ footerText }}</span>
            <span v-if="footerLinks.length" class="footer-links">
              <a v-for="(link, idx) in footerLinks" :key="`${link.href}-${idx}`" :href="link.href" target="_blank" rel="noreferrer">
                {{ link.label }}
              </a>
            </span>
          </div>
        </t-footer>
      </t-layout>
    </t-layout>
  </t-layout>
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
import type { FooterLink } from "@/lib/systemSettings"
import type { TemplateModule, TemplateShellBrand, TemplateShellUser } from "@/components/layout/template-shell.types"
import { useTheme } from "@/composables/useTheme"

const props = defineProps<{
  brand: TemplateShellBrand
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

// `collapsed` is the runtime sidebar state (what the user actually sees).
// `desktopCollapsed` is the *user's preference* on a wide desktop — auto-collapse
// in the 768–1080 mid-range must NOT mutate this preference, otherwise expanding
// the window back past 1080 would not restore the sidebar.
const collapsed = ref(false)
const desktopCollapsed = ref(false)
const isMobile = ref(false)
const drawerOpen = ref(false)
const expanded = ref<string[]>([])

const route = useRoute()
const { isDark, toggle: toggleTheme } = useTheme()

const MOBILE_BREAKPOINT = 768
const RAIL_BREAKPOINT = 1080

const activeModule = computed(
  () =>
    props.modules.find((m) => m.id === props.activeModuleId) || {
      id: "default",
      name: "控制台",
      defaultHref: "/dashboard",
      items: [],
    }
)
const footerLinks = computed(() => props.footerLinks || [])
const footerText = computed(() => props.footerCopyright || `© ${new Date().getFullYear()} ${props.brand.title}. All rights reserved.`)
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

const titleMap = computed(() => {
  const map = new Map<string, string>()
  props.modules.forEach((m) => m.items.forEach((item) => map.set(item.href, item.name)))
  return map
})

// Menu items in `modules` are concrete paths (e.g. `/admin/dashboard/domains`).
// Detail routes like `/admin/dashboard/domains/123` won't match any item, so
// the active highlight is lost. Resolve the deepest menu item whose href is a
// prefix of the current path, falling back to the active module's defaultHref
// so at least the parent submenu's section anchor stays highlighted.
const menuItemsByLength = computed(() =>
  props.modules
    .flatMap((m) => m.items.map((item) => item.href))
    .sort((a, b) => b.length - a.length)
)

const menuActivePath = computed(() => {
  const path = props.activePath
  if (titleMap.value.has(path)) return path
  const match = menuItemsByLength.value.find((href) => path === href || path.startsWith(`${href}/`))
  return match || activeModule.value.defaultHref
})

const resolveTitle = computed(() => {
  const entries = props.modules.flatMap((m) => m.items.map((item) => ({ href: item.href, title: item.name })))
  entries.sort((a, b) => b.href.length - a.href.length)
  return (href: string) => {
    const exact = titleMap.value.get(href)
    if (exact) return exact
    const best = entries.find((x) => href === x.href || href.startsWith(`${x.href}/`))
    if (best) return best.title
    // Fallback chain: route meta title → last URL segment → "页面"
    const metaTitle = typeof route.meta?.title === "string" ? route.meta.title : ""
    if (metaTitle) return metaTitle
    const seg = href.split("?")[0].split("#")[0].split("/").filter(Boolean).pop()
    return seg ? decodeURIComponent(seg) : "页面"
  }
})

const breadcrumb = computed(() => {
  const moduleName = activeModule.value?.name || ""
  const baseTitle = resolveTitle.value(props.activePath)
  const metaTitle = typeof route.meta?.title === "string" ? route.meta.title : ""
  // For deeper routes (e.g. /admin/dashboard/domains/123) the menu won't have
  // a matching entry. If route.meta.title disagrees with the menu-resolved
  // title, append it as a deeper crumb so the user knows where they are.
  const crumbs = ["Dashboard"]
  if (moduleName) crumbs.push(moduleName)
  if (baseTitle && baseTitle !== moduleName) crumbs.push(baseTitle)
  if (metaTitle && metaTitle !== baseTitle && metaTitle !== moduleName) {
    crumbs.push(metaTitle)
  }
  return crumbs
})

const userOptions = computed(() => [
  { content: "个人中心", value: "profile" },
  { content: "退出登录", value: "logout" },
])

const userInitial = computed(() => props.user.username?.charAt(0).toUpperCase() || "U")

// Modules with a single child whose name matches the module name (e.g.
// "管理概览 / 管理概览", "四层转发 / 四层转发") used to render as a
// submenu wrapping a same-named item — TDesign showed both the submenu
// title AND the duplicate child, producing two stacked rows of identical
// text. Fold those into a flat <t-menu-item> so the sidebar reads
// cleanly. We keep the same flattening rule for any single-item module
// since the wrapper submenu adds no information in that case either.
const isFlatModule = (group: TemplateModule): boolean => {
  if (!group.items || group.items.length !== 1) return false
  return true
}

const handleResize = () => {
  const width = window.innerWidth
  const mobile = width < MOBILE_BREAKPOINT
  // Three-tier breakpoints:
  //   < 768px:        mobile drawer
  //   768–1080px:     desktop but auto-collapsed (rail) so content has room
  //   >= 1080px:      desktop fully expanded; respect user's manual preference
  const inRailRange = !mobile && width < RAIL_BREAKPOINT

  if (mobile) {
    isMobile.value = true
    collapsed.value = true
    drawerOpen.value = false
    return
  }

  isMobile.value = false
  drawerOpen.value = false
  // In rail range, force-collapse without touching desktopCollapsed (preference
  // is preserved for when the window widens back).
  collapsed.value = inRailRange ? true : desktopCollapsed.value
}

const toggleSide = () => {
  if (isMobile.value) {
    drawerOpen.value = !drawerOpen.value
    return
  }
  const next = !collapsed.value
  collapsed.value = next
  // Only persist as preference when the window is wide enough that collapse
  // is a true user choice, not an auto-rail.
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

const handleUserAction = (data: { value: string }) => {
  if (data.value === "logout") {
    emit("logout")
  } else if (data.value === "profile") {
    const profileHref = props.activePath.startsWith("/admin") ? "/admin/dashboard/profile" : "/dashboard/profile"
    emit("navigate", profileHref)
  }
}

const handleMenuChange = (value: string) => {
  const nextValue = String(value)
  if (nextValue.startsWith("/")) {
    emit("navigate", nextValue)
  } else {
    const module = props.modules.find((item) => item.id === nextValue)
    if (module?.defaultHref) {
      emit("navigate", module.defaultHref)
    }
  }
  if (isMobile.value) {
    drawerOpen.value = false
  }
}

const handleExpand = (value: string[]) => {
  expanded.value = value.map(String)
}

watch(
  () => [props.activeModuleId, collapsed.value],
  () => {
    if (collapsed.value) return
    if (!expanded.value.includes(props.activeModuleId)) {
      expanded.value = [props.activeModuleId]
    }
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

<style scoped>
.shell-root {
  /* App-shell layout: lock the entire chrome to the viewport so only the
   * content area scrolls. Without this, long dashboard pages cause the
   * BODY to scroll, which drags the sidebar up out of view (the user
   * reported this as "侧边栏显示不全"). With height: 100vh + overflow:
   * hidden on the root and overflow:auto only on .content-area, the
   * sidebar stays pinned and content scrolls independently. */
  height: 100vh;
  overflow: hidden;
  background: var(--app-bg-page);
  display: flex;
  flex-direction: column;
}

/* Inner row layout (sidebar + content). flex:1 so it fills the space
 * below the header; min-height:0 so its children's overflow:auto can
 * actually engage. */
.shell-body {
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

/* ---- Header ----
 * Frosted-glass effect: semi-transparent surface + backdrop blur lifts
 * the header off the content area while still letting hero gradients
 * subtly bleed through. Falls back to solid surface where backdrop-filter
 * isn't supported.
 *
 * Note: not sticky — we use an app-shell layout (.shell-root height:
 * 100vh + .content-area overflow-y: auto), so the header is naturally
 * always visible without needing position: sticky. */
.shell-header {
  display: flex;
  align-items: stretch;
  flex: 0 0 auto;
  height: var(--app-header-height);
  background: color-mix(in srgb, var(--app-surface) 88%, transparent);
  -webkit-backdrop-filter: saturate(180%) blur(12px);
  backdrop-filter: saturate(180%) blur(12px);
  border-bottom: 1px solid var(--app-border);
  position: relative;
  z-index: 100;
  /* Subtle drop shadow lifts the header off the content area on scroll. */
  box-shadow: 0 1px 0 0 rgba(15, 23, 42, 0.02), 0 4px 12px -8px rgba(15, 23, 42, 0.08);
}
@supports not (backdrop-filter: blur(1px)) {
  .shell-header { background: var(--app-surface); }
}

/* Brand zone — visually aligned with sidebar width. Rendered as a real
 * <button> for keyboard accessibility. The dark slate background blends
 * with the gradient sidebar; a subtle gradient overlay on hover gives
 * tactile feedback without shifting the title text. */
.header-brand {
  position: relative;
  display: flex;
  align-items: center;
  gap: 10px;
  width: var(--app-sidebar-width-expanded);
  min-width: var(--app-sidebar-width-expanded);
  padding: 0 20px;
  background:
    radial-gradient(80% 100% at 0% 50%, rgba(99, 91, 255, 0.18), transparent 60%),
    var(--app-sidebar-brand-bg);
  cursor: pointer;
  transition: width var(--app-anim-slow) var(--app-easing-standard),
              min-width var(--app-anim-slow) var(--app-easing-standard),
              background var(--app-anim-base) var(--app-easing-standard);
  overflow: hidden;
  border: 0;
  color: inherit;
  text-align: left;
  font: inherit;
}
.header-brand::after {
  content: "";
  position: absolute;
  bottom: 0;
  left: 12px;
  right: 12px;
  height: 1px;
  background: linear-gradient(90deg, rgba(184, 175, 254, 0.35), transparent);
  opacity: 0.6;
  pointer-events: none;
}

.header-brand.collapsed {
  width: var(--app-sidebar-width-collapsed);
  min-width: var(--app-sidebar-width-collapsed);
  padding: 0 14px;
  justify-content: center;
}

.header-brand:hover {
  background: var(--app-sidebar-brand-bg-hover);
}

.header-brand:focus-visible {
  outline: none;
  box-shadow: inset 0 0 0 2px var(--app-sidebar-text-active);
}

.header-brand-logo {
  width: 30px;
  height: 30px;
  min-width: 30px;
  border-radius: var(--app-input-radius);
  border: 1px solid rgba(255, 255, 255, 0.12);
  background: rgba(255, 255, 255, 0.06);
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
}

.header-brand-logo img {
  width: 100%;
  height: 100%;
  object-fit: contain;
}

.header-brand-avatar {
  width: 32px;
  height: 32px;
  min-width: 32px;
  border-radius: 9px;
  background: var(--app-brand-gradient);
  color: #fff;
  font-size: 15px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  box-shadow:
    0 0 0 1px rgba(255, 255, 255, 0.12),
    0 4px 14px rgba(99, 91, 255, 0.45);
}

.header-brand-text {
  font-size: 15px;
  font-weight: 700;
  color: #fff;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  letter-spacing: 0.2px;
  line-height: 1;
}

.brand-fade-enter-active {
  transition: opacity var(--app-anim-base) ease 0.05s;
}
.brand-fade-leave-active {
  transition: opacity var(--app-anim-fast) ease;
}
.brand-fade-enter-from,
.brand-fade-leave-to {
  opacity: 0;
}

.header-divider {
  width: 1px;
  align-self: stretch;
  background: var(--app-border);
}

/* Content zone — fills remaining header width */
.header-content {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 0 20px;
  gap: 12px;
  min-width: 0;
}

.header-nav {
  display: flex;
  align-items: center;
  gap: 4px;
  min-width: 0;
  flex: 1;
}

/* Unified pill icon button used in the header (collapse / theme / bell).
 * Replaces the per-purpose .collapse-btn + .theme-toggle pair so spacing,
 * focus ring, and hover state stay coherent across all three slots. */
.hdr-icon-btn {
  position: relative;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 36px;
  height: 36px;
  padding: 0 8px;
  border-radius: var(--app-input-radius);
  border: 0;
  background: transparent;
  color: var(--app-text-muted);
  cursor: pointer;
  transition: background var(--app-anim-fast) var(--app-easing-standard),
              color var(--app-anim-fast) var(--app-easing-standard);
}
.hdr-icon-btn:hover {
  color: var(--app-text);
  background: var(--app-surface-hover);
}
.hdr-icon-btn:focus-visible {
  outline: none;
  box-shadow: var(--app-focus-ring);
}
/* Notification dot — placed in the upper-right of the bell icon. The dot
 * is a static visual indicator for now (no unread count yet); when the
 * notifications feature lands the dot can be swapped for a numeric badge. */
.bell-dot {
  position: absolute;
  top: 8px;
  right: 8px;
  width: 7px;
  height: 7px;
  border-radius: 50%;
  background: var(--app-danger, #ef4444);
  box-shadow: 0 0 0 2px var(--app-surface, #fff);
}

.collapse-btn {
  min-height: 36px;
  min-width: 36px;
  color: var(--app-text-muted);
  border-radius: var(--app-input-radius);
  flex-shrink: 0;
}

.collapse-btn:hover {
  color: var(--app-text);
  background: var(--app-surface-hover);
}

.header-breadcrumb {
  display: flex;
  align-items: center;
  min-width: 0;
  flex: 1;
  overflow: hidden;
}

.breadcrumb-item {
  font-size: 13px;
  color: var(--app-text-faint);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
  flex-shrink: 1;
}

.breadcrumb-item:last-child {
  flex-shrink: 0;
}

.breadcrumb-sep {
  margin: 0 4px;
  color: var(--app-border-strong);
  flex-shrink: 0;
  vertical-align: middle;
}

.breadcrumb-item.current {
  font-weight: 600;
  color: var(--app-text-strong);
}

.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.theme-toggle {
  min-width: 36px;
  min-height: 36px;
  border-radius: var(--app-input-radius);
  color: var(--app-text-muted);
}

.theme-toggle:hover {
  color: var(--app-text);
  background: var(--app-surface-hover);
}

/* ---- User dropdown ---- */
.user-trigger {
  display: flex;
  align-items: center;
  gap: 8px;
  cursor: pointer;
  padding: 5px 10px 5px 5px;
  border-radius: var(--app-card-radius-sm);
  transition: background var(--app-anim-fast) var(--app-easing-standard);
  background: transparent;
  border: 0;
  color: inherit;
  font: inherit;
}

.user-trigger:hover {
  background: var(--app-surface-hover);
}

.username {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text);
  max-width: 120px;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.avatar {
  background: var(--app-brand-gradient);
  color: #fff;
  font-weight: 600;
  box-shadow:
    0 0 0 2px rgba(255, 255, 255, 0.9),
    0 0 0 3px rgba(99, 91, 255, 0.35),
    0 4px 12px rgba(99, 91, 255, 0.28);
}

.chevron-icon {
  color: var(--app-text-faint);
  transition: color var(--app-anim-fast) var(--app-easing-standard);
}

.user-trigger:hover .chevron-icon {
  color: var(--app-text-muted);
}

/* ---- Drawer brand (mobile) ---- */
.drawer-brand {
  display: flex;
  align-items: center;
  gap: 10px;
  cursor: pointer;
  background: transparent;
  border: 0;
  padding: 0;
  font: inherit;
  color: var(--app-text-strong);
}

/* ---- Sidebar ----
 * Vertical brand gradient (slate-900 → indigo-tinted slate) gives the
 * rail a sense of depth without being distracting. The pseudo ::after
 * lays a faint right-edge glow so the boundary between sidebar and
 * content reads as a soft transition, not a hard line. The aside is a
 * flex column so the menu fills available space and the foot pill pins
 * to the bottom independent of menu height.
 */
.shell-aside {
  position: relative;
  background:
    radial-gradient(120% 60% at 50% -10%, rgba(99, 91, 255, 0.16), transparent 60%),
    linear-gradient(180deg, #1A1A1A 0%, #161616 60%, #121212 100%);
  overflow: hidden;
  transition: width var(--app-anim-slow) var(--app-easing-standard);
  border-right: none;
  display: flex;
  flex-direction: column;
}
.shell-aside::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    radial-gradient(rgba(255, 255, 255, 0.04) 1px, transparent 1px);
  background-size: 14px 14px;
  mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.6) 0%, rgba(0, 0, 0, 0.1) 30%, transparent 80%);
  -webkit-mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.6) 0%, rgba(0, 0, 0, 0.1) 30%, transparent 80%);
}
.shell-aside::after {
  content: "";
  position: absolute;
  top: 0;
  right: 0;
  bottom: 0;
  width: 1px;
  background: linear-gradient(180deg, rgba(184, 175, 254, 0.18) 0%, rgba(184, 175, 254, 0.06) 30%, transparent 70%);
  pointer-events: none;
}

.shell-aside :deep(.t-default-menu) {
  background: transparent;
  border-right: none;
  /* Verified DOM (tdesign-vue-next 1.18.6 menu.mjs):
   *   .t-default-menu (height: 100% inline)
   *     .t-default-menu__inner
   *       ul.t-menu  ← items live here
   *         li.t-menu__item                ← top-level flat item
   *         li.t-submenu
   *           div.t-menu__item             ← submenu TITLE (a div, NOT
   *                                          ".t-menu__submenu-title" —
   *                                          that class doesn't exist
   *                                          in TDesign and was the
   *                                          source of the alignment
   *                                          drift)
   *           ul.t-menu__sub
   *             li.t-menu__item            ← sub items
   *
   * Make the menu chain a flex column so the .aside-foot pill below
   * stays pinned to the bottom even on tall sidebars. */
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}
.shell-aside :deep(.t-default-menu__inner) {
  display: flex;
  flex-direction: column;
  flex: 1;
  min-height: 0;
}

.shell-aside :deep(.t-menu) {
  background: transparent;
  padding: 10px 8px;
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  overflow-x: hidden;
}

/* Top-level rows — both flat items (.t-menu > .t-menu__item) and
 * submenu titles (.t-menu > .t-submenu > .t-menu__item, rendered as a
 * <div>). TDesign drives indent via the `--padding-left` custom
 * property (default 44px); override BOTH the variable and the literal
 * padding so every leaf and title icon lands at the same x. */
.shell-aside :deep(.t-menu > .t-menu__item),
.shell-aside :deep(.t-menu > .t-submenu > .t-menu__item) {
  --padding-left: 16px;
  padding-left: 16px;
  padding-right: 12px;
  border-radius: 10px;
  color: var(--app-sidebar-text);
  margin: 2px 0;
  transition: background var(--app-anim-fast) var(--app-easing-standard),
              color var(--app-anim-fast) var(--app-easing-standard),
              transform var(--app-anim-fast) var(--app-easing-standard);
  position: relative;
}

.shell-aside :deep(.t-menu > .t-menu__item:hover),
.shell-aside :deep(.t-menu > .t-submenu > .t-menu__item:hover) {
  background: var(--app-sidebar-bg-hover);
  color: var(--app-sidebar-text-strong);
  transform: translateX(2px);
}

.shell-aside :deep(.t-menu > .t-menu__item.t-is-active),
.shell-aside :deep(.t-menu > .t-submenu > .t-menu__item.t-is-active) {
  background: linear-gradient(90deg, rgba(99, 91, 255, 0.22), rgba(99, 91, 255, 0.08) 80%, transparent);
  color: var(--app-sidebar-text-active);
  font-weight: 600;
  box-shadow: inset 0 0 0 1px rgba(184, 175, 254, 0.18);
}

/* Left rail accent on the active row — gradient + glow. */
.shell-aside :deep(.t-menu > .t-menu__item.t-is-active)::before,
.shell-aside :deep(.t-menu > .t-submenu > .t-menu__item.t-is-active)::before {
  content: "";
  position: absolute;
  left: 0;
  top: 6px;
  bottom: 6px;
  width: 3px;
  border-radius: 0 4px 4px 0;
  background: linear-gradient(180deg, #B8AFFE, #635BFF);
  box-shadow: 0 0 12px rgba(184, 175, 254, 0.6);
}

/* Submenu open state on the title row */
.shell-aside :deep(.t-submenu.t-is-opened > .t-menu__item) {
  color: var(--app-sidebar-text-strong);
  background: rgba(255, 255, 255, 0.04);
}

.shell-aside :deep(.t-menu__icon) {
  color: inherit;
}

/* Sub-items inside an opened submenu — text column aligns with parent
 * text. 16(parent pad) + ~16(icon) + ~8(gap) ≈ 40px. */
.shell-aside :deep(.t-menu__sub .t-menu__item) {
  --padding-left: 40px;
  padding-left: 40px;
  font-size: 13px;
  border-radius: 10px;
  margin: 2px 0;
  color: var(--app-sidebar-text);
  transition: background var(--app-anim-fast) var(--app-easing-standard),
              color var(--app-anim-fast) var(--app-easing-standard);
}
.shell-aside :deep(.t-menu__sub .t-menu__item:hover) {
  background: var(--app-sidebar-bg-hover);
  color: var(--app-sidebar-text-strong);
}
.shell-aside :deep(.t-menu__sub .t-menu__item.t-is-active) {
  color: var(--app-sidebar-text-active);
  font-weight: 600;
  background: rgba(99, 91, 255, 0.16);
}

/* ─── Collapsed (rail) mode ──────────────────────────────────────────
 * `.t-is-collapsed` lives on the outer .t-default-menu (verified in
 * menu.mjs:39). In rail mode every row — flat or submenu title — is
 * forced to a centered pill so the icon column is dead-straight. */
.shell-aside :deep(.t-is-collapsed) .t-menu__item,
.shell-aside :deep(.t-is-collapsed) .t-submenu > .t-menu__item {
  --padding-left: 0 !important;
  padding-left: 0 !important;
  padding-right: 0 !important;
  display: flex;
  align-items: center;
  justify-content: center;
  margin: 4px 8px;
  width: auto;
  border-radius: 10px;
}
.shell-aside :deep(.t-is-collapsed) .t-menu__item .t-menu__icon,
.shell-aside :deep(.t-is-collapsed) .t-submenu > .t-menu__item .t-menu__icon {
  margin-right: 0;
}
.shell-aside :deep(.t-is-collapsed) .t-menu__item.t-is-active,
.shell-aside :deep(.t-is-collapsed) .t-submenu > .t-menu__item.t-is-active {
  background: linear-gradient(135deg, rgba(184, 175, 254, 0.32), rgba(99, 91, 255, 0.18));
  box-shadow:
    inset 0 0 0 1px rgba(184, 175, 254, 0.45),
    0 4px 14px rgba(99, 91, 255, 0.28);
}
.shell-aside :deep(.t-is-collapsed) .t-menu__item.t-is-active::before,
.shell-aside :deep(.t-is-collapsed) .t-submenu > .t-menu__item.t-is-active::before {
  display: none;
}
.shell-aside :deep(.t-is-collapsed) .t-menu__item:hover,
.shell-aside :deep(.t-is-collapsed) .t-submenu > .t-menu__item:hover {
  transform: none;
  background: var(--app-sidebar-bg-hover);
}

/* Sidebar scrollbar */
.shell-aside :deep(.t-menu)::-webkit-scrollbar {
  width: 4px;
}
.shell-aside :deep(.t-menu)::-webkit-scrollbar-thumb {
  background: rgba(255, 255, 255, 0.12);
  border-radius: 2px;
}
.shell-aside :deep(.t-menu)::-webkit-scrollbar-thumb:hover {
  background: rgba(255, 255, 255, 0.2);
}

/* ---- Content area ----
 * THIS is the only scroll context in the page (alongside the sidebar's
 * own .t-menu scroll). min-height: 0 lets the inner flex parents
 * collapse so overflow-y: auto can actually engage; without it the
 * content would push body scroll instead and the sidebar would scroll
 * out of view on long pages.
 *
 * Subtle radial brand-tinted background gives depth to the page without
 * adding a hard color. The mask fades the gradient out by mid-page so
 * actual content cards keep their crisp white surface. */
.content-area {
  flex: 1;
  min-height: 0;
  padding: 24px 28px 32px;
  background:
    radial-gradient(60% 50% at 100% 0%, rgba(99, 91, 255, 0.06), transparent 60%),
    radial-gradient(50% 40% at 0% 100%, rgba(255, 105, 180, 0.04), transparent 60%),
    var(--app-bg-page);
  overflow-y: auto;
  position: relative;
}
.content-area::before {
  content: "";
  position: absolute;
  inset: 0;
  pointer-events: none;
  background-image:
    radial-gradient(rgba(15, 23, 42, 0.04) 1px, transparent 1px);
  background-size: 22px 22px;
  mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.5), transparent 30%);
  -webkit-mask-image: linear-gradient(to bottom, rgba(0, 0, 0, 0.5), transparent 30%);
}

.content-inner {
  position: relative;
  width: 100%;
  /* Was max-width: min(1680px, 100%) — capped content at 1680px on
   * wide monitors and centered with auto margins. The user wants the
   * dashboard to use the full screen, so the cap is removed and the
   * content area extends edge-to-edge of the .content-area padding. */
  max-width: none;
  margin: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* t-content (rendered as <main class="t-layout__content">) needs to be
 * a flex column so its child .content-inner can claim flex: 1 height.
 * Without this, the inner chain has no definite height to grow into. */
.content-area :deep(.t-layout__content) {
  display: flex;
  flex-direction: column;
  min-height: 0;
}

/* Route transition — short, low-amplitude fade so the user feels motion
 * without waiting for it. Disable in reduced-motion mode (handled via the
 * global `prefers-reduced-motion` rule in globals.css). */
.route-fade-enter-active,
.route-fade-leave-active {
  transition: opacity 0.16s var(--app-easing-standard),
              transform 0.16s var(--app-easing-standard);
}

.route-fade-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.route-fade-leave-to {
  opacity: 0;
  transform: translateY(-4px);
}

/* ---- Footer ---- */
/* 隐形 footer：随内容滚动，无边线、无底色，与页面背景融为一体。
 * 仅当用户滚到内容最底部时才看到一行小字版权信息。 */
.shell-footer {
  margin-top: 16px;
  padding: 8px 0 0;
  text-align: center;
  font-size: 12px;
  color: var(--app-text-faint);
  line-height: 1.4;
  background: transparent;
}

/* TDesign 的 .t-layout__footer 默认带 padding: 24px 与占位文字色，
 * 用 :deep() 强制覆盖为我们想要的隐形样式。 */
:deep(.t-layout__footer.shell-footer) {
  padding: 8px 0 0;
  background: transparent;
  color: var(--app-text-faint);
}

.footer-inner {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: center;
  gap: 8px 12px;
}

.footer-links {
  display: inline-flex;
  flex-wrap: wrap;
  gap: 8px 12px;
}

.footer-links a {
  color: var(--app-text-faint);
  text-decoration: none;
  transition: color var(--app-anim-fast) var(--app-easing-standard);
}

.footer-links a:hover {
  color: var(--app-brand-strong);
}

/* ---- Mobile responsive ---- */
@media (max-width: 768px) {
  .header-brand {
    width: auto;
    min-width: auto;
    padding: 0 12px;
    gap: 8px;
  }

  .header-divider {
    display: none;
  }

  .header-content {
    padding: 0 12px;
    gap: 8px;
  }

  .collapse-btn {
    min-height: 44px;
  }

  .user-trigger {
    min-height: 44px;
    padding: 4px 6px 4px 4px;
  }

  .username {
    display: none;
  }

  .chevron-icon {
    display: none;
  }

  .content-area {
    padding: 16px;
  }

  .shell-footer,
  :deep(.t-layout__footer.shell-footer) {
    padding: 8px 0 0;
  }
}

@media (max-width: 480px) {
  .header-brand-text {
    display: none;
  }
}
</style>
