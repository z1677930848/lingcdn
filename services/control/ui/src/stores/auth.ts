import { defineStore } from "pinia"
import { ref } from "vue"
import { api, type User, setAuthToken, clearAuthToken } from "@/lib/api"
import { notify } from "@/lib/notify"

export const useAuthStore = defineStore("auth", () => {
  const user = ref<User | null>(null)
  const loading = ref(true)
  const checked = ref(false)
  let pending: Promise<void> | null = null

  const checkAuth = async () => {
    if (checked.value) return
    if (!pending) {
      loading.value = true
      pending = (async () => {
        try {
          const { user: nextUser } = await api.getMe()
          user.value = nextUser
        } catch {
          user.value = null
        } finally {
          loading.value = false
          checked.value = true
          pending = null
        }
      })()
    }
    return pending
  }

  const login = async (identifier: string, password: string, captcha?: { token: string; answer: string }) => {
    const { user: nextUser, token } = await api.login(identifier, password, captcha)
    if (token) setAuthToken(token)
    user.value = nextUser
    notify({
      variant: "success",
      title: "登录成功",
      description: `欢迎回来，${nextUser.username}`,
    })
  }

  const adminLogin = async (identifier: string, password: string, captcha?: { token: string; answer: string }) => {
    const { user: nextUser, token } = await api.adminLogin(identifier, password, captcha)
    if (token) setAuthToken(token)
    user.value = nextUser
    notify({
      variant: "success",
      title: "管理员登录成功",
      description: `欢迎回来，${nextUser.username}`,
    })
  }

  const register = async (
    username: string,
    email: string,
    password: string,
    emailCode?: string,
    captcha?: { token: string; answer: string }
  ) => {
    const { user: nextUser, token } = await api.register(username, email, password, emailCode, captcha)
    if (token) setAuthToken(token)
    user.value = nextUser
    notify({
      variant: "success",
      title: "注册成功",
      description: `欢迎加入，${nextUser.username}`,
    })
  }

  const logout = async () => {
    user.value = null
    clearAuthToken()
    notify({
      variant: "default",
      title: "已退出登录",
      description: "已清理本地会话",
    })
  }

  return {
    user,
    loading,
    checkAuth,
    login,
    adminLogin,
    register,
    logout,
  }
})

