<template>
  <div class="settings-page">
    <PageHeader title="系统配置" subtitle="通知 / 阈值 / 接入凭据 / 数据保留 等系统级设置" />

    <div class="settings-shell" role="region">
      <!-- Left: section nav -->
      <nav class="settings-nav" aria-label="系统设置分组">
        <div v-for="group in navGroups" :key="group.label" class="settings-nav-group">
          <div class="settings-nav-group-label">{{ group.label }}</div>
          <button
            v-for="item in group.items"
            :key="item.value"
            type="button"
            class="settings-nav-item"
            :class="{ active: activeTab === item.value }"
            @click="activeTab = item.value"
          >
            <span class="settings-nav-icon" :class="`tone-${item.tone}`" aria-hidden="true">
              <t-icon :name="item.icon" />
            </span>
            <span class="settings-nav-label">
              <span class="settings-nav-title">{{ item.label }}</span>
              <span class="settings-nav-desc">{{ item.description }}</span>
            </span>
          </button>
        </div>
      </nav>

      <!-- Right: active panel -->
      <section class="settings-panel" :aria-labelledby="`tab-${activeTab}`">
        <div class="settings-panel-head">
          <h2 :id="`tab-${activeTab}`" class="settings-panel-title">{{ currentTab.label }}</h2>
          <p class="settings-panel-desc">{{ currentTab.description }}</p>
        </div>
        <div class="settings-panel-body">
          <component :is="currentTab.component" />
        </div>
      </section>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, ref, type Component } from "vue"
import PageHeader from "@/components/atoms/PageHeader.vue"
import SiteSettingsTab from "@/views/admin/settings/SiteSettingsTab.vue"
import NotificationSettingsTab from "@/views/admin/settings/NotificationSettingsTab.vue"
import NotificationManagementTab from "@/views/admin/settings/NotificationManagementTab.vue"
import ThresholdSettingsTab from "@/views/admin/settings/ThresholdSettingsTab.vue"
import ApiTokenTab from "@/views/admin/settings/ApiTokenTab.vue"
import DomainBlacklistTab from "@/views/admin/settings/DomainBlacklistTab.vue"
import ElasticsearchSettingsTab from "@/views/admin/settings/ElasticsearchSettingsTab.vue"
import DataRetentionTab from "@/views/admin/settings/DataRetentionTab.vue"

type Tone = "info" | "warning" | "danger" | "success" | "neutral"
interface NavItem {
  value: string
  label: string
  description: string
  icon: string
  tone: Tone
  component: Component
}
interface NavGroup {
  label: string
  items: NavItem[]
}

const navGroups: NavGroup[] = [
  {
    label: "平台基础",
    items: [
      {
        value: "site",
        label: "网站设置",
        description: "品牌外观 / SMTP / 联系邮箱 / 注册策略",
        icon: "internet",
        tone: "info",
        component: SiteSettingsTab,
      },
    ],
  },
  {
    label: "通知告警",
    items: [
      {
        value: "notification",
        label: "通知设置",
        description: "邮件 / 钉钉 / 微信 / 飞书 通道开关与 Webhook",
        icon: "notification",
        tone: "info",
        component: NotificationSettingsTab,
      },
      {
        value: "management",
        label: "通知管理",
        description: "每类告警事件的开启/关闭",
        icon: "notification-filled",
        tone: "info",
        component: NotificationManagementTab,
      },
      {
        value: "threshold",
        label: "阈值设置",
        description: "节点 CPU / 内存 / 磁盘 / 带宽 触发阈值",
        icon: "dashboard-1",
        tone: "warning",
        component: ThresholdSettingsTab,
      },
    ],
  },
  {
    label: "接入与防护",
    items: [
      {
        value: "api",
        label: "API 令牌",
        description: "外部系统调用本控制台 API 的访问凭据",
        icon: "key",
        tone: "success",
        component: ApiTokenTab,
      },
      {
        value: "blacklist",
        label: "域名黑名单",
        description: "禁止用户接入的域名（支持通配符）",
        icon: "system-blocked",
        tone: "danger",
        component: DomainBlacklistTab,
      },
    ],
  },
  {
    label: "数据与日志",
    items: [
      {
        value: "es",
        label: "Elasticsearch",
        description: "访问日志检索 / 数据统计后端",
        icon: "search",
        tone: "info",
        component: ElasticsearchSettingsTab,
      },
      {
        value: "retention",
        label: "数据保留",
        description: "各类日志的自动清理周期",
        icon: "history",
        tone: "neutral",
        component: DataRetentionTab,
      },
    ],
  },
]

const allItems = computed(() => navGroups.flatMap((g) => g.items))
const activeTab = ref("site")
const currentTab = computed(
  () => allItems.value.find((item) => item.value === activeTab.value) || allItems.value[0]
)
</script>

