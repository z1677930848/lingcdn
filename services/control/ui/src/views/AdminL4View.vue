<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">四层转发</h1>
        <p class="subtitle">TCP/UDP 转发规则、监听端口与健康检查</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="openDialog()">
          <template #icon><EpIcon name="add" /></template>
          新建规则
        </el-button>
        <el-input v-model="search" clearable placeholder="搜索名称 / 用户 / 端口" style="width:280px" />
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
          empty-text="暂无 L4 转发规则"
          :pagination="{
            defaultCurrent: 1,
            defaultPageSize: 10,
            pageSizeOptions: [10, 20, 50],
            showJumper: true,
            total: filtered.length,
          }"
        />
      </div>
      <div class="admin-mobile-only">
        <div v-if="loading" class="loading-row"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="filtered.length === 0" class="admin-mobile-card-empty">暂无 L4 转发规则</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="rule in filtered" :key="rule.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <span class="admin-mobile-card-title">{{ rule.name || rule.id }}</span>
              <el-tag size="small" :type="rule.enabled ? 'success' : 'info'" effect="light">
                {{ rule.enabled ? '启用' : '停用' }}
              </el-tag>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">用户</span>
                <span class="admin-mobile-card-value">{{ rule.user_id }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">监听</span>
                <span class="admin-mobile-card-value">{{ rule.protocol.toUpperCase() }} :{{ rule.listen_port }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">回源</span>
                <span class="admin-mobile-card-value">{{ rule.origin_host }}:{{ rule.origin_port }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button size="small" type="primary" link @click="openDialog(rule)">编辑</el-button>
              <el-button size="small" type="danger" link @click="confirmDelete(rule)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body
      v-model="dialogOpen"
      :title="editing ? '编辑 L4 规则' : '新建 L4 规则'"
      :confirm-btn="{ content: '保存', loading: saving }"
      cancel-btn="取消"
      @confirm="submit"
    >
      <div class="form-grid-vertical">
        <div class="form-row-v">
          <label class="form-label-v">用户 ID</label>
          <el-input v-model="form.user_id" placeholder="租户用户 ID" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">规则名称</label>
          <el-input v-model="form.name" placeholder="例如：游戏 TCP 转发" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">协议</label>
          <el-radio-group v-model="form.protocol">
            <el-radio value="tcp">TCP</el-radio>
            <el-radio value="udp">UDP</el-radio>
          </el-radio-group>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">监听端口</label>
          <el-input v-model="listenPortText" type="number" placeholder="1-65535" class="form-input-v" style="max-width:240px" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">回源地址</label>
          <el-input v-model="form.origin_host" placeholder="1.2.3.4 或 origin.example.com" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">回源端口</label>
          <el-input v-model="originPortText" type="number" placeholder="1-65535" class="form-input-v" style="max-width:240px" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">回源健康检查</label>
          <el-switch v-model="form.health_check_enabled" />
          <div class="form-hint-v">节点定期探测回源连通性，不健康时暂停转发。</div>
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
import { ElButton, ElTag, ElSwitch } from "element-plus"
import { api, type StreamForward } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(false)
const error = ref("")
const saving = ref(false)
const rules = ref<StreamForward[]>([])
const search = ref("")
const dialogOpen = ref(false)
const editing = ref<StreamForward | null>(null)

const emptyForm = (): StreamForward => ({
  id: "",
  user_id: "",
  name: "",
  protocol: "tcp",
  listen_port: 0,
  origin_host: "",
  origin_port: 0,
  enabled: true,
  health_check_enabled: true,
})

const form = reactive<StreamForward>(emptyForm())

const listenPortText = computed<string>({
  get: () => (form.listen_port ? String(form.listen_port) : ""),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    form.listen_port = Number.isFinite(n) ? Math.max(0, Math.min(65535, Math.round(n))) : 0
  },
})

const originPortText = computed<string>({
  get: () => (form.origin_port ? String(form.origin_port) : ""),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    form.origin_port = Number.isFinite(n) ? Math.max(0, Math.min(65535, Math.round(n))) : 0
  },
})

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return rules.value
  return rules.value.filter((r) => {
    const hay = [r.name, r.user_id, r.origin_host, String(r.listen_port), r.protocol].join(" ").toLowerCase()
    return hay.includes(q)
  })
})

