package server

import (
	"math"
	"sort"
	"sync"
	"time"
)

type nodeMetricPoint struct {
	At             time.Time
	BytesSent      int64
	BytesReceived  int64
	CPUUsage       float64
	MemUsage       float64
	DiskUsage      float64
	TCPEstablished int32
	RequestsTotal  int64
	CacheHits      int64
	CacheMisses    int64
}

type nodeMonitorRecorder struct {
	mu        sync.RWMutex
	points    map[string][]nodeMetricPoint
	retention time.Duration
}

func newNodeMonitorRecorder() *nodeMonitorRecorder {
	return &nodeMonitorRecorder{
		points:    make(map[string][]nodeMetricPoint),
		retention: 48 * time.Hour,
	}
}

func (m *nodeMonitorRecorder) add(nodeID string, p nodeMetricPoint) {
	if nodeID == "" || p.At.IsZero() {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	list := append(m.points[nodeID], p)
	sort.Slice(list, func(i, j int) bool { return list[i].At.Before(list[j].At) })

	cut := time.Now().Add(-m.retention)
	n := 0
	for _, it := range list {
		if it.At.Before(cut) {
			continue
		}
		list[n] = it
		n++
	}
	list = list[:n]
	if len(list) > 2000 {
		list = list[len(list)-2000:]
	}
	m.points[nodeID] = list
}

func (m *nodeMonitorRecorder) window(nodeID string, window time.Duration) []nodeMetricPoint {
	if nodeID == "" {
		return nil
	}
	if window <= 0 {
		window = 60 * time.Second
	}
	m.mu.RLock()
	list := append([]nodeMetricPoint(nil), m.points[nodeID]...)
	m.mu.RUnlock()
	if len(list) < 2 {
		return nil
	}

	end := list[len(list)-1].At
	start := end.Add(-window)
	i := sort.Search(len(list), func(i int) bool {
		return !list[i].At.Before(start)
	})
	if i < 0 || i >= len(list)-1 {
		return nil
	}
	return list[i:]
}

func (m *nodeMonitorRecorder) snapshot(nodeID string) []nodeMetricPoint {
	if nodeID == "" {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]nodeMetricPoint(nil), m.points[nodeID]...)
}

func (m *nodeMonitorRecorder) aggregate(nodeID string, window time.Duration) (nodeAgg, bool) {
	pts := m.window(nodeID, window)
	if len(pts) < 2 {
		return nodeAgg{}, false
	}
	first := pts[0]
	last := pts[len(pts)-1]
	secs := last.At.Sub(first.At).Seconds()
	if secs <= 0 {
		return nodeAgg{}, false
	}
	dSent := last.BytesSent - first.BytesSent
	dRecv := last.BytesReceived - first.BytesReceived

	sumCPU := 0.0
	sumMem := 0.0
	sumDisk := 0.0
	sumTCP := int64(0)
	for _, p := range pts {
		sumCPU += p.CPUUsage
		sumMem += p.MemUsage
		sumDisk += p.DiskUsage
		sumTCP += int64(p.TCPEstablished)
	}
	n := float64(len(pts))

	return nodeAgg{
		StartAt:           first.At,
		EndAt:             last.At,
		DeltaBytesSent:    dSent,
		DeltaBytesRecv:    dRecv,
		AvgUpBps:          float64(dSent) / secs,
		AvgDownBps:        float64(dRecv) / secs,
		AvgCPUUsage:       sumCPU / n,
		AvgMemUsage:       sumMem / n,
		AvgDiskUsage:      sumDisk / n,
		AvgTCPEstablished: float64(sumTCP) / n,
	}, true
}

