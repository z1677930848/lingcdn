<template>
  <div class="settings-tab">
    <ErrorState v-if="error" :message="error" @retry="loadSettings" />
    <LoadingState v-else-if="loading" />
    <el-form v-else label-position="top" :disabled="saving">
      <el-form-item label="通知间隔">
        <div class="input-with-unit">
          <el-input-number v-model="settings.notify_interval" :min="0" />
          <span class="unit">分钟</span>
        </div>
        <div class="hint">通知同一节点后，下次通知间隔，输入 0 表示实时通知</div>
      </el-form-item>

      <el-form-item label="节点 CPU 阈值">
        <div class="input-with-unit">
          <el-input-number v-model="settings.threshold_cpu" :min="0" :max="100" />
          <span class="unit">%</span>
        </div>
        <div class="hint">超过即触发通知，输入 0 表示不启用</div>
      </el-form-item>

      <el-form-item label="节点内存阈值">
        <div class="input-with-unit">
          <el-input-number v-model="settings.threshold_memory" :min="0" :max="100" />
          <span class="unit">%</span>
        </div>
        <div class="hint">超过即触发通知，输入 0 表示不启用</div>
      </el-form-item>

      <el-form-item label="节点磁盘阈值">
        <div class="input-with-unit">
          <el-input-number v-model="settings.threshold_disk" :min="0" :max="100" />
          <span class="unit">%</span>
        </div>
        <div class="hint">超过即触发通知，输入 0 表示不启用</div>
      </el-form-item>

      <el-form-item label="节点带宽（上行）阈值">
        <div class="input-with-unit">
          <el-input-number v-model="settings.threshold_bandwidth_up" :min="0" />
          <span class="unit">KB/s</span>
        </div>
        <div class="hint">超过即触发通知，输入 0 表示不启用</div>
      </el-form-item>

      <el-form-item label="节点带宽（下行）阈值">
        <div class="input-with-unit">
          <el-input-number v-model="settings.threshold_bandwidth_down" :min="0" />
          <span class="unit">KB/s</span>
        </div>
        <div class="hint">超过即触发通知，输入 0 表示不启用</div>
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

type ThresholdSettings = {
  notify_interval: number
  threshold_cpu: number
  threshold_memory: number
  threshold_disk: number
  threshold_bandwidth_up: number
  threshold_bandwidth_down: number
}

const settings = ref<ThresholdSettings>({
  notify_interval: 5,
  threshold_cpu: 0,
  threshold_memory: 0,
  threshold_disk: 0,
  threshold_bandwidth_up: 0,
  threshold_bandwidth_down: 0,
})

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
        notify_interval: Number(data.notify_interval ?? 5),
        threshold_cpu: Number(data.threshold_cpu ?? 0),
        threshold_memory: Number(data.threshold_memory ?? 0),
        threshold_disk: Number(data.threshold_disk ?? 0),
        threshold_bandwidth_up: Number(data.threshold_bandwidth_up ?? 0),
        threshold_bandwidth_down: Number(data.threshold_bandwidth_down ?? 0),
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

