import type { Component } from "vue"
import {
  AddLocation,
  ArrowDown,
  ArrowLeft,
  CircleClose,
  CopyDocument,
  DataBoard,
  Download,
  Grid,
  Link,
  Lock,
  Message,
  Monitor,
  Plus,
  Refresh,
  Search,
  Setting,
  Sort,
  Tools,
  User,
  Wallet,
} from "@element-plus/icons-vue"

/** Map legacy TDesign icon names to Element Plus icon components. */
export const EP_ICON_MAP: Record<string, Component> = {
  add: Plus,
  refresh: Refresh,
  user: User,
  "lock-on": Lock,
  secured: Lock,
  "error-circle": CircleClose,
  "file-copy": CopyDocument,
  download: Download,
  "chevron-down": ArrowDown,
  "chevron-left": ArrowLeft,
  "arrow-left": ArrowLeft,
  search: Search,
  mail: Message,
  dashboard: DataBoard,
  server: Monitor,
  internet: Link,
  swap: Sort,
  setting: Setting,
  app: Grid,
  wallet: Wallet,
  tools: Tools,
  link: Link,
  notification: Message,
  "notification-filled": Message,
  "dashboard-1": DataBoard,
  key: Lock,
  "system-blocked": CircleClose,
  history: Refresh,
}

export function resolveEpIcon(name?: string): Component | undefined {
  if (!name) return undefined
  return EP_ICON_MAP[name] || EP_ICON_MAP[name.toLowerCase()]
}
