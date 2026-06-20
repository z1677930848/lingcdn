<template>
  <div class="auth-shell">
    <div class="auth-card auth-card--wide">
      <header class="auth-header">
        <div class="auth-brand-row">
          <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="auth-logo" />
          <div>
            <p class="auth-kicker">用户注册</p>
            <h1 class="auth-title">{{ brand.title || "LingCDN" }}</h1>
            <p class="auth-lead">创建新账号</p>
          </div>
        </div>
      </header>

      <ol v-if="registerEnabled" class="auth-steps">
        <li><span class="auth-step-num">1</span>填写基本信息</li>
        <li><span class="auth-step-num">2</span>{{ requireEmailVerification ? "验证邮箱" : "提交注册" }}</li>
        <li><span class="auth-step-num">3</span>登录控制台</li>
      </ol>

      <template v-if="registerEnabled">
        <form class="auth-form" @submit.prevent="handleSubmit">
          <div class="auth-field">
            <label class="auth-label">用户名</label>
            <el-input v-model="username" placeholder="请输入用户名" size="large" clearable :disabled="loading">
              <template #prefix><EpIcon name="user" /></template>
            </el-input>
          </div>

          <div class="auth-field">
            <label class="auth-label">邮箱</label>
            <el-input v-model="email" placeholder="请输入邮箱" type="email" size="large" clearable :disabled="loading">
              <template #prefix><EpIcon name="mail" /></template>
            </el-input>
          </div>

          <div class="auth-field">
            <label class="auth-label">密码</label>
            <el-input v-model="password" type="password" placeholder="至少 8 位" size="large" clearable :disabled="loading">
              <template #prefix><EpIcon name="lock-on" /></template>
            </el-input>
          </div>

          <div class="auth-field">
            <label class="auth-label">确认密码</label>
            <el-input v-model="confirmPassword" type="password" placeholder="再次输入密码" size="large" clearable :disabled="loading">
              <template #prefix><EpIcon name="lock-on" /></template>
            </el-input>
          </div>

          <div v-if="requireEmailVerification" class="auth-field">
            <label class="auth-label">邮箱验证码</label>
            <div class="code-row">
              <div class="code-input">
                <el-input v-model="emailCode" placeholder="请输入验证码" size="large" :disabled="loading || codeSending">
                  <template #prefix><EpIcon name="secured" /></template>
                </el-input>
              </div>
              <el-button
                plain
                size="large"
                :disabled="codeSending || countdown > 0"
                :loading="codeSending"
                @click.prevent="sendCode"
              >
                {{ countdown > 0 ? `${countdown}s` : "发送验证码" }}
              </el-button>
            </div>
          </div>

          <div v-if="requireCaptcha && captchaAvailable" class="auth-field">
            <label class="auth-label">图形验证码</label>
            <div class="captcha-row">
              <div class="captcha-input-wrap">
                <el-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4" :disabled="loading">
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
                :disabled="loading"
                @click.prevent="() => loadCaptcha()"
              >
                <EpIcon name="refresh" />
              </el-button>
            </div>
          </div>

          <el-button class="auth-submit" type="primary" native-type="submit" size="large" block :loading="loading">
            注册
          </el-button>
        </form>
      </template>

      <template v-else>
        <div class="auth-notice" role="alert">
          <EpIcon name="error-circle" size="16px" />
          <span>当前已关闭注册，请联系管理员开通。</span>
        </div>
      </template>

      <div class="auth-links">
        <span>已有账号？</span>
        <RouterLink to="/login" class="auth-link">立即登录</RouterLink>
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
import { computed, onMounted, onUnmounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"
import { api } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"
import { resolveRedirect } from "@/lib/resolveRedirect"
import { resolveAuthErrorMessage } from "@/lib/authErrors"
import { notifyAuthFailure } from "@/lib/authCaptchaFlow"

const CAPTCHA_REQUIRED_KEY = "register_captcha_required"

const router = useRouter()
const route = useRoute()
const auth = useAuthStore()
const { brand, footerLinks, footerCopyright, settings } = useSystemSettings()

const username = ref("")
const email = ref("")
const password = ref("")
const confirmPassword = ref("")
const emailCode = ref("")

const requireCaptcha = ref(false)
const captchaAvailable = ref(true)
const captchaId = ref("")
const captchaCode = ref("")
const captchaImage = ref("")

const loading = ref(false)
const codeSending = ref(false)
const countdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const registerEnabled = computed(() => Boolean(settings.value.register_enabled ?? true))
const requireEmailVerification = computed(() => Boolean(settings.value.register_email_verification ?? false))

const isCaptchaDisabled = (err: unknown) => {
  const message = err instanceof Error ? err.message : String(err ?? "")
  const lower = message.toLowerCase()
  return lower.includes("captcha disabled") || lower.includes("captcha not configured")
}

const loadCaptcha = async (silent = false) => {
  try {
    const data = await api.getCaptcha()
    captchaAvailable.value = true
    requireCaptcha.value = true
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
    requireCaptcha.value = true
    if (!silent) {
      notify({ variant: "destructive", title: "获取验证码失败" })
    }
  }
}

const startCountdown = () => {
  countdown.value = 60
  if (countdownTimer) {
    clearInterval(countdownTimer)
  }
  countdownTimer = setInterval(() => {
    countdown.value -= 1
    if (countdown.value <= 0) {
      countdown.value = 0
      if (countdownTimer) {
        clearInterval(countdownTimer)
        countdownTimer = null
      }
    }
  }, 1000)
}

const sendCode = async () => {
  const cleanEmail = email.value.trim().toLowerCase()
  if (!cleanEmail || !cleanEmail.includes("@")) {
    notify({ variant: "warning", title: "请输入有效邮箱" })
    return
  }
  if (requireCaptcha.value && captchaAvailable.value && (!captchaId.value || !captchaCode.value.trim())) {
    notify({ variant: "warning", title: "请输入验证码" })
    return
  }
  codeSending.value = true
  try {
    await api.requestRegisterEmailCode(
      cleanEmail,
      requireCaptcha.value && captchaAvailable.value ? { token: captchaId.value, answer: captchaCode.value.trim() } : undefined
    )
    notify({ variant: "success", title: "验证码已发送，请查收邮件" })
    startCountdown()
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
    if (requireCaptcha.value) {
      await loadCaptcha(true)
    }
  } catch (err) {
    const message = resolveAuthErrorMessage(err instanceof Error ? err.message : String(err ?? ""), "发送验证码失败")
    notify({ variant: "destructive", title: message })
    if (!requireCaptcha.value) {
      requireCaptcha.value = true
      localStorage.setItem(CAPTCHA_REQUIRED_KEY, "true")
    }
    await loadCaptcha()
  } finally {
    codeSending.value = false
  }
}

const handleSubmit = async () => {
  if (!registerEnabled.value) {
    notify({ variant: "warning", title: "系统已关闭注册" })
    return
  }

  const cleanUsername = username.value.trim()
  const cleanEmail = email.value.trim().toLowerCase()
  const cleanEmailCode = emailCode.value.trim()

  if (!cleanUsername || cleanUsername.length < 3) {
    notify({ variant: "warning", title: "用户名至少 3 位" })
    return
  }
  if (!cleanEmail || !cleanEmail.includes("@")) {
    notify({ variant: "warning", title: "请输入有效邮箱" })
    return
  }
  if (!password.value || password.value.length < 8) {
    notify({ variant: "warning", title: "密码长度不能少于8位" })
    return
  }
  if (password.value !== confirmPassword.value) {
    notify({ variant: "warning", title: "两次输入的密码不一致" })
    return
  }
  if (requireEmailVerification.value) {
    if (!cleanEmailCode) {
      notify({ variant: "warning", title: "请输入邮箱验证码" })
      return
    }
    if (cleanEmailCode.length !== 6) {
      notify({ variant: "warning", title: "请输入 6 位验证码" })
      return
    }
  }
  if (requireCaptcha.value && captchaAvailable.value && (!captchaId.value || !captchaCode.value.trim())) {
    notify({ variant: "warning", title: "请输入验证码" })
    return
  }

  loading.value = true
  try {
    await auth.register(
      cleanUsername,
      cleanEmail,
      password.value,
      requireEmailVerification.value ? cleanEmailCode : undefined,
      requireCaptcha.value && captchaAvailable.value ? { token: captchaId.value, answer: captchaCode.value.trim() } : undefined
    )
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
    await router.push(resolveRedirect("/dashboard", route.query.redirect))
  } catch (err: unknown) {
    await notifyAuthFailure({
      err,
      fallback: "注册失败，请检查填写内容",
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
  } else {
    loadCaptcha(true)
  }
})

onUnmounted(() => {
  if (countdownTimer) {
    clearInterval(countdownTimer)
    countdownTimer = null
  }
})
</script>
