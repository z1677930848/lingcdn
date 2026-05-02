<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统公告</h1>
        <p class="subtitle">查看最新公告与系统通知</p>
      </div>
    </div>

    <div v-if="error" class="error-box">
      <span>{{ error }}</span>
      <t-button size="small" theme="primary" @click="() => loadAnnouncements(true)">重试</t-button>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body" style="display:flex;gap:12px;flex-wrap:wrap">
        <t-input v-model="query" clearable placeholder="搜索公告" style="min-width:260px" />
        <t-button theme="primary" :loading="loading" @click="onSearch">搜索</t-button>
        <t-button variant="outline" @click="() => loadAnnouncements(true)">刷新</t-button>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body" style="display:flex;flex-direction:column;gap:12px">
        <div v-if="items.length === 0" class="admin-empty">
          <p class="admin-table-muted" style="margin:0">暂无公告</p>
        </div>
        <div
          v-for="item in items"
          :key="item.id"
          class="notice-item"
          @click="openDetail(item)"
        >
          <div class="notice-title">
            <span>{{ item.title }}</span>
            <t-tag v-if="item.pinned" size="small" theme="primary" variant="light">置顶</t-tag>
          </div>
          <div class="notice-content">{{ excerpt(item.content) }}</div>
          <div class="notice-footer">
            <span>{{ formatDateTime(item.updated_at) }}</span>
            <t-button size="small" variant="text" @click.stop="openDetail(item)">查看详情</t-button>
          </div>
        </div>

        <t-button v-if="hasMore" variant="outline" :loading="loading" @click="onLoadMore">加载更多</t-button>
      </div>
    </t-card>

    <t-dialog v-model:visible="detailOpen" :header="detailItem?.title || '公告详情'" width="620" cancel-btn="关闭" :confirm-btn="null">
      <div class="detail-body">
        <div class="detail-meta">
          <t-tag v-if="detailItem?.pinned" size="small" theme="primary" variant="light">置顶</t-tag>
          <span>{{ formatDateTime(detailItem?.updated_at) }}</span>
        </div>
        <div class="detail-content">{{ detailItem?.content || '-' }}</div>
      </div>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, ref } from "vue"
import { api, type Announcement } from "@/lib/api"

const items = ref<Announcement[]>([])
const total = ref(0)
const page = ref(1)
const pageSize = 10
const loading = ref(false)
const error = ref("")
const query = ref("")
const detailOpen = ref(false)
const detailItem = ref<Announcement | null>(null)

const hasMore = computed(() => items.value.length < total.value)

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const excerpt = (value?: string, limit = 120) => {
  const text = String(value || "").trim()
  if (text.length <= limit) return text
  return `${text.slice(0, limit)}...`
}

const loadAnnouncements = async (reset = false) => {
  if (loading.value) return
  try {
    loading.value = true
    const nextPage = reset ? 1 : page.value
    const res = await api.getPublicAnnouncements({
      page: nextPage,
      pageSize,
      q: query.value.trim() || undefined,
    })
    const list = res.announcements || []
    total.value = typeof res.total === "number" ? res.total : 0
    items.value = reset ? list : [...items.value, ...list]
    error.value = ""
    if (reset) page.value = 1
  } catch (err: any) {
    error.value = err?.message || "加载公告失败"
  } finally {
    loading.value = false
  }
}

const onSearch = () => {
  items.value = []
  loadAnnouncements(true)
}

const onLoadMore = async () => {
  if (loading.value || !hasMore.value) return
  const next = page.value + 1
  page.value = next
  try {
    loading.value = true
    const res = await api.getPublicAnnouncements({
      page: next,
      pageSize,
      q: query.value.trim() || undefined,
    })
    const list = res.announcements || []
    total.value = typeof res.total === "number" ? res.total : 0
    items.value = [...items.value, ...list]
    error.value = ""
  } catch (err: any) {
    error.value = err?.message || "加载公告失败"
  } finally {
    loading.value = false
  }
}

const openDetail = (item: Announcement) => {
  detailItem.value = item
  detailOpen.value = true
}

onMounted(() => {
  loadAnnouncements(true)
})
</script>

<style scoped>
.notice-item {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 8px;
  cursor: pointer;
  transition: all 0.2s ease;
}

.notice-item:hover {
  border-color: #cbd5e1;
  box-shadow: 0 2px 8px rgba(0, 0, 0, 0.04);
  transform: translateY(-1px);
}

.notice-title {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  font-weight: 600;
  color: #0f172a;
}

.notice-content {
  color: #64748b;
  font-size: 13px;
  line-height: 1.5;
}

.notice-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  color: #94a3b8;
  font-size: 12px;
}

.detail-body {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.detail-meta {
  display: flex;
  align-items: center;
  gap: 8px;
  color: #94a3b8;
  font-size: 12px;
}

.detail-content {
  color: #334155;
  font-size: 14px;
  line-height: 1.7;
  white-space: pre-wrap;
}

@media (max-width: 768px) {
  .section-body {
    flex-direction: column;
  }
  .section-body :deep(.t-input) {
    min-width: 0 !important;
    width: 100% !important;
  }
}
</style>
