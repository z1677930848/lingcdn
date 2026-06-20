<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统任务</h1>
        <p class="subtitle">查看与管理系统后台任务运行状态</p>
      </div>
      <div class="header-actions">
        <el-button plain @click="load" :loading="loading">刷新</el-button>
      </div>
    </div>

    <div class="stats">
      <el-card v-for="item in stats" :key="item.label" class="stat-card">
        <div class="stat-label">{{ item.label }}</div>
        <div class="stat-value">{{ item.value }}</div>
      </el-card>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <el-card class="section-card">
      <div class="section-body">
        <div class="filters">
          <EpSelect v-model="source" :options="sourceOptions" style="width:160px" />
          <EpSelect v-model="type" :options="typeOptions" style="width:220px" filterable />
          <EpSelect v-model="status" :options="statusOptions" style="width:160px" />
          <EpSelect v-model="relID" :options="relIDOptions" style="width:180px" filterable />
          <el-input v-model="idQuery" style="width:220px" placeholder="搜索任务 ID" />
          <el-button type="primary" @click="load" :loading="loading">查询</el-button>
          <el-button plain @click="resetFilters">重置</el-button>
        </div>

        <div class="admin-desktop-only">
          <EpDataTable
            :data="pagedTasks"
            :columns="columns"
            row-key="id"
            hover
            stripe
            size="default"
            :loading="loading"
            :pagination="pagination"
            empty-text="暂无任务"
          />
        </div>

        <div class="admin-mobile-only">
          <div v-if="loading" style="text-align:center;padding:24px;color:var(--app-text-faint)">加载中…</div>
          <div v-else-if="pagedTasks.length === 0" style="text-align:center;padding:24px;color:var(--app-text-faint)">暂无任务</div>
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
                <el-button
                  v-if="row.retryable"
                  size="small"
                  plain
                  :loading="retryingId === row.id"
                  @click="handleRetry(row)"
                >重试</el-button>
                <router-link v-if="row.detail_url" :to="row.detail_url" class="link">
                  <el-button size="small" plain>查看详情</el-button>
                </router-link>
                <el-button
                  v-else
                  size="small"
                  plain
                  @click="detailTask = row; detailVisible = true"
                >查看详情</el-button>
              </div>
            </div>
          </div>
          <div v-if="pagedTasks.length > 0" style="padding:12px 0">
            <el-pagination
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
    </el-card>

    <EpDialog append-to-body v-model="detailVisible" header="任务详情" width="600">
      <template #footer>
        <el-button plain @click="detailVisible = false">关闭</el-button>
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
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, h, onMounted, ref } from "vue"
import { RouterLink } from "vue-router"
import { MessagePlugin } from "@/lib/ep-message"
import { ElButton } from "element-plus"
import { api, type SystemTask, type SystemTaskSummary } from "@/lib/api"
import { formatTime, formatTimeFull } from "@/lib/time"
import ErrorState from "@/components/common/ErrorState.vue"

const statusMeta: Record<string, { text: string; color: string; bg: string }> = {
  pending: { text: "等待中", color: "var(--app-text-faint)", bg: "rgba(148, 163, 184, 0.12)" },
  running: { text: "运行中", color: "#409EFF", bg: "rgba(64, 158, 255, 0.12)" },
  success: { text: "成功", color: "var(--app-success-strong)", bg: "rgba(31, 157, 91, 0.12)" },
  failed: { text: "失败", color: "var(--app-danger)", bg: "rgba(239, 68, 68, 0.12)" },
  unknown: { text: "未知", color: "var(--app-border-strong)", bg: "rgba(203, 213, 225, 0.2)" },
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
const error = ref("")
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
    error.value = ""
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
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载任务失败"
    error.value = msg
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
          ? h(ElButton, { size: "small",
                link: true,
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
          : h(ElButton, { size: "small",
                link: true,
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

