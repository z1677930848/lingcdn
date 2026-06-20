<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统升级</h1>
        <p class="subtitle">查看当前版本与执行升级任务</p>
      </div>
      <div class="header-actions">
        <el-switch v-model="autoRefresh" />
        <span class="muted">自动刷新（8s）</span>
        <el-button plain @click="() => loadData(true)" :loading="loading">刷新</el-button>
      </div>
    </div>

    <el-card class="section-card">
      <div class="section-body">
        <div class="row">
          <span class="label">更新通道</span>
          <el-radio-group v-model="channel" variant="default-filled" size="small" @change="handleChannelChange">
            <el-radio-button value="stable">稳定版</el-radio-button>
            <el-radio-button value="beta">测试版</el-radio-button>
          </el-radio-group>
        </div>
      </div>
    </el-card>

    <el-card v-if="error" class="section-card">
      <div class="section-body">
        <span class="error">{{ error }}</span>
      </div>
    </el-card>

    <el-card class="section-card">
      <div class="section-body">
        <div class="upgrade-header">
          <div>
            <div class="muted">版本信息</div>
            <div class="upgrade-version">
              <span>当前 {{ info?.current_version || '-' }}</span>
              <span class="arrow">→</span>
              <span class="latest">最新 {{ info?.latest_version || '-' }}</span>
              <span class="badge" :class="channel === 'beta' ? 'beta' : 'stable'">{{ channelLabel }}</span>
              <el-tag v-if="isLatest" type="success" effect="light" size="small">已是最新</el-tag>
              <el-tag v-else-if="hasUpdate" type="warning" effect="light" size="small">可升级</el-tag>
            </div>
          </div>
          <el-button @click="handleUpgradeControl" :loading="controlLoading" :disabled="isLatest">
            {{ isLatest ? '已是最新' : '执行升级' }}
          </el-button>
        </div>
        <div class="meta-strip">
          <span class="muted">来源：{{ info?.source || '-' }}</span>
          <span class="muted">检查时间：{{ formatDateTime(info?.checked_at) }}</span>
        </div>
        <div v-if="(info?.notes || []).length" class="notes">
          <div v-for="(note, idx) in info?.notes" :key="idx">· {{ note }}</div>
        </div>
      </div>
    </el-card>

    <el-card class="section-card">
      <div class="section-body">
        <div class="panel-title">升级任务</div>
        <div v-if="tasks.length === 0" class="muted">暂无升级任务</div>
        <div v-else class="task-list">
          <div v-for="task in tasks" :key="task.id" class="task-item">
            <div class="task-main">
              <div class="task-title">
                {{ task.type === 'control' ? '控制台升级' : '节点升级' }} → {{ task.target_version || 'latest' }}
              </div>
              <div class="muted">通道：{{ task.channel || 'stable' }} · 创建：{{ formatDateTime(task.created_at) }}</div>
            </div>
            <div class="task-actions">
              <span class="status" :style="statusStyle(task.status)">{{ statusText(task.status) }}</span>
              <span class="muted">{{ task.id.slice(0, 8) }}</span>
              <el-button size="small" plain @click="openLogs(task.id)">查看日志</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <EpDialog append-to-body v-model="logVisible" header="升级日志" width="920px" :footer="null" @close="closeLogs">
      <div class="log-header">
        <div class="log-info">
          <span class="muted">任务</span>
          <span>{{ logTaskId ? logTaskId.slice(0, 12) : '-' }}</span>
          <RouterLink v-if="logTaskId" :to="`/admin/dashboard/upgrade/${encodeURIComponent(logTaskId)}`" class="link">查看详情</RouterLink>
          <span v-if="logLoading" class="muted">加载中...</span>
          <span v-if="logError" class="error">{{ logError }}</span>
        </div>
        <div class="log-actions">
          <el-switch v-model="logAutoScroll" />
          <span class="muted">自动滚动</span>
          <el-button size="small" plain @click="closeLogs">关闭</el-button>
        </div>
      </div>

      <div ref="logBoxRef" class="log-box">
        <div v-if="logLines.length === 0" class="log-empty">暂无日志</div>
        <div v-else>
          <div v-for="(line, idx) in logLines" :key="`${idx}-${line}`" class="log-line">{{ line }}</div>
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { api, type UpgradeInfo, type UpgradeTask } from "@/lib/api"

const channel = ref<"stable" | "beta">("stable")
const info = ref<UpgradeInfo | null>(null)
const tasks = ref<UpgradeTask[]>([])
const loading = ref(true)
const autoRefresh = ref(true)
const error = ref("")
const controlLoading = ref(false)

const logVisible = ref(false)
const logTaskId = ref("")
const logLines = ref<string[]>([])
const logLoading = ref(false)
const logError = ref("")
const logAutoScroll = ref(true)
const logBoxRef = ref<HTMLDivElement | null>(null)

