<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">系统公告</h1>
        <p class="subtitle">查看最新公告与系统通知</p>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="() => loadAnnouncements(true)" />

    <el-card class="section-card">
      <div class="section-body announcements-filter">
        <el-input v-model="query" clearable placeholder="搜索公告" style="min-width:260px" />
        <el-button type="primary" :loading="loading" @click="onSearch">搜索</el-button>
        <el-button plain @click="() => loadAnnouncements(true)">刷新</el-button>
      </div>
    </el-card>

    <el-card class="section-card">
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
            <el-tag v-if="item.pinned" size="small" type="primary" effect="light">置顶</el-tag>
          </div>
          <div class="notice-content">{{ excerpt(item.content) }}</div>
          <div class="notice-footer">
            <span>{{ formatDateTime(item.updated_at) }}</span>
            <el-button size="small" link @click.stop="openDetail(item)">查看详情</el-button>
          </div>
        </div>

        <el-button v-if="hasMore" plain :loading="loading" @click="onLoadMore">加载更多</el-button>
      </div>
    </el-card>

    <EpDialog append-to-body v-model="detailOpen" :title="detailItem?.title || '公告详情'" width="620" cancel-btn="关闭" :confirm-btn="null">
      <div class="detail-body">
        <div class="detail-meta">
          <el-tag v-if="detailItem?.pinned" size="small" type="primary" effect="light">置顶</el-tag>
          <span>{{ formatDateTime(detailItem?.updated_at) }}</span>
        </div>
        <div class="detail-content">{{ detailItem?.content || '-' }}</div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpDialog from "@/components/ep/EpDialog.vue"
import { computed, onMounted, ref } from "vue"
import { api, type Announcement } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

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

