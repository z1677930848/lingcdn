<template>
  <div class="settings-tab">
    <div class="toolbar">
      <t-button theme="primary" @click="dialogVisible = true">
        <template #icon><t-icon name="add" /></template>
        创建新令牌
      </t-button>
    </div>

    <div class="admin-desktop-only">
      <t-table :data="tokens" :columns="columns" row-key="id" :loading="loading" empty="暂无 API 令牌" size="small" />
    </div>
    <div class="admin-mobile-only">
      <div v-if="loading" class="loading-row"><t-loading /></div>
      <div v-else-if="tokens.length === 0" class="admin-mobile-card-empty">暂无 API 令牌</div>
      <div v-else class="admin-mobile-cards">
        <div v-for="token in tokens" :key="token.id" class="admin-mobile-card">
          <div class="admin-mobile-card-header">
            <span class="admin-mobile-card-title"><code>{{ token.token_prefix }}...</code></span>
            <div class="admin-mobile-card-tags">
              <t-tag v-if="!token.expires_at" theme="success" variant="light" size="small">永不过期</t-tag>
              <t-tag v-else-if="new Date(token.expires_at) < new Date()" theme="danger" variant="light" size="small">已过期</t-tag>
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
            <t-popconfirm content="确定要删除此令牌吗？删除后无法恢复。" @confirm="handleDelete(token.id)">
              <t-button theme="danger" variant="text" size="small">删除</t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <t-dialog
      v-model:visible="dialogVisible"
      :header="newToken ? '令牌创建成功' : '创建 API 令牌'"
      @close="handleCloseDialog"
    >
      <template #footer>
        <t-button v-if="newToken" theme="primary" @click="handleCloseDialog">关闭</t-button>
        <template v-else>
          <t-button variant="outline" @click="handleCloseDialog">取消</t-button>
          <t-button theme="primary" @click="handleCreate" :loading="creating">创建</t-button>
        </template>
      </template>

      <div v-if="newToken">
        <div class="warning-banner" role="alert">
          <t-icon name="error-circle" />
          <span>请立即复制并保存此令牌，关闭后将无法再次查看。</span>
        </div>
        <div class="token-box">{{ newToken }}</div>
        <t-button theme="primary" variant="outline" block @click="handleCopyToken">
          <template #icon><t-icon name="file-copy" /></template>
          复制令牌
        </t-button>
      </div>
      <t-form v-else layout="vertical" label-align="top">
        <t-form-item label="令牌描述">
          <t-input v-model="formData.description" placeholder="例如：监控告警系统" />
        </t-form-item>
        <t-form-item label="有效期（天）">
          <t-input-number v-model="formData.ttl_days" :min="0" style="width:100%" />
          <div class="hint">输入 0 表示永不过期</div>
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin, Tag, Button } from "tdesign-vue-next"
import { api, type APIToken } from "@/lib/api"
import { copyText } from "@/lib/clipboard"
import { formatTime } from "@/lib/time"

const tokens = ref<APIToken[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const creating = ref(false)
const newToken = ref<string | null>(null)
const formData = ref({ description: "", ttl_days: 0 })

const loadTokens = async () => {
  try {
    loading.value = true
    const { tokens: data } = await api.listAPITokens()
    tokens.value = data || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载令牌列表失败")
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
        return h(Tag, { theme: "success", variant: "light" }, () => "永不过期")
      }
      const exp = new Date(row.expires_at)
      const now = new Date()
      if (exp < now) {
        return h(Tag, { theme: "danger", variant: "light" }, () => "已过期")
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
        "t-popconfirm",
        { content: "确定要删除此令牌吗？删除后无法恢复。", onConfirm: () => handleDelete(row.id) },
        {
          default: () =>
            h(Button, { theme: "danger", variant: "text", size: "small" }, () => "删除"),
        }
      ),
  },
])

onMounted(() => {
  loadTokens()
})
</script>

<style scoped>
.toolbar {
  margin-bottom: 16px;
}

.loading-row {
  text-align: center;
  padding: 32px 0;
}

.warning-banner {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 12px;
  padding: 10px 14px;
  border-radius: var(--app-input-radius);
  background: var(--app-warning-soft-bg);
  border: 1px solid var(--td-warning-color-2);
  color: var(--app-warning-soft-fg);
  font-size: 13px;
  font-weight: 500;
}

.token-box {
  background: var(--app-surface-muted);
  border: 1px dashed var(--app-border-strong);
  padding: 14px 16px;
  border-radius: var(--app-input-radius);
  word-break: break-all;
  font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  font-size: 13px;
  color: var(--app-text-strong);
  margin-bottom: 12px;
}

.hint {
  font-size: 12px;
  color: var(--app-text-faint);
  margin-top: 6px;
}
</style>

