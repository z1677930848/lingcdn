<template>
  <div class="not-found-page">
    <div class="not-found-container">
      <div class="not-found-code" aria-hidden="true">404</div>
      <h1 class="not-found-title">页面未找到</h1>
      <p class="not-found-description">抱歉，您访问的页面不存在或已被移除</p>
      <el-button type="primary" size="large" @click="goHome">
        返回首页
      </el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"

const router = useRouter()
const auth = useAuthStore()

function goHome() {
  if (auth.user?.role === "admin") {
    router.push("/admin/dashboard")
    return
  }
  if (auth.user) {
    router.push("/dashboard")
    return
  }
  router.push("/login")
}
</script>
