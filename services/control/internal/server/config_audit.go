package server

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/lingcdn/control/internal/store"
)

// auditConfigChange records a configuration mutation for operator review.
func (s *Servers) auditConfigChange(ctx context.Context, actor, action, subject string, before, after any) {
	if s == nil || s.store == nil {
		return
	}
	b, _ := json.Marshal(map[string]any{"before": before, "after": after})
	msg := fmt.Sprintf("%s %s: %s | %s", actor, action, subject, string(b))
	_ = s.store.CreateSystemLog(ctx, &store.SystemLog{
		Type:      "config_audit",
		Status:    "success",
		Message:   msg,
		CreatedAt: time.Now(),
	})
}
