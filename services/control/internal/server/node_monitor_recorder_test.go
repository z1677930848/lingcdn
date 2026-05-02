package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/lingcdn/control/internal/store"
)

func TestNodeMonitorRecorderAggregateBandwidth(t *testing.T) {
	m := newNodeMonitorRecorder()
	base := time.Now().Add(-10 * time.Second).Truncate(time.Second)
	m.add("n1", nodeMetricPoint{At: base, BytesSent: 100, BytesReceived: 50, CPUUsage: 10, MemUsage: 20, DiskUsage: 30, TCPEstablished: 5})
	m.add("n1", nodeMetricPoint{At: base.Add(10 * time.Second), BytesSent: 1100, BytesReceived: 1050, CPUUsage: 30, MemUsage: 40, DiskUsage: 50, TCPEstablished: 15})

	agg, ok := m.aggregate("n1", 60*time.Second)
	if !ok {
		t.Fatalf("expected ok")
	}
	if agg.DeltaBytesSent != 1000 {
		t.Fatalf("delta sent got=%d", agg.DeltaBytesSent)
	}
	if agg.DeltaBytesRecv != 1000 {
		t.Fatalf("delta recv got=%d", agg.DeltaBytesRecv)
	}
	if agg.AvgUpBps != 100 {
		t.Fatalf("up bps got=%v", agg.AvgUpBps)
	}
	if agg.AvgDownBps != 100 {
		t.Fatalf("down bps got=%v", agg.AvgDownBps)
	}
	if agg.AvgCPUUsage < 19.9 || agg.AvgCPUUsage > 20.1 {
		t.Fatalf("avg cpu got=%v", agg.AvgCPUUsage)
	}
	if agg.AvgTCPEstablished < 9.9 || agg.AvgTCPEstablished > 10.1 {
		t.Fatalf("avg conn got=%v", agg.AvgTCPEstablished)
	}
}

func TestNodeMonitorRankHandlerSorts(t *testing.T) {
	db := store.NewMemory("", "")
	ctx := context.Background()
	_ = db.CreateNode(ctx, &store.Node{ID: "n1", Hostname: "node-1", Status: "online"})
	_ = db.CreateNode(ctx, &store.Node{ID: "n2", Hostname: "node-2", Status: "online"})

	s := &Servers{store: db, nodeMonitor: newNodeMonitorRecorder()}
	base := time.Now().Add(-10 * time.Second).Truncate(time.Second)
	s.nodeMonitor.add("n1", nodeMetricPoint{At: base, BytesSent: 0})
	s.nodeMonitor.add("n1", nodeMetricPoint{At: base.Add(10 * time.Second), BytesSent: 1000})
	s.nodeMonitor.add("n2", nodeMetricPoint{At: base, BytesSent: 0})
	s.nodeMonitor.add("n2", nodeMetricPoint{At: base.Add(10 * time.Second), BytesSent: 2000})

	rr := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "http://example.com/api/nodes/monitor/rank?group=bandwidth&window_seconds=60&limit=10", nil)
	s.handleNodeMonitorRank(rr, req)

	var resp monitorRankResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(resp.Nodes) < 2 {
		t.Fatalf("expected 2 nodes")
	}
	if resp.Nodes[0].NodeID != "n2" {
		t.Fatalf("expected n2 first, got=%s", resp.Nodes[0].NodeID)
	}
}
