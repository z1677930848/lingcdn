<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">WAF 策略</h1>
        <p class="subtitle">人机验证 · 5秒盾 · 速率限制 · IP 封禁 — 全局 / 集群 / 域名级别防护。</p>
      </div>
      <div class="header-actions">
        <el-button plain :loading="applyingRuleset" @click="applyOWASPRuleset">
          <template #icon><EpIcon name="secured" /></template>
          应用 OWASP 规则集
        </el-button>
        <el-button type="primary" @click="openDialog()">
          <template #icon><EpIcon name="add" /></template>
          新建策略
        </el-button>
        <el-button link :loading="loading" @click="load">
          <template #icon><EpIcon name="refresh" /></template>
          刷新
        </el-button>
      </div>
    </div>

    <ErrorState v-if="error" :message="error" @retry="load" />

    <!-- 策略列表 -->
    <el-card class="section-card" v-if="!detailPolicy">
      <el-tabs v-model="activeScope" @tab-change="load">
        <el-tab-pane name="all" label="全部" />
        <el-tab-pane name="global" label="全局" />
        <el-tab-pane name="domain" label="按域名" />
        <el-tab-pane name="line_group" label="按集群" />
      </el-tabs>

      <div class="admin-desktop-only">
        <EpDataTable
          :data="visible"
          :columns="columns"
          row-key="id"
          size="small"
          :loading="loading"
          empty-text="暂无策略"
          :pagination="{
            defaultCurrent: 1,
            defaultPageSize: 10,
            pageSizeOptions: [10, 20, 50, 100],
            showJumper: true,
            total: visible.length,
          }"
        />
      </div>
      <div class="admin-mobile-only">
        <div v-if="loading" class="loading-row"><div v-loading="true" style="min-height:48px" /></div>
        <div v-else-if="visible.length === 0" class="admin-mobile-card-empty">暂无策略</div>
        <div v-else class="admin-mobile-cards">
          <div v-for="p in visible" :key="p.id" class="admin-mobile-card">
            <div class="admin-mobile-card-header">
              <span class="admin-mobile-card-title">{{ p.name || p.id }}</span>
              <div class="admin-mobile-card-tags">
                <el-tag size="small" :type="p.enabled ? 'success' : 'info'" effect="light">
                  {{ p.enabled ? '启用' : '停用' }}
                </el-tag>
              </div>
            </div>
            <div class="admin-mobile-card-rows">
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">作用域</span>
                <span class="admin-mobile-card-value">{{ scopeDisplay(p) }}</span>
              </div>
              <div class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">规则数</span>
                <span class="admin-mobile-card-value">{{ (p.rules || []).length }}</span>
              </div>
              <div v-if="p.description" class="admin-mobile-card-row">
                <span class="admin-mobile-card-label">描述</span>
                <span class="admin-mobile-card-value">{{ p.description }}</span>
              </div>
            </div>
            <div class="admin-mobile-card-actions">
              <el-switch :model-value="p.enabled" size="small" @change="(v: boolean) => toggle(p, v)" />
              <span class="switch-text">{{ p.enabled ? '已启用' : '已停用' }}</span>
              <div class="action-spacer" />
              <el-button size="small" type="primary" link @click="enterDetail(p)">规则</el-button>
              <el-button size="small" link @click="openDialog(p)">编辑</el-button>
              <el-button size="small" type="danger" link @click="confirmDelete(p)">删除</el-button>
            </div>
          </div>
        </div>
      </div>
    </el-card>

    <!-- 策略详情（规则管理面板） -->
    <template v-if="detailPolicy">
      <div class="detail-header">
        <el-button link shape="square" @click="detailPolicy = null">
          <template #icon><EpIcon name="arrow-left" /></template>
        </el-button>
        <h2 class="detail-title">{{ detailPolicy.name }}</h2>
        <el-tag :type="detailPolicy.enabled ? 'success' : 'info'" size="default">
          {{ detailPolicy.enabled ? '已启用' : '已停用' }}
        </el-tag>
        <el-tag type="primary" effect="light" size="default">{{ scopeDisplay(detailPolicy) }}</el-tag>
      </div>

      <el-card class="section-card" header-bordered title="防护规则">
        <template #actions>
          <el-button type="primary" size="small" @click="openRuleDialog()">
            <template #icon><EpIcon name="add" /></template>
            添加规则
          </el-button>
        </template>
        <EpDataTable
          :data="detailPolicy.rules || []"
          :columns="ruleColumns"
          row-key="id"
          size="small"
          empty-text="暂无规则，点击「添加规则」创建人机验证或限速策略"
        />
      </el-card>
    </template>

    <!-- 策略编辑对话框 -->
    <EpDialog append-to-body
      v-model="dialogOpen"
      :title="editing ? '编辑 WAF 策略' : '新建 WAF 策略'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      @confirm="submit"
    >
      <div class="form-grid-vertical">
        <div class="form-row-v">
          <label class="form-label-v">名称</label>
          <el-input v-model="form.name" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">作用域</label>
          <el-radio-group v-model="form.scope" class="form-input-v">
            <el-radio value="global">全局</el-radio>
            <el-radio value="domain">域名</el-radio>
            <el-radio value="line_group">集群</el-radio>
          </el-radio-group>
          <div class="form-hint-v">全局策略作用于所有请求；域名/集群策略仅作用于下方选择的目标。</div>
        </div>
        <div class="form-row-v" v-if="form.scope === 'domain'">
          <label class="form-label-v">目标域名</label>
          <EpSelect v-model="form.scope_id" :options="domainOptions" filterable class="form-input-v" />
        </div>
        <div class="form-row-v" v-if="form.scope === 'line_group'">
          <label class="form-label-v">目标集群</label>
          <EpSelect v-model="form.scope_id" :options="clusterOptions" filterable class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">描述</label>
          <el-input v-model="form.description" :autosize="{ minRows: 2 }" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="form.enabled" />
        </div>
      </div>
    </EpDialog>

    <!-- 规则编辑对话框 -->
    <EpDialog append-to-body
      v-model="ruleDialogOpen"
      :title="ruleEditing ? '编辑防护规则' : '添加防护规则'"
      :confirm-btn="{ content: '保存' }"
      cancel-btn="取消"
      width="640px"
      @confirm="submitRule"
    >
      <div class="form-grid-vertical">
        <div class="form-row-v">
          <label class="form-label-v">规则类型</label>
          <EpSelect v-model="ruleForm.type" :options="ruleTypeOptions" class="form-input-v" />
          <div class="form-hint-v">{{ ruleTypeHint }}</div>
        </div>

        <!-- 人机验证字段 -->
        <template v-if="ruleForm.type === 'challenge_captcha'">
          <div class="form-row-v">
            <label class="form-label-v">验证码类型</label>
            <EpSelect v-model="ruleForm.captcha_type" :options="captchaTypeOptions" class="form-input-v" />
          </div>
          <div class="form-row-v">
            <label class="form-label-v">触发 QPS</label>
            <el-input-number v-model="ruleForm.auto_challenge_qps" :min="1" :max="100000" class="form-input-v" style="max-width:200px" />
            <div class="form-hint-v">每 IP 每秒请求超过此值触发人机验证。</div>
          </div>
          <div class="form-row-v">
            <label class="form-label-v">失败上限</label>
            <el-input-number v-model="ruleForm.threshold" :min="1" :max="100" class="form-input-v" style="max-width:200px" />
            <div class="form-hint-v">验证失败达到此次数后直接封禁。</div>
          </div>
        </template>

        <!-- 5秒盾字段 -->
        <template v-if="ruleForm.type === 'shield_5s'">
          <div class="form-row-v">
            <label class="form-label-v">触发 QPS</label>
            <el-input-number v-model="ruleForm.auto_challenge_qps" :min="1" :max="100000" class="form-input-v" style="max-width:200px" />
            <div class="form-hint-v">每 IP 每秒请求超过此值触发 5 秒浏览器检测。</div>
          </div>
          <div class="form-row-v">
            <label class="form-label-v">盾持续秒数</label>
            <el-input-number v-model="ruleForm.shield_seconds" :min="3" :max="30" class="form-input-v" style="max-width:200px" />
          </div>
        </template>

        <!-- 速率限制字段 -->
        <template v-if="ruleForm.type === 'rate_limit'">
          <div class="form-row-v">
            <label class="form-label-v">请求阈值</label>
            <el-input-number v-model="ruleForm.threshold" :min="1" :max="100000" class="form-input-v" style="max-width:200px" />
            <div class="form-hint-v">在时间窗口内，单 IP 请求超过此数触发动作。</div>
          </div>
          <div class="form-row-v">
            <label class="form-label-v">时间窗口 (秒)</label>
            <el-input-number v-model="ruleForm.window_seconds" :min="1" :max="86400" class="form-input-v" style="max-width:200px" />
          </div>
        </template>

        <!-- IP/CIDR 字段 -->
        <template v-if="ruleForm.type === 'ip_cidr'">
          <div class="form-row-v">
            <label class="form-label-v">动作</label>
            <el-radio-group v-model="ruleForm.action" class="form-input-v">
              <el-radio value="deny">封禁</el-radio>
              <el-radio value="allow">放行</el-radio>
            </el-radio-group>
          </div>
          <div class="form-row-v">
            <label class="form-label-v">IP / CIDR</label>
            <el-input v-model="ruleForm.value" placeholder="192.168.1.0/24 或单个 IP" class="form-input-v" />
          </div>
        </template>

        <!-- 通用字段 -->
        <div class="form-row-v">
          <label class="form-label-v">封禁时长 (秒)</label>
          <el-input-number v-model="ruleForm.ban_seconds" :min="0" :max="86400" class="form-input-v" style="max-width:200px" />
          <div class="form-hint-v">触发后封禁 IP 的时长，0 表示不封禁仅拦截本次。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">封禁模式</label>
          <EpSelect v-model="ruleForm.ban_mode" :options="banModeOptions" class="form-input-v" style="max-width:200px" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">路径前缀</label>
          <el-input v-model="ruleForm.path_prefix" placeholder="留空匹配全部路径" class="form-input-v" />
          <div class="form-hint-v">仅对匹配此前缀的路径生效，如 /api/</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">优先级</label>
          <el-input-number v-model="ruleForm.priority" :min="1" :max="10000" class="form-input-v" style="max-width:200px" />
          <div class="form-hint-v">数值越小越先匹配。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">备注</label>
          <el-input v-model="ruleForm.note" class="form-input-v" />
        </div>
        <div class="form-row-v">
          <label class="form-label-v">仅记录</label>
          <el-switch v-model="ruleForm.log_only" />
          <div class="form-hint-v">开启后仅记录日志不执行封禁，用于调试。</div>
        </div>
        <div class="form-row-v">
          <label class="form-label-v">启用</label>
          <el-switch v-model="ruleForm.enabled" />
        </div>
      </div>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import { MessagePlugin } from "@/lib/ep-message"
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, reactive, ref } from "vue"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElButton, ElTag, ElSwitch } from "element-plus"
import { api, type Cluster, type Domain, type WAFPolicy, type WAFRule } from "@/lib/api"
import ErrorState from "@/components/common/ErrorState.vue"

