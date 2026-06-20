import { readFileSync, writeFileSync, readdirSync, statSync } from "node:fs"
import { join, extname } from "node:path"
import { fileURLToPath } from "node:url"

const root = join(fileURLToPath(new URL("..", import.meta.url)), "src")
let changed = 0

function walk(dir) {
  for (const name of readdirSync(dir)) {
    const path = join(dir, name)
    if (statSync(path).isDirectory()) {
      walk(path)
      continue
    }
    if (extname(path) !== ".vue") continue
    const src = readFileSync(path, "utf8")
    const next = src
      .replace(/<t-dialog(?!\s+attach=)/g, '<t-dialog attach="body"')
      .replace(/<t-drawer(?!\s+attach=)/g, '<t-drawer attach="body"')
    if (next !== src) {
      writeFileSync(path, next, "utf8")
      changed++
    }
  }
}

walk(root)
console.log(`Updated ${changed} files`)
