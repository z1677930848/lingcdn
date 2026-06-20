package server

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	wafrules "github.com/lingcdn/control/internal/waf"
)

func TestReplaceWAFRulesAcceptsOWASPRuleset(t *testing.T) {
	s := testServers(t)
	ctx := context.Background()
	rs, ok := wafrules.GetRuleset("owasp_common")
	if !ok {
		t.Fatal("owasp_common ruleset missing")
	}
	if err := s.replaceWAFRules(ctx, "policy-1", rs.Rules, time.Now()); err != nil {
		t.Fatalf("replace OWASP rules: %v", err)
	}
}

func TestIsValidWAFRuleTypeIncludesOWASP(t *testing.T) {
	for _, typ := range []string{
		"sql_injection", "xss", "path_traversal", "ua_block", "method_block",
	} {
		if !isValidWAFRuleType(typ) {
			t.Fatalf("expected valid rule type %q", typ)
		}
	}
}

func TestHandleWAFRulesetApplyOWASPCommon(t *testing.T) {
	s := testServers(t)

	req := httptest.NewRequest(http.MethodPost, "/api/waf/rulesets/owasp_common", strings.NewReader("{}"))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	s.handleWAFRulesetApply(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["ok"] != true {
		t.Fatalf("expected ok=true, got %#v", resp["ok"])
	}
	if resp["ruleset"] != "owasp_common" {
		t.Fatalf("unexpected ruleset: %#v", resp["ruleset"])
	}
	pol, ok := resp["policy"].(map[string]any)
	if !ok {
		t.Fatalf("missing policy in response: %#v", resp)
	}
	rules, ok := pol["rules"].([]any)
	if !ok || len(rules) != 5 {
		t.Fatalf("expected 5 rules in policy, got %#v", pol["rules"])
	}
}
