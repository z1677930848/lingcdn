<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">更新日志</h1>
        <p class="subtitle">任务 {{ shortId }}</p>
      </div>
      <div class="header-actions">
        <t-switch v-model="autoScroll" />
        <span class="muted">自动滚动</span>
        <t-button variant="outline" @click="load" :loading="loading">刷新</t-button>
        <RouterLink to="/admin/dashboard/upgrade" class="link">
          <t-button variant="outline">返回</t-button>
        </RouterLink>
      </div>
    </div>

    <t-card v-if="error" class="section-card" bordered>
      <div class="section-body">
        <p class="error">{{ error }}</p>
        <t-button size="small" variant="outline" @click="load">重试</t-button>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div ref="boxRef" class="log-box">
          <div v-if="lines.length === 0" class="log-empty">暂无日志（任务刚创建时可能需要等待几秒）</div>
          <div v-else>
            <div v-for="(line, idx) in lines" :key="`${idx}-${line}`" class="log-line">{{ line }}</div>
          </div>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import { MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"

const route = useRoute()
const taskId = computed(() => String(route.params.id || "").trim())
const shortId = computed(() => (taskId.value ? taskId.value.slice(0, 12) : "-"))

const lines = ref<string[]>([])
const loading = ref(true)
const error = ref("")
const autoScroll = ref(true)
const boxRef = ref<HTMLDivElement | null>(null)

let timer: ReturnType<typeof setInterval> | null = null

const load = async () => {
  if (!taskId.value) return
  try {
    loading.value = true
    const out = await api.getUpgradeTaskLogs(taskId.value)
    lines.value = (out.logs || []).map((l) => {
      const ts = l.ts ? new Date(l.ts).toLocaleString("zh-CN") : "-"
      const nid = String(l.node_id || "").trim()
      const prefix = `${ts} [${String(l.level || "").toUpperCase()}]${nid ? ` (${nid})` : ""}`
      return `${prefix} ${String(l.message || "")}`.trim()
    })
    error.value = ""
  } catch (err: any) {
    error.value = err.message || "加载任务日志失败"
    MessagePlugin.error(error.value)
  } finally {
    loading.value = false
  }
}

watch(
  () => lines.value.length,
  async () => {
    if (!autoScroll.value) return
    await nextTick()
    if (boxRef.value) {
      boxRef.value.scrollTop = boxRef.value.scrollHeight
    }
  }
)

onMounted(() => {
  load()
  timer = setInterval(load, 1200)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>

<style scoped>
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
}

.muted {
  font-size: 12px;
  color: #94a3b8;
}

.log-box {
  height: 520px;
  overflow: auto;
  background: #0b1020;
  color: #d6e4ff;
  border-radius: 8px;
  padding: 12px;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 12px;
  line-height: 1.6;
  border: 1px solid rgba(255, 255, 255, 0.08);
}

.log-line {
  white-space: pre-wrap;
  word-break: break-word;
}

.log-empty {
  color: rgba(214, 228, 255, 0.7);
}

.link {
  text-decoration: none;
}

.error {
  color: #ef4444;
  margin-bottom: 8px;
}

@media (max-width: 768px) {
  .header-actions {
    width: 100%;
    flex-wrap: wrap;
  }

  .header-actions > * {
    flex-shrink: 0;
  }

  .log-box {
    height: 360px;
  }
}
</style>

