<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">证书管理</h1>
        <p class="subtitle">管理所有用户的 HTTPS 证书，支持上传和 ACME 自动签发</p>
      </div>
      <div class="header-actions">
        <el-button type="primary" @click="openUpload">添加证书</el-button>
        <el-button :disabled="!canReapply" @click="reapplyACME">重新申请</el-button>
        <EpDropdown :options="moreOptions" @click="onMoreAction">
          <el-button plain>更多操作 <template #suffix><EpIcon name="chevron-down" /></template></el-button>
        </EpDropdown>
        <EpSelect v-model="filterDomain" placeholder="域名" clearable style="width:140px" :options="domainFilterOptions" />
        <el-input v-model="searchText" placeholder="输入域名,模糊搜索" clearable style="width:200px">
          <template #suffix><EpIcon name="search" /></template>
        </el-input>
        <el-button :loading="loading" plain @click="load">
          <template #icon><EpIcon name="refresh" /></template>
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
          empty-text="暂无数据"
          :pagination="pagination"
          :selected-row-keys="selectedIds"
          @select-change="onSelectChange"
        />
      </div>
      <div class="admin-mobile-only">
        <LoadingState v-if="loading && filtered.length === 0" />
        <div v-else-if="filtered.length === 0" class="admin-mobile-card-empty">暂无证书</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="cert in filtered" :key="cert.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div style="min-width:0">
                <div class="admin-mobile-card-title">#{{ cert.id }} {{ cert.domain }}</div>
                <div class="admin-mobile-card-subtitle">{{ cert.name || '-' }}</div>
              </div>
              <div class="admin-mobile-card-tags">
                <el-tag :type="statusTheme(cert.status)" effect="light" size="small">{{ statusLabel(cert.status) }}</el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row"><span class="admin-mobile-card-label">用户</span><span class="admin-mobile-card-value">{{ userNameOf(cert.user_id) }}</span></div>
              <div class="admin-mobile-card-row"><span class="admin-mobile-card-label">类型</span><span class="admin-mobile-card-value">{{ cert.type === 'acme' ? 'ACME' : '上传' }}</span></div>
              <div class="admin-mobile-card-row"><span class="admin-mobile-card-label">到期时间</span><span class="admin-mobile-card-value">{{ formatTime(cert.expires_at) }}</span></div>
              <div class="admin-mobile-card-row"><span class="admin-mobile-card-label">自动续签</span><span class="admin-mobile-card-value">{{ cert.auto_renew ? '是' : '否' }}</span></div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button v-if="cert.status==='failed'" size="small" link @click="reapplySingle(cert)">重新申请</el-button>
              <el-button size="small" type="danger" link @click="handleDelete(cert)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 添加证书对话框 -->
    <EpDialog append-to-body v-model="uploadOpen" header="添加证书" width="600" :confirm-btn="{ content: '添加', loading: uploadLoading }" cancel-btn="取消" @confirm="submitUpload">
      <div class="form-stack-compact">
        <div class="form-row-compact"><div class="form-label">证书域名</div><el-input v-model="uploadForm.domain" placeholder="example.com 或 *.example.com" /></div>
        <div class="form-row-compact"><div class="form-label">证书名称</div><el-input v-model="uploadForm.name" placeholder="可选" /></div>
        <div class="form-row-compact"><div class="form-label">归属用户</div><EpSelect v-model="uploadForm.userId" :options="userOptions" placeholder="选择用户" clearable filterable /></div>
        <div class="form-row-compact align-top"><div class="form-label">证书内容 (PEM)</div><el-input v-model="uploadForm.certPEM" :autosize="{ minRows: 4 }" placeholder="-----BEGIN CERTIFICATE-----" /></div>
        <div class="form-row-compact align-top"><div class="form-label">私钥内容 (PEM)</div><el-input v-model="uploadForm.keyPEM" :autosize="{ minRows: 4 }" placeholder="-----BEGIN PRIVATE KEY-----" /></div>
      </div>
    </EpDialog>

    <!-- ACME 申请对话框 -->
    <EpDialog append-to-body v-model="acmeOpen" header="申请免费证书 (Let's Encrypt)" width="500" :confirm-btn="{ content: '申请', loading: acmeLoading }" cancel-btn="取消" @confirm="submitACME">
      <div class="form-stack-compact">
        <div class="form-row-compact"><div class="form-label">域名</div><EpSelect v-model="acmeForm.domain" :options="managedDomainOptions" placeholder="选择已接入的域名" filterable /></div>
        <div class="form-row-compact"><div class="form-label">归属用户</div><EpSelect v-model="acmeForm.userId" :options="userOptions" placeholder="选择用户" clearable filterable /></div>
        <div class="form-tip">系统将通过 ACME 协议自动申请 Let's Encrypt 证书。域名必须已接入并解析到 CDN 节点。</div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpDropdown from "@/components/ep/EpDropdown.vue"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, ref } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElTag } from "element-plus"
