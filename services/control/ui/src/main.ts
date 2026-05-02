import { createApp } from "vue"
import { createPinia } from "pinia"
import TDesign from "tdesign-vue-next"
import { MotionPlugin } from "@vueuse/motion"
import "tdesign-vue-next/es/style/index.css"
import "./styles/tokens.css"
import "./styles/tdesign-theme.css"
import "./styles/globals.css"
import "./styles/shared.css"
import "./styles/admin-shared.css"
import "./styles/auth.css"
import App from "./App.vue"
import router from "./router"

const app = createApp(App)

app.use(createPinia())
app.use(router)
app.use(TDesign)
// MotionPlugin enables the `v-motion` directive used by the dashboard
// hero/cards for entrance animations. It respects prefers-reduced-motion
// out of the box, so users with motion preferences disabled see static
// layouts. Lightweight (~6KB) and tree-shakeable.
app.use(MotionPlugin)

app.mount("#app")

const boot = document.getElementById("app-boot")
if (boot) {
  boot.style.transition = "opacity 240ms ease"
  boot.style.opacity = "0"
  setTimeout(() => boot.remove(), 280)
}

