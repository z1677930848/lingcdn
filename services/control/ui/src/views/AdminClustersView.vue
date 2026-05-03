<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">集群管理</h1>
        <p class="subtitle">管理 CDN 集群，每个集群绑定独立的 DNS 域名，直接管理节点线路/权重/主备。</p>
      </div>
      <t-button :loading="loading" @click="loadData">刷新</t-button>
    </div>

    <t-card class="card" bordered>
      <div class="toolbar">
        <div class="toolbar-left">
          <t-button theme="primary" @click="openCreate">新建集群</t-button>
        </div>
        <div class="toolbar-right">
          <t-input v-model="search" clearable class="search-input" placeholder="搜索集群名称/域名" />
        </div>
      </div>

      <div class="admin-desktop-only">
        <t-table :data="filtered" :columns="columns" row-key="id" size="small" :loading="loading" empty="暂无集群" />
      </div>

      <div class="admin-mobile-only">
        <div v-if="loading" style="text-align:center;padding:24px;color:#94a3b8">加载中...</div>
        <div v-else-if="filtered.length === 0" style="text-align:center;padding:24px;color:#94a3b8">暂无集群</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="row in filtered" :key="row.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ row.name }}</div>
              <div class="admin-mobile-card-tags">
                <t-tag :theme="row.enabled ? 'success' : 'default'" variant="light" size="small">{{ row.enabled ? '启用' : '停用' }}</t-tag>
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
              <t-button size="small" variant="outline" @click="openNodeDrawer(row)">节点</t-button>
              <t-button size="small" variant="outline" @click="openEdit(row)">编辑</t-button>
              <t-button size="small" theme="danger" variant="outline" @click="deleteTarget = row">删除</t-button>
            </div>
          </div>
        </div>
      </div>
    </t-card>

    <!-- 集群 新建/编辑 -->
    <t-dialog
      v-model:visible="editVisible"
      :header="editTarget?.id ? '编辑集群' : '新建集群'"
      :confirm-btn="{ content: saving ? '保存中...' : '保存', loading: saving, theme: 'primary' }"
      cancel-btn="取消"
      @confirm="handleSave"
    >
      <div class="form-grid">
        <t-form layout="vertical">
          <t-form-item label="集群名称">
            <t-input v-model="form.name" placeholder="请输入集群名称" />
          </t-form-item>
          <t-form-item label="DNS域名">
            <t-select v-model="form.dns_zone" :options="domainOptions" placeholder="选择域名" clearable filterable />
          </t-form-item>
          <t-form-item label="DNS模式">
            <t-select v-model="form.dns_mode" :options="dnsModeOptions" />
          </t-form-item>
          <t-form-item label="CNAME">
            <t-input v-model="form.cname" placeholder="DNS CNAME 记录名（可选）" />
          </t-form-item>
          <t-form-item label="描述">
            <t-input v-model="form.description" placeholder="可选描述信息" />
          </t-form-item>
          <t-form-item label="启用">
            <t-switch v-model="form.enabled" />
          </t-form-item>
        </t-form>
      </div>
    </t-dialog>

    <!-- 删除集群 -->
    <t-dialog
      v-model:visible="deleteVisible"
      header="删除集群"
      :confirm-btn="{ theme: 'danger', loading: deleting, content: deleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleDelete"
    >
      <div class="confirm-text">确认删除集群 <strong>{{ deleteTarget?.name || deleteTarget?.id }}</strong> 吗？</div>
    </t-dialog>

    <!-- ========== 节点管理抽屉（直接挂在集群下） ========== -->
    <t-drawer
      v-model:visible="nodeDrawerVisible"
      size="720px"
      :footer="false"
    >
      <template #header>
        <div class="drawer-header-with-back">
          <t-button class="drawer-back-btn" variant="text" shape="square" @click="nodeDrawerVisible = false">
            <template #icon><t-icon name="chevron-left" /></template>
          </t-button>
          <span>节点管理 — {{ nodeCluster?.name || '' }}</span>
        </div>
      </template>
      <div v-if="nodeCluster" class="drawer-body">
        <div class="drawer-toolbar">
          <t-button theme="primary" size="small" @click="openAddNode">添加节点</t-button>
          <t-select v-model="nodeLineFilter" :options="lineFilterOptions" class="line-select" size="small" />
          <t-button size="small" :loading="nodesLoading" @click="loadNodes">刷新</t-button>
        </div>

        <div class="admin-desktop-only">
          <t-table :data="clusterNodes" :columns="nodeColumns" row-key="node_id" size="small" :loading="nodesLoading" empty="暂无节点" />
        </div>

        <div class="admin-mobile-only">
          <div v-if="nodesLoading" style="text-align:center;padding:24px;color:#94a3b8">加载中...</div>
          <div v-else-if="clusterNodes.length === 0" style="text-align:center;padding:24px;color:#94a3b8">暂无节点</div>
          <div v-else class="admin-mobile-cards">
            <div v-for="row in clusterNodes" :key="row.node_id" class="admin-mobile-card">
              <div class="admin-mobile-card-header">
                <div class="admin-mobile-card-title">{{ row.node?.hostname || row.node_id }}</div>
                <div class="admin-mobile-card-tags">
                  <t-tag :theme="(nodeStatusTagInfo(row.node).theme as any)" variant="light" size="small">{{ nodeStatusTagInfo(row.node).label }}</t-tag>
                  <t-tag :theme="row.enabled ? 'success' : 'default'" variant="light" size="small">{{ row.enabled ? '已绑定' : '已解绑' }}</t-tag>
                  <t-tag v-if="row.backup" theme="warning" variant="light" size="small">备用</t-tag>
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
                <t-button size="small" variant="outline" @click="openEditNode(row)">编辑</t-button>
                <t-button size="small" theme="danger" variant="outline" @click="handleRemoveNode(row)">移除</t-button>
              </div>
            </div>
          </div>
        </div>
      </div>
    </t-drawer>

    <!-- 添加节点 -->
    <t-dialog
      v-model:visible="addNodeVisible"
      header="添加节点"
      :confirm-btn="{ content: addingNode ? '添加中...' : '添加', loading: addingNode, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleAddNode"
    >
      <t-form layout="vertical">
        <t-form-item label="线路">
          <t-select v-model="addNodeLine" :options="dnsLineOptions" />
        </t-form-item>
        <t-form-item label="选择节点">
          <t-loading v-if="availableNodesLoading" size="small" />
          <div v-else-if="availableNodes.length === 0" class="no-nodes">没有可用的节点</div>
          <div v-else class="node-check-list">
            <t-checkbox-group v-model="addNodeIds">
              <t-checkbox v-for="node in availableNodes" :key="node.id" :value="node.id" class="node-check-item">
                <span class="node-label">{{ node.hostname }}</span>
                <span class="node-ip">{{ node.public_ip || '-' }}</span>
                <span v-if="node.region" class="node-region">{{ node.region }}</span>
              </t-checkbox>
            </t-checkbox-group>
          </div>
        </t-form-item>
      </t-form>
    </t-dialog>

    <!-- 编辑节点设置 -->
    <t-dialog
      v-model:visible="editNodeVisible"
      header="编辑节点设置"
      :confirm-btn="{ content: updatingNode ? '保存中...' : '保存', loading: updatingNode, theme: 'primary' }"
      cancel-btn="取消"
      width="480"
      @confirm="handleUpdateNode"
    >
      <t-form layout="vertical">
        <t-form-item label="线路">
          <t-select v-model="editNodeForm.line" :options="dnsLineOptions" />
        </t-form-item>
        <t-form-item label="权重">
          <t-input-number v-model="editNodeForm.weight" :min="0" :max="100" />
        </t-form-item>
        <t-form-item label="启用">
          <t-switch v-model="editNodeForm.enabled" />
        </t-form-item>
        <t-form-item label="备用节点">
          <t-switch v-model="editNodeForm.backup" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from "vue"
