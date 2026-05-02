package server

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/lingcdn/control/internal/ttlcache"
	"github.com/lingcdn/control/internal/templates"
)

type templateOverrideCacheValue struct {
	hasOverride bool
	content     string
}

var templateOverrideCache = ttlcache.New[templateOverrideCacheValue](ttlcache.WithShards(256))

const templateOverrideCacheTTL = 30 * time.Second

func invalidateTemplateOverrideCache(key string) {
	templateOverrideCache.Delete(strings.TrimSpace(key))
}

func (s *Servers) effectiveTemplateContent(ctx context.Context, key string) (content string, customized bool, err error) {
	def, ok := getTemplateDef(key)
	if !ok {
		return "", false, nil
	}
	content = def.DefaultContent
	customized = false
	if s == nil || s.store == nil {
		return content, customized, nil
	}

	if v, ok := templateOverrideCache.Get(key); ok {
		if v.hasOverride {
			return v.content, true, nil
		}
		return content, false, nil
	}

	ov, err := s.store.GetGlobalTemplateOverride(ctx, key)
	if err != nil {
		return content, customized, err
	}
	if ov != nil {
		templateOverrideCache.Set(key, templateOverrideCacheValue{hasOverride: true, content: ov.Content}, templateOverrideCacheTTL)
		return ov.Content, true, nil
	}
	templateOverrideCache.Set(key, templateOverrideCacheValue{hasOverride: false}, templateOverrideCacheTTL)
	return content, customized, nil
}

func getTemplateDef(key string) (templates.GlobalTemplateDef, bool) {
	for _, d := range templates.DefaultGlobalTemplateDefs() {
		if d.Key == key {
			return d, true
		}
	}
	return templates.GlobalTemplateDef{}, false
}

func renderTemplateText(content string, values map[string]string) string {
	out := content
	for k, v := range values {
		out = strings.ReplaceAll(out, "{{"+k+"}}", v)
	}
	return out
}

func (s *Servers) defaultEmailRegisterBody(ctx context.Context, systemName, email, code string, ttlMinutes int) (string, error) {
	content, _, err := s.effectiveTemplateContent(ctx, "email.register_code.text")
	if err != nil {
		return "", err
	}
	values := map[string]string{
		"system_name":  systemName,
		"email":        email,
		"code":         code,
		"ttl_minutes":  strconv.Itoa(ttlMinutes),
	}
	return renderTemplateText(content, values), nil
}

func (s *Servers) defaultEmailPasswordResetBody(ctx context.Context, systemName, email, code string, ttlMinutes int) (string, error) {
	values := map[string]string{
		"system_name": systemName,
		"email":       email,
		"code":        code,
		"ttl_minutes": strconv.Itoa(ttlMinutes),
	}
	content, _, err := s.effectiveTemplateContent(ctx, "email.password_reset_code.text")
	if err != nil {
		fallback := "您好，\n您正在重置 {{system_name}} 账号密码。\n验证码：{{code}}\n有效期：{{ttl_minutes}} 分钟\n\n如非本人操作，请忽略此邮件。"
		return renderTemplateText(fallback, values), nil
	}
	if strings.TrimSpace(content) == "" {
		fallback := "您好，\n您正在重置 {{system_name}} 账号密码。\n验证码：{{code}}\n有效期：{{ttl_minutes}} 分钟\n\n如非本人操作，请忽略此邮件。"
		return renderTemplateText(fallback, values), nil
	}
	return renderTemplateText(content, values), nil
}

func (s *Servers) defaultWAFChallengeTemplate(ctx context.Context) (string, error) {
	content, _, err := s.effectiveTemplateContent(ctx, "waf.challenge.page")
	if err != nil {
		return "", err
	}
	return content, nil
}

func (s *Servers) defaultWAFBanTemplate(ctx context.Context) (string, error) {
	content, _, err := s.effectiveTemplateContent(ctx, "waf.ban.page")
	if err != nil {
		return "", err
	}
	return content, nil
}

func (s *Servers) defaultErrorTemplate(ctx context.Context, status int) (string, error) {
	var key string
	switch status {
	case 502:
		key = "error.502.html"
	case 504:
		key = "error.504.html"
	default:
		return "", nil
	}
	content, _, err := s.effectiveTemplateContent(ctx, key)
	if err != nil {
		return "", err
	}
	return content, nil
}
