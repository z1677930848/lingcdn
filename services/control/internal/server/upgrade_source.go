package server

import "strings"

// Keep the official auth portal fixed unless a test overrides it explicitly.
const authPortalBase = "https://auth.lingcdn.cloud"

// hardcodedUpgradePortalBase remains overrideable for tests.
var hardcodedUpgradePortalBase = authPortalBase

func normalizePortalBase(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if !strings.Contains(raw, "://") && !strings.HasPrefix(raw, "/") {
		raw = "https://" + raw
	}
	return strings.TrimRight(raw, "/")
}

func (s *Servers) portalBase() string {
	if base := normalizePortalBase(hardcodedUpgradePortalBase); base != "" {
		return base
	}
	return authPortalBase
}

func (s *Servers) upgradePortalBase() string {
	return s.portalBase()
}
