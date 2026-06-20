<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">全局模板</h1>
        <p class="subtitle">统一管理系统模板内容与默认占位符</p>
      </div>
      <div class="header-actions">
        <el-button plain @click="() => loadTemplates(activeKey)" :loading="loading">刷新</el-button>
        <el-button type="primary" @click="handleSave" :loading="saving" :disabled="!activeKey">保存</el-button>
        <el-button type="danger" plain @click="handleReset" :loading="saving" :disabled="!activeKey || !activeTemplate?.customized">重置默认</el-button>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="() => loadTemplates(activeKey)" />

    <el-card class="section-card">
      <div class="section-body">
        <div class="list-header">
          <div class="list-title">模板列表</div>
          <el-button size="small" link @click="collapsed = !collapsed">{{ collapsed ? '展开' : '收起' }}</el-button>
        </div>
        <div v-if="!collapsed" class="list-body">
          <div v-for="group in grouped" :key="group.key" class="group-block">
            <div class="group-title">{{ group.key }}</div>
            <div class="group-grid">
              <button
                v-for="item in group.items"
                :key="item.key"
                type="button"
                class="group-item"
                :class="{ active: item.key === activeKey }"
                @click="handleSelect(item.key)"
                :disabled="loading || saving"
              >
                <span class="dot" />
                <span class="name">{{ item.name }}</span>
              </button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <div class="actions-row">
      <div class="actions-left">
        <EpSelect
          :value="activeTemplate?.mode || 'text'"
          :options="modeOptions"
          disabled
          style="width: 140px"
        />
      </div>
      <div class="actions-right">
        <span class="meta-key">{{ activeTemplate?.key || '-' }}</span>
        <el-tag v-if="activeTemplate?.customized" type="success" effect="light">已自定义</el-tag>
        <el-tag v-else effect="light">默认</el-tag>
      </div>
    </div>

    <el-card class="section-card">
      <div class="section-body">
        <div class="placeholder-row" v-if="activeTemplate?.placeholders?.length">
          占位符：{{ activeTemplate.placeholders.join(' ') }}
        </div>
        <el-input v-model="content" :autosize="{ minRows: 16, maxRows: 26 }" :disabled="!activeKey || loading" />
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpSelect from "@/components/ep/EpSelect.vue"
import { computed, onMounted, ref } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { api, type GlobalTemplate } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

type GroupedTemplates = { key: string; items: GlobalTemplate[] }

const templates = ref<GlobalTemplate[]>([])
const activeKey = ref("")
const collapsed = ref(false)
const loading = ref(false)
const error = ref("")
const saving = ref(false)
const content = ref("")

const modeOptions = [
  { label: "text", value: "text" },
  { label: "html", value: "html" },
  { label: "json", value: "json" },
]

const grouped = computed<GroupedTemplates[]>(() => {
  const groups: Record<string, GlobalTemplate[]> = {}
  for (const t of templates.value) {
    const g = t.group || "其他"
    if (!groups[g]) groups[g] = []
    groups[g].push(t)
  }
  return Object.entries(groups)
    .map(([key, items]) => ({ key, items: items.sort((a, b) => a.name.localeCompare(b.name, "zh-CN")) }))
    .sort((a, b) => a.key.localeCompare(b.key, "zh-CN"))
})

const activeTemplate = computed(() => templates.value.find((t) => t.key === activeKey.value))

const loadTemplates = async (keepKey?: string) => {
  try {
    loading.value = true
    error.value = ""
    const { templates: list } = await api.listGlobalTemplates()
    templates.value = list || []
    const nextKey = keepKey && list?.some((t) => t.key === keepKey) ? keepKey : list?.[0]?.key || ""
    activeKey.value = nextKey
    const nextTpl = list?.find((t) => t.key === nextKey)
    content.value = nextTpl?.content ?? ""
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载模板失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleSelect = (key: string) => {
  activeKey.value = key
  const tpl = templates.value.find((t) => t.key === key)
  content.value = tpl?.content ?? ""
}

const handleSave = async () => {
  if (!activeKey.value) return
  try {
    saving.value = true
    await api.updateGlobalTemplate(activeKey.value, content.value)
    MessagePlugin.success("保存成功")
    await loadTemplates(activeKey.value)
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存失败")
  } finally {
    saving.value = false
  }
}

const handleReset = async () => {
  if (!activeKey.value) return
  // Reset wipes every tenant's customized template for this key back to
  // the system default — confirm once so the admin knows the blast
  // radius is global, not just their own session.
  const inst = DialogPlugin.confirm({
    header: "恢复默认模板",
    body: `确认将模板「${activeKey.value}」恢复为默认版本吗？当前自定义内容会被立即覆盖，此操作不可撤销。`,
    theme: "warning",
    confirmBtn: { content: "恢复默认", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      if (!activeKey.value) { inst.destroy(); return }
      try {
        saving.value = true
        await api.resetGlobalTemplate(activeKey.value)
        MessagePlugin.success("已恢复默认模板")
        await loadTemplates(activeKey.value)
      } catch (err: any) {
        MessagePlugin.error(err.message || "重置失败")
      } finally {
        saving.value = false
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

onMounted(() => {
  loadTemplates()
})
</script>


