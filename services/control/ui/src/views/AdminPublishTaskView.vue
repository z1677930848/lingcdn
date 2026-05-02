<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">发布任务详情</h1>
        <p class="subtitle">任务 ID：{{ taskId || '-' }}</p>
      </div>
      <div class="header-actions">
        <t-button variant="outline" @click="goBack">返回</t-button>
        <t-button variant="outline" @click="() => load(true)" :loading="loading">刷新</t-button>
      </div>
    </div>

    <t-card v-if="!taskId" class="section-card" bordered>
      <div class="section-body">缺少任务 ID 参数</div>
    </t-card>

    <template v-else>
      <t-card class="section-card" bordered>
        <div class="section-body">
          <div class="meta-row">
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

          <div class="meta-row">
            <span class="meta">节点总数：{{ task?.total_nodes ?? 0 }}</span>
            <span class="meta success">成功：{{ task?.success_nodes ?? 0 }}</span>
            <span class="meta danger">失败：{{ task?.failed_nodes ?? 0 }}</span>
            <div class="auto-refresh">
              <t-switch v-model="autoRefresh" />
              <span class="muted">自动刷新（8s）</span>
            </div>
          </div>
        </div>
      </t-card>

      <t-card class="section-card" bordered>
        <div class="admin-desktop-only">
          <t-table
            :data="nodes"
            :columns="columns"
            row-key="node_id"
            bordered
            hover
            stripe
            size="medium"
            :loading="loading"
            empty="暂无节点"
          />
        </div>

        <div class="admin-mobile-only">
          <div v-if="loading" style="text-align:center;padding:24px;color:#94a3b8">加载中…</div>
          <div v-else-if="nodes.length === 0" style="text-align:center;padding:24px;color:#94a3b8">暂无节点</div>
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
      </t-card>
    </template>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onUnmounted, ref } from "vue"
import { useRoute, useRouter } from "vue-router"
import { MessagePlugin } from "tdesign-vue-next"
import { api, type PublishTask, type PublishTaskNode } from "@/lib/api"

const route = useRoute()
const router = useRouter()

const taskId = computed(() => String(route.params.id || "").trim())
const task = ref<PublishTask | null>(null)
const nodes = ref<PublishTaskNode[]>([])
const loading = ref(true)
const autoRefresh = ref(true)

const statusMetaMap: Record<string, { text: string; color: string; bg: string }> = {
  running: { text: "运行中", color: "#635BFF", bg: "rgba(99, 91, 255, 0.12)" },
  success: { text: "成功", color: "#1f9d5b", bg: "rgba(31, 157, 91, 0.12)" },
  failed: { text: "失败", color: "#ef4444", bg: "rgba(245, 34, 45, 0.12)" },
  unknown: { text: "未知", color: "#cbd5e1", bg: "rgba(203, 213, 225, 0.2)" },
}

const statusMeta = computed(() => statusMetaMap[task.value?.status || "unknown"] || statusMetaMap.unknown)

const load = async (showLoading = true) => {
  if (!taskId.value) return
  try {
    if (showLoading) loading.value = true
    const res = await api.getPublishTask(taskId.value)
    task.value = res.task || null
    nodes.value = res.nodes || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载发布任务失败")
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

<style scoped>
.header-actions {
  display: flex;
  gap: 8px;
}

.meta-row {
  display: flex;
  flex-wrap: wrap;
  gap: 12px;
  margin-bottom: 8px;
}

.meta {
  font-size: 12px;
  color: #94a3b8;
}

.success {
  color: #1f9d5b;
}

.danger {
  color: #ef4444;
}

.badge {
  padding: 2px 8px;
  border-radius: 8px;
  font-size: 12px;
}

.auto-refresh {
  display: flex;
  align-items: center;
  gap: 6px;
}

.muted {
  font-size: 12px;
  color: #94a3b8;
}

@media (max-width: 768px) {
  .meta-row {
    flex-direction: column;
    gap: 6px;
  }
}
</style>
