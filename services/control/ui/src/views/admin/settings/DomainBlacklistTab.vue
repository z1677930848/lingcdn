<template>
  <div class="settings-tab">
    <p class="tab-desc">添加到黑名单的域名将无法被用户添加。支持通配符，例如：<code>*.example.com</code></p>

    <ErrorState v-if="error && list.length === 0" :message="error" @retry="loadList" />

    <div class="toolbar">
      <el-button type="primary" @click="dialogVisible = true">
        <template #icon><EpIcon name="add" /></template>
        添加黑名单
      </el-button>
    </div>

    <div class="admin-desktop-only">
      <EpDataTable :data="list" :columns="columns" row-key="id" :loading="loading" empty-text="暂无黑名单记录" size="small" />
    </div>
    <div class="admin-mobile-only">
      <LoadingState v-if="loading && list.length === 0" />
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
            <el-popconfirm content="确定要删除此黑名单记录吗？" @confirm="handleDelete(item.id)">
              <el-button type="danger" link size="small">删除</el-button>
            </el-popconfirm>
          </div>
        </div>
      </div>
    </div>

    <EpDialog append-to-body v-model="dialogVisible" header="添加域名黑名单">
      <template #footer>
        <el-button plain @click="dialogVisible = false">取消</el-button>
        <el-button type="primary" @click="handleCreate" :loading="creating">添加</el-button>
      </template>
      <el-form label-position="top">
        <el-form-item label="域名">
          <el-input v-model="formData.domain" placeholder="example.com 或 *.example.com" />
        </el-form-item>
        <el-form-item label="原因（可选）">
          <el-input v-model="formData.reason" />
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
import { ElButton, ElPopconfirm } from "element-plus"
import { api, type DomainBlacklist } from "@/lib/api"
import { formatTime } from "@/lib/time"
import ErrorState from "@/components/common/ErrorState.vue"
import LoadingState from "@/components/common/LoadingState.vue"

const list = ref<DomainBlacklist[]>([])
const loading = ref(false)
const error = ref("")
const dialogVisible = ref(false)
const creating = ref(false)
const formData = ref({ domain: "", reason: "" })

const loadList = async () => {
  try {
    loading.value = true
    error.value = ""
    const { blacklist } = await api.listDomainBlacklist()
    list.value = blacklist || []
  } catch (err: unknown) {
    const msg = err instanceof Error ? err.message : "加载黑名单失败"
    error.value = msg
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
        ElPopconfirm,
        { title: "确定要删除此黑名单记录吗？", onConfirm: () => handleDelete(row.id) },
        {
          reference: () =>
            h(ElButton, { type: "danger", link: true, size: "small" }, () => "删除"),
        }
      ),
  },
])

onMounted(() => {
  loadList()
})
</script>

