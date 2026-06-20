<template>
  <div class="admin-page">
    <ErrorState v-if="error" :message="error" @retry="loadUsers" />

    <el-card class="admin-list-card">
      <el-tabs v-model="activeTab" class="admin-tabs">
        <el-tab-pane name="list" label="用户列表">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <el-button type="primary" @click="isDialogOpen = true">新增用户</el-button>
              <EpDropdown :options="bulkOptions" @click="handleBulkMenu">
                <el-button plain>更多操作</el-button>
              </EpDropdown>
            </div>
            <div class="admin-toolbar-right">
              <EpSelect v-model="searchField" :options="searchFieldOptions" class="admin-search-select" />
              <el-input v-model="searchQuery" class="admin-search-input" clearable placeholder="搜索" />
            </div>
          </div>

          <div class="admin-filters-row">
            <div class="admin-filters-left">
              <EpSelect v-model="roleFilter" :options="roleOptions" class="admin-filter-select" />
              <EpSelect v-model="statusFilter" :options="statusOptions" class="admin-filter-select" />
              <EpSelect v-model="groupFilter" :options="groupFilterOptions" class="admin-filter-select" />
              <EpSelect v-model="sortKey" :options="sortOptions" class="admin-filter-select-large" />
            </div>
            <el-button plain @click="loadUsers">刷新</el-button>
          </div>

          <!-- Desktop: table view -->
          <div class="admin-users-desktop">
            <el-empty v-if="filteredUsers.length === 0" :description="searchQuery ? '没有匹配的用户' : '暂无用户数据'" class="admin-empty">
              <el-button v-if="!searchQuery" type="primary" @click="isDialogOpen = true">创建第一个用户</el-button>
            </el-empty>
            <EpDataTable
              v-else
              :data="pagedUsers"
              :columns="columns"
              row-key="id"
              hover
              stripe
              size="default"
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
                    <el-checkbox
                      :checked="selectedRowKeys.includes(user.id)"
                      @change="(checked: boolean) => toggleMobileSelect(user.id, checked)"
                    />
                    <span class="admin-user-card-name">{{ user.username || "-" }}</span>
                  </div>
                  <div class="admin-user-card-tags">
                    <el-tag :type="user.role === 'admin' ? 'primary' : 'info'" effect="light" size="small">
                      {{ user.role === "admin" ? "管理员" : "普通用户" }}
                    </el-tag>
                    <el-tag :type="!user.status || user.status === 'active' ? 'success' : 'info'" effect="light" size="small">
                      {{ !user.status || user.status === "active" ? "正常" : "禁用" }}
                    </el-tag>
                  </div>
                </div>
                <div class="admin-user-card-email">{{ user.email || "-" }}</div>
                <div class="admin-user-card-meta">
                  <span>用户ID: {{ displayUserId(user) }}</span>
                  <span>最后登录: {{ formatDateTime(user.last_login_at) }}</span>
                  <span>IP: {{ user.last_login_ip || "-" }}{{ user.last_login_location ? ` (${user.last_login_location})` : "" }}</span>
                </div>
                <div class="admin-user-card-actions">
                  <el-button size="small" plain @click="openEditUser(user)">编辑</el-button>
                  <el-button size="small" plain @click="handleToggleStatus(user)">
                    {{ user.status === "disabled" ? "启用" : "禁用" }}
                  </el-button>
                  <el-button size="small" plain @click="openResetPassword([user.id], '重置密码')">重置密码</el-button>
                  <el-button size="small" type="danger" plain @click="deleteTarget = user">删除</el-button>
                </div>
              </div>
            </div>
            <div v-if="filteredUsers.length > 0" class="admin-user-card-pagination">
              <el-pagination
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
        </el-tab-pane>

        <el-tab-pane name="group" label="用户分组">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <el-button type="primary" @click="() => openGroupDialog()">新建分组</el-button>
            </div>
            <div class="admin-toolbar-right">
              <el-button plain @click="loadUserGroups">刷新</el-button>
            </div>
          </div>
          <el-empty v-if="!groupsLoading && userGroups.length === 0" description="暂无用户分组" class="admin-empty" />
          <EpDataTable
            v-else
            :data="userGroups"
            :columns="groupColumns"
            row-key="id"
            :loading="groupsLoading"
            hover
            stripe
          />
        </el-tab-pane>
      </el-tabs>
    </el-card>

    <EpDialog append-to-body
      v-model="isDialogOpen"
      header="新增用户"
      :confirm-btn="{ content: creating ? '提交中...' : '创建', loading: creating, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleCreateUser"
      @close="resetDialog"
    >
      <el-form label-position="top">
        <el-form-item label="用户名">
          <el-input v-model="newUser.username" :disabled="creating" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="newUser.email" type="email" :disabled="creating" />
        </el-form-item>
        <el-form-item label="密码">
          <el-input v-model="newUser.password" type="password" :disabled="creating" />
        </el-form-item>
        <el-form-item label="角色">
          <EpSelect v-model="newUser.role" :options="roleSelectOptions" :disabled="creating" />
        </el-form-item>
        <el-form-item label="用户分组">
          <EpSelect v-model="newUser.group_id" :options="groupSelectOptions" :disabled="creating" clearable />
        </el-form-item>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="deleteDialogVisible"
      header="删除用户"
      :confirm-btn="{ theme: 'danger', loading: deleting, content: deleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleDeleteUser"
      @close="() => (deleteTarget = null)"
    >
      <div>确认删除用户 <strong>{{ deleteTarget?.username || deleteTarget?.email || deleteTarget?.id }}</strong> 吗？删除后不可恢复。</div>
    </EpDialog>

    <EpDialog append-to-body
      v-model="resetDialogVisible"
      :title="resetTarget?.title || '重置密码'"
      :confirm-btn="{ theme: 'primary', loading: resetting, content: resetting ? '提交中...' : '确认重置' }"
      cancel-btn="取消"
      @confirm="handleResetPassword"
      @close="() => (resetTarget = null)"
    >
      <el-form label-position="top">
        <el-form-item label="新密码">
          <el-input v-model="resetPassword" type="password" :disabled="resetting" />
        </el-form-item>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="bulkDeleteOpen"
      header="批量删除用户"
      :confirm-btn="{ theme: 'danger', loading: bulkDeleting, content: bulkDeleting ? '删除中...' : '删除' }"
      cancel-btn="取消"
      @confirm="handleBulkDelete"
      @close="() => (bulkDeleteOpen = false)"
    >
      <div>确认删除选中的 <strong>{{ selectedIds.length }}</strong> 个用户吗？删除后不可恢复。</div>
    </EpDialog>

    <EpDialog append-to-body
      v-model="groupDialogOpen"
      :title="groupEditing ? '编辑用户分组' : '新建用户分组'"
      :confirm-btn="{ content: groupSaving ? '提交中...' : '保存', loading: groupSaving, theme: 'primary' }"
      cancel-btn="取消"
      @confirm="handleSaveGroup"
      @close="resetGroupDialog"
    >
      <el-form label-position="top">
        <el-form-item label="名称">
          <el-input v-model="groupForm.name" :disabled="groupSaving" />
        </el-form-item>
        <el-form-item label="描述">
          <el-input v-model="groupForm.description" :disabled="groupSaving" />
        </el-form-item>
        <el-form-item label="权限（预留，当前不限制用户功能）">
          <p class="group-perm-hint">分组仅用于用户归类与筛选。勾选权限项暂不影响控制台菜单与 API，普通用户默认拥有全部产品功能。</p>
          <el-checkbox-group v-model="groupForm.permissions" :disabled="groupSaving">
            <el-checkbox v-for="perm in availablePermissions" :key="perm.key" :value="perm.key">{{ perm.label }}</el-checkbox>
          </el-checkbox-group>
        </el-form-item>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="assignGroupOpen"
      header="分配用户分组"
      :confirm-btn="{ content: assignGroupSaving ? '提交中...' : '保存', loading: assignGroupSaving, theme: 'primary' }"
      cancel-btn="取消"
      @confirm="handleAssignGroup"
      @close="() => (assignGroupTarget = null)"
    >
      <el-form label-position="top">
        <el-form-item label="用户">
          <el-input :model-value="assignGroupTarget?.username || assignGroupTarget?.email || ''" disabled />
        </el-form-item>
        <el-form-item label="分组">
          <EpSelect v-model="assignGroupId" :options="groupSelectOptions" clearable />
        </el-form-item>
      </el-form>
    </EpDialog>

    <EpDialog append-to-body
      v-model="editUserOpen"
      header="编辑用户"
      :confirm-btn="{ content: editUserSaving ? '提交中...' : '保存', loading: editUserSaving, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleSaveEditUser"
      @close="resetEditUserDialog"
    >
      <el-form label-position="top">
        <el-form-item label="用户ID">
          <el-input :model-value="editUserTarget ? displayUserId(editUserTarget) : '-'" disabled />
        </el-form-item>
        <el-form-item label="用户名">
          <el-input v-model="editUserForm.username" :disabled="editUserSaving" />
        </el-form-item>
        <el-form-item label="邮箱">
          <el-input v-model="editUserForm.email" type="email" :disabled="editUserSaving" />
        </el-form-item>
        <el-form-item label="角色">
          <EpSelect v-model="editUserForm.role" :options="roleSelectOptions" :disabled="editUserSaving || editUserIsSelf" />
          <p v-if="editUserIsSelf" class="group-perm-hint">不能修改自己的角色</p>
        </el-form-item>
      </el-form>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpDropdown from "@/components/ep/EpDropdown.vue"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, h, onMounted, ref, watch } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag, ElButton } from "element-plus"
