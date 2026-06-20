package dnsprovider

import (
	"fmt"
	"strings"
)

// ManagedRecord is a provider record used by EnsureManagedRecords.
type ManagedRecord struct {
	ID    string
	Value string
	TTL   int64
}

// EnsureResult counts record mutations.
type EnsureResult struct {
	Created, Updated, Deleted int
}

func normalizeDesiredValues(values []string) map[string]struct{} {
	desired := make(map[string]struct{}, len(values))
	for _, v := range values {
		v = strings.Trim(strings.TrimSpace(v), ".")
		if v == "" {
			continue
		}
		desired[v] = struct{}{}
	}
	return desired
}

func EnsureManagedRecords(
	existing []ManagedRecord,
	desired map[string]struct{},
	ttl int64,
	deleteFn func(id string) error,
	updateFn func(id, val string) error,
	createFn func(val string) error,
) (EnsureResult, error) {
	var res EnsureResult
	present := make(map[string]bool, len(desired))

	for _, rec := range existing {
		val := strings.Trim(strings.TrimSpace(rec.Value), ".")
		if _, ok := desired[val]; !ok {
			if err := deleteFn(rec.ID); err != nil {
				return res, err
			}
			res.Deleted++
			continue
		}
		present[val] = true
		if ttl > 0 && rec.TTL != ttl {
			if err := updateFn(rec.ID, val); err != nil {
				return res, err
			}
			res.Updated++
		}
	}

	for val := range desired {
		if present[val] {
			continue
		}
		if err := createFn(val); err != nil {
			return res, err
		}
		res.Created++
	}
	return res, nil
}

func formatEnsureMessage(provider string, recordType RecordType, fqdn string, res EnsureResult) string {
	return fmt.Sprintf("%s ensured %s %s (create=%d update=%d delete=%d)", provider, recordType, fqdn, res.Created, res.Updated, res.Deleted)
}

func syncVerifiedMessage(provider string) string {
	return fmt.Sprintf("%s 凭证校验通过；请使用「同步解析 / 记录修复 / 记录清理」执行完整 DNS 同步", provider)
}
