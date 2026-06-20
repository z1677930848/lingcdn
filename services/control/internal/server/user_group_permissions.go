package server

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

const (
	PermDomainsWrite        = "domains:write"
	PermCertificatesWrite   = "certificates:write"
	PermStreamForwardsWrite = "stream_forwards:write"
	PermBalanceWithdraw     = "balance:withdraw"
	PermOrdersPurchase      = "orders:purchase"
	PermPurgeExecute        = "purge:execute"
	PermWAFEdit             = "waf:edit"
)

type userGroupPermissionDef struct {
	Key   string `json:"key"`
	Label string `json:"label"`
}

var userGroupPermissionCatalog = []userGroupPermissionDef{
	{Key: PermDomainsWrite, Label: "域名管理"},
	{Key: PermCertificatesWrite, Label: "证书管理"},
	{Key: PermStreamForwardsWrite, Label: "四层转发"},
	{Key: PermBalanceWithdraw, Label: "余额提现"},
	{Key: PermOrdersPurchase, Label: "购买套餐"},
	{Key: PermPurgeExecute, Label: "缓存刷新"},
	{Key: PermWAFEdit, Label: "WAF 规则"},
}

func normalizeUserGroupPermissions(perms []string) []string {
	if len(perms) == 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(userGroupPermissionCatalog))
	for _, d := range userGroupPermissionCatalog {
		allowed[d.Key] = struct{}{}
	}
	out := make([]string, 0, len(perms))
	seen := make(map[string]struct{}, len(perms))
	for _, raw := range perms {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		if _, ok := allowed[p]; !ok {
			continue
		}
		if _, dup := seen[p]; dup {
			continue
		}
		seen[p] = struct{}{}
		out = append(out, p)
	}
	slices.Sort(out)
	return out
}

func userPermissionAllowed(perms []string, perm string) bool {
	_ = perms
	_ = perm
	// User groups are organizational only; normal users retain full product access.
	return true
}

func (s *Servers) requireUserPermission(w http.ResponseWriter, ctx context.Context, perm string) bool {
	if isAdmin(ctx) {
		return true
	}
	perms, _ := ctx.Value(ctxKeyPermissions).([]string)
	if userPermissionAllowed(perms, perm) {
		return true
	}
	writeJSON(w, http.StatusForbidden, map[string]any{"error": "当前用户分组无此操作权限"})
	return false
}
