package server

import (
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
}

type nodeMonitorRecorder struct {
	mu        sync.RWMutex
	points    map[string][]nodeMetricPoint
	retention time.Duration
}

func newNodeMonitorRecorder() *nodeMonitorRecorder {
	return &nodeMonitorRecorder{
		points:    make(map[string][]nodeMetricPoint),
		retention: 2 * time.Hour,
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
		StartAt:         first.At,
		EndAt:           last.At,
		DeltaBytesSent:  dSent,
		DeltaBytesRecv:  dRecv,
		AvgUpBps:        float64(dSent) / secs,
		AvgDownBps:      float64(dRecv) / secs,
		AvgCPUUsage:     sumCPU / n,
		AvgMemUsage:     sumMem / n,
		AvgDiskUsage:    sumDisk / n,
		AvgTCPEstablished: float64(sumTCP) / n,
	}, true
}

type nodeAgg struct {
	StartAt            time.Time
	EndAt              time.Time
	DeltaBytesSent     int64
	DeltaBytesRecv     int64
	AvgUpBps           float64
	AvgDownBps         float64
	AvgCPUUsage        float64
	AvgMemUsage        float64
	AvgDiskUsage       float64
	AvgTCPEstablished  float64
}
