<template>
  <div class="forgot-page">
    <!-- Left hero panel -->
    <div class="forgot-hero">
      <div class="hero-orb hero-orb-1" />
      <div class="hero-orb hero-orb-2" />
      <div class="hero-bg">
        <svg class="hero-svg" viewBox="0 0 480 700" fill="none" xmlns="http://www.w3.org/2000/svg">
          <!-- Subtle grid -->
          <defs>
            <pattern id="forgot-grid" width="48" height="48" patternUnits="userSpaceOnUse">
              <path d="M 48 0 L 0 0 0 48" fill="none" stroke="rgba(255,255,255,0.025)" stroke-width="1"/>
            </pattern>
          </defs>
          <rect width="480" height="700" fill="url(#forgot-grid)" />
          <!-- Key icon -->
          <circle cx="240" cy="300" r="40" fill="rgba(6,182,212,0.05)" stroke="rgba(6,182,212,0.12)" stroke-width="1" />
          <circle cx="240" cy="300" r="70" fill="none" stroke="rgba(6,182,212,0.06)" stroke-width="1" class="key-ring kr1"/>
          <circle cx="240" cy="300" r="100" fill="none" stroke="rgba(6,182,212,0.03)" stroke-width="1" class="key-ring kr2"/>
          <!-- Key shape -->
          <circle cx="232" cy="290" r="12" fill="none" stroke="rgba(6,182,212,0.3)" stroke-width="2" />
          <line x1="244" y1="290" x2="268" y2="290" stroke="rgba(6,182,212,0.3)" stroke-width="2" />
          <line x1="260" y1="290" x2="260" y2="300" stroke="rgba(6,182,212,0.3)" stroke-width="2" />
          <line x1="268" y1="290" x2="268" y2="300" stroke="rgba(6,182,212,0.3)" stroke-width="2" />
        </svg>
      </div>
      <div class="hero-content">
        <div class="hero-badge">密码找回</div>
        <h2 class="hero-title">重置密码</h2>
        <p class="hero-desc">通过邮箱验证码安全地重置您的密码</p>
        <div class="hero-steps">
          <div class="hero-step">
            <span class="hero-step-num">1</span>
            <span>输入注册邮箱</span>
          </div>
          <div class="hero-step">
            <span class="hero-step-num">2</span>
            <span>获取验证码</span>
          </div>
          <div class="hero-step">
            <span class="hero-step-num">3</span>
            <span>设置新密码</span>
          </div>
        </div>
      </div>
    </div>

    <!-- Right form panel -->
    <div class="forgot-form-side">
      <div class="form-container">
        <div class="form-card">
          <div class="form-header">
            <div class="brand-row">
              <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="brand-logo" />
              <div class="brand-info">
                <h1 class="brand-title">{{ brand.title || "LingCDN" }}</h1>
                <p class="form-subtitle">找回密码</p>
              </div>
            </div>
          </div>

          <template v-if="step === 'request'">
            <form class="form" @submit.prevent="handleRequest">
              <div class="field">
                <label class="field-label">邮箱地址</label>
                <t-input v-model="email" placeholder="请输入注册邮箱" type="email" size="large" clearable :disabled="sending">
                  <template #prefixIcon><t-icon name="mail" /></template>
                </t-input>
              </div>

              <div v-if="requireCaptcha && captchaAvailable" class="field">
                <label class="field-label">图形验证码</label>
                <div class="captcha-row">
                  <div class="captcha-input-wrap">
                    <t-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4" :disabled="sending">
                      <template #prefixIcon><t-icon name="image" /></template>
                    </t-input>
                  </div>
                  <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
                    <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" />
                    <span v-else>{{ captchaImage }}</span>
                  </div>
                  <t-button variant="outline" size="large" class="captcha-refresh" :disabled="sending" @click.prevent="() => loadCaptcha()">
                    <t-icon name="refresh" />
                  </t-button>
                </div>
              </div>

              <t-button class="submit-button" theme="primary" type="submit" size="large" block :loading="sending">获取验证码</t-button>
            </form>
          </template>

          <template v-else>
            <form class="form" @submit.prevent="handleReset">
              <div class="field">
                <label class="field-label">邮箱地址</label>
                <t-input v-model="email" placeholder="邮箱地址" type="email" size="large" disabled>
                  <template #prefixIcon><t-icon name="mail" /></template>
                </t-input>
              </div>

              <div class="field">
                <label class="field-label">验证码</label>
                <div class="code-row">
                  <div class="code-input">
                    <t-input v-model="resetCode" placeholder="6位验证码" size="large" :maxlength="6" :disabled="resetting">
                      <template #prefixIcon><t-icon name="secured" /></template>
                    </t-input>
                  </div>
                  <t-button
                    variant="outline"
                    size="large"
                    :disabled="countdown > 0 || sending"
                    :loading="sending"
                    @click.prevent="handleRequest"
                  >
                    {{ countdown > 0 ? `${countdown}s` : "重新发送" }}
                  </t-button>
                </div>
              </div>

              <div class="field">
                <label class="field-label">新密码</label>
                <t-input v-model="newPassword" type="password" placeholder="至少8位" size="large" clearable :disabled="resetting">
                  <template #prefixIcon><t-icon name="lock-on" /></template>
                </t-input>
              </div>

              <div class="field">
                <label class="field-label">确认新密码</label>
                <t-input v-model="confirmPassword" type="password" placeholder="再次输入新密码" size="large" clearable :disabled="resetting">
                  <template #prefixIcon><t-icon name="lock-on" /></template>
                </t-input>
              </div>

              <div class="actions">
                <t-button variant="outline" size="large" :disabled="resetting" @click.prevent="handleBack">修改邮箱</t-button>
                <t-button class="submit-button" theme="primary" type="submit" size="large" :loading="resetting">重置密码</t-button>
              </div>
            </form>
          </template>

          <div class="footer">
            <RouterLink to="/login" class="footer-link">← 返回登录</RouterLink>
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

