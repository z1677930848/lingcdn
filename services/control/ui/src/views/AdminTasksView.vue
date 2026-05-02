<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统任务</h1>
        <p class="subtitle">查看与管理系统后台任务运行状态</p>
      </div>
      <div class="header-actions">
        <t-button variant="outline" @click="load" :loading="loading">刷新</t-button>
      </div>
    </div>

    <div class="stats">
      <t-card v-for="item in stats" :key="item.label" class="stat-card">
        <div class="stat-label">{{ item.label }}</div>
        <div class="stat-value">{{ item.value }}</div>
      </t-card>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <div class="filters">
          <t-select v-model="source" :options="sourceOptions" style="width:160px" />
          <t-select v-model="type" :options="typeOptions" style="width:220px" filterable />
          <t-select v-model="status" :options="statusOptions" style="width:160px" />
          <t-select v-model="relID" :options="relIDOptions" style="width:180px" filterable />
          <t-input v-model="idQuery" style="width:220px" placeholder="搜索任务 ID" />
          <t-button theme="primary" @click="load" :loading="loading">查询</t-button>
          <t-button variant="outline" @click="resetFilters">重置</t-button>
        </div>

        <div class="admin-desktop-only">
          <t-table
            :data="pagedTasks"
            :columns="columns"
            row-key="id"
            bordered
            hover
            stripe
            size="medium"
            :loading="loading"
            :pagination="pagination"
            empty="暂无任务"
          />
        </div>

        <div class="admin-mobile-only">
          <div v-if="loading" style="text-align:center;padding:24px;color:#94a3b8">加载中…</div>
          <div v-else-if="pagedTasks.length === 0" style="text-align:center;padding:24px;color:#94a3b8">暂无任务</div>
          <div v-else class="admin-mobile-cards">
            <div v-for="row in pagedTasks" :key="row.id" class="admin-mobile-card">
              <div class="admin-mobile-card-header">
                <div class="admin-mobile-card-title">{{ typeLabel(row.type) }}</div>
                <div class="admin-mobile-card-tags">
                  <span
                    :style="{
                      display: 'inline-flex',
                      padding: '2px 8px',
                      borderRadius: '2px',
                      fontSize: '12px',
                      background: (statusMeta[row.status] || statusMeta.unknown).bg,
                      color: (statusMeta[row.status] || statusMeta.unknown).color,
                    }"
                  >{{ (statusMeta[row.status] || statusMeta.unknown).text }}</span>
                </div>
              </div>
              <div class="admin-mobile-card-subtitle">{{ sourceLabel(row.source) }}</div>
              <div class="admin-mobile-card-rows">
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">ID</span>
                  <span class="admin-mobile-card-value mono" style="font-size:11px;word-break:break-all">{{ row.id }}</span>
                </div>
                <div v-if="row.message" class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">消息</span>
                  <span class="admin-mobile-card-value" style="max-width:200px;overflow:hidden;text-overflow:ellipsis;white-space:nowrap">{{ row.message }}</span>
                </div>
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">时间</span>
                  <span class="admin-mobile-card-value mono" style="font-size:11px">{{ formatTime(row.created_at) }} → {{ formatTime(row.updated_at) }}</span>
                </div>
                <div class="admin-mobile-card-row">
                  <span class="admin-mobile-card-label">子任务</span>
                  <span class="admin-mobile-card-value">{{ row.sub_tasks ?? 0 }}</span>
                </div>
              </div>
              <div class="admin-mobile-card-actions">
                <t-button
                  v-if="row.retryable"
                  size="small"
                  variant="outline"
                  :loading="retryingId === row.id"
                  @click="handleRetry(row)"
                >重试</t-button>
                <router-link v-if="row.detail_url" :to="row.detail_url" class="link">
                  <t-button size="small" variant="outline">查看详情</t-button>
                </router-link>
                <t-button
                  v-else
                  size="small"
                  variant="outline"
                  @click="detailTask = row; detailVisible = true"
                >查看详情</t-button>
              </div>
            </div>
          </div>
          <div v-if="pagedTasks.length > 0" style="padding:12px 0">
            <t-pagination
              :current="page"
              :page-size="pageSize"
              :total="tasks.length"
              show-jumper
              :page-size-options="[10, 20, 50, 100]"
              @change="(info: any) => { page = info.current; pageSize = info.pageSize }"
            />
          </div>
        </div>
      </div>
    </t-card>

    <t-dialog v-model:visible="detailVisible" header="任务详情" width="600">
      <template #footer>
        <t-button variant="outline" @click="detailVisible = false">关闭</t-button>
      </template>
      <div v-if="detailTask" class="detail">
        <div class="detail-grid">
          <div>
            <div class="detail-label">任务 ID</div>
            <div class="mono">{{ detailTask.id }}</div>
          </div>
          <div>
            <div class="detail-label">来源</div>
            <div>{{ sourceLabel(detailTask.source) }}</div>
          </div>
          <div>
            <div class="detail-label">类型</div>
            <div>{{ typeLabel(detailTask.type) }}<span class="detail-code">{{ detailTask.type }}</span></div>
          </div>
          <div>
            <div class="detail-label">状态</div>
            <div>
              <span
                :style="{
                  display: 'inline-flex',
                  padding: '2px 8px',
                  borderRadius: '2px',
                  fontSize: '12px',
                  background: (statusMeta[detailTask.status] || statusMeta.unknown).bg,
                  color: (statusMeta[detailTask.status] || statusMeta.unknown).color,
                }"
              >{{ (statusMeta[detailTask.status] || statusMeta.unknown).text }}</span>
            </div>
          </div>
          <div v-if="detailTask.rel_id">
            <div class="detail-label">关联 ID</div>
            <div class="mono">{{ detailTask.rel_id }}</div>
          </div>
          <div>
            <div class="detail-label">子任务</div>
            <div>{{ detailTask.sub_tasks ?? 0 }}</div>
          </div>
          <div>
            <div class="detail-label">创建时间</div>
            <div :title="formatTimeFull(detailTask.created_at)">{{ formatTime(detailTask.created_at) }}</div>
          </div>
          <div>
            <div class="detail-label">更新时间</div>
            <div :title="formatTimeFull(detailTask.updated_at)">{{ formatTime(detailTask.updated_at) }}</div>
          </div>
        </div>
        <div>
          <div class="detail-label">消息</div>
          <pre class="detail-pre">{{ detailTask.message || '-' }}</pre>
        </div>
        <div v-if="detailTask.status === 'failed'" class="detail-hint">
          <template v-if="detailTask.type === 'system.report'">
            <strong>system.report 失败常见原因：</strong><br/>
            &bull; <strong>status 401</strong>：节点令牌失效或过期，请检查节点 token<br/>
            &bull; <strong>status 403</strong>：节点未被授权或被禁用<br/>
            &bull; <strong>网络异常</strong>：节点到 gRPC 控制面网络不可达
          </template>
          <template v-else-if="detailTask.type === 'license.verify'">
            <strong>license.verify 失败：</strong>请检查授权状态、网络连通性与时间同步
          </template>
          <template v-else-if="detailTask.type?.startsWith('dns.')">
            <strong>DNS 任务失败：</strong>请检查 DNS 提供商凭据、Token 权限与 Zone 配置是否正确
          </template>
        </div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { RouterLink } from "vue-router"
