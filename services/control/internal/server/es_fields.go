package server

import (
	"strings"

	"github.com/lingcdn/control/internal/store"
)

// resolveESDomainField returns the ES field used for domain term/terms filters.
// The pushed index template maps "domain" as keyword directly; legacy configs
// incorrectly used "domain.keyword" which returns zero hits after templates apply.
func resolveESDomainField(settings *store.Settings) string {
	field := "domain"
	if settings != nil {
		if v := strings.TrimSpace(settings.ElasticsearchDomainField); v != "" {
			field = v
		}
	}
	if field == "domain.keyword" {
		return "domain"
	}
	return field
}

// resolveESClientIPField returns the ES field for client IP filters.
// The index template maps client_ip as type ip, not text+keyword.
func resolveESClientIPField() string {
	return "client_ip"
}
