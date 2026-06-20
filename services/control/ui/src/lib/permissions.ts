/**
 * User-group permission keys (must match backend user_group_permissions.go).
 */
export const PermDomainsWrite = "domains:write"
export const PermCertificatesWrite = "certificates:write"
export const PermStreamForwardsWrite = "stream_forwards:write"
export const PermBalanceWithdraw = "balance:withdraw"
export const PermOrdersPurchase = "orders:purchase"
export const PermPurgeExecute = "purge:execute"
export const PermWAFEdit = "waf:edit"

/** User groups are organizational only — do not gate menu/API by these keys. */
export function userPermissionAllowed(
  _permissions: string[] | undefined,
  _perm: string
): boolean {
  return true
}
