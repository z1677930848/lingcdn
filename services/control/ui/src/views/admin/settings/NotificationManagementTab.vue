<template>
  <div class="settings-tab">
    <ErrorState v-if="error" :message="error" @retry="loadSettings" />
    <LoadingState v-else-if="loading" />
    <el-form v-else label-position="top" :disabled="saving">
      <el-form-item v-for="item in notificationItems" :key="item.key" label="">
        <div class="notify-item">
          <div class="notify-info">
            <div class="notify-label">{{ item.label }}</div>
            <div class="notify-desc">{{ item.description }}</div>
          </div>
          <el-switch v-model="settings[item.key]" />
        </div>
      </el-form-item>

      <div class="actions">
        <el-button type="primary" @click="handleSave" :loading="saving">保存</el-button>
      </div>
    </el-form>
  </div>
</template>

<script setup lang="ts">
import { onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

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
    label: "工单用户动态",
    description: "用户新建或回复工单时通知（钉钉/企微/飞书 Webhook）",
  },
] as const

const loading = ref(false)
const error = ref("")
const saving = ref(false)

const loadSettings = async () => {
  try {
    loading.value = true
    error.value = ""
    const { settings: data } = await api.getSettings()
    if (data) {
      settings.value = {
        notify_node_resource: Boolean(data.notify_node_resource ?? false),
        notify_node_monitor: Boolean(data.notify_node_monitor ?? false),
        notify_ticket_reply: Boolean(data.notify_ticket_reply ?? false),
      }
    }
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


