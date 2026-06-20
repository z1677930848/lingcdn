<template>
  <div class="admin-page">
    <div class="header">
      <div>
        <h1 class="title">系统公告</h1>
        <p class="subtitle">发布系统公告并管理展示状态</p>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="loadData" />

    <el-card class="admin-list-card">
      <div class="admin-toolbar">
        <div class="admin-toolbar-left">
          <el-button type="primary" @click="createOpen = true">新增公告</el-button>
          <el-button plain @click="loadData">刷新</el-button>
        </div>
        <div class="admin-toolbar-right">
          <EpSelect v-model="statusFilter" :options="statusOptions" class="admin-filter-select" />
          <el-input v-model="query" class="admin-search-input" clearable placeholder="搜索" />
        </div>
      </div>

      <el-empty v-if="announcements.length === 0" :description="query ? '未找到匹配的公告' : '暂无公告数据'" class="admin-empty">
        <el-button v-if="!query" type="primary" @click="createOpen = true">创建第一条公告</el-button>
      </el-empty>
      <div v-else class="admin-desktop-only">
        <EpDataTable
          :data="announcements"
          :columns="columns"
          row-key="id"
          hover
          stripe
          :loading="loading"
          :pagination="pagination"
        />
      </div>
      <div v-if="announcements.length > 0" class="admin-mobile-only">
        <div v-if="loading" style="text-align:center;padding:32px 0"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="announcements.length === 0" class="admin-mobile-card-empty">暂无数据</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="item in announcements" :key="item.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ item.title }}</div>
              <div class="admin-mobile-card-tags">
                <el-tag v-if="item.pinned" size="small" type="primary">置顶</el-tag>
                <el-tag :type="item.status === 'published' ? 'success' : 'info'" size="small">{{ item.status === 'published' ? '已发布' : '草稿' }}</el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-subtitle">{{ (item.content || '').slice(0, 80) || '-' }}</div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">创建时间</span>
                <span class="admin-mobile-card-value">{{ formatDateTime(item.created_at) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">更新时间</span>
                <span class="admin-mobile-card-value">{{ formatDateTime(item.updated_at) }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-button size="small" link @click="openEdit(item)">编辑</el-button>
              <el-button size="small" link :loading="statusUpdatingId === item.id" @click="handleToggleStatus(item)">{{ item.status === 'published' ? '转草稿' : '发布' }}</el-button>
              <el-button size="small" link :loading="pinUpdatingId === item.id" @click="handleTogglePin(item)">{{ item.pinned ? '取消置顶' : '置顶' }}</el-button>
              <el-button size="small" link type="danger" @click="deleteTarget = item">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body
      v-model="createOpen"
      header="新增公告"
      :confirm-btn="{ content: creating ? '提交中...' : '提交', loading: creating, theme: 'primary' }"
      cancel-btn="取消"
      width="560"
      @confirm="handleCreate"
      @close="resetCreateForm"
    >
      <el-form label-position="top">
        <el-form-item label="标题">
          <el-input v-model="form.title" :disabled="creating" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="form.content" :disabled="creating" :autosize="{ minRows: 4, maxRows: 8 }" />
        </el-form-item>
        <div class="grid-2">
          <el-form-item label="状态">
            <EpSelect v-model="form.status" :options="publishOptions" :disabled="creating" />
          </el-form-item>
          <el-form-item label="置顶">
            <el-switch v-model="form.pinned" :disabled="creating" />
          </el-form-item>
        </div>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="editOpen"
      header="编辑公告"
      :confirm-btn="{ content: updating ? '保存中...' : '保存', loading: updating, theme: 'primary' }"
      cancel-btn="取消"
      width="560"
      @confirm="handleUpdate"
      @close="closeEdit"
    >
      <el-form label-position="top">
        <el-form-item label="标题">
          <el-input v-model="editForm.title" :disabled="updating" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="editForm.content" :disabled="updating" :autosize="{ minRows: 4, maxRows: 8 }" />
        </el-form-item>
        <div class="grid-2">
          <el-form-item label="状态">
            <EpSelect v-model="editForm.status" :options="publishOptions" :disabled="updating" />
          </el-form-item>
          <el-form-item label="置顶">
            <el-switch v-model="editForm.pinned" :disabled="updating" />
          </el-form-item>
        </div>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="deleteDialogVisible"
      header="删除公告"
      :confirm-btn="{ content: deleting ? '删除中...' : '确认删除', loading: deleting, theme: 'danger' }"
      cancel-btn="取消"
      @confirm="handleDelete"
      @close="() => (deleteTarget = null)"
    >
      <div>删除后将无法恢复，确认删除该公告吗？</div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag, ElButton } from "element-plus"
import { api, type Announcement } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const announcements = ref<Announcement[]>([])
const total = ref(0)
const loading = ref(true)
const error = ref("")
const statusFilter = ref("all")
const query = ref("")
const page = ref(1)
const pageSize = ref(10)

const createOpen = ref(false)
const editOpen = ref(false)
const creating = ref(false)
const updating = ref(false)
const statusUpdatingId = ref<string | null>(null)
const pinUpdatingId = ref<string | null>(null)
const deleting = ref(false)
const deleteTarget = ref<Announcement | null>(null)
const editingAnnouncement = ref<Announcement | null>(null)

const form = ref({ title: "", content: "", status: "published", pinned: false })
const editForm = ref({ title: "", content: "", status: "published", pinned: false })

const deleteDialogVisible = computed(() => !!deleteTarget.value)

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "已发布", value: "published" },
  { label: "草稿", value: "draft" },
]

