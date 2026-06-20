#!/usr/bin/env node
import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "src")

function walk(dir) {
  const out = []
  for (const name of fs.readdirSync(dir)) {
    const full = path.join(dir, name)
    const stat = fs.statSync(full)
    if (stat.isDirectory()) out.push(...walk(full))
    else if (/\.(vue|ts)$/.test(name)) out.push(full)
  }
  return out
}

const TAG_MAP = [
  ["t-tab-panel", "el-tab-pane"],
  ["t-input-number", "el-input-number"],
  ["t-date-range-picker", "el-date-picker"],
  ["t-descriptions-item", "el-descriptions-item"],
  ["t-collapse-panel", "el-collapse-item"],
  ["t-radio-button", "el-radio-button"],
  ["t-radio-group", "el-radio-group"],
  ["t-checkbox-group", "el-checkbox-group"],
  ["t-dropdown-menu", "el-dropdown-menu"],
  ["t-dropdown-item", "el-dropdown-item"],
  ["t-menu-item", "el-menu-item"],
  ["t-submenu", "el-sub-menu"],
  ["t-popconfirm", "el-popconfirm"],
  ["t-pagination", "el-pagination"],
  ["t-descriptions", "el-descriptions"],
  ["t-form-item", "el-form-item"],
  ["t-textarea", "el-input"],
  ["t-button", "el-button"],
  ["t-table", "EpDataTable"],
  ["t-dialog", "EpDialog"],
  ["t-drawer", "el-drawer"],
  ["t-option", "el-option"],
  ["t-collapse", "el-collapse"],
  ["t-checkbox", "el-checkbox"],
  ["t-dropdown", "el-dropdown"],
  ["t-skeleton", "el-skeleton"],
  ["t-menu", "el-menu"],
  ["t-input", "el-input"],
  ["t-radio", "el-radio"],
  ["t-select", "el-select"],
  ["t-switch", "el-switch"],
  ["t-layout", "el-container"],
  ["t-header", "el-header"],
  ["t-content", "el-main"],
  ["t-aside", "el-aside"],
  ["t-avatar", "el-avatar"],
  ["t-divider", "el-divider"],
  ["t-form", "el-form"],
  ["t-alert", "el-alert"],
  ["t-badge", "el-badge"],
  ["t-card", "el-card"],
  ["t-tabs", "el-tabs"],
  ["t-link", "el-link"],
  ["t-space", "el-space"],
  ["t-empty", "el-empty"],
  ["t-icon", "EpIcon"],
  ["t-tag", "el-tag"],
]

function fixImports(s) {
  s = s.replace(/import \{ MessagePlugin \} from "tdesign-vue-next"/g, 'import { MessagePlugin } from "@/lib/ep-message"')
  s = s.replace(
    /import \{ DialogPlugin([^}]*)\} from "tdesign-vue-next"/g,
    (_, rest) => {
      const ep = []
      if (rest.includes("Button")) ep.push("ElButton")
      if (rest.includes("Tag")) ep.push("ElTag")
      if (rest.includes("Switch")) ep.push("ElSwitch")
      if (rest.includes("Link")) ep.push("ElLink")
      if (rest.includes("Dropdown")) ep.push("ElDropdown", "ElDropdownMenu", "ElDropdownItem")
      const lines = ['import { DialogPlugin } from "@/lib/ep-dialog"']
      if (ep.length) lines.push(`import { ${[...new Set(ep)].join(", ")} } from "element-plus"`)
      return lines.join("\n")
    }
  )
  s = s.replace(
    /import \{([^}]+)\} from "tdesign-vue-next"/g,
    (_, body) => {
      const parts = body.split(",").map((x) => x.trim()).filter(Boolean)
      const lines = []
      const ep = []
      for (const part of parts) {
        if (part === "MessagePlugin") lines.push('import { MessagePlugin } from "@/lib/ep-message"')
        else if (part === "DialogPlugin") lines.push('import { DialogPlugin } from "@/lib/ep-dialog"')
        else if (part === "Button") ep.push("ElButton")
        else if (part === "Tag") ep.push("ElTag")
        else if (part === "Switch") ep.push("ElSwitch")
        else if (part === "Link") ep.push("ElLink")
        else if (part === "Dropdown") ep.push("ElDropdown")
        else if (part === "DropdownMenu") ep.push("ElDropdownMenu")
        else if (part === "DropdownItem") ep.push("ElDropdownItem")
      }
      if (ep.length) lines.push(`import { ${[...new Set(ep)].join(", ")} } from "element-plus"`)
      return lines.join("\n")
    }
  )
  return s
}

function addEpComponentImports(s) {
  if (!s.includes("<script")) return s
  const imports = []
  if (s.includes("EpDataTable") && !s.includes("@/components/ep/EpDataTable.vue")) {
    imports.push('import EpDataTable from "@/components/ep/EpDataTable.vue"')
  }
  if (s.includes("EpDialog") && !s.includes("@/components/ep/EpDialog.vue")) {
    imports.push('import EpDialog from "@/components/ep/EpDialog.vue"')
  }
  if (s.includes("EpIcon") && !s.includes("@/components/ep/EpIcon.vue")) {
    imports.push('import EpIcon from "@/components/ep/EpIcon.vue"')
  }
  if (!imports.length) return s
  return s.replace(/<script setup lang="ts">\n/, `<script setup lang="ts">\n${imports.join("\n")}\n`)
}

