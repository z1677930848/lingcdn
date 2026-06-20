<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">工单管理</h1>
        <p class="subtitle">处理用户提交的咨询与技术支持请求</p>
      </div>
      <el-button :loading="loading" @click="loadTickets">刷新</el-button>
    </div>

    <el-card class="section-card" bordered>
      <div class="filters">
        <EpSelect v-model="statusFilter" clearable placeholder="状态" style="width:140px" :options="statusOptions" />
        <el-input v-model="userFilter" clearable placeholder="用户ID" style="width:200px" />
        <el-button type="primary" @click="() => { page = 1; loadTickets() }">查询</el-button>
      </div>
      <LoadingState v-if="loading && tickets.length === 0" />
      <ErrorState v-else-if="error" :message="error" @retry="loadTickets" />
      <EpDataTable v-else :data="tickets" :columns="columns" row-key="id" size="small" empty-text="暂无工单" @row-click="onRowClick" />
      <div v-if="total > 0" class="pager">
        <el-pagination
          :current="page"
          :page-size="pageSize"
          :total="total"
          :page-size-options="[20, 50, 100]"
          show-page-size
          size="small"
          @current-change="onPageChange"
          @page-size-change="onPageSizeChange"
        />
      </div>
    </el-card>

    <el-drawer append-to-body v-model="detailOpen" :title="detail?.subject || '工单详情'" size="520px" :footer="false">
      <div v-if="detail" class="detail-wrap">
        <div class="ticket-meta-grid">
          <div><span class="label">用户</span>{{ detail.username || detail.user_id }}</div>
          <div><span class="label">邮箱</span>{{ detail.email || "-" }}</div>
          <div>
            <span class="label">状态</span>
            <EpSelect v-model="editStatus" size="small" style="width:120px" :options="statusOptions" @change="saveMeta" />
          </div>
          <div>
            <span class="label">优先级</span>
            <EpSelect v-model="editPriority" size="small" style="width:120px" :options="priorityOptions" @change="saveMeta" />
          </div>
        </div>
        <div class="ticket-thread ticket-thread--admin">
          <div v-for="r in replies" :key="r.id" class="ticket-reply" :class="r.author_role === 'admin' ? 'ticket-reply--admin-alt' : 'ticket-reply--user'">
            <div class="ticket-reply-head ticket-reply-head--compact">
              <strong>{{ r.author_name }} ({{ r.author_role === 'admin' ? '管理员' : '用户' }})</strong>
              <span class="muted">{{ formatTime(r.created_at) }}</span>
            </div>
            <div class="ticket-reply-body">{{ r.body }}</div>
          </div>
        </div>
        <div class="ticket-reply-box">
          <el-input v-model="replyText" :autosize="{ minRows: 4 }" placeholder="输入管理员回复" />
          <el-button type="primary" block :loading="replying" @click="submitReply">发送回复</el-button>
        </div>
      </div>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { h, onMounted, ref, watch } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag } from "element-plus"
import { api, type Ticket, type TicketReply } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(false)
const error = ref("")
const tickets = ref<Ticket[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const statusFilter = ref("")
const userFilter = ref("")
const detailOpen = ref(false)
const detail = ref<Ticket | null>(null)
const replies = ref<TicketReply[]>([])
const replyText = ref("")
const replying = ref(false)
const editStatus = ref("open")
const editPriority = ref("normal")

const statusOptions = [
  { label: "待处理", value: "open" },
  { label: "已回复", value: "replied" },
  { label: "已关闭", value: "closed" },
]
const priorityOptions = [
  { label: "低", value: "low" },
  { label: "普通", value: "normal" },
  { label: "高", value: "high" },
]

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN")
}

const columns = [
  { colKey: "subject", title: "主题", ellipsis: true },
  { colKey: "username", title: "用户", width: 120, cell: (_h: unknown, { row }: { row: Ticket }) => row.username || row.user_id },
  {
    colKey: "status",
    title: "状态",
    width: 90,
    cell: (_h: unknown, { row }: { row: Ticket }) =>
      h(ElTag, { size: "small", effect: "light" }, () => statusOptions.find((o) => o.value === row.status)?.label || row.status),
  },
  { colKey: "reply_count", title: "回复", width: 70 },
  { colKey: "updated_at", title: "更新时间", width: 170, cell: (_h: unknown, { row }: { row: Ticket }) => formatTime(row.updated_at) },
]

const loadTickets = async () => {
  try {
    loading.value = true
    error.value = ""
    const res = await api.listAdminTickets({
      status: statusFilter.value || undefined,
      user_id: userFilter.value.trim() || undefined,
      page: page.value,
      page_size: pageSize.value,
    })
    tickets.value = res.tickets || []
    total.value = res.total || 0
  } catch (e: unknown) {
    error.value = e instanceof Error ? e.message : "加载失败"
  } finally {
    loading.value = false
  }
}

const onPageChange = (v: number) => {
  page.value = v
  loadTickets()
}

const onPageSizeChange = (v: number) => {
  pageSize.value = v
  page.value = 1
  loadTickets()
}

const openDetail = async (id: string) => {
  const res = await api.getAdminTicket(id)
  detail.value = res.ticket
  replies.value = res.replies || []
  editStatus.value = res.ticket.status
  editPriority.value = res.ticket.priority || "normal"
  replyText.value = ""
  detailOpen.value = true
}

const onRowClick = ({ row }: { row: Ticket }) => {
  openDetail(row.id).catch((e: unknown) => MessagePlugin.error(e instanceof Error ? e.message : "加载失败"))
}

const submitReply = async () => {
  if (!detail.value || !replyText.value.trim()) return
  replying.value = true
  try {
    await api.replyAdminTicket(detail.value.id, replyText.value.trim())
    await openDetail(detail.value.id)
    replyText.value = ""
    await loadTickets()
    MessagePlugin.success("回复已发送")
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "回复失败")
  } finally {
    replying.value = false
  }
}

const saveMeta = async () => {
  if (!detail.value) return
  try {
    const res = await api.updateAdminTicket(detail.value.id, {
      status: editStatus.value,
      priority: editPriority.value,
    })
    detail.value = res.ticket
    await loadTickets()
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "更新失败")
  }
}

watch(detailOpen, (v) => {
  if (!v) detail.value = null
})

onMounted(loadTickets)
</script>

