<template>
  <div class="settings-tab">
    <t-form layout="vertical" label-align="top" :disabled="loading || saving">
      <t-form-item label="邮件通知">
        <t-switch v-model="settings.email_enabled" />
      </t-form-item>

      <t-form-item label="钉钉群机器人通知">
        <t-switch v-model="settings.dingtalk_enabled" />
      </t-form-item>
      <t-form-item v-if="settings.dingtalk_enabled" label="">
        <t-input v-model="settings.dingtalk_webhook" placeholder="钉钉机器人 Webhook URL" />
      </t-form-item>

      <t-form-item label="微信群机器人通知">
        <t-switch v-model="settings.wechat_enabled" />
      </t-form-item>
      <t-form-item v-if="settings.wechat_enabled" label="">
        <t-input v-model="settings.wechat_webhook" placeholder="微信机器人 Webhook URL" />
      </t-form-item>

      <t-form-item label="飞书群机器人通知">
        <t-switch v-model="settings.feishu_enabled" />
      </t-form-item>
      <t-form-item v-if="settings.feishu_enabled" label="">
        <t-input v-model="settings.feishu_webhook" placeholder="飞书机器人 Webhook URL" />
      </t-form-item>

      <div class="actions">
        <t-button theme="primary" @click="handleSave" :loading="saving">保存</t-button>
      </div>
    </t-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"

type NotificationSettings = {
  email_enabled: boolean
  dingtalk_enabled: boolean
  dingtalk_webhook: string
  wechat_enabled: boolean
  wechat_webhook: string
  feishu_enabled: boolean
  feishu_webhook: string
}

const settings = ref<NotificationSettings>({
  email_enabled: false,
  dingtalk_enabled: false,
  dingtalk_webhook: "",
  wechat_enabled: false,
  wechat_webhook: "",
  feishu_enabled: false,
  feishu_webhook: "",
})

const loading = ref(false)
const saving = ref(false)

const loadSettings = async () => {
  try {
    loading.value = true
    const { settings: data } = await api.getSettings()
    if (data) {
      settings.value = {
        email_enabled: Boolean(data.email_enabled ?? false),
        dingtalk_enabled: Boolean(data.dingtalk_enabled ?? false),
        dingtalk_webhook: String(data.dingtalk_webhook ?? ""),
        wechat_enabled: Boolean(data.wechat_enabled ?? false),
        wechat_webhook: String(data.wechat_webhook ?? ""),
        feishu_enabled: Boolean(data.feishu_enabled ?? false),
        feishu_webhook: String(data.feishu_webhook ?? ""),
      }
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载设置失败")
  } finally {
    loading.value = false
  }
}

const handleSave = async () => {
  saving.value = true
  try {
    await api.updateSettings(settings.value)
    MessagePlugin.success("设置保存成功")
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存设置失败")
  } finally {
    saving.value = false
  }
}

onMounted(() => {
  loadSettings()
})
</script>

<style scoped>
.actions {
  margin-top: 12px;
}
</style>