function migrateContent(content, file) {
  if (file.endsWith(`${path.sep}main.ts`)) {
    return `import { createApp } from "vue"
import { createPinia } from "pinia"
import ElementPlus from "element-plus"
import zhCn from "element-plus/es/locale/lang/zh-cn"
import { MotionPlugin } from "@vueuse/motion"
import "element-plus/dist/index.css"
import "./styles/tokens.css"
import "./styles/element-plus-theme.css"
import "./styles/globals.css"
import "./styles/shared.css"
import "./styles/layout.css"
import "./styles/components.css"
import "./styles/settings.css"
import "./styles/admin-views.css"
import "./styles/domain-detail.css"
import "./styles/admin-shared.css"
import "./styles/overview.css"
import "./styles/dashboard.css"
import "./styles/pages.css"
import "./styles/auth.css"
import App from "./App.vue"
import router from "./router"

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(ElementPlus, { locale: zhCn })
app.use(MotionPlugin)

app.mount("#app")

const boot = document.getElementById("app-boot")
if (boot) {
  boot.style.transition = "opacity 240ms ease"
  boot.style.opacity = "0"
  setTimeout(() => boot.remove(), 280)
}
`
  }

  if (file.endsWith(`${path.sep}notify.ts`)) {
    return `import { ElMessage } from "element-plus"

export type NotifyVariant = "default" | "success" | "warning" | "destructive" | "info"

export interface NotifyOptions {
  variant?: NotifyVariant
  title: string
  description?: string
}

export function notify({ variant = "default", title, description }: NotifyOptions) {
  const content = description ? \`\${title}：\${description}\` : title
  if (variant === "success") return ElMessage.success(content)
  if (variant === "warning") return ElMessage.warning(content)
  if (variant === "destructive") return ElMessage.error(content)
  return ElMessage.info(content)
}
`
  }

  if (file.endsWith(`${path.sep}App.vue`)) {
    return `<template>
  <el-config-provider>
    <div class="app-root">
      <div v-if="routeLoading" class="route-progress" aria-hidden="true">
        <div class="route-progress-bar" />
      </div>
      <RouterView v-slot="{ Component, route }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.fullPath" />
        </transition>
      </RouterView>
    </div>
  </el-config-provider>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useAuthStore } from "@/stores/auth"
import { routeLoading } from "@/lib/routeLoading"

const auth = useAuthStore()

onMounted(() => {
  auth.checkAuth()
})
</script>
`
  }

  let s = content
  s = fixImports(s)

  for (const [from, to] of TAG_MAP) {
    s = s.replace(new RegExp(`<${from}(?=\\s|>|/)`, "g"), `<${to}`)
    s = s.replace(new RegExp(`</${from}>`, "g"), `</${to}>`)
  }

  s = s
    .replace(/\btheme="primary"/g, 'type="primary"')
    .replace(/\btheme="danger"/g, 'type="danger"')
    .replace(/\btheme="success"/g, 'type="success"')
    .replace(/\btheme="warning"/g, 'type="warning"')
    .replace(/\btheme="default"/g, 'type="info"')
    .replace(/\bvariant="outline"/g, "plain")
    .replace(/\bvariant="text"/g, "link")
    .replace(/v-model:visible/g, "v-model")
    .replace(/:header=/g, ":title=")
    .replace(/\battach="body"/g, "append-to-body")
    .replace(/layout="vertical"/g, 'label-position="top"')
    .replace(/label-align="top"/g, 'label-position="top"')
    .replace(/\bempty=/g, "empty-text=")
    .replace(/#prefixIcon/g, "#prefix")
    .replace(/#suffixIcon/g, "#suffix")

  s = s
    .replace(/\bh\(Button,/g, "h(ElButton,")
    .replace(/\bh\(Tag,/g, "h(ElTag,")
    .replace(/\bh\(Switch,/g, "h(ElSwitch,")
    .replace(/\bh\(Link,/g, "h(ElLink,")
    .replace(/\bh\(Dropdown,/g, "h(ElDropdown,")
    .replace(/\bh\(DropdownMenu,/g, "h(ElDropdownMenu,")
    .replace(/\bh\(DropdownItem,/g, "h(ElDropdownItem,")

  s = addEpComponentImports(s)
  return s
}

let changed = 0
for (const file of walk(root)) {
  if (file.includes(`${path.sep}components${path.sep}ep${path.sep}`)) continue
  if (file.includes(`${path.sep}lib${path.sep}ep-`)) continue
  if (file.includes("TemplateShell.vue")) continue
  const before = fs.readFileSync(file, "utf8")
  const after = migrateContent(before, file)
  if (after !== before) {
    fs.writeFileSync(file, after, "utf8")
    changed++
    console.log("updated", path.relative(root, file))
  }
}

console.log(`Done. ${changed} files updated.`)
