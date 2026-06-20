<template>
  <div class="settings-page">
    <section class="settings-panel" :aria-labelledby="`tab-${currentTab.tab}`">
      <div class="settings-panel-head">
        <h2 :id="`tab-${currentTab.tab}`" class="settings-panel-title">{{ currentTab.label }}</h2>
        <p class="settings-panel-desc">{{ currentTab.description }}</p>
      </div>
      <div class="settings-panel-body">
        <component :is="currentTab.component" />
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, watch } from "vue"
import { useRoute, useRouter } from "vue-router"
import {
  ADMIN_SETTINGS_NAV_ITEMS,
  isValidAdminSettingsTab,
  resolveAdminSettingsTab,
} from "@/lib/adminSettingsNav"

const route = useRoute()
const router = useRouter()

const currentTab = computed(() => resolveAdminSettingsTab(route.params.tab as string | undefined))

watch(
  () => route.params.tab,
  (tab) => {
    if (!isValidAdminSettingsTab(tab as string | undefined)) {
      router.replace(ADMIN_SETTINGS_NAV_ITEMS[0].href)
    }
  },
  { immediate: true }
)
</script>
