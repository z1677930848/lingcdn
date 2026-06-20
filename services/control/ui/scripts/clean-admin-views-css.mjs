import fs from "node:fs"

const path = "src/styles/admin-views.css"
let css = fs.readFileSync(path, "utf8")

const dupBlock =
  /\/\* --- src\/views\/(AdminWAFPoliciesView|AdminCacheRulesView|DashboardRulesView)\.vue --- \*\/[\s\S]*?(?=\/\* --- src\/views\/|\n\/\* --- [A-Z]|$)/g
css = css.replace(dupBlock, "")

const flexFormGrid =
  /\n\.form-grid \{\s*display: flex;\s*flex-direction: column;\s*gap: 16px;\s*\}\n/g
css = css.replace(flexFormGrid, "\n")

const formInputV = /\n\.form-input-v \{\s*width: 100%;\s*\}\n/g
css = css.replace(formInputV, "\n")

const formHintGrid = /\n\.form-hint-v \{\s*grid-column: 2;[\s\S]*?\}\n/g
css = css.replace(formHintGrid, "\n")

fs.writeFileSync(path, css)
console.log("cleaned", path, css.length)
