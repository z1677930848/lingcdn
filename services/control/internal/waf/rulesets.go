package waf

import (
	"github.com/google/uuid"

	"github.com/lingcdn/control/internal/store"
)

// Ruleset is a named collection of WAF rules for one-click apply.
type Ruleset struct {
	Name        string
	Description string
	Rules       []*store.WAFRule
}

// ListRulesets returns built-in OWASP-style rulesets.
func ListRulesets() []Ruleset {
	return []Ruleset{
		{
			Name:        "owasp_common",
			Description: "OWASP common protections: SQLi, XSS, path traversal, scanner UA",
			Rules:       owaspCommonRules(),
		},
	}
}

// GetRuleset returns a ruleset by name.
func GetRuleset(name string) (Ruleset, bool) {
	for _, rs := range ListRulesets() {
		if rs.Name == name {
			return rs, true
		}
	}
	return Ruleset{}, false
}

func owaspCommonRules() []*store.WAFRule {
	mk := func(ruleType, value, note string, priority int32) *store.WAFRule {
		return &store.WAFRule{
			ID:       uuid.NewString(),
			Type:     ruleType,
			Action:   "deny",
			Value:    value,
			Note:     note,
			Priority: priority,
			Enabled:  true,
		}
	}
	rules := []*store.WAFRule{
		mk("sql_injection", `(?i)(union\s+select|select\s+.+\s+from|insert\s+into|drop\s+table|update\s+.+\s+set|or\s+1\s*=\s*1|'\s*or\s*'1)`, "SQL injection patterns", 10),
		mk("xss", `(?i)(<script|javascript:|onerror\s*=|onload\s*=|document\.cookie)`, "Cross-site scripting patterns", 20),
		mk("path_traversal", `(?i)(\.\./|\.\.\\|%2e%2e%2f|%2e%2e/)`, "Path traversal attempts", 30),
		mk("ua_block", `(?i)(sqlmap|nikto|nmap|masscan|acunetix|nessus|dirbuster)`, "Known scanner user agents", 40),
	}
	traceBlock := mk("", "", "Block TRACE method", 50)
	traceBlock.Type = "method_block"
	traceBlock.Methods = []string{"TRACE"}
	rules = append(rules, traceBlock)
	return rules
}
