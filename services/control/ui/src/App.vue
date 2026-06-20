<template>
  <el-config-provider>
    <div class="app-root">
      <div v-if="routeLoading" class="route-progress" aria-hidden="true">
        <div class="route-progress-bar" />
      </div>
      <RouterView v-slot="{ Component, route }">
        <transition name="page" mode="out-in">
          <component :is="Component" :key="route.fullPath" />
        </transition>
      </RouterView>
    </div>
  </el-config-provider>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useAuthStore } from "@/stores/auth"
import { routeLoading } from "@/lib/routeLoading"

const auth = useAuthStore()

onMounted(() => {
  auth.checkAuth()
})
</script>
