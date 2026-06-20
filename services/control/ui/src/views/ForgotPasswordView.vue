<template>
  <div class="auth-shell">
    <div class="auth-card auth-card--wide">
      <header class="auth-header">
        <div class="auth-brand-row">
          <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="auth-logo" />
          <div>
            <p class="auth-kicker">密码找回</p>
            <h1 class="auth-title">{{ brand.title || "LingCDN" }}</h1>
            <p class="auth-lead">{{ step === "request" ? "输入注册邮箱获取验证码" : "设置新密码" }}</p>
          </div>
        </div>
      </header>

      <ol v-if="step === 'request'" class="auth-steps">
        <li><span class="auth-step-num">1</span>输入注册邮箱</li>
        <li><span class="auth-step-num">2</span>获取验证码</li>
        <li><span class="auth-step-num">3</span>设置新密码</li>
      </ol>

      <template v-if="step === 'request'">
        <form class="auth-form" @submit.prevent="handleRequest">
          <div class="auth-field">
            <label class="auth-label">邮箱地址</label>
            <el-input v-model="email" placeholder="请输入注册邮箱" type="email" size="large" clearable :disabled="sending">
              <template #prefix><EpIcon name="mail" /></template>
            </el-input>
          </div>

          <div v-if="requireCaptcha && captchaAvailable" class="auth-field">
            <label class="auth-label">图形验证码</label>
            <div class="captcha-row">
              <div class="captcha-input-wrap">
                <el-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4" :disabled="sending">
                  <template #prefix><EpIcon name="secured" /></template>
                </el-input>
              </div>
              <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
                <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" />
                <span v-else>{{ captchaImage }}</span>
              </div>
              <el-button plain size="large" class="captcha-refresh" :disabled="sending" @click.prevent="() => loadCaptcha()">
                <EpIcon name="refresh" />
              </el-button>
            </div>
          </div>

          <el-button class="auth-submit" type="primary" native-type="submit" size="large" block :loading="sending">
            获取验证码
          </el-button>
        </form>
      </template>

      <template v-else>
        <form class="auth-form" @submit.prevent="handleReset">
          <div class="auth-field">
            <label class="auth-label">邮箱地址</label>
            <el-input v-model="email" placeholder="邮箱地址" type="email" size="large" disabled>
              <template #prefix><EpIcon name="mail" /></template>
            </el-input>
          </div>

          <div class="auth-field">
            <label class="auth-label">验证码</label>
            <div class="code-row">
              <div class="code-input">
                <el-input v-model="resetCode" placeholder="6 位验证码" size="large" :maxlength="6" :disabled="resetting">
                  <template #prefix><EpIcon name="secured" /></template>
                </el-input>
              </div>
              <el-button
                plain
                size="large"
                :disabled="countdown > 0 || sending"
                :loading="sending"
                @click.prevent="handleRequest"
              >
                {{ countdown > 0 ? `${countdown}s` : "重新发送" }}
              </el-button>
            </div>
          </div>

          <div class="auth-field">
            <label class="auth-label">新密码</label>
            <el-input v-model="newPassword" type="password" placeholder="至少 8 位" size="large" clearable :disabled="resetting">
              <template #prefix><EpIcon name="lock-on" /></template>
            </el-input>
          </div>

          <div class="auth-field">
            <label class="auth-label">确认新密码</label>
            <el-input v-model="confirmPassword" type="password" placeholder="再次输入新密码" size="large" clearable :disabled="resetting">
              <template #prefix><EpIcon name="lock-on" /></template>
            </el-input>
          </div>

          <div class="auth-actions">
            <el-button plain size="large" :disabled="resetting" @click.prevent="handleBack">修改邮箱</el-button>
            <el-button class="auth-submit" type="primary" native-type="submit" size="large" :loading="resetting">重置密码</el-button>
          </div>
        </form>
      </template>

      <div class="auth-links">
        <RouterLink to="/login" class="auth-link">← 返回登录</RouterLink>
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
import { onMounted, onUnmounted, ref } from "vue"
import { useRouter } from "vue-router"
import { api, FEATURE_UNAVAILABLE_MESSAGE } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"

const CAPTCHA_REQUIRED_KEY = "forgot_password_captcha_required"

const router = useRouter()
const { brand, footerLinks, footerCopyright } = useSystemSettings()

const step = ref<"request" | "reset">("request")
const sending = ref(false)
const resetting = ref(false)
const countdown = ref(0)
let countdownTimer: ReturnType<typeof setInterval> | null = null

const email = ref("")
const resetCode = ref("")
const newPassword = ref("")
const confirmPassword = ref("")

const requireCaptcha = ref(false)
const captchaAvailable = ref(true)
const captchaId = ref("")
const captchaCode = ref("")
const captchaImage = ref("")

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

const handleRequest = async () => {
  const cleanEmail = email.value.trim().toLowerCase()
  if (!cleanEmail || !cleanEmail.includes("@")) {
    notify({ variant: "warning", title: "请输入有效邮箱" })
    return
  }
  if (requireCaptcha.value && captchaAvailable.value && (!captchaId.value || !captchaCode.value.trim())) {
    notify({ variant: "warning", title: "请输入验证码" })
    return
  }

  sending.value = true
  try {
    await api.requestPasswordReset(cleanEmail)
    step.value = "reset"
    startCountdown()
    notify({ variant: "success", title: "验证码已发送，请查收邮件" })
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
    if (requireCaptcha.value) {
      await loadCaptcha(true)
    }
  } catch (err) {
    const message = err instanceof Error ? err.message : FEATURE_UNAVAILABLE_MESSAGE
    notify({ variant: "warning", title: message })
    if (!requireCaptcha.value) {
      requireCaptcha.value = true
      localStorage.setItem(CAPTCHA_REQUIRED_KEY, "true")
    }
    await loadCaptcha()
  } finally {
    sending.value = false
  }
}

const handleReset = async () => {
  const cleanEmail = email.value.trim().toLowerCase()
  const cleanCode = resetCode.value.trim()
  if (!cleanEmail || !cleanEmail.includes("@")) {
    notify({ variant: "warning", title: "请输入有效邮箱" })
    return
  }
  if (cleanCode.length !== 6) {
    notify({ variant: "warning", title: "请输入 6 位验证码" })
    return
  }
  if (newPassword.value.length < 8) {
    notify({ variant: "warning", title: "密码长度不能少于8位" })
    return
  }
  if (newPassword.value !== confirmPassword.value) {
    notify({ variant: "warning", title: "两次输入的密码不一致" })
    return
  }

  resetting.value = true
  try {
    await api.confirmPasswordReset(cleanEmail, cleanCode, newPassword.value)
    notify({ variant: "success", title: "密码已更新，请重新登录" })
    await router.push("/login")
  } catch (err) {
    const message = err instanceof Error ? err.message : FEATURE_UNAVAILABLE_MESSAGE
    notify({ variant: "warning", title: message })
  } finally {
    resetting.value = false
  }
}

const handleBack = () => {
  step.value = "request"
  resetCode.value = ""
  newPassword.value = ""
  confirmPassword.value = ""
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
