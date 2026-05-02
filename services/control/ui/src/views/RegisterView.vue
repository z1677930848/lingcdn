<template>
  <div class="register-page">
    <!-- Left hero panel -->
    <div class="register-hero">
      <div class="hero-orb hero-orb-1" />
      <div class="hero-orb hero-orb-2" />
      <div class="hero-bg">
        <svg class="hero-svg" viewBox="0 0 480 700" fill="none" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <linearGradient id="reg-flow" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stop-color="rgba(16,185,129,0)" />
              <stop offset="50%" stop-color="rgba(16,185,129,0.5)" />
              <stop offset="100%" stop-color="rgba(16,185,129,0)" />
            </linearGradient>
          </defs>
          <!-- Connections -->
          <line x1="120" y1="150" x2="260" y2="240" stroke="rgba(255,255,255,0.05)" stroke-width="1"/>
          <line x1="260" y1="240" x2="360" y2="190" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="260" y1="240" x2="320" y2="380" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="120" y1="150" x2="80" y2="320" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="80" y1="320" x2="200" y2="440" stroke="rgba(255,255,255,0.03)" stroke-width="1"/>
          <line x1="320" y1="380" x2="200" y2="440" stroke="rgba(255,255,255,0.03)" stroke-width="1"/>
          <!-- Flowing particle -->
          <circle r="2" fill="url(#reg-flow)">
            <animateMotion dur="5s" repeatCount="indefinite" path="M120,150 L260,240 L320,380 L200,440" />
          </circle>
          <!-- Nodes -->
          <circle cx="120" cy="150" r="3" fill="rgba(16,185,129,0.3)" />
          <circle cx="120" cy="150" r="7" fill="none" stroke="rgba(16,185,129,0.12)" stroke-width="1" class="node-ring nr1"/>
          <circle cx="260" cy="240" r="4" fill="rgba(16,185,129,0.45)" />
          <circle cx="260" cy="240" r="10" fill="none" stroke="rgba(16,185,129,0.1)" stroke-width="1" class="node-ring nr2"/>
          <circle cx="360" cy="190" r="2.5" fill="rgba(255,255,255,0.15)" />
          <circle cx="320" cy="380" r="3" fill="rgba(52,211,153,0.3)" />
          <circle cx="80" cy="320" r="3" fill="rgba(16,185,129,0.25)" />
          <circle cx="200" cy="440" r="3.5" fill="rgba(255,255,255,0.2)" />
          <circle cx="200" cy="440" r="8" fill="none" stroke="rgba(16,185,129,0.08)" stroke-width="1" class="node-ring nr3"/>
          <!-- User plus icon -->
          <circle cx="240" cy="520" r="32" fill="rgba(16,185,129,0.05)" stroke="rgba(16,185,129,0.12)" stroke-width="1" />
          <circle cx="240" cy="510" r="10" fill="none" stroke="rgba(16,185,129,0.25)" stroke-width="1.5" />
          <path d="M230 530 Q230 520 240 518 Q250 520 250 530" fill="none" stroke="rgba(16,185,129,0.25)" stroke-width="1.5" />
          <line x1="260" y1="508" x2="260" y2="518" stroke="rgba(16,185,129,0.3)" stroke-width="1.5" />
          <line x1="255" y1="513" x2="265" y2="513" stroke="rgba(16,185,129,0.3)" stroke-width="1.5" />
        </svg>
      </div>
      <div class="hero-content">
        <div class="hero-badge">新用户注册</div>
        <h2 class="hero-title">开启加速之旅</h2>
        <p class="hero-desc">注册账号，享受全球节点加速服务</p>
        <div class="hero-steps">
          <div class="hero-step">
            <span class="hero-step-num">1</span>
            <span>填写基本信息</span>
          </div>
          <div class="hero-step">
            <span class="hero-step-num">2</span>
            <span>验证邮箱</span>
          </div>
          <div class="hero-step">
            <span class="hero-step-num">3</span>
            <span>开始使用</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Right form panel -->
    <div class="register-form-side">
      <div class="form-container">
        <div class="form-card">
          <div class="form-header">
            <div class="brand-row">
              <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="brand-logo" />
              <div class="brand-info">
                <h1 class="brand-title">{{ brand.title || "LingCDN" }}</h1>
                <p class="form-subtitle">创建新账号</p>
              </div>
            </div>
          </div>

          <template v-if="registerEnabled">
            <form class="form" @submit.prevent="handleSubmit">
              <div class="field">
                <label class="field-label">用户名</label>
                <t-input v-model="username" placeholder="请输入用户名" size="large" clearable :disabled="loading">
                  <template #prefixIcon><t-icon name="user" /></template>
                </t-input>
              </div>

              <div class="field">
                <label class="field-label">邮箱</label>
                <t-input v-model="email" placeholder="请输入邮箱" type="email" size="large" clearable :disabled="loading">
                  <template #prefixIcon><t-icon name="mail" /></template>
                </t-input>
              </div>

              <div class="field">
                <label class="field-label">密码</label>
                <t-input v-model="password" type="password" placeholder="至少 8 位" size="large" clearable :disabled="loading">
                  <template #prefixIcon><t-icon name="lock-on" /></template>
                </t-input>
              </div>

              <div class="field">
                <label class="field-label">确认密码</label>
                <t-input v-model="confirmPassword" type="password" placeholder="再次输入密码" size="large" clearable :disabled="loading">
                  <template #prefixIcon><t-icon name="lock-on" /></template>
                </t-input>
              </div>

              <div v-if="requireEmailVerification" class="field">
                <label class="field-label">邮箱验证码</label>
                <div class="code-row">
                  <div class="code-input">
                    <t-input v-model="emailCode" placeholder="请输入验证码" size="large" :disabled="loading || codeSending">
                      <template #prefixIcon><t-icon name="secured" /></template>
                    </t-input>
                  </div>
                  <t-button
                    class="code-button"
                    variant="outline"
                    size="large"
                    :disabled="codeSending || countdown > 0"
                    :loading="codeSending"
                    @click.prevent="sendCode"
                  >
                    {{ countdown > 0 ? `${countdown}s` : "发送验证码" }}
                  </t-button>
                </div>
              </div>

              <div v-if="requireCaptcha && captchaAvailable" class="field">
                <label class="field-label">图形验证码</label>
                <div class="captcha-row">
                  <div class="captcha-input-wrap">
                    <t-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4" :disabled="loading">
                      <template #prefixIcon><t-icon name="image" /></template>
                    </t-input>
                  </div>
                  <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
                    <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" />
                    <span v-else>{{ captchaImage }}</span>
                  </div>
                  <t-button variant="outline" size="large" class="captcha-refresh" :disabled="loading" @click.prevent="() => loadCaptcha()">
                    <t-icon name="refresh" />
                  </t-button>
                </div>
              </div>

              <t-button class="submit-button" theme="primary" type="submit" size="large" block :loading="loading">
                注册
              </t-button>
            </form>
          </template>

          <template v-else>
            <div class="disabled-notice" role="alert">
              当前已关闭注册，请联系管理员开通。
              <div style="margin-top: 12px">
                <RouterLink to="/login">返回登录</RouterLink>
              </div>
            </div>
          </template>

          <div class="footer">
            <span>已有账号？</span>
            <RouterLink to="/login" class="footer-link">立即登录</RouterLink>
          </div>
        </div>

        <div class="copyright">
          <div>{{ footerCopyright }}</div>
          <div v-if="footerLinks.length" class="footer-links">
            <a v-for="(link, idx) in footerLinks" :key="`${link.href}-${idx}`" :href="link.href" target="_blank" rel="noreferrer">
              {{ link.label }}
            </a>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, ref } from "vue"
import { useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"
import { api } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"

const CAPTCHA_REQUIRED_KEY = "register_captcha_required"

const router = useRouter()
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
    const message = err instanceof Error ? err.message : "发送验证码失败"
    notify({ variant: "destructive", title: "发送验证码失败", description: message })
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
    await router.push("/dashboard")
  } catch (err) {
    const message = err instanceof Error ? err.message : "注册失败"
    notify({ variant: "destructive", title: "注册失败", description: message })
    if (!requireCaptcha.value) {
      requireCaptcha.value = true
      localStorage.setItem(CAPTCHA_REQUIRED_KEY, "true")
    }
    await loadCaptcha()
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

<style scoped>
.register-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  width: 100%;
  background: #f0f2f7;
}

/* ---- Left hero panel ---- */
.register-hero {
  flex: 0 0 42%;
  max-width: 540px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: linear-gradient(160deg, #020617 0%, #0f172a 35%, #064e3b 100%);
}

.hero-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.hero-orb-1 {
  width: 260px; height: 260px;
  background: rgba(16, 185, 129, 0.1);
  top: -40px; left: -60px;
  animation: float-orb 12s ease-in-out infinite;
}
.hero-orb-2 {
  width: 180px; height: 180px;
  background: rgba(52, 211, 153, 0.08);
  bottom: 15%; right: -40px;
  animation: float-orb 14s ease-in-out infinite reverse;
}

@keyframes float-orb {
  0%, 100% { transform: translate(0, 0) scale(1); }
  33% { transform: translate(12px, -16px) scale(1.04); }
  66% { transform: translate(-8px, 12px) scale(0.96); }
}

.hero-bg {
  position: absolute;
  inset: 0;
  opacity: 0.8;
}

.hero-svg {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.node-ring {
  animation: ring-expand 4s ease-out infinite;
}
.node-ring.nr1 { animation-delay: 0s; }
.node-ring.nr2 { animation-delay: 1.3s; }
.node-ring.nr3 { animation-delay: 2.6s; }

@keyframes ring-expand {
  0% { opacity: 0.5; }
  50% { opacity: 0.15; }
  100% { opacity: 0; }
}

.hero-content {
  position: relative;
  z-index: 1;
  padding: 48px 36px;
  color: #fff;
  max-width: 380px;
  animation: hero-enter 0.8s ease-out both;
}

@keyframes hero-enter {
  from { opacity: 0; transform: translateY(24px); }
  to { opacity: 1; transform: translateY(0); }
}

.hero-badge {
  display: inline-block;
  padding: 5px 16px;
  border-radius: 20px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 1.5px;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  color: #34d399;
  margin-bottom: 20px;
  backdrop-filter: blur(8px);
}

.hero-title {
  font-size: 32px;
  font-weight: 800;
  line-height: 1.2;
  margin: 0 0 12px;
  background: linear-gradient(135deg, #fff 0%, #6ee7b7 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-desc {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.4);
  margin: 0 0 36px;
  letter-spacing: 0.5px;
}

.hero-steps {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.hero-step {
  display: flex;
  align-items: center;
  gap: 14px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.55);
}

.hero-step-num {
  width: 28px;
  height: 28px;
  border-radius: 50%;
  background: rgba(16, 185, 129, 0.1);
  border: 1px solid rgba(16, 185, 129, 0.2);
  color: #34d399;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

/* ---- Right form panel ---- */
.register-form-side {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 40px;
  min-height: 100vh;
  min-height: 100dvh;
  background: #f0f2f7;
  overflow-y: auto;
}

.form-container {
  width: 100%;
  max-width: 440px;
  animation: form-enter 0.6s ease-out 0.2s both;
}

@keyframes form-enter {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}

.form-card {
  background: #fff;
  border-radius: 16px;
  padding: 32px 28px 24px;
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.04),
    0 6px 24px rgba(0, 0, 0, 0.06);
}

.form-header {
  margin-bottom: 24px;
}

.brand-row {
  display: flex;
  align-items: center;
  gap: 14px;
}

.brand-logo {
  width: 42px;
  height: 42px;
  border-radius: 12px;
  object-fit: cover;
  border: 1px solid #e5e7eb;
  background: #fff;
  flex-shrink: 0;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.06);
}

.brand-info {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.brand-title {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  margin: 0;
  line-height: 1.2;
}

.form-subtitle {
  font-size: 13px;
  color: #94a3b8;
  margin: 0;
}

.form {
  margin-top: 0;
}

.field {
  margin-bottom: 16px;
}

.field:last-of-type {
  margin-bottom: 0;
}

.field-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  margin-bottom: 5px;
  letter-spacing: 0.3px;
}

.field :deep(.t-input),
.field :deep(.t-input__wrap) {
  width: 100%;
}

.field :deep(.t-input.t-is-focused) {
  border-color: #10b981;
  box-shadow: 0 0 0 3px rgba(16, 185, 129, 0.18);
}

.field :deep(.t-input__prefix) {
  color: var(--app-text-faint);
}

.code-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.code-input {
  flex: 1;
  min-width: 0;
}

.code-input :deep(.t-input),
.code-input :deep(.t-input__wrap) {
  width: 100%;
}

.code-button {
  flex-shrink: 0;
  min-width: 110px;
}

.captcha-row {
  display: flex;
  align-items: center;
  gap: 10px;
}

.captcha-input-wrap {
  flex: 1;
  min-width: 0;
}

.captcha-input-wrap :deep(.t-input),
.captcha-input-wrap :deep(.t-input__wrap) {
  width: 100%;
}

.captcha-box {
  height: 40px;
  min-width: 104px;
  padding: 0;
  border-radius: 10px;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  display: flex;
  align-items: center;
  justify-content: center;
  user-select: none;
  cursor: pointer;
  overflow: hidden;
  transition: all 0.2s;
}

.captcha-box:hover {
  border-color: #94a3b8;
  transform: scale(1.02);
}

.captcha-box img {
  display: block;
  width: 120px;
  height: 40px;
}

.captcha-refresh {
  flex-shrink: 0;
}

.submit-button {
  margin-top: 20px;
  height: 46px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.5px;
  transition: all 0.2s ease;
}

.form-card :deep(.submit-button.t-button--theme-primary) {
  background: linear-gradient(135deg, #10b981 0%, #059669 100%);
  border: none;
}

.form-card :deep(.submit-button.t-button--theme-primary:hover) {
  background: linear-gradient(135deg, #059669 0%, #047857 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px rgba(5, 150, 105, 0.3);
}

.form-card :deep(.submit-button.t-button--theme-primary:active) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(5, 150, 105, 0.2);
}

.disabled-notice {
  text-align: center;
  padding: 24px 0;
  color: #64748b;
  font-size: 14px;
}

.disabled-notice a {
  color: #10b981;
  text-decoration: none;
  font-weight: 500;
}

.footer {
  margin-top: 20px;
  text-align: center;
  color: #94a3b8;
  font-size: 13px;
}

.footer-link {
  color: #10b981;
  text-decoration: none;
  font-weight: 600;
  transition: color 0.2s;
}

.footer-link:hover {
  color: #059669;
}

.copyright {
  margin-top: 20px;
  text-align: center;
  color: #94a3b8;
  font-size: 12px;
}

.footer-links {
  margin-top: 6px;
  display: flex;
  gap: 12px;
  justify-content: center;
  flex-wrap: wrap;
}

.footer-links a {
  color: #94a3b8;
  text-decoration: none;
  transition: color 0.2s;
}

.footer-links a:hover {
  color: #10b981;
}

/* TDesign overrides — input/button radius now comes from tokens.css. */
.form-card :deep(.t-input),
.form-card :deep(.t-input__wrap) {
  transition: all 0.2s ease;
}

/* ---- Mobile ---- */
@media (max-width: 900px) {
  .register-hero {
    display: none;
  }

  .register-page {
    background: linear-gradient(160deg, #020617 0%, #0f172a 40%, #064e3b 100%);
    justify-content: center;
    align-items: center;
  }

  .register-form-side {
    padding: 24px 16px;
    min-height: auto;
    background: transparent;
  }

  .form-card {
    box-shadow: 0 20px 60px rgba(0, 0, 0, 0.25);
  }

  .copyright {
    color: rgba(255, 255, 255, 0.35);
  }

  .footer-links a {
    color: rgba(255, 255, 255, 0.35);
  }
}

@media (max-width: 480px) {
  .register-form-side {
    padding: 16px 12px;
    align-items: flex-start;
    padding-top: max(24px, 6vh);
  }

  .form-card {
    padding: 24px 20px 20px;
    border-radius: 14px;
  }

  .brand-title {
    font-size: 20px;
  }

  .brand-logo {
    width: 36px;
    height: 36px;
  }

  .field {
    margin-bottom: 12px;
  }

  .code-row {
    flex-wrap: wrap;
  }

  .code-input {
    flex: 1 1 100%;
  }

  .code-button {
    flex: 1 1 100%;
  }

  .captcha-row {
    flex-wrap: wrap;
    row-gap: 8px;
  }

  .captcha-input-wrap {
    flex: 1 1 100%;
  }

  .submit-button {
    margin-top: 14px;
    height: 44px;
  }
}
</style>

