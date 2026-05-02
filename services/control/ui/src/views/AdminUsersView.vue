<template>
  <div class="admin-page">
    <div v-if="error" class="admin-error-box">
      {{ error }}
      <t-button size="small" theme="primary" @click="loadUsers">重试</t-button>
    </div>

    <t-card bordered class="admin-list-card">
      <t-tabs v-model="activeTab" class="admin-tabs">
        <t-tab-panel value="list" label="用户列表">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button theme="primary" @click="isDialogOpen = true">新增用户</t-button>
              <t-dropdown :options="bulkOptions" @click="handleBulkMenu">
                <t-button variant="outline">更多操作</t-button>
              </t-dropdown>
            </div>
            <div class="admin-toolbar-right">
              <t-select v-model="searchField" :options="searchFieldOptions" class="admin-search-select" />
              <t-input v-model="searchQuery" class="admin-search-input" clearable placeholder="搜索" />
            </div>
          </div>

          <div class="admin-filters-row">
            <div class="admin-filters-left">
              <t-select v-model="roleFilter" :options="roleOptions" class="admin-filter-select" />
              <t-select v-model="statusFilter" :options="statusOptions" class="admin-filter-select" />
              <t-select v-model="sortKey" :options="sortOptions" class="admin-filter-select-large" />
            </div>
            <t-button variant="outline" @click="loadUsers">刷新</t-button>
          </div>

          <!-- Desktop: table view -->
          <div class="admin-users-desktop">
            <t-empty v-if="filteredUsers.length === 0" :description="searchQuery ? '没有匹配的用户' : '暂无用户数据'" class="admin-empty">
              <t-button v-if="!searchQuery" theme="primary" @click="isDialogOpen = true">创建第一个用户</t-button>
            </t-empty>
            <t-table
              v-else
              :data="pagedUsers"
              :columns="columns"
              row-key="id"
              bordered
              hover
              stripe
              size="medium"
              :loading="loading"
              :selected-row-keys="selectedRowKeys"
              :pagination="pagination"
              @select-change="handleSelectChange"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-users-mobile">
            <div v-if="filteredUsers.length === 0" class="admin-user-card-empty">
              {{ searchQuery ? "没有匹配的用户" : "暂无用户数据" }}
            </div>
            <div v-else class="admin-user-cards">
              <div v-for="user in pagedUsers" :key="user.id" class="admin-user-card">
                <div class="admin-user-card-header">
                  <div class="admin-user-card-left">
                    <t-checkbox
                      :checked="selectedRowKeys.includes(user.id)"
                      @change="(checked: boolean) => toggleMobileSelect(user.id, checked)"
                    />
                    <span class="admin-user-card-name">{{ user.username || "-" }}</span>
                  </div>
                  <div class="admin-user-card-tags">
                    <t-tag :theme="user.role === 'admin' ? 'primary' : 'default'" variant="light" size="small">
                      {{ user.role === "admin" ? "管理员" : "普通用户" }}
                    </t-tag>
                    <t-tag :theme="!user.status || user.status === 'active' ? 'success' : 'default'" variant="light" size="small">
                      {{ !user.status || user.status === "active" ? "正常" : "禁用" }}
                    </t-tag>
                  </div>
                </div>
                <div class="admin-user-card-email">{{ user.email || "-" }}</div>
                <div class="admin-user-card-meta">
                  <span>最后登录: {{ formatDateTime(user.last_login_at) }}</span>
                  <span>IP: {{ user.last_login_ip || "-" }}{{ user.last_login_location ? ` (${user.last_login_location})` : "" }}</span>
                </div>
                <div class="admin-user-card-actions">
                  <t-button size="small" variant="outline" @click="handleToggleStatus(user)">
                    {{ user.status === "disabled" ? "启用" : "禁用" }}
                  </t-button>
                  <t-button size="small" variant="outline" @click="openResetPassword([user.id], '重置密码')">重置密码</t-button>
                  <t-button size="small" theme="danger" variant="outline" @click="deleteTarget = user">删除</t-button>
                </div>
              </div>
            </div>
            <div v-if="filteredUsers.length > 0" class="admin-user-card-pagination">
              <t-pagination
                :current="page"
                :page-size="pageSize"
                :total="filteredUsers.length"
                :page-size-options="[10, 20, 50]"
                show-page-size
                size="small"
                @current-change="(v: number) => (page = v)"
                @page-size-change="(v: number) => { pageSize = v; page = 1 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="group" label="用户分组">
          <div class="admin-group-placeholder">
            <t-empty description="暂未开放用户分组功能" />
          </div>
        </t-tab-panel>
      </t-tabs>
    </t-card>

    <t-dialog
      v-model:visible="isDialogOpen"
      header="新增用户"
      :confirm-btn="{ content: creating ? '提交中...' : '创建', loading: creating, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleCreateUser"
      @close="resetDialog"
    >
      <t-form layout="vertical" label-align="top">
        <t-form-item label="用户名">
          <t-input v-model="newUser.username" :disabled="creating" />
        </t-form-item>
        <t-form-item label="邮箱">
          <t-input v-model="newUser.email" type="email" :disabled="creating" />
        </t-form-item>
        <t-form-item label="密码">
          <t-input v-model="newUser.password" type="password" :disabled="creating" />
        </t-form-item>
        <t-form-item label="角色">
          <t-select v-model="newUser.role" :options="roleSelectOptions" :disabled="creating" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="deleteDialogVisible"
      header="删除用户"
      :confirm-btn="{ theme: 'danger', loading: deleting, content: deleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleDeleteUser"
      @close="() => (deleteTarget = null)"
    >
      <div>确认删除用户 <strong>{{ deleteTarget?.username || deleteTarget?.email || deleteTarget?.id }}</strong> 吗？删除后不可恢复。</div>
    </t-dialog>

    <t-dialog
      v-model:visible="resetDialogVisible"
      :header="resetTarget?.title || '重置密码'"
      :confirm-btn="{ theme: 'primary', loading: resetting, content: resetting ? '提交中...' : '确认重置' }"
      cancel-btn="取消"
      @confirm="handleResetPassword"
      @close="() => (resetTarget = null)"
    >
      <t-form layout="vertical" label-align="top">
        <t-form-item label="新密码">
          <t-input v-model="resetPassword" type="password" :disabled="resetting" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="bulkDeleteOpen"
      header="批量删除用户"
      :confirm-btn="{ theme: 'danger', loading: bulkDeleting, content: bulkDeleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleBulkDelete"
      @close="() => (bulkDeleteOpen = false)"
    >
      <div>确认删除选中的 <strong>{{ selectedIds.length }}</strong> 个用户吗？删除后不可恢复。</div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin, Tag, Button } from "tdesign-vue-next"
