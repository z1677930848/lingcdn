import type { Component } from "vue"
import SiteSettingsTab from "@/views/admin/settings/SiteSettingsTab.vue"
import ControlDomainSettingsTab from "@/views/admin/settings/ControlDomainSettingsTab.vue"
import NotificationSettingsTab from "@/views/admin/settings/NotificationSettingsTab.vue"
import NotificationManagementTab from "@/views/admin/settings/NotificationManagementTab.vue"
import ThresholdSettingsTab from "@/views/admin/settings/ThresholdSettingsTab.vue"
import ApiTokenTab from "@/views/admin/settings/ApiTokenTab.vue"
import DomainBlacklistTab from "@/views/admin/settings/DomainBlacklistTab.vue"
import ElasticsearchSettingsTab from "@/views/admin/settings/ElasticsearchSettingsTab.vue"
import DataRetentionTab from "@/views/admin/settings/DataRetentionTab.vue"

export type AdminSettingsNavItem = {
  tab: string
  label: string
  description: string
  href: string
  component: Component
}

const SETTINGS_BASE = "/admin/dashboard/settings"

export const ADMIN_SETTINGS_NAV_ITEMS: AdminSettingsNavItem[] = [
  {
    tab: "site",
    label: "网站设置",
    description: "品牌外观 / SMTP / 联系邮箱 / 注册策略",
    href: `${SETTINGS_BASE}/site`,
    component: SiteSettingsTab,
  },
  {
    tab: "control-domain",
    label: "主控域名",
    description: "面板对外域名 / DNS 校验 / 节点安装地址",
    href: `${SETTINGS_BASE}/control-domain`,
    component: ControlDomainSettingsTab,
  },
  {
    tab: "notification",
    label: "通知设置",
    description: "邮件 / 钉钉 / 微信 / 飞书 通道开关与 Webhook",
    href: `${SETTINGS_BASE}/notification`,
    component: NotificationSettingsTab,
  },
  {
    tab: "management",
    label: "通知管理",
    description: "每类告警事件的开启/关闭",
    href: `${SETTINGS_BASE}/management`,
    component: NotificationManagementTab,
  },
  {
    tab: "threshold",
    label: "阈值设置",
    description: "节点 CPU / 内存 / 磁盘 / 带宽 触发阈值",
    href: `${SETTINGS_BASE}/threshold`,
    component: ThresholdSettingsTab,
  },
  {
    tab: "api",
    label: "API 令牌",
    description: "外部系统调用本控制台 API 的访问凭据",
    href: `${SETTINGS_BASE}/api`,
    component: ApiTokenTab,
  },
  {
    tab: "blacklist",
    label: "域名黑名单",
    description: "禁止用户接入的域名（支持通配符）",
    href: `${SETTINGS_BASE}/blacklist`,
    component: DomainBlacklistTab,
  },
  {
    tab: "es",
    label: "Elasticsearch",
    description: "访问日志检索 / 数据统计后端",
    href: `${SETTINGS_BASE}/es`,
    component: ElasticsearchSettingsTab,
  },
  {
    tab: "retention",
    label: "数据保留",
    description: "各类日志的自动清理周期",
    href: `${SETTINGS_BASE}/retention`,
    component: DataRetentionTab,
  },
]

export function resolveAdminSettingsTab(tab: string | undefined): AdminSettingsNavItem {
  const key = String(tab || "").trim()
  return ADMIN_SETTINGS_NAV_ITEMS.find((item) => item.tab === key) || ADMIN_SETTINGS_NAV_ITEMS[0]
}

export function isValidAdminSettingsTab(tab: string | undefined): boolean {
  const key = String(tab || "").trim()
  return ADMIN_SETTINGS_NAV_ITEMS.some((item) => item.tab === key)
}
