<template>
  <div class="settings-tab">
    <t-form layout="vertical" label-align="top" :disabled="loading || saving">
      <t-form-item v-for="item in notificationItems" :key="item.key" label="">
        <div class="notify-item">
          <div class="notify-info">
            <div class="notify-label">{{ item.label }}</div>
            <div class="notify-desc">{{ item.description }}</div>
          </div>
          <t-switch v-model="settings[item.key]" />
        </div>
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

type NotificationTypes = {
  notify_node_resource: boolean
  notify_node_monitor: boolean
  notify_ticket_reply: boolean
}

const settings = ref<NotificationTypes>({
  notify_node_resource: false,
  notify_node_monitor: false,
  notify_ticket_reply: false,
})

const notificationItems = [
  {
    key: "notify_node_resource",
    label: "节点资源监控",
    description: "实时监控节点资源使用情况",
  },
  {
    key: "notify_node_monitor",
    label: "节点存活检测",
    description: "监控节点在线状态，离线时告警",
  },
  {
    key: "notify_ticket_reply",
    label: "工单回复通知",
    description: "客户回复工单通知",
  },
] as const

const loading = ref(false)
const saving = ref(false)

const loadSettings = async () => {
  try {
    loading.value = true
    const { settings: data } = await api.getSettings()
    if (data) {
      settings.value = {
        notify_node_resource: Boolean(data.notify_node_resource ?? false),
        notify_node_monitor: Boolean(data.notify_node_monitor ?? false),
        notify_ticket_reply: Boolean(data.notify_ticket_reply ?? false),
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
.notify-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  padding: 10px 14px;
  border: 1px solid var(--app-border);
  border-radius: var(--app-input-radius);
  background: var(--app-surface);
  transition: background var(--app-anim-fast) var(--app-easing-standard),
              border-color var(--app-anim-fast) var(--app-easing-standard);
}

.notify-item:hover {
  background: var(--app-surface-muted);
  border-color: var(--app-border-strong);
}

.notify-info {
  display: flex;
  flex-direction: column;
  gap: 4px;
  min-width: 0;
}

.notify-label {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-strong);
}

.notify-desc {
  font-size: 12px;
  color: var(--app-text-faint);
}

.actions {
  margin-top: 12px;
}
</style>

