<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">工单支持</h1>
        <p class="subtitle">提交问题、查看回复与处理进度</p>
      </div>
      <el-button type="primary" @click="openCreate">新建工单</el-button>
    </div>

    <el-card class="section-card" bordered>
      <div class="filters">
        <EpSelect v-model="statusFilter" clearable placeholder="全部状态" style="width:160px" :options="statusOptions" @change="() => { page = 1; loadTickets() }" />
        <el-button :loading="loading" @click="loadTickets">刷新</el-button>
      </div>
      <LoadingState v-if="loading && tickets.length === 0" />
      <ErrorState v-else-if="error" :message="error" @retry="loadTickets" />
      <EpDataTable v-else :data="tickets" :columns="columns" row-key="id" size="small" empty-text="暂无工单" @row-click="onRowClick" />
      <div v-if="total > 0" class="pager">
        <el-pagination
          :current="page"
          :page-size="pageSize"
          :total="total"
          :page-size-options="[10, 20, 50]"
          show-page-size
          size="small"
          @current-change="onPageChange"
          @page-size-change="onPageSizeChange"
        />
      </div>
    </el-card>

    <EpDialog append-to-body v-model="createOpen" header="新建工单" :confirm-btn="{ content: '提交', loading: creating }" @confirm="submitCreate">
      <div class="form-grid-vertical">
        <el-form label-position="top">
          <el-form-item label="主题">
            <el-input v-model="createForm.subject" placeholder="简要描述问题" />
          </el-form-item>
          <el-form-item label="分类">
            <EpSelect v-model="createForm.category" :options="categoryOptions" />
          </el-form-item>
          <el-form-item label="详细描述">
            <el-input v-model="createForm.body" :autosize="{ minRows: 4 }" placeholder="请描述问题现象、域名、复现步骤等" />
          </el-form-item>
        </el-form>
      </div>
    </EpDialog>

    <EpDialog append-to-body v-model="detailOpen" :title="detail?.subject || '工单详情'" width="720px" :confirm-btn="null" cancel-btn="关闭">
      <div v-if="detail" class="detail-wrap">
        <div class="meta-row">
          <el-tag size="small" effect="light">{{ statusLabel(detail.status) }}</el-tag>
          <span class="muted">{{ categoryLabel(detail.category) }} · 更新于 {{ formatTime(detail.updated_at) }}</span>
        </div>
        <div class="ticket-thread">
          <div v-for="r in replies" :key="r.id" class="ticket-reply" :class="r.author_role === 'admin' ? 'ticket-reply--admin' : 'ticket-reply--user'">
            <div class="ticket-reply-head">
              <strong>{{ r.author_name || r.author_role }}</strong>
              <span class="muted">{{ formatTime(r.created_at) }}</span>
            </div>
            <div class="ticket-reply-body">{{ r.body }}</div>
          </div>
        </div>
        <div v-if="detail.status !== 'closed'" class="ticket-reply-box">
          <el-input v-model="replyText" :autosize="{ minRows: 3 }" placeholder="输入回复内容" />
          <div class="ticket-reply-actions">
            <el-button type="primary" :loading="replying" @click="submitReply">发送回复</el-button>
            <el-button plain :loading="closing" @click="closeCurrent">关闭工单</el-button>
          </div>
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import { h, onMounted, ref } from "vue"
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
const createOpen = ref(false)
const creating = ref(false)
const detailOpen = ref(false)
const detail = ref<Ticket | null>(null)
const replies = ref<TicketReply[]>([])
const replyText = ref("")
const replying = ref(false)
const closing = ref(false)

const statusOptions = [
  { label: "待处理", value: "open" },
  { label: "已回复", value: "replied" },
  { label: "已关闭", value: "closed" },
]
const categoryOptions = [
  { label: "一般咨询", value: "general" },
  { label: "账单/充值", value: "billing" },
  { label: "技术问题", value: "technical" },
]

const statusLabel = (s: string) => statusOptions.find((o) => o.value === s)?.label || s
const categoryLabel = (c: string) => categoryOptions.find((o) => o.value === c)?.label || c

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  return Number.isNaN(d.getTime()) ? v : d.toLocaleString("zh-CN")
}

const columns = [
  { colKey: "subject", title: "主题", ellipsis: true },
  {
    colKey: "status",
    title: "状态",
    width: 100,
    cell: (_h: unknown, { row }: { row: Ticket }) =>
      h(ElTag, { size: "small", effect: "light", type: row.status === "closed" ? "info" : row.status === "replied" ? "success" : "warning" }, () => statusLabel(row.status)),
  },
  { colKey: "category", title: "分类", width: 100, cell: (_h: unknown, { row }: { row: Ticket }) => categoryLabel(row.category) },
  { colKey: "updated_at", title: "更新时间", width: 180, cell: (_h: unknown, { row }: { row: Ticket }) => formatTime(row.updated_at) },
]

const loadTickets = async () => {
  try {
    loading.value = true
    error.value = ""
    const res = await api.listTickets({
      status: statusFilter.value || undefined,
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

const openCreate = () => {
  createForm.value = { subject: "", category: "general", body: "" }
  createOpen.value = true
}

const createForm = ref({ subject: "", category: "general", body: "" })

const submitCreate = async () => {
  if (!createForm.value.subject.trim() || !createForm.value.body.trim()) {
    MessagePlugin.warning("请填写主题和描述")
    return
  }
  creating.value = true
  try {
    await api.createTicket(createForm.value)
    MessagePlugin.success("工单已提交")
    createOpen.value = false
    await loadTickets()
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "提交失败")
  } finally {
    creating.value = false
  }
}

const openDetail = async (id: string) => {
  try {
    const res = await api.getTicket(id)
    detail.value = res.ticket
    replies.value = res.replies || []
    replyText.value = ""
    detailOpen.value = true
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "加载详情失败")
  }
}

const onRowClick = ({ row }: { row: Ticket }) => openDetail(row.id)

const submitReply = async () => {
  if (!detail.value || !replyText.value.trim()) return
  replying.value = true
  try {
    const res = await api.replyTicket(detail.value.id, replyText.value.trim())
    detail.value = res.ticket
    const full = await api.getTicket(detail.value.id)
    replies.value = full.replies || []
    replyText.value = ""
    await loadTickets()
    MessagePlugin.success("回复已发送")
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "回复失败")
  } finally {
    replying.value = false
  }
}

const closeCurrent = async () => {
  if (!detail.value) return
  closing.value = true
  try {
    const res = await api.closeTicket(detail.value.id)
    detail.value = res.ticket
    await loadTickets()
    MessagePlugin.success("工单已关闭")
  } catch (e: unknown) {
    MessagePlugin.error(e instanceof Error ? e.message : "关闭失败")
  } finally {
    closing.value = false
  }
}

onMounted(loadTickets)
</script>

