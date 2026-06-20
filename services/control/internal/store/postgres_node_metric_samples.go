package store

import (
	"context"
	"time"
)

func (p *Postgres) InsertNodeMetricSample(ctx context.Context, nodeID string, sampledAt time.Time, requestsTotal, bytesSent int64) error {
	if nodeID == "" || sampledAt.IsZero() {
		return nil
	}
	_, err := p.pool.Exec(ctx, `
		INSERT INTO node_metric_samples (node_id, sampled_at, requests_total, bytes_sent)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (node_id, sampled_at) DO UPDATE
			SET requests_total = EXCLUDED.requests_total,
			    bytes_sent = EXCLUDED.bytes_sent
	`, nodeID, sampledAt, requestsTotal, bytesSent)
	return err
}

func (p *Postgres) loadNodeMetricSamples(ctx context.Context, start, end time.Time) (map[string][]NodeMetricSample, error) {
	lookback := start.Add(-3 * time.Hour)
	rows, err := p.pool.Query(ctx, `
		SELECT node_id, sampled_at, requests_total, bytes_sent
		FROM node_metric_samples
		WHERE sampled_at >= $1 AND sampled_at <= $2
		ORDER BY node_id, sampled_at
	`, lookback, end)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string][]NodeMetricSample)
	for rows.Next() {
		var s NodeMetricSample
		if err := rows.Scan(&s.NodeID, &s.SampledAt, &s.RequestsTotal, &s.BytesSent); err != nil {
			return nil, err
		}
		out[s.NodeID] = append(out[s.NodeID], s)
	}
	return out, rows.Err()
}

func (p *Postgres) SumNodeRequestsInWindow(ctx context.Context, start, end time.Time) (int64, error) {
	byNode, err := p.NodeRequestsInWindowByNode(ctx, start, end)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, n := range byNode {
		total += n
	}
	return total, nil
}

func (p *Postgres) NodeRequestsInWindowByNode(ctx context.Context, start, end time.Time) (map[string]int64, error) {
	byNode, err := p.loadNodeMetricSamples(ctx, start, end)
	if err != nil {
		return nil, err
	}
	out := make(map[string]int64, len(byNode))
	for nodeID, samples := range byNode {
		if delta := requestsDeltaInWindow(samples, start, end); delta > 0 {
			out[nodeID] = delta
		}
	}
	return out, nil
}
