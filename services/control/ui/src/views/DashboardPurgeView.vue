<template>
  <div class="page">
    <div class="header">
      <div>
        <h1 class="title">缓存刷新</h1>
        <p class="subtitle">快速刷新 CDN 缓存，使源站最新内容生效</p>
      </div>
    </div>

    <t-card class="section-card" bordered>
      <div class="section-body">
        <t-tabs v-model="purgeType">
          <t-tab-panel value="url" label="URL 刷新" />
          <t-tab-panel value="dir" label="目录刷新" />
        </t-tabs>

        <div class="purge-form">
          <div class="form-tip" v-if="purgeType === 'url'">
            请输入需要刷新的 URL，每行一个（含 http:// 或 https://），最多 20 条。
          </div>
          <div class="form-tip" v-else>
            请输入需要刷新的目录路径，每行一个（如 http://example.com/images/），最多 10 条。
          </div>
          <t-textarea
            v-model="purgeInput"
            :placeholder="purgeType === 'url' ? 'https://example.com/path/file.js\nhttps://example.com/path/file.css' : 'https://example.com/images/\nhttps://example.com/static/'"
            :rows="8"
            :maxlength="purgeType === 'url' ? 5000 : 2000"
          />
          <div class="purge-actions">
            <t-button theme="primary" :loading="purging" @click="handlePurge">
              提交刷新
            </t-button>
            <span class="muted">{{ urlCount }} 条</span>
          </div>
        </div>

        <div v-if="purgeHistory.length > 0" class="purge-history">
          <div class="purge-history-title">最近提交</div>
          <div v-for="item in purgeHistory" :key="item.id" class="purge-history-item">
            <div class="purge-history-main">
              <t-tag :theme="item.ok ? 'success' : 'danger'" variant="light" size="small">
                {{ item.ok ? "已提交" : "失败" }}
              </t-tag>
              <span class="muted" :title="formatTimeFull(item.time)">{{ formatTime(item.time) }}</span>
            </div>
            <div class="purge-history-urls">{{ item.urls.slice(0, 3).join(", ") }}{{ item.urls.length > 3 ? ` ...共${item.urls.length}条` : "" }}</div>
          </div>
        </div>
      </div>
    </t-card>
  </div>
</template>

<script setup lang="ts">
import { computed, ref } from "vue"
import { DialogPlugin, MessagePlugin } from "tdesign-vue-next"
import { api } from "@/lib/api"
import { formatTime, formatTimeFull } from "@/lib/time"

const purgeType = ref<"url" | "dir">("url")
const purgeInput = ref("")
const purging = ref(false)

interface PurgeRecord {
  id: string
  ok: boolean
  urls: string[]
  time: string
}
const purgeHistory = ref<PurgeRecord[]>([])

const urlCount = computed(() => {
  const lines = purgeInput.value.trim().split("\n").filter((l) => l.trim())
  return lines.length
})

const handlePurge = async () => {
  const lines = purgeInput.value
    .trim()
    .split("\n")
    .map((l) => l.trim())
    .filter(Boolean)

  if (lines.length === 0) {
    MessagePlugin.warning("请输入需要刷新的 URL")
    return
  }

  const maxCount = purgeType.value === "url" ? 20 : 10
  if (lines.length > maxCount) {
    MessagePlugin.warning(`最多提交 ${maxCount} 条`)
    return
  }

  // If directory purge, ensure trailing slash
  const urls = purgeType.value === "dir"
    ? lines.map((l) => (l.endsWith("/") ? l : l + "/"))
    : lines

  // Purging is a globally destructive cache op: every edge node drops
  // the listed entries, causing a traffic spike toward the origin. Ask
  // once before we fire, and surface the count + type so users see what
  // they're actually triggering.
  const kindLabel = purgeType.value === "dir" ? "目录" : "URL"
  const previewMax = 3
  const preview = urls.slice(0, previewMax).map((u) => `• ${u}`).join("\n")
  const tail = urls.length > previewMax ? `\n……共 ${urls.length} 条` : ""
  const inst = DialogPlugin.confirm({
    header: "提交缓存刷新",
    body: `即将刷新 ${urls.length} 个${kindLabel}，所有边缘节点会清除对应缓存并回源站取最新内容，可能造成短时源站压力上升。\n\n${preview}${tail}`,
    theme: "warning",
    confirmBtn: { content: "确认刷新", theme: "primary" },
    cancelBtn: "取消",
    onConfirm: async () => {
      inst.destroy()
      await submitPurge(urls)
    },
    onClose: () => inst.destroy(),
  })
}

const submitPurge = async (urls: string[]) => {
  purging.value = true
  try {
    const res = await api.purgeURLs(urls)
    if (res.ok) {
      MessagePlugin.success("缓存刷新请求已提交")
      purgeHistory.value.unshift({
        id: res.request_id || String(Date.now()),
        ok: true,
        urls,
        time: new Date().toISOString(),
      })
      purgeInput.value = ""
    } else {
      MessagePlugin.error(res.message || "提交失败")
      purgeHistory.value.unshift({
        id: String(Date.now()),
        ok: false,
        urls,
        time: new Date().toISOString(),
      })
    }
  } catch (err: any) {
    MessagePlugin.error(err.message || "提交缓存刷新失败")
    purgeHistory.value.unshift({
      id: String(Date.now()),
      ok: false,
      urls,
      time: new Date().toLocaleString("zh-CN"),
    })
  } finally {
    purging.value = false
  }
}
</script>

<style scoped>
.section-body {
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.purge-form {
  display: flex;
  flex-direction: column;
  gap: 12px;
  max-width: 720px;
}

.form-tip {
  font-size: 13px;
  color: #94a3b8;
}

.purge-actions {
  display: flex;
  align-items: center;
  gap: 12px;
}

.muted {
  font-size: 12px;
  color: #94a3b8;
}

.purge-history {
  border-top: 1px solid #f1f5f9;
  padding-top: 16px;
}

.purge-history-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 12px;
}

.purge-history-item {
  padding: 10px 0;
  border-bottom: 1px solid #f8fafc;
}

.purge-history-main {
  display: flex;
  align-items: center;
  gap: 8px;
  margin-bottom: 4px;
}

.purge-history-urls {
  font-size: 13px;
  color: #64748b;
  word-break: break-all;
}
</style>
