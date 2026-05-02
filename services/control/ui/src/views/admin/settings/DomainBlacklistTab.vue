<template>
  <div class="settings-tab">
    <p class="desc">添加到黑名单的域名将无法被用户添加。支持通配符，例如：<code>*.example.com</code></p>

    <div class="toolbar">
      <t-button theme="primary" @click="dialogVisible = true">
        <template #icon><t-icon name="add" /></template>
        添加黑名单
      </t-button>
    </div>

    <div class="admin-desktop-only">
      <t-table :data="list" :columns="columns" row-key="id" :loading="loading" empty="暂无黑名单记录" size="small" />
    </div>
    <div class="admin-mobile-only">
      <div v-if="loading" class="loading-row"><t-loading /></div>
      <div v-else-if="list.length === 0" class="admin-mobile-card-empty">暂无黑名单记录</div>
      <div v-else class="admin-mobile-cards">
        <div v-for="item in list" :key="item.id" class="admin-mobile-card">
          <div class="admin-mobile-card-header">
            <span class="admin-mobile-card-title">{{ item.domain }}</span>
          </div>
          <div class="admin-mobile-card-rows">
            <div class="admin-mobile-card-row">
              <span class="admin-mobile-card-label">原因</span>
              <span class="admin-mobile-card-value">{{ item.reason || '-' }}</span>
            </div>
            <div class="admin-mobile-card-row">
              <span class="admin-mobile-card-label">添加时间</span>
              <span class="admin-mobile-card-value">{{ formatTime(item.created_at) }}</span>
            </div>
          </div>
          <div class="admin-mobile-card-actions">
            <t-popconfirm content="确定要删除此黑名单记录吗？" @confirm="handleDelete(item.id)">
              <t-button theme="danger" variant="text" size="small">删除</t-button>
            </t-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <t-dialog v-model:visible="dialogVisible" header="添加域名黑名单">
      <template #footer>
        <t-button variant="outline" @click="dialogVisible = false">取消</t-button>
        <t-button theme="primary" @click="handleCreate" :loading="creating">添加</t-button>
      </template>
      <t-form layout="vertical" label-align="top">
        <t-form-item label="域名">
          <t-input v-model="formData.domain" placeholder="example.com 或 *.example.com" />
        </t-form-item>
        <t-form-item label="原因（可选）">
          <t-input v-model="formData.reason" />
        </t-form-item>
      </t-form>
    </t-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, h, onMounted, ref } from "vue"
import { MessagePlugin, Button } from "tdesign-vue-next"
import { api, type DomainBlacklist } from "@/lib/api"
import { formatTime } from "@/lib/time"

const list = ref<DomainBlacklist[]>([])
const loading = ref(false)
const dialogVisible = ref(false)
const creating = ref(false)
const formData = ref({ domain: "", reason: "" })

const loadList = async () => {
  try {
    loading.value = true
    const { blacklist } = await api.listDomainBlacklist()
    list.value = blacklist || []
  } catch (err: any) {
    MessagePlugin.error(err.message || "加载黑名单失败")
  } finally {
    loading.value = false
  }
}

const handleCreate = async () => {
  if (!formData.value.domain.trim()) {
    MessagePlugin.warning("请输入域名")
    return
  }
  creating.value = true
  try {
    await api.createDomainBlacklist({
      domain: formData.value.domain.trim(),
      reason: formData.value.reason.trim(),
    })
    MessagePlugin.success("添加成功")
    dialogVisible.value = false
    formData.value = { domain: "", reason: "" }
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "添加失败")
  } finally {
    creating.value = false
  }
}

const handleDelete = async (id: string) => {
  try {
    await api.deleteDomainBlacklist(id)
    MessagePlugin.success("删除成功")
    await loadList()
  } catch (err: any) {
    MessagePlugin.error(err.message || "删除失败")
  }
}

const columns = computed(() => [
  { colKey: "domain", title: "域名", ellipsis: true },
  {
    colKey: "reason",
    title: "原因",
    ellipsis: true,
    cell: (_h: any, { row }: { row: DomainBlacklist }) => row.reason || "-",
  },
  {
    colKey: "created_at",
    title: "添加时间",
    width: 180,
    cell: (_h: any, { row }: { row: DomainBlacklist }) => formatTime(row.created_at),
  },
  {
    colKey: "actions",
    title: "操作",
    width: 120,
    cell: (_h: any, { row }: { row: DomainBlacklist }) =>
      h(
        "t-popconfirm",
        { content: "确定要删除此黑名单记录吗？", onConfirm: () => handleDelete(row.id) },
        {
          default: () =>
            h(Button, { theme: "danger", variant: "text", size: "small" }, () => "删除"),
        }
      ),
  },
])

onMounted(() => {
  loadList()
})
</script>

<style scoped>
.desc {
  color: var(--app-text-muted);
  font-size: 13px;
  margin: 0 0 16px;
  line-height: 1.6;
}

.desc code {
  background: var(--app-surface-muted);
  border: 1px solid var(--app-border);
  padding: 1px 6px;
  border-radius: 4px;
  font-size: 12px;
  color: var(--app-text-strong);
}

.toolbar {
  margin-bottom: 16px;
}

.loading-row {
  text-align: center;
  padding: 32px 0;
}
</style>

