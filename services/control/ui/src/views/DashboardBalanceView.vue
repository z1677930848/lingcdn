<template>
  <LoadingState v-if="loading" />
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">账户余额</h1>
        <p class="subtitle">充值、提现与余额流水</p>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="loadData" />

    <PermissionNotice
      v-if="hasRestrictions && !canBalanceWithdraw"
      show
      message="当前分组无提现权限，仍可充值与查看流水。"
    />

    <el-card class="section-card">
      <div class="section-body balance-card">
        <div>
          <div class="label">当前余额</div>
          <div class="amount">{{ formatMoney(balanceAmount) }}</div>
        </div>
        <el-button type="primary" @click="loadData">刷新</el-button>
      </div>
    </el-card>

    <el-card class="section-card">
      <div class="section-body">
        <el-tabs v-model="activeTab">
          <el-tab-pane name="recharge" label="余额充值">
            <div class="form-grid-narrow">
              <div>
                <div class="form-label">充值方式</div>
                <el-radio-group v-model="rechargeMethod">
                  <el-radio v-for="item in rechargeMethods" :key="item.value" :value="item.value">
                    {{ item.label }}
                  </el-radio>
                </el-radio-group>
              </div>
              <div class="form-field">
                <div class="form-label">充值金额（元）</div>
                <el-input v-model="rechargeAmount" type="number" placeholder="请输入充值金额" />
              </div>
              <el-button type="primary" :loading="rechargeLoading" @click="handleRecharge">确认充值</el-button>

              <div v-if="rechargePayUrl" class="pay-box">
                <div class="form-label">支付链接</div>
                <a :href="rechargePayUrl" target="_blank" class="pay-link">{{ rechargePayUrl }}</a>
                <p class="feature-tip">若未自动跳转，请点击上方链接完成支付。支付成功后余额将自动到账。</p>
              </div>
              <div v-if="rechargeError" class="form-alert">
                <el-alert theme="error" :message="rechargeError" />
              </div>
            </div>
          </el-tab-pane>

          <el-tab-pane v-if="canBalanceWithdraw" name="withdraw" label="余额提现">
            <div class="form-grid-narrow">
              <div>
                <div class="form-label">提现方式</div>
                <el-radio-group v-model="withdrawMethod">
                  <el-radio v-for="item in withdrawMethods" :key="item.value" :value="item.value">
                    {{ item.label }}
                  </el-radio>
                </el-radio-group>
              </div>
              <div class="inline-grid">
                <div>
                  <div class="form-label">账户名称</div>
                  <el-input v-model="withdrawAccountName" placeholder="收款人姓名" />
                </div>
                <div>
                  <div class="form-label">账户号码</div>
                  <el-input v-model="withdrawAccountNo" placeholder="账号 / 卡号" />
                </div>
                <div>
                  <div class="form-label">提现金额（元）</div>
                  <el-input v-model="withdrawAmount" type="number" placeholder="请输入提现金额" />
                </div>
              </div>
              <div class="form-field">
                <div class="form-label">备注</div>
                <el-input v-model="withdrawNote" placeholder="可选" />
              </div>
              <p class="feature-tip">提交后需管理员审核，审核通过后余额将扣减并打款。</p>
              <el-button type="primary" :loading="withdrawLoading" @click="handleWithdraw">提交提现申请</el-button>
              <div v-if="withdrawError" class="form-alert">
                <el-alert theme="error" :message="withdrawError" />
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-card>

    <el-card class="section-card">
      <div class="section-body">
        <el-tabs v-model="recordTab">
          <el-tab-pane name="transactions" label="余额流水">
            <div class="admin-desktop-only">
              <EpDataTable :data="transactions" :columns="transactionColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="transactions.length === 0" class="admin-mobile-card-empty">暂无流水</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="(tx, i) in transactions" :key="i" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ tx.type || '-' }}</span>
                    <span :style="{ fontWeight: 600, color: tx.amount_cents >= 0 ? 'var(--app-success)' : 'var(--app-danger)' }">{{ tx.amount_cents >= 0 ? '+' : '' }}{{ formatMoney(tx.amount_cents) }}</span>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">余额</span>
                      <span class="admin-mobile-card-value">{{ formatMoney(tx.balance_cents) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">备注</span>
                      <span class="admin-mobile-card-value">{{ tx.note || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(tx.created_at) }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>
          <el-tab-pane name="recharges" label="充值记录">
            <div class="admin-desktop-only">
              <EpDataTable :data="recharges" :columns="rechargeColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="recharges.length === 0" class="admin-mobile-card-empty">暂无记录</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="item in recharges" :key="item.id || item.out_trade_no" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ formatMoney(item.amount_cents) }}</span>
                    <el-tag :type="item.status === 'paid' ? 'success' : item.status === 'pending' ? 'warning' : 'info'" effect="light" size="small">{{ item.status === 'paid' ? '已到账' : item.status === 'pending' ? '处理中' : (item.status || '-') }}</el-tag>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">单号</span>
                      <span class="admin-mobile-card-value">{{ item.out_trade_no || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">方式</span>
                      <span class="admin-mobile-card-value">{{ item.payment_method || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(item.created_at) }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>
          <el-tab-pane name="withdrawals" label="提现记录">
            <div class="admin-desktop-only">
              <EpDataTable :data="withdrawals" :columns="withdrawalColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="withdrawals.length === 0" class="admin-mobile-card-empty">暂无记录</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="item in withdrawals" :key="item.id" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ formatMoney(item.amount_cents) }}</span>
                    <el-tag :type="item.status === 'approved' ? 'success' : item.status === 'rejected' ? 'danger' : 'warning'" effect="light" size="small">{{ item.status === 'approved' ? '已通过' : item.status === 'rejected' ? '已拒绝' : '待审核' }}</el-tag>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">方式</span>
                      <span class="admin-mobile-card-value">{{ item.method || '-' }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">时间</span>
                      <span class="admin-mobile-card-value">{{ formatTime(item.created_at) }}</span>
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </el-tab-pane>
        </el-tabs>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag } from "element-plus"
import LoadingState from "@/components/common/LoadingState.vue"
import ErrorState from "@/components/common/ErrorState.vue"
import PermissionNotice from "@/components/common/PermissionNotice.vue"
import { useUserPermissions } from "@/composables/useUserPermissions"
import {
  api,
  type BalanceAccount,
  type BalanceRecharge,
  type BalanceTransaction,
  type BalanceWithdrawal,
} from "@/lib/api"

const { canBalanceWithdraw, hasRestrictions } = useUserPermissions()

const rechargeMethods = [
  { label: "微信支付", value: "wxpay" },
  { label: "支付宝", value: "alipay" },
  { label: "QQ 钱包", value: "qqpay" },
]

const withdrawMethods = [
  { label: "微信", value: "wechat" },
  { label: "支付宝", value: "alipay" },
  { label: "银行卡", value: "bank" },
]

const account = ref<BalanceAccount | null>(null)
const transactions = ref<BalanceTransaction[]>([])
const recharges = ref<BalanceRecharge[]>([])
const withdrawals = ref<BalanceWithdrawal[]>([])
const loading = ref(true)
const error = ref("")

const rechargeMethod = ref("alipay")
const rechargeAmount = ref("")
const rechargeLoading = ref(false)
const rechargePayUrl = ref("")
const rechargeError = ref("")

const withdrawMethod = ref("alipay")
const withdrawAmount = ref("")
const withdrawAccountName = ref("")
const withdrawAccountNo = ref("")
const withdrawNote = ref("")
const withdrawLoading = ref(false)
const withdrawError = ref("")

const activeTab = ref("recharge")
const recordTab = ref("transactions")

const loadData = async () => {
  try {
    loading.value = true
    const [accountRes, transactionsRes, rechargesRes, withdrawalsRes] = await Promise.all([
      api.getBalanceAccount(),
      api.listBalanceTransactions({ page: 1, pageSize: 50 }),
      api.listBalanceRecharges({ page: 1, pageSize: 50 }),
      api.listBalanceWithdrawals({ page: 1, pageSize: 50 }),
    ])
    account.value = accountRes.account || null
    transactions.value = transactionsRes.transactions || []
    recharges.value = rechargesRes.recharges || []
    withdrawals.value = withdrawalsRes.withdrawals || []
    error.value = ""
  } catch (err: any) {
    account.value = null
    transactions.value = []
    recharges.value = []
    withdrawals.value = []
    error.value = err.message || "加载余额数据失败"
  } finally {
    loading.value = false
  }
}

const formatMoney = (cents?: number) => {
  const value = typeof cents === "number" ? cents / 100 : 0
  return `${value.toFixed(2)} CNY`
}

const formatTime = (value?: string) => {
  if (!value) return "-"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return "-"
  return date.toLocaleString("zh-CN")
}

const balanceAmount = computed(() => account.value?.balance_cents ?? 0)

const handleRecharge = async () => {
  rechargeError.value = ""
  rechargePayUrl.value = ""
  const yuan = Number.parseFloat(rechargeAmount.value || "0")
  if (!Number.isFinite(yuan) || yuan <= 0) {
    rechargeError.value = "请输入有效的充值金额"
    return
  }
  const cents = Math.round(yuan * 100)
  rechargeLoading.value = true
  try {
    const res = await api.createBalanceRecharge({
      amount_cents: cents,
      payment_method: rechargeMethod.value,
    })
    rechargePayUrl.value = res.pay_url || res.recharge?.payment_url || ""
    if (rechargePayUrl.value) {
      window.open(rechargePayUrl.value, "_blank")
    }
    MessagePlugin.success("充值订单已创建，请完成支付")
    await loadData()
  } catch (err: any) {
    rechargeError.value = err.message || "创建充值订单失败"
  } finally {
    rechargeLoading.value = false
  }
}

const handleWithdraw = async () => {
  withdrawError.value = ""
  const yuan = Number.parseFloat(withdrawAmount.value || "0")
  if (!Number.isFinite(yuan) || yuan <= 0) {
    withdrawError.value = "请输入有效的提现金额"
    return
  }
  if (!withdrawAccountName.value.trim() || !withdrawAccountNo.value.trim()) {
    withdrawError.value = "请填写完整的收款账户信息"
    return
  }
  const cents = Math.round(yuan * 100)
  withdrawLoading.value = true
  try {
    await api.createBalanceWithdrawal({
      amount_cents: cents,
      method: withdrawMethod.value,
      account_name: withdrawAccountName.value.trim(),
      account_no: withdrawAccountNo.value.trim(),
      note: withdrawNote.value.trim(),
    })
    MessagePlugin.success("提现申请已提交，请等待审核")
    withdrawAmount.value = ""
    withdrawAccountName.value = ""
    withdrawAccountNo.value = ""
    withdrawNote.value = ""
    recordTab.value = "withdrawals"
    await loadData()
  } catch (err: any) {
    withdrawError.value = err.message || "提交提现申请失败"
  } finally {
    withdrawLoading.value = false
  }
}

const transactionColumns = computed(() => [
  { colKey: "created_at", title: "时间", width: 160, cell: (_h: any, { row }: { row: BalanceTransaction }) => formatTime(row.created_at) },
  { colKey: "type", title: "类型", width: 120 },
  {
    colKey: "amount_cents",
    title: "变动金额",
    width: 140,
    cell: (_h: any, { row }: { row: BalanceTransaction }) =>
      h("span", { style: `color:${row.amount_cents >= 0 ? 'var(--app-success)' : 'var(--app-danger)'}` }, `${row.amount_cents >= 0 ? "+" : ""}${formatMoney(row.amount_cents)}`),
  },
  { colKey: "balance_cents", title: "余额", width: 140, cell: (_h: any, { row }: { row: BalanceTransaction }) => formatMoney(row.balance_cents) },
  { colKey: "note", title: "备注", minWidth: 180 },
])

const rechargeColumns = computed(() => [
  { colKey: "out_trade_no", title: "充值单号", minWidth: 160 },
  { colKey: "amount_cents", title: "金额", width: 140, cell: (_h: any, { row }: { row: BalanceRecharge }) => formatMoney(row.amount_cents) },
  { colKey: "payment_method", title: "方式", width: 120 },
  {
    colKey: "status",
    title: "状态",
    width: 120,
    cell: (_h: any, { row }: { row: BalanceRecharge }) => {
      const status = row.status || "pending"
      const theme = status === "paid" ? "success" : status === "pending" ? "warning" : "info"
      const text = status === "paid" ? "已到账" : status === "pending" ? "处理中" : status
      return h(ElTag, { theme, effect: "light" }, () => text)
    },
  },
  { colKey: "created_at", title: "创建时间", width: 170, cell: (_h: any, { row }: { row: BalanceRecharge }) => formatTime(row.created_at) },
])

const withdrawalColumns = computed(() => [
  { colKey: "amount_cents", title: "金额", width: 140, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatMoney(row.amount_cents) },
  { colKey: "method", title: "方式", width: 120 },
  {
    colKey: "status",
    title: "状态",
    width: 120,
    cell: (_h: any, { row }: { row: BalanceWithdrawal }) => {
      const status = row.status || "pending"
      const theme = status === "approved" ? "success" : status === "rejected" ? "danger" : "warning"
      const text = status === "approved" ? "已通过" : status === "rejected" ? "已拒绝" : "待审核"
      return h(ElTag, { theme, effect: "light" }, () => text)
    },
  },
  { colKey: "created_at", title: "申请时间", width: 170, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatTime(row.created_at) },
])

onMounted(() => {
  loadData()
})
</script>

