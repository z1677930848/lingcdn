<template>
  <div class="admin-page finance-stats-page">
    <el-card class="admin-list-card">
      <div class="admin-toolbar">
        <div class="admin-toolbar-left">
          <el-date-picker v-model="dateRange" @change="loadStats" />
        </div>
        <div class="admin-toolbar-right">
          <el-button plain @click="resetDateRange">重置</el-button>
          <el-button type="primary" :loading="loading" @click="loadStats">刷新</el-button>
        </div>
      </div>

      <div class="stats-cards">
        <el-card class="stat-card">
          <div class="stat-title">充值总额</div>
          <div class="stat-value">{{ formatMoney(totalRechargeCents) }}</div>
          <div class="stat-sub">充值笔数 {{ totalRechargeCount }}</div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-title">管理员调整净额</div>
          <div class="stat-value">{{ formatMoney(totalAdjustCents) }}</div>
          <div class="stat-sub">调整笔数 {{ totalAdjustCount }}</div>
        </el-card>
        <el-card class="stat-card">
          <div class="stat-title">综合净额</div>
          <div class="stat-value">{{ formatMoney(totalCents) }}</div>
          <div class="stat-sub">总笔数 {{ totalCount }}</div>
        </el-card>
      </div>

      <ErrorState v-if="error" :message="error" @retry="loadStats" />

      <div class="admin-desktop-only" style="margin-top: 12px">
        <EpDataTable
          :data="stats"
          :columns="columns"
          row-key="day"
          hover
          stripe
          :loading="loading"
        />
      </div>
      <div class="admin-mobile-only" style="margin-top: 12px">
        <div v-if="loading" style="text-align:center;padding:32px 0"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="stats.length === 0" class="admin-mobile-card-empty">暂无数据</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="item in stats" :key="item.day" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <div class="admin-mobile-card-title">{{ item.day }}</div>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">充值金额</span>
                <span class="admin-mobile-card-value">{{ formatMoney(item.recharge_cents) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">充值笔数</span>
                <span class="admin-mobile-card-value">{{ item.recharge_count }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">调整净额</span>
                <span class="admin-mobile-card-value" :style="{ color: (Number(item.adjust_cents) || 0) < 0 ? '#cf1322' : '#389e0d' }">{{ formatMoney(item.adjust_cents) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">调整笔数</span>
                <span class="admin-mobile-card-value">{{ item.adjust_count }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">综合净额</span>
                <span class="admin-mobile-card-value" :style="{ color: (Number(item.total_cents) || 0) < 0 ? '#cf1322' : '#389e0d', fontWeight: '600' }">{{ formatMoney(item.total_cents) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">总笔数</span>
                <span class="admin-mobile-card-value">{{ item.total_count }}</span>
              </div>
            </div>
          </div>
        </div>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { api, type BalanceStatsDay } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(false)
const error = ref("")
const stats = ref<BalanceStatsDay[]>([])
const dateRange = ref<string[]>([])

const formatMoney = (cents?: number, currency = "CNY") => {
  const value = typeof cents === "number" ? cents / 100 : 0
  return `${value.toFixed(2)} ${currency}`
}

const totalRechargeCents = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.recharge_cents) || 0), 0))
const totalRechargeCount = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.recharge_count) || 0), 0))
const totalAdjustCents = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.adjust_cents) || 0), 0))
const totalAdjustCount = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.adjust_count) || 0), 0))
const totalCents = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.total_cents) || 0), 0))
const totalCount = computed(() => stats.value.reduce((sum, item) => sum + (Number(item.total_count) || 0), 0))

const columns = computed(() => [
  { colKey: "day", title: "日期", width: 140 },
  {
    colKey: "recharge_cents",
    title: "充值金额",
    width: 140,
    cell: (_h: any, { row }: { row: BalanceStatsDay }) => formatMoney(row.recharge_cents),
  },
  { colKey: "recharge_count", title: "充值笔数", width: 120 },
  {
    colKey: "adjust_cents",
    title: "调整净额",
    width: 140,
    cell: (_h: any, { row }: { row: BalanceStatsDay }) => {
      const val = Number(row.adjust_cents) || 0
      const color = val < 0 ? "#cf1322" : "#389e0d"
      return h("span", { style: { color } }, formatMoney(val))
    },
  },
  { colKey: "adjust_count", title: "调整笔数", width: 120 },
  {
    colKey: "total_cents",
    title: "综合净额",
    width: 140,
    cell: (_h: any, { row }: { row: BalanceStatsDay }) => {
      const val = Number(row.total_cents) || 0
      const color = val < 0 ? "#cf1322" : "#389e0d"
      return h("span", { style: { color, fontWeight: 600 } }, formatMoney(val))
    },
  },
  { colKey: "total_count", title: "总笔数", width: 120 },
])

const fmt = (d: Date) => {
  const y = d.getFullYear()
  const m = String(d.getMonth() + 1).padStart(2, "0")
  const day = String(d.getDate()).padStart(2, "0")
  return `${y}-${m}-${day}`
}

const setDefaultRange = () => {
  const now = new Date()
  const start = new Date(now.getFullYear(), now.getMonth(), 1)
  dateRange.value = [fmt(start), fmt(now)]
}

const loadStats = async () => {
  if (!dateRange.value?.[0] || !dateRange.value?.[1]) {
    setDefaultRange()
  }
  loading.value = true
  error.value = ""
  try {
    const from = dateRange.value[0]
    const to = dateRange.value[1]
    const res = await api.adminBalanceStats({
      from,
      to,
      start_date: from,
      end_date: to,
    })
    stats.value = Array.isArray(res.stats) ? res.stats : []
  } catch (err: any) {
    const message = err?.message || "加载财务统计失败"
    error.value = message
    MessagePlugin.error(message)
  } finally {
    loading.value = false
  }
}

const resetDateRange = async () => {
  setDefaultRange()
  await loadStats()
}

onMounted(async () => {
  setDefaultRange()
  await loadStats()
})
</script>

