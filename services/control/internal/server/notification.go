package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// sendWebhookNotification sends a notification to configured webhooks (DingTalk, WeChat, Feishu).
// It reads settings from the database and sends to all enabled channels.
func (s *Servers) sendWebhookNotification(ctx context.Context, title, content string) {
	if s == nil || s.store == nil {
		return
	}

	settings, err := s.store.GetSettings(ctx)
	if err != nil || settings == nil {
		log.Ctx(ctx).Warn().Err(err).Msg("failed to get settings for webhook notification")
		return
	}

	// Send to DingTalk
	if settings.DingtalkEnabled && strings.TrimSpace(settings.DingtalkWebhook) != "" {
		go func() {
			if err := sendDingTalkWebhook(settings.DingtalkWebhook, title, content); err != nil {
				log.Warn().Err(err).Msg("failed to send DingTalk webhook")
			}
		}()
	}

	// Send to WeChat Work
	if settings.WechatEnabled && strings.TrimSpace(settings.WechatWebhook) != "" {
		go func() {
			if err := sendWeChatWebhook(settings.WechatWebhook, title, content); err != nil {
				log.Warn().Err(err).Msg("failed to send WeChat webhook")
			}
		}()
	}

	// Send to Feishu/Lark
	if settings.FeishuEnabled && strings.TrimSpace(settings.FeishuWebhook) != "" {
		go func() {
			if err := sendFeishuWebhook(settings.FeishuWebhook, title, content); err != nil {
				log.Warn().Err(err).Msg("failed to send Feishu webhook")
			}
		}()
	}
}

// sendDingTalkWebhook sends a markdown message to DingTalk webhook.
func sendDingTalkWebhook(webhookURL, title, content string) error {
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"title": title,
			"text":  fmt.Sprintf("### %s\n\n%s", title, content),
		},
	}
	return postWebhook(webhookURL, payload)
}

// sendWeChatWebhook sends a markdown message to WeChat Work webhook.
func sendWeChatWebhook(webhookURL, title, content string) error {
	payload := map[string]interface{}{
		"msgtype": "markdown",
		"markdown": map[string]string{
			"content": fmt.Sprintf("**%s**\n\n%s", title, content),
		},
	}
	return postWebhook(webhookURL, payload)
}

// sendFeishuWebhook sends a text message to Feishu/Lark webhook.
func sendFeishuWebhook(webhookURL, title, content string) error {
	payload := map[string]interface{}{
		"msg_type": "text",
		"content": map[string]string{
			"text": fmt.Sprintf("%s\n\n%s", title, content),
		},
	}
	return postWebhook(webhookURL, payload)
}

// postWebhook is a helper to POST JSON to a webhook URL.
func postWebhook(url string, payload interface{}) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal webhook payload: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create webhook request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("send webhook: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}
	return nil
}
