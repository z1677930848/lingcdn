<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">发布任务详情</h1>
        <p class="subtitle">任务 ID：{{ taskId || '-' }}</p>
      </div>
      <div class="header-actions">
        <el-button plain @click="goBack">返回</el-button>
        <el-button plain @click="() => load(true)" :loading="loading">刷新</el-button>
      </div>
    </div>

    <el-card v-if="!taskId" class="section-card">
      <div class="section-body">缺少任务 ID 参数</div>
    </el-card>

    <template v-else>
      <LoadingState v-if="loading && !task" />
      <ErrorState v-else-if="error && !task" :message="error" @retry="() => load(true)" />
      <template v-else>
      <el-alert v-if="error" theme="error" :message="error" style="margin-bottom: 16px" />
      <el-card class="section-card">
        <div class="section-body">
          <div class="meta-strip">
            <span class="meta">版本：{{ task?.version || '-' }}</span>
            <span class="meta">触发：{{ task?.trigger || '-' }}</span>
            <span class="meta">原因：{{ task?.reason || '-' }}</span>
            <span class="meta">开始：{{ formatDateTime(task?.started_at) }}</span>
            <span class="meta">结束：{{ formatDateTime(task?.completed_at) }}</span>
            <span class="meta">
              状态：
              <span class="badge" :style="{ background: statusMeta.bg, color: statusMeta.color }">{{ statusMeta.text }}</span>
            </span>
          </div>

          <div class="meta-strip">
            <span class="meta">节点总数：{{ task?.total_nodes ?? 0 }}</span>
            <span class="meta success">成功：{{ task?.success_nodes ?? 0 }}</span>
            <span class="meta danger">失败：{{ task?.failed_nodes ?? 0 }}</span>
            <div class="auto-refresh">
              <el-switch v-model="autoRefresh" />
              <span class="muted">自动刷新（8s）</span>
            </div>
          </div>
        </div>
      </el-card>

      <el-card class="section-card">
        <div class="admin-desktop-only">
          <EpDataTable
            :data="nodes"
            :columns="columns"
            row-key="node_id"
            hover
            stripe
            size="default"
            :loading="loading"
            empty-text="暂无节点"
          />
        </div>

        <div class="admin-mobile-only">
          <div v-if="loading" style="text-align:center;padding:24px;color:var(--app-text-faint)">加载中…</div>
          <div v-else-if="nodes.length === 0" style="text-align:center;padding:24px;color:var(--app-text-faint)">暂无节点</div>
          <div v-else class="admin-mobile-cards">
            <div v-for="row in nodes" :key="row.node_id" class="admin-mobile-card">
              <div class="admin-mobile-card-header">
                <div class="admin-mobile-card-title" style="font-size:13px;word-break:break-all">{{ row.node_id }}</div>
                <div class="admin-mobile-card-tags">
                  <span
                    :style="{
                      display: 'inline-flex',
                      padding: '2px 8px',
                      borderRadius: '2px',
                      fontSize: '12px',
                      background: (statusMetaMap[row.status] || statusMetaMap.unknown).bg,
                      color: (statusMetaMap[row.status] || statusMetaMap.unknown).color,
                    }"
                  >{{ (statusMetaMap[row.status] || statusMetaMap.unknown).text }}</span>
                </div>
              </div>
              <div class="admin-mobile-card-rows">
                <div v-if="row.message" class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">消息</span>
                  <span class="admin-mobile-card-value" style="word-break:break-word">{{ row.message }}</span>
                </div>
              </div>
            </div>
          </div>
        </div>
      </el-card>
      </template>
    </template>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, h, onMounted, onUnmounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { api, type PublishTask, type PublishTaskNode } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const route = useRoute()
const router = useRouter()

const taskId = computed(() => String(route.params.id || "").trim())
const task = ref<PublishTask | null>(null)
const nodes = ref<PublishTaskNode[]>([])
const loading = ref(true)
const error = ref("")
const autoRefresh = ref(true)

const statusMetaMap: Record<string, { text: string; color: string; bg: string }> = {
  running: { text: "运行中", color: "#409EFF", bg: "rgba(64, 158, 255, 0.12)" },
  success: { text: "成功", color: "var(--app-success-strong)", bg: "rgba(31, 157, 91, 0.12)" },
  failed: { text: "失败", color: "var(--app-danger)", bg: "rgba(245, 34, 45, 0.12)" },
  unknown: { text: "未知", color: "var(--app-border-strong)", bg: "rgba(203, 213, 225, 0.2)" },
}

const statusMeta = computed(() => statusMetaMap[task.value?.status || "unknown"] || statusMetaMap.unknown)

const load = async (showLoading = true) => {
  if (!taskId.value) return
  try {
    if (showLoading) loading.value = true
    const res = await api.getPublishTask(taskId.value)
    task.value = res.task || null
    nodes.value = res.nodes || []
    error.value = ""
  } catch (err: unknown) {
    error.value = err instanceof Error ? err.message : "加载发布任务失败"
  } finally {
    if (showLoading) loading.value = false
  }
}

const columns = computed(() => [
  {
    colKey: "node_id",
    title: "节点 ID",
    width: 220,
    cell: (_h: any, { row }: { row: PublishTaskNode }) => row.node_id,
  },
  {
    colKey: "status",
    title: "状态",
    width: 100,
    cell: (_h: any, { row }: { row: PublishTaskNode }) => {
      const meta = statusMetaMap[row.status] || statusMetaMap.unknown
      return h(
        "span",
        {
          style: {
            display: "inline-flex",
            padding: "2px 8px",
            borderRadius: "2px",
            fontSize: "12px",
            background: meta.bg,
            color: meta.color,
          },
        },
        meta.text
      )
    },
  },
  {
    colKey: "message",
    title: "消息",
    cell: (_h: any, { row }: { row: PublishTaskNode }) => row.message || "-",
  },
])

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  return new Date(value).toLocaleString("zh-CN")
}

const goBack = () => {
  router.back()
}

let timer: ReturnType<typeof setInterval> | null = null

onMounted(() => {
  load()
  timer = setInterval(() => {
    if (autoRefresh.value) load(false)
  }, 8000)
})

onUnmounted(() => {
  if (timer) clearInterval(timer)
})
</script>