import { MessagePlugin, Button } from "tdesign-vue-next"
import { api, type SystemTask, type SystemTaskSummary } from "@/lib/api"
import { formatTime, formatTimeFull } from "@/lib/time"

const statusMeta: Record<string, { text: string; color: string; bg: string }> = {
  pending: { text: "等待中", color: "#94a3b8", bg: "rgba(148, 163, 184, 0.12)" },
  running: { text: "运行中", color: "#635BFF", bg: "rgba(99, 91, 255, 0.12)" },
  success: { text: "成功", color: "#1f9d5b", bg: "rgba(31, 157, 91, 0.12)" },
  failed: { text: "失败", color: "#ef4444", bg: "rgba(239, 68, 68, 0.12)" },
  unknown: { text: "未知", color: "#cbd5e1", bg: "rgba(203, 213, 225, 0.2)" },
}

const typeLabelMap: Record<string, string> = {
  "system.report": "节点上报",
  "license.verify": "授权校验",
  "dns.sync": "DNS 同步",
  "dns.recover": "DNS 恢复",
  "dns.cleanup": "DNS 清理",
  "upgrade.node": "节点升级",
  "upgrade.control": "控制台升级",
  "publish.config": "配置发布",
  "sync": "DNS 同步",
  "recover": "DNS 恢复",
  "cleanup": "DNS 清理",
}

