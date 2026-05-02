<template>
  <div v-if="loading" class="loading-row">
    <t-loading />
  </div>
  <div v-else class="site-settings">
    <!-- 品牌与外观 -->
    <section class="block">
      <div class="block-title">品牌与外观</div>
      <div class="block-desc">显示在控制台顶栏、登录页和浏览器标签页上的站点身份。</div>

      <div class="brand-preview">
        <div class="brand-preview-card">
          <div class="brand-preview-avatar">
            <img v-if="settings.logo" :src="settings.logo" :alt="settings.system_name" />
            <span v-else>{{ (settings.system_name || 'C').charAt(0) }}</span>
          </div>
          <div class="brand-preview-meta">
            <div class="brand-preview-title">{{ settings.system_name || 'LingCDN' }}</div>
            <div class="brand-preview-sub">控制台预览</div>
          </div>
        </div>
      </div>

      <t-form layout="vertical" label-align="top" :disabled="saving">
        <div class="grid-2">
          <t-form-item label="站点名称">
            <t-input v-model="settings.system_name" placeholder="LingCDN" />
          </t-form-item>
          <t-form-item label="Logo URL">
            <t-input v-model="settings.logo" placeholder="/logo.svg 或 https://…/logo.png" />
          </t-form-item>
        </div>
        <div class="grid-2">
          <t-form-item label="Favicon URL">
            <t-input v-model="settings.favicon" placeholder="/favicon.ico" />
          </t-form-item>
          <t-form-item label="Footer 版权">
            <t-input v-model="settings.footer_copyright" placeholder="© {year} {name}. All rights reserved." />
            <div class="hint">支持 <code>{year}</code> / <code>{name}</code> 占位符</div>
          </t-form-item>
        </div>
        <t-form-item label="Footer 链接">
          <t-textarea
            v-model="settings.footer_links"
            :autosize="{ minRows: 3, maxRows: 8 }"
            placeholder="服务条款|/terms&#10;隐私政策|/privacy"
          />
          <div class="hint">每行一个，格式 <code>显示文字|目标 URL</code>。空白行会被忽略。</div>
        </t-form-item>
      </t-form>
    </section>

    <div class="section-divider" />

    <!-- SMTP -->
    <section class="block">
      <div class="block-title">SMTP 邮件通道</div>
      <div class="block-desc">注册验证、密码找回、订单通知等邮件均经此通道发出。</div>

      <t-form layout="vertical" label-align="top" :disabled="saving">
        <div class="grid-2">
          <t-form-item label="SMTP 主机">
            <t-input v-model="settings.smtp_host" placeholder="smtp.example.com" />
          </t-form-item>
          <t-form-item label="SMTP 端口">
            <t-input-number v-model="settings.smtp_port" :min="1" :max="65535" />
          </t-form-item>
        </div>
        <div class="grid-2">
          <t-form-item label="SMTP 用户名">
            <t-input v-model="settings.smtp_username" autocomplete="off" />
          </t-form-item>
          <t-form-item label="SMTP 密码">
            <t-input
              v-model="settings.smtp_password"
              type="password"
              autocomplete="new-password"
              placeholder="留空则保留当前密码"
            />
          </t-form-item>
        </div>
        <div class="grid-2">
          <t-form-item label="发件人邮箱">
            <t-input v-model="settings.smtp_from" placeholder="no-reply@example.com" />
          </t-form-item>
          <t-form-item label="发件人名称">
            <t-input v-model="settings.smtp_from_name" placeholder="LingCDN" />
          </t-form-item>
        </div>
      </t-form>
    </section>

    <div class="section-divider" />

    <!-- 联系邮箱 -->
    <section class="block">
      <div class="block-title">对外联系邮箱</div>
      <div class="block-desc">显示在站内底部、订单页和系统提醒中。</div>

      <t-form layout="vertical" label-align="top" :disabled="saving">
        <div class="grid-2">
          <t-form-item label="销售邮箱">
            <t-input v-model="settings.sales_email" placeholder="sales@example.com" />
          </t-form-item>
          <t-form-item label="支持邮箱">
            <t-input v-model="settings.support_email" placeholder="support@example.com" />
          </t-form-item>
        </div>
      </t-form>
    </section>

    <div class="section-divider" />

    <!-- 注册与通知 -->
    <section class="block">
      <div class="block-title">注册与通知策略</div>
      <div class="block-desc">控制平台对外的开放注册行为与内部版本提醒。</div>

      <div class="toggle-list" :class="{ disabled: saving }">
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">开放注册</div>
            <div class="toggle-desc">关闭后新用户无法自助注册账号，需管理员代开。</div>
          </div>
          <t-switch v-model="settings.register_enabled" :disabled="saving" />
        </div>
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">注册邮箱验证</div>
            <div class="toggle-desc">注册时要求验证邮箱所有权，依赖上方 SMTP 通道可用。</div>
          </div>
          <t-switch v-model="settings.register_email_verification" :disabled="saving" />
        </div>
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">新版本通知</div>
            <div class="toggle-desc">控制台检测到主控有可用新版本时，向管理员发送邮件提醒。</div>
          </div>
          <t-switch v-model="settings.notify_new_build" :disabled="saving" />
        </div>
      </div>
    </section>

    <div class="actions">
      <span v-if="dirty" class="dirty-pill" role="status">
        <span class="dirty-dot" aria-hidden="true" />
        有未保存的修改
      </span>
      <div class="actions-spacer" />
      <t-button variant="outline" :disabled="saving || !dirty" @click="resetSettings">取消</t-button>
      <t-button theme="primary" :loading="saving" :disabled="!dirty" @click="handleSave">保存设置</t-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"

