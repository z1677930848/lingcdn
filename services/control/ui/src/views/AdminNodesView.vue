<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">节点管理</h1>
        <p class="subtitle">查看与管理所有 CDN 边缘节点 / 监控状态 / 升级控制</p>
      </div>
    </div>

    <ErrorState v-if="listError" :message="listError" @retry="() => loadList({ page: 1 })" />

    <el-card class="section-card">
      <el-tabs v-model="tab">
        <el-tab-pane name="list" label="节点列表">
          <div class="toolbar">
            <el-input v-model="search" clearable placeholder="搜索主机名 / IP" class="w-260" />
            <EpSelect v-model="statusFilter" class="w-140" :options="statusOptions" />
            <el-button plain @click="handleOpenInstallDialog">添加节点</el-button>
            <el-button :loading="loading" @click="() => loadList({ page: 1 })">刷新</el-button>
          </div>
          <div v-if="selectedRowKeys.length > 0" class="batch-bar">
            <span class="batch-bar-label">已选择 {{ selectedRowKeys.length }} 个节点</span>
            <el-button size="small" type="warning" @click="openUpgradeDialog(selectedRowKeys)">批量升级</el-button>
            <el-button size="small" type="primary" plain @click="handleBatchEnable">批量启用</el-button>
            <el-button size="small" type="danger" plain @click="handleBatchDisable">批量禁用</el-button>
            <el-button size="small" type="danger" @click="handleBatchDelete">批量删除</el-button>
            <el-button size="small" plain @click="selectedRowKeys = []">取消选择</el-button>
          </div>

          <div class="admin-desktop-only">
            <EpDataTable
              :data="nodes"
              :columns="listColumns"
              row-key="id"
              size="small"
              :loading="loading"
              :pagination="listPagination"
              hover
              stripe
              empty-text="暂无节点"
              :selected-row-keys="selectedRowKeys"
              @select-change="handleSelectionChange"
            />
          </div>

          <div class="admin-mobile-only">
            <LoadingState v-if="loading && nodes.length === 0" />
            <template v-else>
              <div v-if="nodes.length" class="admin-mobile-cards">
                <div v-for="node in nodes" :key="node.id" class="admin-mobile-card">
                  <div class="admin-mobile-card-header">
                    <div style="min-width:0">
                      <div class="admin-mobile-card-title">{{ node.hostname || node.id }}</div>
                      <div class="admin-mobile-card-subtitle">{{ node.public_ip || '-' }}</div>
                    </div>
                    <div class="admin-mobile-card-tags">
                      <el-tag :type="(statusTagInfo(node).theme as any)" effect="light" size="small">{{ statusTagInfo(node).label }}</el-tag>
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
                        <el-tag
                          v-if="upgradeNodeMap[node.id] && upgradeNodeMap[node.id].status === 'upgrade_needed'"
                          type="warning" effect="light" size="small" style="margin-left:6px"
                        >
                          可升级到 {{ upgradeNodeMap[node.id].target_version }}
                        </el-tag>
                      </span>
                    </div>
                    <div class="admin-mobile-card-row">
                      <span class="admin-mobile-card-label">心跳</span>
                      <span class="admin-mobile-card-value">{{ formatTime(node.last_heartbeat) }}</span>
                    </div>
                  </div>
                  <div class="admin-mobile-card-actions">
                    <el-button size="small" link type="primary" @click="openDetail(node)">详情</el-button>
                    <el-button
                      v-if="upgradeNodeMap[node.id] && upgradeNodeMap[node.id].status === 'upgrade_needed'"
                      size="small" plain @click="openUpgradeDialog([node.id])"
                    >升级</el-button>
                    <el-dropdown trigger="click" @click="handleMobileDropdown($event, node)">
                      <el-button size="small" link>更多 ▾</el-button>
                      <el-dropdown-menu>
                        <el-dropdown-item :value="'toggle'">
                          {{ (node.status || '').trim().toLowerCase() !== 'disabled' ? '禁用节点' : '启用节点' }}
                        </el-dropdown-item>
                        <el-dropdown-item :value="'monitor'">监控配置</el-dropdown-item>
                        <el-dropdown-item :value="'reset-token'">重置令牌</el-dropdown-item>
                        <el-dropdown-item divider />
                        <el-dropdown-item :value="'delete'" class="dropdown-danger">删除节点</el-dropdown-item>
                      </el-dropdown-menu>
                    </el-dropdown>
                  </div>
                </div>
                <el-pagination
                  class="admin-mobile-pagination"
                  :current-page="listPagination.current"
                  :page-size="listPagination.pageSize"
                  :total="listPagination.total"
                  layout="prev, pager, next"
                  size="small"
                  @current-change="(v: number) => handleListPageChange({ current: v, pageSize: listPagination.pageSize })"
                />
              </div>
              <div v-else class="admin-mobile-card-empty">暂无节点</div>
            </template>
          </div>
        </el-tab-pane>

      </el-tabs>
    </el-card>

    <EpDialog append-to-body
      v-model="monitorDialogOpen"
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
          <el-switch v-model="monitorForm.enabled" />
        </div>
        <div>
          <div class="dialog-label">探测协议</div>
          <el-radio-group v-model="monitorForm.protocol">
            <el-radio value="http">HTTP</el-radio>
            <el-radio value="ping">Ping</el-radio>
            <el-radio value="tcp">TCP</el-radio>
          </el-radio-group>
        </div>
        <div class="grid-2">
          <div>
            <div class="dialog-label">超时（秒）</div>
            <el-input-number v-model="monitorForm.timeoutSeconds" :min="1" :max="60" />
          </div>
          <div>
            <div class="dialog-label">端口</div>
            <!-- Plain numeric input; the stepper encouraged +1 clicks to
                 walk 80→443, which is the wrong affordance for port
                 fields. Browser validates min/max via attributes. -->
            <el-input v-model="monitorPortText" type="number" :disabled="monitorForm.protocol === 'ping'" placeholder="1-65535" />
          </div>
        </div>
        <div>
          <div class="dialog-label">连续失败阈值</div>
          <el-input-number v-model="monitorForm.failThreshold" :min="1" :max="10" />
        </div>
        <div class="dialog-tip">HTTP/TCP 协议会探测端口；Ping 协议忽略端口，仅做 ICMP 探测。</div>
      </div>
    </EpDialog>

    <EpDialog append-to-body v-model="installDialogOpen" header="添加节点" width="760px">
      <template #footer>
        <el-button plain @click="installDialogOpen = false">关闭</el-button>
        <el-button v-if="installMode === 'manual'" :loading="installLoading" @click="handleFetchInstallCommand()">
          重新生成命令
        </el-button>
        <el-button v-else :loading="installLoading" @click="handleInstallViaSSH()">
          通过 SSH 安装
        </el-button>
      </template>
      <div class="dialog-grid">
        <div v-if="offlineLocalBundles" class="dialog-grid offline-bundle-panel">
          <div class="dialog-title">离线节点包</div>
          <div class="dialog-tip">
            当前为离线授权模式，节点安装包由主控本地分发（无需授权站）。请先上传
            <code>lingcdn-node-x.y.z-linux-amd64.tar.gz</code>，再生成安装命令。
          </div>
          <div class="bundle-upload-row">
            <input ref="bundleFileInput" type="file" accept=".tar.gz" class="bundle-file-input" @change="handleBundleFileChange" />
            <el-button plain :loading="bundleUploading" @click="triggerBundleUpload">上传节点包</el-button>
            <el-button plain :loading="bundleLoading" @click="loadLocalBundles">刷新列表</el-button>
          </div>
          <div v-if="localBundles.length" class="bundle-list">
            <div v-for="item in localBundles" :key="item.filename" class="bundle-item">
              <span>{{ item.filename }}</span>
              <span class="bundle-meta">{{ formatBytes(item.size_bytes) }} · {{ item.checksum.slice(0, 12) }}…</span>
              <el-button size="small" link type="danger" @click="handleDeleteBundle(item.filename)">删除</el-button>
            </div>
          </div>
          <div v-else class="dialog-tip">尚未上传节点包。</div>
          <el-divider />
        </div>
        <div>
          <div class="dialog-label">安装方式</div>
          <el-radio-group v-model="installMode">
            <el-radio value="manual">手动复制命令</el-radio>
            <el-radio value="ssh">通过 SSH 自动安装</el-radio>
          </el-radio-group>
        </div>
        <div v-if="installMode === 'manual'" class="dialog-grid">
          <div class="dialog-title">1. 复制安装命令</div>
          <div class="dialog-tip">
            <template v-if="installCommandStyle === 'local'">在目标节点以 root 身份执行下列命令；安装包将从主控本地下载。</template>
            <template v-else>在目标节点以 root 身份执行下列命令完成安装。</template>
          </div>
          <div>
            <div class="dialog-label">安装命令</div>
            <el-input v-model="installCommand" :autosize="{ minRows: 5, maxRows: 10 }" />
            <div class="copy-row">

              <el-button plain :disabled="!installCommand" @click="copyCommand">复制命令</el-button>
            </div>
          </div>
          <el-divider />
          <div class="dialog-title">2. 等待节点上线</div>
          <div class="dialog-tip">节点首次上报通常 1-2 分钟，gRPC 通道建立后会出现在列表。</div>
          <el-button plain :loading="loading" @click="() => loadList({ page: 1 })">刷新列表</el-button>
        </div>

        <div v-else class="dialog-grid">
          <div class="dialog-title">1. SSH 凭据</div>
          <div class="dialog-tip">请填写 SSH 凭据，目标主机需为 Linux 且当前 SSH 用户具备 root 权限。</div>
          <div class="grid-2">
            <div>
              <div class="dialog-label">主机 IP / 域名</div>
              <el-input v-model="installParams.sshHost" />
            </div>
            <div>
              <div class="dialog-label">SSH 端口</div>
              <el-input v-model="sshPortText" type="number" placeholder="1-65535" />
            </div>
          </div>
          <div class="grid-2">
            <div>
              <div class="dialog-label">SSH 用户</div>
              <el-input v-model="installParams.sshUser" />
            </div>
            <div>
              <div class="dialog-label">SSH 密码</div>
              <el-input v-model="installParams.sshPassword" type="password" autocomplete="new-password" />
            </div>
          </div>
          <div class="dialog-tip">SSH 凭据仅用于本次安装，不会保存在数据库。</div>
          <el-divider />
          <div class="dialog-title">2. 安装结果</div>
          <div v-if="sshInstallResult" class="ssh-result-card">
            <div class="ssh-result-head">
              <span class="ssh-result-title">{{ sshInstallResult.message }}</span>
              <el-tag
                :type="sshInstallResult.status === 'installed' ? 'success' : sshInstallResult.status === 'installed_waiting_register' ? 'warning' : 'danger'"
                effect="light"
              >
                {{
                  sshInstallResult.status === "installed"
                    ? "已安装"
                    : sshInstallResult.status === "installed_waiting_register"
                      ? "等待注册"
                      : "失败"
                }}
              </el-tag>
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
          <el-button plain :loading="loading" @click="() => loadList({ page: 1 })">刷新列表</el-button>
        </div>
      </div>
    </EpDialog>

    <EpDialog append-to-body v-model="tokenDialogOpen" header="节点令牌">
      <template #footer>
        <el-button plain :disabled="!tokenValue" @click="copyToken">复制令牌</el-button>
        <el-button @click="tokenDialogOpen = false">关闭</el-button>
      </template>
      <div class="dialog-grid">
        <div class="dialog-tip">令牌仅在此处显示一次，请妥善保存。</div>
        <div class="token-box">{{ tokenValue || '-' }}</div>
      </div>
    </EpDialog>

    <EpDialog append-to-body v-model="upgradeDialogOpen" :title="upgradeNodeIDs.length ? '升级所选节点' : '升级所有节点'">
      <template #footer>
        <el-button plain @click="upgradeDialogOpen = false">取消</el-button>
        <el-button :loading="upgradeLoading" @click="submitUpgrade">提交升级</el-button>
      </template>
      <div class="dialog-grid">
        <div class="dialog-tip">升级过程中节点可能会短暂离线；已禁用或通信异常的节点会被跳过，不会下发升级命令。</div>
        <div class="grid-2">
          <div>
            <div class="dialog-label">目标版本</div>
            <el-input v-model="upgradeTargetVersion" />
          </div>
          <div>
            <div class="dialog-label">强制升级</div>
            <EpSelect v-model="upgradeForceValue" :options="[{ label: '否', value: 'no' }, { label: '是', value: 'yes' }]" />
          </div>
        </div>
      </div>
    </EpDialog>

    <!-- ========== 节点详情抽屉 ========== -->
    <el-drawer
      v-model="detailDrawerVisible"
      append-to-body
      destroy-on-close
      size="720px"
      class="node-detail-drawer"
    >
      <template #header>
        <div class="node-detail-header">
          <div class="node-detail-header-main">
            <div class="node-detail-title">{{ detailNode?.hostname || detailNode?.id || "节点详情" }}</div>
            <div v-if="detailNode" class="node-detail-subtitle mono">{{ detailNode.public_ip || "暂无公网 IP" }}</div>
          </div>
          <div v-if="detailNode" class="node-detail-header-tags">
            <el-tag :type="epTagType(statusTagInfo(detailNode).theme)" effect="light" size="small">
              {{ statusTagInfo(detailNode).label }}
            </el-tag>
            <el-tag v-if="detailNeedsUpgrade" type="warning" effect="light" size="small">
              可升级到 {{ detailUpgradeTarget }}
            </el-tag>
          </div>
        </div>
      </template>

      <div v-if="detailNode" class="node-detail-body">
        <el-card shadow="never" class="node-detail-card">
          <template #header>
            <span class="node-detail-card-title">资源使用</span>
          </template>
          <div class="node-detail-metrics">
            <div class="node-detail-metric">
              <el-progress
                type="dashboard"
                :percentage="metricPercent(detailNode.cpu_usage)"
                :color="progressColor(detailNode.cpu_usage)"
                :width="118"
              />
              <div class="node-detail-metric-label">CPU</div>
              <div class="node-detail-metric-meta">{{ detailNode.cpu_count ?? "-" }} 核</div>
            </div>
            <div class="node-detail-metric">
              <el-progress
                type="dashboard"
                :percentage="metricPercent(detailNode.mem_usage)"
                :color="progressColor(detailNode.mem_usage)"
                :width="118"
              />
              <div class="node-detail-metric-label">内存</div>
              <div class="node-detail-metric-meta">{{ formatBytes(detailNode.mem_total) }}</div>
            </div>
            <div class="node-detail-metric">
              <el-progress
                type="dashboard"
                :percentage="metricPercent(detailNode.disk_usage)"
                :color="progressColor(detailNode.disk_usage)"
                :width="118"
              />
              <div class="node-detail-metric-label">磁盘</div>
              <div class="node-detail-metric-meta">{{ formatBytes(detailNode.disk_total) }}</div>
            </div>
          </div>
        </el-card>

        <el-card shadow="never" class="node-detail-card">
          <template #header>
            <span class="node-detail-card-title">基础信息</span>
          </template>
          <el-descriptions :column="2" border size="small" class="node-detail-desc">
            <el-descriptions-item label="节点 ID">
              <span class="mono">{{ detailNode.id }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="主机名">{{ detailNode.hostname || "-" }}</el-descriptions-item>
            <el-descriptions-item label="公网 IP">
              <span class="mono">{{ detailNode.public_ip || "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="状态">
              <el-tag :type="epTagType(statusTagInfo(detailNode).theme)" effect="light" size="small">
                {{ statusTagInfo(detailNode).label }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="节点版本">
              <span class="mono">{{ detailNode.version || "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="配置版本">
              <span class="mono">{{ detailNode.config_version || "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="Capabilities" :span="2">
              <span class="mono">{{ (detailNode.capabilities || []).join(", ") || "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="最近心跳">{{ formatTime(detailNode.last_heartbeat) }}</el-descriptions-item>
            <el-descriptions-item label="通信状态">
              <el-tag :type="detailNode.comm_ok ? 'success' : 'danger'" effect="light" size="small">
                {{ detailNode.comm_ok ? "正常" : "异常" }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="上报状态">
              <el-tag :type="detailNode.report_ok ? 'success' : 'danger'" effect="light" size="small">
                {{ detailNode.report_ok ? "正常" : "异常" }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="创建时间">{{ formatTime(detailNode.created_at) }}</el-descriptions-item>
            <el-descriptions-item label="更新时间">{{ formatTime(detailNode.updated_at) }}</el-descriptions-item>
          </el-descriptions>
        </el-card>

        <el-card shadow="never" class="node-detail-card">
          <template #header>
            <span class="node-detail-card-title">流量与连接</span>
          </template>
          <el-descriptions :column="2" border size="small" class="node-detail-desc">
            <el-descriptions-item label="上行带宽">
              <span class="mono">↑ {{ formatBitsPerSec(detailNode.bandwidth_up_bps) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="下行带宽">
              <span class="mono">↓ {{ formatBitsPerSec(detailNode.bandwidth_down_bps) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="累计发送">
              <span class="mono">{{ formatBytes(detailNode.bytes_sent) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="累计接收">
              <span class="mono">{{ formatBytes(detailNode.bytes_received) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="本月发送">
              <span class="mono">{{ formatBytes(detailNode.month_bytes_sent) }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="TCP ESTABLISHED">
              <span class="mono">{{ detailNode.tcp_established ?? "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="TCP SYN_RECV">
              <span class="mono">{{ detailNode.tcp_syn_recv ?? "-" }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="TCP TIME_WAIT">
              <span class="mono">{{ detailNode.tcp_time_wait ?? "-" }}</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>

        <el-card shadow="never" class="node-detail-card">
          <template #header>
            <div class="node-detail-card-head">
              <span class="node-detail-card-title">监控状态</span>
              <el-button size="small" link type="primary" @click="openMonitorDialog(detailNode)">编辑配置</el-button>
            </div>
          </template>
          <el-descriptions :column="2" border size="small" class="node-detail-desc">
            <el-descriptions-item label="监控开关">
              <el-tag :type="detailNode.monitor_enabled ? 'success' : 'info'" effect="light" size="small">
                {{ detailNode.monitor_enabled ? "已开启" : "未开启" }}
              </el-tag>
            </el-descriptions-item>
            <el-descriptions-item label="协议">{{ (detailNode.monitor_protocol || "http").toUpperCase() }}</el-descriptions-item>
            <el-descriptions-item label="超时">{{ detailNode.monitor_timeout_seconds ?? 5 }}s</el-descriptions-item>
            <el-descriptions-item label="端口">{{ detailNode.monitor_port ?? 80 }}</el-descriptions-item>
            <el-descriptions-item label="失败阈值">{{ detailNode.monitor_fail_threshold ?? 3 }}</el-descriptions-item>
            <el-descriptions-item label="连续失败">
              <span class="mono">{{ detailNode.monitor_fail_count ?? 0 }}</span>
            </el-descriptions-item>
            <el-descriptions-item label="最近探测">{{ formatTime(detailNode.monitor_last_at) }}</el-descriptions-item>
            <el-descriptions-item label="最近结果">
              <el-tag
                v-if="detailNode.monitor_enabled"
                :type="detailNode.monitor_last_ok ? 'success' : 'danger'"
                effect="light"
                size="small"
              >
                {{ detailNode.monitor_last_ok ? "正常" : "异常" }}
              </el-tag>
              <span v-else>-</span>
            </el-descriptions-item>
            <el-descriptions-item label="延迟">
              <span class="mono">
                {{ detailNode.monitor_last_latency_ms != null ? `${detailNode.monitor_last_latency_ms}ms` : "-" }}
              </span>
            </el-descriptions-item>
            <el-descriptions-item v-if="detailNode.monitor_last_error" label="错误信息" :span="2">
              <span class="detail-error">{{ detailNode.monitor_last_error }}</span>
            </el-descriptions-item>
          </el-descriptions>
        </el-card>
      </div>

      <template v-if="detailNode" #footer>
        <div class="node-detail-footer">
          <el-button @click="detailDrawerVisible = false">关闭</el-button>
          <div class="node-detail-footer-actions">
            <el-button plain @click="openMonitorDialog(detailNode)">监控配置</el-button>
            <el-button v-if="detailNeedsUpgrade" type="warning" plain @click="openUpgradeDialog([detailNode.id])">
              升级节点
            </el-button>
            <el-button plain @click="handleToggleDisableConfirm(detailNode)">
              {{ detailIsDisabled ? "启用节点" : "禁用节点" }}
            </el-button>
            <el-button plain @click="handleResetTokenConfirm(detailNode)">重置令牌</el-button>
            <el-button type="danger" plain @click="handleDeleteNodeConfirm(detailNode)">删除节点</el-button>
          </div>
        </div>
      </template>
    </el-drawer>
  </div>
</template>

<script setup lang="ts">
import EpSelect from "@/components/ep/EpSelect.vue"
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, onBeforeUnmount, ref, watch } from "vue"
import { epTagType } from "@/lib/ep-tag"
import { MessagePlugin } from "@/lib/ep-message"
import { DialogPlugin } from "@/lib/ep-dialog"
import { ElTag, ElButton, ElDropdown, ElDropdownMenu, ElDropdownItem } from "element-plus"
import { api, type LocalBundleRecord, type Node, type NodeInstallSSHResult, type UpgradeNode } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { useNodeListSSE } from "@/composables/useNodeListSSE"
import ErrorState from "@/components/common/ErrorState.vue"
import LoadingState from "@/components/common/LoadingState.vue"

const tab = ref<"list">("list")
const loading = ref(false)
const listError = ref("")
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

const detailUpgradeInfo = computed(() => {
  if (!detailNode.value) return null
  return upgradeNodeMap.value[detailNode.value.id] || null
})

const detailNeedsUpgrade = computed(() => detailUpgradeInfo.value?.status === "upgrade_needed")

const detailUpgradeTarget = computed(() => detailUpgradeInfo.value?.target_version || "")

const detailIsDisabled = computed(() => {
  if (!detailNode.value) return false
  return (detailNode.value.status || "").trim().toLowerCase() === "disabled"
})

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
const installParams = ref({
  masterHost: "",
  ttlMinutes: 60,
  sshHost: "",
  sshPort: 22,
  sshUser: "root",
  sshPassword: "",
})
const installCommand = ref("")
const installCommandStyle = ref("")
const sshInstallResult = ref<NodeInstallSSHResult | null>(null)
const offlineLocalBundles = ref(false)
const localBundles = ref<LocalBundleRecord[]>([])
const bundleLoading = ref(false)
const bundleUploading = ref(false)
const bundleFileInput = ref<HTMLInputElement | null>(null)

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
const handleSelectionChange = (value: Array<string | number>) => {
  selectedRowKeys.value = value.map(String)
}
const selectedDisabledNodeIDs = computed(() => {
  const selected = new Set(selectedRowKeys.value)
  return nodes.value
    .filter((node) => selected.has(node.id) && (node.status || "").trim().toLowerCase() === "disabled")
    .map((node) => node.id)
})

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
  if (s === "pending") return { theme: "info", label: "待激活" }
  if (node?.comm_ok === false) return { theme: "danger", label: "通信异常" }
  if (node?.report_ok === false) return { theme: "warning", label: "上报异常" }
  if (!s) return { theme: "info", label: "未知" }
  if (s === "online" || s === "running") return { theme: "success", label: "在线" }
  if (s === "offline") return { theme: "warning", label: "离线" }
  if (s === "stopped") return { theme: "warning", label: "已停止" }
  return { theme: "info", label: node?.status || "未知" }
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

const metricPercent = (value?: number) => Math.max(0, Math.min(100, Math.round(value ?? 0)))

const progressColor = (value?: number) => {
  const v = value ?? 0
  if (v >= 90) return "var(--el-color-danger)"
  if (v >= 70) return "#E6A23C"
  return "var(--el-color-success)"
}

const skippedReasonLabel = (reason?: string) => {
  const text = String(reason || "").trim()
  if (!text) return "未知原因"
  if (text === "node disabled") return "节点已禁用"
  if (text === "communication timeout") return "通信超时"
  if (text === "never connected") return "从未连接"
  if (text === "node not found") return "节点不存在"
  return text
}

const formatSkippedNodes = (skipped?: { id: string; hostname?: string; reason?: string }[]) => {
  return (skipped || [])
    .map((s) => `${s.hostname || s.id}（${skippedReasonLabel(s.reason)}）`)
    .join("、")
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
  loadUpgradeInfo()
}

const loadUpgradeInfo = async () => {
  try {
    const upgrade = await api.getUpgradeInfo()
    const m: Record<string, UpgradeNode> = {}
    for (const n of upgrade?.nodes || []) {
      if (n?.id) m[n.id] = n
    }
    upgradeNodeMap.value = m
  } catch {
    // 升级徽章非关键路径，静默忽略
  }
}

const loadList = async (opts?: { page?: number; pageSize?: number }) => {
  try {
    loading.value = true
    listError.value = ""
    const nextPage = opts?.page ?? page.value
    const nextPageSize = opts?.pageSize ?? pageSize.value
    const q = search.value.trim()
    const status = statusFilter.value === "all" || statusFilter.value === "unknown" ? "" : statusFilter.value
    const res = await api.listNodesOverview({
      page: nextPage,
      pageSize: nextPageSize,
      q: q || undefined,
      status: status || undefined,
    })
    nodes.value = res.nodes || []
    page.value = res.page || nextPage
    pageSize.value = res.page_size || nextPageSize
    total.value = res.total || 0
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载节点列表失败"
    listError.value = msg
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
        const names = formatSkippedNodes(skipped)
        MessagePlugin.warning(`以下节点跳过升级：${names}`)
      } else {
        MessagePlugin.warning(res.message || "升级请求被拒绝")
      }
    } else {
      const id = String(res.task_id || "").trim()
      const skipped = (res as any).skipped as { id: string; hostname?: string; reason: string }[] | undefined
      if (skipped && skipped.length > 0) {
        const names = formatSkippedNodes(skipped)
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
const handleBatchEnable = () => {
  const ids = selectedDisabledNodeIDs.value
  if (!selectedRowKeys.value.length) return
  if (!ids.length) {
    MessagePlugin.warning("所选节点中没有已禁用节点")
    return
  }
  const skipped = selectedRowKeys.value.length - ids.length
  const dlg = DialogPlugin.confirm({
    header: "批量启用节点",
    body: skipped > 0
      ? `确认启用所选 ${ids.length} 个已禁用节点？另外 ${skipped} 个非禁用节点将跳过。`
      : `确认启用所选 ${ids.length} 个节点？启用后将恢复接收流量。`,
    confirmBtn: { content: "批量启用", theme: "primary" },
    cancelBtn: "取消",
    onConfirm: async () => {
      dlg.destroy()
      try {
        await Promise.all(ids.map((id) => api.updateNode(id, { status: "online" })))
        MessagePlugin.success(`已启用 ${ids.length} 个节点`)
        selectedRowKeys.value = []
        await loadList()
      } catch (err: any) {
        MessagePlugin.error(err.message || "批量启用失败")
      }
    },
    onClose: () => { dlg.destroy() },
  })
}

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
  installCommandStyle.value = ""
  sshInstallResult.value = null
  installParams.value.sshPassword = ""
  installDialogOpen.value = true
  void loadOfflineBundleContext()
  handleFetchInstallCommand()
}

const loadOfflineBundleContext = async () => {
  try {
    const lic = await api.getSystemLicenseStatus()
    offlineLocalBundles.value = String(lic.mode || "").toLowerCase() === "offline"
    if (offlineLocalBundles.value) {
      await loadLocalBundles()
    } else {
      localBundles.value = []
    }
  } catch {
    offlineLocalBundles.value = false
    localBundles.value = []
  }
}

const loadLocalBundles = async () => {
  if (!offlineLocalBundles.value) return
  try {
    bundleLoading.value = true
    const res = await api.getLocalBundles()
    localBundles.value = res.bundles || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载节点包列表失败")
  } finally {
    bundleLoading.value = false
  }
}

const triggerBundleUpload = () => {
  bundleFileInput.value?.click()
}

const handleBundleFileChange = async (ev: Event) => {
  const input = ev.target as HTMLInputElement
  const file = input.files?.[0]
  input.value = ""
  if (!file) return
  try {
    bundleUploading.value = true
    await api.uploadLocalBundle(file)
    MessagePlugin.success("节点包上传成功")
    await loadLocalBundles()
  } catch (err: any) {
    MessagePlugin.error(err.message || "上传失败")
  } finally {
    bundleUploading.value = false
  }
}

const handleDeleteBundle = async (filename: string) => {
  try {
    await api.deleteLocalBundle(filename)
    MessagePlugin.success("已删除")
    await loadLocalBundles()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除失败")
  }
}

const handleFetchInstallCommand = async () => {
  if (installLoading.value) return
  try {
    installLoading.value = true
    const res = await api.getNodeInstallCommand({
      masterHost: installParams.value.masterHost.trim() || undefined,
      ttlMinutes: Number(installParams.value.ttlMinutes) > 0 ? Number(installParams.value.ttlMinutes) : undefined,
    })
    installCommand.value = res.command || ""
    installCommandStyle.value = res.style || ""
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
        h("span", { style: "font-size:12px;color:var(--app-text-faint)" }, row.public_ip || "-"),
      ]),
  },
  {
    colKey: "status",
    title: "状态",
    width: 110,
    cell: (_h: any, { row }: { row: Node }) => {
      const info = statusTagInfo(row)
      return h(ElTag, { type: epTagType(info.theme), effect: "light" }, () => info.label)
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
          h(ElTag, { type: "warning", effect: "light", size: "small" }, () => `可升级到 ${upgrade.target_version}`),
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
      const theme = !enabled ? "info" : ok ? "success" : "danger"
      const protoLabel = proto === "ping" ? "ping" : `${proto}:${port || "-"}`
      const errMsg = String(row.monitor_last_error || "")
      const shortErr = errMsg.length > 60 ? `${errMsg.slice(0, 60)}...` : errMsg
      return h("div", { style: "display:flex;flex-direction:column;gap:4px" }, [
        h("div", { style: "display:flex;align-items:center;gap:8px" }, [
          h(ElTag, { type: epTagType(theme), effect: "light" }, () => label),
          h(ElButton, { size: "small", link: true, onClick: () => openMonitorDialog(row) },
            () => "配置"
          ),
        ]),
        h("span", { style: "font-size:12px;color:var(--app-text-faint)" }, `${protoLabel} · 超时 ${timeout}s · 阈值 ${threshold}`),
        enabled && row.monitor_last_at
          ? h(
              "span",
              { style: "font-size:12px;color:var(--app-text-faint)" },
              `${row.monitor_last_ok ? "✓" : "✗"} ${formatTime(row.monitor_last_at)} · ${Number(
                row.monitor_last_latency_ms || 0
              )}ms`
            )
          : null,
        enabled && shortErr ? h("span", { style: "font-size:12px;color:var(--app-danger)" }, shortErr) : null,
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
        { content: togglingToDisabled ? "禁用节点" : "启用节点", value: "toggle", theme: togglingToDisabled ? "error" : "info" },
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
        h(ElButton, { size: "small", link: true, type: "primary", onClick: () => openDetail(row) },
          () => "详情"
        ),
        !isLatest
          ? h(ElButton, { size: "small", plain: true, onClick: () => openUpgradeDialog([row.id]) },
              () => "升级"
            )
          : null,
        h(ElDropdown, { trigger: "click", onCommand: handleDropdownClick }, {
            default: () => h(ElButton, { size: "small", link: true }, () => "更多 ▾"),
            dropdown: () =>
              h(ElDropdownMenu, {}, () =>
                dropdownItems.map((item: any) =>
                  item.divider
                    ? null
                    : h(
                        ElDropdownItem,
                        {
                          command: item.value,
                          divided: item.value === "delete",
                          class: item.theme === "error" ? "dropdown-danger" : "",
                        },
                        () => item.content
                      )
                )
              ),
          }),
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
  onChange: (pi: { current: number; pageSize: number }) => {
    handleListPageChange(pi)
  },
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
  loadUpgradeInfo()
  sseConnect()
  startAutoRefresh()
  startUpgradeInfoRefresh()
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
    const res = await api.listNodesOverview({
      page: page.value,
      pageSize: pageSize.value,
      q: q || undefined,
      status: status || undefined,
    })
    nodes.value = res.nodes || []
    total.value = res.total || 0
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

const UPGRADE_INFO_INTERVAL = 5 * 60_000
let upgradeInfoTimer: ReturnType<typeof setInterval> | null = null

const startUpgradeInfoRefresh = () => {
  stopUpgradeInfoRefresh()
  upgradeInfoTimer = setInterval(() => {
    if (tab.value === "list") loadUpgradeInfo()
  }, UPGRADE_INFO_INTERVAL)
}

const stopUpgradeInfoRefresh = () => {
  if (upgradeInfoTimer) {
    clearInterval(upgradeInfoTimer)
    upgradeInfoTimer = null
  }
}

onBeforeUnmount(() => {
  stopAutoRefresh()
  stopUpgradeInfoRefresh()
  sseDisconnect()
})
</script>

<style scoped>
.node-detail-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  width: 100%;
  padding-right: 8px;
}

.node-detail-header-main {
  min-width: 0;
}

.node-detail-title {
  font-size: 18px;
  font-weight: 600;
  color: var(--app-text-strong);
  line-height: 1.3;
}

.node-detail-subtitle {
  margin-top: 4px;
  font-size: 13px;
  color: var(--app-text-muted);
}

.node-detail-header-tags {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  flex-shrink: 0;
}

.node-detail-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
  padding: 4px 20px 20px;
}

.node-detail-card :deep(.el-card__header) {
  padding: 14px 16px;
  border-bottom: 1px solid var(--app-border);
}

.node-detail-card :deep(.el-card__body) {
  padding: 16px;
}

.node-detail-card-title {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.node-detail-card-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.node-detail-metrics {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 16px;
}

.node-detail-metric {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  padding: 8px 4px 4px;
  border-radius: var(--app-card-radius-sm);
  background: var(--app-surface-muted);
}

.node-detail-metric-label {
  font-size: 14px;
  font-weight: 600;
  color: var(--app-text-strong);
}

.node-detail-metric-meta {
  font-size: 12px;
  color: var(--app-text-faint);
  text-align: center;
}

.node-detail-desc :deep(.el-descriptions__label) {
  width: 112px;
  color: var(--app-text-muted);
  font-weight: 500;
}

.node-detail-desc :deep(.el-descriptions__content) {
  color: var(--app-text-strong);
}

.node-detail-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  width: 100%;
}

.node-detail-footer-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  justify-content: flex-end;
}

@media (max-width: 768px) {
  .node-detail-header {
    flex-direction: column;
    align-items: flex-start;
  }

  .node-detail-body {
    padding: 4px 16px 16px;
  }

  .node-detail-metrics {
    grid-template-columns: 1fr;
  }

  .node-detail-footer {
    flex-direction: column;
    align-items: stretch;
  }

  .node-detail-footer-actions {
    justify-content: stretch;
  }

  .node-detail-footer-actions :deep(.el-button) {
    flex: 1 1 calc(50% - 4px);
    margin: 0;
  }
}
</style>
