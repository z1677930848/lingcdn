<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">缓存规则</h1>
        <p class="subtitle">按 Host + 路径模式控制缓存 TTL、查询参数与优先级。</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="openDialog()">
          <template #icon><EpIcon name="add" /></template>
          新建规则
        </el-button>
        <el-input v-model="search" clearable placeholder="按名称 / Host / 路径搜索" style="width:280px" />
        <el-button link :loading="loading" @click="load">
          <template #icon><EpIcon name="refresh" /></template>
          刷新
        </el-button>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <el-card class="section-card">
      <div class="admin-desktop-only">
        <EpDataTable
          :data="filtered"
          :columns="columns"
          row-key="id"
          size="small"
          :loading="loading"
          empty-text="暂无缓存规则"
          :pagination="{
            defaultCurrent: 1,
            defaultPageSize: 10,
            pageSizeOptions: [10, 20, 50, 100],
            showJumper: true,
            total: filtered.length,
          }"
        />
      </div>
      <div class="admin-mobile-only">
        <div v-if="loading" class="loading-row"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="filtered.length === 0" class="admin-mobile-card-empty">暂无缓存规则</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="rule in filtered" :key="rule.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <span class="admin-mobile-card-title">{{ rule.name || rule.id }}</span>
              <div class="admin-mobile-card-tags">
                <el-tag size="small" :type="rule.enabled ? 'success' : 'info'" effect="light">
                  {{ rule.enabled ? '启用' : '停用' }}
                </el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">Host</span>
                <span class="admin-mobile-card-value"><code>{{ rule.host_pattern || '*' }}</code></span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">路径</span>
                <span class="admin-mobile-card-value"><code>{{ rule.path_pattern || '/*' }}</code></span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">TTL</span>
                <span class="admin-mobile-card-value">{{ rule.ttl_seconds }}s</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">优先级</span>
                <span class="admin-mobile-card-value">{{ rule.priority ?? 100 }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">缓存 Query</span>
                <span class="admin-mobile-card-value">{{ rule.cache_query_params ? '是' : '否' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">更新时间</span>
                <span class="admin-mobile-card-value">{{ formatTime(rule.updated_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-switch :model-value="rule.enabled" size="small" @change="(v: boolean) => toggle(rule, v)" />
              <span class="switch-text">{{ rule.enabled ? '已启用' : '已停用' }}</span>
              <div class="action-spacer" />
              <el-button size="small" type="primary" link @click="openDialog(rule)">编辑</el-button>
              <el-button size="small" type="danger" link @click="confirmDelete(rule)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body
      v-model="dialogOpen"
      :title="editing ? '编辑缓存规则' : '新建缓存规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submit"
    >
      <div class="form-grid-vertical">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <el-input v-model="form.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">Host 模式</label>
          <el-input v-model="form.host_pattern" placeholder="example.com 或 *.example.com 或 *" class="form-input-v" />
          <div class="form-hint-v">支持精确、单级通配（*.name）与全局（*）。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">路径模式</label>
          <el-input v-model="form.path_pattern" placeholder="/* 或 /api/*" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">TTL (秒)</label>
          <el-input v-model="ttlSecondsText" type="number" class="form-input-v" style="max-width:240px" placeholder="0-31536000" />
          <div class="form-hint-v">0 表示不缓存；最大 31536000（365 天）</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">优先级</label>
          <el-input v-model="priorityText" type="number" class="form-input-v" style="max-width:240px" placeholder="1-10000" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">缓存 Query</label>
          <el-switch v-model="form.cache_query_params" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="form.enabled" />
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, reactive, ref } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElSwitch } from "element-plus"
import { api, type CacheRule } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(false)
const error = ref("")
const rules = ref<CacheRule[]>([])
const search = ref("")
const dialogOpen = ref(false)
const editing = ref<CacheRule | null>(null)

const form = reactive<CacheRule>({
  id: "",
  name: "",
  host_pattern: "",
  path_pattern: "/*",
  methods: ["GET"],
  ttl_seconds: 300,
  cache_query_params: false,
  priority: 100,
  enabled: true,
})

const ttlSecondsText = computed<string>({
  get: () => (form.ttl_seconds == null ? "" : String(form.ttl_seconds)),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    if (!Number.isFinite(n)) {
      form.ttl_seconds = 0
      return
    }
    form.ttl_seconds = Math.max(0, Math.min(31536000, Math.round(n)))
  },
})

const priorityText = computed<string>({
  get: () => (form.priority == null ? "" : String(form.priority)),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    if (!Number.isFinite(n)) {
      form.priority = 100
      return
    }
    form.priority = Math.max(1, Math.min(10000, Math.round(n)))
  },
})

const load = async () => {
  loading.value = true
  error.value = ""
  try {
    const res = await api.listCacheRules()
    rules.value = res.cache_rules || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return rules.value
  return rules.value.filter((r) =>
    [r.name, r.host_pattern, r.path_pattern].filter(Boolean).some((v) => String(v).toLowerCase().includes(q)),
  )
})

const openDialog = (rule?: CacheRule) => {
  editing.value = rule || null
  if (rule) {
    Object.assign(form, rule)
  } else {
    form.id = ""
    form.name = "新规则"
    form.host_pattern = "*"
    form.path_pattern = "/*"
    form.methods = ["GET"]
    form.ttl_seconds = 300
    form.cache_query_params = false
    form.priority = 100
    form.enabled = true
  }
  dialogOpen.value = true
}

const submit = async () => {
  try {
    if (editing.value) {
      await api.updateCacheRule(editing.value.id, form)
      MessagePlugin.success("规则已更新")
    } else {
      await api.createCacheRule(form)
      MessagePlugin.success("规则已创建")
    }
    dialogOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  }
}

const toggle = async (rule: CacheRule, enabled: boolean) => {
  try {
    await api.updateCacheRule(rule.id, { ...rule, enabled })
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新失败")
  }
}

const confirmDelete = (rule: CacheRule) => {
  const inst = DialogPlugin.confirm({
    header: "删除缓存规则",
    body: `确认删除规则「${rule.name || rule.id}」吗？此操作不可撤销。`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    onConfirm: async () => {
      try {
        await api.deleteCacheRule(rule.id)
        MessagePlugin.success("规则已删除")
        await load()
      } catch (err: any) {
        MessagePlugin.error(err?.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

const columns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 160 },
  { colKey: "host_pattern", title: "Host 模式", width: 180 },
  { colKey: "path_pattern", title: "路径模式", minWidth: 180 },
  {
    colKey: "ttl_seconds",
    title: "TTL (秒)",
    width: 110,
    cell: (_h: any, { row }: { row: CacheRule }) => String(row.ttl_seconds || 0),
  },
  {
    colKey: "priority",
    title: "优先级",
    width: 100,
    cell: (_h: any, { row }: { row: CacheRule }) => String(row.priority ?? 100),
  },
  {
    colKey: "cache_query_params",
    title: "Query",
    width: 80,
    cell: (_h: any, { row }: { row: CacheRule }) => (row.cache_query_params ? "是" : "否"),
  },
  {
    colKey: "enabled",
    title: "启用",
    width: 90,
    cell: (_h: any, { row }: { row: CacheRule }) =>
      h(ElSwitch, { size: "small", modelValue: row.enabled, "onUpdate:modelValue": (v: boolean) => toggle(row, v) }),
  },
  {
    colKey: "updated_at",
    title: "更新时间",
    width: 180,
    cell: (_h: any, { row }: { row: CacheRule }) => formatTime(row.updated_at),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    fixed: "right",
    cell: (_h: any, { row }: { row: CacheRule }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(ElButton, { size: "small", type: "primary", link: true, onClick: () => openDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => confirmDelete(row) }, () => "删除"),
      ]),
  },
])

onMounted(load)
</script>