import { api, type User, type UserGroup, type UserGroupPermission } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"
import { useAuthStore } from "@/stores/auth"

type SearchField = "id" | "username" | "email" | "keyword"

type ResetTarget = { ids: string[]; title: string }

const auth = useAuthStore()

const users = ref<User[]>([])
const loading = ref(true)
const error = ref("")
const searchQuery = ref("")
const searchField = ref<SearchField>("keyword")
const roleFilter = ref("all")
const statusFilter = ref("all")
const groupFilter = ref("all")
const sortKey = ref("id_asc")
const selectedRowKeys = ref<Array<string | number>>([])
const activeTab = ref("list")
const page = ref(1)
const pageSize = ref(10)

const isDialogOpen = ref(false)
const creating = ref(false)
const newUser = ref({ username: "", email: "", password: "", role: "user", group_id: "" })

const deleteTarget = ref<User | null>(null)
const deleting = ref(false)

const resetTarget = ref<ResetTarget | null>(null)
const resetPassword = ref("")
const resetting = ref(false)

const bulkDeleteOpen = ref(false)
const bulkDeleting = ref(false)

const userGroups = ref<UserGroup[]>([])
const availablePermissions = ref<UserGroupPermission[]>([])
const groupsLoading = ref(false)
const groupDialogOpen = ref(false)
const groupSaving = ref(false)
const groupEditing = ref<UserGroup | null>(null)
const groupForm = ref({ name: "", description: "", permissions: [] as string[] })
const assignGroupOpen = ref(false)
const assignGroupSaving = ref(false)
const assignGroupTarget = ref<User | null>(null)
const assignGroupId = ref("")
const editUserOpen = ref(false)
const editUserSaving = ref(false)
const editUserTarget = ref<User | null>(null)
const editUserForm = ref({ username: "", email: "", role: "user" })

