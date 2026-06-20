<template>
  <el-dialog
    v-model="open"
    :title="header"
    :width="width"
    append-to-body
    destroy-on-close
    :close-on-click-modal="closeOnClickModal"
    @closed="emit('closed')"
  >
    <slot />
    <template v-if="showFooter" #footer>
      <el-button @click="handleCancel">{{ cancelLabel }}</el-button>
      <el-button :type="confirmType" :loading="confirmLoading" @click="handleConfirm">
        {{ confirmLabel }}
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
import { computed } from "vue"

export type EpDialogBtn = string | { content?: string; theme?: string; loading?: boolean }

const props = withDefaults(
  defineProps<{
    modelValue?: boolean
    visible?: boolean
    header?: string
    width?: string | number
    confirmBtn?: EpDialogBtn | null
    cancelBtn?: EpDialogBtn | string | null
    closeOnClickModal?: boolean
    showFooter?: boolean
  }>(),
  {
    modelValue: undefined,
    visible: false,
    header: "",
    width: "520px",
    closeOnClickModal: false,
    showFooter: true,
  }
)

const emit = defineEmits<{
  (e: "update:modelValue", value: boolean): void
  (e: "update:visible", value: boolean): void
  (e: "confirm"): void
  (e: "cancel"): void
  (e: "closed"): void
}>()

const open = computed({
  get: () => props.modelValue ?? props.visible ?? false,
  set: (value: boolean) => {
    emit("update:modelValue", value)
    emit("update:visible", value)
  },
})

const confirmLabel = computed(() => {
  const btn = props.confirmBtn
  if (btn == null) return "确定"
  if (typeof btn === "string") return btn
  return btn.content || "确定"
})

const cancelLabel = computed(() => {
  const btn = props.cancelBtn
  if (btn == null) return "取消"
  if (typeof btn === "string") return btn
  return typeof btn === "object" ? btn.content || "取消" : "取消"
})

const confirmLoading = computed(() => {
  const btn = props.confirmBtn
  return typeof btn === "object" && btn?.loading === true
})

const confirmType = computed(() => {
  const btn = props.confirmBtn
  if (typeof btn === "object" && btn?.theme === "danger") return "danger"
  return "primary"
})

function handleConfirm() {
  emit("confirm")
}

function handleCancel() {
  open.value = false
  emit("cancel")
}
</script>