import { api, type User } from "@/lib/api"

type SearchField = "id" | "username" | "email" | "keyword"

type ResetTarget = { ids: string[]; title: string }

const users = ref<User[]>([])
const loading = ref(true)
const error = ref("")
const searchQuery = ref("")
const searchField = ref<SearchField>("keyword")
const roleFilter = ref("all")
const statusFilter = ref("all")
const sortKey = ref("created_desc")
const selectedRowKeys = ref<Array<string | number>>([])
const activeTab = ref("list")
const page = ref(1)
const pageSize = ref(10)

const isDialogOpen = ref(false)
const creating = ref(false)
const newUser = ref({ username: "", email: "", password: "", role: "user" })

const deleteTarget = ref<User | null>(null)
const deleting = ref(false)

const resetTarget = ref<ResetTarget | null>(null)
const resetPassword = ref("")
const resetting = ref(false)

const bulkDeleteOpen = ref(false)
const bulkDeleting = ref(false)

const deleteDialogVisible = computed(() => !!deleteTarget.value)
const resetDialogVisible = computed(() => !!resetTarget.value)

const searchFieldOptions = [
  { label: "用户ID", value: "id" },
  { label: "用户名", value: "username" },
  { label: "邮箱", value: "email" },
  { label: "关键词", value: "keyword" },
]

const roleOptions = [
  { label: "全部角色", value: "all" },
  { label: "管理员", value: "admin" },
  { label: "普通用户", value: "user" },
]

const roleSelectOptions = [
  { label: "普通用户", value: "user" },
  { label: "管理员", value: "admin" },
]

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "正常", value: "active" },
  { label: "禁用", value: "disabled" },
]

const sortOptions = [
  { label: "创建时间（新→旧）", value: "created_desc" },
  { label: "创建时间（旧→新）", value: "created_asc" },
  { label: "用户名（A→Z）", value: "username_asc" },
]

const bulkOptions = [
  { content: "刷新", value: "refresh" },
  { content: "清空选择", value: "clear" },
  { content: "批量启用", value: "enable" },
  { content: "批量禁用", value: "disable" },
  { content: "批量重置密码", value: "reset_password" },
  { content: "批量删除", value: "delete" },
]

