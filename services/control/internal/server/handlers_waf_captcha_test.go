package server

import (
	"context"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func TestReplaceWAFRulesPersistsCaptchaType(t *testing.T) {
	s := testServers(t)
	ctx := context.Background()
	now := time.Now()
	rules := []*store.WAFRule{{
		ID:          "rule-1",
		Type:        "challenge_captcha",
		Action:      "deny",
		CaptchaType: "rotate",
		Enabled:     true,
		Threshold:   3,
	}}
	if err := s.replaceWAFRules(ctx, "policy-1", rules, now); err != nil {
		t.Fatalf("replace rules: %v", err)
	}
	got, err := s.store.ListWAFRules(ctx, "policy-1")
	if err != nil {
		t.Fatalf("list rules: %v", err)
	}
	if len(got) != 1 || got[0].CaptchaType != "rotate" {
		t.Fatalf("captcha_type not persisted: %#v", got)
	}
}
