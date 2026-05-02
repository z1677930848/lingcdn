package server

// Elasticsearch index template management. Filebeat on the edge nodes
// writes documents into cdn-access-YYYY.MM.DD and cdn-error-YYYY.MM.DD
// indices. Auto-created daily indices inherit field mappings from a
// matching index template — without one, ES falls back to dynamic
// mapping (every string becomes both `text` and `.keyword`, which
// bloats storage and breaks aggregations on long fields). We push the
// templates whenever the operator saves or tests the ES connection so
// existing-cluster installs converge quickly without requiring a manual
// curl.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// pushESIndexTemplates is best-effort: any failure is logged at warn
// level but never propagated, so a transient ES outage during settings
// save does not block the admin save request itself. Returns the list
// of template names that were successfully pushed.
func pushESIndexTemplates(ctx context.Context, settings *store.Settings) []string {
	if settings == nil {
		return nil
	}
	esURL := strings.TrimSpace(settings.ElasticsearchURL)
	if esURL == "" {
		return nil
	}

	indexBase := strings.TrimSpace(settings.ElasticsearchIndex)
	if indexBase == "" {
		indexBase = "cdn-access"
	}
	indexBase = strings.TrimSuffix(indexBase, "-*")
	indexBase = strings.TrimSuffix(indexBase, "*")

	templates := []struct {
		name string
		body map[string]any
	}{
		{name: indexBase, body: cdnAccessTemplate(indexBase)},
		{name: "cdn-error", body: cdnErrorTemplate("cdn-error")},
	}

	applied := make([]string, 0, len(templates))
	for _, tpl := range templates {
		if err := putESTemplate(ctx, settings, tpl.name, tpl.body); err != nil {
			log.Ctx(ctx).Warn().Err(err).Str("template", tpl.name).Msg("ES index template push failed — auto-created indices will fall back to dynamic mapping")
			continue
		}
		applied = append(applied, tpl.name)
	}
	return applied
}

func putESTemplate(ctx context.Context, settings *store.Settings, name string, body map[string]any) error {
	encoded, err := json.Marshal(body)
	if err != nil {
		return err
	}
	target := strings.TrimRight(settings.ElasticsearchURL, "/") + "/_index_template/" + name

	rctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(rctx, http.MethodPut, target, bytes.NewReader(encoded))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if settings.ElasticsearchUser != "" {
		req.SetBasicAuth(settings.ElasticsearchUser, settings.ElasticsearchPass)
	}

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		buf, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &esTemplateError{status: resp.StatusCode, body: string(buf)}
	}
	log.Ctx(ctx).Info().Str("template", name).Msg("ES index template applied")
	return nil
}

type esTemplateError struct {
	status int
	body   string
}

func (e *esTemplateError) Error() string {
	return "ES rejected template push (status=" + intToStr(e.status) + "): " + e.body
}

func intToStr(n int) string {
	// Tiny helper to avoid pulling in fmt for a single use.
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// cdnAccessTemplate is the index template applied to access-log indices.
// Mappings are intentionally conservative: keyword for high-cardinality
// fields used in aggregations, ip for client_ip, integer/long for
// numeric stats, date for the timestamps. Unknown string fields fall
// back to keyword via the dynamic_template (saving the .keyword
// duplicate).
func cdnAccessTemplate(indexBase string) map[string]any {
	return map[string]any{
		"index_patterns": []string{indexBase + "-*"},
		"priority":       100,
		"template": map[string]any{
			"settings": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
				"refresh_interval":   "5s",
			},
			"mappings": map[string]any{
				"dynamic_templates": []map[string]any{
					{
						"strings_as_keyword": map[string]any{
							"match_mapping_type": "string",
							"mapping": map[string]any{
								"type":         "keyword",
								"ignore_above": 1024,
							},
						},
					},
				},
				"properties": map[string]any{
					"@timestamp":      map[string]any{"type": "date"},
					"timestamp":       map[string]any{"type": "date"},
					"status":          map[string]any{"type": "integer"},
					"bytes":           map[string]any{"type": "long"},
					"bytes_sent":      map[string]any{"type": "long"},
					"bytes_in":        map[string]any{"type": "long"},
					"duration_ms":     map[string]any{"type": "float"},
					"request_time_ms": map[string]any{"type": "float"},
					"domain":          map[string]any{"type": "keyword"},
					"client_ip":       map[string]any{"type": "ip"},
					"node_id":         map[string]any{"type": "keyword"},
					"node":            map[string]any{"type": "keyword"},
					"method":          map[string]any{"type": "keyword"},
					"host":            map[string]any{"type": "keyword"},
					"path":            map[string]any{"type": "keyword"},
					"user_agent":      map[string]any{"type": "keyword"},
					"referer":         map[string]any{"type": "keyword"},
					"cache_status":    map[string]any{"type": "keyword"},
					"location":        map[string]any{"type": "keyword"},
					"error":           map[string]any{"type": "text"},
				},
			},
		},
	}
}

// cdnErrorTemplate is the index template applied to error-log indices.
// The shape mirrors the on-disk JSON written by the node's
// log_reporter: timestamp + level + message + node identity + free-form
// labels. message stays text (full-text searchable in Kibana); node_id
// / level / target stay keyword for filtering.
func cdnErrorTemplate(indexBase string) map[string]any {
	return map[string]any{
		"index_patterns": []string{indexBase + "-*"},
		"priority":       100,
		"template": map[string]any{
			"settings": map[string]any{
				"number_of_shards":   1,
				"number_of_replicas": 0,
				"refresh_interval":   "5s",
			},
			"mappings": map[string]any{
				"dynamic_templates": []map[string]any{
					{
						"strings_as_keyword": map[string]any{
							"match_mapping_type": "string",
							"mapping": map[string]any{
								"type":         "keyword",
								"ignore_above": 1024,
							},
						},
					},
				},
				"properties": map[string]any{
					"@timestamp": map[string]any{"type": "date"},
					"level":      map[string]any{"type": "keyword"},
					"message":    map[string]any{"type": "text"},
					"target":     map[string]any{"type": "keyword"},
					"source":     map[string]any{"type": "keyword"},
					"node_id":    map[string]any{"type": "keyword"},
					"node":       map[string]any{"type": "keyword"},
				},
			},
		},
	}
}
