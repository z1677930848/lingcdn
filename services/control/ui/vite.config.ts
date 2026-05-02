import { defineConfig } from "vite"
import vue from "@vitejs/plugin-vue"
import path from "node:path"

export default defineConfig({
  plugins: [vue()],
  resolve: {
    alias: {
      "@": path.resolve(__dirname, "src"),
    },
  },
  server: {
    port: 5174,
    proxy: {
      "/api": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
      "/healthz": {
        target: "http://127.0.0.1:8080",
        changeOrigin: true,
      },
    },
  },
  preview: {
    port: 5174,
  },
  build: {
    chunkSizeWarningLimit: 800,
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (!id.includes("node_modules")) return
          if (id.includes("echarts") || id.includes("zrender")) return "echarts"
          if (id.includes("@highcharts/map-collection")) return "map-data"
          if (id.includes("tdesign-vue-next") || id.includes("tdesign-icons-vue-next")) return "tdesign"
          if (
            id.includes("vue-router") ||
            id.includes("/pinia/") ||
            id.includes("@vue/runtime") ||
            id.includes("@vue/shared") ||
            id.includes("@vue/reactivity") ||
            id.includes("@vue/compiler") ||
            id.includes("/vue/")
          ) {
            return "vue-core"
          }
          return "vendor"
        },
      },
    },
  },
})
