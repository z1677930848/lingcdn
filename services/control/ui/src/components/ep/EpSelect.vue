<template>
  <el-select v-bind="selectAttrs" v-model="model" :multiple="multiple">
    <el-option
      v-for="opt in normalizedOptions"
      :key="String(opt.value)"
      :label="opt.label"
      :value="opt.value"
      :disabled="opt.disabled"
    />
  </el-select>
</template>

<script setup lang="ts">
import { computed, useAttrs } from "vue"

type Option = { label: string; value: string | number | boolean; disabled?: boolean }

const props = defineProps<{
  options?: Option[]
  multiple?: boolean
}>()

const model = defineModel<string | number | boolean | Array<string | number> | null>({ default: undefined })

const attrs = useAttrs()
const selectAttrs = computed(() => {
  const { options: _o, ...rest } = attrs as Record<string, unknown>
  return rest
})

const normalizedOptions = computed(() => props.options ?? (attrs.options as Option[] | undefined) ?? [])
</script>