const loadUsers = async () => {
  try {
    loading.value = true
    const { users: list } = await api.getUsers()
    users.value = list || []
    error.value = ""
  } catch (err: any) {
    const message = err.message || "加载用户列表失败"
    error.value = message
    MessagePlugin.error(message)
  } finally {
    loading.value = false
  }
}

const resetDialog = () => {
  newUser.value = { username: "", email: "", password: "", role: "user" }
  isDialogOpen.value = false
}

const handleCreateUser = async () => {
  if (creating.value) return
  if (!newUser.value.username.trim() || !newUser.value.email.trim() || !newUser.value.password.trim()) {
    MessagePlugin.error("请填写完整信息")
    return
  }
  if (newUser.value.password.trim().length < 8) {
    MessagePlugin.error("密码长度不能少于8位")
    return
  }
  creating.value = true
  try {
    await api.createUser(
      newUser.value.username.trim(),
      newUser.value.email.trim(),
      newUser.value.password.trim(),
      newUser.value.role
    )
    MessagePlugin.success("用户创建成功")
    resetDialog()
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "创建用户失败")
  } finally {
    creating.value = false
  }
}

const handleDeleteUser = async () => {
  if (!deleteTarget.value || deleting.value) return
  deleting.value = true
  try {
    await api.deleteUser(deleteTarget.value.id)
    MessagePlugin.success("用户已删除")
    deleteTarget.value = null
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除用户失败")
  } finally {
    deleting.value = false
  }
}

const handleToggleStatus = async (user: User) => {
  if (!user?.id) return
  const nextStatus = user.status === "disabled" ? "active" : "disabled"
  try {
    await api.updateUser(user.id, { status: nextStatus })
    MessagePlugin.success(nextStatus === "disabled" ? "已禁用用户" : "已启用用户")
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新用户状态失败")
  }
}

const openResetPassword = (ids: string[], title: string) => {
  resetPassword.value = ""
  resetTarget.value = { ids, title }
}

const handleResetPassword = async () => {
  if (!resetTarget.value || resetting.value) return
  if (resetPassword.value.trim().length < 8) {
    MessagePlugin.error("密码长度不能少于8位")
    return
  }
  resetting.value = true
  try {
    if (resetTarget.value.ids.length === 1) {
      await api.updateUser(resetTarget.value.ids[0], { password: resetPassword.value.trim() })
    } else {
      await api.bulkUsers({ ids: resetTarget.value.ids, action: "reset_password", password: resetPassword.value.trim() })
    }
    MessagePlugin.success("密码已重置")
    resetTarget.value = null
    resetPassword.value = ""
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "重置密码失败")
  } finally {
    resetting.value = false
  }
}

const handleBulkAction = async (action: "enable" | "disable") => {
  const ids = selectedIds.value
  if (ids.length === 0) {
    MessagePlugin.warning("请先选择用户")
    return
  }
  try {
    await api.bulkUsers({ ids, action })
    MessagePlugin.success(action === "disable" ? "已批量禁用" : "已批量启用")
    selectedRowKeys.value = []
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "批量操作失败")
  }
}

const handleBulkDelete = async () => {
  if (bulkDeleting.value) return
  const ids = selectedIds.value
  if (ids.length === 0) {
    MessagePlugin.warning("请先选择用户")
    return
  }
  bulkDeleting.value = true
  try {
    await api.bulkUsers({ ids, action: "delete" })
    MessagePlugin.success("已批量删除")
    selectedRowKeys.value = []
    bulkDeleteOpen.value = false
    await loadUsers()
  } catch (err: any) {
    MessagePlugin.error(err.message || "批量删除失败")
  } finally {
    bulkDeleting.value = false
  }
}

const filteredUsers = computed(() => {
  const query = searchQuery.value.trim().toLowerCase()
  let data = users.value.filter((user) => {
    if (roleFilter.value !== "all" && user.role !== roleFilter.value) return false
    if (statusFilter.value !== "all") {
      const status = user.status || "active"
      if (status !== statusFilter.value) return false
    }

    if (!query) return true
    const id = String(user.id || "").toLowerCase()
    const username = String(user.username || "").toLowerCase()
    const email = String(user.email || "").toLowerCase()
    switch (searchField.value) {
      case "id":
        return id.includes(query)
      case "username":
        return username.includes(query)
      case "email":
        return email.includes(query)
      default:
        return id.includes(query) || username.includes(query) || email.includes(query)
    }
  })

  data = [...data].sort((a, b) => {
    switch (sortKey.value) {
      case "created_asc":
        return new Date(a.created_at || 0).getTime() - new Date(b.created_at || 0).getTime()
      case "username_asc":
        return String(a.username || "").localeCompare(String(b.username || ""), "zh-CN")
      case "created_desc":
      default:
        return new Date(b.created_at || 0).getTime() - new Date(a.created_at || 0).getTime()
    }
  })

  return data
})

