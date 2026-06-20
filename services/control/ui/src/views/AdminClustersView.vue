<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">集群管理</h1>
        <p class="subtitle">管理 CDN 集群，每个集群绑定独立的 DNS 域名，直接管理节点线路/权重/主备。</p>
      </div>
      <el-button :loading="loading" @click="loadData">刷新</el-button>
    </div>

    <ErrorState v-if="error" :message="error" @retry="loadData" />

    <el-card class="cluster-page-card">
      <div class="toolbar">
        <div class="toolbar-left">
          <el-button type="primary" @click="openCreate">新建集群</el-button>
        </div>
        <div class="toolbar-right">
          <el-input v-model="search" clearable class="search-input" placeholder="搜索集群名称/域名" />
        </div>
      </div>

      <div class="admin-desktop-only">
        <EpDataTable :data="filtered" :columns="columns" row-key="id" size="small" :loading="loading" empty-text="暂无集群" />
      </div>

      <div class="admin-mobile-only">
        <LoadingState v-if="loading && filtered.length === 0" />
        <div v-else-if="filtered.length === 0" style="text-align:center;padding:24px;color:var(--app-text-faint)">暂无集群</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="row in filtered" :key="row.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ row.name }}</div>
              <div class="admin-mobile-card-tags">
                <el-tag :type="row.enabled ? 'success' : 'info'" effect="light" size="small">{{ row.enabled ? '启用' : '停用' }}</el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">DNS域名</span>
                <span class="admin-mobile-card-value mono">{{ row.dns_zone || '-' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">CNAME</span>
                <span class="admin-mobile-card-value mono">{{ row.cname || '-' }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">创建时间</span>
                <span class="admin-mobile-card-value">{{ formatTime(row.created_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button size="small" plain @click="openNodeDrawer(row)">节点</el-button>
              <el-button size="small" plain @click="openEdit(row)">编辑</el-button>
              <el-button size="small" type="danger" plain @click="deleteTarget = row">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 集群 新建/编辑 -->
    <EpDialog append-to-body
      v-model="editVisible"
      :title="editTarget?.id ? '编辑集群' : '新建集群'"
      :confirm-btn="{ content: saving ? '保存中...' : '保存', loading: saving, theme: 'primary' }"
      cancel-btn="取消"
      @confirm="handleSave"
    >
      <div class="cluster-dialog-grid">
        <el-form label-position="top">
          <el-form-item label="集群名称">
            <el-input v-model="form.name" placeholder="请输入集群名称" />
          </el-form-item>
          <el-form-item label="DNS域名">
            <EpSelect v-model="form.dns_zone" :options="domainOptions" placeholder="选择域名" clearable filterable />
          </el-form-item>
          <el-form-item label="DNS模式">
            <EpSelect v-model="form.dns_mode" :options="dnsModeOptions" />
          </el-form-item>
          <el-form-item label="CNAME">
            <el-input v-model="form.cname" placeholder="DNS CNAME 记录名（可选）" />
          </el-form-item>
          <el-form-item label="描述">
            <el-input v-model="form.description" placeholder="可选描述信息" />
          </el-form-item>
          <el-form-item label="启用">
            <el-switch v-model="form.enabled" />
          </el-form-item>
        </el-form>
      </div>
    </EpDialog>

    <!-- 删除集群 -->
    <EpDialog append-to-body
      v-model="deleteVisible"
      header="删除集群"
      :confirm-btn="{ theme: 'danger', loading: deleting, content: deleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleDelete"
    >
      <div class="confirm-text">确认删除集群 <strong>{{ deleteTarget?.name || deleteTarget?.id }}</strong> 吗？</div>
    </EpDialog>

    <!-- ========== 节点管理抽屉（直接挂在集群下） ========== -->
    <el-drawer append-to-body
      v-model="nodeDrawerVisible"
      size="720px"
      :footer="false"
    >
      <template #header>
        <div class="drawer-header-with-back">
          <el-button class="drawer-back-btn" link shape="square" @click="nodeDrawerVisible = false">
            <template #icon><EpIcon name="chevron-left" /></template>
          </el-button>
          <span>节点管理 — {{ nodeCluster?.name || '' }}</span>
        </div>
      </template>
      <div v-if="nodeCluster" class="drawer-body">
        <div class="drawer-toolbar">
          <el-button type="primary" size="small" @click="openAddNode">添加节点</el-button>
          <EpSelect v-model="nodeLineFilter" :options="lineFilterOptions" class="line-select" size="small" />
          <el-button size="small" :loading="nodesLoading" @click="loadNodes">刷新</el-button>
        </div>

        <ErrorState v-if="nodesError" :message="nodesError" @retry="loadNodes" />

        <template v-else>
        <div class="admin-desktop-only">
          <EpDataTable :data="clusterNodes" :columns="nodeColumns" row-key="node_id" size="small" :loading="nodesLoading" empty-text="暂无节点" />
        </div>

        <div class="admin-mobile-only">
          <div v-if="nodesLoading" style="text-align:center;padding:24px;color:var(--app-text-faint)">加载中...</div>
          <div v-else-if="clusterNodes.length === 0" style="text-align:center;padding:24px;color:var(--app-text-faint)">暂无节点</div>
          <div v-else class="admin-mobile-cards">
            <div v-for="row in clusterNodes" :key="row.node_id" class="admin-mobile-card">
              <div class="admin-mobile-card-header">
                <div class="admin-mobile-card-title">{{ row.node?.hostname || row.node_id }}</div>
                <div class="admin-mobile-card-tags">
                  <el-tag :type="(nodeStatusTagInfo(row.node).theme as any)" effect="light" size="small">{{ nodeStatusTagInfo(row.node).label }}</el-tag>
                  <el-tag :type="row.enabled ? 'success' : 'info'" effect="light" size="small">{{ row.enabled ? '已绑定' : '已解绑' }}</el-tag>
                  <el-tag v-if="row.backup" type="warning" effect="light" size="small">备用</el-tag>
                </div>
              </div>
              <div v-if="row.node?.public_ip" class="admin-mobile-card-subtitle mono">{{ row.node.public_ip }}</div>
              <div class="admin-mobile-card-rows">
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">线路</span>
                  <span class="admin-mobile-card-value">{{ row.line || '默认' }}</span>
                </div>
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">权重</span>
                  <span class="admin-mobile-card-value">{{ row.weight ?? 0 }}</span>
                </div>
              </div>
              <div class="admin-mobile-card-actions">
                <el-button size="small" plain @click="openEditNode(row)">编辑</el-button>
                <el-button size="small" type="danger" plain @click="handleRemoveNode(row)">移除</el-button>
              </div>
            </div>
          </div>
        </div>
        </template>
      </div>
    </el-drawer>

    <!-- 添加节点 -->
    <EpDialog append-to-body
      v-model="addNodeVisible"
      header="添加节点"
      :confirm-btn="{ content: addingNode ? '添加中...' : '添加', loading: addingNode, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleAddNode"
    >
      <el-form label-position="top">
        <el-form-item label="线路">
          <EpSelect v-model="addNodeLine" :options="dnsLineOptions" />
        </el-form-item>
        <el-form-item label="选择节点">
          <LoadingState v-if="availableNodesLoading" size="small" text="加载可用节点…" />
          <ErrorState v-else-if="availableNodesError" :message="availableNodesError" @retry="loadAvailableNodes" />
          <div v-else-if="availableNodes.length === 0" class="no-nodes">没有可用的节点</div>
          <div v-else class="node-check-list">
            <el-checkbox-group v-model="addNodeIds">
              <el-checkbox v-for="node in availableNodes" :key="node.id" :value="node.id" class="node-check-item">
                <span class="node-label">{{ node.hostname }}</span>
                <span class="node-ip">{{ node.public_ip || '-' }}</span>
                <span v-if="node.region" class="node-region">{{ node.region }}</span>
              </el-checkbox>
            </el-checkbox-group>
          </div>
        </el-form-item>
      </el-form>
    </EpDialog>

    <!-- 编辑节点设置 -->
    <EpDialog append-to-body
      v-model="editNodeVisible"
      header="编辑节点设置"
      :confirm-btn="{ content: updatingNode ? '保存中...' : '保存', loading: updatingNode, theme: 'primary' }"
      cancel-btn="取消"
      width="480"
      @confirm="handleUpdateNode"
    >
      <el-form label-position="top">
        <el-form-item label="线路">
          <EpSelect v-model="editNodeForm.line" :options="dnsLineOptions" />
        </el-form-item>
        <el-form-item label="权重">
          <el-input-number v-model="editNodeForm.weight" :min="0" :max="100" />
        </el-form-item>
        <el-form-item label="启用">
          <el-switch v-model="editNodeForm.enabled" />
        </el-form-item>
        <el-form-item label="备用节点">
          <el-switch v-model="editNodeForm.backup" />
        </el-form-item>
      </el-form>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, ref, watch } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElButton, ElTag } from "element-plus"
import { api, type Cluster, type ClusterNodeMeta, type Node as CDNNode } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"
import LoadingState from "@/components/common/LoadingState.vue"

/* ==================== 集群列表 ==================== */
const loading = ref(true)
const error = ref("")
const saving = ref(false)
const deleting = ref(false)
const search = ref("")
const clusters = ref<Cluster[]>([])
const providerDomains = ref<{ name: string; record_count: number }[]>([])
const dnsLines = ref<{ value: string; label: string }[]>([{ value: "默认", label: "默认" }])

const editTarget = ref<Cluster | null>(null)
const deleteTarget = ref<Cluster | null>(null)

const dnsModeOptions = [
  { label: "完整域名 (zone)", value: "zone" },
  { label: "子域名前缀 (subdomain)", value: "subdomain" },
]

const domainOptions = computed(() =>
  providerDomains.value.map((d) => ({ label: d.name, value: d.name }))
)

const form = ref({ name: "", dns_zone: "", dns_mode: "zone", cname: "", description: "", enabled: true })

const editVisible = computed({
  get: () => !!editTarget.value,
  set: (v) => { if (!v) editTarget.value = null },
})

const deleteVisible = computed({
  get: () => !!deleteTarget.value,
  set: (v) => { if (!v) deleteTarget.value = null },
})

const loadData = async () => {
  try {
    loading.value = true
    error.value = ""
    const [res, lRes, pRes] = await Promise.all([api.listClusters(), api.getDNSLines(), api.getProviderDomains()])
    clusters.value = res?.clusters || []
    providerDomains.value = pRes?.domains || []
    if (lRes?.lines?.length) dnsLines.value = lRes.lines
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载集群列表失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

onMounted(loadData)

const resetForm = () => {
  form.value = { name: "", dns_zone: "", dns_mode: "zone", cname: "", description: "", enabled: true }
}

const openCreate = () => {
  resetForm()
  editTarget.value = { id: "", name: "", dns_zone: "", dns_mode: "zone", enabled: true }
}

const openEdit = (row: Cluster) => {
  form.value = {
    name: row.name || "",
    dns_zone: row.dns_zone || "",
    dns_mode: row.dns_mode || "zone",
    cname: row.cname || "",
    description: row.description || "",
    enabled: row.enabled ?? true,
  }
  editTarget.value = row
}

const handleSave = async () => {
  if (saving.value || !editTarget.value) return
  if (!form.value.name.trim()) { MessagePlugin.warning("集群名称必填"); return }
  if (!form.value.dns_zone) { MessagePlugin.warning("DNS域名必填"); return }
  saving.value = true
  try {
    const payload: Partial<Cluster> = {
      name: form.value.name.trim(),
      dns_zone: form.value.dns_zone,
      dns_mode: form.value.dns_mode,
      cname: form.value.cname.trim(),
      description: form.value.description.trim(),
      enabled: form.value.enabled,
    }
    if (editTarget.value.id) {
      await api.updateCluster(editTarget.value.id, payload)
      MessagePlugin.success("已更新集群")
    } else {
      await api.createCluster(payload)
      MessagePlugin.success("已新建集群")
    }
    editTarget.value = null
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存失败")
  } finally {
    saving.value = false
  }
}

const handleDelete = async () => {
  if (!deleteTarget.value || deleting.value) return
  deleting.value = true
  try {
    await api.deleteCluster(deleteTarget.value.id)
    MessagePlugin.success("已删除集群")
    deleteTarget.value = null
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除失败")
  } finally {
    deleting.value = false
  }
}

const filtered = computed(() => {
  const q = search.value.trim().toLowerCase()
  if (!q) return clusters.value
  return clusters.value.filter((c) => {
    const hay = [c.id, c.name, c.dns_zone, c.cname, c.description].filter(Boolean).join(" ").toLowerCase()
    return hay.includes(q)
  })
})

const formatTime = (t?: string) => {
  if (!t) return "-"
  try { return new Date(t).toLocaleString("zh-CN") } catch { return t }
}

const dnsModeLabel = (mode: string) => {
  if (mode === "zone") return "完整域名"
  if (mode === "subdomain") return "子域名前缀"
  return mode || "-"
}

const columns = [
  {
    colKey: "name", title: "名称", minWidth: 140,
    cell: (_h: any, { row }: { row: Cluster }) => h("span", { style: "font-weight:600" }, row.name),
  },
  {
    colKey: "dns_zone", title: "DNS域名", minWidth: 180,
    cell: (_h: any, { row }: { row: Cluster }) => h("span", { class: "mono" }, row.dns_zone || "-"),
  },
  {
    colKey: "cname", title: "CNAME", minWidth: 160,
    cell: (_h: any, { row }: { row: Cluster }) => h("span", { class: "mono" }, row.cname || "-"),
  },
  {
    colKey: "dns_mode", title: "DNS模式", minWidth: 120,
    cell: (_h: any, { row }: { row: Cluster }) => dnsModeLabel(row.dns_mode),
  },
  {
    colKey: "enabled", title: "状态", width: 100,
    cell: (_h: any, { row }: { row: Cluster }) =>
      h(ElTag, { type: row.enabled ? "success" : "info", effect: "light", size: "small" } as any, () => (row.enabled ? "启用" : "停用")),
  },
  {
    colKey: "created_at", title: "创建时间", minWidth: 170,
    cell: (_h: any, { row }: { row: Cluster }) => h("span", { class: "table-muted" }, formatTime(row.created_at)),
  },
  {
    colKey: "actions", title: "操作", width: 200, fixed: "right",
    cell: (_h: any, { row }: { row: Cluster }) =>
      h("div", { style: "display:flex;gap:8px" }, [
        h(ElButton, { size: "small", link: true, onClick: () => openNodeDrawer(row) }, () => "节点"),
        h(ElButton, { size: "small", link: true, onClick: () => openEdit(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => (deleteTarget.value = row) }, () => "删除"),
      ]),
  },
]

/* ==================== 节点管理（直接挂在集群下） ==================== */
const nodeDrawerVisible = ref(false)
const nodeCluster = ref<Cluster | null>(null)
const nodeLineFilter = ref("all")
const clusterNodes = ref<ClusterNodeMeta[]>([])
const nodesLoading = ref(false)
const nodesError = ref("")

const dnsLineOptions = computed(() => dnsLines.value)
const lineFilterOptions = computed(() => [{ label: "全部线路", value: "all" }, ...dnsLines.value])

const openNodeDrawer = (cluster: Cluster) => {
  nodeCluster.value = cluster
  nodeLineFilter.value = "all"
  nodeDrawerVisible.value = true
  loadNodes()
}

const loadNodes = async () => {
  if (!nodeCluster.value) return
  nodesLoading.value = true
  nodesError.value = ""
  try {
    const line = nodeLineFilter.value === "all" ? undefined : nodeLineFilter.value
    const res = await api.listClusterNodes(nodeCluster.value.id, line)
    clusterNodes.value = res?.nodes || []
  } catch (err: unknown) {
    nodesError.value = err instanceof Error ? err.message : "加载节点列表失败"
  } finally {
    nodesLoading.value = false
  }
}

watch(nodeLineFilter, () => { if (nodeDrawerVisible.value) loadNodes() })

// Same precedence rules as AdminNodesView: explicit disabled/pending first,
// then transport/report health flags, then the raw status string. We read the
// node's own state here, NOT the cluster-binding "enabled" flag.
const nodeStatusTagInfo = (node?: CDNNode | null): { theme: string; label: string } => {
  const s = (node?.status || "").trim().toLowerCase()
  if (s === "disabled") return { theme: "danger", label: "已禁用" }
  if (s === "pending") return { theme: "info", label: "待激活" }
  if (node?.comm_ok === false) return { theme: "danger", label: "通信异常" }
  if (node?.report_ok === false) return { theme: "warning", label: "上报异常" }
  if (!s) return { theme: "info", label: "未知" }
  if (s === "online" || s === "running") return { theme: "success", label: "在线" }
  if (s === "offline") return { theme: "warning", label: "离线" }
  if (s === "stopped") return { theme: "warning", label: "已停止" }
  return { theme: "info", label: node?.status || "未知" }
}

const nodeColumns = [
  {
    colKey: "hostname", title: "节点", minWidth: 140,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h("div", null, [
        h("div", { style: "font-weight:600" }, row.node?.hostname || row.node_id),
        row.node?.public_ip ? h("div", { class: "table-muted" }, row.node.public_ip) : null,
      ]),
  },
  {
    colKey: "node_status", title: "节点状态", width: 100,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) => {
      const info = nodeStatusTagInfo(row.node)
      return h(ElTag, { type: info.theme, effect: "light", size: "small" } as any, () => info.label)
    },
  },
  {
    colKey: "line", title: "线路", width: 100,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h(ElTag, { effect: "light", size: "small" } as any, () => row.line || "默认"),
  },
  {
    colKey: "weight", title: "权重", width: 70,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) => row.weight ?? 0,
  },
  {
    colKey: "enabled", title: "绑定", width: 80,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h(ElTag, { type: row.enabled ? "success" : "info", effect: "light", size: "small" } as any, () => (row.enabled ? "已绑定" : "已解绑")),
  },
  {
    colKey: "backup", title: "备用", width: 70,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      row.backup ? h(ElTag, { type: "warning", effect: "light", size: "small" } as any, () => "备用") : h("span", { class: "table-muted" }, "-"),
  },
  {
    colKey: "actions", title: "操作", width: 130, fixed: "right",
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h("div", { style: "display:flex;gap:8px" }, [
        h(ElButton, { size: "small", link: true, onClick: () => openEditNode(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => handleRemoveNode(row) }, () => "移除"),
      ]),
  },
]

/* --- 添加节点 --- */
const addNodeVisible = ref(false)
const addNodeLine = ref("默认")
const addNodeIds = ref<string[]>([])
const addingNode = ref(false)
const availableNodes = ref<CDNNode[]>([])
const availableNodesLoading = ref(false)
const availableNodesError = ref("")

const loadAvailableNodes = async () => {
  if (!nodeCluster.value) return
  availableNodesLoading.value = true
  availableNodesError.value = ""
  try {
    const res = await api.listClusterAvailableNodes(nodeCluster.value.id)
    availableNodes.value = res?.nodes || []
  } catch (err: unknown) {
    availableNodesError.value = err instanceof Error ? err.message : "加载可用节点失败"
  } finally {
    availableNodesLoading.value = false
  }
}

const openAddNode = async () => {
  addNodeLine.value = "默认"
  addNodeIds.value = []
  addNodeVisible.value = true
  await loadAvailableNodes()
}

const handleAddNode = async () => {
  if (addingNode.value || !nodeCluster.value) return
  if (addNodeIds.value.length === 0) { MessagePlugin.warning("请选择至少一个节点"); return }
  addingNode.value = true
  try {
    await api.addClusterNodes(nodeCluster.value.id, addNodeIds.value, addNodeLine.value)
    MessagePlugin.success("已添加节点")
    addNodeVisible.value = false
    await loadNodes()
  } catch (err: any) {
    MessagePlugin.error(err.message || "添加节点失败")
  } finally {
    addingNode.value = false
  }
}

/* --- 编辑节点 --- */
const editNodeVisible = ref(false)
const editNodeTarget = ref<ClusterNodeMeta | null>(null)
const updatingNode = ref(false)
const editNodeForm = ref({ line: "默认", weight: 0, enabled: true, backup: false })

const openEditNode = (row: ClusterNodeMeta) => {
  editNodeTarget.value = row
  editNodeForm.value = {
    line: row.line || "默认",
    weight: row.weight ?? 0,
    enabled: row.enabled ?? true,
    backup: row.backup ?? false,
  }
  editNodeVisible.value = true
}

const handleUpdateNode = async () => {
  if (updatingNode.value || !nodeCluster.value || !editNodeTarget.value) return
  updatingNode.value = true
  try {
    await api.updateClusterNode(nodeCluster.value.id, editNodeTarget.value.node_id, {
      line: editNodeForm.value.line,
      weight: editNodeForm.value.weight,
      enabled: editNodeForm.value.enabled,
      backup: editNodeForm.value.backup,
    })
    MessagePlugin.success("已更新节点设置")
    editNodeVisible.value = false
    await loadNodes()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新节点设置失败")
  } finally {
    updatingNode.value = false
  }
}

/* --- 移除节点 --- */
const handleRemoveNode = async (row: ClusterNodeMeta) => {
  if (!nodeCluster.value) return
  try {
    await api.deleteClusterNode(nodeCluster.value.id, row.node_id, row.line)
    MessagePlugin.success("已移除节点")
    await loadNodes()
  } catch (err: any) {
    MessagePlugin.error(err.message || "移除节点失败")
  }
}
</script>