const loading = ref(false)
const applyingRuleset = ref(false)
const error = ref("")
const policies = ref<WAFPolicy[]>([])
const domains = ref<Domain[]>([])
const clusters = ref<Cluster[]>([])
const activeScope = ref<"all" | "global" | "domain" | "line_group">("all")

// 策略详情（规则管理）
const detailPolicy = ref<WAFPolicy | null>(null)

const dialogOpen = ref(false)
const editing = ref<WAFPolicy | null>(null)
const form = reactive<WAFPolicy>({
  id: "",
  name: "",
  scope: "global",
  scope_id: "",
  description: "",
  enabled: true,
})

// 规则编辑
const ruleDialogOpen = ref(false)
const ruleEditing = ref<WAFRule | null>(null)
const ruleForm = reactive<WAFRule>({
  id: "",
  policy_id: "",
  type: "challenge_captcha",
  action: "deny",
  value: "",
  threshold: 3,
  window_seconds: 1,
  shield_seconds: 5,
  auto_challenge_qps: 50,
  ban_seconds: 300,
  ban_mode: "ipset",
  captcha_type: "slide",
  path_prefix: "",
  note: "",
  log_only: false,
  priority: 100,
  enabled: true,
})

const ruleTypeOptions = [
  { label: "人机验证（Captcha）", value: "challenge_captcha" },
  { label: "5秒盾（JS 检测）", value: "shield_5s" },
  { label: "速率限制", value: "rate_limit" },
  { label: "IP / CIDR 黑白名单", value: "ip_cidr" },
]