type SiteSettings = {
  system_name: string
  logo: string
  favicon: string
  footer_copyright: string
  footer_links: string

  smtp_host: string
  smtp_port: number
  smtp_username: string
  smtp_password: string
  smtp_from: string
  smtp_from_name: string

  sales_email: string
  support_email: string

  notify_new_build: boolean
  register_enabled: boolean
  register_email_verification: boolean
}

const defaults: SiteSettings = {
  system_name: "LingCDN",
  logo: "",
  favicon: "",
  footer_copyright: `© {year} {name}. All rights reserved.`,
  footer_links: "",

  smtp_host: "",
  smtp_port: 587,
  smtp_username: "",
  smtp_password: "",
  smtp_from: "",
  smtp_from_name: "",

  sales_email: "",
  support_email: "",

  notify_new_build: true,
  register_enabled: true,
  register_email_verification: false,
}

const settings = ref<SiteSettings>({ ...defaults })
const loadedSnapshot = ref<SiteSettings>({ ...defaults })
const loading = ref(true)
const saving = ref(false)

// `dirty` powers the visible "未保存" pill so admins don't navigate away
// thinking their edits stuck. JSON compare is cheap on a flat 16-key
// object and avoids hand-rolling a per-field diff.
const dirty = computed(() => JSON.stringify(settings.value) !== JSON.stringify(loadedSnapshot.value))

const loadSettings = async () => {
  try {
    loading.value = true
    const { settings: data } = await api.getSettings()
    const src = data || {}
    const next: SiteSettings = {
      system_name: String(src.system_name ?? defaults.system_name),
      logo: String(src.logo ?? ""),
      favicon: String(src.favicon ?? ""),
      footer_copyright: String(src.footer_copyright ?? defaults.footer_copyright),
      footer_links: String(src.footer_links ?? ""),

      smtp_host: String(src.smtp_host ?? ""),
      smtp_port: Number(src.smtp_port ?? defaults.smtp_port) || defaults.smtp_port,
      smtp_username: String(src.smtp_username ?? ""),
      // Password is intentionally not echoed back from the server. Leave
      // empty — the API treats empty as "keep existing".
      smtp_password: "",
      smtp_from: String(src.smtp_from ?? ""),
      smtp_from_name: String(src.smtp_from_name ?? ""),

      sales_email: String(src.sales_email ?? ""),
      support_email: String(src.support_email ?? ""),

      notify_new_build: Boolean(src.notify_new_build ?? defaults.notify_new_build),
      register_enabled: Boolean(src.register_enabled ?? defaults.register_enabled),
      register_email_verification: Boolean(
        src.register_email_verification ?? defaults.register_email_verification
      ),
    }
    settings.value = next
    loadedSnapshot.value = { ...next }
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载设置失败")
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    const payload: Partial<SiteSettings> = { ...settings.value }
    if (!payload.smtp_password) delete payload.smtp_password
    await api.updateSettings(payload)
    loadedSnapshot.value = { ...settings.value, smtp_password: "" }
    settings.value = { ...loadedSnapshot.value }
    MessagePlugin.success("设置保存成功")
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存设置失败")
  } finally {
    saving.value = false
  }
}

