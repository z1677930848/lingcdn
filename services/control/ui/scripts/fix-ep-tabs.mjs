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

function fix(content) {
  let s = content
  // Element Plus tab-pane uses `name`, not TDesign's `value`
  s = s.replace(/(<el-tab-pane\b[^>]*?)\bvalue=/g, "$1name=")
  s = s.replace(/(<el-tab-pane\b[^>]*?):value=/g, "$1:name=")
  // Element Plus collapse-item uses `name`, not `value`
  s = s.replace(/(<el-collapse-item\b[^>]*?)\bvalue=/g, "$1name=")
  s = s.replace(/(<el-collapse-item\b[^>]*?):value=/g, "$1:name=")
  // Element Plus tabs tab-change event
  s = s.replace(/(<el-tabs\b[^>]*?)@change=/g, "$1@tab-change=")
  s = s.replace(/variant="light"/g, 'effect="light"')
  s = s.replace(/variant: "light"/g, 'effect: "light"')
  return s
}

let n = 0
for (const file of walk(root)) {
  const before = fs.readFileSync(file, "utf8")
  const after = fix(before)
  if (after !== before) {
    fs.writeFileSync(file, after, "utf8")
    n++
    console.log("fixed:", path.relative(root, file))
  }
}
console.log(`Fixed ${n} vue files.`)