const publishOptions = [
  { label: "已发布", value: "published" },
  { label: "草稿", value: "draft" },
]

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const loadData = async () => {
  try {
    loading.value = true
    const res = await api.getAdminAnnouncements({
      page: page.value,
      pageSize: pageSize.value,
      status: statusFilter.value !== "all" ? statusFilter.value : undefined,
      q: query.value.trim() || undefined,
    })
    announcements.value = res.announcements || []
    total.value = typeof res.total === "number" ? res.total : 0
    error.value = ""
  } catch (err: any) {
    const message = err?.message || "加载公告失败"
    error.value = message
    MessagePlugin.error(message)
  } finally {
    loading.value = false
  }
}

const openEdit = (announcement: Announcement) => {
  editingAnnouncement.value = announcement
  editForm.value = {
    title: announcement.title || "",
    content: announcement.content || "",
    status: announcement.status || "published",
    pinned: Boolean(announcement.pinned),
  }
  editOpen.value = true
}

const resetCreateForm = () => {
  createOpen.value = false
  form.value = { title: "", content: "", status: "published", pinned: false }
}

const closeEdit = () => {
  editOpen.value = false
  editingAnnouncement.value = null
}

const handleCreate = async () => {
  if (!form.value.title.trim()) {
    MessagePlugin.error("请输入公告标题")
    return
  }
  if (creating.value) return
  creating.value = true
  try {
    await api.createAnnouncement({
      title: form.value.title.trim(),
      content: form.value.content.trim(),
      status: form.value.status,
      pinned: form.value.pinned,
    })
    MessagePlugin.success("公告已创建")
    resetCreateForm()
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "创建公告失败")
  } finally {
    creating.value = false
  }
}

const handleUpdate = async () => {
  if (!editingAnnouncement.value) return
  if (!editForm.value.title.trim()) {
    MessagePlugin.error("请输入公告标题")
    return
  }
  if (updating.value) return
  updating.value = true
  try {
    await api.updateAnnouncement(editingAnnouncement.value.id, {
      title: editForm.value.title.trim(),
      content: editForm.value.content.trim(),
      status: editForm.value.status,
      pinned: editForm.value.pinned,
    })
    MessagePlugin.success("公告已更新")
    closeEdit()
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新公告失败")
  } finally {
    updating.value = false
  }
}

const handleToggleStatus = async (announcement: Announcement) => {
  if (statusUpdatingId.value === announcement.id) return
  const nextStatus = announcement.status === "published" ? "draft" : "published"
  statusUpdatingId.value = announcement.id
  try {
    await api.updateAnnouncement(announcement.id, { status: nextStatus })
    MessagePlugin.success(nextStatus === "published" ? "已发布" : "已转为草稿")
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新状态失败")
  } finally {
    statusUpdatingId.value = null
  }
}

const handleTogglePin = async (announcement: Announcement) => {
  if (pinUpdatingId.value === announcement.id) return
  pinUpdatingId.value = announcement.id
  try {
    await api.updateAnnouncement(announcement.id, { pinned: !announcement.pinned })
    MessagePlugin.success(!announcement.pinned ? "已置顶" : "已取消置顶")
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "更新置顶失败")
  } finally {
    pinUpdatingId.value = null
  }
}

const handleDelete = async () => {
  if (!deleteTarget.value || deleting.value) return
  deleting.value = true
  try {
    await api.deleteAnnouncement(deleteTarget.value.id)
    MessagePlugin.success("公告已删除")
    deleteTarget.value = null
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "删除公告失败")
  } finally {
    deleting.value = false
  }
}

const columns = computed(() => [
  {
    colKey: "title",
    title: "标题",
    width: 260,
    cell: (_h: any, { row }: { row: Announcement }) =>
      h("div", { class: "admin-table-title" }, [
        h("span", null, row.title),
        row.pinned ? h(ElTag, { size: "small", type: "primary", style: "margin-left:8px" }, () => "置顶") : null,
      ]),
  },
  {
    colKey: "content",
    title: "内容",
    cell: (_h: any, { row }: { row: Announcement }) => h("span", { class: "admin-table-description" }, row.content || "-"),
  },
  {
    colKey: "status",
    title: "状态",
    width: 100,
    cell: (_h: any, { row }: { row: Announcement }) =>
      h(ElTag, { type: row.status === "published" ? "success" : "info", size: "small" }, () =>
        row.status === "published" ? "已发布" : "草稿"
      ),
  },
  { colKey: "created_at", title: "创建时间", width: 170, cell: (_h: any, { row }: { row: Announcement }) => formatDateTime(row.created_at) },
  { colKey: "updated_at", title: "更新时间", width: 170, cell: (_h: any, { row }: { row: Announcement }) => formatDateTime(row.updated_at) },
  {
    colKey: "actions",
    title: "操作",
    width: 220,
    fixed: "right",
    cell: (_h: any, { row }: { row: Announcement }) =>
      h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
        h(ElButton, { size: "small", link: true, onClick: () => openEdit(row) }, () => "编辑"),
        h(ElButton, { size: "small", link: true, loading: statusUpdatingId.value === row.id, onClick: () => handleToggleStatus(row) },
          () => (row.status === "published" ? "转草稿" : "发布")
        ),
        h(ElButton, { size: "small", link: true, loading: pinUpdatingId.value === row.id, onClick: () => handleTogglePin(row) },
          () => (row.pinned ? "取消置顶" : "置顶")
        ),
        h(ElButton, { size: "small", link: true, type: "danger", onClick: () => (deleteTarget.value = row) },
          () => "删除"
        ),
      ]),
  },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pi: { current: number; pageSize: number }) => {
    page.value = pi.current
    pageSize.value = pi.pageSize
    loadData()
  },
}))

onMounted(() => {
  loadData()
})
</script>
