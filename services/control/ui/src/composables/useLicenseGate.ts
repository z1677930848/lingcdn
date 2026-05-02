import { computed, ref } from "vue"
import { api, type SystemLicenseState } from "@/lib/api"

const RAW_NOT_FOUND_PATTERNS = ["not found", "no license", "license required"]

const STATUS_LABEL: Record<string, string> = {
  unlicensed: "系统未授权",
  expired: "系统授权已过期",
  revoked: "系统授权已撤销",
  limited: "系统授权受限",
  paused: "系统授权已暂停，请重新授权",
  suspended: "系统授权已暂停，请重新授权",
}

const BLOCKING_STATUSES = new Set(["", "unlicensed", "revoked", "paused", "suspended"])
const WARNING_STATUSES = new Set(["expired", "limited"])

function resolveLicenseNotice(status: string, reason?: string): string {
  const raw = String(reason || "").trim().toLowerCase()
  if (RAW_NOT_FOUND_PATTERNS.some((needle) => raw.includes(needle))) {
    return "系统未授权"
  }
  const nodeMatch = raw.match(/node limit\D+\((\d+)\/(\d+)\)/)
  if (nodeMatch) {
    return `系统授权受限：节点数已达上限（${nodeMatch[1]}/${nodeMatch[2]}）`
  }
  return STATUS_LABEL[String(status || "").toLowerCase()] || "系统未授权"
}

/**
 * License-gate state shared between AdminLayout and UserLayout.
 *
 * `shouldFetch` decides whether the current user should query the license
 * endpoint at all (admin vs. regular user have different gating semantics —
 * admins always check; regular users only check when an admin user exists).
 */
export function useLicenseGate(shouldFetch: () => boolean) {
  const license = ref<SystemLicenseState | null>(null)
  const loading = ref(true)

  const statusCode = computed(() => String(license.value?.status || "").toLowerCase())
  const blocked = computed(() => !!license.value && BLOCKING_STATUSES.has(statusCode.value))
  const warning = computed(() => !!license.value && WARNING_STATUSES.has(statusCode.value))
  const notice = computed(() => resolveLicenseNotice(statusCode.value, license.value?.reason))

  const fetchStatus = async () => {
    if (!shouldFetch()) {
      loading.value = false
      return
    }
    loading.value = true
    try {
      const res = await api.getSystemLicenseStatus()
      license.value = res.license
    } catch {
      // Soft-fail: if the endpoint is unreachable, leave previous state.
    } finally {
      loading.value = false
    }
  }

  return {
    license,
    loading,
    statusCode,
    blocked,
    warning,
    notice,
    fetchStatus,
  }
}
