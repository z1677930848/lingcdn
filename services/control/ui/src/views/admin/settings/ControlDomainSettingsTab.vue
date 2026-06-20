<template>
  <div class="settings-tab">
    <ErrorState v-if="error" :message="error" @retry="loadAll" />
    <LoadingState v-else-if="loading" />
    <div v-else>
      <section class="block">
        <div class="block-title">主控对外域名</div>
        <div class="block-desc">
          绑定后，节点安装命令、离线升级包地址等将优先使用此域名。请先将域名 A 记录解析到服务器公网 IP。
        </div>

        <div v-if="identity.control_public_url" class="preview-card">
          <div class="preview-label">当前公开地址</div>
          <div class="preview-value">{{ identity.control_public_url }}</div>
          <div v-if="identity.control_grpc_endpoint" class="preview-sub">
            gRPC：{{ identity.control_grpc_endpoint }}
          </div>
        </div>

        <el-form label-position="top" :disabled="saving">
          <el-form-item label="主控域名">
            <el-input v-model="settings.control_domain" placeholder="cduser.example.com" />
          </el-form-item>
          <div class="grid-2">
            <el-form-item label="公开 URL 使用 HTTPS">
              <el-switch v-model="settings.control_public_https" />
              <div class="hint">开启后生成 https:// 链接（需配置反向代理或 ACME）</div>
            </el-form-item>
            <el-form-item label="IP 访问重定向到域名">
              <el-switch v-model="settings.control_redirect_ip" />
              <div class="hint">通过 IP 访问时 301 跳转到绑定域名</div>
            </el-form-item>
          </div>
        </el-form>
      </section>

      <section class="block">
        <div class="block-title">DNS 校验</div>
        <div class="block-desc">
          检查域名是否已解析到本机公网 IP（{{ identity.public_ip || "未配置 PUBLIC_IP" }}）。
        </div>
        <div class="verify-row">
          <el-button plain :loading="verifying" @click="handleVerify">检测 DNS</el-button>
          <el-tag v-if="verifyResult" :type="verifyResult.ok ? 'success' : 'warning'" effect="light">
            {{ verifyResult.ok ? "解析正确" : "解析未匹配" }}
          </el-tag>
        </div>
        <div v-if="verifyResult" class="verify-detail">
          <div>记录：{{ (verifyResult.records || []).join(", ") || "无" }}</div>
          <div v-if="verifyResult.expected">期望：{{ verifyResult.expected }}</div>
          <div v-if="verifyResult.error" class="verify-error">{{ verifyResult.error }}</div>
        </div>
      </section>

      <div class="actions">
        <el-button type="primary" :loading="saving" @click="handleSave">保存</el-button>
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

type ControlDomainSettings = {
  control_domain: string
  control_public_https: boolean
  control_redirect_ip: boolean
}

type ControlIdentity = {
  control_domain?: string
  control_public_url?: string
  control_grpc_endpoint?: string
  public_ip?: string
}

type VerifyResult = {
  ok: boolean
  domain?: string
  records?: string[]
  expected?: string
  error?: string
}

const settings = ref<ControlDomainSettings>({
  control_domain: "",
  control_public_https: false,
  control_redirect_ip: false,
})

const identity = ref<ControlIdentity>({})
const loading = ref(false)
const error = ref("")
const saving = ref(false)
const verifying = ref(false)
const verifyResult = ref<VerifyResult | null>(null)

const loadAll = async () => {
  try {
    loading.value = true
    error.value = ""
    const [{ settings: data }, info] = await Promise.all([
      api.getSettings(),
      api.getControlDomainInfo(),
    ])
    identity.value = info
    if (data) {
      settings.value = {
        control_domain: String(data.control_domain ?? ""),
        control_public_https: Boolean(data.control_public_https),
        control_redirect_ip: Boolean(data.control_redirect_ip),
      }
    }
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : "加载设置失败"
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    await api.updateSettings({
      control_domain: settings.value.control_domain.trim(),
      control_public_https: settings.value.control_public_https,
      control_redirect_ip: settings.value.control_redirect_ip,
    })
    identity.value = await api.getControlDomainInfo()
    MessagePlugin.success("主控域名设置已保存")
    verifyResult.value = null
  } catch (err: unknown) {
    MessagePlugin.error(err instanceof Error ? err.message : "保存失败")
  } finally {
    saving.value = false
  }
}

const handleVerify = async () => {
  verifying.value = true
  try {
    const domain = settings.value.control_domain.trim()
    verifyResult.value = await api.verifyControlDomain(domain || undefined)
    if (verifyResult.value?.ok) {
      MessagePlugin.success("DNS 解析正确")
    } else {
      MessagePlugin.warning("DNS 尚未正确解析到本机 IP")
    }
  } catch (err: unknown) {
    MessagePlugin.error(err instanceof Error ? err.message : "检测失败")
  } finally {
    verifying.value = false
  }
}

onMounted(loadAll)
</script>