import { MessagePlugin, Button, Tag } from "tdesign-vue-next"
import { api, type Cluster, type ClusterNodeMeta, type Node as CDNNode } from "@/lib/api"

/* ==================== 集群列表 ==================== */
const loading = ref(true)
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
    const [res, lRes, pRes] = await Promise.all([api.listClusters(), api.getDNSLines(), api.getProviderDomains()])
    clusters.value = res?.clusters || []
    providerDomains.value = pRes?.domains || []
    if (lRes?.lines?.length) dnsLines.value = lRes.lines
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载集群列表失败")
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
      h(Tag, { theme: row.enabled ? "success" : "default", variant: "light", size: "small" } as any, () => (row.enabled ? "启用" : "停用")),
  },
  {
    colKey: "created_at", title: "创建时间", minWidth: 170,
    cell: (_h: any, { row }: { row: Cluster }) => h("span", { class: "table-muted" }, formatTime(row.created_at)),
  },
  {
    colKey: "actions", title: "操作", width: 200, fixed: "right",
    cell: (_h: any, { row }: { row: Cluster }) =>
      h("div", { style: "display:flex;gap:8px" }, [
        h(Button, { size: "small", variant: "text", onClick: () => openNodeDrawer(row) }, () => "节点"),
        h(Button, { size: "small", variant: "text", onClick: () => openEdit(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => (deleteTarget.value = row) }, () => "删除"),
      ]),
  },
]