<style scoped>
.forgot-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  width: 100%;
  background: #f0f2f7;
}

/* ---- Left hero panel ---- */
.forgot-hero {
  flex: 0 0 42%;
  max-width: 540px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: linear-gradient(160deg, #020617 0%, #0f172a 35%, #164e63 100%);
}

.hero-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.hero-orb-1 {
  width: 260px; height: 260px;
  background: rgba(6, 182, 212, 0.08);
  top: -40px; right: -60px;
  animation: float-orb 13s ease-in-out infinite;
}
.hero-orb-2 {
  width: 180px; height: 180px;
  background: rgba(14, 165, 233, 0.06);
  bottom: 15%; left: -40px;
  animation: float-orb 11s ease-in-out infinite reverse;
}

@keyframes float-orb {
  0%, 100% { transform: translate(0, 0) scale(1); }
  33% { transform: translate(10px, -14px) scale(1.03); }
  66% { transform: translate(-8px, 10px) scale(0.97); }
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

.key-ring {
  animation: ring-pulse 5s ease-in-out infinite;
}
.key-ring.kr1 { animation-delay: 0s; }
.key-ring.kr2 { animation-delay: 2.5s; }

@keyframes ring-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
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
  background: rgba(6, 182, 212, 0.1);
  border: 1px solid rgba(6, 182, 212, 0.2);
  color: #67e8f9;
  margin-bottom: 20px;
  backdrop-filter: blur(8px);
}

.hero-title {
  font-size: 32px;
  font-weight: 800;
  line-height: 1.2;
  margin: 0 0 12px;
  background: linear-gradient(135deg, #fff 0%, #67e8f9 100%);
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
  background: rgba(6, 182, 212, 0.1);
  border: 1px solid rgba(6, 182, 212, 0.2);
  color: #67e8f9;
  font-size: 12px;
  font-weight: 700;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

/* ---- Right form panel ---- */
.forgot-form-side {
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
  max-width: 420px;
  animation: form-enter 0.6s ease-out 0.2s both;
}

@keyframes form-enter {
  from { opacity: 0; transform: translateY(16px); }
  to { opacity: 1; transform: translateY(0); }
}

.form-card {
  background: #fff;
  border-radius: 16px;
  padding: 36px 32px 28px;
  box-shadow:
    0 1px 3px rgba(0, 0, 0, 0.04),
    0 6px 24px rgba(0, 0, 0, 0.06);
}

.form-header {
  margin-bottom: 28px;
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
  margin-bottom: 18px;
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
  border-color: #06b6d4;
  box-shadow: 0 0 0 3px rgba(6, 182, 212, 0.18);
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

.actions {
  display: flex;
  gap: 12px;
  margin-top: 20px;
}

.actions .t-button {
  flex: 1;
}

.submit-button {
  margin-top: 20px;
  height: 46px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.5px;
  transition: all 0.2s ease;
}

.actions .submit-button {
  margin-top: 0;
}

.form-card :deep(.submit-button.t-button--theme-primary) {
  background: linear-gradient(135deg, #06b6d4 0%, #0891b2 100%);
  border: none;
}

.form-card :deep(.submit-button.t-button--theme-primary:hover) {
  background: linear-gradient(135deg, #0891b2 0%, #0e7490 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px rgba(8, 145, 178, 0.3);
}

.form-card :deep(.submit-button.t-button--theme-primary:active) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(8, 145, 178, 0.2);
}

.footer {
  margin-top: 22px;
  text-align: center;
  font-size: 13px;
}

.footer-link {
  color: #64748b;
  text-decoration: none;
  font-weight: 500;
  transition: color 0.2s;
}

.footer-link:hover {
  color: #334155;
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
  color: #06b6d4;
}

/* TDesign overrides — input/button radius now comes from tokens.css. */
.form-card :deep(.t-input),
.form-card :deep(.t-input__wrap) {
  transition: all 0.2s ease;
}

/* ---- Mobile ---- */
@media (max-width: 900px) {
  .forgot-hero {
    display: none;
  }

  .forgot-page {
    background: linear-gradient(160deg, #020617 0%, #0f172a 40%, #164e63 100%);
    justify-content: center;
    align-items: center;
  }

  .forgot-form-side {
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
  .forgot-form-side {
    padding: 16px 12px;
    align-items: flex-start;
    padding-top: max(30px, 8vh);
  }

  .form-card {
    padding: 28px 22px 24px;
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
    margin-bottom: 14px;
  }

  .code-row {
    flex-direction: column;
  }

  .code-input {
    width: 100%;
  }

  .actions {
    flex-direction: column;
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

