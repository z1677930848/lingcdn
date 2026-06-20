/**
 * Full EP admin style migration:
 * 1. Replace hardcoded slate/legacy colors with design tokens
 * 2. Normalize font-weight 700/800 -> 600 in <style> blocks
 * 3. Strip scoped rules that duplicate shared.css utilities
 */
import fs from "node:fs"
import path from "node:path"
import { fileURLToPath } from "node:url"

const ROOT = new URL("../src", import.meta.url)

const COLOR_MAP = [
  ["#0f172a", "var(--app-text-strong)"],
  ["#1e293b", "var(--app-text-strong)"],
  ["#334155", "var(--app-text)"],
  ["#475569", "var(--app-text-muted)"],
  ["#64748b", "var(--app-text-muted)"],
  ["#94a3b8", "var(--app-text-faint)"],
  ["#a8abb2", "var(--app-text-faint)"],
  ["#cbd5e1", "var(--app-border-strong)"],
  ["#e2e8f0", "var(--app-border)"],
  ["#f1f5f9", "var(--app-divider-soft)"],
  ["#f8fafc", "var(--app-surface-muted)"],
  ["#eff6ff", "var(--app-brand-soft-bg)"],
  ["#dbeafe", "var(--app-brand-soft-border)"],
  ["#2563eb", "var(--td-brand-color)"],
  ["#1d4ed8", "var(--td-brand-color-7)"],
  ["#00a870", "var(--app-success)"],
  ["#1f9d5b", "var(--app-success-strong)"],
  ["#ef4444", "var(--app-danger)"],
  ["#f56c6c", "var(--app-danger)"],
  ["#b45309", "var(--app-warning-strong)"],
  ["#fff7e6", "var(--app-warning-soft-bg)"],
  ["#ffe4b5", "var(--app-warning-soft-border)"],
  ["#92400e", "var(--app-warning-soft-fg)"],
  ["#6366f1", "var(--td-brand-color)"],
]

const INLINE_STYLE_COLOR_MAP = [
  ["color:#94a3b8", "color:var(--app-text-faint)"],
  ["color: #94a3b8", "color: var(--app-text-faint)"],
  ["color:#00a870", "color:var(--app-success)"],
  ["color: #00a870", "color: var(--app-success)"],
  ["color:#ef4444", "color:var(--app-danger)"],
  ["color: #ef4444", "color: var(--app-danger)"],
]

/** Single-class selectors fully defined in shared.css — safe to drop from scoped. */
const DUP_SELECTORS = new Set([
  "muted",
  "mono",
  "link",
  "loading",
  "loading-row",
  "page-loading",
  "empty",
  "filters",
  "filter-item",
  "time-buttons",
  "stats",
  "stat-card",
  "stat-label",
  "stat-value",
  "series-grid",
  "series-card",
  "series-head",
  "series-title",
  "series-value",
  "tab-alert",
  "form-tip",
  "message",
  "detail",
  "detail-grid",
  "detail-label",
  "detail-code",
  "detail-pre",
  "detail-hint",
  "balance-card",
  "label",
  "amount",
  "pay-box",
  "pay-link",
  "inline-grid",
  "form-field",
  "form-alert",
  "feature-tip",
  "monitor-alert",
  "product-grid",
  "product-head",
  "product-name",
  "product-desc",
  "product-status",
  "product-meta",
  "product-actions",
  "meta-row",
  "meta-label",
  "meta-value",
  "renewal-hint",
  "detail-content",
  "detail-section",
  "detail-section-title",
  "detail-row",
  "detail-features",
  "empty-state",
  "loading-wrap",
  "spin",
  "page",
  "section-card",
  "section-body",
  "header-actions",
  "toolbar",
  "batch-bar",
  "batch-bar-label",
  "w-260",
  "w-140",
  "dialog-grid",
  "dialog-title",
  "dialog-label",
  "dialog-tip",
  "grid-2",
  "row",
  "row-label",
  "copy-row",
  "token-box",
  "ssh-result-card",
  "ssh-result-head",
  "ssh-result-title",
  "ssh-result-meta",
  "ssh-node-meta",
  "ssh-log-box",
  "bundle-upload-row",
  "bundle-file-input",
  "bundle-list",
  "bundle-item",
  "bundle-meta",
  "detail-body",
  "detail-item",
  "detail-item-full",
  "detail-value",
  "detail-error",
  "ring-row",
  "ring-card",
  "ring-svg",
  "ring-label",
  "ring-value",
  "ring-title",
  "ring-meta",
  "drawer-header-with-back",
  "drawer-back-btn",
  "log-box",
  "log-line",
  "log-empty",
  "log-actions",
  "l4-notice",
  "info-banner",
  "notice-text",
  "group-item",
  "dot",
  "name",
  "actions-row",
  "actions-left",
  "actions-right",
  "form-row-v",
  "form-label-v",
  "form-grid-vertical",
  "error",
  "search-input",
  "filter-select",
  "filter-select-wide",
  "header-actions-extra",
  "header-actions-more",
  "meta-error-banner",
  "sync-pending",
  "sync-failed",
  "sync-dot",
  "sync-dot-running",
  "sync-dot-failed",
  "stack",
  "title-row",
  "row-actions",
  "form-inline",
  "form-switches",
  "switch-item",
  "amount-stack",
  "actions",
  "value",
  "upgrade-header",
  "upgrade-version",
  "arrow",
  "latest",
  "notes",
  "panel-title",
  "task-list",
  "task-item",
  "task-title",
  "task-actions",
  "status",
  "log-header",
  "log-info",
])

