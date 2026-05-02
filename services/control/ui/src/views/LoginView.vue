<template>
  <div class="login-page">
    <!-- Left branded panel -->
    <div class="login-hero">
      <!-- Gradient mesh orbs -->
      <div class="hero-orb hero-orb-1" />
      <div class="hero-orb hero-orb-2" />
      <div class="hero-orb hero-orb-3" />

      <div class="hero-bg">
        <svg class="hero-nodes" viewBox="0 0 480 700" fill="none" xmlns="http://www.w3.org/2000/svg">
          <defs>
            <linearGradient id="flow1" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stop-color="rgba(56,189,248,0)" />
              <stop offset="50%" stop-color="rgba(56,189,248,0.6)" />
              <stop offset="100%" stop-color="rgba(56,189,248,0)" />
            </linearGradient>
            <linearGradient id="flow2" x1="0" y1="0" x2="1" y2="0">
              <stop offset="0%" stop-color="rgba(147,197,253,0)" />
              <stop offset="50%" stop-color="rgba(147,197,253,0.4)" />
              <stop offset="100%" stop-color="rgba(147,197,253,0)" />
            </linearGradient>
          </defs>
          <!-- Connection lines -->
          <line x1="100" y1="120" x2="260" y2="200" stroke="rgba(255,255,255,0.06)" stroke-width="1"/>
          <line x1="260" y1="200" x2="380" y2="160" stroke="rgba(255,255,255,0.05)" stroke-width="1"/>
          <line x1="260" y1="200" x2="340" y2="350" stroke="rgba(255,255,255,0.05)" stroke-width="1"/>
          <line x1="100" y1="120" x2="60" y2="300" stroke="rgba(255,255,255,0.05)" stroke-width="1"/>
          <line x1="60" y1="300" x2="200" y2="400" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="340" y1="350" x2="200" y2="400" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="200" y1="400" x2="300" y2="520" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="340" y1="350" x2="420" y2="450" stroke="rgba(255,255,255,0.03)" stroke-width="1"/>
          <line x1="60" y1="300" x2="140" y2="520" stroke="rgba(255,255,255,0.03)" stroke-width="1"/>
          <line x1="380" y1="160" x2="420" y2="280" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <line x1="420" y1="280" x2="340" y2="350" stroke="rgba(255,255,255,0.04)" stroke-width="1"/>
          <!-- Animated flow particles -->
          <circle r="2" fill="url(#flow1)">
            <animateMotion dur="4s" repeatCount="indefinite" path="M100,120 L260,200 L340,350 L200,400" />
          </circle>
          <circle r="1.5" fill="url(#flow2)">
            <animateMotion dur="5s" repeatCount="indefinite" begin="1.5s" path="M380,160 L260,200 L60,300 L200,400 L300,520" />
          </circle>
          <circle r="1.5" fill="url(#flow1)">
            <animateMotion dur="6s" repeatCount="indefinite" begin="3s" path="M420,280 L340,350 L420,450" />
          </circle>
          <!-- Node dots with glow -->
          <circle cx="100" cy="120" r="3" fill="rgba(255,255,255,0.2)" />
          <circle cx="100" cy="120" r="6" fill="none" stroke="rgba(56,189,248,0.15)" stroke-width="1" class="node-ring nr1"/>
          <circle cx="260" cy="200" r="4" fill="rgba(56,189,248,0.5)" />
          <circle cx="260" cy="200" r="10" fill="none" stroke="rgba(56,189,248,0.12)" stroke-width="1" class="node-ring nr2"/>
          <circle cx="380" cy="160" r="2.5" fill="rgba(255,255,255,0.18)" />
          <circle cx="340" cy="350" r="3" fill="rgba(147,197,253,0.35)" />
          <circle cx="340" cy="350" r="8" fill="none" stroke="rgba(147,197,253,0.1)" stroke-width="1" class="node-ring nr3"/>
          <circle cx="60" cy="300" r="3" fill="rgba(56,189,248,0.3)" />
          <circle cx="200" cy="400" r="3.5" fill="rgba(255,255,255,0.25)" />
          <circle cx="200" cy="400" r="9" fill="none" stroke="rgba(56,189,248,0.08)" stroke-width="1" class="node-ring nr4"/>
          <circle cx="300" cy="520" r="2" fill="rgba(147,197,253,0.2)" />
          <circle cx="420" cy="280" r="2.5" fill="rgba(255,255,255,0.15)" />
          <circle cx="420" cy="450" r="2" fill="rgba(56,189,248,0.15)" />
          <circle cx="140" cy="520" r="2" fill="rgba(255,255,255,0.12)" />
        </svg>
      </div>

      <div class="hero-content">
        <div class="hero-badge">CDN 控制台</div>
        <h2 class="hero-title">极速分发<br/>智能加速</h2>
        <p class="hero-desc">全球节点 · 智能调度 · 安全防护 · 极速分发</p>

        <div class="hero-stats-card">
          <div class="hero-stat">
            <span class="hero-stat-value">99.9%</span>
            <span class="hero-stat-label">服务可用性</span>
          </div>
          <div class="hero-stat-divider" />
          <div class="hero-stat">
            <span class="hero-stat-value">&lt;10ms</span>
            <span class="hero-stat-label">边缘节点延迟</span>
          </div>
          <div class="hero-stat-divider" />
          <div class="hero-stat">
            <span class="hero-stat-value">50+</span>
            <span class="hero-stat-label">全球节点</span>
          </div>
        </div>

        <div class="hero-features">
          <div class="hero-feature-item">
            <span class="hero-feature-icon">⚡</span>
            <span>全球边缘节点覆盖</span>
          </div>
          <div class="hero-feature-item">
            <span class="hero-feature-icon">🛡</span>
            <span>DDoS / CC / WAF 全方位防护</span>
          </div>
          <div class="hero-feature-item">
            <span class="hero-feature-icon">🚀</span>
            <span>智能调度与极速分发</span>
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
                <p class="form-subtitle">登录到您的账户</p>
              </div>
            </div>
          </div>

          <div v-if="licenseNotice" class="license-notice" role="alert">
            <t-icon name="error-circle" size="16px" />
            <span>{{ licenseNotice }}</span>
          </div>

          <form class="form" @submit.prevent="handleSubmit">
            <div class="field">
              <label class="field-label">用户名 / 邮箱</label>
              <t-input
                v-model="identifier"
                placeholder="请输入用户名或邮箱"
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
              <label class="field-label">图形验证码</label>
              <div class="captcha-row">
                <div class="captcha-input-wrap">
                  <t-input v-model="captchaCode" placeholder="请输入验证码" size="large" :maxlength="4">
                    <template #prefixIcon><t-icon name="secured" /></template>
                  </t-input>
                </div>
                <div v-if="captchaImage" class="captcha-box" @click="() => loadCaptcha()">
                  <img v-if="captchaImage.startsWith('data:image')" :src="captchaImage" alt="captcha" />
                  <span v-else>{{ captchaImage }}</span>
                </div>
                <t-button variant="outline" size="large" class="captcha-refresh" @click.prevent="() => loadCaptcha()">
                  <t-icon name="refresh" />
                </t-button>
              </div>
            </div>

            <t-button class="submit-button" block theme="primary" type="submit" size="large" :loading="loading">
              登录
            </t-button>
          </form>

          <div class="footer">
            <template v-if="registerEnabled">
              <RouterLink to="/register" class="footer-link">立即注册</RouterLink>
              <span class="footer-dot">·</span>
            </template>
            <RouterLink to="/forgot-password" class="footer-link">忘记密码？</RouterLink>
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
import { computed, onMounted, ref } from "vue"
import { useRouter } from "vue-router"
import { useAuthStore } from "@/stores/auth"
import { api } from "@/lib/api"
import { notify } from "@/lib/notify"
import { useSystemSettings } from "@/lib/systemSettings"