<style scoped>
.settings-page {
  display: flex;
  flex-direction: column;
  gap: var(--app-page-gap);
}

.settings-shell {
  display: grid;
  grid-template-columns: 280px minmax(0, 1fr);
  gap: var(--app-page-gap);
  align-items: stretch;
}

/* ---- Nav (left rail) ---- */
.settings-nav {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: var(--app-card-radius);
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 14px;
  box-shadow: var(--app-shadow-card);
  align-self: flex-start;
  position: sticky;
  top: calc(var(--app-header-height) + 16px);
}

.settings-nav-group {
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.settings-nav-group-label {
  font-size: 11px;
  font-weight: 600;
  color: var(--app-text-faint);
  letter-spacing: 0.4px;
  padding: 4px 8px 6px;
  text-transform: uppercase;
}

.settings-nav-item {
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 10px;
  border-radius: var(--app-input-radius);
  border: 1px solid transparent;
  background: transparent;
  cursor: pointer;
  text-align: left;
  font: inherit;
  color: var(--app-text);
  width: 100%;
  transition: background var(--app-anim-fast) var(--app-easing-standard),
              border-color var(--app-anim-fast) var(--app-easing-standard),
              color var(--app-anim-fast) var(--app-easing-standard);
}

.settings-nav-item:hover {
  background: var(--app-surface-hover);
  color: var(--app-text-strong);
}

.settings-nav-item.active {
  background: var(--app-brand-soft-bg);
  color: var(--app-brand-strong);
  border-color: var(--app-brand-soft-border);
}

.settings-nav-item.active .settings-nav-title {
  color: var(--app-brand-strong);
  font-weight: 600;
}

.settings-nav-icon {
  width: 32px;
  height: 32px;
  border-radius: var(--app-input-radius);
  display: inline-flex;
  align-items: center;
  justify-content: center;
  font-size: 16px;
  flex-shrink: 0;
}

.settings-nav-icon.tone-info {
  background: var(--app-info-soft-bg);
  color: var(--app-info-soft-fg);
}
.settings-nav-icon.tone-warning {
  background: var(--app-warning-soft-bg);
  color: var(--app-warning-soft-fg);
}
.settings-nav-icon.tone-danger {
  background: var(--app-danger-soft-bg);
  color: var(--app-danger-soft-fg);
}
.settings-nav-icon.tone-success {
  background: var(--app-success-soft-bg);
  color: var(--app-success-soft-fg);
}
.settings-nav-icon.tone-neutral {
  background: var(--app-neutral-soft-bg);
  color: var(--app-neutral-soft-fg);
}

.settings-nav-label {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
  flex: 1;
}

.settings-nav-title {
  font-size: 13px;
  font-weight: 500;
  color: var(--app-text-strong);
  line-height: 1.3;
}

.settings-nav-desc {
  font-size: 11px;
  color: var(--app-text-faint);
  line-height: 1.4;
  white-space: normal;
  word-break: break-word;
}

/* ---- Right panel ---- */
.settings-panel {
  background: var(--app-surface);
  border: 1px solid var(--app-border);
  border-radius: var(--app-card-radius);
  box-shadow: var(--app-shadow-card);
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.settings-panel-head {
  padding: 20px 24px 14px;
  border-bottom: 1px solid var(--app-divider);
}

.settings-panel-title {
  font-size: 16px;
  font-weight: 700;
  color: var(--app-text-strong);
  margin: 0;
  letter-spacing: -0.01em;
}

.settings-panel-desc {
  font-size: 12px;
  color: var(--app-text-muted);
  margin: 4px 0 0;
}

.settings-panel-body {
  padding: 20px 24px 24px;
  min-width: 0;
}

/* ---- Mobile: stack the two panes vertically; nav becomes a horizontal
 * scroller so all 7 entries stay reachable without scrolling the page. */
@media (max-width: 960px) {
  .settings-shell {
    grid-template-columns: 1fr;
  }

  .settings-nav {
    position: static;
    flex-direction: row;
    overflow-x: auto;
    -webkit-overflow-scrolling: touch;
    padding: 8px;
    gap: 4px;
  }

  .settings-nav-group {
    flex-direction: row;
    flex-shrink: 0;
    gap: 4px;
  }

  .settings-nav-group-label {
    display: none;
  }

  .settings-nav-item {
    flex-shrink: 0;
    flex-direction: column;
    align-items: center;
    text-align: center;
    min-width: 96px;
    padding: 10px 8px;
  }

  .settings-nav-desc {
    display: none;
  }

  .settings-panel-head {
    padding: 16px 16px 12px;
  }

  .settings-panel-body {
    padding: 16px;
  }
}
</style>
