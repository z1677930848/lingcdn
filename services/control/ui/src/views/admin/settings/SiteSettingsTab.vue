<template>
  <LoadingState v-if="loading" />
  <ErrorState v-else-if="error" :message="error" @retry="loadSettings" />
  <div v-else class="site-settings">
    <!-- 品牌与外观 -->
    <section class="block">
      <div class="block-title">品牌与外观</div>
      <div class="block-desc">显示在控制台顶栏、登录页和浏览器标签页上的站点身份。</div>

      <div class="brand-preview">
        <div class="brand-preview-card">
          <template v-if="settings.sidebar_brand_mode === 'logo'">
            <div class="brand-preview-avatar brand-preview-avatar--logo">
              <img v-if="settings.logo" :src="settings.logo" :alt="settings.system_name" />
              <span v-else>{{ (settings.system_name || 'C').charAt(0) }}</span>
            </div>
          </template>
          <template v-else>
            <div class="brand-preview-meta brand-preview-meta--solo">
              <div class="brand-preview-title">{{ settings.system_name || 'LingCDN' }}</div>
              <div class="brand-preview-sub">侧栏显示系统名称</div>
            </div>
          </template>
          <div v-if="settings.sidebar_brand_mode === 'logo'" class="brand-preview-meta">
            <div class="brand-preview-title">{{ settings.system_name || 'LingCDN' }}</div>
            <div class="brand-preview-sub">侧栏仅显示 Logo</div>
          </div>
        </div>
      </div>

      <el-form label-position="top" :disabled="saving">
        <el-form-item label="侧栏品牌显示">
          <el-radio-group v-model="settings.sidebar_brand_mode">
            <el-radio value="name">系统名称</el-radio>
            <el-radio value="logo">Logo</el-radio>
          </el-radio-group>
          <div class="hint">控制控制台左侧顶部仅显示系统名称或 Logo 之一；登录页仍可同时展示名称与 Logo。</div>
        </el-form-item>
        <div class="grid-2">
          <el-form-item label="站点名称">
            <el-input v-model="settings.system_name" placeholder="LingCDN" />
          </el-form-item>
          <el-form-item label="Logo URL">
            <el-input v-model="settings.logo" placeholder="/logo.svg 或 https://…/logo.png" />
          </el-form-item>
        </div>
        <div class="grid-2">
          <el-form-item label="Favicon URL">
            <el-input v-model="settings.favicon" placeholder="/favicon.ico" />
          </el-form-item>
          <el-form-item label="Footer 版权">
            <el-input v-model="settings.footer_copyright" placeholder="© {year} {name}. All rights reserved." />
            <div class="hint">支持 <code>{year}</code> / <code>{name}</code> 占位符</div>
          </el-form-item>
        </div>
        <el-form-item label="Footer 链接">
          <el-input
            v-model="settings.footer_links"
            :autosize="{ minRows: 3, maxRows: 8 }"
            placeholder="服务条款|/terms&#10;隐私政策|/privacy"
          />
          <div class="hint">每行一个，格式 <code>显示文字|目标 URL</code>。空白行会被忽略。</div>
        </el-form-item>
      </el-form>
    </section>

    <div class="section-divider" />

    <!-- SMTP -->
    <section class="block">
      <div class="block-title">SMTP 邮件通道</div>
      <div class="block-desc">注册验证、密码找回、订单通知等邮件均经此通道发出。</div>

      <el-form label-position="top" :disabled="saving">
        <div class="grid-2">
          <el-form-item label="SMTP 主机">
            <el-input v-model="settings.smtp_host" placeholder="smtp.example.com" />
          </el-form-item>
          <el-form-item label="SMTP 端口">
            <el-input-number v-model="settings.smtp_port" :min="1" :max="65535" />
          </el-form-item>
        </div>
        <div class="grid-2">
          <el-form-item label="SMTP 用户名">
            <el-input v-model="settings.smtp_username" autocomplete="off" />
          </el-form-item>
          <el-form-item label="SMTP 密码">
            <el-input
              v-model="settings.smtp_password"
              type="password"
              autocomplete="new-password"
              placeholder="留空则保留当前密码"
            />
          </el-form-item>
        </div>
        <div class="grid-2">
          <el-form-item label="发件人邮箱">
            <el-input v-model="settings.smtp_from" placeholder="no-reply@example.com" />
          </el-form-item>
          <el-form-item label="发件人名称">
            <el-input v-model="settings.smtp_from_name" placeholder="LingCDN" />
          </el-form-item>
        </div>
      </el-form>
    </section>

    <div class="section-divider" />

    <!-- 联系邮箱 -->
    <section class="block">
      <div class="block-title">对外联系邮箱</div>
      <div class="block-desc">显示在站内底部、订单页和系统提醒中。</div>

      <el-form label-position="top" :disabled="saving">
        <div class="grid-2">
          <el-form-item label="销售邮箱">
            <el-input v-model="settings.sales_email" placeholder="sales@example.com" />
          </el-form-item>
          <el-form-item label="支持邮箱">
            <el-input v-model="settings.support_email" placeholder="support@example.com" />
          </el-form-item>
        </div>
      </el-form>
    </section>

    <div class="section-divider" />

    <!-- 注册与通知 -->
    <section class="block">
      <div class="block-title">注册与通知策略</div>
      <div class="block-desc">控制平台对外的开放注册行为与内部版本提醒。</div>

      <el-form label-position="top" :disabled="saving">
        <el-form-item label="续费开放时间">
          <div class="renewal-window-input">
            <el-input-number v-model="settings.renewal_before_expiry_days" :min="0" :max="3650" />
            <span class="input-unit">天</span>
          </div>
          <div class="hint">用户仅可在套餐到期前指定天数内续费；填 0 表示到期后才可再次购买同套餐。</div>
        </el-form-item>
      </el-form>

      <div class="toggle-list" :class="{ disabled: saving }">
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">开放注册</div>
            <div class="toggle-desc">关闭后新用户无法自助注册账号，需管理员代开。</div>
          </div>
          <el-switch v-model="settings.register_enabled" :disabled="saving" />
        </div>
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">注册邮箱验证</div>
            <div class="toggle-desc">注册时要求验证邮箱所有权，依赖上方 SMTP 通道可用。</div>
          </div>
          <el-switch v-model="settings.register_email_verification" :disabled="saving" />
        </div>
        <div class="toggle-row">
          <div class="toggle-meta">
            <div class="toggle-label">新版本通知</div>
            <div class="toggle-desc">控制台检测到主控有可用新版本时，向管理员发送邮件提醒。</div>
          </div>
          <el-switch v-model="settings.notify_new_build" :disabled="saving" />
        </div>
      </div>
    </section>

    <div class="actions">
      <span v-if="dirty" class="dirty-pill" role="status">
        <span class="dirty-dot" aria-hidden="true" />
        有未保存的修改
      </span>
      <div class="actions-spacer" />
      <el-button plain :disabled="saving || !dirty" @click="resetSettings">取消</el-button>
      <el-button type="primary" :loading="saving" :disabled="!dirty" @click="handleSave">保存设置</el-button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

