<template>
  <div v-if="loading" class="loading">加载中...</div>
  <div v-else class="page">
    <div class="header">
      <div>
        <h1 class="title">账户余额</h1>
        <p class="subtitle">充值、提现与余额流水</p>
      </div>
    </div>

    <div v-if="error" class="error-box">
      <span>{{ error }}</span>
      <t-button size="small" theme="primary" @click="loadData">重试</t-button>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body balance-card">
        <div>
          <div class="label">当前余额</div>
          <div class="amount">{{ formatMoney(balanceAmount) }}</div>
        </div>
        <t-button theme="primary" @click="loadData">刷新</t-button>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <t-tabs v-model="activeTab">
          <t-tab-panel value="recharge" label="余额充值">
            <div class="form-grid">
              <div class="feature-tip">{{ rechargeNotice }}</div>
              <div>
                <div class="form-label">充值方式</div>
                <t-radio-group v-model="rechargeMethod" disabled>
                  <t-radio v-for="item in rechargeMethods" :key="item.value" :value="item.value">
                    {{ item.label }}
                  </t-radio>
                </t-radio-group>
              </div>
              <div class="form-field">
                <div class="form-label">充值金额</div>
                <t-input v-model="rechargeAmount" disabled />
              </div>
              <t-button theme="primary" :loading="false" :disabled="true" @click="handleRecharge">确认充值</t-button>
            </div>
          </t-tab-panel>

          <t-tab-panel value="withdraw" label="余额提现">
            <div class="form-grid">
              <div class="feature-tip">{{ withdrawNotice }}</div>
              <div>
                <div class="form-label">提现方式</div>
                <t-radio-group v-model="withdrawMethod" disabled>
                  <t-radio v-for="item in withdrawMethods" :key="item.value" :value="item.value">
                    {{ item.label }}
                  </t-radio>
                </t-radio-group>
              </div>
              <div class="inline-grid">
                <div>
                  <div class="form-label">账户名称</div>
                  <t-input v-model="withdrawAccountName" disabled />
                </div>
                <div>
                  <div class="form-label">账户号码</div>
                  <t-input v-model="withdrawAccountNo" disabled />
                </div>
                <div>
                  <div class="form-label">提现金额</div>
                  <t-input v-model="withdrawAmount" disabled />
                </div>
              </div>
              <div class="form-field">
                <div class="form-label">备注</div>
                <t-input v-model="withdrawNote" disabled />
              </div>
              <t-button theme="primary" :loading="false" :disabled="true" @click="handleWithdraw">提交提现申请</t-button>
            </div>
          </t-tab-panel>
        </t-tabs>
      </div>
    </t-card>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <t-tabs v-model="recordTab">
          <t-tab-panel value="transactions" label="余额流水">
            <div class="admin-desktop-only">
              <t-table :data="transactions" :columns="transactionColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="transactions.length === 0" class="admin-mobile-card-empty">暂无流水</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="(tx, i) in transactions" :key="i" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ tx.type || '-' }}</span>
                    <span :style="{ fontWeight: 600, color: tx.amount_cents >= 0 ? '#00a870' : '#ef4444' }">{{ tx.amount_cents >= 0 ? '+' : '' }}{{ formatMoney(tx.amount_cents) }}</span>
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
          </t-tab-panel>
          <t-tab-panel value="recharges" label="充值记录">
            <div class="admin-desktop-only">
              <t-table :data="recharges" :columns="rechargeColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="recharges.length === 0" class="admin-mobile-card-empty">暂无记录</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="item in recharges" :key="item.id || item.out_trade_no" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ formatMoney(item.amount_cents) }}</span>
                    <t-tag :theme="item.status === 'paid' ? 'success' : item.status === 'pending' ? 'warning' : 'default'" variant="light" size="small">{{ item.status === 'paid' ? '已到账' : item.status === 'pending' ? '处理中' : (item.status || '-') }}</t-tag>
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
          </t-tab-panel>
          <t-tab-panel value="withdrawals" label="提现记录">
            <div class="admin-desktop-only">
              <t-table :data="withdrawals" :columns="withdrawalColumns" row-key="id" />
            </div>
            <div class="admin-mobile-only">
              <div v-if="withdrawals.length === 0" class="admin-mobile-card-empty">暂无记录</div>
              <div v-else class="admin-mobile-cards">
                <div v-for="item in withdrawals" :key="item.id" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <span class="admin-mobile-card-title">{{ formatMoney(item.amount_cents) }}</span>
                    <t-tag :theme="item.status === 'approved' ? 'success' : item.status === 'rejected' ? 'danger' : 'warning'" variant="light" size="small">{{ item.status === 'approved' ? '已通过' : item.status === 'rejected' ? '已拒绝' : '待审核' }}</t-tag>
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
          </t-tab-panel>
        </t-tabs>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin, Tag } from "tdesign-vue-next"