const captchaTypeOptions = [
  { label: "滑块验证", value: "slide" },
  { label: "点选验证", value: "click" },
  { label: "旋转验证", value: "rotate" },
  { label: "区域滑块", value: "slide_region" },
  { label: "JS 挑战", value: "js_challenge" },
]

const banModeOptions = [
  { label: "ipset（内核级丢包）", value: "ipset" },
  { label: "drop（应用层丢弃）", value: "drop" },
  { label: "page（返回封禁页面）", value: "page" },
]

const ruleTypeHint = computed(() => {
  const m: Record<string, string> = {
    challenge_captcha: "当 QPS 超过阈值时弹出验证码，验证失败达上限后封禁 IP。",
    shield_5s: "当 QPS 超过阈值时返回 JS 检测页面，浏览器通过后放行。",
    rate_limit: "在时间窗口内请求超过阈值直接执行封禁动作。",
    ip_cidr: "按 IP 或 CIDR 段直接放行/封禁。",
  }
  return m[ruleForm.type] || ""
})

const ruleTypeLabel = (type: string) => {
  const m: Record<string, string> = {
    challenge_captcha: "人机验证",
    shield_5s: "5秒盾",
    rate_limit: "限速",
    ip_cidr: "IP规则",
    geo_block: "地域拦截",
    block_transparent_proxy: "透明代理拦截",
    sql_injection: "SQL 注入",
    xss: "XSS",
    path_traversal: "路径穿越",
    ua_block: "扫描器 UA",
    method_block: "HTTP 方法",
  }
  return m[type] || type
}

