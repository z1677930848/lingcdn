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
    else if (/\.vue$/.test(name)) out.push(full)
  }
  return out
}

function ensureImport(content, needle, importLine) {
  if (!content.includes(needle) || content.includes(importLine.trim())) return content
  return content.replace(/<script setup lang="ts">\r?\n/, (m) => `${m}${importLine}\n`)
}

function fix(content) {
  let s = content
  s = s.replace(/label-position="top"\s+label-position="top"/g, 'label-position="top"')
  s = s.replace(/<t-loading\s*\/>/g, '<div v-loading="true" style="min-height:48px" />')
  s = s.replace(/\s:hover(\s|>|\/)/g, "$1")
  s = s.replace(/\s:stripe(\s|>|\/)/g, "$1")
  s = s.replace(/<el-tag([^>]*):theme=/g, "<el-tag$1:type=")
  s = s.replace(/<el-button([^>]*):theme=/g, "<el-button$1:type=")
  s = s.replace(/<el-button([^>]*):variant="outline"/g, "<el-button$1plain")
  s = s.replace(/<el-button([^>]*):variant="text"/g, "<el-button$1link")
  s = s.replace(/<el-button([^>]*):variant="base"/g, "<el-button$1")
  s = s.replace(/type="primary"\s+type="submit"/g, 'type="primary" native-type="submit"')
  s = s.replace(/type="submit"\s+type="primary"/g, 'type="primary" native-type="submit"')
  s = s.replace(/size="medium"/g, 'size="default"')
  s = s.replace(/ElTag,\s*\{\s*([^}]*?)theme:\s*"default"/g, 'ElTag, { $1type: "info"')
  s = s.replace(/ElTag,\s*\{\s*([^}]*?)theme:/g, "ElTag, { $1type:")
  s = s.replace(/ElButton,\s*\{\s*([^}]*?)theme:\s*"primary"/g, 'ElButton, { $1type: "primary"')
  s = s.replace(/ElButton,\s*\{\s*([^}]*?)theme:\s*"danger"/g, 'ElButton, { $1type: "danger"')
  s = s.replace(/ElButton,\s*\{\s*([^}]*?)theme:\s*"default"/g, "ElButton, { $1")
  s = s.replace(/ElButton,\s*\{\s*([^}]*?)variant:\s*"text"/g, "ElButton, { $1link: true")
  s = s.replace(/ElButton,\s*\{\s*([^}]*?)variant:\s*"outline"/g, "ElButton, { $1plain: true")
  s = s.replace(/\bh\(\s*Button\b/g, "h(ElButton")
  s = s.replace(/\bh\(\s*Link\b/g, "h(ElLink")
  s = s.replace(/\bh\(\s*Tag\b/g, "h(ElTag")
  s = s.replace(/\bh\(\s*DropdownItem\b/g, "h(ElDropdownItem")
  s = s.replace(/\bh\(\s*Dropdown\b/g, "h(ElDropdown")
  s = s.replace(/<el-select([^>]*):options=/g, "<EpSelect$1:options=")
  s = s.replace(/<el-dropdown([^>]*):options=/g, "<EpDropdown$1:options=")
  s = s.replace(/type="default"/g, 'type="info"')
  s = s.replace(/type='default'/g, "type='info'")
  s = s.replace(/TagTheme = "success" \| "warning" \| "danger" \| "default" \| "primary"/g, 'TagTheme = "success" | "warning" | "danger" | "info" | "primary"')
  s = s.replace(/pending: "default"/g, 'pending: "info"')
  s = s.replace(/\|\| "default"/g, '|| "info"')
  s = s.replace(/theme: "default"/g, 'theme: "info"')
  s = s.replace(/\? 'success' : 'default'/g, "? 'success' : 'info'")
  s = s.replace(/\? "success" : "default"/g, '? "success" : "info"')
  s = s.replace(/: 'default'/g, ": 'info'")
  s = s.replace(/: "default"/g, ': "info"')
  s = s.replace(/type: "default"/g, 'type: "info"')
  s = s.replace(/type: 'default'/g, "type: 'info'")
  s = s.replace(/return "default"/g, 'return "info"')
  s = s.replace(/:type="inline \? 'default'/g, ":type=\"inline ? 'info'")
  s = s.replace(/"default" \| "success"/g, '"info" | "success"')
  s = s.replace(/"default" as const/g, '"info" as const')
  s = s.replace(/@click="openGroupDialog"/g, '@click="() => openGroupDialog()"')

  if (s.includes("<EpSelect") && !s.includes('from "@/components/ep/EpSelect.vue"')) {
    s = ensureImport(s, "<EpSelect", 'import EpSelect from "@/components/ep/EpSelect.vue"')
  }
  if (s.includes("<EpDropdown") && !s.includes('from "@/components/ep/EpDropdown.vue"')) {
    s = ensureImport(s, "<EpDropdown", 'import EpDropdown from "@/components/ep/EpDropdown.vue"')
  }
  if (/\bMessagePlugin\b/.test(s) && !s.includes('from "@/lib/ep-message"')) {
    s = ensureImport(s, "MessagePlugin", 'import { MessagePlugin } from "@/lib/ep-message"')
  }
  if (/\bDialogPlugin\b/.test(s) && !s.includes('from "@/lib/ep-dialog"')) {
    s = ensureImport(s, "DialogPlugin", 'import { DialogPlugin } from "@/lib/ep-dialog"')
  }

  const elementPlusImports = []
  if (/\bh\(ElButton\b/.test(s)) elementPlusImports.push("ElButton")
  if (/\bh\(ElLink\b/.test(s)) elementPlusImports.push("ElLink")
  if (/\bh\(ElTag\b/.test(s)) elementPlusImports.push("ElTag")
  if (/\bh\(ElDropdown\b/.test(s)) elementPlusImports.push("ElDropdown")
  if (/\bh\(ElDropdownItem\b/.test(s)) elementPlusImports.push("ElDropdownItem")
  if (/\bh\(ElDropdownMenu\b/.test(s)) elementPlusImports.push("ElDropdownMenu")
  if (elementPlusImports.length && !s.includes('from "element-plus"')) {
    s = ensureImport(s, elementPlusImports[0], `import { ${elementPlusImports.join(", ")} } from "element-plus"`)
  } else if (elementPlusImports.length) {
    s = s.replace(/import\s*\{([^}]*)\}\s*from\s*"element-plus"/, (m, inner) => {
      const items = inner.split(",").map((x) => x.trim()).filter(Boolean)
      for (const item of elementPlusImports) {
        if (!items.includes(item)) items.push(item)
      }
      return `import { ${items.join(", ")} } from "element-plus"`
    })
  }

  return s
}

let n = 0
for (const file of walk(root)) {
  const before = fs.readFileSync(file, "utf8")
  const after = fix(before)
  if (after !== before) {
    fs.writeFileSync(file, after, "utf8")
    n++
  }
}
console.log(`Fixed ${n} vue files.`)
