import fs from "node:fs"

const files = process.argv.slice(2)
for (const f of files) {
  let t = fs.readFileSync(f, "utf8")
  const next = t.replace(/\r?\n<style scoped>[\s\S]*?<\/style>\r?\n?/g, "\n")
  if (next !== t) {
    fs.writeFileSync(f, next)
    console.log("stripped", f)
  } else {
    console.log("skip", f)
  }
}
