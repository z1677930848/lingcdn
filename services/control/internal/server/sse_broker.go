package server

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/lingcdn/control/internal/store"
)

// sseFrame is a single SSE message: event name + JSON payload.
type sseFrame struct {
	event string
	data  []byte
}

// sseClient represents a connected SSE browser client.
type sseClient struct {
	ch   chan sseFrame
	done chan struct{}
}

// sseBroker manages SSE client connections and broadcasts aggregated
// monitoring data plus per-task sync state updates.
type sseBroker struct {
	mu       sync.RWMutex
	clients  map[*sseClient]struct{}
	notifyCh chan struct{}
	stopCh   chan struct{}
	recorder *nodeMonitorRecorder
	store    store.Store
}

const (
	sseCoalesceWindow = 3 * time.Second
	sseMaxClients     = 100
	sseClientBufSize  = 4
	sseKeepalive      = 30 * time.Second
)

func newSSEBroker(recorder *nodeMonitorRecorder, st store.Store) *sseBroker {
	return &sseBroker{
		clients:  make(map[*sseClient]struct{}),
		notifyCh: make(chan struct{}, 1),
		stopCh:   make(chan struct{}),
		recorder: recorder,
		store:    st,
	}
}

func (b *sseBroker) run() {
	timer := time.NewTimer(sseCoalesceWindow)
	timer.Stop()
	pending := false

	for {
		select {
		case <-b.notifyCh:
			if !pending {
				timer.Reset(sseCoalesceWindow)
				pending = true
			}
		case <-timer.C:
			pending = false
			b.mu.RLock()
			n := len(b.clients)
			b.mu.RUnlock()
			if n == 0 {
				continue
			}
			data, err := b.buildPayload()
			if err != nil {
				log.Warn().Err(err).Msg("sse: failed to build payload")
				continue
			}
			if len(data) > 0 {
				b.broadcastFrame(sseFrame{event: "monitor", data: data})
			}
		case <-b.stopCh:
			timer.Stop()
			return
		}
	}
}

func (b *sseBroker) stop() {
	select {
	case b.stopCh <- struct{}{}:
	default:
	}
}

func (b *sseBroker) notify() {
	select {
	case b.notifyCh <- struct{}{}:
	default:
	}
}

// notifyTask pushes a per-task state update to all connected SSE clients
// immediately (no coalescing, to keep sync indicators responsive).
func (b *sseBroker) notifyTask(evt syncTaskEvent) {
	if b == nil {
		return
	}
	data, err := json.Marshal(evt)
	if err != nil {
		log.Warn().Err(err).Msg("sse: failed to marshal task event")
		return
	}
	b.broadcastFrame(sseFrame{event: "sync", data: data})
}

func (b *sseBroker) subscribe() *sseClient {
	c := &sseClient{
		ch:   make(chan sseFrame, sseClientBufSize),
		done: make(chan struct{}),
	}
	b.mu.Lock()
	if len(b.clients) >= sseMaxClients {
		b.mu.Unlock()
		close(c.done)
		return c
	}
	b.clients[c] = struct{}{}
	b.mu.Unlock()
	return c
}

func (b *sseBroker) unsubscribe(c *sseClient) {
	b.mu.Lock()
	if _, ok := b.clients[c]; ok {
		delete(b.clients, c)
		close(c.done)
	}
	b.mu.Unlock()
}

func (b *sseBroker) broadcastFrame(f sseFrame) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	for c := range b.clients {
		select {
		case c.ch <- f:
		default:
			// slow client, drop message
		}
	}
}

// monitorSSEPayload is the SSE event data sent to browsers.
type monitorSSEPayload struct {
	Rank   *monitorRankResponse              `json:"rank"`
	Series map[string]*monitorSeriesResponse `json:"series"`
}

// syncTaskEvent is the SSE payload for a publish/DNS task state transition.
// Subject links the task back to a business entity (e.g. "domain:<id>") so the
// UI can mark the relevant row as syncing / failed.
type syncTaskEvent struct {
	Kind        string    `json:"kind"` // publish | dns
	ID          string    `json:"id"`
	Subject     string    `json:"subject,omitempty"`
	Status      string    `json:"status"` // running | success | failed
	Message     string    `json:"message,omitempty"`
	StartedAt   time.Time `json:"started_at"`
	CompletedAt time.Time `json:"completed_at,omitempty"`
}

func (b *sseBroker) buildPayload() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	nodes, err := b.store.ListNodes(ctx)
	if err != nil {
		return nil, err
	}

	rank := b.buildRank(nodes)
	series := b.buildSeries(nodes)

	payload := monitorSSEPayload{
		Rank:   rank,
		Series: series,
	}
	return json.Marshal(payload)
}

