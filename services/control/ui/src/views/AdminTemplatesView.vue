<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">全局模板</h1>
        <p class="subtitle">统一管理系统模板内容与默认占位符</p>
      </div>
      <div class="header-actions">
        <t-button variant="outline" @click="() => loadTemplates(activeKey)" :loading="loading">刷新</t-button>
        <t-button theme="primary" @click="handleSave" :loading="saving" :disabled="!activeKey">保存</t-button>
        <t-button theme="danger" variant="outline" @click="handleReset" :loading="saving" :disabled="!activeKey || !activeTemplate?.customized">重置默认</t-button>
      </div>
    </div>

    <t-card bordered class="section-card">
      <div class="section-body">
        <div class="list-header">
          <div class="list-title">模板列表</div>
          <t-button size="small" variant="text" @click="collapsed = !collapsed">{{ collapsed ? '展开' : '收起' }}</t-button>
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
    </t-card>

    <div class="actions-row">
      <div class="actions-left">
        <t-select
          :value="activeTemplate?.mode || 'text'"
          :options="modeOptions"
          disabled
          style="width: 140px"
        />
      </div>
      <div class="actions-right">
        <span class="meta-key">{{ activeTemplate?.key || '-' }}</span>
        <t-tag v-if="activeTemplate?.customized" theme="success" variant="light">已自定义</t-tag>
        <t-tag v-else variant="light">默认</t-tag>
      </div>
    </div>

    <t-card bordered class="section-card">
      <div class="section-body">
        <div class="placeholder-row" v-if="activeTemplate?.placeholders?.length">
          占位符：{{ activeTemplate.placeholders.join(' ') }}
        </div>
        <t-textarea v-model="content" :autosize="{ minRows: 16, maxRows: 26 }" :disabled="!activeKey || loading" />
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { DialogPlugin, MessagePlugin } from "tdesign-vue-next"
import { api, type GlobalTemplate } from "@/lib/api"

type GroupedTemplates = { key: string; items: GlobalTemplate[] }

const templates = ref<GlobalTemplate[]>([])
const activeKey = ref("")
const collapsed = ref(false)
const loading = ref(false)
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
    const { templates: list } = await api.listGlobalTemplates()
    templates.value = list || []
    const nextKey = keepKey && list?.some((t) => t.key === keepKey) ? keepKey : list?.[0]?.key || ""
    activeKey.value = nextKey
    const nextTpl = list?.find((t) => t.key === nextKey)
    content.value = nextTpl?.content ?? ""
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载模板失败")
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

<style scoped>
.header-actions {
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

.list-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 12px;
}

.list-title {
  font-size: 14px;
  font-weight: 600;
  color: #0f172a;
}

.list-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.group-block {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.group-title {
  font-size: 12px;
  color: #475569;
}

.group-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
  gap: 8px;
}

.group-item {
  display: flex;
  align-items: center;
  gap: 8px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  padding: 8px 10px;
  background: #f1f5f9;
  cursor: pointer;
  text-align: left;
}

.group-item.active {
  border-color: #7aa2ff;
  background: #eef3ff;
}

.group-item:disabled {
  cursor: not-allowed;
  opacity: 0.6;
}

.dot {
  width: 8px;
  height: 8px;
  border-radius: 8px;
  background: var(--td-brand-color);
}

.name {
  font-size: 13px;
  color: #0f172a;
}

.actions-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.actions-left,
.actions-right {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.meta-key {
  font-size: 12px;
  color: #475569;
}

.placeholder-row {
  font-size: 12px;
  color: #475569;
  margin-bottom: 8px;
}

@media (max-width: 768px) {
  .header-actions {
    width: 100%;
  }

  .header-actions > * {
    flex: 1;
    min-width: 0;
  }

  .group-grid {
    grid-template-columns: repeat(auto-fit, minmax(130px, 1fr));
  }

  .actions-row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>

