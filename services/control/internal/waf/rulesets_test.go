package waf

import "testing"

func TestOWASPCommonRulesCompile(t *testing.T) {
	rs, ok := GetRuleset("owasp_common")
	if !ok {
		t.Fatal("owasp_common ruleset missing")
	}
	if len(rs.Rules) != 5 {
		t.Fatalf("expected 5 rules, got %d", len(rs.Rules))
	}
	for _, rule := range rs.Rules {
		if rule == nil {
			t.Fatal("nil rule in ruleset")
		}
		if rule.Type == "method_block" {
			if len(rule.Methods) != 1 || rule.Methods[0] != "TRACE" {
				t.Fatalf("method_block should target TRACE, got %#v", rule.Methods)
			}
			continue
		}
		if rule.Value == "" {
			t.Fatalf("rule %s missing pattern", rule.Type)
		}
	}
}