const deleteDialogVisible = computed(() => !!deleteTarget.value)
const resetDialogVisible = computed(() => !!resetTarget.value)
const editUserIsSelf = computed(() => {
  const targetId = editUserTarget.value?.id
  const selfId = auth.user?.id
  return !!targetId && !!selfId && targetId === selfId
})

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
  { label: "用户ID（小→大）", value: "id_asc" },
  { label: "创建时间（新→旧）", value: "created_desc" },
  { label: "创建时间（旧→新）", value: "created_asc" },
  { label: "用户名（A→Z）", value: "username_asc" },
]

const groupSelectOptions = computed(() => [
  { label: "未分组", value: "" },
  ...userGroups.value.map((g) => ({ label: g.name, value: g.id })),
])

const groupFilterOptions = computed(() => [
  { label: "全部分组", value: "all" },
  { label: "未分组", value: "none" },
  ...userGroups.value.map((g) => ({ label: g.name, value: g.id })),
])

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
  newUser.value = { username: "", email: "", password: "", role: "user", group_id: "" }
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
      newUser.value.role,
      newUser.value.group_id
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
    if (groupFilter.value !== "all") {
      if (groupFilter.value === "none") {
        if (user.group_id) return false
      } else if ((user.group_id || "") !== groupFilter.value) {
        return false
      }
    }

    if (!query) return true
    const numericId = String(user.numeric_id ?? "")
    const id = String(user.id || "").toLowerCase()
    const username = String(user.username || "").toLowerCase()
    const email = String(user.email || "").toLowerCase()
    switch (searchField.value) {
      case "id":
        return numericId.includes(query) || id.includes(query)
      case "username":
        return username.includes(query)
      case "email":
        return email.includes(query)
      default:
        return numericId.includes(query) || id.includes(query) || username.includes(query) || email.includes(query)
    }
  })

  data = [...data].sort((a, b) => {
    switch (sortKey.value) {
      case "id_asc":
        return (a.numeric_id ?? 0) - (b.numeric_id ?? 0)
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

const displayUserId = (user: User) => {
  const id = user.numeric_id
  if (id != null && id > 0) return String(id)
  return "-"
}

const columns = computed(() => [
  { colKey: "row-select", type: "multiple", width: 48, fixed: "left" },
  {
    colKey: "numeric_id",
    title: "用户ID",
    width: 90,
    cell: (_h: any, { row }: { row: User }) =>
      h("span", { class: "admin-table-mono" }, displayUserId(row)),
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
      h(ElTag, { type: row.role === "admin" ? "primary" : "info", effect: "light", size: "small" }, () =>
        row.role === "admin" ? "管理员" : "普通用户"
      ),
  },
  {
    colKey: "status",
    title: "状态",
    width: 80,
    cell: (_h: any, { row }: { row: User }) => {
      const isActive = !row.status || row.status === "active"
      return h(ElTag, { type: isActive ? "success" : "info", effect: "light", size: "small" }, () =>
        isActive ? "正常" : "禁用"
      )
    },
  },
  {
    colKey: "group_name",
    title: "分组",
    width: 120,
    cell: (_h: any, { row }: { row: User }) =>
      h("span", { class: "admin-table-muted" }, row.group_name || "未分组"),
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
    width: 320,
    fixed: "right",
    cell: (_h: any, { row }: { row: User }) => {
      const isDisabled = row.status === "disabled"
      return h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
        h(ElButton, { size: "small", link: true, onClick: () => openEditUser(row) }, () => "编辑"),
        h(ElButton, { size: "small", link: true, onClick: () => handleToggleStatus(row) }, () => (isDisabled ? "启用" : "禁用")),
        h(ElButton, { size: "small", link: true, onClick: () => openAssignGroup(row) }, () => "分组"),
        h(ElButton, { size: "small", link: true, onClick: () => openResetPassword([row.id], "重置密码") }, () => "重置密码"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => (deleteTarget.value = row) }, () => "删除"),
      ])
    },
  },
])

