import { computed } from "vue"
import { useAuthStore } from "@/stores/auth"
import {
  PermBalanceWithdraw,
  PermCertificatesWrite,
  PermDomainsWrite,
  PermOrdersPurchase,
  PermPurgeExecute,
  PermStreamForwardsWrite,
  PermWAFEdit,
  userPermissionAllowed,
} from "@/lib/permissions"

/** User-group permission helpers for dashboard write/read-only UI. */
export function useUserPermissions() {
  const auth = useAuthStore()
  const perms = computed(() => auth.user?.permissions)
  const isAdmin = computed(() => auth.user?.role === "admin")

  const can = (perm: string) => isAdmin.value || userPermissionAllowed(perms.value, perm)

  const hasRestrictions = computed(() => !isAdmin.value && Array.isArray(perms.value) && perms.value.length > 0)

  return {
    isAdmin,
    hasRestrictions,
    can,
    canDomainsWrite: computed(() => can(PermDomainsWrite)),
    canCertificatesWrite: computed(() => can(PermCertificatesWrite)),
    canStreamForwardsWrite: computed(() => can(PermStreamForwardsWrite)),
    canBalanceWithdraw: computed(() => can(PermBalanceWithdraw)),
    canOrdersPurchase: computed(() => can(PermOrdersPurchase)),
    canPurgeExecute: computed(() => can(PermPurgeExecute)),
    canWAFEdit: computed(() => can(PermWAFEdit)),
  }
}