/* ==================== 节点管理（直接挂在集群下） ==================== */
const nodeDrawerVisible = ref(false)
const nodeCluster = ref<Cluster | null>(null)
const nodeLineFilter = ref("all")
const clusterNodes = ref<ClusterNodeMeta[]>([])
const nodesLoading = ref(false)

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
  try {
    const line = nodeLineFilter.value === "all" ? undefined : nodeLineFilter.value
    const res = await api.listClusterNodes(nodeCluster.value.id, line)
    clusterNodes.value = res?.nodes || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载节点列表失败")
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
  if (s === "pending") return { theme: "default", label: "待激活" }
  if (node?.comm_ok === false) return { theme: "danger", label: "通信异常" }
  if (node?.report_ok === false) return { theme: "warning", label: "上报异常" }
  if (!s) return { theme: "default", label: "未知" }
  if (s === "online" || s === "running") return { theme: "success", label: "在线" }
  if (s === "offline") return { theme: "warning", label: "离线" }
  if (s === "stopped") return { theme: "warning", label: "已停止" }
  return { theme: "default", label: node?.status || "未知" }
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
      return h(Tag, { theme: info.theme, variant: "light", size: "small" } as any, () => info.label)
    },
  },
  {
    colKey: "line", title: "线路", width: 100,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h(Tag, { variant: "light", size: "small" } as any, () => row.line || "默认"),
  },
  {
    colKey: "weight", title: "权重", width: 70,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) => row.weight ?? 0,
  },
  {
    colKey: "enabled", title: "绑定", width: 80,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h(Tag, { theme: row.enabled ? "success" : "default", variant: "light", size: "small" } as any, () => (row.enabled ? "已绑定" : "已解绑")),
  },
  {
    colKey: "backup", title: "备用", width: 70,
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      row.backup ? h(Tag, { theme: "warning", variant: "light", size: "small" } as any, () => "备用") : h("span", { class: "table-muted" }, "-"),
  },
  {
    colKey: "actions", title: "操作", width: 130, fixed: "right",
    cell: (_h: any, { row }: { row: ClusterNodeMeta }) =>
      h("div", { style: "display:flex;gap:8px" }, [
        h(Button, { size: "small", variant: "text", onClick: () => openEditNode(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => handleRemoveNode(row) }, () => "移除"),
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

const openAddNode = async () => {
  addNodeLine.value = "默认"
  addNodeIds.value = []
  addNodeVisible.value = true
  if (!nodeCluster.value) return
  availableNodesLoading.value = true
  try {
    const res = await api.listClusterAvailableNodes(nodeCluster.value.id)
    availableNodes.value = res?.nodes || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载可用节点失败")
  } finally {
    availableNodesLoading.value = false
  }
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

<style scoped>
.page { display: flex; flex-direction: column; gap: 16px; }
.header { display: flex; align-items: flex-start; justify-content: space-between; gap: 12px; }
.title { font-size: 20px; font-weight: 700; margin: 0; line-height: 1.4; }
.subtitle { font-size: 13px; color: #94a3b8; margin: 4px 0 0; }
.card {
  border-radius: 8px;
  border: 1px solid #e2e8f0;
  overflow: hidden;
  /* 让卡片拿走 .page 在 flex column 里的剩余高度 —— 空集群下卡片才
   * 不会缩成顶部一小条，下面留一大片灰底空白。`.page` 已经是 flex
   * column（line 556），所以直接 flex: 1 就够了。 */
  flex: 1;
  min-height: 0;
}
.toolbar { display: flex; justify-content: space-between; align-items: center; gap: 12px; flex-wrap: wrap; margin-bottom: 12px; }
.toolbar-left, .toolbar-right { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.search-input { width: 220px; }
.form-grid { display: grid; gap: 12px; }
.mono { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", "Courier New", monospace; }
.table-muted { font-size: 12px; color: #94a3b8; }
.confirm-text { line-height: 1.6; }
.drawer-body { display: flex; flex-direction: column; gap: 12px; }
.drawer-toolbar { display: flex; align-items: center; gap: 10px; flex-wrap: wrap; }
.line-select { width: 140px; }
.node-check-list { max-height: 320px; overflow-y: auto; display: flex; flex-direction: column; gap: 4px; }
.node-check-item { display: flex; align-items: center; gap: 8px; padding: 4px 0; }
.node-label { font-weight: 600; }
.node-ip { font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace; font-size: 12px; color: #94a3b8; }
.node-region { font-size: 12px; color: #475569; background: #f1f5f9; padding: 1px 6px; border-radius: 4px; }
.no-nodes { color: #94a3b8; font-size: 13px; padding: 12px 0; }

/* 抽屉标题栏返回按钮 */
.drawer-header-with-back {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 16px;
  font-weight: 600;
  width: 100%;
}

/* 桌面端默认隐藏返回按钮（drawer 自带关闭 X），仅在移动端显示。
 * 注意：默认规则必须放在媒体查询之前，否则当 specificity 相同时
 * 后写的 `display: none` 会覆盖媒体查询里的 `display: inline-flex`，
 * 导致移动端也看不到返回按钮（曾经的 bug）。 */
.drawer-back-btn {
  display: none;
}

@media (max-width: 768px) {
  .toolbar { flex-direction: column; align-items: stretch; }
  .toolbar-left, .toolbar-right { width: 100%; }
  .search-input { width: 100%; }
  .drawer-toolbar { flex-wrap: wrap; }
  .line-select { width: 100%; }
  .drawer-back-btn { display: inline-flex; }
}
</style>