import {
  api,
  type BalanceAccount,
  type BalanceRecharge,
  type BalanceTransaction,
  type BalanceWithdrawal,
} from "@/lib/api"

const rechargeMethods = [
  { label: "微信支付", value: "wxpay" },
  { label: "支付宝", value: "alipay" },
  { label: "转账汇款", value: "transfer" },
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
const rechargeNotice = "在线充值暂未开放，请联系管理员入账。"
const withdrawNotice = "在线提现暂未开放，请联系管理员处理。"

const rechargeMethod = ref("alipay")
const rechargeAmount = ref("")

const withdrawMethod = ref("alipay")
const withdrawAmount = ref("")
const withdrawAccountName = ref("")
const withdrawAccountNo = ref("")
const withdrawNote = ref("")

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
  void rechargeAmount
  void rechargeMethod
  MessagePlugin.warning(rechargeNotice)
}

const handleWithdraw = async () => {
  void withdrawAmount
  void withdrawMethod
  void withdrawAccountName
  void withdrawAccountNo
  void withdrawNote
  MessagePlugin.warning(withdrawNotice)
}

const transactionColumns = computed(() => [
  { colKey: "created_at", title: "时间", width: 160, cell: (_h: any, { row }: { row: BalanceTransaction }) => formatTime(row.created_at) },
  { colKey: "type", title: "类型", width: 120 },
  {
    colKey: "amount_cents",
    title: "变动金额",
    width: 140,
    cell: (_h: any, { row }: { row: BalanceTransaction }) =>
      h("span", { style: `color:${row.amount_cents >= 0 ? '#00a870' : '#ef4444'}` }, `${row.amount_cents >= 0 ? "+" : ""}${formatMoney(row.amount_cents)}`),
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
      const theme = status === "paid" ? "success" : status === "pending" ? "warning" : "default"
      const text = status === "paid" ? "已到账" : status === "pending" ? "处理中" : status
      return h(Tag, { theme, variant: "light" }, () => text)
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
      return h(Tag, { theme, variant: "light" }, () => text)
    },
  },
  { colKey: "created_at", title: "申请时间", width: 170, cell: (_h: any, { row }: { row: BalanceWithdrawal }) => formatTime(row.created_at) },
])

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.loading {
  min-height: 60vh;
  display: flex;
  align-items: center;
  justify-content: center;
}

.balance-card {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
}

.label {
  font-size: 13px;
  color: #94a3b8;
  font-weight: 500;
  text-transform: uppercase;
  letter-spacing: 0.3px;
}

.amount {
  font-size: 28px;
  font-weight: 800;
  color: #0f172a;
  margin-top: 6px;
  letter-spacing: -0.02em;
}

.form-grid {
  display: flex;
  flex-direction: column;
  gap: 16px;
  max-width: 640px;
}

.form-field {
  max-width: 320px;
}

.form-label {
  font-size: 14px;
  color: #475569;
  font-weight: 500;
  margin-bottom: 8px;
}

.feature-tip {
  font-size: 13px;
  color: #94a3b8;
}

.inline-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 16px;
}

@media (max-width: 768px) {
  .balance-card {
    flex-direction: column;
    align-items: flex-start;
  }
  .form-field {
    max-width: 100%;
  }
}
</style>