const pagedUsers = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredUsers.value.slice(start, start + pageSize.value)
})

const selectedIds = computed(() => selectedRowKeys.value.map((key) => String(key)).filter(Boolean))

const formatDate = (value?: string) => {
  if (!value) return "-"
  return new Date(value).toLocaleDateString("zh-CN")
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  return new Date(value).toLocaleString("zh-CN")
}

const columns = computed(() => [
  { colKey: "row-select", type: "multiple", width: 48, fixed: "left" },
  {
    colKey: "serial",
    title: "ID",
    width: 70,
    cell: (_h: any, { rowIndex }: { rowIndex: number }) => (page.value - 1) * pageSize.value + rowIndex + 1,
  },
  {
    colKey: "email",
    title: "邮箱",
    minWidth: 220,
    cell: (_h: any, { row }: { row: User }) => row.email || "-",
  },
  {
    colKey: "username",
    title: "用户名",
    minWidth: 140,
    cell: (_h: any, { row }: { row: User }) => row.username || "-",
  },
  {
    colKey: "role",
    title: "角色",
    width: 100,
    cell: (_h: any, { row }: { row: User }) =>
      h(Tag, { theme: row.role === "admin" ? "primary" : "default", variant: "light", size: "small" }, () =>
        row.role === "admin" ? "管理员" : "普通用户"
      ),
  },
  {
    colKey: "status",
    title: "状态",
    width: 80,
    cell: (_h: any, { row }: { row: User }) => {
      const isActive = !row.status || row.status === "active"
      return h(Tag, { theme: isActive ? "success" : "default", variant: "light", size: "small" }, () =>
        isActive ? "正常" : "禁用"
      )
    },
  },
  {
    colKey: "last_login_ip",
    title: "登录IP",
    width: 150,
    cell: (_h: any, { row }: { row: User }) =>
      h("span", { class: "admin-table-mono" }, row.last_login_ip || "-"),
  },
  {
    colKey: "last_login_location",
    title: "登录地址",
    width: 140,
    cell: (_h: any, { row }: { row: User }) =>
      h("span", { class: "admin-table-muted" }, row.last_login_location || "-"),
  },
  {
    colKey: "last_login_at",
    title: "最后登录",
    width: 170,
    cell: (_h: any, { row }: { row: User }) =>
      h("span", { class: "admin-table-muted" }, formatDateTime(row.last_login_at)),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 220,
    fixed: "right",
    cell: (_h: any, { row }: { row: User }) => {
      const isDisabled = row.status === "disabled"
      return h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
        h(Button, { size: "small", variant: "text", onClick: () => handleToggleStatus(row) }, () => (isDisabled ? "启用" : "禁用")),
        h(Button, { size: "small", variant: "text", onClick: () => openResetPassword([row.id], "重置密码") }, () => "重置密码"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => (deleteTarget.value = row) }, () => "删除"),
      ])
    },
  },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: filteredUsers.value.length,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pageInfo: { current: number; pageSize: number }) => {
    page.value = pageInfo.current
    pageSize.value = pageInfo.pageSize
  },
}))

const handleSelectChange = (keys: Array<string | number>) => {
  selectedRowKeys.value = keys
}

const handleBulkMenu = (data: { value: string }) => {
  switch (data.value) {
    case "refresh":
      loadUsers()
      break
    case "clear":
      selectedRowKeys.value = []
      break
    case "enable":
      handleBulkAction("enable")
      break
    case "disable":
      handleBulkAction("disable")
      break
    case "reset_password":
      if (selectedIds.value.length === 0) {
        MessagePlugin.warning("请先选择用户")
        return
      }
      openResetPassword(selectedIds.value, `批量重置密码（${selectedIds.value.length} 个用户）`)
      break
    case "delete":
      if (selectedIds.value.length === 0) {
        MessagePlugin.warning("请先选择用户")
        return
      }
      bulkDeleteOpen.value = true
      break
    default:
      break
  }
}

const toggleMobileSelect = (id: string, checked: boolean) => {
  if (checked) {
    if (!selectedRowKeys.value.includes(id)) {
      selectedRowKeys.value = [...selectedRowKeys.value, id]
    }
  } else {
    selectedRowKeys.value = selectedRowKeys.value.filter((key) => key !== id)
  }
}

onMounted(() => {
  loadUsers()
})
</script>
