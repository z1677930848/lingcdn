<template>
  <div class="auth-shell">
    <div class="auth-card">
      <header class="auth-header">
        <div class="auth-brand-row">
          <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="auth-logo" />
          <div>
            <p class="auth-kicker">用户控制台</p>
            <h1 class="auth-title">{{ brand.title || "LingCDN" }}</h1>
            <p class="auth-lead">登录到您的账户</p>
          </div>
        </div>
      </header>

      <div v-if="licenseNotice" class="auth-notice" role="alert">
        <EpIcon name="error-circle" size="16px" />
        <span>{{ licenseNotice }}</span>
      </div>

      <form class="auth-form" @submit.prevent="handleSubmit">
        <div class="auth-field">
          <label class="auth-label">用户名 / 邮箱</label>
          <el-input
            v-model="identifier"
            placeholder="请输入用户名或邮箱"
            :disabled="loading"
            size="large"
            clearable
          >
            <template #prefix><EpIcon name="user" /></template>
          </el-input>
        </div>

        <div class="auth-field">
          <label class="auth-label">密码</label>
          <el-input
            v-model="password"
            type="password"
            placeholder="请输入密码"
            :disabled="loading"
            size="large"
            clearable
          >
            <template #prefix><EpIcon name="lock-on" /></template>
          </el-input>
        </div>

        <div v-if="requireCaptcha && captchaAvailable" class="auth-field">
          <label class="auth-label">图形验证码</label>
          <div class="captcha-row">
            <div class="captcha-input-wrap">
              <el-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4">
                <template #prefix><EpIcon name="secured" /></template>
              </el-input>
            </div>
            <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
              <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" />
              <span v-else>{{ captchaImage }}</span>
            </div>
            <el-button
              plain
              size="large"
              class="captcha-refresh"
              aria-label="刷新验证码"
              @click.prevent="() => loadCaptcha()"
            >
              <EpIcon name="refresh" />
            </el-button>
          </div>
        </div>

        <el-button class="auth-submit" block type="primary" native-type="submit" size="large" :loading="loading">
          登录
        </el-button>
      </form>

      <div class="auth-links">
        <template v-if="registerEnabled">
          <RouterLink to="/register" class="auth-link">立即注册</RouterLink>
          <span class="auth-dot">·</span>
        </template>
        <RouterLink to="/forgot-password" class="auth-link">忘记密码？</RouterLink>
      </div>
    </div>

    <footer class="auth-foot">
      <div>{{ footerCopyright }}</div>
      <div v-if="footerLinks.length" class="auth-foot-links">
        <a v-for="(link, idx) in footerLinks" :key="`${link.href}-${idx}`" :href="link.href" target="_blank" rel="noreferrer">
          {{ link.label }}
        </a>
      </div>
    </footer>
  </div>
</template>

<script setup lang="ts">
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, onMounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"
import { api } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"
import { resolveRedirect } from "@/lib/resolveRedirect"
import { notifyAuthFailure } from "@/lib/authCaptchaFlow"

const CAPTCHA_REQUIRED_KEY = "login_captcha_required"

const auth = useAuthStore()
const router = useRouter()
const route = useRoute()
const { brand, footerLinks, footerCopyright, settings } = useSystemSettings()

const identifier = ref("")
const password = ref("")
const captchaCode = ref("")
const captchaId = ref("")
const captchaImage = ref("")
const requireCaptcha = ref(false)
const loading = ref(false)
const licenseNotice = ref("")

const registerEnabled = computed(() => Boolean(settings.value.register_enabled ?? true))

const isCaptchaDisabled = (err: unknown) => {
  const message = err instanceof Error ? err.message : String(err ?? "")
  const lower = message.toLowerCase()
  return lower.includes("captcha disabled") || lower.includes("captcha not configured")
}

const resolveLicenseNotice = (status: string, reason?: string) => {
  const st = String(status || "").toLowerCase()
  if (!st || st === "active") return ""

  const raw = String(reason || "").trim().toLowerCase()

  // 后端 reason 包含 "not found"/"no license"/"license required" 时统一提示未授权
  if (raw.includes("not found") || raw.includes("no license") || raw.includes("license required")) {
    return "系统尚未授权，部分功能可能受限"
  }

  // 解析节点超限的具体数量
  const nodeMatch = raw.match(/node limit\D+\((\d+)\/(\d+)\)/)
  if (nodeMatch) {
    return `授权节点数已超限（${nodeMatch[1]}/${nodeMatch[2]}），请联系管理员`
  }

  const statusMap: Record<string, string> = {
    unlicensed: "系统尚未授权，部分功能可能受限",
    expired: "授权已过期，请联系管理员续期",
    revoked: "授权已被吊销，请联系管理员",
    limited: "授权节点数已超限，请联系管理员",
    paused: "授权已暂停，请联系管理员",
    suspended: "授权已暂停，请联系管理员",
  }

  return statusMap[st] || "授权状态异常，请联系管理员"
}

const captchaAvailable = ref(true) // whether the backend supports captcha

const loadCaptcha = async (silent = false) => {
  try {
    const data = await api.getCaptcha()
    captchaAvailable.value = true
    captchaId.value = data.token
    captchaImage.value = data.question
    captchaCode.value = ""
  } catch (err) {
    if (isCaptchaDisabled(err)) {
      captchaAvailable.value = false
      requireCaptcha.value = false
      captchaId.value = ""
      captchaImage.value = ""
      captchaCode.value = ""
      localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
      return
    }
    if (!silent) {
      notify({ variant: "destructive", title: "获取验证码失败" })
    }
  }
}

const handleSubmit = async () => {
  if (requireCaptcha.value && captchaAvailable.value && (!captchaCode.value || !captchaId.value)) {
    notify({ variant: "warning", title: "请输入验证码" })
    return
  }
  const cleanIdentifier = identifier.value.trim()
  if (!cleanIdentifier) {
    notify({ variant: "warning", title: "请输入用户名或邮箱" })
    return
  }
  if (!password.value) {
    notify({ variant: "warning", title: "请输入密码" })
    return
  }

  loading.value = true
  try {
    await auth.login(cleanIdentifier, password.value, requireCaptcha.value && captchaAvailable.value ? { token: captchaId.value, answer: captchaCode.value.trim() } : undefined)
    await router.push(resolveRedirect("/dashboard", route.query.redirect))
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
  } catch (err: unknown) {
    await notifyAuthFailure({
      err,
      fallback: "登录失败，请检查凭据",
      requireCaptcha,
      captchaStorageKey: CAPTCHA_REQUIRED_KEY,
      loadCaptcha,
      notify,
    })
  } finally {
    loading.value = false
  }
}

onMounted(() => {
  const needsCaptcha = localStorage.getItem(CAPTCHA_REQUIRED_KEY) === "true"
  if (needsCaptcha) {
    requireCaptcha.value = true
    loadCaptcha()
  }

  api
    .getPublicLicenseStatus()
    .then((res) => {
      licenseNotice.value = resolveLicenseNotice(res.status, res.reason)
    })
    .catch(() => {})
})
</script>
