<template>
  <el-popover placement="bottom-end" :width="320" trigger="click">
    <template #reference>
      <el-badge :value="0" :max="99" :offset="[-2, 6]">
        <el-button link aria-label="站内通知">通知</el-button>
      </el-badge>
    </template>
    <div class="notify-popup-panel">
      <div class="panel-title">站内通知</div>
      <EmptyState compact title="暂无通知" />
    </div>
  </el-popover>
</template>

<script setup lang="ts">
import EmptyState from "@/components/atoms/EmptyState.vue"
</script>
