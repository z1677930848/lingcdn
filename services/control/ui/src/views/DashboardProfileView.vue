<template>
  <div v-if="loading" class="profile-loading">
    <t-loading size="medium" />
  </div>
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">用户中心</h1>
        <p class="subtitle">管理您的账户信息与安全设置</p>
      </div>
    </div>

    <!-- 基本信息 -->
    <t-card class="section-card" bordered>
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
              <t-tag v-if="user?.role === 'admin'" theme="primary" size="small">管理员</t-tag>
              <t-tag v-else theme="default" size="small">普通用户</t-tag>
            </div>
          </div>
          <div class="profile-info-item">
            <div class="profile-info-label">账户状态</div>
            <div class="profile-info-value">
              <t-tag v-if="user?.status === 'active'" theme="success" size="small">正常</t-tag>
              <t-tag v-else-if="user?.status === 'disabled'" theme="danger" size="small">已禁用</t-tag>
              <t-tag v-else theme="warning" size="small">{{ user?.status ?? "-" }}</t-tag>
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
              <span v-if="user?.last_login_location" style="color:#94a3b8;margin-left:6px">{{ user.last_login_location }}</span>
            </div>
          </div>
        </div>
      </div>
    </t-card>

    <!-- 修改密码 -->
    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="profile-section-head">
          <div class="profile-section-title">修改密码</div>
        </div>
        <t-form layout="vertical" label-align="top" :disabled="passwordSaving" class="password-form">
          <t-form-item label="当前密码">
            <t-input v-model="passwordForm.oldPassword" type="password" placeholder="请输入当前密码" />
          </t-form-item>
          <t-form-item label="新密码">
            <t-input v-model="passwordForm.newPassword" type="password" placeholder="请输入新密码（至少 8 位）" />
          </t-form-item>
          <t-form-item label="确认新密码">
            <t-input v-model="passwordForm.confirmPassword" type="password" placeholder="请再次输入新密码" />
          </t-form-item>
          <div class="profile-actions">
            <t-button theme="primary" :loading="passwordSaving" @click="handleChangePassword">修改密码</t-button>
          </div>
        </t-form>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { useAuthStore } from "@/stores/auth"
import { api, type User } from "@/lib/api"

const auth = useAuthStore()
const user = ref<User | null>(null)
const loading = ref(true)

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
    const res = await api.getMe()
    user.value = res.user
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载用户信息失败")
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

<style scoped>
.profile-loading {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

.profile-section-head {
  margin-bottom: 20px;
  padding-bottom: 12px;
  border-bottom: 1px solid #f1f5f9;
}

.profile-section-title {
  font-size: 16px;
  font-weight: 700;
  color: #0f172a;
}

.profile-info-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px 24px;
}

.profile-info-item {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 12px 14px;
  border-radius: 10px;
  background: #f8fafc;
  border: 1px solid #f1f5f9;
  transition: border-color 0.2s ease;
}

.profile-info-item:hover {
  border-color: #e2e8f0;
}

.profile-info-label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.profile-info-value {
  font-size: 14px;
  color: #0f172a;
  font-weight: 500;
  word-break: break-all;
}

.password-form {
  max-width: 420px;
}

.profile-actions {
  margin-top: 12px;
}

@media (max-width: 640px) {
  .profile-info-grid {
    grid-template-columns: 1fr;
    gap: 12px;
  }
}
</style>
