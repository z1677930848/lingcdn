<template>
  <div class="app-root">
    <RouterView />
  </div>
</template>

<script setup lang="ts">
import { onMounted } from "vue"
import { useAuthStore } from "@/stores/auth"

const auth = useAuthStore()

onMounted(() => {
  auth.checkAuth()
})
</script>

<style scoped>
.app-root {
  min-height: 100vh;
  background: var(--app-bg-page);
  color: var(--app-text-strong);
}
</style>
