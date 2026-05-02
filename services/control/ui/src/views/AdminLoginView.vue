<template>
  <div class="admin-login-page">
    <!-- Left branded panel -->
    <div class="login-hero">
      <!-- Gradient orbs -->
      <div class="hero-orb hero-orb-1" />
      <div class="hero-orb hero-orb-2" />

      <div class="hero-bg">
        <svg class="hero-grid" viewBox="0 0 480 700" fill="none" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <pattern id="admin-grid" width="40" height="40" patternUnits="userSpaceOnUse">
              <path d="M 40 0 L 0 0 0 40" fill="none" stroke="rgba(255,255,255,0.03)" stroke-width="1"/>
            </pattern>
            <radialGradient id="radar-glow" cx="50%" cy="50%" r="50%">
              <stop offset="0%" stop-color="rgba(251,191,36,0.06)" />
              <stop offset="100%" stop-color="rgba(251,191,36,0)" />
            </radialGradient>
          </defs>
          <rect width="480" height="700" fill="url(#admin-grid)" />
          <!-- Radar circles -->
          <circle cx="240" cy="320" r="80" stroke="rgba(251,191,36,0.1)" stroke-width="1" fill="none" class="ring r1"/>
          <circle cx="240" cy="320" r="140" stroke="rgba(251,191,36,0.06)" stroke-width="1" fill="none" class="ring r2"/>
          <circle cx="240" cy="320" r="200" stroke="rgba(251,191,36,0.03)" stroke-width="1" fill="none" class="ring r3"/>
          <circle cx="240" cy="320" r="260" stroke="rgba(251,191,36,0.02)" stroke-width="1" fill="none" />
          <!-- Radar sweep -->
          <path d="M240,320 L240,60" stroke="rgba(251,191,36,0.15)" stroke-width="1" class="radar-sweep" />
          <!-- Radar glow center -->
          <circle cx="240" cy="320" r="60" fill="url(#radar-glow)" />
          <!-- Shield -->
          <path d="M240 280 L215 296 L215 322 Q215 344 240 356 Q265 344 265 322 L265 296 Z"
                fill="rgba(251,191,36,0.06)" stroke="rgba(251,191,36,0.18)" stroke-width="1.5" class="shield"/>
          <path d="M232 315 L238 322 L250 306" stroke="rgba(251,191,36,0.4)" stroke-width="2" fill="none" stroke-linecap="round" stroke-linejoin="round"/>
          <!-- Dot nodes on rings -->
          <circle cx="320" cy="320" r="3" fill="rgba(251,191,36,0.2)" class="radar-dot rd1"/>
          <circle cx="180" cy="220" r="2.5" fill="rgba(251,191,36,0.15)" class="radar-dot rd2"/>
          <circle cx="340" cy="200" r="2" fill="rgba(251,191,36,0.12)" class="radar-dot rd3"/>
          <circle cx="140" cy="380" r="2.5" fill="rgba(251,191,36,0.1)" class="radar-dot rd4"/>
        </svg>
      </div>

      <div class="hero-content">
        <div class="hero-badge">ADMIN</div>
        <h2 class="hero-title">管理控制台</h2>
        <p class="hero-desc">系统管理 · 节点运维 · 安全配置</p>

        <div class="hero-feature-card">
          <div class="hero-feature-item">
            <span class="hero-feature-icon">🖥️</span>
            <div class="hero-feature-text">
              <span class="hero-feature-name">节点管理</span>
              <span class="hero-feature-sub">实时监控与运维</span>
            </div>
          </div>
          <div class="hero-feature-item">
            <span class="hero-feature-icon">🔒</span>
            <div class="hero-feature-text">
              <span class="hero-feature-name">安全防护</span>
              <span class="hero-feature-sub">WAF / DDoS / CC</span>
            </div>
          </div>
          <div class="hero-feature-item">
            <span class="hero-feature-icon">🌐</span>
            <div class="hero-feature-text">
              <span class="hero-feature-name">域名管理</span>
              <span class="hero-feature-sub">证书与 DNS 配置</span>
            </div>
          </div>
          <div class="hero-feature-item">
            <span class="hero-feature-icon">⚙️</span>
            <div class="hero-feature-text">
              <span class="hero-feature-name">系统升级</span>
              <span class="hero-feature-sub">一键升级与配置</span>
            </div>
          </div>
        </div>
      </div>
    </div>

    <!-- Right form panel -->
    <div class="login-form-side">
      <div class="form-container">
        <div class="form-card">
          <div class="form-header">
            <div class="brand-row">
              <img v-if="brand.logo" :src="brand.logo" :alt="brand.title" class="brand-logo" />
              <div class="brand-info">
                <h1 class="brand-title">{{ brand.title || "LingCDN" }}</h1>
                <p class="form-subtitle">管理员登录</p>
              </div>
            </div>
          </div>

          <form class="form" @submit.prevent="handleSubmit">
            <div class="field">
              <label class="field-label">管理员账号</label>
              <t-input
                v-model="identifier"
                placeholder="请输入管理员账号"
                :disabled="loading"
                size="large"
                clearable
              >
                <template #prefixIcon><t-icon name="user" /></template>
              </t-input>
            </div>
            <div class="field">
              <label class="field-label">密码</label>
              <t-input
                v-model="password"
                type="password"
                placeholder="请输入密码"
                :disabled="loading"
                size="large"
                clearable
              >
                <template #prefixIcon><t-icon name="lock-on" /></template>
              </t-input>
            </div>

            <div v-if="requireCaptcha && captchaAvailable" class="field">
              <label class="field-label">验证码</label>
              <div class="captcha-row">
                <div class="captcha-input-wrap">
                  <t-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4">
                    <template #prefixIcon><t-icon name="secured" /></template>
                  </t-input>
                </div>
                <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
                  <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" class="captcha-image" />
                  <span v-else>{{ captchaImage }}</span>
                </div>
                <t-button variant="outline" size="large" class="captcha-refresh" @click.prevent="() => loadCaptcha()">
                  <t-icon name="refresh" />
                </t-button>
              </div>
            </div>

            <t-button class="submit-button" block theme="primary" type="submit" size="large" :loading="loading">
              进入管理系统
            </t-button>
          </form>

          <div class="footer">
            <RouterLink to="/login" class="footer-link">← 返回用户登录</RouterLink>
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
import { onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"
import { api } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"

const CAPTCHA_REQUIRED_KEY = "admin_login_captcha_required"

const auth = useAuthStore()
const router = useRouter()
const { brand, footerLinks, footerCopyright } = useSystemSettings()

const identifier = ref("")
const password = ref("")
const captchaCode = ref("")
const captchaId = ref("")
const captchaImage = ref("")
const requireCaptcha = ref(false)
const loading = ref(false)

const isCaptchaDisabled = (err: unknown) => {
  const message = err instanceof Error ? err.message : String(err ?? "")
  const lower = message.toLowerCase()
  return lower.includes("captcha disabled") || lower.includes("captcha not configured")
}

const captchaAvailable = ref(true)

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
    notify({ variant: "warning", title: "请输入管理员账号" })
    return
  }
  if (!password.value) {
    notify({ variant: "warning", title: "请输入管理员密码" })
    return
  }

  loading.value = true
  try {
    await auth.adminLogin(cleanIdentifier, password.value, requireCaptcha.value && captchaAvailable.value ? { token: captchaId.value, answer: captchaCode.value.trim() } : undefined)
    await router.push("/admin/dashboard")
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
  } catch (err: any) {
    const errorMapping: Record<string, string> = {
      "invalid credentials": "用户名或密码错误",
      "unauthorized": "未授权，请重新登录",
      "user not found": "用户不存在",
      "invalid password": "密码错误",
      "account disabled": "账号已被禁用",
      "account locked": "账号已被锁定",
      "admin only": "需要管理员权限",
      "captcha required": "请完成验证码",
      "captcha invalid": "验证码无效，请刷新后重试",
      "captcha expired": "验证码已过期，请刷新后重试",
      "captcha mismatch": "验证码错误",
    }

    const rawError = err?.message || "登录失败"
    const errorMsg = errorMapping[rawError.toLowerCase()] || "管理员登录失败，请检查凭据"
    notify({ variant: "destructive", title: errorMsg })

    if (!requireCaptcha.value) {
      requireCaptcha.value = true
      localStorage.setItem(CAPTCHA_REQUIRED_KEY, "true")
      try {
        await loadCaptcha()
      } catch {
        // captcha may be disabled
      }
    } else {
      await loadCaptcha()
    }
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
})
</script>

