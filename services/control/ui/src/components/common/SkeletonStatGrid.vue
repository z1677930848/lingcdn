<template>
  <div class="skeleton-stat-grid" :style="{ '--cols': String(cols) }">
    <el-card v-for="i in count" :key="i" class="skeleton-stat-card">
      <el-skeleton
        animation="gradient"
        :row-col="[
          { width: '45%', height: '14px' },
          { width: '65%', height: '28px', margin: '12px 0 8px' },
          { width: '35%', height: '12px' },
        ]"
      />
    </el-card>
  </div>
</template>

<script setup lang="ts">
withDefaults(
  defineProps<{
    count?: number
    cols?: number
  }>(),
  {
    count: 4,
    cols: 4,
  },
)
</script>

