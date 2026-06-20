<template>
  <el-alert v-if="show" theme="info" class="perm-notice">
    {{ message }}
  </el-alert>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    show?: boolean
    message?: string
  }>(),
  {
    show: true,
    message: "当前用户分组为只读模式，保存与删除等写操作已隐藏。如需完整权限请联系管理员。",
  }
)
</script>