<style scoped>
.admin-login-page {
  min-height: 100vh;
  min-height: 100dvh;
  display: flex;
  width: 100%;
  background: #f0f2f7;
}

/* ---- Left hero panel ---- */
.login-hero {
  flex: 0 0 46%;
  max-width: 600px;
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  background: linear-gradient(160deg, #0a0a1a 0%, #1a1a2e 35%, #0f3460 100%);
}

/* Floating gradient orbs */
.hero-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.hero-orb-1 {
  width: 280px; height: 280px;
  background: rgba(251, 191, 36, 0.06);
  top: -40px; right: -60px;
  animation: float-orb 14s ease-in-out infinite;
}
.hero-orb-2 {
  width: 200px; height: 200px;
  background: rgba(99, 102, 241, 0.06);
  bottom: 15%; left: -50px;
  animation: float-orb 12s ease-in-out infinite reverse;
}

@keyframes float-orb {
  0%, 100% { transform: translate(0, 0) scale(1); }
  33% { transform: translate(12px, -18px) scale(1.04); }
  66% { transform: translate(-8px, 12px) scale(0.96); }
}

.hero-bg {
  position: absolute;
  inset: 0;
}

.hero-grid {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Radar ring pulse */
.ring {
  animation: ring-pulse 6s ease-in-out infinite;
}
.ring.r1 { animation-delay: 0s; }
.ring.r2 { animation-delay: 2s; }
.ring.r3 { animation-delay: 4s; }

@keyframes ring-pulse {
  0%, 100% { opacity: 0.4; }
  50% { opacity: 1; }
}

/* Radar sweep rotation */
.radar-sweep {
  transform-origin: 240px 320px;
  animation: sweep-rotate 8s linear infinite;
}

@keyframes sweep-rotate {
  from { transform: rotate(0deg); }
  to { transform: rotate(360deg); }
}

/* Radar dots blink */
.radar-dot {
  animation: dot-blink 3s ease-in-out infinite;
}
.radar-dot.rd1 { animation-delay: 0s; }
.radar-dot.rd2 { animation-delay: 0.8s; }
.radar-dot.rd3 { animation-delay: 1.6s; }
.radar-dot.rd4 { animation-delay: 2.4s; }

@keyframes dot-blink {
  0%, 100% { opacity: 0.3; }
  50% { opacity: 1; }
}

.shield {
  animation: shield-glow 3s ease-in-out infinite;
}

@keyframes shield-glow {
  0%, 100% { opacity: 0.7; }
  50% { opacity: 1; }
}

.hero-content {
  position: relative;
  z-index: 1;
  padding: 48px 36px;
  color: #fff;
  max-width: 420px;
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
  letter-spacing: 2px;
  text-transform: uppercase;
  background: rgba(251, 191, 36, 0.08);
  border: 1px solid rgba(251, 191, 36, 0.2);
  color: #fbbf24;
  margin-bottom: 20px;
  backdrop-filter: blur(8px);
}

.hero-title {
  font-size: 34px;
  font-weight: 800;
  line-height: 1.2;
  margin: 0 0 12px;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #fff 0%, #fbbf24 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-desc {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.4);
  margin: 0 0 32px;
  letter-spacing: 1px;
}

/* Glassmorphism feature card */
.hero-feature-card {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.hero-feature-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 14px;
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.03);
  border: 1px solid rgba(255, 255, 255, 0.06);
  backdrop-filter: blur(8px);
  transition: background 0.2s;
}

.hero-feature-item:hover {
  background: rgba(255, 255, 255, 0.06);
}

.hero-feature-icon {
  font-size: 16px;
  flex-shrink: 0;
  margin-top: 1px;
}

.hero-feature-text {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.hero-feature-name {
  font-size: 13px;
  font-weight: 600;
  color: rgba(255, 255, 255, 0.8);
}

.hero-feature-sub {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.35);
}

/* ---- Right form panel ---- */
.login-form-side {
  flex: 1;
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 32px 40px;
  min-height: 100vh;
  min-height: 100dvh;
  background: #f0f2f7;
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

/* Form card */
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
  width: 44px;
  height: 44px;
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
  margin-bottom: 20px;
}

.field:last-of-type {
  margin-bottom: 0;
}

.field-label {
  display: block;
  font-size: 12px;
  font-weight: 600;
  color: #475569;
  margin-bottom: 6px;
  letter-spacing: 0.3px;
}

.field :deep(.t-input),
.field :deep(.t-input__wrap) {
  width: 100%;
}

/* Input focus glow — admin uses amber accent. Selector specificity
 * (.field [data-v-xxx] :deep(.t-input.t-is-focused)) already beats TDesign's
 * default rule, so no !important needed. */
.field :deep(.t-input.t-is-focused) {
  border-color: #f59e0b;
  box-shadow: 0 0 0 3px rgba(245, 158, 11, 0.18);
}

.field :deep(.t-input__prefix) {
  color: var(--app-text-faint);
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

.captcha-image {
  width: 100%;
  height: 100%;
  display: block;
}

.captcha-refresh {
  flex-shrink: 0;
}

/* Gradient submit button (amber for admin) */
.submit-button {
  margin-top: 24px;
  height: 46px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.5px;
  transition: all 0.2s ease;
}

.form-card :deep(.submit-button.t-button--theme-primary) {
  background: linear-gradient(135deg, #f59e0b 0%, #d97706 100%);
  border: none;
}

.form-card :deep(.submit-button.t-button--theme-primary:hover) {
  background: linear-gradient(135deg, #d97706 0%, #b45309 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px rgba(217, 119, 6, 0.3);
}

.form-card :deep(.submit-button.t-button--theme-primary:active) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(217, 119, 6, 0.2);
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
  margin-top: 24px;
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
  color: #f59e0b;
}

/* TDesign overrides — input/button radius now comes from tokens.css
 * (--app-input-radius / --app-button-radius). Local override removed. */
.form-card :deep(.t-input),
.form-card :deep(.t-input__wrap) {
  transition: all 0.2s ease;
}

/* ---- Mobile ---- */
@media (max-width: 900px) {
  .login-hero {
    display: none;
  }

  .admin-login-page {
    background: linear-gradient(160deg, #0a0a1a 0%, #1a1a2e 40%, #0f3460 100%);
    justify-content: center;
    align-items: center;
  }

  .login-form-side {
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
  .login-form-side {
    padding: 16px 12px;
    align-items: flex-start;
    padding-top: max(32px, 8vh);
  }

  .form-card {
    padding: 28px 22px 24px;
    border-radius: 14px;
  }

  .brand-title {
    font-size: 20px;
  }

  .brand-logo {
    width: 38px;
    height: 38px;
  }

  .field {
    margin-bottom: 16px;
  }

  .captcha-row {
    flex-wrap: wrap;
    row-gap: 8px;
  }

  .captcha-input-wrap {
    flex: 1 1 100%;
  }

  .submit-button {
    margin-top: 16px;
    height: 44px;
  }
}
</style>