const applyOWASPRuleset = async () => {
  applyingRuleset.value = true
  try {
    await api.applyWAFRuleset("owasp_common", { scope: "global" })
    MessagePlugin.success("OWASP 规则集已应用，配置将自动同步到节点")
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "应用规则集失败")
  } finally {
    applyingRuleset.value = false
  }
}

const load = async () => {
  loading.value = true
  error.value = ""
  try {
    const [pRes, dRes, cRes] = await Promise.all([
      api.listWAFPolicies(),
      api.listDomains(),
      api.listClusters(),
    ])
    policies.value = pRes.policies || []
    domains.value = dRes.domains || []
    clusters.value = cRes.clusters || []
    // 如果正在查看详情，同步刷新
    if (detailPolicy.value) {
      const fresh = policies.value.find(p => p.id === detailPolicy.value!.id)
      detailPolicy.value = fresh || null
    }
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const domainOptions = computed(() =>
  (domains.value || []).map((d) => ({ label: d.name, value: d.id })),
)
const clusterOptions = computed(() =>
  (clusters.value || []).map((c) => ({ label: c.name || c.id, value: c.id })),
)

const visible = computed(() => {
  if (activeScope.value === "all") return policies.value
  return policies.value.filter((p) => p.scope === activeScope.value)
})

const openDialog = (p?: WAFPolicy) => {
  editing.value = p || null
  if (p) {
    Object.assign(form, { ...p, rules: undefined })
  } else {
    form.id = ""
    form.name = "新策略"
    form.scope = "global"
    form.scope_id = ""
    form.description = ""
    form.enabled = true
  }
  dialogOpen.value = true
}

const submit = async () => {
  if (form.scope !== "global" && !String(form.scope_id || "").trim()) {
    MessagePlugin.error("请选择目标 " + (form.scope === "domain" ? "域名" : "集群"))
    return
  }
  try {
    if (editing.value) {
      await api.updateWAFPolicy(editing.value.id, form)
      MessagePlugin.success("策略已更新")
    } else {
      await api.createWAFPolicy(form)
      MessagePlugin.success("策略已创建")
    }
    dialogOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  }
}

const toggle = async (p: WAFPolicy, enabled: boolean) => {
  const prev = p.enabled
  p.enabled = enabled
  try {
    await api.setWAFPolicyEnabled(p.id, enabled)
    await load()
  } catch (err: any) {
    p.enabled = prev
    MessagePlugin.error(err?.message || "更新失败")
  }
}

const confirmDelete = (p: WAFPolicy) => {
  const inst = DialogPlugin.confirm({
    header: "删除 WAF 策略",
    body: `确认删除策略「${p.name || p.id}」吗？其下所有规则一并删除。`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    onConfirm: async () => {
      try {
        await api.deleteWAFPolicy(p.id)
        MessagePlugin.success("策略已删除")
        if (detailPolicy.value?.id === p.id) detailPolicy.value = null
        await load()
      } catch (err: any) {
        MessagePlugin.error(err?.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const enterDetail = (p: WAFPolicy) => {
  detailPolicy.value = p
}

const scopeDisplay = (p: WAFPolicy) => {
  if (p.scope === "global") return "全局"
  if (p.scope === "domain") {
    const d = domains.value.find((x) => x.id === p.scope_id)
    return `域名 · ${d?.name || p.scope_id || "-"}`
  }
  if (p.scope === "line_group") {
    const c = clusters.value.find((x) => x.id === p.scope_id)
    return `集群 · ${c?.name || p.scope_id || "-"}`
  }
  return p.scope
}

// --- 规则管理 ---
const openRuleDialog = (rule?: WAFRule) => {
  ruleEditing.value = rule || null
  if (rule) {
    Object.assign(ruleForm, rule)
  } else {
    ruleForm.id = ""
    ruleForm.policy_id = detailPolicy.value?.id || ""
    ruleForm.type = "challenge_captcha"
    ruleForm.action = "deny"
    ruleForm.value = ""
    ruleForm.threshold = 3
    ruleForm.window_seconds = 1
    ruleForm.shield_seconds = 5
    ruleForm.auto_challenge_qps = 50
    ruleForm.ban_seconds = 300
    ruleForm.ban_mode = "ipset"
    ruleForm.captcha_type = "slide"
    ruleForm.path_prefix = ""
    ruleForm.note = ""
    ruleForm.log_only = false
    ruleForm.priority = 100
    ruleForm.enabled = true
  }
  ruleDialogOpen.value = true
}

const submitRule = async () => {
  if (!detailPolicy.value) return
  const pol = detailPolicy.value
  // 构建新的 rules 列表：替换编辑的或追加新的
  let rules = [...(pol.rules || [])]
  if (ruleEditing.value) {
    rules = rules.map(r => r.id === ruleEditing.value!.id ? { ...ruleForm } : r)
  } else {
    rules.push({ ...ruleForm })
  }
  try {
    await api.updateWAFPolicy(pol.id, { ...pol, rules } as any)
    MessagePlugin.success(ruleEditing.value ? "规则已更新" : "规则已添加")
    ruleDialogOpen.value = false
    await load()
  } catch (err: any) {
    MessagePlugin.error(err?.message || "保存失败")
  }
}

const toggleRule = async (rule: WAFRule, enabled: boolean) => {
  if (!detailPolicy.value) return
  const pol = detailPolicy.value
  const prev = rule.enabled
  rule.enabled = enabled
  const rules = (pol.rules || []).map(r => r.id === rule.id ? { ...r, enabled } : r)
  try {
    await api.updateWAFPolicy(pol.id, { ...pol, rules } as any)
    await load()
  } catch (err: any) {
    rule.enabled = prev
    MessagePlugin.error(err?.message || "更新失败")
  }
}

const deleteRule = (rule: WAFRule) => {
  if (!detailPolicy.value) return
  const pol = detailPolicy.value
  const inst = DialogPlugin.confirm({
    header: "删除规则",
    body: `确认删除此${ruleTypeLabel(rule.type)}规则吗？`,
    theme: "warning",
    confirmBtn: { content: "删除", theme: "danger" },
    onConfirm: async () => {
      const rules = (pol.rules || []).filter(r => r.id !== rule.id)
      try {
        await api.updateWAFPolicy(pol.id, { ...pol, rules } as any)
        MessagePlugin.success("规则已删除")
        await load()
      } catch (err: any) {
        MessagePlugin.error(err?.message || "删除失败")
      } finally {
        inst.destroy()
      }
    },
    onClose: () => inst.destroy(),
  })
}

const ruleSummary = (rule: WAFRule) => {
  if (rule.type === "challenge_captcha") {
    const ct = rule.captcha_type || "slide"
    return `QPS>${rule.auto_challenge_qps || 0} → ${ct}验证, 失败${rule.threshold || 0}次封禁`
  }
  if (rule.type === "shield_5s") {
    return `QPS>${rule.auto_challenge_qps || 0} → ${rule.shield_seconds || 5}秒盾`
  }
  if (rule.type === "rate_limit") {
    return `${rule.threshold || 0}次/${rule.window_seconds || 1}秒`
  }
  if (rule.type === "ip_cidr") {
    return `${rule.action === "allow" ? "放行" : "封禁"} ${rule.value || "-"}`
  }
  if (rule.type === "method_block") {
    return `拦截 ${(rule.methods || []).join(", ") || rule.value || "-"}`
  }
  if (["sql_injection", "xss", "path_traversal", "ua_block"].includes(rule.type)) {
    return rule.note || "正则匹配"
  }
  if (rule.type === "geo_block") {
    return `拦截地域 ${(rule as any).geo_countries?.join?.(", ") || "-"}`
  }
  return rule.type
}

// --- 策略列表表格 ---
const columns = computed(() => [
  { colKey: "name", title: "名称", minWidth: 160 },
  {
    colKey: "scope",
    title: "作用域",
    width: 200,
    cell: (_h: any, { row }: { row: WAFPolicy }) => scopeDisplay(row),
  },
  {
    colKey: "rules_count",
    title: "规则数",
    width: 80,
    cell: (_h: any, { row }: { row: WAFPolicy }) => String((row.rules || []).length),
  },
  { colKey: "description", title: "描述", minWidth: 180 },
  {
    colKey: "enabled",
    title: "启用",
    width: 80,
    cell: (_h: any, { row }: { row: WAFPolicy }) =>
      h(ElSwitch, { size: "small", modelValue: row.enabled, "onUpdate:modelValue": (v: boolean) => toggle(row, v) }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 180,
    fixed: "right",
    cell: (_h: any, { row }: { row: WAFPolicy }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(ElButton, { size: "small", type: "primary", link: true, onClick: () => enterDetail(row) }, () => "规则"),
        h(ElButton, { size: "small", link: true, onClick: () => openDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => confirmDelete(row) }, () => "删除"),
      ]),
  },
])

// --- 规则表格 ---
const ruleColumns = computed(() => [
  {
    colKey: "type",
    title: "类型",
    width: 110,
    cell: (_h: any, { row }: { row: WAFRule }) => {
      const themeMap: Record<string, "info" | "success" | "warning" | "primary" | "danger"> = {
        challenge_captcha: "warning",
        shield_5s: "primary",
        rate_limit: "danger",
        ip_cidr: "info",
        sql_injection: "danger",
        xss: "danger",
        path_traversal: "danger",
        ua_block: "warning",
        method_block: "warning",
        geo_block: "primary",
        block_transparent_proxy: "primary",
      }
      return h(ElTag, { type: themeMap[row.type] || ("info" as const), size: "small", effect: "light" }, () => ruleTypeLabel(row.type))
    },
  },
  {
    colKey: "summary",
    title: "配置摘要",
    minWidth: 240,
    cell: (_h: any, { row }: { row: WAFRule }) => ruleSummary(row),
  },
  {
    colKey: "ban",
    title: "封禁",
    width: 120,
    cell: (_h: any, { row }: { row: WAFRule }) =>
      `${row.ban_seconds || 0}秒 / ${row.ban_mode || "ipset"}`,
  },
  {
    colKey: "priority",
    title: "优先级",
    width: 80,
    cell: (_h: any, { row }: { row: WAFRule }) => String(row.priority || 0),
  },
  {
    colKey: "enabled",
    title: "启用",
    width: 80,
    cell: (_h: any, { row }: { row: WAFRule }) =>
      h(ElSwitch, { size: "small", modelValue: row.enabled, "onUpdate:modelValue": (v: boolean) => toggleRule(row, v) }),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 140,
    fixed: "right",
    cell: (_h: any, { row }: { row: WAFRule }) =>
      h("div", { style: "display:flex;gap:4px" }, [
        h(ElButton, { size: "small", type: "primary", link: true, onClick: () => openRuleDialog(row) }, () => "编辑"),
        h(ElButton, { size: "small", type: "danger", link: true, onClick: () => deleteRule(row) }, () => "删除"),
      ]),
  },
])

onMounted(load)
</script>