function walk(dir, files = []) {
  for (const name of fs.readdirSync(dir)) {
    const p = path.join(dir, name)
    const st = fs.statSync(p)
    if (st.isDirectory()) {
      if (name === "node_modules") continue
      walk(p, files)
    } else if (name.endsWith(".vue")) {
      files.push(p)
    }
  }
  return files
}

function replaceColors(text) {
  let out = text
  for (const [from, to] of COLOR_MAP) {
    const re = new RegExp(from.replace("#", "#"), "gi")
    out = out.replace(re, to)
  }
  for (const [from, to] of INLINE_STYLE_COLOR_MAP) {
    out = out.split(from).join(to)
  }
  return out
}

function normalizeStyleBlock(css) {
  let out = css
  out = out.replace(/font-weight:\s*800/g, "font-weight: 600")
  out = out.replace(/font-weight:\s*700/g, "font-weight: 600")
  out = out.replace(/border-radius:\s*12px/g, "border-radius: var(--app-card-radius)")
  out = out.replace(/border-radius:\s*10px/g, "border-radius: var(--app-card-radius-sm)")
  out = out.replace(/border-radius:\s*8px/g, "border-radius: var(--app-card-radius-sm)")
  return out
}

function stripDuplicateRules(css) {
  // Split on closing braces at rule level (simple parser for flat rules)
  const rules = []
  let depth = 0
  let start = 0
  for (let i = 0; i < css.length; i++) {
    if (css[i] === "{") depth++
    if (css[i] === "}") {
      depth--
      if (depth === 0) {
        rules.push(css.slice(start, i + 1).trim())
        start = i + 1
      }
    }
  }
  const kept = []
  for (const rule of rules) {
    const m = rule.match(/^([^{]+)\{/)
    if (!m) {
      kept.push(rule)
      continue
    }
    const selector = m[1].trim()
    // Keep :deep, media, keyframes, compound selectors
    if (
      selector.includes("@") ||
      selector.includes(":deep") ||
      selector.includes(",") ||
      selector.includes(" ") ||
      selector.includes(">") ||
      selector.includes("+") ||
      selector.includes(":")
    ) {
      kept.push(rule)
      continue
    }
    const cls = selector.replace(/^\./, "").trim()
    if (DUP_SELECTORS.has(cls)) {
      continue
    }
    kept.push(rule)
  }
  return kept.join("\n\n")
}

function processFile(filePath) {
  let content = fs.readFileSync(filePath, "utf8")
  const styleRe = /<style([^>]*)scoped([^>]*)>([\s\S]*?)<\/style>/g
  let changed = false

  content = replaceColors(content)

  content = content.replace(styleRe, (full, a, b, css) => {
    let newCss = normalizeStyleBlock(css)
    newCss = stripDuplicateRules(newCss)
    newCss = newCss.replace(/\n{3,}/g, "\n\n").trim()
    if (newCss !== css.trim()) changed = true
    if (!newCss) {
      changed = true
      return ""
    }
    return `<style${a}scoped${b}>\n${newCss}\n</style>`
  })

  // Remove empty scoped style tags
  content = content.replace(/<style[^>]*scoped[^>]*>\s*<\/style>\n?/g, () => {
    changed = true
    return ""
  })

  if (changed || content !== fs.readFileSync(filePath, "utf8")) {
    fs.writeFileSync(filePath, content)
    return true
  }
  return false
}

const files = walk(fileURLToPath(ROOT))
let count = 0
for (const f of files) {
  const before = fs.readFileSync(f, "utf8")
  processFile(f)
  const after = fs.readFileSync(f, "utf8")
  if (before !== after) {
    count++
    console.log("updated:", path.relative(fileURLToPath(ROOT), f))
  }
}
console.log(`Done. ${count} files updated.`)
