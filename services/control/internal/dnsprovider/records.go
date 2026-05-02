package dnsprovider

import (
	"fmt"
	"net"
	"strings"
)

type RecordType string

const (
	RecordTypeA     RecordType = "A"
	RecordTypeAAAA  RecordType = "AAAA"
	RecordTypeCNAME RecordType = "CNAME"
)

// DNSRecord represents a flattened DNS record.
type DNSRecord struct {
	Name  string
	Type  RecordType
	Value string
	TTL   int64
	Line  string
}

func NormalizeZone(zone string) string {
	return strings.Trim(strings.TrimSpace(zone), ".")
}

func NormalizeName(name string) string {
	name = strings.Trim(strings.TrimSpace(name), ".")
	if name == "" {
		return "@"
	}
	return name
}

func JoinFQDN(name, zone string) string {
	zone = NormalizeZone(zone)
	name = NormalizeName(name)
	if name == "@" {
		return zone
	}
	return name + "." + zone
}

func SplitByZone(fqdn, zone string) (string, bool) {
	fqdn = strings.Trim(strings.TrimSpace(fqdn), ".")
	zone = NormalizeZone(zone)
	if fqdn == "" || zone == "" {
		return "", false
	}
	if strings.EqualFold(fqdn, zone) {
		return "@", true
	}
	suffix := "." + zone
	if !strings.HasSuffix(strings.ToLower(fqdn), strings.ToLower(suffix)) {
		return "", false
	}
	rr := strings.TrimSuffix(fqdn, suffix)
	rr = strings.Trim(rr, ".")
	if rr == "" {
		return "@", true
	}
	return rr, true
}

func validateRecordValues(recordType RecordType, values []string) error {
	switch recordType {
	case RecordTypeA, RecordTypeAAAA:
		for _, v := range values {
			ip := net.ParseIP(strings.TrimSpace(v))
			if ip == nil {
				return fmt.Errorf("invalid IP value: %s", v)
			}
			if recordType == RecordTypeA && ip.To4() == nil {
				return fmt.Errorf("expected IPv4 but got: %s", v)
			}
			if recordType == RecordTypeAAAA && ip.To4() != nil {
				return fmt.Errorf("expected IPv6 but got: %s", v)
			}
		}
		return nil
	case RecordTypeCNAME:
		if len(values) > 1 {
			return fmt.Errorf("CNAME expects 0 or 1 value, got %d", len(values))
		}
		if len(values) == 0 {
			return nil
		}
		v := strings.Trim(strings.TrimSpace(values[0]), ".")
		if v == "" {
			return fmt.Errorf("CNAME value required")
		}
		return nil
	default:
		return fmt.Errorf("unsupported record type: %s", recordType)
	}
}