func (b *sseBroker) buildRank(nodes []*store.Node) *monitorRankResponse {
	window := 60 * time.Second
	list := make([]monitorRankEntry, 0, len(nodes))
	for _, n := range nodes {
		if n == nil {
			continue
		}
		entry := monitorRankEntry{
			NodeID:      n.ID,
			Hostname:    n.Hostname,
			Iface:       "all",
			OutBps:      n.BandwidthUpBps,
			InBps:       n.BandwidthDownBps,
			CPUUsage:    n.CPUUsage,
			MemUsage:    n.MemUsage,
			DiskUsage:   n.DiskUsage,
			Connections: float64(n.TCPEstablished),
		}
		if b.recorder != nil {
			if agg, ok := b.recorder.aggregate(n.ID, window); ok {
				entry.OutBps = agg.AvgUpBps
				entry.InBps = agg.AvgDownBps
				entry.CPUUsage = agg.AvgCPUUsage
				entry.MemUsage = agg.AvgMemUsage
				entry.DiskUsage = agg.AvgDiskUsage
				entry.Connections = agg.AvgTCPEstablished
				entry.DeltaBytesSent = agg.DeltaBytesSent
				entry.DeltaBytesRecv = agg.DeltaBytesRecv
			}
		}
		list = append(list, entry)
	}
	sort.Slice(list, func(i, j int) bool { return list[i].OutBps > list[j].OutBps })
	if len(list) > 100 {
		list = list[:100]
	}
	for i := range list {
		list[i].Rank = i + 1
	}
	return &monitorRankResponse{
		Group:         "bandwidth",
		WindowSeconds: 60,
		Nodes:         list,
	}
}

func (b *sseBroker) buildSeries(nodes []*store.Node) map[string]*monitorSeriesResponse {
	metrics := []string{"bandwidth_out", "cpu", "mem", "connections", "disk"}
	windowSec := 60
	points := 30
	window := time.Duration(windowSec) * time.Second
	step := time.Duration(int(math.Max(1, float64(windowSec/(points-1))))) * time.Second
	end := time.Now()
	start := end.Add(-window)

	nodePoints := map[string][]nodeMetricPoint{}
	if b.recorder != nil {
		for _, n := range nodes {
			if n == nil {
				continue
			}
			nodePoints[n.ID] = b.recorder.snapshot(n.ID)
		}
	}

	result := make(map[string]*monitorSeriesResponse, len(metrics))
	for _, metric := range metrics {
		var out []monitorSeriesPoint
		for t := start; !t.After(end); t = t.Add(step) {
			total := 0.0
			if b.recorder != nil {
				for _, n := range nodes {
					if n == nil {
						continue
					}
					total += sseValueAt(nodePoints[n.ID], t, metric)
				}
			} else {
				for _, n := range nodes {
					if n == nil {
						continue
					}
					switch metric {
					case "cpu":
						total += n.CPUUsage
					case "mem":
						total += n.MemUsage
					case "disk":
						total += n.DiskUsage
					case "connections":
						total += float64(n.TCPEstablished)
					case "bandwidth_in":
						total += n.BandwidthDownBps
					default:
						total += n.BandwidthUpBps
					}
				}
			}
			out = append(out, monitorSeriesPoint{Timestamp: t.Unix(), Value: total})
		}
		result[metric] = &monitorSeriesResponse{
			Metric:        metric,
			WindowSeconds: windowSec,
			StepSeconds:   int(step.Seconds()),
			Points:        out,
		}
	}
	return result
}

func sseValueAt(pts []nodeMetricPoint, at time.Time, metric string) float64 {
	if len(pts) == 0 {
		return 0
	}
	i := sort.Search(len(pts), func(i int) bool { return !pts[i].At.Before(at) })
	idx := i
	if idx >= len(pts) {
		idx = len(pts) - 1
	} else if idx > 0 && pts[idx].At.After(at) {
		idx--
	}
	if idx < 0 {
		return 0
	}
	p := pts[idx]
	switch metric {
	case "cpu":
		return p.CPUUsage
	case "mem":
		return p.MemUsage
	case "disk":
		return p.DiskUsage
	case "connections", "conn":
		return float64(p.TCPEstablished)
	case "bandwidth_in":
		if idx == 0 {
			return 0
		}
		prev := pts[idx-1]
		secs := p.At.Sub(prev.At).Seconds()
		if secs <= 0 {
			return 0
		}
		return float64(p.BytesReceived-prev.BytesReceived) / secs
	default: // bandwidth_out
		if idx == 0 {
			return 0
		}
		prev := pts[idx-1]
		secs := p.At.Sub(prev.At).Seconds()
		if secs <= 0 {
			return 0
		}
		return float64(p.BytesSent-prev.BytesSent) / secs
	}
}

// handleNodeMonitorStream serves SSE connections for real-time monitoring.
func (s *Servers) handleNodeMonitorStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"error": "请求方法不允许"})
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "不支持流式传输"})
		return
	}

	// Disable the server's WriteTimeout for this long-lived connection.
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(time.Time{})

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	client := s.sseBroker.subscribe()
	defer s.sseBroker.unsubscribe(client)

	// Send initial snapshot.
	if initial, err := s.sseBroker.buildPayload(); err == nil && len(initial) > 0 {
		fmt.Fprintf(w, "event: monitor\ndata: %s\n\n", initial)
		flusher.Flush()
	}

	keepalive := time.NewTicker(sseKeepalive)
	defer keepalive.Stop()

	for {
		select {
		case frame, ok := <-client.ch:
			if !ok {
				return
			}
			evt := frame.event
			if evt == "" {
				evt = "monitor"
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", evt, frame.data)
			flusher.Flush()
		case <-keepalive.C:
			fmt.Fprintf(w, ": keepalive\n\n")
			flusher.Flush()
		case <-r.Context().Done():
			return
		case <-client.done:
			return
		}
	}
}
