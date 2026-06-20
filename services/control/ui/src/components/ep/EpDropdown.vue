<template>
  <el-dropdown v-bind="dropdownAttrs" @command="onCommand">
    <slot />
    <template #dropdown>
      <el-dropdown-menu>
        <el-dropdown-item
          v-for="opt in normalizedOptions"
          :key="String(opt.value)"
          :command="opt.value"
          :disabled="opt.disabled"
          :divided="opt.divided"
        >
          {{ opt.content ?? opt.label }}
        </el-dropdown-item>
      </el-dropdown-menu>
    </template>
  </el-dropdown>
</template>

<script setup lang="ts">
import { computed, useAttrs } from "vue"

type Option = {
  label?: string
  content?: string
  value: string | number
  disabled?: boolean
  divided?: boolean
}

const props = defineProps<{
  options?: Option[]
}>()

const emit = defineEmits<{
  (e: "click", data: { value: string | number }): void
  (e: "command", value: string | number): void
}>()

const attrs = useAttrs()
const dropdownAttrs = computed(() => {
  const { options: _o, onClick: _c, ...rest } = attrs as Record<string, unknown>
  return rest
})

const normalizedOptions = computed(() => props.options ?? (attrs.options as Option[] | undefined) ?? [])

function onCommand(value: string | number) {
  emit("command", value)
  emit("click", { value })
}
</script>
