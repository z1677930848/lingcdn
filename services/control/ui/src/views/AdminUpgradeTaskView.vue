<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">更新日志</h1>
        <p class="subtitle">任务 {{ shortId }}</p>
      </div>
      <div class="header-actions">
        <el-switch v-model="autoScroll" />
        <span class="muted">自动滚动</span>
        <el-button plain @click="load" :loading="loading">刷新</el-button>
        <RouterLink to="/admin/dashboard/upgrade" class="link">
          <el-button plain>返回</el-button>
        </RouterLink>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <el-card class="section-card">
      <div class="section-body">
        <div ref="boxRef" class="log-box log-box--tall">
          <div v-if="lines.length === 0" class="log-empty">暂无日志（任务刚创建时可能需要等待几秒）</div>
          <div v-else>
            <div v-for="(line, idx) in lines" :key="`${idx}-${line}`" class="log-line">{{ line }}</div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onMounted, onUnmounted, ref, watch } from "vue"
import { useRoute } from "vue-router"
import { api } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

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

