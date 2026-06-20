<template>
  <LoadingState v-if="loading" />
  <ErrorState v-else-if="error" :message="error" @retry="loadUser" />
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">用户中心</h1>
        <p class="subtitle">管理您的账户信息与安全设置</p>
      </div>
    </div>

    <!-- 基本信息 -->
    <el-card class="section-card">
      <div class="section-body">
        <div class="profile-section-head">
          <div class="profile-section-title">基本信息</div>
        </div>
        <div class="profile-info-grid">
          <div class="profile-info-item">
            <div class="profile-info-label">用户ID</div>
            <div class="profile-info-value">{{ user?.numeric_id ?? "-" }}</div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">用户名</div>
            <div class="profile-info-value">{{ user?.username ?? "-" }}</div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">邮箱</div>
            <div class="profile-info-value">{{ user?.email ?? "-" }}</div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">角色</div>
            <div class="profile-info-value">
              <el-tag v-if="user?.role === 'admin'" type="primary" size="small">管理员</el-tag>
              <el-tag v-else type="info" size="small">普通用户</el-tag>
            </div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">账户状态</div>
            <div class="profile-info-value">
              <el-tag v-if="user?.status === 'active'" type="success" size="small">正常</el-tag>
              <el-tag v-else-if="user?.status === 'disabled'" type="danger" size="small">已禁用</el-tag>
              <el-tag v-else type="warning" size="small">{{ user?.status ?? "-" }}</el-tag>
            </div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">注册时间</div>
            <div class="profile-info-value">{{ formatDateTime(user?.created_at) }}</div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">最近登录</div>
            <div class="profile-info-value">{{ formatDateTime(user?.last_login_at) }}</div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">登录 IP</div>
            <div class="profile-info-value">
              {{ user?.last_login_ip || "-" }}
              <span v-if="user?.last_login_location" style="color:var(--app-text-faint);margin-left:6px">{{ user.last_login_location }}</span>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 修改密码 -->
    <el-card class="section-card">
      <div class="section-body">
        <div class="profile-section-head">
          <div class="profile-section-title">修改密码</div>
        </div>
        <el-form label-position="top" :disabled="passwordSaving" class="password-form">
          <el-form-item label="当前密码">
            <el-input v-model="passwordForm.oldPassword" type="password" placeholder="请输入当前密码" />
          </el-form-item>
          <el-form-item label="新密码">
            <el-input v-model="passwordForm.newPassword" type="password" placeholder="请输入新密码（至少 8 位）" />
          </el-form-item>
          <el-form-item label="确认新密码">
            <el-input v-model="passwordForm.confirmPassword" type="password" placeholder="请再次输入新密码" />
          </el-form-item>
          <div class="profile-actions">
            <el-button type="primary" :loading="passwordSaving" @click="handleChangePassword">修改密码</el-button>
          </div>
        </el-form>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { useAuthStore } from "@/stores/auth"
import { api, type User } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const auth = useAuthStore()
const user = ref<User | null>(null)
const loading = ref(true)
const error = ref("")

const passwordForm = reactive({
  oldPassword: "",
  newPassword: "",
  confirmPassword: "",
})
const passwordSaving = ref(false)

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const loadUser = async () => {
  try {
    loading.value = true
    error.value = ""
    const res = await api.getMe()
    user.value = res.user
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载用户信息失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleChangePassword = async () => {
  if (!passwordForm.oldPassword) {
    MessagePlugin.warning("请输入当前密码")
    return
  }
  if (!passwordForm.newPassword) {
    MessagePlugin.warning("请输入新密码")
    return
  }
  if (passwordForm.newPassword.length < 8) {
    MessagePlugin.warning("新密码长度不能少于 8 位")
    return
  }
  if (passwordForm.newPassword !== passwordForm.confirmPassword) {
    MessagePlugin.warning("两次输入的新密码不一致")
    return
  }
  passwordSaving.value = true
  try {
    await api.changePassword(passwordForm.oldPassword, passwordForm.newPassword)
    MessagePlugin.success("密码修改成功")
    passwordForm.oldPassword = ""
    passwordForm.newPassword = ""
    passwordForm.confirmPassword = ""
  } catch (err: any) {
    MessagePlugin.error(err.message || "密码修改失败")
  } finally {
    passwordSaving.value = false
  }
}

onMounted(async () => {
  if (!auth.user && auth.loading) {
    await auth.checkAuth()
  }
  await loadUser()
})
</script>