let refreshTimer: ReturnType<typeof setInterval> | null = null
let logTimer: ReturnType<typeof setInterval> | null = null

const channelLabel = computed(() => (channel.value === "beta" ? "测试版" : "稳定版"))

const isLatest = computed(() => {
  if (!info.value) return false
  const current = (info.value.current_version || "").trim()
  const latest = (info.value.latest_version || "").trim()
  return current !== "" && latest !== "" && current === latest
})

const hasUpdate = computed(() => {
  if (!info.value) return false
  const current = (info.value.current_version || "").trim()
  const latest = (info.value.latest_version || "").trim()
  return current !== "" && latest !== "" && current !== latest
})

const loadData = async (showLoading = true) => {
  try {
    if (showLoading) loading.value = true
    error.value = ""
    const [infoRes, tasksRes] = await Promise.all([api.getUpgradeInfo(channel.value), api.getUpgradeTasks()])
    info.value = infoRes
    tasks.value = tasksRes.tasks || []
  } catch (err: any) {
    error.value = err.message || "加载升级信息失败"
  } finally {
    if (showLoading) loading.value = false
  }
}

const handleChannelChange = () => {
  loadData(true)
}

const openLogs = (taskId: string) => {
  const id = String(taskId || "").trim()
  if (!id) return
  logTaskId.value = id
  logLines.value = []
  logError.value = ""
  logVisible.value = true
}

const closeLogs = () => {
  logVisible.value = false
  logTaskId.value = ""
  logLines.value = []
  logError.value = ""
}

const handleUpgradeControl = () => {
  const dlg = DialogPlugin.confirm({
    header: "确认升级",
    body: `确认升级到${channelLabel.value}最新版本？升级过程可能影响服务。`,
    confirmBtn: "确认升级",
    cancelBtn: "取消",
    onConfirm: async () => {
      dlg.destroy()
      try {
        controlLoading.value = true
        const res = await api.upgradeControl(channel.value)
        MessagePlugin.success(res.message || "升级任务已提交")
        if (res.task_id) {
          openLogs(res.task_id)
        }
        await loadData(true)
      } catch (err: any) {
        MessagePlugin.error(err.message || "升级失败")
      } finally {
        controlLoading.value = false
      }
    },
    onClose: () => {
      dlg.destroy()
    },
  })
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  return new Date(value).toLocaleString("zh-CN")
}

const statusText = (status?: string) => {
  const map: Record<string, string> = {
    pending: "等待中",
    running: "运行中",
    completed: "已完成",
    failed: "失败",
  }
  return map[status || ""] || "未知"
}

const statusStyle = (status?: string) => {
  const map: Record<string, { color: string; bg: string }> = {
    pending: { color: "var(--app-text-muted)", bg: "rgba(71, 85, 105, 0.12)" },
    running: { color: "#409EFF", bg: "rgba(64, 158, 255, 0.12)" },
    completed: { color: "#16a34a", bg: "rgba(22, 163, 74, 0.12)" },
    failed: { color: "var(--app-danger)", bg: "rgba(245, 34, 45, 0.12)" },
    unknown: { color: "var(--app-border-strong)", bg: "rgba(203, 213, 225, 0.2)" },
  }
  return map[status || ""] || map.unknown
}

const loadLogLines = async () => {
  if (!logTaskId.value) return
  try {
    logLoading.value = true
    const out = await api.getUpgradeTaskLogs(logTaskId.value)
    const lines = (out.logs || []).map((l) => {
      const ts = l.ts ? new Date(l.ts).toLocaleString("zh-CN") : "-"
      const nid = String(l.node_id || "").trim()
      const prefix = `${ts} [${String(l.level || "").toUpperCase()}]${nid ? ` (${nid})` : ""}`
      return `${prefix} ${String(l.message || "")}`.trim()
    })
    logLines.value = lines
    logError.value = ""
  } catch (err: any) {
    logError.value = err.message || "加载日志失败"
  } finally {
    logLoading.value = false
  }
}

watch(logVisible, async (visible) => {
  if (visible) {
    await loadLogLines()
    if (logTimer) clearInterval(logTimer)
    logTimer = setInterval(loadLogLines, 1200)
  } else if (logTimer) {
    clearInterval(logTimer)
    logTimer = null
  }
})

watch(
  () => logLines.value.length,
  async () => {
    if (!logVisible.value || !logAutoScroll.value) return
    await nextTick()
    if (logBoxRef.value) {
      logBoxRef.value.scrollTop = logBoxRef.value.scrollHeight
    }
  }
)

onMounted(() => {
  loadData(true)
  refreshTimer = setInterval(() => {
    if (autoRefresh.value) loadData(false)
  }, 8000)
})

onUnmounted(() => {
  if (refreshTimer) clearInterval(refreshTimer)
  if (logTimer) clearInterval(logTimer)
})
</script>
