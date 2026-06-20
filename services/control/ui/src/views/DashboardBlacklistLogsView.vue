<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">拦截日志</h1>
        <p class="subtitle">查看被 WAF/CC 防护拦截的访问记录</p>
      </div>
      <div class="header-actions">
        <el-button :loading="loading" @click="loadBans">刷新</el-button>
      </div>
    </div>

    <el-card class="section-card">
      <div class="section-body">
        <LoadingState v-if="loading && bans.length === 0" />
        <ErrorState v-else-if="error" :message="error" @retry="loadBans" />
        <template v-else>
          <div v-if="bans.length === 0" class="empty-state">暂无拦截记录</div>

          <div v-else>
            <div class="admin-desktop-only">
              <EpDataTable :data="bans" :columns="columns" row-key="ip" size="small" empty-text="暂无数据" />
            </div>
            <div class="admin-mobile-only">
              <div class="admin-mobile-cards">
                <div v-for="ban in bans" :key="ban.ip" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ ban.ip }}</span>
                    <el-tag type="danger" effect="light" size="small">已拦截</el-tag>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">原因</span>
                      <span class="admin-mobile-card-value">{{ ban.reason || "-" }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">触发次数</span>
                      <span class="admin-mobile-card-value">{{ ban.strikes || 0 }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">来源节点</span>
                      <span class="admin-mobile-card-value">{{ ban.node_id || "-" }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">过期时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(ban.expires_at) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">创建时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(ban.created_at) }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </div>
        </template>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api } from "@/lib/api"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"

interface WAFBan {
  ip: string
  reason?: string
  strikes?: number
  node_id?: string
  expires_at?: string
  created_at?: string
}

const loading = ref(false)
const error = ref("")
const bans = ref<WAFBan[]>([])

const formatTime = (value?: string) => {
  if (!value) return "-"
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return "-"
  return d.toLocaleString("zh-CN")
}

const loadBans = async () => {
  loading.value = true
  error.value = ""
  try {
    const res = await api.listWAFBans()
    bans.value = res.bans || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载拦截日志失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const columns = [
  { colKey: "ip", title: "IP 地址", minWidth: 150 },
  { colKey: "reason", title: "拦截原因", minWidth: 200, cell: (_h: any, { row }: { row: WAFBan }) => row.reason || "-" },
  { colKey: "strikes", title: "触发次数", width: 100 },
  { colKey: "node_id", title: "来源节点", width: 140, cell: (_h: any, { row }: { row: WAFBan }) => row.node_id || "-" },
  { colKey: "expires_at", title: "过期时间", width: 170, cell: (_h: any, { row }: { row: WAFBan }) => formatTime(row.expires_at) },
  { colKey: "created_at", title: "创建时间", width: 170, cell: (_h: any, { row }: { row: WAFBan }) => formatTime(row.created_at) },
]

onMounted(() => { loadBans() })
</script>