const CAPTCHA_REQUIRED_KEY = "login_captcha_required"

const auth = useAuthStore()
const router = useRouter()
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
    await router.push("/dashboard")
    localStorage.removeItem(CAPTCHA_REQUIRED_KEY)
  } catch (err: any) {
    const errorMapping: Record<string, string> = {
      "invalid credentials": "用户名或密码错误",
      "unauthorized": "未授权，请重新登录",
      "user not found": "用户不存在",
      "invalid password": "密码错误",
      "account disabled": "账号已被禁用",
      "account locked": "账号已被锁定",
      "captcha required": "请完成验证码",
      "captcha invalid": "验证码无效，请刷新后重试",
      "captcha expired": "验证码已过期，请刷新后重试",
      "captcha mismatch": "验证码错误",
    }

    const rawError = err?.message || "登录失败"
    const errorMsg = errorMapping[rawError.toLowerCase()] || "登录失败，请检查凭据"
    notify({ variant: "destructive", title: errorMsg })

    // Only attempt to load captcha if it's not disabled on the backend
    if (!requireCaptcha.value) {
      requireCaptcha.value = true
      localStorage.setItem(CAPTCHA_REQUIRED_KEY, "true")
      try {
        await loadCaptcha()
      } catch {
        // If loadCaptcha fails (e.g. captcha disabled), keep requireCaptcha as-is
        // so the user doesn't get stuck in a loop
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

  api
    .getPublicLicenseStatus()
    .then((res) => {
      licenseNotice.value = resolveLicenseNotice(res.status, res.reason)
    })
    .catch(() => {})
})
</script>

<style scoped>
.login-page {
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
  background: linear-gradient(160deg, #020617 0%, #0f172a 35%, #0c4a6e 100%);
}

/* Floating gradient orbs for depth */
.hero-orb {
  position: absolute;
  border-radius: 50%;
  filter: blur(80px);
  pointer-events: none;
}
.hero-orb-1 {
  width: 300px; height: 300px;
  background: rgba(56, 189, 248, 0.12);
  top: -60px; left: -80px;
  animation: float-orb 12s ease-in-out infinite;
}
.hero-orb-2 {
  width: 200px; height: 200px;
  background: rgba(99, 102, 241, 0.1);
  bottom: 10%; right: -40px;
  animation: float-orb 15s ease-in-out infinite reverse;
}
.hero-orb-3 {
  width: 160px; height: 160px;
  background: rgba(16, 185, 129, 0.08);
  top: 50%; left: 30%;
  animation: float-orb 10s ease-in-out 2s infinite;
}

@keyframes float-orb {
  0%, 100% { transform: translate(0, 0) scale(1); }
  33% { transform: translate(15px, -20px) scale(1.05); }
  66% { transform: translate(-10px, 15px) scale(0.95); }
}

.hero-bg {
  position: absolute;
  inset: 0;
  opacity: 0.8;
}

.hero-nodes {
  width: 100%;
  height: 100%;
  object-fit: cover;
}

/* Node ring pulse animation */
.node-ring {
  animation: ring-expand 4s ease-out infinite;
}
.node-ring.nr1 { animation-delay: 0s; }
.node-ring.nr2 { animation-delay: 1s; }
.node-ring.nr3 { animation-delay: 2s; }
.node-ring.nr4 { animation-delay: 3s; }

@keyframes ring-expand {
  0% { opacity: 0.5; transform-origin: center; }
  50% { opacity: 0.15; }
  100% { opacity: 0; }
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
  letter-spacing: 1.5px;
  background: rgba(56, 189, 248, 0.1);
  border: 1px solid rgba(56, 189, 248, 0.2);
  color: #7dd3fc;
  margin-bottom: 20px;
  backdrop-filter: blur(8px);
}

.hero-title {
  font-size: 34px;
  font-weight: 800;
  line-height: 1.2;
  margin: 0 0 12px;
  letter-spacing: -0.5px;
  background: linear-gradient(135deg, #fff 0%, #B8AFFE 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.hero-desc {
  font-size: 14px;
  color: rgba(255, 255, 255, 0.45);
  margin: 0 0 32px;
  letter-spacing: 1px;
}

/* Glassmorphism stats card */
.hero-stats-card {
  display: flex;
  align-items: center;
  gap: 20px;
  padding: 18px 24px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.04);
  border: 1px solid rgba(255, 255, 255, 0.08);
  backdrop-filter: blur(12px);
  margin-bottom: 32px;
}

.hero-stat {
  display: flex;
  flex-direction: column;
  gap: 3px;
  flex: 1;
  text-align: center;
}

.hero-stat-value {
  font-size: 20px;
  font-weight: 700;
  color: #7dd3fc;
  font-variant-numeric: tabular-nums;
}

.hero-stat-label {
  font-size: 11px;
  color: rgba(255, 255, 255, 0.35);
  letter-spacing: 0.3px;
}

.hero-stat-divider {
  width: 1px;
  height: 32px;
  background: rgba(255, 255, 255, 0.08);
  flex-shrink: 0;
}

/* Feature list */
.hero-features {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.hero-feature-item {
  display: flex;
  align-items: center;
  gap: 10px;
  font-size: 13px;
  color: rgba(255, 255, 255, 0.5);
  transition: color 0.2s;
}

.hero-feature-icon {
  font-size: 14px;
  width: 20px;
  text-align: center;
  flex-shrink: 0;
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

/* Form card with shadow */
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

.license-notice {
  margin-bottom: 20px;
  padding: 10px 14px;
  border-radius: 10px;
  background: #fffbeb;
  border: 1px solid #fde68a;
  color: #92400e;
  font-size: 13px;
  display: flex;
  align-items: center;
  gap: 8px;
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

/* Input focus glow — picks up brand from tokens. Specificity already
 * beats TDesign's default, so no !important needed. */
.field :deep(.t-input.t-is-focused) {
  border-color: var(--td-brand-color);
  box-shadow: var(--app-focus-ring);
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

.captcha-box img {
  display: block;
  width: 120px;
  height: 40px;
}

.captcha-refresh {
  flex-shrink: 0;
}

/* Gradient submit button */
.submit-button {
  margin-top: 24px;
  height: 46px;
  font-size: 15px;
  font-weight: 600;
  letter-spacing: 0.5px;
  transition: all 0.2s ease;
}

.form-card :deep(.submit-button.t-button--theme-primary) {
  background: linear-gradient(135deg, #635BFF 0%, #4D44E0 100%);
  border: none;
}

.form-card :deep(.submit-button.t-button--theme-primary:hover) {
  background: linear-gradient(135deg, #4D44E0 0%, #4D44E0 100%);
  transform: translateY(-1px);
  box-shadow: 0 4px 16px rgba(77, 68, 224, 0.3);
}

.form-card :deep(.submit-button.t-button--theme-primary:active) {
  transform: translateY(0);
  box-shadow: 0 2px 8px rgba(77, 68, 224, 0.2);
}

.footer {
  margin-top: 22px;
  text-align: center;
  font-size: 13px;
  color: #94a3b8;
}

.footer-link {
  color: #635BFF;
  text-decoration: none;
  font-weight: 500;
  transition: color 0.2s;
}

.footer-link:hover {
  color: #4D44E0;
}

.footer-dot {
  margin: 0 8px;
  color: #cbd5e1;
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
  color: #635BFF;
}

/* TDesign overrides — input/button radius now comes from tokens.css. */
.form-card :deep(.t-input),
.form-card :deep(.t-input__wrap) {
  transition: all 0.2s ease;
}

/* ---- Mobile: stack vertically, hide hero ---- */
@media (max-width: 900px) {
  .login-hero {
    display: none;
  }

  .login-page {
    background: linear-gradient(160deg, #020617 0%, #0f172a 40%, #1e3a5f 100%);
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