const resetSettings = () => {
  settings.value = { ...loadedSnapshot.value }
}

onMounted(loadSettings)
</script>

<style scoped>
.site-settings {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.loading-row {
  text-align: center;
  padding: 32px 0;
}

.block {
  padding-top: 4px;
}

.block-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
  margin-bottom: 4px;
}

.block-desc {
  font-size: 12px;
  color: var(--app-text-muted);
  margin-bottom: 14px;
  line-height: 1.5;
}

.section-divider {
  height: 1px;
  background: var(--app-divider);
  margin: 20px 0;
}

.grid-2 {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px 24px;
}

.hint {
  font-size: 12px;
  color: var(--app-text-faint);
  margin-top: 4px;
  line-height: 1.5;
}

.hint code {
  background: var(--app-surface-muted);
  border: 1px solid var(--app-border);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 11px;
  color: var(--app-text);
}

.brand-preview {
  margin-bottom: 16px;
  padding: 14px 16px;
  border-radius: var(--app-input-radius);
  border: 1px dashed var(--app-border-strong);
  background: var(--app-surface-muted);
}

.brand-preview-card {
  display: flex;
  align-items: center;
  gap: 12px;
}

.brand-preview-avatar {
  width: 40px;
  height: 40px;
  border-radius: var(--app-input-radius);
  background: var(--app-brand-gradient);
  color: #fff;
  font-size: 16px;
  font-weight: 700;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  overflow: hidden;
  flex-shrink: 0;
}

.brand-preview-avatar img {
  width: 100%;
  height: 100%;
  object-fit: contain;
  background: rgba(255, 255, 255, 0.06);
}

.brand-preview-meta {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.brand-preview-title {
  font-size: 15px;
  font-weight: 700;
  color: var(--app-text-strong);
  letter-spacing: 0.2px;
}

.brand-preview-sub {
  font-size: 12px;
  color: var(--app-text-faint);
}

.toggle-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.toggle-list.disabled .toggle-row {
  opacity: 0.7;
}

.toggle-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 16px;
  padding: 12px 14px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-input-radius);
  background: var(--app-surface);
  transition: border-color var(--app-anim-base) var(--app-easing-standard),
              background var(--app-anim-base) var(--app-easing-standard);
}

.toggle-row:hover {
  border-color: var(--app-border-strong);
  background: var(--app-surface-muted);
}

.toggle-meta {
  min-width: 0;
}

.toggle-label {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.toggle-desc {
  font-size: 12px;
  color: var(--app-text-muted);
  margin-top: 4px;
  line-height: 1.5;
}

.actions {
  margin-top: 24px;
  padding-top: 16px;
  border-top: 1px solid var(--app-divider);
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.actions-spacer {
  flex: 1;
}

.dirty-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 4px 10px;
  border-radius: var(--app-pill-radius);
  font-size: 12px;
  font-weight: 500;
  background: var(--app-warning-soft-bg);
  color: var(--app-warning-soft-fg);
}

.dirty-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--app-warning);
  animation: dirty-pulse 1.6s ease-in-out infinite;
}

@keyframes dirty-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

@media (max-width: 720px) {
  .grid-2 {
    grid-template-columns: 1fr;
  }

  .actions {
    flex-direction: column;
    align-items: stretch;
  }

  .actions-spacer {
    display: none;
  }

  .dirty-pill {
    justify-content: center;
  }
}
</style>
