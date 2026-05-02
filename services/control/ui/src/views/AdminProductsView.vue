
<template>
  <div class="admin-page">
    <div v-if="error" class="admin-error-box">
      {{ error }}
      <t-button size="small" theme="primary" @click="loadData">重试</t-button>
    </div>

    <t-card bordered class="admin-list-card">
      <t-tabs v-model="activeTab" class="admin-tabs">
        <t-tab-panel value="list" label="套餐列表">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button theme="primary" @click="openCreate">新增套餐</t-button>
              <t-button variant="outline" @click="openGroupDialog()">管理分组</t-button>
            </div>
            <div class="admin-toolbar-right">
              <t-select v-model="searchField" :options="searchFieldOptions" class="admin-search-select" />
              <t-input v-model="searchText" class="admin-search-input" clearable placeholder="搜索" />
            </div>
          </div>

          <div class="admin-filters-row">
            <div class="admin-filters-left">
              <t-select v-model="groupFilter" :options="groupOptions" class="admin-filter-select-large" />
              <t-select v-model="sortKey" :options="sortOptions" class="admin-filter-select-large" />
            </div>
            <t-button variant="outline" @click="loadData">刷新</t-button>
          </div>

          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <t-empty v-if="filteredProducts.length === 0" :description="searchText ? '暂无匹配套餐' : '暂无套餐数据'" class="admin-empty" />
            <t-table
              v-else
              :data="pagedProducts"
              :columns="columns"
              row-key="id"
              bordered
              hover
              stripe
              size="medium"
              :loading="loading"
              :selected-row-keys="selectedRowKeys"
              :pagination="pagination"
              @select-change="handleSelectChange"
            />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="filteredProducts.length === 0" class="admin-mobile-card-empty">
              {{ searchText ? '暂无匹配套餐' : '暂无套餐数据' }}
            </div>
            <div v-else class="admin-mobile-cards">
              <div v-for="product in pagedProducts" :key="product.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ product.name || '-' }}</span>
                  <div class="admin-mobile-card-tags">
                    <t-tag :theme="product.enabled ? 'success' : 'default'" variant="light" size="small">
                      {{ product.enabled ? '启用' : '停用' }}
                    </t-tag>
                    <t-tag theme="default" variant="light" size="small">
                      {{ product.currency || 'CNY' }}
                    </t-tag>
                  </div>
                </div>
                <div v-if="product.slug" class="admin-mobile-card-subtitle">{{ product.slug }}</div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">分组</span>
                    <span class="admin-mobile-card-value">{{ groupNameMap[product.group_id || ''] || '未分组' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">月付</span>
                    <span class="admin-mobile-card-value">{{ toYuan(product.price_month_cents ?? product.price_cents ?? 0).toFixed(2) }} 元</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">季度付</span>
                    <span class="admin-mobile-card-value">{{ toYuan(product.price_quarter_cents ?? 0).toFixed(2) }} 元</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">年付</span>
                    <span class="admin-mobile-card-value">{{ toYuan(product.price_year_cents ?? 0).toFixed(2) }} 元</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">月流量</span>
                    <span class="admin-mobile-card-value">{{ bytesToGB(product.monthly_traffic_bytes) === null ? '不限' : bytesToGB(product.monthly_traffic_bytes) + ' GB' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">带宽</span>
                    <span class="admin-mobile-card-value">{{ bpsToMbps(product.bandwidth_bps) === null ? '不限' : bpsToMbps(product.bandwidth_bps) + ' Mbps' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">连接数</span>
                    <span class="admin-mobile-card-value">{{ product.conn_limit == null ? '不限' : product.conn_limit }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">域名数</span>
                    <span class="admin-mobile-card-value">{{ product.domain_limit == null ? '不限' : product.domain_limit }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">主域名数</span>
                    <span class="admin-mobile-card-value">{{ product.primary_domain_limit == null ? '不限' : product.primary_domain_limit }}</span>
                  </div>
                </div>
                <div class="admin-mobile-card-actions">
                  <t-button size="small" variant="outline" @click="openEdit(product)">编辑</t-button>
                  <t-button size="small" theme="danger" variant="outline" @click="handleDelete(product)">删除</t-button>
                </div>
              </div>
            </div>
            <div v-if="filteredProducts.length > 0" class="admin-mobile-pagination">
              <t-pagination
                :current="page"
                :page-size="pageSize"
                :total="filteredProducts.length"
                :page-size-options="[10, 20, 50, 100]"
                show-page-size
                size="small"
                @current-change="(v: number) => (page = v)"
                @page-size-change="(v: number) => { pageSize = v; page = 1 }"
              />
            </div>
          </div>
        </t-tab-panel>

        <t-tab-panel value="group" label="套餐分组">
          <div class="admin-toolbar">
            <div class="admin-toolbar-left">
              <t-button theme="primary" @click="openGroupDialog()">新增分组</t-button>
            </div>
            <div class="admin-toolbar-right">
              <t-button variant="outline" @click="loadData">刷新</t-button>
            </div>
          </div>

          <!-- Desktop: table view -->
          <div class="admin-desktop-only">
            <t-empty v-if="groups.length === 0" description="暂无分组" class="admin-empty" />
            <t-table v-else :data="groups" :columns="groupColumns" row-key="id" bordered hover stripe size="medium" :loading="loading" />
          </div>

          <!-- Mobile: card view -->
          <div class="admin-mobile-only">
            <div v-if="groups.length === 0" class="admin-mobile-card-empty">暂无分组</div>
            <div v-else class="admin-mobile-cards">
              <div v-for="g in groups" :key="g.id" class="admin-mobile-card">
                <div class="admin-mobile-card-header">
                  <span class="admin-mobile-card-title">{{ g.name || '-' }}</span>
                </div>
                <div class="admin-mobile-card-rows">
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">排序</span>
                    <span class="admin-mobile-card-value">{{ g.sort ?? 100 }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">描述</span>
                    <span class="admin-mobile-card-value">{{ g.description || '-' }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">创建时间</span>
                    <span class="admin-mobile-card-value">{{ formatDate(g.created_at) }}</span>
                  </div>
                  <div class="admin-mobile-card-row">
                    <span class="admin-mobile-card-label">更新时间</span>
                    <span class="admin-mobile-card-value">{{ formatDate(g.updated_at) }}</span>
                  </div>
                </div>
                <div class="admin-mobile-card-actions">
                  <t-button size="small" variant="outline" @click="openGroupDialog(g)">编辑</t-button>
                  <t-button size="small" theme="danger" variant="outline" @click="deleteGroup(g)">删除</t-button>
                </div>
              </div>
            </div>
          </div>
        </t-tab-panel>
      </t-tabs>
    </t-card>

    <t-dialog
      v-model:visible="createDialogVisible"
      header="新增套餐"
      :confirm-btn="{ content: creating ? '创建中...' : '创建', loading: creating, theme: 'primary' }"
      cancel-btn="取消"
      width="760"
      @confirm="handleCreate"
    >
      <t-form layout="vertical" label-align="top" class="admin-form-grid">
        <t-form-item label="套餐名称">
          <t-input v-model="createForm.name" />
        </t-form-item>
        <t-form-item label="标识">
          <t-input v-model="createForm.slug" />
        </t-form-item>
        <t-form-item label="分组">
          <t-select v-model="createForm.group_id" :options="[{ label: '未分组', value: '' }, ...groups.map((g) => ({ label: g.name, value: g.id }))]" />
        </t-form-item>
        <t-form-item label="排序">
          <t-input-number v-model="createForm.sort" :min="1" :step="1" />
        </t-form-item>
        <t-form-item label="集群">
          <t-select v-model="createForm.cluster_id" :options="[{ label: '无集群', value: '' }, ...clusterOptions]" />
        </t-form-item>
        <t-form-item label="币种">
          <t-select v-model="createForm.currency" :options="[{ label: 'CNY', value: 'CNY' }]" />
        </t-form-item>
        <t-form-item label="描述" style="grid-column: 1 / -1">
          <t-textarea v-model="createForm.description" :autosize="{ minRows: 3, maxRows: 6 }" />
        </t-form-item>

        <t-form-item label="月流量 (GB)">
          <div class="form-row">
            <t-switch :model-value="createForm.monthly_traffic_gb === null" @change="(v) => (createForm.monthly_traffic_gb = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.monthly_traffic_gb" :min="0" :step="1" :disabled="createForm.monthly_traffic_gb === null" />
          </div>
        </t-form-item>
        <t-form-item label="带宽 (Mbps)">
          <div class="form-row">
            <t-switch :model-value="createForm.bandwidth_mbps === null" @change="(v) => (createForm.bandwidth_mbps = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.bandwidth_mbps" :min="0" :step="1" :disabled="createForm.bandwidth_mbps === null" />
          </div>
        </t-form-item>
        <t-form-item label="连接数">
          <div class="form-row">
            <t-switch :model-value="createForm.conn_limit === null" @change="(v) => (createForm.conn_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.conn_limit" :min="0" :step="1" :disabled="createForm.conn_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="域名数">
          <div class="form-row">
            <t-switch :model-value="createForm.domain_limit === null" @change="(v) => (createForm.domain_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.domain_limit" :min="0" :step="1" :disabled="createForm.domain_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="主域名数">
          <div class="form-row">
            <t-switch :model-value="createForm.primary_domain_limit === null" @change="(v) => (createForm.primary_domain_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.primary_domain_limit" :min="0" :step="1" :disabled="createForm.primary_domain_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="HTTP 端口数">
          <div class="form-row">
            <t-switch :model-value="createForm.http_port_limit === null" @change="(v) => (createForm.http_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.http_port_limit" :min="0" :step="1" :disabled="createForm.http_port_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="转发端口数">
          <div class="form-row">
            <t-switch :model-value="createForm.stream_port_limit === null" @change="(v) => (createForm.stream_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.stream_port_limit" :min="0" :step="1" :disabled="createForm.stream_port_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="非标准端口数">
          <div class="form-row">
            <t-switch :model-value="createForm.non_std_port_limit === null" @change="(v) => (createForm.non_std_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="createForm.non_std_port_limit" :min="0" :step="1" :disabled="createForm.non_std_port_limit === null" />
          </div>
        </t-form-item>

        <t-form-item label="Websocket">
          <t-switch v-model="createForm.websocket" />
        </t-form-item>
        <t-form-item label="自定义 CC 规则">
          <t-switch v-model="createForm.custom_cc_rules" />
        </t-form-item>
        <t-form-item label="HTTP3">
          <t-switch v-model="createForm.http3" />
        </t-form-item>
        <t-form-item label="L2 节点回源">
          <t-switch v-model="createForm.l2_origin" />
        </t-form-item>
        <t-form-item label="CC 防护">
          <t-input v-model="createForm.cc_protection" />
        </t-form-item>
        <t-form-item label="DDoS 防护">
          <t-input v-model="createForm.ddos_protection" />
        </t-form-item>

        <t-form-item label="月付 (元)">
          <t-input-number v-model="createForm.price_month" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="季度付 (元)">
          <t-input-number v-model="createForm.price_quarter" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="年付 (元)">
          <t-input-number v-model="createForm.price_year" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="启用">
          <t-switch v-model="createForm.enabled" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="editDialogVisible"
      header="编辑套餐"
      :confirm-btn="{ content: updating ? '保存中...' : '保存', loading: updating, theme: 'primary' }"
      cancel-btn="取消"
      width="760"
      @confirm="handleUpdate"
      @close="closeEdit"
    >
      <t-form layout="vertical" label-align="top" class="admin-form-grid">
        <t-form-item label="套餐名称">
          <t-input v-model="editForm.name" />
        </t-form-item>
        <t-form-item label="标识">
          <t-input v-model="editForm.slug" />
        </t-form-item>
        <t-form-item label="分组">
          <t-select v-model="editForm.group_id" :options="[{ label: '未分组', value: '' }, ...groups.map((g) => ({ label: g.name, value: g.id }))]" />
        </t-form-item>
        <t-form-item label="排序">
          <t-input-number v-model="editForm.sort" :min="1" :step="1" />
        </t-form-item>
        <t-form-item label="集群">
          <t-select v-model="editForm.cluster_id" :options="[{ label: '无集群', value: '' }, ...clusterOptions]" />
        </t-form-item>
        <t-form-item label="币种">
          <t-select v-model="editForm.currency" :options="[{ label: 'CNY', value: 'CNY' }]" />
        </t-form-item>
        <t-form-item label="描述" style="grid-column: 1 / -1">
          <t-textarea v-model="editForm.description" :autosize="{ minRows: 3, maxRows: 6 }" />
        </t-form-item>

        <t-form-item label="月流量 (GB)">
          <div class="form-row">
            <t-switch :model-value="editForm.monthly_traffic_gb === null" @change="(v) => (editForm.monthly_traffic_gb = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.monthly_traffic_gb" :min="0" :step="1" :disabled="editForm.monthly_traffic_gb === null" />
          </div>
        </t-form-item>
        <t-form-item label="带宽 (Mbps)">
          <div class="form-row">
            <t-switch :model-value="editForm.bandwidth_mbps === null" @change="(v) => (editForm.bandwidth_mbps = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.bandwidth_mbps" :min="0" :step="1" :disabled="editForm.bandwidth_mbps === null" />
          </div>
        </t-form-item>
        <t-form-item label="连接数">
          <div class="form-row">
            <t-switch :model-value="editForm.conn_limit === null" @change="(v) => (editForm.conn_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.conn_limit" :min="0" :step="1" :disabled="editForm.conn_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="域名数">
          <div class="form-row">
            <t-switch :model-value="editForm.domain_limit === null" @change="(v) => (editForm.domain_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.domain_limit" :min="0" :step="1" :disabled="editForm.domain_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="主域名数">
          <div class="form-row">
            <t-switch :model-value="editForm.primary_domain_limit === null" @change="(v) => (editForm.primary_domain_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.primary_domain_limit" :min="0" :step="1" :disabled="editForm.primary_domain_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="HTTP 端口数">
          <div class="form-row">
            <t-switch :model-value="editForm.http_port_limit === null" @change="(v) => (editForm.http_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.http_port_limit" :min="0" :step="1" :disabled="editForm.http_port_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="转发端口数">
          <div class="form-row">
            <t-switch :model-value="editForm.stream_port_limit === null" @change="(v) => (editForm.stream_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.stream_port_limit" :min="0" :step="1" :disabled="editForm.stream_port_limit === null" />
          </div>
        </t-form-item>
        <t-form-item label="非标准端口数">
          <div class="form-row">
            <t-switch :model-value="editForm.non_std_port_limit === null" @change="(v) => (editForm.non_std_port_limit = v ? null : 0)" />
            <span class="admin-table-muted">不限</span>
            <t-input-number v-model="editForm.non_std_port_limit" :min="0" :step="1" :disabled="editForm.non_std_port_limit === null" />
          </div>
        </t-form-item>

        <t-form-item label="Websocket">
          <t-switch v-model="editForm.websocket" />
        </t-form-item>
        <t-form-item label="自定义 CC 规则">
          <t-switch v-model="editForm.custom_cc_rules" />
        </t-form-item>
        <t-form-item label="HTTP3">
          <t-switch v-model="editForm.http3" />
        </t-form-item>
        <t-form-item label="L2 节点回源">
          <t-switch v-model="editForm.l2_origin" />
        </t-form-item>
        <t-form-item label="CC 防护">
          <t-input v-model="editForm.cc_protection" />
        </t-form-item>
        <t-form-item label="DDoS 防护">
          <t-input v-model="editForm.ddos_protection" />
        </t-form-item>

        <t-form-item label="月付 (元)">
          <t-input-number v-model="editForm.price_month" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="季度付 (元)">
          <t-input-number v-model="editForm.price_quarter" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="年付 (元)">
          <t-input-number v-model="editForm.price_year" :min="0" :step="1" />
        </t-form-item>
        <t-form-item label="启用">
          <t-switch v-model="editForm.enabled" />
        </t-form-item>
      </t-form>
    </t-dialog>

    <t-dialog
      v-model:visible="groupDialogVisible"
      :header="groupEditing ? '编辑分组' : '新增分组'"
      :confirm-btn="{ content: groupSaving ? '保存中...' : '保存', loading: groupSaving, theme: 'primary' }"
      cancel-btn="取消"
      width="520"
      @confirm="saveGroup"
      @close="closeGroupDialog"
    >
      <t-form layout="vertical" label-align="top">
        <t-form-item label="分组名称">
          <t-input v-model="groupName" />
        </t-form-item>
        <t-form-item label="排序">
          <t-input-number v-model="groupSort" :min="1" :step="1" />
        </t-form-item>
        <t-form-item label="描述">
          <t-textarea v-model="groupDescription" :autosize="{ minRows: 3, maxRows: 6 }" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref, watch } from "vue"
import { MessagePlugin, Tag, Button } from "tdesign-vue-next"
import { api, type Cluster, type Product, type ProductGroup } from "@/lib/api"

type SearchField = "keyword" | "name" | "slug" | "description"

type ProductFormState = {
  name: string
  slug: string
  description: string
  group_id: string
  sort: number
  region: string
  line_group_id: string
  cluster_id: string

  monthly_traffic_gb: number | null
  bandwidth_mbps: number | null
  conn_limit: number | null

  domain_limit: number | null
  primary_domain_limit: number | null
  http_port_limit: number | null
  stream_port_limit: number | null
  non_std_port_limit: number | null

  websocket: boolean
  custom_cc_rules: boolean
  http3: boolean
  l2_origin: boolean
  cc_protection: string
  ddos_protection: string

  price_month: number
  price_quarter: number
  price_year: number
  currency: string
  enabled: boolean
}

const defaultFormState: ProductFormState = {
  name: "",
  slug: "",
  description: "",
  group_id: "",
  sort: 100,
  region: "",
  line_group_id: "",
  cluster_id: "",

  monthly_traffic_gb: null,
  bandwidth_mbps: null,
  conn_limit: null,

  domain_limit: null,
  primary_domain_limit: null,
  http_port_limit: null,
  stream_port_limit: null,
  non_std_port_limit: null,

  websocket: true,
  custom_cc_rules: true,
  http3: false,
  l2_origin: false,
  cc_protection: "",
  ddos_protection: "",

  price_month: 0,
  price_quarter: 0,
  price_year: 0,
  currency: "CNY",
  enabled: true,
}

const products = ref<Product[]>([])
const groups = ref<ProductGroup[]>([])
const lineGroups = ref<Cluster[]>([])
const clusters = ref<Cluster[]>([])
const clusterOptions = computed(() => clusters.value.map(c => ({ label: c.name, value: c.id })))
const clusterNameMap = computed(() => {
  const map: Record<string, string> = {}
  for (const c of clusters.value) {
    if (c?.id) map[c.id] = c.name
  }
  return map
})
const loading = ref(true)
const error = ref("")
const searchText = ref("")
const searchField = ref<SearchField>("keyword")
const sortKey = ref("created_desc")
const groupFilter = ref("all")
const activeTab = ref("list")
const selectedRowKeys = ref<Array<string | number>>([])
const page = ref(1)
const pageSize = ref(10)

const createDialogVisible = ref(false)
const editDialogVisible = ref(false)
const creating = ref(false)
const updating = ref(false)
const createForm = ref<ProductFormState>({ ...defaultFormState })
const editForm = ref<ProductFormState>({ ...defaultFormState })
const editingProduct = ref<Product | null>(null)

const groupDialogVisible = ref(false)
const groupEditing = ref<ProductGroup | null>(null)
const groupName = ref("")
const groupSort = ref(100)
const groupDescription = ref("")
const groupSaving = ref(false)

const searchFieldOptions = [
  { label: "关键词", value: "keyword" },
  { label: "套餐名", value: "name" },
  { label: "标识", value: "slug" },
  { label: "描述", value: "description" },
]

const sortOptions = [
  { label: "创建时间（新→旧）", value: "created_desc" },
  { label: "创建时间（旧→新）", value: "created_asc" },
  { label: "排序（小→大）", value: "sort_asc" },
  { label: "排序（大→小）", value: "sort_desc" },
  { label: "名称（A→Z）", value: "name_asc" },
  { label: "名称（Z→A）", value: "name_desc" },
]

const groupOptions = computed(() => [
  { label: "全部分组", value: "all" },
  ...groups.value.map((g) => ({ label: g.name, value: g.id })),
])

watch([searchText, searchField, sortKey, groupFilter, pageSize], () => {
  page.value = 1
})

const loadData = async () => {
  try {
    loading.value = true
    const [productsRes, groupsRes, clustersRes] = await Promise.all([
      api.getProducts(),
      api.getProductGroups(),
      api.listClusters(),
    ])
    products.value = productsRes?.products || []
    groups.value = groupsRes?.groups || []
    lineGroups.value = clustersRes?.clusters || []
    clusters.value = clustersRes?.clusters || []
    error.value = ""
  } catch (err: any) {
    const msg = err.message || "加载套餐列表失败"
    error.value = msg
    MessagePlugin.error(msg)
  } finally {
    loading.value = false
  }
}

const formatDate = (value?: string) => {
  if (!value) return "-"
  return new Date(value).toLocaleDateString("zh-CN")
}

const bytesToGB = (v?: number | null) => {
  if (v === null || v === undefined) return null
  const gb = v / (1024 * 1024 * 1024)
  return Math.round(gb * 100) / 100
}

const gbToBytes = (gb?: number | null) => {
  if (gb === null || gb === undefined) return null
  return Math.round(gb * 1024 * 1024 * 1024)
}

const bpsToMbps = (v?: number | null) => {
  if (v === null || v === undefined) return null
  const mbps = v / 1_000_000
  return Math.round(mbps * 100) / 100
}

const mbpsToBps = (mbps?: number | null) => {
  if (mbps === null || mbps === undefined) return null
  return Math.round(mbps * 1_000_000)
}

const toCents = (value?: number) => Math.round((value || 0) * 100)
const toYuan = (cents?: number) => {
  const v = typeof cents === "number" ? cents / 100 : 0
  return Math.round(v * 100) / 100
}

const validateForm = (form: ProductFormState) => {
  if (!form.name.trim()) return "请输入套餐名称"
  if (form.sort <= 0) return "排序必须大于 0"
  if (form.price_month < 0 || form.price_quarter < 0 || form.price_year < 0) return "价格不能小于 0"
  if (!String(form.currency || "").trim()) return "请选择币种"
  return ""
}

const openCreate = () => {
  createForm.value = { ...defaultFormState }
  createDialogVisible.value = true
}

const openGroupDialog = (g?: ProductGroup) => {
  groupEditing.value = g || null
  groupName.value = g?.name || ""
  groupSort.value = g?.sort || 100
  groupDescription.value = g?.description || ""
  groupDialogVisible.value = true
}

const closeGroupDialog = () => {
  groupDialogVisible.value = false
  groupEditing.value = null
  groupName.value = ""
  groupSort.value = 100
  groupDescription.value = ""
}

const saveGroup = async () => {
  if (groupSaving.value) return
  const name = groupName.value.trim()
  if (!name) {
    MessagePlugin.error("请输入分组名称")
    return
  }
  groupSaving.value = true
  try {
    if (groupEditing.value) {
      await api.updateProductGroup(groupEditing.value.id, {
        name,
        sort: groupSort.value || 100,
        description: groupDescription.value,
      })
      MessagePlugin.success("分组已更新")
    } else {
      await api.createProductGroup({
        name,
        sort: groupSort.value || 100,
        description: groupDescription.value,
      })
      MessagePlugin.success("分组创建成功")
    }
    closeGroupDialog()
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "保存失败")
  } finally {
    groupSaving.value = false
  }
}

const deleteGroup = async (g: ProductGroup) => {
  if (!g?.id) return
  if (!window.confirm(`确认删除分组“${g.name}”吗？`)) return
  try {
    await api.deleteProductGroup(g.id)
    MessagePlugin.success("分组已删除")
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除失败")
  }
}

const openEdit = (p: Product) => {
  editingProduct.value = p
  editForm.value = {
    name: p.name || "",
    slug: p.slug || "",
    description: p.description || "",
    group_id: p.group_id || "",
    sort: p.sort ?? 100,
    region: p.region || "",
    // Server's canonical field is `line_group_id`; mirror it into the UI's
    // `cluster_id` binding so the cluster dropdown pre-selects correctly
    // when re-editing. Without this fallback, persisted bindings appear
    // "unset" in the UI even though they're stored on the server.
    line_group_id: p.line_group_id || p.cluster_id || "",
    cluster_id: p.cluster_id || p.line_group_id || "",

    monthly_traffic_gb: bytesToGB(p.monthly_traffic_bytes),
    bandwidth_mbps: bpsToMbps(p.bandwidth_bps),
    conn_limit: p.conn_limit ?? null,

    domain_limit: p.domain_limit ?? null,
    primary_domain_limit: p.primary_domain_limit ?? null,
    http_port_limit: p.http_port_limit ?? null,
    stream_port_limit: p.stream_port_limit ?? null,
    non_std_port_limit: p.non_std_port_limit ?? null,

    websocket: p.websocket ?? true,
    custom_cc_rules: p.custom_cc_rules ?? true,
    http3: p.http3 ?? false,
    l2_origin: p.l2_origin ?? false,
    cc_protection: p.cc_protection || "",
    ddos_protection: p.ddos_protection || "",

    price_month: toYuan(p.price_month_cents ?? p.price_cents ?? 0),
    price_quarter: toYuan(p.price_quarter_cents ?? 0),
    price_year: toYuan(p.price_year_cents ?? 0),
    currency: p.currency || "CNY",
    enabled: Boolean(p.enabled),
  }
  editDialogVisible.value = true
}

const closeEdit = () => {
  editDialogVisible.value = false
  editingProduct.value = null
  editForm.value = { ...defaultFormState }
}

const handleCreate = async () => {
  if (creating.value) return
  const errMsg = validateForm(createForm.value)
  if (errMsg) {
    MessagePlugin.error(errMsg)
    return
  }
  creating.value = true
  try {
    await api.createProduct({
      name: createForm.value.name.trim(),
      slug: createForm.value.slug.trim(),
      description: createForm.value.description.trim(),
      group_id: createForm.value.group_id || "",
      sort: createForm.value.sort || 100,
      region: createForm.value.region,
      line_group_id: createForm.value.line_group_id,
      cluster_id: createForm.value.cluster_id,

      monthly_traffic_bytes: gbToBytes(createForm.value.monthly_traffic_gb),
      bandwidth_bps: mbpsToBps(createForm.value.bandwidth_mbps),
      conn_limit: createForm.value.conn_limit,
      domain_limit: createForm.value.domain_limit,
      primary_domain_limit: createForm.value.primary_domain_limit,
      http_port_limit: createForm.value.http_port_limit,
      stream_port_limit: createForm.value.stream_port_limit,
      non_std_port_limit: createForm.value.non_std_port_limit,
      websocket: createForm.value.websocket,
      custom_cc_rules: createForm.value.custom_cc_rules,
      http3: createForm.value.http3,
      l2_origin: createForm.value.l2_origin,
      cc_protection: createForm.value.cc_protection,
      ddos_protection: createForm.value.ddos_protection,

      price_month_cents: toCents(createForm.value.price_month),
      price_quarter_cents: toCents(createForm.value.price_quarter),
      price_year_cents: toCents(createForm.value.price_year),
      currency: String(createForm.value.currency || "CNY").trim(),
      enabled: Boolean(createForm.value.enabled),
    })
    MessagePlugin.success("套餐创建成功")
    createDialogVisible.value = false
    createForm.value = { ...defaultFormState }
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "创建失败")
  } finally {
    creating.value = false
  }
}

const handleUpdate = async () => {
  if (!editingProduct.value || updating.value) return
  const errMsg = validateForm(editForm.value)
  if (errMsg) {
    MessagePlugin.error(errMsg)
    return
  }
  updating.value = true
  try {
    await api.updateProduct(editingProduct.value.id, {
      name: editForm.value.name.trim(),
      slug: editForm.value.slug.trim(),
      description: editForm.value.description.trim(),
      group_id: editForm.value.group_id || "",
      sort: editForm.value.sort || 100,
      region: editForm.value.region,
      line_group_id: editForm.value.line_group_id,
      cluster_id: editForm.value.cluster_id,

      monthly_traffic_bytes: gbToBytes(editForm.value.monthly_traffic_gb),
      bandwidth_bps: mbpsToBps(editForm.value.bandwidth_mbps),
      conn_limit: editForm.value.conn_limit,
      domain_limit: editForm.value.domain_limit,
      primary_domain_limit: editForm.value.primary_domain_limit,
      http_port_limit: editForm.value.http_port_limit,
      stream_port_limit: editForm.value.stream_port_limit,
      non_std_port_limit: editForm.value.non_std_port_limit,
      websocket: editForm.value.websocket,
      custom_cc_rules: editForm.value.custom_cc_rules,
      http3: editForm.value.http3,
      l2_origin: editForm.value.l2_origin,
      cc_protection: editForm.value.cc_protection,
      ddos_protection: editForm.value.ddos_protection,

      price_month_cents: toCents(editForm.value.price_month),
      price_quarter_cents: toCents(editForm.value.price_quarter),
      price_year_cents: toCents(editForm.value.price_year),
      currency: String(editForm.value.currency || "CNY").trim(),
      enabled: Boolean(editForm.value.enabled),
    })
    MessagePlugin.success("套餐已更新")
    closeEdit()
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "更新失败")
  } finally {
    updating.value = false
  }
}

const handleDelete = async (p: Product) => {
  if (!p?.id) return
  if (!window.confirm(`确认删除套餐“${p.name}”吗？`)) return
  try {
    await api.deleteProduct(p.id)
    MessagePlugin.success("套餐已删除")
    await loadData()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除失败")
  }
}

const groupNameMap = computed(() => {
  const map: Record<string, string> = {}
  for (const g of groups.value) {
    if (g?.id) map[g.id] = g.name
  }
  return map
})

const lineGroupNameMap = computed(() => {
  const map: Record<string, string> = {}
  for (const lg of lineGroups.value) {
    if (lg?.id) map[lg.id] = lg.name
  }
  return map
})

const filteredProducts = computed(() => {
  const query = searchText.value.trim().toLowerCase()
  let data = products.value.filter((product) => {
    if (!query) return true
    const name = product.name?.toLowerCase() || ""
    const slug = product.slug?.toLowerCase() || ""
    const desc = product.description?.toLowerCase() || ""
    switch (searchField.value) {
      case "name":
        return name.includes(query)
      case "slug":
        return slug.includes(query)
      case "description":
        return desc.includes(query)
      default:
        return name.includes(query) || slug.includes(query) || desc.includes(query)
    }
  })

  if (groupFilter.value !== "all") {
    data = data.filter((p) => (p.group_id || "") === groupFilter.value)
  }

  data = [...data].sort((a, b) => {
    switch (sortKey.value) {
      case "sort_asc":
        return (a.sort ?? 100) - (b.sort ?? 100)
      case "sort_desc":
        return (b.sort ?? 100) - (a.sort ?? 100)
      case "created_asc":
        return new Date(a.created_at).getTime() - new Date(b.created_at).getTime()
      case "name_asc":
        return (a.name || "").localeCompare(b.name || "", "zh-CN")
      case "name_desc":
        return (b.name || "").localeCompare(a.name || "", "zh-CN")
      case "created_desc":
      default:
        return new Date(b.created_at).getTime() - new Date(a.created_at).getTime()
    }
  })

  return data
})

const pagedProducts = computed(() => {
  const start = (page.value - 1) * pageSize.value
  return filteredProducts.value.slice(start, start + pageSize.value)
})

const columns = computed(() => [
  { colKey: "row-select", type: "multiple", width: 48, fixed: "left" },
  {
    colKey: "serial",
    title: "ID",
    width: 70,
    cell: (_h: any, { rowIndex }: { rowIndex: number }) => (page.value - 1) * pageSize.value + rowIndex + 1,
  },
  {
    colKey: "name",
    title: "套餐名称",
    minWidth: 200,
    cell: (_h: any, { row }: { row: Product }) =>
      h("div", { style: "display:flex;flex-direction:column;gap:4px" }, [
        h("span", { style: "font-weight:600;color:#0f172a;font-size:14px" }, row.name || "-"),
        h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
          h(Tag, { theme: row.enabled ? "success" : "default", variant: "light", size: "small" }, () =>
            row.enabled ? "启用" : "停用"
          ),
          h(Tag, { theme: "default", variant: "light", size: "small" }, () => row.currency || "CNY"),
          h(
            Tag,
            { theme: "default", variant: "light", size: "small" },
            () => groupNameMap.value[row.group_id || ""] || "未分组"
          ),
        ]),
      ]),
  },
  {
    colKey: "sort",
    title: "排序",
    width: 100,
    cell: (_h: any, { row }: { row: Product }) => h("span", { class: "admin-table-muted" }, String(row.sort ?? 100)),
  },
  {
    colKey: "cluster_id",
    title: "集群",
    width: 140,
    cell: (_h: any, { row }: { row: Product }) => {
      // Fall back to line_group_id: the server stores the binding there but
      // older payloads (and the current API response) may not also surface
      // it under cluster_id.
      const id = row.cluster_id || row.line_group_id || ""
      return h("span", { class: "admin-table-muted" }, clusterNameMap.value[id] || id || "-")
    },
  },
  {
    colKey: "quota",
    title: "流量/带宽/连接数",
    minWidth: 220,
    cell: (_h: any, { row }: { row: Product }) => {
      const traffic = bytesToGB(row.monthly_traffic_bytes)
      const bandwidth = bpsToMbps(row.bandwidth_bps)
      const conn = row.conn_limit ?? null
      const t = traffic === null ? "不限" : `${traffic}GB`
      const b = bandwidth === null ? "不限" : `${bandwidth}Mbps`
      const c = conn === null ? "不限" : String(conn)
      return h("span", { class: "admin-table-muted" }, `${t} / ${b} / ${c}`)
    },
  },
  {
    colKey: "domains",
    title: "域名数/主域名数",
    width: 180,
    cell: (_h: any, { row }: { row: Product }) => {
      const d = row.domain_limit ?? null
      const pd = row.primary_domain_limit ?? null
      const a = d === null ? "不限" : String(d)
      const b = pd === null ? "不限" : String(pd)
      return h("span", { class: "admin-table-muted" }, `${a} / ${b}`)
    },
  },
  {
    colKey: "ports",
    title: "HTTP 端口/转发端口",
    width: 180,
    cell: (_h: any, { row }: { row: Product }) => {
      const hp = row.http_port_limit ?? null
      const sp = row.stream_port_limit ?? null
      const a = hp === null ? "不限" : String(hp)
      const b = sp === null ? "不限" : String(sp)
      return h("span", { class: "admin-table-muted" }, `${a} / ${b}`)
    },
  },
  {
    colKey: "websocket",
    title: "Websocket",
    width: 120,
    cell: (_h: any, { row }: { row: Product }) =>
      h(Tag, { theme: row.websocket ? "success" : "default", variant: "light", size: "small" }, () =>
        row.websocket ? "允许" : "禁止"
      ),
  },
  {
    colKey: "price",
    title: "定价",
    width: 220,
    cell: (_h: any, { row }: { row: Product }) => {
      const cur = row.currency || "CNY"
      const m = toYuan(row.price_month_cents ?? row.price_cents ?? 0).toFixed(2)
      const q = toYuan(row.price_quarter_cents ?? 0).toFixed(2)
      const y = toYuan(row.price_year_cents ?? 0).toFixed(2)
      return h("span", { class: "admin-table-muted" }, `月付 ${m} / 季度 ${q} / 年付 ${y} ${cur}`)
    },
  },
  {
    colKey: "slug",
    title: "标识",
    minWidth: 160,
    cell: (_h: any, { row }: { row: Product }) => h("span", { class: "admin-table-muted" }, row.slug || "-"),
  },
  {
    colKey: "description",
    title: "描述",
    minWidth: 240,
    cell: (_h: any, { row }: { row: Product }) => h("span", { class: "admin-table-muted" }, row.description || "-"),
  },
  {
    colKey: "created_at",
    title: "创建时间",
    width: 160,
    cell: (_h: any, { row }: { row: Product }) => h("span", { class: "admin-table-muted" }, formatDate(row.created_at)),
  },
  {
    colKey: "updated_at",
    title: "更新时间",
    width: 160,
    cell: (_h: any, { row }: { row: Product }) => h("span", { class: "admin-table-muted" }, formatDate(row.updated_at)),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 180,
    fixed: "right",
    cell: (_h: any, { row }: { row: Product }) =>
      h("div", { style: "display:flex;gap:6px;flex-wrap:wrap" }, [
        h(Button, { size: "small", variant: "text", onClick: () => openEdit(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => handleDelete(row) }, () => "删除"),
      ]),
  },
])

const groupColumns = computed(() => [
  { colKey: "name", title: "分组名称", minWidth: 200 },
  { colKey: "sort", title: "排序", width: 100 },
  {
    colKey: "description",
    title: "描述",
    minWidth: 240,
    cell: (_h: any, { row }: { row: ProductGroup }) => h("span", { class: "admin-table-muted" }, row.description || "-"),
  },
  {
    colKey: "created_at",
    title: "创建时间",
    width: 180,
    cell: (_h: any, { row }: { row: ProductGroup }) => h("span", { class: "admin-table-muted" }, formatDate(row.created_at)),
  },
  {
    colKey: "updated_at",
    title: "更新时间",
    width: 180,
    cell: (_h: any, { row }: { row: ProductGroup }) => h("span", { class: "admin-table-muted" }, formatDate(row.updated_at)),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 160,
    cell: (_h: any, { row }: { row: ProductGroup }) =>
      h("div", { style: "display:flex;gap:6px" }, [
        h(Button, { size: "small", variant: "text", onClick: () => openGroupDialog(row) }, () => "编辑"),
        h(Button, { size: "small", theme: "danger", variant: "text", onClick: () => deleteGroup(row) }, () => "删除"),
      ]),
  },
])

const pagination = computed(() => ({
  current: page.value,
  pageSize: pageSize.value,
  total: filteredProducts.value.length,
  showJumper: true,
  showPageSize: true,
  pageSizeOptions: [10, 20, 50, 100],
  onChange: (pageInfo: { current: number; pageSize: number }) => {
    page.value = pageInfo.current
    pageSize.value = pageInfo.pageSize
  },
}))

const handleSelectChange = (keys: Array<string | number>) => {
  selectedRowKeys.value = keys
}

onMounted(() => {
  loadData()
})
</script>

<style scoped>
.form-row {
  display: flex;
  align-items: center;
  gap: 12px;
  flex-wrap: wrap;
}
</style>