const sourceLabelMap: Record<string, string> = {
  dns: "DNS",
  upgrade: "升级",
  system: "系统",
  publish: "发布",
  background: "后台",
}

const typeLabel = (type: string) => typeLabelMap[type] || type
const sourceLabel = (source: string) => sourceLabelMap[source] || source

const loading = ref(false)
const summary = ref<SystemTaskSummary | null>(null)
const tasks = ref<SystemTask[]>([])

const source = ref("")
const type = ref("")
const status = ref("")
const relID = ref("")
const idQuery = ref("")

const page = ref(1)
const pageSize = ref(10)

const detailVisible = ref(false)
const detailTask = ref<SystemTask | null>(null)
const retryingId = ref("")

const load = async () => {
  try {
    loading.value = true
    const res = await api.listSystemTasks({
      source: source.value || undefined,
      type: type.value || undefined,
      status: status.value || undefined,
      rel_id: relID.value || undefined,
      id: idQuery.value || undefined,
      limit: 500,
    })
    summary.value = res.summary
    tasks.value = (res.tasks || []).filter(t => t.type !== "license.verify")
    page.value = 1
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载任务失败")
  } finally {
    loading.value = false
  }
}

const handleRetry = async (row: SystemTask) => {
  try {
    retryingId.value = row.id
    const res = await api.retrySystemTask(row.id)
    MessagePlugin.success(`已重试，任务 ID：${res.task_id}`)
    await load()
  } catch (err: any) {
    MessagePlugin.error(err.message || "重试失败")
  } finally {
    retryingId.value = ""
  }
}

const stats = computed(() => [
  { label: "任务总数", value: summary.value?.tasks ?? "-" },
  { label: "子任务", value: summary.value?.sub_tasks ?? "-" },
  { label: "等待中", value: summary.value?.pending ?? "-" },
  { label: "运行中", value: summary.value?.running ?? "-" },
  { label: "成功", value: summary.value?.success ?? "-" },
  { label: "失败", value: summary.value?.failed ?? "-" },
  { label: "未知", value: summary.value?.unknown ?? "-" },
])

const typeOptions = computed(() => {
  const set = new Set<string>()
  for (const t of tasks.value) {
    if (t.type) set.add(t.type)
  }
  return [
    { label: "全部类型", value: "" },
    ...Array.from(set)
      .sort((a, b) => a.localeCompare(b))
      .map((v) => ({ label: typeLabel(v), value: v })),
  ]
})

const sourceOptions = computed(() => {
  const set = new Set<string>()
  for (const t of tasks.value) {
    if (t.source) set.add(t.source)
  }
  return [
    { label: "全部来源", value: "" },
    ...Array.from(set)
      .sort((a, b) => a.localeCompare(b))
      .map((v) => ({ label: sourceLabel(v), value: v })),
  ]
})

const relIDOptions = computed(() => {
  const set = new Set<string>()
  for (const t of tasks.value) {
    if (t.rel_id) set.add(t.rel_id)
  }
  return [
    { label: "全部 RelID", value: "" },
    ...Array.from(set)
      .sort((a, b) => a.localeCompare(b))
      .slice(0, 50)
      .map((v) => ({ label: v, value: v })),
  ]
})

const statusOptions = [
  { label: "全部状态", value: "" },
  { label: "等待中", value: "pending" },
  { label: "运行中", value: "running" },
  { label: "成功", value: "success" },
  { label: "失败", value: "failed" },
  { label: "未知", value: "unknown" },
]

const pagedTasks = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return tasks.value.slice(start, start + pageSize.value)
})