type SiteSettings = {
  system_name: string
  logo: string
  favicon: string
  sidebar_brand_mode: "name" | "logo"
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
  renewal_before_expiry_days: number
}

const defaults: SiteSettings = {
  system_name: "LingCDN",
  logo: "",
  favicon: "",
  sidebar_brand_mode: "name",
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
  renewal_before_expiry_days: 30,
}

const settings = ref<SiteSettings>({ ...defaults })
const loadedSnapshot = ref<SiteSettings>({ ...defaults })
const loading = ref(true)
const error = ref("")
const saving = ref(false)

// `dirty` powers the visible "未保存" pill so admins don't navigate away
// thinking their edits stuck. JSON compare is cheap on a flat 16-key
// object and avoids hand-rolling a per-field diff.
const dirty = computed(() => JSON.stringify(settings.value) !== JSON.stringify(loadedSnapshot.value))

const loadSettings = async () => {
  try {
    loading.value = true
    error.value = ""
    const { settings: data } = await api.getSettings()
    const src = data || {}
    const next: SiteSettings = {
      system_name: String(src.system_name ?? defaults.system_name),
      logo: String(src.logo ?? ""),
      favicon: String(src.favicon ?? ""),
      sidebar_brand_mode: String(src.sidebar_brand_mode ?? defaults.sidebar_brand_mode) === "logo" ? "logo" : "name",
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
      renewal_before_expiry_days: Number(
        src.renewal_before_expiry_days ?? defaults.renewal_before_expiry_days
      ),
    }
    settings.value = next
    loadedSnapshot.value = { ...next }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载设置失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    const payload: Partial<SiteSettings> = { ...settings.value }
    payload.renewal_before_expiry_days = Math.max(
      0,
      Math.min(3650, Number(payload.renewal_before_expiry_days) || 0)
    )
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

