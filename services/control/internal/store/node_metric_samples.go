package store

import (
	"context"
	"sort"
	"time"
)

const nodeMetricSampleRetention = 48 * time.Hour

// NodeMetricSample is a point-in-time snapshot of a node's cumulative counters.
type NodeMetricSample struct {
	NodeID        string
	SampledAt     time.Time
	RequestsTotal int64
	BytesSent     int64
}

// requestsDeltaInWindow returns the request-count increase for samples within
// [start, end] using counter deltas (never the lifetime cumulative value).
func requestsDeltaInWindow(samples []NodeMetricSample, start, end time.Time) int64 {
	if len(samples) == 0 || end.Before(start) {
		return 0
	}
	sort.Slice(samples, func(i, j int) bool {
		return samples[i].SampledAt.Before(samples[j].SampledAt)
	})

	var baseline *NodeMetricSample
	var latest *NodeMetricSample
	for i := range samples {
		s := &samples[i]
		if s.SampledAt.After(end) {
			break
		}
		if !s.SampledAt.After(start) {
			baseline = s
		}
		latest = s
	}
	if latest == nil {
		return 0
	}
	if baseline == nil {
		for i := range samples {
			s := &samples[i]
			if !s.SampledAt.Before(start) && !s.SampledAt.After(end) {
				baseline = s
				break
			}
		}
	}
	if baseline == nil || latest.SampledAt.Before(baseline.SampledAt) {
		return 0
	}
	delta := latest.RequestsTotal - baseline.RequestsTotal
	if delta < 0 {
		return 0
	}
	return delta
}

func pruneNodeMetricSamples(samples []NodeMetricSample, cutoff time.Time) []NodeMetricSample {
	if len(samples) == 0 {
		return samples
	}
	n := 0
	for _, s := range samples {
		if s.SampledAt.Before(cutoff) {
			continue
		}
		samples[n] = s
		n++
	}
	samples = samples[:n]
	if len(samples) > 4000 {
		samples = samples[len(samples)-4000:]
	}
	return samples
}

// InsertNodeMetricSample appends a telemetry snapshot for windowed request totals.
func (m *Memory) InsertNodeMetricSample(ctx context.Context, nodeID string, sampledAt time.Time, requestsTotal, bytesSent int64) error {
	_ = ctx
	if nodeID == "" || sampledAt.IsZero() {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.nodeMetricSamples == nil {
		m.nodeMetricSamples = make(map[string][]NodeMetricSample)
	}
	cutoff := time.Now().Add(-nodeMetricSampleRetention)
	list := append(m.nodeMetricSamples[nodeID], NodeMetricSample{
		NodeID:        nodeID,
		SampledAt:     sampledAt,
		RequestsTotal: requestsTotal,
		BytesSent:     bytesSent,
	})
	m.nodeMetricSamples[nodeID] = pruneNodeMetricSamples(list, cutoff)
	return nil
}

// SumNodeRequestsInWindow totals in-window request deltas across all nodes.
func (m *Memory) SumNodeRequestsInWindow(ctx context.Context, start, end time.Time) (int64, error) {
	byNode, err := m.NodeRequestsInWindowByNode(ctx, start, end)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, n := range byNode {
		total += n
	}
	return total, nil
}

// NodeRequestsInWindowByNode returns per-node in-window request deltas.
func (m *Memory) NodeRequestsInWindowByNode(ctx context.Context, start, end time.Time) (map[string]int64, error) {
	_ = ctx
	lookback := start.Add(-3 * time.Hour)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make(map[string]int64, len(m.nodeMetricSamples))
	for nodeID, samples := range m.nodeMetricSamples {
		filtered := make([]NodeMetricSample, 0, len(samples))
		for _, s := range samples {
			if s.SampledAt.After(end) || s.SampledAt.Before(lookback) {
				continue
			}
			filtered = append(filtered, s)
		}
		if delta := requestsDeltaInWindow(filtered, start, end); delta > 0 {
			out[nodeID] = delta
		}
	}
	return out, nil
}
