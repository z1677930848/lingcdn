import { createApp } from "vue"
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
