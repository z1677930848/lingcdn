<template>
  <div class="admin-page">
    <div class="header">
      <div>
        <h1 class="title">余额管理</h1>
        <p class="subtitle">查看余额账户、处理充值与提现申请</p>
      </div>
    </div>

    <t-card bordered class="admin-list-card">
      <t-tabs v-model="activeTab" class="admin-tabs">
        <t-tab-panel value="accounts" label="余额账户">
          <div v-if="accountsError" class="admin-error-box">
            {{ accountsError }}
            <t-button size="small" theme="primary" @click="loadAccounts">重试</t-button>
          </div>
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button variant="outline" @click="loadAccounts">刷新</t-button>
            </div>
            <div class="admin-toolbar-right">
              <t-input v-model="accountsUserId" class="admin-search-input" clearable placeholder="用户ID" />
            </div>
          </div>
          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <t-table
              :data="accounts"
              :columns="accountColumns"
              row-key="user_id"
              bordered
              hover
              stripe
              :loading="accountsLoading"
              :pagination="accountsPagination"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="accounts.length === 0 && !accountsLoading" class="admin-mobile-card-empty">
              暂无账户数据
            </div>
            <div v-else class="admin-mobile-cards">
              <div v-for="row in accounts" :key="row.user_id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">{{ row.user_id }}</div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">余额</span>
                    <span class="admin-mobile-card-value">{{ formatMoney(row.balance_cents) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">更新时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.updated_at) }}</span>
                  </div>
                </div>
              </div>
            </div>
            <div v-if="accountsTotal > 0" class="admin-mobile-pagination">
              <t-pagination
                :current="accountsPage"
                :page-size="accountsPageSize"
                :total="accountsTotal"
                :page-size-options="[10, 20, 50]"
                show-page-size
                size="small"
                @current-change="(v: number) => { accountsPage = v }"
                @page-size-change="(v: number) => { accountsPageSize = v; accountsPage = 1 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="recharges" label="充值记录">
          <div v-if="rechargesError" class="admin-error-box">
            {{ rechargesError }}
            <t-button size="small" theme="primary" @click="loadRecharges">重试</t-button>
          </div>
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button variant="outline" @click="loadRecharges">刷新</t-button>
            </div>
            <div class="admin-toolbar-right">
              <t-select v-model="rechargeStatus" :options="rechargeStatusOptions" class="admin-filter-select-large" />
              <t-input v-model="rechargeUserId" class="admin-search-input" clearable placeholder="用户ID" />
            </div>
          </div>
          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <t-table
              :data="recharges"
              :columns="rechargeColumns"
              row-key="id"
              bordered
              hover
              stripe
              :loading="rechargesLoading"
              :pagination="rechargePagination"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="recharges.length === 0 && !rechargesLoading" class="admin-mobile-card-empty">
              暂无充值记录
            </div>
            <div v-else class="admin-mobile-cards">
              <div v-for="row in recharges" :key="row.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">{{ row.user_id }}</div>
                  <div class="admin-mobile-card-tags">
                    <t-tag
                      :theme="statusTheme(row.status)"
                      variant="light"
                      size="small"
                    >
                      {{ statusText(row.status) }}
                    </t-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-subtitle">{{ row.out_trade_no }}</div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">金额</span>
                    <span class="admin-mobile-card-value">{{ formatMoney(row.amount_cents, row.currency || 'CNY') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">方式</span>
                    <span class="admin-mobile-card-value">{{ row.payment_method || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">渠道</span>
                    <span class="admin-mobile-card-value">{{ row.payment_provider || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">交易号</span>
                    <span class="admin-mobile-card-value">{{ row.trade_no || '-' }}</span>
                  </div>
                  <div v-if="row.payment_url" class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">支付链接</span>
                    <a class="admin-mobile-card-value pay-link" :href="row.payment_url" target="_blank">打开</a>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">创建时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.created_at) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">到账时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.paid_at) }}</span>
                  </div>
                </div>
                <div class="admin-mobile-card-actions">
                  <t-button size="small" variant="outline" @click="openRechargeDialog(row)">更新</t-button>
                </div>
              </div>
            </div>
            <div v-if="rechargeTotal > 0" class="admin-mobile-pagination">
              <t-pagination
                :current="rechargePage"
                :page-size="rechargePageSize"
                :total="rechargeTotal"
                :page-size-options="[10, 20, 50]"
                show-page-size
                size="small"
                @current-change="(v: number) => { rechargePage = v }"
                @page-size-change="(v: number) => { rechargePageSize = v; rechargePage = 1 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="withdrawals" label="提现申请">
          <div v-if="withdrawalsError" class="admin-error-box">
            {{ withdrawalsError }}
            <t-button size="small" theme="primary" @click="loadWithdrawals">重试</t-button>
          </div>
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button variant="outline" @click="loadWithdrawals">刷新</t-button>
            </div>
            <div class="admin-toolbar-right">
              <t-select v-model="withdrawalStatus" :options="withdrawalStatusOptions" class="admin-filter-select-large" />
              <t-input v-model="withdrawalUserId" class="admin-search-input" clearable placeholder="用户ID" />
            </div>
          </div>
          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <t-table
              :data="withdrawals"
              :columns="withdrawalColumns"
              row-key="id"
              bordered
              hover
              stripe
              :loading="withdrawalsLoading"
              :pagination="withdrawalPagination"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="withdrawals.length === 0 && !withdrawalsLoading" class="admin-mobile-card-empty">
              暂无提现申请
            </div>
            <div v-else class="admin-mobile-cards">
              <div v-for="row in withdrawals" :key="row.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <div class="admin-mobile-card-title">{{ row.user_id }}</div>
                  <div class="admin-mobile-card-tags">
                    <t-tag
                      :theme="row.status === 'approved' ? 'success' : row.status === 'rejected' ? 'danger' : 'warning'"
                      variant="light"
                      size="small"
                    >
                      {{ row.status === 'approved' ? '已通过' : row.status === 'rejected' ? '已拒绝' : '待审核' }}
                    </t-tag>
                  </div>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">金额</span>
                    <span class="admin-mobile-card-value">{{ formatMoney(row.amount_cents, row.currency || 'CNY') }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">方式</span>
                    <span class="admin-mobile-card-value">{{ row.method || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">账户名</span>
                    <span class="admin-mobile-card-value">{{ row.account_name || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">账户号</span>
                    <span class="admin-mobile-card-value">{{ row.account_no || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">申请时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.created_at) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">审核时间</span>
                    <span class="admin-mobile-card-value">{{ formatDateTime(row.reviewed_at) }}</span>
                  </div>
                </div>
                <div class="admin-mobile-card-actions">
                  <t-button size="small" variant="outline" @click="openWithdrawalDialog(row)">审核</t-button>
                </div>
              </div>
            </div>
            <div v-if="withdrawalTotal > 0" class="admin-mobile-pagination">
              <t-pagination
                :current="withdrawalPage"
                :page-size="withdrawalPageSize"
                :total="withdrawalTotal"
                :page-size-options="[10, 20, 50]"
                show-page-size
                size="small"
                @current-change="(v: number) => { withdrawalPage = v }"
                @page-size-change="(v: number) => { withdrawalPageSize = v; withdrawalPage = 1 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="adjust" label="余额调整">
          <div class="section-body">
            <div class="admin-table-muted" style="margin-bottom:16px">
              仅管理员可操作，支持增加或扣减余额。
            </div>
            <t-form layout="vertical" label-align="top" class="admin-balance-adjust-form">
              <t-form-item label="用户 ID">
                <t-input v-model="adjustUserId" />
              </t-form-item>
              <t-form-item label="调整类型">
                <t-select v-model="adjustType" :options="adjustTypeOptions" />
              </t-form-item>
              <t-form-item label="金额（元）">
                <t-input v-model="adjustAmount" />
              </t-form-item>
              <t-form-item label="备注（选填）">
                <t-input v-model="adjustNote" placeholder="可选填写调整备注" />
              </t-form-item>
              <t-button theme="primary" :loading="adjusting" @click="handleAdjust">确认调整</t-button>
            </t-form>
          </div>
        </t-tab-panel>
      </t-tabs>
    </t-card>

    <t-dialog
      v-model:visible="rechargeDialogOpen"
      header="更新充值状态"
      :confirm-btn="{ content: rechargeUpdating ? '更新中...' : '确认更新', loading: rechargeUpdating, theme: 'primary' }"
      cancel-btn="取消"
      width="480"
      @confirm="handleRechargeUpdate"
      @close="closeRechargeDialog"
    >
      <div class="admin-table-muted" style="margin-bottom:12px">
        <div>充值单号：{{ editingRecharge?.out_trade_no || '-' }}</div>
        <div>支付渠道：{{ editingRecharge?.payment_provider || '-' }}</div>
        <div>支付方式：{{ editingRecharge?.payment_method || '-' }}</div>
        <div v-if="editingRecharge?.payment_url">支付链接：<a :href="editingRecharge.payment_url" target="_blank">打开支付链接</a></div>
      </div>
      <t-form layout="vertical" label-align="top">
        <t-form-item label="状态">
          <t-select v-model="rechargeDialogStatus" :options="rechargeDialogOptions" />
        </t-form-item>
        <t-form-item label="交易号">
          <t-input v-model="rechargeDialogTradeNo" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="withdrawalDialogOpen"
      header="审核提现申请"
      :confirm-btn="{ content: withdrawalUpdating ? '更新中...' : '确认审核', loading: withdrawalUpdating, theme: 'primary' }"
      cancel-btn="取消"
      width="480"
      @confirm="handleWithdrawalUpdate"
      @close="closeWithdrawalDialog"
    >
      <div class="admin-table-muted" style="margin-bottom:12px">
        提现金额：{{ editingWithdrawal ? formatMoney(editingWithdrawal.amount_cents, editingWithdrawal.currency) : '-' }}
      </div>
      <t-form layout="vertical" label-align="top">
        <t-form-item label="状态">
          <t-select v-model="withdrawalDialogStatus" :options="withdrawalDialogOptions" />
        </t-form-item>
        <t-form-item label="备注">
          <t-input v-model="withdrawalDialogNote" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, ref, watch } from "vue"
import { MessagePlugin, Tag, Button } from "tdesign-vue-next"
import { api, type BalanceAccount, type BalanceRecharge, type BalanceWithdrawal } from "@/lib/api"

const activeTab = ref("accounts")

const accounts = ref<BalanceAccount[]>([])
const accountsLoading = ref(false)
const accountsError = ref("")
const accountsUserId = ref("")
const accountsPage = ref(1)
const accountsPageSize = ref(10)
const accountsTotal = ref(0)

const recharges = ref<BalanceRecharge[]>([])
const rechargesLoading = ref(false)
const rechargesError = ref("")
const rechargeStatus = ref("all")
const rechargeUserId = ref("")
const rechargePage = ref(1)
const rechargePageSize = ref(10)
const rechargeTotal = ref(0)

const withdrawals = ref<BalanceWithdrawal[]>([])
const withdrawalsLoading = ref(false)
const withdrawalsError = ref("")
const withdrawalStatus = ref("all")
const withdrawalUserId = ref("")
const withdrawalPage = ref(1)
const withdrawalPageSize = ref(10)
const withdrawalTotal = ref(0)

const adjustUserId = ref("")
const adjustType = ref("increase")
const adjustAmount = ref("")
const adjustNote = ref("")
const adjusting = ref(false)

const editingRecharge = ref<BalanceRecharge | null>(null)
const rechargeDialogOpen = ref(false)
const rechargeDialogStatus = ref("pending")
const rechargeDialogTradeNo = ref("")
const rechargeUpdating = ref(false)

const editingWithdrawal = ref<BalanceWithdrawal | null>(null)
const withdrawalDialogOpen = ref(false)
const withdrawalDialogStatus = ref("pending")
const withdrawalDialogNote = ref("")
const withdrawalUpdating = ref(false)

const rechargeStatusOptions = [
  { label: "全部状态", value: "all" },
  { label: "待处理", value: "pending" },
  { label: "已到账", value: "paid" },
  { label: "失败", value: "failed" },
  { label: "已关闭", value: "closed" },
  { label: "已取消", value: "cancelled" },
]

const withdrawalStatusOptions = [
  { label: "全部状态", value: "all" },
  { label: "待审核", value: "pending" },
  { label: "已通过", value: "approved" },
  { label: "已拒绝", value: "rejected" },
]

const adjustTypeOptions = [
  { label: "增加余额", value: "increase" },
  { label: "扣减余额", value: "decrease" },
]

const rechargeDialogOptions = [
  { label: "待处理", value: "pending" },
  { label: "已到账", value: "paid" },
  { label: "失败", value: "failed" },
  { label: "已关闭", value: "closed" },
  { label: "已取消", value: "cancelled" },
]

const withdrawalDialogOptions = [
  { label: "待审核", value: "pending" },
  { label: "已通过", value: "approved" },
  { label: "已拒绝", value: "rejected" },
]

const formatMoney = (cents?: number, currency = "CNY") => {
  const value = typeof cents === "number" ? cents / 100 : 0
  return `${value.toFixed(2)} ${currency}`
}

const formatDateTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const statusText = (status?: string) => {
  if (status === "paid") return "已到账"
  if (status === "pending") return "待处理"
  if (status === "failed") return "失败"
  if (status === "closed") return "已关闭"
  if (status === "cancelled") return "已取消"
  return status || "-"
}

const statusTheme = (status?: string) => {
  if (status === "paid") return "success"
  if (status === "pending") return "warning"
  if (status === "failed") return "danger"
  return "default"
}

const loadAccounts = async () => {
  try {
    accountsLoading.value = true
    const { accounts: list, total } = await api.adminListBalanceAccounts({
      page: accountsPage.value,
      pageSize: accountsPageSize.value,
      userId: accountsUserId.value.trim() || undefined,
    })
    accounts.value = list || []
    accountsTotal.value = typeof total === "number" ? total : list?.length || 0
    accountsError.value = ""
  } catch (err: any) {
    const msg = err?.message || "加载账户余额失败"
    accountsError.value = msg
    MessagePlugin.error(msg)
  } finally {
    accountsLoading.value = false
  }
}

const loadRecharges = async () => {
  try {
    rechargesLoading.value = true
    const { recharges: list, total } = await api.adminListBalanceRecharges({
      page: rechargePage.value,
      pageSize: rechargePageSize.value,
      status: rechargeStatus.value === "all" ? undefined : rechargeStatus.value,
      userId: rechargeUserId.value.trim() || undefined,
    })
    recharges.value = list || []
    rechargeTotal.value = typeof total === "number" ? total : list?.length || 0
    rechargesError.value = ""
  } catch (err: any) {
    const msg = err?.message || "加载充值记录失败"
    rechargesError.value = msg
    MessagePlugin.error(msg)
  } finally {
    rechargesLoading.value = false
  }
}

const loadWithdrawals = async () => {
  try {
    withdrawalsLoading.value = true
    const { withdrawals: list, total } = await api.adminListBalanceWithdrawals({
      page: withdrawalPage.value,
      pageSize: withdrawalPageSize.value,
      status: withdrawalStatus.value === "all" ? undefined : withdrawalStatus.value,
      userId: withdrawalUserId.value.trim() || undefined,
    })
    withdrawals.value = list || []
    withdrawalTotal.value = typeof total === "number" ? total : list?.length || 0
    withdrawalsError.value = ""
  } catch (err: any) {
    const msg = err?.message || "加载提现申请失败"
    withdrawalsError.value = msg
    MessagePlugin.error(msg)
  } finally {
    withdrawalsLoading.value = false
  }
}

const openRechargeDialog = (recharge: BalanceRecharge) => {
  editingRecharge.value = recharge
  rechargeDialogStatus.value = recharge.status || "pending"
  rechargeDialogTradeNo.value = recharge.trade_no || ""
  rechargeDialogOpen.value = true
}

const closeRechargeDialog = () => {
  rechargeDialogOpen.value = false
  editingRecharge.value = null
}

const handleRechargeUpdate = async () => {
  if (!editingRecharge.value || rechargeUpdating.value) return
  rechargeUpdating.value = true
  try {
    await api.adminUpdateBalanceRecharge(editingRecharge.value.id, {
      status: rechargeDialogStatus.value,
      trade_no: rechargeDialogTradeNo.value.trim() || undefined,
    })
    MessagePlugin.success("充值状态已更新")
    closeRechargeDialog()
    await loadRecharges()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新充值失败")
  } finally {
    rechargeUpdating.value = false
  }
}

const openWithdrawalDialog = (withdrawal: BalanceWithdrawal) => {
  editingWithdrawal.value = withdrawal
  withdrawalDialogStatus.value = withdrawal.status || "pending"
  withdrawalDialogNote.value = withdrawal.note || ""
  withdrawalDialogOpen.value = true
}

const closeWithdrawalDialog = () => {
  withdrawalDialogOpen.value = false
  editingWithdrawal.value = null
}

const handleWithdrawalUpdate = async () => {
  if (!editingWithdrawal.value || withdrawalUpdating.value) return
  withdrawalUpdating.value = true
  try {
    await api.adminUpdateBalanceWithdrawal(editingWithdrawal.value.id, {
      status: withdrawalDialogStatus.value,
      note: withdrawalDialogNote.value.trim() || undefined,
    })
    MessagePlugin.success("提现状态已更新")
    closeWithdrawalDialog()
    await loadWithdrawals()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新提现失败")
  } finally {
    withdrawalUpdating.value = false
  }
}

const handleAdjust = async () => {
  if (adjusting.value) return
  const amount = Number.parseFloat(adjustAmount.value)
  if (!adjustUserId.value.trim()) {
    MessagePlugin.error("请输入用户ID")
    return
  }
  if (!Number.isFinite(amount) || amount <= 0) {
    MessagePlugin.error("请输入有效的金额")
    return
  }
  const amountCents = Math.round(amount * 100) * (adjustType.value === "decrease" ? -1 : 1)
  adjusting.value = true
  try {
    await api.adminAdjustBalance({
      user_id: adjustUserId.value.trim(),
      amount_cents: amountCents,
      note: adjustNote.value.trim() || undefined,
    })
    MessagePlugin.success("余额已调整")
    adjustAmount.value = ""
    adjustNote.value = ""
    await loadAccounts()
  } catch (err: any) {
    MessagePlugin.error(err.message || "调整余额失败")
  } finally {
    adjusting.value = false
  }
}

const accountColumns = computed(() => [
  { colKey: "user_id", title: "用户 ID", minWidth: 160 },
  { colKey: "balance_cents", title: "余额", width: 140, cell: (_h: any, { row }: { row: BalanceAccount }) => formatMoney(row.balance_cents) },
  { colKey: "updated_at", title: "更新时间", width: 180, cell: (_h: any, { row }: { row: BalanceAccount }) => formatDateTime(row.updated_at) },
])

const rechargeColumns = computed(() => [
  { colKey: "out_trade_no", title: "充值单号", minWidth: 180 },
  { colKey: "user_id", title: "用户", minWidth: 140 },
  { colKey: "amount_cents", title: "金额", width: 120, cell: (_h: any, { row }: { row: BalanceRecharge }) => formatMoney(row.amount_cents, row.currency || "CNY") },
  { colKey: "payment_method", title: "方式", width: 120 },
  { colKey: "payment_provider", title: "渠道", width: 120 },
  { colKey: "trade_no", title: "交易号", minWidth: 160, cell: (_h: any, { row }: { row: BalanceRecharge }) => row.trade_no || "-" },
  {
    colKey: "payment_url",
    title: "支付链接",
    width: 100,
    cell: (_h: any, { row }: { row: BalanceRecharge }) =>
      row.payment_url ? h("a", { href: row.payment_url, target: "_blank", class: "pay-link" }, "打开") : "-",
  },
  {
    colKey: "status",
    title: "状态",
    width: 120,
    cell: (_h: any, { row }: { row: BalanceRecharge }) => {
      return h(Tag, { theme: statusTheme(row.status) }, () => statusText(row.status))
    },
  },
  { colKey: "created_at", title: "创建时间", width: 170, cell: (_h: any, { row }: { row: BalanceRecharge }) => formatDateTime(row.created_at) },
  { colKey: "paid_at", title: "到账时间", width: 170, cell: (_h: any, { row }: { row: BalanceRecharge }) => formatDateTime(row.paid_at) },
  {
    colKey: "actions",
    title: "操作",
    width: 120,
    fixed: "right",
    cell: (_h: any, { row }: { row: BalanceRecharge }) => h(Button, { size: "small", variant: "text", onClick: () => openRechargeDialog(row) }, () => "更新"),
  },
])

const withdrawalColumns = computed(() => [
  { colKey: "user_id", title: "用户", minWidth: 140 },
  { colKey: "amount_cents", title: "金额", width: 120, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatMoney(row.amount_cents, row.currency || "CNY") },
  { colKey: "method", title: "方式", width: 120 },
  { colKey: "account_name", title: "账户名", minWidth: 120 },
  { colKey: "account_no", title: "账户号", minWidth: 160 },
  {
    colKey: "status",
    title: "状态",
    width: 120,
    cell: (_h: any, { row }: { row: BalanceWithdrawal }) => {
      const status = row.status || "pending"
      const theme = status === "approved" ? "success" : status === "rejected" ? "danger" : "warning"
      const text = status === "approved" ? "已通过" : status === "rejected" ? "已拒绝" : "待审核"
      return h(Tag, { theme }, () => text)
    },
  },
  { colKey: "created_at", title: "申请时间", width: 170, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatDateTime(row.created_at) },
  { colKey: "reviewed_at", title: "审核时间", width: 170, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatDateTime(row.reviewed_at) },
  {
    colKey: "actions",
    title: "操作",
    width: 120,
    fixed: "right",
    cell: (_h: any, { row }: { row: BalanceWithdrawal }) => h(Button, { size: "small", variant: "text", onClick: () => openWithdrawalDialog(row) }, () => "审核"),
  },
])

const accountsPagination = computed(() => ({
  current: accountsPage.value,
  pageSize: accountsPageSize.value,
  total: accountsTotal.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50],
  onChange: (pi: { current: number; pageSize: number }) => {
    accountsPage.value = pi.current
    accountsPageSize.value = pi.pageSize
  },
}))

const rechargePagination = computed(() => ({
  current: rechargePage.value,
  pageSize: rechargePageSize.value,
  total: rechargeTotal.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50],
  onChange: (pi: { current: number; pageSize: number }) => {
    rechargePage.value = pi.current
    rechargePageSize.value = pi.pageSize
  },
}))

const withdrawalPagination = computed(() => ({
  current: withdrawalPage.value,
  pageSize: withdrawalPageSize.value,
  total: withdrawalTotal.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50],
  onChange: (pi: { current: number; pageSize: number }) => {
    withdrawalPage.value = pi.current
    withdrawalPageSize.value = pi.pageSize
  },
}))

watch([activeTab, accountsPage, accountsPageSize, accountsUserId], () => {
  if (activeTab.value === "accounts") loadAccounts()
})

watch([activeTab, rechargePage, rechargePageSize, rechargeStatus, rechargeUserId], () => {
  if (activeTab.value === "recharges") loadRecharges()
})

watch([activeTab, withdrawalPage, withdrawalPageSize, withdrawalStatus, withdrawalUserId], () => {
  if (activeTab.value === "withdrawals") loadWithdrawals()
})

watch(activeTab, () => {
  if (activeTab.value === "accounts") loadAccounts()
  if (activeTab.value === "recharges") loadRecharges()
  if (activeTab.value === "withdrawals") loadWithdrawals()
})
</script>

<style scoped>
.admin-balance-adjust-form {
  max-width: 420px;
}

.pay-link {
  color: #2563eb;
  font-weight: 600;
}

@media (max-width: 768px) {
  .admin-balance-adjust-form {
    max-width: 100%;
  }
}
</style>
