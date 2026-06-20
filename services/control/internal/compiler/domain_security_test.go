package compiler

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/lingcdn/control/internal/store"
)

func TestSecModeToActionCaptchaTypes(t *testing.T) {
	tests := map[string]string{
		"slide":    "slide",
		"click":    "click",
		"rotate":   "rotate",
		"captcha":  "slide_region",
		"js":       "js_challenge",
		"shield5s": "",
	}
	for mode, wantCaptcha := range tests {
		action, captcha := secModeToAction(mode)
		if mode == "shield5s" {
			if action != "shield" || captcha != "" {
				t.Fatalf("shield5s: got action=%q captcha=%q", action, captcha)
			}
			continue
		}
		if action != "challenge" {
			t.Fatalf("mode %q: action=%q want challenge", mode, action)
		}
		if captcha != wantCaptcha {
			t.Fatalf("mode %q: captcha=%q want %q", mode, captcha, wantCaptcha)
		}
	}
}

func TestSecCCRuleActionRespectsFilter(t *testing.T) {
	action, captcha := secCCRuleAction("放行", "slide")
	if action != "allow" || captcha != "" {
		t.Fatalf("allow: action=%q captcha=%q", action, captcha)
	}
	action, captcha = secCCRuleAction("拦截", "slide")
	if action != "deny" || captcha != "" {
		t.Fatalf("deny: action=%q captcha=%q", action, captcha)
	}
	action, captcha = secCCRuleAction("验证", "rotate")
	if action != "challenge" || captcha != "rotate" {
		t.Fatalf("challenge: action=%q captcha=%q", action, captcha)
	}
}

func TestCompileDomainSecurityDefaultModeSlide(t *testing.T) {
	mem := store.NewMemory("svc", "admin")
	ctx := context.Background()
	d := &store.Domain{
		ID:      "dom-1",
		Name:    "example.com",
		Enabled: true,
		Security: &store.DomainSecurity{
			DefaultMode: "slide",
			FailLimit:   5,
			BanSeconds:  600,
		},
	}
	if err := mem.CreateDomain(ctx, d); err != nil {
		t.Fatalf("create domain: %v", err)
	}
	_, payload, err := New(mem).Compile(ctx)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	var cfg NodeConfig
	if err := json.Unmarshal(payload, &cfg); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	var found bool
	for _, p := range cfg.WAFPolicies {
		if p.ScopeID != "dom-1" {
			continue
		}
		for _, r := range p.Rules {
			if r.Note == "domain default mode: slide" {
				found = true
				if r.CaptchaType != "slide" {
					t.Fatalf("captcha_type=%q want slide", r.CaptchaType)
				}
				if r.Threshold != 5 {
					t.Fatalf("threshold=%d want 5", r.Threshold)
				}
			}
		}
	}
	if !found {
		t.Fatal("domain default slide rule missing from compiled config")
	}
}