const columns = computed(() => [
  {
    colKey: "source",
    title: "来源",
    width: 90,
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", null, sourceLabel(row.source)),
  },
  {
    colKey: "id",
    title: "ID",
    width: 220,
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", { class: "mono" }, row.id),
  },
  {
    colKey: "rel_id",
    title: "RelID",
    width: 90,
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", { class: "mono" }, row.rel_id || "-"),
  },
  {
    colKey: "type",
    title: "类型",
    width: 160,
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", null, typeLabel(row.type)),
  },
  {
    colKey: "sub_tasks",
    title: "子任务",
    width: 100,
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", { class: "mono" }, String(row.sub_tasks ?? 0)),
  },
  {
    colKey: "message",
    title: "消息",
    cell: (_h: any, { row }: { row: SystemTask }) => h("span", { class: "message", title: row.message }, row.message),
  },
  {
    colKey: "time",
    title: "创建 / 更新",
    width: 240,
    cell: (_h: any, { row }: { row: SystemTask }) => {
      const start = formatTime(row.created_at)
      const end = formatTime(row.updated_at)
      return h("span", { class: "mono" }, `${start} → ${end}`)
    },
  },
  {
    colKey: "status",
    title: "状态",
    width: 100,
    cell: (_h: any, { row }: { row: SystemTask }) => {
      const meta = statusMeta[row.status] || statusMeta.unknown
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
    colKey: "actions",
    title: "操作",
    width: 160,
    cell: (_h: any, { row }: { row: SystemTask }) =>
      h("div", { style: "display:flex;gap:10px" }, [
        row.retryable
          ? h(
              Button,
              {
                size: "small",
                variant: "text",
                loading: retryingId.value === row.id,
                onClick: async () => {
                  try {
                    retryingId.value = row.id
                    const res = await api.retrySystemTask(row.id)
                    MessagePlugin.success(`已重试，任务 ID：${res.task_id}`)
                    await load()
                  } catch (err: any) {
                    MessagePlugin.error(err.message || "重试失败")
                  } finally {
                    retryingId.value = ""
                  }
                },
              },
              () => "重试"
            )
          : null,
        row.detail_url
          ? h(
              RouterLink,
              { to: row.detail_url, class: "link" },
              { default: () => "查看详情" }
            )
          : h(
              Button,
              {
                size: "small",
                variant: "text",
                onClick: () => {
                  detailTask.value = row
                  detailVisible.value = true
                },
              },
              () => "查看详情"
            ),
      ]),
  },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: tasks.value.length,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pageInfo: { current: number; pageSize: number }) => {
    page.value = pageInfo.current
    pageSize.value = pageInfo.pageSize
  },
}))

const resetFilters = () => {
  source.value = ""
  type.value = ""
  status.value = ""
  relID.value = ""
  idQuery.value = ""
}

onMounted(() => {
  load()
})
</script>

<style scoped>
.header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.muted {
  font-size: 12px;
  color: #94a3b8;
}

.stats {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(140px, 1fr));
  gap: 12px;
  min-width: 0;
}

.stat-card {
  padding: 12px;
  border: 1px solid #e2e8f0;
}

.stat-label {
  font-size: 12px;
  color: #94a3b8;
}

.stat-value {
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}

.filters {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 12px;
}

.page {
  min-width: 0;
}

.section-card {
  min-width: 0;
}

.section-body {
  min-width: 0;
}

.section-body :deep(.t-table) {
  min-width: 100%;
}

@media (max-width: 768px) {
  .header-actions {
    width: 100%;
    justify-content: flex-start;
  }

  .stats {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .filters > * {
    width: 100% !important;
    min-width: 0 !important;
  }

  .filters :deep(.t-button) {
    min-height: 44px;
  }

  .section-body {
    padding: 12px;
  }

  .message {
    max-width: 220px;
  }
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.message {
  display: inline-block;
  max-width: 360px;
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.link {
  color: var(--td-brand-color);
  font-size: 12px;
  text-decoration: none;
}

.detail {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px 16px;
}

.detail-label {
  font-size: 12px;
  color: #94a3b8;
  margin-bottom: 2px;
}

.detail-code {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 11px;
  color: #94a3b8;
  margin-left: 6px;
}

.detail-pre {
  background: #f8fafc;
  border-radius: 8px;
  padding: 8px;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
}

.detail-hint {
  background: #fff7e6;
  border: 1px solid #ffe4b5;
  border-radius: 8px;
  padding: 10px 12px;
  font-size: 12px;
  line-height: 1.8;
  color: #475569;
}
</style>
