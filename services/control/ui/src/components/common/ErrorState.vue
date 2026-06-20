<template>
  <div
    :class="['error-state', { 'error-state--inline': inline }]"
    role="alert"
  >
    <div class="error-icon-wrap">
      <EpIcon name="error-circle" :size="inline ? '18px' : '32px'" class="error-icon" />
    </div>
    <p class="error-message">{{ message }}</p>
    <el-button
      v-if="showRetry"
      :type="inline ? 'info' : 'primary'"
      plain
      size="small"
      @click="$emit('retry')"
    >
      <template #icon><EpIcon name="refresh" /></template>
      重试
    </el-button>
  </div>
</template>

<script setup lang="ts">
import EpIcon from "@/components/ep/EpIcon.vue"
withDefaults(
  defineProps<{
    message?: string
    showRetry?: boolean
    inline?: boolean
  }>(),
  {
    message: "加载失败，请稍后重试",
    showRetry: true,
    inline: false,
  },
)

defineEmits<{
  retry: []
}>()
</script>