const groupColumns = computed(() => [
  {
    colKey: "name",
    title: "名称",
    minWidth: 160,
    cell: (_h: any, { row }: { row: UserGroup }) => row.name || "-",
  },
  {
    colKey: "description",
    title: "描述",
    minWidth: 180,
    cell: (_h: any, { row }: { row: UserGroup }) =>
      h("span", { class: "admin-table-muted" }, row.description || "-"),
  },
  {
    colKey: "permissions",
    title: "权限",
    width: 100,
    cell: (_h: any, { row }: { row: UserGroup }) =>
      h("span", { class: "admin-table-muted" }, row.permissions?.length ? `${row.permissions.length} 项` : "不限"),
  },
  {
    colKey: "created_at",
    title: "创建时间",
    width: 170,
    cell: (_h: any, { row }: { row: UserGroup }) =>
      h("span", { class: "admin-table-muted" }, formatDateTime(row.created_at)),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 160,
    fixed: "right",
    cell: (_h: any, { row }: { row: UserGroup }) =>
      h("div", { style: "display:flex;gap:6px" }, [
        h(ElButton, { size: "small", link: true, onClick: () => openGroupDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => handleDeleteGroup(row) }, () => "删除"),
      ]),
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

const loadUserGroups = async () => {
  groupsLoading.value = true
  try {
    const res = await api.getUserGroups()
    userGroups.value = res.groups || []
    availablePermissions.value = res.available_permissions || []
  } catch (e: any) {
    MessagePlugin.error(e?.message || "加载用户分组失败")
  } finally {
    groupsLoading.value = false
  }
}

const openGroupDialog = (group?: UserGroup) => {
  groupEditing.value = group || null
  groupForm.value = {
    name: group?.name || "",
    description: group?.description || "",
    permissions: [...(group?.permissions || [])],
  }
  groupDialogOpen.value = true
}

const resetGroupDialog = () => {
  groupEditing.value = null
  groupForm.value = { name: "", description: "", permissions: [] }
}

const openAssignGroup = (user: User) => {
  assignGroupTarget.value = user
  assignGroupId.value = user.group_id || ""
  assignGroupOpen.value = true
}

const openEditUser = (user: User) => {
  editUserTarget.value = user
  editUserForm.value = {
    username: user.username || "",
    email: user.email || "",
    role: user.role || "user",
  }
  editUserOpen.value = true
}

const resetEditUserDialog = () => {
  editUserTarget.value = null
  editUserForm.value = { username: "", email: "", role: "user" }
}

const handleSaveEditUser = async () => {
  const target = editUserTarget.value
  if (!target || editUserSaving.value) return false
  const username = editUserForm.value.username.trim()
  const email = editUserForm.value.email.trim()
  if (!username || !email) {
    MessagePlugin.warning("请填写用户名和邮箱")
    return false
  }
  if (!email.includes("@")) {
    MessagePlugin.warning("邮箱格式无效")
    return false
  }
  editUserSaving.value = true
  try {
    const payload: { username: string; email: string; role?: string } = { username, email }
    if (!editUserIsSelf.value && editUserForm.value.role !== target.role) {
      payload.role = editUserForm.value.role
    }
    await api.updateUser(target.id, payload)
    MessagePlugin.success("已保存")
    editUserOpen.value = false
    resetEditUserDialog()
    await loadUsers()
    return true
  } catch (e: any) {
    MessagePlugin.error(e?.message || "保存失败")
    return false
  } finally {
    editUserSaving.value = false
  }
}

const handleAssignGroup = async () => {
  if (!assignGroupTarget.value || assignGroupSaving.value) return false
  assignGroupSaving.value = true
  try {
    await api.updateUser(assignGroupTarget.value.id, { group_id: assignGroupId.value })
    MessagePlugin.success("已更新分组")
    assignGroupOpen.value = false
    assignGroupTarget.value = null
    await loadUsers()
    return true
  } catch (e: any) {
    MessagePlugin.error(e?.message || "分配失败")
    return false
  } finally {
    assignGroupSaving.value = false
  }
}

const handleSaveGroup = async () => {
  const name = groupForm.value.name.trim()
  if (!name) {
    MessagePlugin.warning("请填写名称")
    return false
  }
  groupSaving.value = true
  try {
    if (groupEditing.value) {
      await api.updateUserGroup(groupEditing.value.id, {
        name,
        description: groupForm.value.description.trim(),
        permissions: groupForm.value.permissions,
      })
      MessagePlugin.success("已更新")
    } else {
      await api.createUserGroup({
        name,
        description: groupForm.value.description.trim(),
        permissions: groupForm.value.permissions,
      })
      MessagePlugin.success("已创建")
    }
    groupDialogOpen.value = false
    resetGroupDialog()
    await loadUserGroups()
    return true
  } catch (e: any) {
    MessagePlugin.error(e?.message || "保存失败")
    return false
  } finally {
    groupSaving.value = false
  }
}

const handleDeleteGroup = async (group: UserGroup) => {
  try {
    await api.deleteUserGroup(group.id)
    MessagePlugin.success("已删除")
    await loadUserGroups()
  } catch (e: any) {
    MessagePlugin.error(e?.message || "删除失败")
  }
}

onMounted(() => {
  loadUsers()
  loadUserGroups()
})

watch(activeTab, (tab) => {
  if (tab === "group" && userGroups.value.length === 0 && !groupsLoading.value) {
    loadUserGroups()
  }
})
</script>