import { api, type Certificate, type Domain, type User } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"
import LoadingState from "@/components/common/LoadingState.vue"

const loading = ref(false)
const error = ref("")
const certs = ref<Certificate[]>([])
const domains = ref<Domain[]>([])
const users = ref<User[]>([])
const selectedIds = ref<number[]>([])
const searchText = ref("")
const filterDomain = ref("")

// Upload
const uploadOpen = ref(false)
const uploadLoading = ref(false)
const uploadForm = ref({ name: "", domain: "", userId: "", certPEM: "", keyPEM: "" })

// ACME
const acmeOpen = ref(false)
const acmeLoading = ref(false)
const acmeForm = ref({ domain: "", userId: "" })

const load = async () => {
  try {
    loading.value = true
    error.value = ""
    const [certsRes, domainsRes, usersRes] = await Promise.all([
      api.listCertificates(),
      api.listDomains(),
      api.getUsers().catch(() => ({ users: [] })),
    ])
    certs.value = certsRes.certificates || []
    domains.value = domainsRes.domains || []
    users.value = (usersRes as any).users || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const userMap = computed(() => {
  const m: Record<string, User> = {}
  for (const u of users.value) m[u.id] = u
  return m
})
const userNameOf = (uid?: string) => {
  if (!uid) return "-"
  const u = userMap.value[uid]
  return u ? (u.username || u.email || uid) : uid.slice(0, 8)
}
const userOptions = computed(() => users.value.map(u => ({ label: u.username || u.email || u.id, value: u.id })))
const managedDomainOptions = computed(() => domains.value.map(d => ({ label: d.name, value: d.name })))
const domainFilterOptions = computed(() => {
  const s = new Set(certs.value.map(c => c.domain))
  return [...s].sort().map(d => ({ label: d, value: d }))
})

const filtered = computed(() => {
  let list = certs.value
  if (filterDomain.value) list = list.filter(c => c.domain === filterDomain.value)
  if (searchText.value) {
    const q = searchText.value.toLowerCase()
    list = list.filter(c => c.domain.toLowerCase().includes(q) || (c.name || "").toLowerCase().includes(q))
  }
  return list
})

const pagination = computed(() => ({
  defaultCurrent: 1,
  defaultPageSize: 10,
  pageSizeOptions: [10, 20, 50],
  showJumper: true,
  total: filtered.value.length,
}))

const canReapply = computed(() => selectedIds.value.length === 1 && certs.value.find(c => c.id === selectedIds.value[0])?.status === "failed")

const moreOptions = [
  { content: "批量删除", value: "bulk-delete" },
  { content: "申请免费证书 (ACME)", value: "acme" },
]

const onSelectChange = (val: number[]) => { selectedIds.value = val }

const onMoreAction = (item: any) => {
  if (item.value === "bulk-delete") bulkDelete()
  else if (item.value === "acme") openACME()
}

type TagTheme = "success" | "warning" | "danger" | "info" | "primary"
const statusThemeMap: Record<string, TagTheme> = { active: "success", expiring: "warning", expired: "danger", failed: "danger", pending: "info" }
const statusTheme = (s: string): TagTheme => statusThemeMap[s] || "info"
const statusLabel = (s: string) => ({ active: "正常", expiring: "即将到期", expired: "已过期", failed: "失败", pending: "申请中" }[s] || s)

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

const openUpload = () => {
  uploadForm.value = { name: "", domain: "", userId: "", certPEM: "", keyPEM: "" }
  uploadOpen.value = true
}

const openACME = () => {
  acmeForm.value = { domain: "", userId: "" }
  acmeOpen.value = true
}

const submitUpload = async () => {
  if (uploadLoading.value) return
  try {
    uploadLoading.value = true
    const d = uploadForm.value.domain.trim()
    if (!d) { MessagePlugin.error("请输入域名"); return }
    if (!uploadForm.value.certPEM.trim() || !uploadForm.value.keyPEM.trim()) { MessagePlugin.error("请输入证书和私钥"); return }
    await api.createCertificate({
      name: uploadForm.value.name.trim() || d,
      domain: d,
      user_id: uploadForm.value.userId || undefined,
      cert_pem: uploadForm.value.certPEM.trim(),
      key_pem: uploadForm.value.keyPEM.trim(),
    })
    MessagePlugin.success("证书添加成功")
    uploadOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err.message || "添加失败")
  } finally {
    uploadLoading.value = false
  }
}

const submitACME = async () => {
  if (acmeLoading.value) return
  try {
    acmeLoading.value = true
    if (!acmeForm.value.domain) { MessagePlugin.error("请选择域名"); return }
    const res = await api.requestACMECertificate({ domain: acmeForm.value.domain, user_id: acmeForm.value.userId || undefined })
    if (res.ok) {
      MessagePlugin.success(`证书申请成功，到期时间：${formatTime(res.expires_at)}`)
      acmeOpen.value = false
      await load()
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "申请失败")
  } finally {
    acmeLoading.value = false
  }
}

const reapplyACME = () => {
  const cert = certs.value.find(c => c.id === selectedIds.value[0])
  if (cert) reapplySingle(cert)
}

const reapplySingle = async (cert: Certificate) => {
  try {
    const res = await api.requestACMECertificate({ domain: cert.domain, user_id: cert.user_id })
    if (res.ok) {
      MessagePlugin.success(`重新申请成功`)
      await load()
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "重新申请失败")
  }
}

const handleDelete = (cert: Certificate) => {
  const inst = DialogPlugin.confirm({
    header: "删除证书",
    body: `确认删除证书 #${cert.id}「${cert.name || cert.domain}」吗？`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      try {
        await api.deleteCertificate(cert.id)
        certs.value = certs.value.filter(c => c.id !== cert.id)
        MessagePlugin.success("已删除")
      } catch (err: any) {
        MessagePlugin.error(err.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const bulkDelete = async () => {
  if (selectedIds.value.length === 0) { MessagePlugin.warning("请先选择证书"); return }
  const inst = DialogPlugin.confirm({
    header: "批量删除",
    body: `确认删除选中的 ${selectedIds.value.length} 张证书吗？`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      try {
        for (const id of selectedIds.value) await api.deleteCertificate(id)
        MessagePlugin.success("批量删除成功")
        selectedIds.value = []
        await load()
      } catch (err: any) {
        MessagePlugin.error(err.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const columns = computed(() => [
  { colKey: "row-select", type: "multiple" as const, width: 40 },
  { colKey: "id", title: "ID", width: 60, cell: (_h: any, { row }: { row: Certificate }) => h("span", { class: "mono" }, String(row.id)) },
  { colKey: "user_id", title: "用户", width: 120, cell: (_h: any, { row }: { row: Certificate }) => h("span", {}, userNameOf(row.user_id)) },
  { colKey: "name", title: "名称", minWidth: 140, cell: (_h: any, { row }: { row: Certificate }) => h("span", { style: "font-weight:500" }, row.name || "-") },
  { colKey: "type", title: "类型", width: 80, cell: (_h: any, { row }: { row: Certificate }) => h(ElTag, { effect: "light", size: "small", type: row.type === "acme" ? "primary" : "info" }, () => row.type === "acme" ? "ACME" : "上传") },
  { colKey: "domain", title: "域名", minWidth: 180, cell: (_h: any, { row }: { row: Certificate }) => h("span", { class: "mono" }, row.domain) },
  { colKey: "created_at", title: "创建时间", width: 170, cell: (_h: any, { row }: { row: Certificate }) => h("span", { class: "mono" }, formatTime(row.created_at)) },
  { colKey: "expires_at", title: "到期时间", width: 170, cell: (_h: any, { row }: { row: Certificate }) => h("span", { class: "mono", style: ["expired", "expiring"].includes(row.status) ? "color:var(--app-danger)" : "" }, formatTime(row.expires_at)) },
  { colKey: "auto_renew", title: "自动续签", width: 90, cell: (_h: any, { row }: { row: Certificate }) => h(ElTag, { effect: "light", size: "small", type: row.auto_renew ? "success" : "info" }, () => row.auto_renew ? "是" : "否") },
  { colKey: "status", title: "状态", width: 90, cell: (_h: any, { row }: { row: Certificate }) => h(ElTag, { effect: "light", size: "small", type: statusTheme(row.status) }, () => statusLabel(row.status)) },
  {
    colKey: "actions", title: "操作", width: 140,
    cell: (_h: any, { row }: { row: Certificate }) => h("div", { style: "display:flex;gap:6px" }, [
      row.status === "failed" ? h(ElButton, { size: "small", link: true, onClick: () => reapplySingle(row) }, () => "重新申请") : null,
      h(ElButton, { size: "small", type: "danger", link: true, onClick: () => handleDelete(row) }, () => "删除"),
    ]),
  },
])

onMounted(() => { load() })
</script>

