<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">节点管理</h1>
        <p class="subtitle">查看与管理所有 CDN 边缘节点 / 监控状态 / 升级控制</p>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <t-tabs v-model="tab">
        <t-tab-panel value="list" label="节点列表">
          <div class="toolbar">
            <t-input v-model="search" clearable placeholder="搜索主机名 / IP" class="w-260" />
            <t-select v-model="statusFilter" class="w-140" :options="statusOptions" />
            <t-button variant="outline" @click="handleOpenInstallDialog">添加节点</t-button>
            <t-button :loading="loading" @click="() => loadList({ page: 1 })">刷新</t-button>
          </div>
          <div v-if="selectedRowKeys.length > 0" class="batch-bar">
            <span class="batch-bar-label">已选择 {{ selectedRowKeys.length }} 个节点</span>
            <t-button size="small" theme="warning" @click="openUpgradeDialog(selectedRowKeys)">批量升级</t-button>
            <t-button size="small" theme="danger" variant="outline" @click="handleBatchDisable">批量禁用</t-button>
            <t-button size="small" theme="danger" @click="handleBatchDelete">批量删除</t-button>
            <t-button size="small" variant="outline" @click="selectedRowKeys = []">取消选择</t-button>
          </div>

          <div class="admin-desktop-only">
            <t-table
              :data="nodes"
              :columns="listColumns"
              row-key="id"
              size="small"
              :loading="loading"
              :pagination="listPagination"
              bordered
              hover
              stripe
              empty="暂无节点"
              :selected-row-keys="selectedRowKeys"
              @select-change="handleSelectionChange"
              @page-change="handleListPageChange"
            />
          </div>

          <div class="admin-mobile-only">
            <div v-if="loading" style="text-align:center;padding:32px 0"><t-loading /></div>
            <template v-else>
              <div v-if="nodes.length" class="admin-mobile-cards">
                <div v-for="node in nodes" :key="node.id" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <div style="min-width:0">
                      <div class="admin-mobile-card-title">{{ node.hostname || node.id }}</div>
                      <div class="admin-mobile-card-subtitle">{{ node.public_ip || '-' }}</div>
                    </div>
                    <div class="admin-mobile-card-tags">
                      <t-tag :theme="(statusTagInfo(node).theme as any)" variant="light" size="small">{{ statusTagInfo(node).label }}</t-tag>
                    </div>
                  </div>
                  <div class="admin-mobile-card-rows">
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">CPU</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(node.cpu_usage) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">内存</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(node.mem_usage) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">磁盘</span>
                      <span class="admin-mobile-card-value">{{ formatPercent(node.disk_usage) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">带宽</span>
                      <span class="admin-mobile-card-value">↑{{ formatBitsPerSec(node.bandwidth_up_bps) }} ↓{{ formatBitsPerSec(node.bandwidth_down_bps) }}</span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">版本</span>
                      <span class="admin-mobile-card-value">
                        {{ node.version || '-' }}
                        <t-tag
                          v-if="upgradeNodeMap[node.id] && upgradeNodeMap[node.id].status === 'upgrade_needed'"
                          theme="warning" variant="light" size="small" style="margin-left:6px"
                        >
                          可升级到 {{ upgradeNodeMap[node.id].target_version }}
                        </t-tag>
                      </span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">心跳</span>
                      <span class="admin-mobile-card-value">{{ formatTime(node.last_heartbeat) }}</span>
                    </div>
                  </div>
                  <div class="admin-mobile-card-actions">
                    <t-button size="small" variant="text" theme="primary" @click="openDetail(node)">详情</t-button>
                    <t-button
                      v-if="upgradeNodeMap[node.id] && upgradeNodeMap[node.id].status === 'upgrade_needed'"
                      size="small" variant="outline" @click="openUpgradeDialog([node.id])"
                    >升级</t-button>
                    <t-dropdown trigger="click" @click="handleMobileDropdown($event, node)">
                      <t-button size="small" variant="text">更多 ▾</t-button>
                      <t-dropdown-menu>
                        <t-dropdown-item :value="'toggle'">
                          {{ (node.status || '').trim().toLowerCase() !== 'disabled' ? '禁用节点' : '启用节点' }}
                        </t-dropdown-item>
                        <t-dropdown-item :value="'monitor'">监控配置</t-dropdown-item>
                        <t-dropdown-item :value="'reset-token'">重置令牌</t-dropdown-item>
                        <t-dropdown-item divider />
                        <t-dropdown-item :value="'delete'" class="dropdown-danger">删除节点</t-dropdown-item>
                      </t-dropdown-menu>
                    </t-dropdown>
                  </div>
                </div>
                <t-pagination
                  class="admin-mobile-pagination"
                  :current="listPagination.current"
                  :page-size="listPagination.pageSize"
                  :total="listPagination.total"
                  :show-jumper="false"
                  size="small"
                  @current-change="(v: number) => handleListPageChange({ current: v, pageSize: listPagination.pageSize })"
                />
              </div>
              <div v-else class="admin-mobile-card-empty">暂无节点</div>
            </template>
          </div>
        </t-tab-panel>

      </t-tabs>
    </t-card>

    <t-dialog
      v-model:visible="monitorDialogOpen"
      header="节点监控配置"
      :confirm-btn="{ content: monitorSaving ? '保存中...' : '保存', loading: monitorSaving, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="handleSaveMonitorConfig"
    >
      <div class="dialog-grid">
        <div class="dialog-tip">节点：{{ monitorTarget?.hostname || monitorTarget?.id || '-' }}</div>
        <div class="row">
          <span class="row-label">启用监控</span>
          <t-switch v-model="monitorForm.enabled" />
        </div>
        <div>
          <div class="dialog-label">探测协议</div>
          <t-radio-group v-model="monitorForm.protocol">
            <t-radio value="http">HTTP</t-radio>
            <t-radio value="ping">Ping</t-radio>
            <t-radio value="tcp">TCP</t-radio>
          </t-radio-group>
        </div>
        <div class="grid-2">
          <div>
            <div class="dialog-label">超时（秒）</div>
            <t-input-number v-model="monitorForm.timeoutSeconds" :min="1" :max="60" />
          </div>
          <div>
            <div class="dialog-label">端口</div>
            <!-- Plain numeric input; the stepper encouraged +1 clicks to
                 walk 80→443, which is the wrong affordance for port
                 fields. Browser validates min/max via attributes. -->
            <t-input v-model="monitorPortText" type="number" :disabled="monitorForm.protocol === 'ping'" placeholder="1-65535" />
          </div>
        </div>
        <div>
          <div class="dialog-label">连续失败阈值</div>
          <t-input-number v-model="monitorForm.failThreshold" :min="1" :max="10" />
        </div>
        <div class="dialog-tip">HTTP/TCP 协议会探测端口；Ping 协议忽略端口，仅做 ICMP 探测。</div>
      </div>
    </t-dialog>

    <t-dialog v-model:visible="installDialogOpen" header="添加节点" width="760px">
      <template #footer>
        <t-button variant="outline" @click="installDialogOpen = false">关闭</t-button>
        <t-button v-if="installMode === 'manual'" :loading="installLoading" @click="handleFetchInstallCommand()">
          重新生成命令
        </t-button>
        <t-button v-else :loading="installLoading" @click="handleInstallViaSSH()">
          通过 SSH 安装
        </t-button>
      </template>
      <div class="dialog-grid">
        <div>
          <div class="dialog-label">安装方式</div>
          <t-radio-group v-model="installMode">
            <t-radio value="manual">手动复制命令</t-radio>
            <t-radio value="ssh">通过 SSH 自动安装</t-radio>
          </t-radio-group>
        </div>
        <div v-if="installMode === 'manual'" class="dialog-grid">
          <div class="dialog-title">1. 复制安装命令</div>
          <div class="dialog-tip">在目标节点以 root 身份执行下列命令完成安装。</div>
          <div>
            <div class="dialog-label">安装命令</div>
            <t-textarea v-model="installCommand" :autosize="{ minRows: 5, maxRows: 10 }" />
            <div class="copy-row">

              <t-button variant="outline" :disabled="!installCommand" @click="copyCommand">复制命令</t-button>
            </div>
          </div>
          <t-divider />
          <div class="dialog-title">2. 等待节点上线</div>
          <div class="dialog-tip">节点首次上报通常 1-2 分钟，gRPC 通道建立后会出现在列表。</div>
          <t-button variant="outline" :loading="loading" @click="() => loadList({ page: 1 })">刷新列表</t-button>
        </div>

        <div v-else class="dialog-grid">
          <div class="dialog-title">1. SSH 凭据</div>
          <div class="dialog-tip">请填写 SSH 凭据，目标主机需为 Linux 且当前 SSH 用户具备 root 权限。</div>
          <div class="grid-2">
            <div>
              <div class="dialog-label">主机 IP / 域名</div>
              <t-input v-model="installParams.sshHost" />
            </div>
            <div>
              <div class="dialog-label">SSH 端口</div>
              <t-input v-model="sshPortText" type="number" placeholder="1-65535" />
            </div>
          </div>
          <div class="grid-2">
            <div>
              <div class="dialog-label">SSH 用户</div>
              <t-input v-model="installParams.sshUser" />
            </div>
            <div>
              <div class="dialog-label">SSH 密码</div>
              <t-input v-model="installParams.sshPassword" type="password" autocomplete="new-password" />
            </div>
          </div>
          <div class="dialog-tip">SSH 凭据仅用于本次安装，不会保存在数据库。</div>
          <t-divider />
          <div class="dialog-title">2. 安装结果</div>
          <div v-if="sshInstallResult" class="ssh-result-card">
            <div class="ssh-result-head">
              <span class="ssh-result-title">{{ sshInstallResult.message }}</span>
              <t-tag
                :theme="sshInstallResult.status === 'installed' ? 'success' : sshInstallResult.status === 'installed_waiting_register' ? 'warning' : 'danger'"
                variant="light"
              >
                {{
                  sshInstallResult.status === "installed"
                    ? "已安装"
                    : sshInstallResult.status === "installed_waiting_register"
                      ? "等待注册"
                      : "失败"
                }}
              </t-tag>
            </div>
            <div class="ssh-result-meta">
              <span>Master Host：{{ sshInstallResult.master_host }}</span>
              <span>令牌过期：{{ formatTime(sshInstallResult.expires_at) }}</span>
              <span>远端主机名：{{ sshInstallResult.remote_hostname || "-" }}</span>
            </div>
            <div v-if="sshInstallResult.node" class="ssh-node-meta">
              <span>节点 ID：{{ sshInstallResult.node.id }}</span>
              <span>主机名：{{ sshInstallResult.node.hostname }}</span>
              <span>公网 IP：{{ sshInstallResult.node.public_ip || "-" }}</span>
              <span>状态：{{ sshInstallResult.node.status || "-" }}</span>
            </div>
          </div>
          <div v-else class="dialog-tip">点击"通过 SSH 安装"按钮开始执行远程安装。</div>
          <div>
            <div class="dialog-label">执行日志</div>
            <pre class="ssh-log-box">{{ sshInstallResult?.logs?.length ? sshInstallResult.logs.join('\n') : '暂无日志' }}</pre>
          </div>
          <t-button variant="outline" :loading="loading" @click="() => loadList({ page: 1 })">刷新列表</t-button>
        </div>
      </div>
    </t-dialog>

    <t-dialog v-model:visible="tokenDialogOpen" header="节点令牌">
      <template #footer>
        <t-button variant="outline" :disabled="!tokenValue" @click="copyToken">复制令牌</t-button>
        <t-button @click="tokenDialogOpen = false">关闭</t-button>
      </template>
      <div class="dialog-grid">
        <div class="dialog-tip">令牌仅在此处显示一次，请妥善保存。</div>
        <div class="token-box">{{ tokenValue || '-' }}</div>
      </div>
    </t-dialog>

    <t-dialog v-model:visible="upgradeDialogOpen" :header="upgradeNodeIDs.length ? '升级所选节点' : '升级所有节点'">
      <template #footer>
        <t-button variant="outline" @click="upgradeDialogOpen = false">取消</t-button>
        <t-button :loading="upgradeLoading" @click="submitUpgrade">提交升级</t-button>
      </template>
      <div class="dialog-grid">
        <div class="dialog-tip">升级过程中节点可能会短暂离线，请谨慎操作。</div>
        <div class="grid-2">
          <div>
            <div class="dialog-label">目标版本</div>
            <t-input v-model="upgradeTargetVersion" />
          </div>
          <div>
            <div class="dialog-label">强制升级</div>
            <t-select v-model="upgradeForceValue" :options="[{ label: '否', value: 'no' }, { label: '是', value: 'yes' }]" />
          </div>
        </div>
      </div>
    </t-dialog>

    <!-- ========== 节点详情抽屉 ========== -->
    <t-drawer
      v-model:visible="detailDrawerVisible"
      size="640px"
      :footer="false"
    >
      <template #header>
        <div class="drawer-header-with-back">
          <t-button class="drawer-back-btn" variant="text" shape="square" @click="detailDrawerVisible = false">
            <template #icon><t-icon name="chevron-left" /></template>
          </t-button>
          <span>节点详情 · {{ detailNode?.hostname || '' }}</span>
        </div>
      </template>
      <div v-if="detailNode" class="detail-body">
        <!-- CPU / 内存 / 磁盘 -->
        <div class="detail-section">
          <div class="detail-section-title">资源使用</div>
          <div class="ring-row">
            <div class="ring-card">
              <svg class="ring-svg" viewBox="0 0 120 120">
                <circle cx="60" cy="60" r="52" fill="none" stroke="#e2e8f0" stroke-width="10" />
                <circle cx="60" cy="60" r="52" fill="none"
                  :stroke="ringColor(detailNode.cpu_usage)" stroke-width="10"
                  stroke-linecap="round"
                  :stroke-dasharray="ringDash(detailNode.cpu_usage)"
                  transform="rotate(-90 60 60)" />
              </svg>
              <div class="ring-label">
                <span class="ring-value">{{ Math.round(detailNode.cpu_usage ?? 0) }}%</span>
                <span class="ring-title">CPU</span>
              </div>
            </div>
            <div class="ring-card">
              <svg class="ring-svg" viewBox="0 0 120 120">
                <circle cx="60" cy="60" r="52" fill="none" stroke="#e2e8f0" stroke-width="10" />
                <circle cx="60" cy="60" r="52" fill="none"
                  :stroke="ringColor(detailNode.mem_usage)" stroke-width="10"
                  stroke-linecap="round"
                  :stroke-dasharray="ringDash(detailNode.mem_usage)"
                  transform="rotate(-90 60 60)" />
              </svg>
              <div class="ring-label">
                <span class="ring-value">{{ Math.round(detailNode.mem_usage ?? 0) }}%</span>
                <span class="ring-title">内存</span>
              </div>
            </div>
            <div class="ring-card">
              <svg class="ring-svg" viewBox="0 0 120 120">
                <circle cx="60" cy="60" r="52" fill="none" stroke="#e2e8f0" stroke-width="10" />
                <circle cx="60" cy="60" r="52" fill="none"
                  :stroke="ringColor(detailNode.disk_usage)" stroke-width="10"
                  stroke-linecap="round"
                  :stroke-dasharray="ringDash(detailNode.disk_usage)"
                  transform="rotate(-90 60 60)" />
              </svg>
              <div class="ring-label">
                <span class="ring-value">{{ Math.round(detailNode.disk_usage ?? 0) }}%</span>
                <span class="ring-title">磁盘</span>
              </div>
            </div>
          </div>
          <div class="ring-meta">
            <span>CPU {{ detailNode.cpu_count ?? '-' }} 核</span>
            <span>内存 {{ formatBytes(detailNode.mem_total) }}</span>
            <span>磁盘 {{ formatBytes(detailNode.disk_total) }}</span>
          </div>
        </div>

        <div class="detail-section">
          <div class="detail-section-title">基础信息</div>
          <div class="detail-grid">
            <div class="detail-item"><span class="detail-label">节点 ID</span><span class="detail-value mono">{{ detailNode.id }}</span></div>
            <div class="detail-item"><span class="detail-label">主机名</span><span class="detail-value">{{ detailNode.hostname || '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">公网 IP</span><span class="detail-value mono">{{ detailNode.public_ip || '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">状态</span><span class="detail-value"><t-tag :theme="statusTagInfo(detailNode).theme as any" variant="light" size="small">{{ statusTagInfo(detailNode).label }}</t-tag></span></div>
            <div class="detail-item"><span class="detail-label">节点版本</span><span class="detail-value mono">{{ detailNode.version || '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">配置版本</span><span class="detail-value mono">{{ detailNode.config_version || '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">Capabilities</span><span class="detail-value mono">{{ (detailNode.capabilities || []).join(', ') || '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">最近心跳</span><span class="detail-value">{{ formatTime(detailNode.last_heartbeat) }}</span></div>
            <div class="detail-item"><span class="detail-label">通信状态</span><span class="detail-value"><t-tag :theme="detailNode.comm_ok ? 'success' : 'danger'" variant="light" size="small">{{ detailNode.comm_ok ? '正常' : '异常' }}</t-tag></span></div>
            <div class="detail-item"><span class="detail-label">上报状态</span><span class="detail-value"><t-tag :theme="detailNode.report_ok ? 'success' : 'danger'" variant="light" size="small">{{ detailNode.report_ok ? '正常' : '异常' }}</t-tag></span></div>
            <div class="detail-item"><span class="detail-label">创建时间</span><span class="detail-value">{{ formatTime(detailNode.created_at) }}</span></div>
            <div class="detail-item"><span class="detail-label">更新时间</span><span class="detail-value">{{ formatTime(detailNode.updated_at) }}</span></div>
          </div>
        </div>

        <div class="detail-section">
          <div class="detail-section-title">流量与连接</div>
          <div class="detail-grid">
            <div class="detail-item"><span class="detail-label">上行带宽</span><span class="detail-value mono">{{ formatBitsPerSec(detailNode.bandwidth_up_bps) }}</span></div>
            <div class="detail-item"><span class="detail-label">下行带宽</span><span class="detail-value mono">{{ formatBitsPerSec(detailNode.bandwidth_down_bps) }}</span></div>
            <div class="detail-item"><span class="detail-label">累计发送</span><span class="detail-value mono">{{ formatBytes(detailNode.bytes_sent) }}</span></div>
            <div class="detail-item"><span class="detail-label">累计接收</span><span class="detail-value mono">{{ formatBytes(detailNode.bytes_received) }}</span></div>
            <div class="detail-item"><span class="detail-label">本月发送</span><span class="detail-value mono">{{ formatBytes(detailNode.month_bytes_sent) }}</span></div>
            <div class="detail-item"><span class="detail-label">TCP ESTABLISHED</span><span class="detail-value mono">{{ detailNode.tcp_established ?? '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">TCP SYN_RECV</span><span class="detail-value mono">{{ detailNode.tcp_syn_recv ?? '-' }}</span></div>
            <div class="detail-item"><span class="detail-label">TCP TIME_WAIT</span><span class="detail-value mono">{{ detailNode.tcp_time_wait ?? '-' }}</span></div>
          </div>
        </div>

        <div class="detail-section">
          <div class="detail-section-title">监控状态</div>
          <div class="detail-grid">
            <div class="detail-item"><span class="detail-label">监控开关</span><span class="detail-value"><t-tag :theme="detailNode.monitor_enabled ? 'success' : 'default'" variant="light" size="small">{{ detailNode.monitor_enabled ? '已开启' : '未开启' }}</t-tag></span></div>
            <div class="detail-item"><span class="detail-label">协议</span><span class="detail-value">{{ (detailNode.monitor_protocol || 'http').toUpperCase() }}</span></div>
            <div class="detail-item"><span class="detail-label">超时</span><span class="detail-value">{{ detailNode.monitor_timeout_seconds ?? 5 }}s</span></div>
            <div class="detail-item"><span class="detail-label">端口</span><span class="detail-value">{{ detailNode.monitor_port ?? 80 }}</span></div>
            <div class="detail-item"><span class="detail-label">失败阈值</span><span class="detail-value">{{ detailNode.monitor_fail_threshold ?? 3 }}</span></div>
            <div class="detail-item"><span class="detail-label">连续失败</span><span class="detail-value mono">{{ detailNode.monitor_fail_count ?? 0 }}</span></div>
            <div class="detail-item"><span class="detail-label">最近探测</span><span class="detail-value">{{ formatTime(detailNode.monitor_last_at) }}</span></div>
            <div class="detail-item"><span class="detail-label">最近结果</span><span class="detail-value"><t-tag v-if="detailNode.monitor_enabled" :theme="detailNode.monitor_last_ok ? 'success' : 'danger'" variant="light" size="small">{{ detailNode.monitor_last_ok ? '正常' : '异常' }}</t-tag><span v-else>-</span></span></div>
            <div class="detail-item"><span class="detail-label">延迟</span><span class="detail-value mono">{{ detailNode.monitor_last_latency_ms != null ? detailNode.monitor_last_latency_ms + 'ms' : '-' }}</span></div>
            <div v-if="detailNode.monitor_last_error" class="detail-item detail-item-full"><span class="detail-label">错误信息</span><span class="detail-value detail-error">{{ detailNode.monitor_last_error }}</span></div>
          </div>
        </div>
      </div>
    </t-drawer>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, onBeforeUnmount, ref, watch } from "vue"
import { MessagePlugin, Tag, Button, Dropdown, DropdownMenu, DropdownItem, DialogPlugin } from "tdesign-vue-next"
import { api, type Node, type NodeInstallSSHResult, type UpgradeNode } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { useNodeListSSE } from "@/composables/useNodeListSSE"

const tab = ref<"list">("list")
const loading = ref(false)
const nodes = ref<Node[]>([])
const page = ref(1)
const pageSize = ref(20)
const total = ref(0)

// SSE 实时指标
const { metricsMap, connected: sseConnected, connect: sseConnect, disconnect: sseDisconnect } = useNodeListSSE()
const search = ref("")
const statusFilter = ref<"all" | "disabled" | "online" | "offline" | "stopped" | "pending" | "unknown">("all")

const detailDrawerVisible = ref(false)
const detailNode = ref<Node | null>(null)

const openDetail = (node: Node) => {
  detailNode.value = node
  detailDrawerVisible.value = true
}

const upgradeNodeMap = ref<Record<string, UpgradeNode>>({})

const monitorDialogOpen = ref(false)
const monitorSaving = ref(false)
const monitorTarget = ref<Node | null>(null)
const monitorForm = ref({
  enabled: false,
  protocol: "http",
  timeoutSeconds: 5,
  port: 80,
  failThreshold: 3,
})

const installDialogOpen = ref(false)
const installLoading = ref(false)
const installMode = ref<"manual" | "ssh">("manual")
const FIXED_PORTAL_BASE = "https://auh.lingcdn.cloud"
const installParams = ref({
  masterHost: "",
  ttlMinutes: 60,
  sshHost: "",
  sshPort: 22,
  sshUser: "root",
  sshPassword: "",
})
const installCommand = ref("")
const sshInstallResult = ref<NodeInstallSSHResult | null>(null)

const monitorPortText = computed<string>({
  get: () => (monitorForm.value.port == null ? "" : String(monitorForm.value.port)),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    if (!Number.isFinite(n)) {
      monitorForm.value.port = 80
      return
    }
    monitorForm.value.port = Math.max(1, Math.min(65535, Math.round(n)))
  },
})

const sshPortText = computed<string>({
  get: () => (installParams.value.sshPort == null ? "" : String(installParams.value.sshPort)),
  set: (v: string) => {
    const n = Number(String(v || "").trim())
    if (!Number.isFinite(n)) {
      installParams.value.sshPort = 22
      return
    }
    installParams.value.sshPort = Math.max(1, Math.min(65535, Math.round(n)))
  },
})

const tokenDialogOpen = ref(false)
const tokenValue = ref("")

const upgradeDialogOpen = ref(false)
const upgradeLoading = ref(false)
const upgradeNodeIDs = ref<string[]>([])
const upgradeTargetVersion = ref("latest")
const upgradeForceValue = ref("no")

const selectedRowKeys = ref<string[]>([])
const handleSelectionChange = (value: string[], _context: any) => {
  selectedRowKeys.value = value
}

const statusOptions = [
  { label: "全部状态", value: "all" },
  { label: "在线", value: "online" },
  { label: "离线", value: "offline" },
  { label: "已停止", value: "stopped" },
  { label: "待激活", value: "pending" },
  { label: "已禁用", value: "disabled" },
  { label: "未知", value: "unknown" },
]

// statusTagInfo derives the status tag shown in the node list/detail.
// Priority chain (highest first):
//   1. disabled  — admin shut the node off; nothing else matters
//   2. pending   — node has a row but never registered yet
//   3. comm_ok===false  — server saw heartbeat go stale (>90s)
//   4. report_ok===false — telemetry push went stale (>120s)
//   5. fall back to node.status (online/offline/stopped/...)
// We strictly compare to `false` so legacy payloads that omit the
// field don't get falsely flagged — only an explicit server-side
// `false` flips the tag to abnormal.
const statusTagInfo = (node?: Pick<Node, "status" | "comm_ok" | "report_ok"> | null) => {
  const s = (node?.status || "").trim().toLowerCase()
  if (s === "disabled") return { theme: "danger", label: "已禁用" }
  if (s === "pending") return { theme: "default", label: "待激活" }
  if (node?.comm_ok === false) return { theme: "danger", label: "通信异常" }
  if (node?.report_ok === false) return { theme: "warning", label: "上报异常" }
  if (!s) return { theme: "default", label: "未知" }
  if (s === "online" || s === "running") return { theme: "success", label: "在线" }
  if (s === "offline") return { theme: "warning", label: "离线" }
  if (s === "stopped") return { theme: "warning", label: "已停止" }
  return { theme: "default", label: node?.status || "未知" }
}

const formatPercent = (v?: number) => {
  if (v === undefined || v === null || !Number.isFinite(v)) return "-"
  return `${Math.max(0, Math.min(100, v)).toFixed(1)}%`
}

const formatBytes = (bytes?: number) => {
  if (bytes === undefined || bytes === null || !Number.isFinite(bytes)) return "-"
  const n = Math.max(0, bytes)
  const units = ["B", "KB", "MB", "GB", "TB", "PB"]
  let i = 0
  let v = n
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024
    i += 1
  }
  if (i === 0) return `${Math.round(v)} ${units[i]}`
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`
}

const formatBytesPerSec = (bps?: number) => {
  if (bps === undefined || bps === null || !Number.isFinite(bps)) return "-"
  return `${formatBytes(bps)}/s`
}

const formatBitsPerSec = (bytesPerSec?: number) => {
  if (bytesPerSec === undefined || bytesPerSec === null || !Number.isFinite(bytesPerSec)) return "-"
  const bits = Math.max(0, bytesPerSec) * 8
  const units = ["bps", "Kbps", "Mbps", "Gbps", "Tbps"]
  let i = 0
  let v = bits
  while (v >= 1000 && i < units.length - 1) {
    v /= 1000
    i += 1
  }
  if (i === 0) return `${Math.round(v)} ${units[i]}`
  return `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} ${units[i]}`
}

const RING_CIRCUMFERENCE = 2 * Math.PI * 52 // ~326.73

const ringColor = (value?: number) => {
  const v = value ?? 0
  if (v >= 90) return "#ef4444"
  if (v >= 70) return "#ff7d00"
  return "#00b42a"
}

const ringDash = (value?: number) => {
  const v = Math.max(0, Math.min(100, value ?? 0))
  const filled = (v / 100) * RING_CIRCUMFERENCE
  const gap = RING_CIRCUMFERENCE - filled
  return `${filled} ${gap}`
}

const formatTime = (v?: string) => {
  if (!v) return "-"
  const d = new Date(v)
  if (Number.isNaN(d.getTime())) return v
  return d.toLocaleString("zh-CN")
}

const openUpgradeDialog = (nodeIDs?: string[]) => {
  upgradeNodeIDs.value = nodeIDs || []
  upgradeTargetVersion.value = "latest"
  upgradeForceValue.value = "no"
  upgradeDialogOpen.value = true
}

const loadList = async (opts?: { page?: number; pageSize?: number }) => {
  try {
    loading.value = true
    const nextPage = opts?.page ?? page.value
    const nextPageSize = opts?.pageSize ?? pageSize.value
    const q = search.value.trim()
    const status = statusFilter.value === "all" || statusFilter.value === "unknown" ? "" : statusFilter.value
    const [res, upgrade] = await Promise.all([
      api.listNodesOverview({
        page: nextPage,
        pageSize: nextPageSize,
        q: q || undefined,
        status: status || undefined,
      }),
      api.getUpgradeInfo(),
    ])
    nodes.value = res.nodes || []
    page.value = res.page || nextPage
    pageSize.value = res.page_size || nextPageSize
    total.value = res.total || 0
    const m: Record<string, UpgradeNode> = {}
    for (const n of upgrade?.nodes || []) {
      if (n?.id) m[n.id] = n
    }
    upgradeNodeMap.value = m
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载节点列表失败")
  } finally {
    loading.value = false
  }
}

const submitUpgrade = async () => {
  if (upgradeLoading.value) return
  try {
    upgradeLoading.value = true
    const res = await api.upgradeNodes({
      node_ids: upgradeNodeIDs.value.length > 0 ? upgradeNodeIDs.value : undefined,
      target_version: String(upgradeTargetVersion.value || "").trim() || "latest",
      force: upgradeForceValue.value === "yes",
    })
    if (res.ok === false) {
      // Backend rejected — e.g. all nodes have communication issues
      const skipped = (res as any).skipped as { id: string; hostname?: string; reason: string }[] | undefined
      if (skipped && skipped.length > 0) {
        const names = skipped.map((s: any) => s.hostname || s.id).join(", ")
        MessagePlugin.warning(`以下节点跳过升级：${names}`)
      } else {
        MessagePlugin.warning(res.message || "升级请求被拒绝")
      }
    } else {
      const id = String(res.task_id || "").trim()
      const skipped = (res as any).skipped as { id: string; hostname?: string; reason: string }[] | undefined
      if (skipped && skipped.length > 0) {
        const names = skipped.map((s: any) => s.hostname || s.id).join(", ")
        MessagePlugin.warning(`以下节点跳过升级：${names}`)
      }
      MessagePlugin.success(id ? `已提交升级任务：${id.slice(0, 8)}` : res.message || "已提交升级任务")
    }
    upgradeDialogOpen.value = false
    selectedRowKeys.value = []
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "提交升级任务失败")
  } finally {
    upgradeLoading.value = false
  }
}

const openMonitorDialog = (node: Node) => {
  const proto = String(node.monitor_protocol || "http").toLowerCase() || "http"
  monitorTarget.value = node
  monitorForm.value = {
    enabled: Boolean(node.monitor_enabled),
    protocol: proto,
    timeoutSeconds: Number(node.monitor_timeout_seconds || 5),
    port: Number(node.monitor_port || 80),
    failThreshold: Number(node.monitor_fail_threshold || 3),
  }
  monitorDialogOpen.value = true
}

const handleSaveMonitorConfig = async () => {
  if (!monitorTarget.value || monitorSaving.value) return
  try {
    monitorSaving.value = true
    await api.updateNodeMonitorConfig(monitorTarget.value.id, {
      enabled: monitorForm.value.enabled,
      protocol: monitorForm.value.protocol,
      timeout_seconds: monitorForm.value.timeoutSeconds,
      port: monitorForm.value.port,
      fail_threshold: monitorForm.value.failThreshold,
    })
    MessagePlugin.success("监控配置已保存")
    monitorDialogOpen.value = false
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存监控配置失败")
  } finally {
    monitorSaving.value = false
  }
}

const handleToggleDisable = async (node: Node) => {
  const s = (node.status || "").trim().toLowerCase()
  const target = s === "disabled" ? "online" : "disabled"
  try {
    await api.updateNode(node.id, { status: target })
    MessagePlugin.success(target === "disabled" ? "节点已禁用" : "节点已启用")
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "切换节点状态失败")
  }
}

const handleResetToken = async (node: Node) => {
  try {
    const res = await api.updateNode(node.id, { token: "reset" })
    const token = res.token || ""
    if (!token) {
      MessagePlugin.warning("未返回新的令牌")
      return
    }
    tokenValue.value = token
    tokenDialogOpen.value = true
    MessagePlugin.success("令牌已重置")
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "重置令牌失败")
  }
}

const handleDeleteNode = async (node: Node) => {
  try {
    await api.deleteNode(node.id)
    MessagePlugin.success("节点已删除")
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除节点失败")
  }
}

// ========== 危险操作二次确认 ==========
const handleToggleDisableConfirm = (node: Node) => {
  const togglingToDisabled = (node.status || "").trim().toLowerCase() !== "disabled"
  const dlg = DialogPlugin.confirm({
    header: togglingToDisabled ? "确认禁用节点" : "确认启用节点",
    body: togglingToDisabled
      ? `确定禁用节点 "${node.hostname || node.id}"？禁用后将不再接收流量。`
      : `确定启用节点 "${node.hostname || node.id}"？启用后将恢复接收流量。`,
    confirmBtn: { content: togglingToDisabled ? "禁用" : "启用", theme: togglingToDisabled ? "danger" : "primary" },
    cancelBtn: "取消",
    onConfirm: async () => { dlg.destroy(); await handleToggleDisable(node) },
    onClose: () => { dlg.destroy() },
  })
}

const handleResetTokenConfirm = (node: Node) => {
  const dlg = DialogPlugin.confirm({
    header: "确认重置令牌",
    body: `确定重置节点 "${node.hostname || node.id}" 的令牌？旧令牌将立即失效，需要在节点上重新写入。`,
    confirmBtn: "重置",
    cancelBtn: "取消",
    onConfirm: async () => { dlg.destroy(); await handleResetToken(node) },
    onClose: () => { dlg.destroy() },
  })
}

const handleDeleteNodeConfirm = (node: Node) => {
  const dlg = DialogPlugin.confirm({
    header: "确认删除节点",
    body: `确定删除节点 "${node.hostname || node.id}"？此操作无法撤销，节点上的所有配置将被清除。`,
    confirmBtn: { content: "删除", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => { dlg.destroy(); await handleDeleteNode(node) },
    onClose: () => { dlg.destroy() },
  })
}

// ========== 批量操作 ==========
const handleBatchDisable = () => {
  const ids = selectedRowKeys.value
  if (!ids.length) return
  const dlg = DialogPlugin.confirm({
    header: "批量禁用节点",
    body: `确认禁用所选 ${ids.length} 个节点？`,
    confirmBtn: { content: "批量禁用", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      dlg.destroy()
      try {
        await Promise.all(ids.map((id) => api.updateNode(id, { status: "disabled" })))
        MessagePlugin.success(`已禁用 ${ids.length} 个节点`)
        selectedRowKeys.value = []
        await loadList()
      } catch (err: any) {
        MessagePlugin.error(err.message || "批量禁用失败")
      }
    },
    onClose: () => { dlg.destroy() },
  })
}

const handleBatchDelete = () => {
  const ids = selectedRowKeys.value
  if (!ids.length) return
  const dlg = DialogPlugin.confirm({
    header: "批量删除节点",
    body: `确认删除所选 ${ids.length} 个节点？此操作不可撤销。`,
    confirmBtn: { content: "批量删除", theme: "danger" },
    cancelBtn: "取消",
    onConfirm: async () => {
      dlg.destroy()
      try {
        await Promise.all(ids.map((id) => api.deleteNode(id)))
        MessagePlugin.success(`已删除 ${ids.length} 个节点`)
        selectedRowKeys.value = []
        await loadList()
      } catch (err: any) {
        MessagePlugin.error(err.message || "批量删除失败")
      }
    },
    onClose: () => { dlg.destroy() },
  })
}

// ========== 移动端下拉菜单 ==========
const handleMobileDropdown = (data: any, node: Node) => {
  const val = typeof data === "object" ? data.value : data
  if (val === "toggle") handleToggleDisableConfirm(node)
  else if (val === "reset-token") handleResetTokenConfirm(node)
  else if (val === "monitor") openMonitorDialog(node)
  else if (val === "delete") handleDeleteNodeConfirm(node)
}

const handleOpenInstallDialog = () => {
  installMode.value = "manual"
  installCommand.value = ""
  sshInstallResult.value = null
  installParams.value.sshPassword = ""
  installDialogOpen.value = true
  handleFetchInstallCommand()
}

const handleFetchInstallCommand = async () => {
  if (installLoading.value) return
  try {
    installLoading.value = true
    const res = await api.getNodeInstallCommand({
      portalBase: FIXED_PORTAL_BASE,
      masterHost: installParams.value.masterHost.trim() || undefined,
      ttlMinutes: Number(installParams.value.ttlMinutes) > 0 ? Number(installParams.value.ttlMinutes) : undefined,
    })
    installCommand.value = res.command || ""
    if (!res.command) {
      MessagePlugin.warning("未生成安装命令，请检查 master_host 配置")
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "获取安装命令失败")
  } finally {
    installLoading.value = false
  }
}

const handleInstallViaSSH = async () => {
  if (installLoading.value) return
  const sshHost = installParams.value.sshHost.trim()
  const sshUser = installParams.value.sshUser.trim() || "root"
  const sshPassword = installParams.value.sshPassword
  if (!sshHost) {
    MessagePlugin.warning("请填写主机 IP 或域名")
    return
  }
  if (!sshPassword) {
    MessagePlugin.warning("请填写 SSH 密码")
    return
  }
  try {
    installLoading.value = true
    sshInstallResult.value = await api.installNodeViaSSH({
      ssh_host: sshHost,
      ssh_port: Number(installParams.value.sshPort) > 0 ? Number(installParams.value.sshPort) : 22,
      ssh_user: sshUser,
      ssh_password: sshPassword,
      master_host: installParams.value.masterHost.trim() || undefined,
      ttl_minutes: Number(installParams.value.ttlMinutes) > 0 ? Number(installParams.value.ttlMinutes) : undefined,
      portal_base: FIXED_PORTAL_BASE,
    })
    if (sshInstallResult.value.ok) {
      MessagePlugin.success(
        sshInstallResult.value.status === "installed" ? "节点安装成功" : "节点已安装，等待注册"
      )
      await loadList({ page: 1 })
    } else {
      MessagePlugin.error(sshInstallResult.value.message || "节点安装失败")
    }
  } catch (err: any) {
    sshInstallResult.value = null
    MessagePlugin.error(err.message || "通过 SSH 安装失败")
  } finally {
    installLoading.value = false
  }
}

const copyCommand = async () => {
  if (!installCommand.value) return
  const ok = await copyText(installCommand.value)
  if (ok) {
    MessagePlugin.success("安装命令已复制到剪贴板")
  } else {
    MessagePlugin.error("复制失败，请手动复制")
  }
}

const copyToken = async () => {
  if (!tokenValue.value) return
  const ok = await copyText(tokenValue.value)
  if (ok) {
    MessagePlugin.success("令牌已复制到剪贴板")
  } else {
    MessagePlugin.error("复制失败，请手动复制")
  }
}

const listColumns = computed(() => [
  {
    colKey: "row-select",
    type: "multiple",
    width: 46,
  },
  {
    colKey: "hostname",
    title: "主机名 / IP",
    minWidth: 240,
    cell: (_h: any, { row }: { row: Node }) =>
      h("div", { style: "display:flex;flex-direction:column" }, [
        h("span", { style: "font-weight:600" }, row.hostname || row.id),
        h("span", { style: "font-size:12px;color:#94a3b8" }, row.public_ip || "-"),
      ]),
  },
  {
    colKey: "status",
    title: "状态",
    width: 110,
    cell: (_h: any, { row }: { row: Node }) => {
      const info = statusTagInfo(row)
      return h(Tag, { theme: info.theme as any, variant: "light" }, () => info.label)
    },
  },
  {
    colKey: "version",
    title: "版本",
    width: 180,
    cell: (_h: any, { row }: { row: Node }) => {
      const upgrade = upgradeNodeMap.value[row.id]
      const ver = row.version || "-"
      const needsUpgrade = upgrade && upgrade.status === "upgrade_needed"
      if (!needsUpgrade) return h("span", { class: "mono" }, ver)
      return h("div", { style: "display:flex;flex-direction:column;gap:2px" }, [
        h("span", { class: "mono" }, ver),
        h("div", { style: "display:flex;align-items:center;gap:4px" }, [
          h(Tag, { theme: "warning", variant: "light", size: "small" }, () => `可升级到 ${upgrade.target_version}`),
        ]),
      ])
    },
  },
  {
    colKey: "monitor",
    title: "监控",
    minWidth: 220,
    cell: (_h: any, { row }: { row: Node }) => {
      const enabled = Boolean(row.monitor_enabled)
      const proto = String(row.monitor_protocol || "http").toLowerCase()
      const timeout = Number(row.monitor_timeout_seconds || 5)
      const port = Number(row.monitor_port || 80)
      const threshold = Number(row.monitor_fail_threshold || 3)
      const failCount = Number(row.monitor_fail_count || 0)
      const ok = enabled && failCount < threshold
      const label = !enabled ? "未启用" : ok ? "正常" : "异常"
      const theme = !enabled ? "default" : ok ? "success" : "danger"
      const protoLabel = proto === "ping" ? "ping" : `${proto}:${port || "-"}`
      const errMsg = String(row.monitor_last_error || "")
      const shortErr = errMsg.length > 60 ? `${errMsg.slice(0, 60)}...` : errMsg
      return h("div", { style: "display:flex;flex-direction:column;gap:4px" }, [
        h("div", { style: "display:flex;align-items:center;gap:8px" }, [
          h(Tag, { theme, variant: "light" }, () => label),
          h(
            Button,
            { size: "small", variant: "text", onClick: () => openMonitorDialog(row) },
            () => "配置"
          ),
        ]),
        h("span", { style: "font-size:12px;color:#94a3b8" }, `${protoLabel} · 超时 ${timeout}s · 阈值 ${threshold}`),
        enabled && row.monitor_last_at
          ? h(
              "span",
              { style: "font-size:12px;color:#94a3b8" },
              `${row.monitor_last_ok ? "✓" : "✗"} ${formatTime(row.monitor_last_at)} · ${Number(
                row.monitor_last_latency_ms || 0
              )}ms`
            )
          : null,
        enabled && shortErr ? h("span", { style: "font-size:12px;color:#ef4444" }, shortErr) : null,
      ])
    },
  },
  {
    colKey: "cpu",
    title: "CPU",
    width: 120,
    cell: (_h: any, { row }: { row: Node }) => h("span", { class: "mono" }, formatPercent(row.cpu_usage)),
  },
  {
    colKey: "mem",
    title: "内存",
    width: 120,
    cell: (_h: any, { row }: { row: Node }) => h("span", { class: "mono" }, formatPercent(row.mem_usage)),
  },
  {
    colKey: "disk",
    title: "磁盘",
    width: 120,
    cell: (_h: any, { row }: { row: Node }) => h("span", { class: "mono" }, formatPercent(row.disk_usage)),
  },
  {
    colKey: "bandwidth",
    title: "带宽",
    minWidth: 180,
    cell: (_h: any, { row }: { row: Node }) =>
      h("div", { style: "display:flex;flex-direction:column" }, [
        h("span", { class: "mono" }, `↑ ${formatBitsPerSec(row.bandwidth_up_bps)}`),
        h("span", { class: "mono muted" }, `↓ ${formatBitsPerSec(row.bandwidth_down_bps)}`),
      ]),
  },
  {
    colKey: "tcp",
    title: "TCP 连接",
    minWidth: 160,
    cell: (_h: any, { row }: { row: Node }) =>
      h("span", { class: "mono" },
        `EST ${row.tcp_established ?? 0} / SYN ${row.tcp_syn_recv ?? 0} / TW ${row.tcp_time_wait ?? 0}`
      ),
  },
  {
    colKey: "actions",
    title: "操作",
    minWidth: 200,
    cell: (_h: any, { row }: { row: Node }) => {
      const upgrade = upgradeNodeMap.value[row.id]
      const isLatest = upgrade ? upgrade.current_version === upgrade.target_version : true
      const togglingToDisabled = (row.status || "").trim().toLowerCase() !== "disabled"
      const dropdownItems = [
        { content: togglingToDisabled ? "禁用节点" : "启用节点", value: "toggle", theme: togglingToDisabled ? "error" : "default" },
        { content: "重置令牌", value: "reset-token" },
        { content: "监控配置", value: "monitor" },
        { divider: true },
        { content: "删除节点", value: "delete", theme: "error" },
      ]
      const handleDropdownClick = (data: any) => {
        const val = typeof data === "object" ? data.value : data
        if (val === "toggle") handleToggleDisableConfirm(row)
        else if (val === "reset-token") handleResetTokenConfirm(row)
        else if (val === "monitor") openMonitorDialog(row)
        else if (val === "delete") handleDeleteNodeConfirm(row)
      }
      return h("div", { style: "display:flex;gap:6px;flex-wrap:wrap;align-items:center" }, [
        h(
          Button,
          { size: "small", variant: "text", theme: "primary", onClick: () => openDetail(row) },
          () => "详情"
        ),
        !isLatest
          ? h(
              Button,
              { size: "small", variant: "outline", onClick: () => openUpgradeDialog([row.id]) },
              () => "升级"
            )
          : null,
        h(
          Dropdown,
          { trigger: "click", onClick: handleDropdownClick },
          {
            default: () => h(Button, { size: "small", variant: "text" }, () => "更多 ▾"),
            dropdown: () =>
              h(DropdownMenu, {}, () =>
                dropdownItems.map((item: any) =>
                  item.divider
                    ? h(DropdownItem, { divider: true })
                    : h(
                        DropdownItem,
                        { value: item.value, class: item.theme === "error" ? "dropdown-danger" : "" },
                        () => item.content
                      )
                )
              ),
          }
        ),
      ])
    },
  },
])

const listPagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: total.value,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
}))

const handleListPageChange = (pagination: { current: number; pageSize: number }) => {
  page.value = pagination.current
  pageSize.value = pagination.pageSize
  loadList({ page: pagination.current, pageSize: pagination.pageSize })
}

let listTimer: ReturnType<typeof setTimeout> | null = null

watch(tab, (next) => {
  if (next === "list") loadList({ page: 1 })
})

watch([search, statusFilter, tab], () => {
  if (tab.value !== "list") return
  if (listTimer) clearTimeout(listTimer)
  listTimer = setTimeout(() => loadList({ page: 1 }), 300)
})

watch(installMode, () => {
  installCommand.value = ""
  sshInstallResult.value = null
})

onMounted(() => {
  if (tab.value === "list") loadList({ page: 1 })
  sseConnect()
  startAutoRefresh()
})

/* ---- SSE 实时指标合并 ---- */
watch(metricsMap, (m) => {
  if (!m || Object.keys(m).length === 0) return
  // 把 SSE 的实时指标合并到 nodes 列表
  for (const node of nodes.value) {
    const live = m[node.id]
    if (!live) continue
    node.cpu_usage = live.cpu_usage
    node.mem_usage = live.mem_usage
    node.disk_usage = live.disk_usage
    node.bandwidth_up_bps = live.out_bps
    node.bandwidth_down_bps = live.in_bps
    node.tcp_established = live.connections
  }
  // 详情抽屉也要同步
  if (detailDrawerVisible.value && detailNode.value) {
    const live = m[detailNode.value.id]
    if (live) {
      detailNode.value = {
        ...detailNode.value,
        cpu_usage: live.cpu_usage,
        mem_usage: live.mem_usage,
        disk_usage: live.disk_usage,
        bandwidth_up_bps: live.out_bps,
        bandwidth_down_bps: live.in_bps,
        tcp_established: live.connections,
      }
    }
  }
})

/* ---- 静默轮询作为 SSE 兜底 ---- */
const POLL_INTERVAL_SSE = 15_000     // SSE 在线：15 秒
const POLL_INTERVAL_FALLBACK = 5_000 // SSE 离线：5 秒
let autoRefreshTimer: ReturnType<typeof setInterval> | null = null

const silentRefreshList = async () => {
  if (loading.value) return
  try {
    const q = search.value.trim()
    const status = statusFilter.value === "all" || statusFilter.value === "unknown" ? "" : statusFilter.value
    const [res, upgrade] = await Promise.all([
      api.listNodesOverview({
        page: page.value,
        pageSize: pageSize.value,
        q: q || undefined,
        status: status || undefined,
      }),
      api.getUpgradeInfo(),
    ])
    nodes.value = res.nodes || []
    total.value = res.total || 0
    const m: Record<string, UpgradeNode> = {}
    for (const n of upgrade?.nodes || []) {
      if (n?.id) m[n.id] = n
    }
    upgradeNodeMap.value = m
    // 同步详情抽屉的快照
    if (detailDrawerVisible.value && detailNode.value) {
      const updated = (res.nodes || []).find((n: any) => n.id === detailNode.value!.id)
      if (updated) detailNode.value = updated as Node
    }
  } catch {
    // 静默忽略
  }
}

const startAutoRefresh = () => {
  stopAutoRefresh()
  const interval = sseConnected.value ? POLL_INTERVAL_SSE : POLL_INTERVAL_FALLBACK
  autoRefreshTimer = setInterval(() => {
    if (tab.value === "list") silentRefreshList()
  }, interval)
}

const stopAutoRefresh = () => {
  if (autoRefreshTimer) {
    clearInterval(autoRefreshTimer)
    autoRefreshTimer = null
  }
}

// SSE 连接状态变化时调整轮询频率
watch(sseConnected, () => {
  startAutoRefresh()
})

onBeforeUnmount(() => {
  stopAutoRefresh()
  sseDisconnect()
})
</script>

<style scoped>
.toolbar {
  display: flex;
  gap: 10px;
  align-items: center;
  flex-wrap: wrap;
  margin-bottom: 12px;
}

.batch-bar {
  display: flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
  padding: 10px 16px;
  margin-bottom: 12px;
  background: linear-gradient(135deg, rgba(99, 91, 255, 0.04) 0%, rgba(99, 91, 255, 0.08) 100%);
  border: 1px solid rgba(99, 91, 255, 0.15);
  border-radius: 10px;
}

.batch-bar-label {
  font-size: 13px;
  font-weight: 600;
  color: #3D35BC;
  margin-right: 4px;
}

.page {
  min-width: 0;
}

.section-card {
  min-width: 0;
}

.section-card :deep(.t-tabs) {
  min-width: 0;
}

.section-card :deep(.t-tabs__content) {
  min-width: 0;
}

.section-card :deep(.t-table) {
  min-width: 100%;
}

.w-260 {
  width: 260px;
}

.w-140 {
  width: 140px;
}

.dialog-grid {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.dialog-title {
  font-size: 18px;
  font-weight: 700;
}

.dialog-label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 500;
  margin-bottom: 6px;
}

.dialog-tip {
  font-size: 12px;
  color: #94a3b8;
}

.grid-2 {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 12px;
}

.row {
  display: flex;
  align-items: center;
  gap: 12px;
}

.row-label {
  width: 90px;
  font-size: 12px;
  color: #94a3b8;
  font-weight: 500;
}

.copy-row {
  margin-top: 10px;
  display: flex;
  justify-content: space-between;
  gap: 10px;
  flex-wrap: wrap;
}

.token-box {
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  padding: 12px 14px;
  border-radius: 10px;
  word-break: break-all;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  color: #334155;
}

.ssh-result-card {
  border: 1px solid #e2e8f0;
  border-radius: 10px;
  padding: 14px;
  background: #f8fafc;
  display: flex;
  flex-direction: column;
  gap: 10px;
  transition: border-color 0.2s ease;
}

.ssh-result-card:hover {
  border-color: #cbd5e1;
}

.ssh-result-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  flex-wrap: wrap;
}

.ssh-result-title {
  font-weight: 600;
}

.ssh-result-meta,
.ssh-node-meta {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 8px 12px;
  font-size: 12px;
  color: #475569;
}

.ssh-log-box {
  margin: 0;
  min-height: 180px;
  max-height: 320px;
  overflow: auto;
  padding: 12px;
  border-radius: 8px;
  background: #0f172a;
  color: #e5e7eb;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
}

.mono.muted {
  color: #94a3b8;
}

@media (max-width: 768px) {
  .toolbar > * {
    width: 100% !important;
    min-width: 0 !important;
  }

  .toolbar :deep(.t-button) {
    min-height: 44px;
  }

  .grid-2 {
    grid-template-columns: 1fr;
  }

  .ssh-result-meta,
  .ssh-node-meta {
    grid-template-columns: 1fr;
  }

  .row {
    flex-direction: column;
    align-items: flex-start;
  }

  .row-label {
    width: auto;
  }
}

/* 详情抽屉 */
.detail-body {
  display: flex;
  flex-direction: column;
  gap: 20px;
}

.detail-section {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.detail-section-title {
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
  padding-bottom: 8px;
  border-bottom: 1px solid #f1f5f9;
}

.detail-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 10px 16px;
}

.detail-item {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.detail-item-full {
  grid-column: 1 / -1;
}

.detail-label {
  font-size: 12px;
  color: #94a3b8;
  font-weight: 500;
}

.detail-value {
  font-size: 13px;
  color: #0f172a;
  font-weight: 500;
  word-break: break-all;
}

.detail-error {
  color: #ef4444;
  font-size: 12px;
  font-weight: 500;
}

/* 资源使用环 */
.ring-row {
  display: flex;
  gap: 24px;
  justify-content: center;
  flex-wrap: wrap;
}

.ring-card {
  position: relative;
  width: 120px;
  height: 120px;
  display: flex;
  align-items: center;
  justify-content: center;
}

.ring-svg {
  width: 120px;
  height: 120px;
}

.ring-label {
  position: absolute;
  inset: 0;
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  pointer-events: none;
}

.ring-value {
  font-size: 22px;
  font-weight: 700;
  color: #0f172a;
  line-height: 1.2;
}

.ring-title {
  font-size: 12px;
  color: #94a3b8;
  margin-top: 2px;
}

.ring-meta {
  display: flex;
  justify-content: center;
  gap: 20px;
  font-size: 12px;
  color: #94a3b8;
  flex-wrap: wrap;
  margin-top: 4px;
}

/* 抽屉头部带返回按钮（移动端） */
.drawer-header-with-back {
  display: flex;
  align-items: center;
  gap: 4px;
  font-size: 16px;
  font-weight: 600;
  width: 100%;
}

.drawer-back-btn {
  display: none;
}

@media (max-width: 768px) {
  .drawer-back-btn {
    display: inline-flex;
  }
}
</style>

<style>
/* 全局（非 scoped）：危险下拉项 */
.dropdown-danger {
  color: #ef4444 !important;
}
</style>