// clusterTrends builds overview trend series from recorded per-node metrics.
func (m *nodeMonitorRecorder) clusterTrends(window string) []overviewTrendSeries {
	if m == nil {
		return nil
	}

	points, step := overviewTrendParams(window)
	if points < 2 {
		return nil
	}

	m.mu.RLock()
	all := make(map[string][]nodeMetricPoint, len(m.points))
	for id, list := range m.points {
		all[id] = append([]nodeMetricPoint(nil), list...)
	}
	m.mu.RUnlock()

	if len(all) == 0 {
		return nil
	}

	now := time.Now()
	start := now.Add(-time.Duration(points-1) * step)
	buckets := make([]struct {
		bwBps      float64
		requests   float64
		trafficGB  float64
		have       bool
	}, points)

	for _, list := range all {
		if len(list) < 2 {
			continue
		}
		for i := 0; i < len(list)-1; i++ {
			a, b := list[i], list[i+1]
			secs := b.At.Sub(a.At).Seconds()
			if secs <= 0 {
				continue
			}
			mid := a.At.Add(time.Duration(secs/2) * time.Second)
			if mid.Before(start) || mid.After(now) {
				continue
			}
			idx := int(mid.Sub(start) / step)
			if idx < 0 || idx >= points {
				continue
			}
			dSent := float64(b.BytesSent - a.BytesSent)
			dReq := float64(b.RequestsTotal - a.RequestsTotal)
			if dReq < 0 {
				dReq = 0
			}
			buckets[idx].bwBps += dSent / secs
			buckets[idx].requests += dReq
			buckets[idx].trafficGB += dSent / 1e9
			buckets[idx].have = true
		}
	}

	makeSeries := func(key, name, unit string, pick func(int) float64) overviewTrendSeries {
		s := overviewTrendSeries{Key: key, Name: name, Unit: unit, Points: make([]overviewTrendPoint, points)}
		for i := 0; i < points; i++ {
			val := 0.0
			if buckets[i].have {
				val = pick(i)
			}
			s.Points[i] = overviewTrendPoint{
				Timestamp: start.Add(time.Duration(i) * step),
				Value:     math.Round(val*100) / 100,
			}
		}
		return s
	}

	return []overviewTrendSeries{
		makeSeries("bandwidth", "Bandwidth", "Mbps", func(i int) float64 {
			return (buckets[i].bwBps * 8) / 1e6
		}),
		makeSeries("requests", "Requests", "count", func(i int) float64 {
			return buckets[i].requests
		}),
		makeSeries("traffic", "Traffic", "GB", func(i int) float64 {
			return buckets[i].trafficGB
		}),
	}
}

// clusterRequestsTotal sums positive request-counter deltas across all nodes
// within the selected overview window. This yields real in-window request counts
// from node telemetry (nginx lingcdn_requests_total), not TCP connections or
// lifetime cumulative totals projected across the chart.
func (m *nodeMonitorRecorder) clusterRequestsTotal(window string) int64 {
	if m == nil {
		return 0
	}
	win := overviewWindowDuration(window)
	now := time.Now()
	start := now.Add(-win)

	m.mu.RLock()
	all := make(map[string][]nodeMetricPoint, len(m.points))
	for id, list := range m.points {
		all[id] = append([]nodeMetricPoint(nil), list...)
	}
	m.mu.RUnlock()

	var total int64
	for _, list := range all {
		if len(list) < 2 {
			continue
		}
		for i := 0; i < len(list)-1; i++ {
			a, b := list[i], list[i+1]
			if b.At.Before(start) || a.At.After(now) {
				continue
			}
			dReq := b.RequestsTotal - a.RequestsTotal
			if dReq > 0 {
				total += dReq
			}
		}
	}
	return total
}

func overviewWindowDuration(window string) time.Duration {
	switch window {
	case "7d":
		return 7 * 24 * time.Hour
	case "30d":
		return 30 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

const overviewTelemetryRetention = 48 * time.Hour

func overviewTelemetryWindowStart(window string) time.Time {
	dur := overviewWindowDuration(window)
	if dur > overviewTelemetryRetention {
		dur = overviewTelemetryRetention
	}
	return time.Now().Add(-dur)
}

type nodeAgg struct {
	StartAt           time.Time
	EndAt             time.Time
	DeltaBytesSent    int64
	DeltaBytesRecv    int64
	AvgUpBps          float64
	AvgDownBps        float64
	AvgCPUUsage       float64
	AvgMemUsage       float64
	AvgDiskUsage      float64
	AvgTCPEstablished float64
}

func overviewTrendParams(window string) (points int, step time.Duration) {
	switch window {
	case "7d":
		return 14, 12 * time.Hour
	case "30d":
		return 15, 24 * time.Hour
	default:
		return 12, 2 * time.Hour
	}
}
