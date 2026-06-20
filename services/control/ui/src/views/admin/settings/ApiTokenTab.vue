<template>
  <div class="settings-tab">
    <ErrorState v-if="error && tokens.length === 0" :message="error" @retry="loadTokens" />

    <div class="toolbar">
      <el-button type="primary" @click="dialogVisible = true">
        <template #icon><EpIcon name="add" /></template>
        创建新令牌
      </el-button>
    </div>

    <div class="admin-desktop-only">
      <EpDataTable :data="tokens" :columns="columns" row-key="id" :loading="loading" empty-text="暂无 API 令牌" size="small" />
    </div>
    <div class="admin-mobile-only">
      <LoadingState v-if="loading && tokens.length === 0" />
      <div v-else-if="tokens.length === 0" class="admin-mobile-card-empty">暂无 API 令牌</div>
      <div v-else class="admin-mobile-cards">
        <div v-for="token in tokens" :key="token.id" class="admin-mobile-card">
          <div class="admin-mobile-card-header">
            <span class="admin-mobile-card-title"><code>{{ token.token_prefix }}...</code></span>
            <div class="admin-mobile-card-tags">
              <el-tag v-if="!token.expires_at" type="success" effect="light" size="small">永不过期</el-tag>
              <el-tag v-else-if="new Date(token.expires_at) < new Date()" type="danger" effect="light" size="small">已过期</el-tag>
            </div>
          </div>
          <div class="admin-mobile-card-rows">
            <div class="admin-mobile-card-row">
              <span class="admin-mobile-card-label">描述</span>
              <span class="admin-mobile-card-value">{{ token.description || '-' }}</span>
            </div>
            <div v-if="token.expires_at" class="admin-mobile-card-row">
              <span class="admin-mobile-card-label">过期时间</span>
              <span class="admin-mobile-card-value">{{ formatTime(token.expires_at) }}</span>
            </div>
            <div class="admin-mobile-card-row">
              <span class="admin-mobile-card-label">创建时间</span>
              <span class="admin-mobile-card-value">{{ formatTime(token.created_at) }}</span>
            </div>
          </div>
          <div class="admin-mobile-card-actions">
            <el-popconfirm content="确定要删除此令牌吗？删除后无法恢复。" @confirm="handleDelete(token.id)">
              <el-button type="danger" link size="small">删除</el-button>
            </el-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <EpDialog append-to-body
      v-model="dialogVisible"
      :title="newToken ? '令牌创建成功' : '创建 API 令牌'"
      @close="handleCloseDialog"
    >
      <template #footer>
        <el-button v-if="newToken" type="primary" @click="handleCloseDialog">关闭</el-button>
        <template v-else>
          <el-button plain @click="handleCloseDialog">取消</el-button>
          <el-button type="primary" @click="handleCreate" :loading="creating">创建</el-button>
        </template>
      </template>

      <div v-if="newToken">
        <div class="warning-banner" role="alert">
          <EpIcon name="error-circle" />
          <span>请立即复制并保存此令牌，关闭后将无法再次查看。</span>
        </div>
        <div class="token-box">{{ newToken }}</div>
        <el-button type="primary" plain block @click="handleCopyToken">
          <template #icon><EpIcon name="file-copy" /></template>
          复制令牌
        </el-button>
      </div>
      <el-form v-else label-position="top">
        <el-form-item label="令牌描述">
          <el-input v-model="formData.description" placeholder="例如：监控告警系统" />
        </el-form-item>
        <el-form-item label="有效期（天）">
          <el-input-number v-model="formData.ttl_days" :min="0" style="width:100%" />
          <div class="hint">输入 0 表示永不过期</div>
        </el-form-item>
      </el-form>
    </EpDialog>
  </div>
</template>

<script setup lang="ts">
import EpDataTable from "@/components/ep/EpDataTable.vue"
import EpDialog from "@/components/ep/EpDialog.vue"
import EpIcon from "@/components/ep/EpIcon.vue"
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin } from "@/lib/ep-message"
import { ElTag, ElButton, ElPopconfirm } from "element-plus"
import { api, type APIToken } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { formatTime } from "@/lib/time"
import ErrorState from "@/components/common/ErrorState.vue"
import LoadingState from "@/components/common/LoadingState.vue"

const tokens = ref<APIToken[]>([])
const loading = ref(false)
const error = ref("")
const dialogVisible = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)
const formData = ref({ description: "", ttl_days: 0 })

const loadTokens = async () => {
  try {
    loading.value = true
    error.value = ""
    const { tokens: data } = await api.listAPITokens()
    tokens.value = data || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载令牌列表失败"
    error.value = msg
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  if (!formData.value.description.trim()) {
    MessagePlugin.warning("请输入令牌描述")
    return
  }
  creating.value = true
  try {
    const { token } = await api.createAPIToken({
      description: formData.value.description.trim(),
      ttl_days: formData.value.ttl_days > 0 ? formData.value.ttl_days : undefined,
    })
    newToken.value = token
    await loadTokens()
    MessagePlugin.success("令牌创建成功")
  } catch (err: any) {
    MessagePlugin.error(err.message || "创建令牌失败")
  } finally {
    creating.value = false
  }
}

const handleDelete = async (id: string) => {
  try {
    await api.deleteAPIToken(id)
    MessagePlugin.success("令牌已删除")
    await loadTokens()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除令牌失败")
  }
}

const handleCopyToken = async () => {
  if (!newToken.value) return
  const ok = await copyText(newToken.value)
  if (ok) {
    MessagePlugin.success("令牌已复制")
  } else {
    MessagePlugin.error("复制失败，请手动复制")
  }
}

const handleCloseDialog = () => {
  dialogVisible.value = false
  newToken.value = null
  formData.value = { description: "", ttl_days: 0 }
}

const columns = computed(() => [
  {
    colKey: "token_prefix",
    title: "令牌前缀",
    width: 140,
    cell: (_h: any, { row }: { row: APIToken }) => h("code", {}, `${row.token_prefix}...`),
  },
  { colKey: "description", title: "描述", ellipsis: true },
  {
    colKey: "expires_at",
    title: "过期时间",
    width: 180,
    cell: (_h: any, { row }: { row: APIToken }) => {
      if (!row.expires_at) {
        return h(ElTag, { type: "success", effect: "light" }, () => "永不过期")
      }
      const exp = new Date(row.expires_at)
      const now = new Date()
      if (exp < now) {
        return h(ElTag, { type: "danger", effect: "light" }, () => "已过期")
      }
      return formatTime(row.expires_at)
    },
  },
  {
    colKey: "created_at",
    title: "创建时间",
    width: 180,
    cell: (_h: any, { row }: { row: APIToken }) => formatTime(row.created_at),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 120,
    cell: (_h: any, { row }: { row: APIToken }) =>
      h(
        ElPopconfirm,
        { title: "确定要删除此令牌吗？删除后无法恢复。", onConfirm: () => handleDelete(row.id) },
        {
          reference: () =>
            h(ElButton, { type: "danger", link: true, size: "small" }, () => "删除"),
        }
      ),
  },
])

onMounted(() => {
  loadTokens()
})
</script>