const load = async () => {
  loading.value = true
  error.value = ""
  try {
    const res = await api.adminListStreamForwards()
    rules.value = res.stream_forwards || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载 L4 规则失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const openDialog = (rule?: StreamForward) => {
  editing.value = rule || null
  Object.assign(form, rule ? { ...rule } : emptyForm())
  dialogOpen.value = true
}

const submit = async () => {
  if (!form.user_id.trim()) {
    MessagePlugin.warning("请填写用户 ID")
    return
  }
  if (!form.origin_host.trim()) {
    MessagePlugin.warning("请填写回源地址")
    return
  }
  if (form.listen_port <= 0 || form.origin_port <= 0) {
    MessagePlugin.warning("请填写有效的端口")
    return
  }
  saving.value = true
  try {
    const payload = {
      user_id: form.user_id.trim(),
      name: form.name.trim(),
      protocol: form.protocol,
      listen_port: form.listen_port,
      origin_host: form.origin_host.trim(),
      origin_port: form.origin_port,
      enabled: form.enabled,
      health_check_enabled: form.health_check_enabled,
    }
    if (editing.value?.id) {
      await api.updateStreamForward(editing.value.id, payload)
      MessagePlugin.success("规则已更新")
    } else {
      await api.createStreamForward(payload)
      MessagePlugin.success("规则已创建")
    }
    dialogOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存失败")
  } finally {
    saving.value = false
  }
}

const toggle = async (rule: StreamForward, enabled: boolean) => {
  try {
    await api.updateStreamForward(rule.id, { ...rule, enabled })
    rule.enabled = enabled
    MessagePlugin.success(enabled ? "已启用" : "已停用")
  } catch (err: any) {
    MessagePlugin.error(err.message || "操作失败")
    await load()
  }
}

const confirmDelete = (rule: StreamForward) => {
  DialogPlugin.confirm({
    header: "删除规则",
    body: `确定删除 ${rule.name || rule.id}（端口 ${rule.listen_port}）？`,
    confirmBtn: "删除",
    cancelBtn: "取消",
    theme: "danger",
    onConfirm: async () => {
      try {
        await api.deleteStreamForward(rule.id)
        MessagePlugin.success("已删除")
        await load()
      } catch (err: any) {
        MessagePlugin.error(err.message || "删除失败")
      }
    },
  })
}

const columns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 120, cell: (_h: any, { row }: { row: StreamForward }) => row.name || "-" },
  { colKey: "user_id", title: "用户", width: 120 },
  {
    colKey: "listen",
    title: "监听",
    width: 120,
    cell: (_h: any, { row }: { row: StreamForward }) => `${(row.protocol || "tcp").toUpperCase()} :${row.listen_port}`,
  },
  {
    colKey: "origin",
    title: "回源",
    minWidth: 160,
    cell: (_h: any, { row }: { row: StreamForward }) => `${row.origin_host}:${row.origin_port}`,
  },
  {
    colKey: "health_check_enabled",
    title: "健康检查",
    width: 100,
    cell: (_h: any, { row }: { row: StreamForward }) =>
      h(ElTag, { type: row.health_check_enabled ? "success" : "info", effect: "light", size: "small" }, () =>
        row.health_check_enabled ? "开启" : "关闭"
      ),
  },
  {
    colKey: "enabled",
    title: "状态",
    width: 100,
    cell: (_h: any, { row }: { row: StreamForward }) =>
      h(ElSwitch, { modelValue: row.enabled, size: "small", onChange: (v: boolean) => toggle(row, v) }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    cell: (_h: any, { row }: { row: StreamForward }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(ElButton, { size: "small", link: true, type: "primary", onClick: () => openDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", link: true, type: "danger", onClick: () => confirmDelete(row) }, () => "删除"),
      ]),
  },
])

onMounted(() => { load() })
</script>
