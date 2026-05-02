package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Metrics holds all Prometheus metrics for the control plane.
type Metrics struct {
	// Server metrics
	ControlUp prometheus.Gauge

	// Node metrics
	NodesConnected  prometheus.Gauge
	NodesRegistered prometheus.Gauge
	NodesByStatus   *prometheus.GaugeVec

	// gRPC metrics
	GRPCRequestsTotal   *prometheus.CounterVec
	GRPCRequestDuration *prometheus.HistogramVec
	GRPCStreamActive    prometheus.Gauge

	// HTTP metrics
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Config metrics
	ConfigPublishTotal   prometheus.Counter
	ConfigPublishErrors  prometheus.Counter
	ConfigVersion        *prometheus.GaugeVec
	ConfigCompileDuration prometheus.Histogram

	// Purge metrics
	PurgeRequestsTotal prometheus.Counter
	PurgeURLsTotal     prometheus.Counter
	PurgeErrors        prometheus.Counter

	// Store metrics
	StoreOperationsTotal   *prometheus.CounterVec
	StoreOperationDuration *prometheus.HistogramVec
	StoreErrors            *prometheus.CounterVec
}

// New creates and registers all Prometheus metrics.
func New() *Metrics {
	m := &Metrics{
		// Server metrics
		ControlUp: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "up",
			Help:      "Control plane is up and running",
		}),

		// Node metrics
		NodesConnected: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "nodes_connected",
			Help:      "Number of currently connected nodes",
		}),
		NodesRegistered: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "nodes_registered",
			Help:      "Total number of registered nodes",
		}),
		NodesByStatus: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "nodes_by_status",
			Help:      "Number of nodes by status",
		}, []string{"status"}),

		// gRPC metrics
		GRPCRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "grpc_requests_total",
			Help:      "Total number of gRPC requests",
		}, []string{"method", "code"}),
		GRPCRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "grpc_request_duration_seconds",
			Help:      "gRPC request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method"}),
		GRPCStreamActive: promauto.NewGauge(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "grpc_streams_active",
			Help:      "Number of active gRPC streams",
		}),

		// HTTP metrics
		HTTPRequestsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "http_requests_total",
			Help:      "Total number of HTTP requests",
		}, []string{"method", "path", "status"}),
		HTTPRequestDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds",
			Buckets:   prometheus.DefBuckets,
		}, []string{"method", "path"}),

		// Config metrics
		ConfigPublishTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "config_publish_total",
			Help:      "Total number of config publishes",
		}),
		ConfigPublishErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "config_publish_errors_total",
			Help:      "Total number of config publish errors",
		}),
		ConfigVersion: promauto.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "config_version_info",
			Help:      "Current config version info",
		}, []string{"version"}),
		ConfigCompileDuration: promauto.NewHistogram(prometheus.HistogramOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "config_compile_duration_seconds",
			Help:      "Config compilation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		}),

		// Purge metrics
		PurgeRequestsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "purge_requests_total",
			Help:      "Total number of purge requests",
		}),
		PurgeURLsTotal: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "purge_urls_total",
			Help:      "Total number of URLs purged",
		}),
		PurgeErrors: promauto.NewCounter(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "purge_errors_total",
			Help:      "Total number of purge errors",
		}),

		// Store metrics
		StoreOperationsTotal: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "store_operations_total",
			Help:      "Total number of store operations",
		}, []string{"operation", "entity"}),
		StoreOperationDuration: promauto.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "store_operation_duration_seconds",
			Help:      "Store operation duration in seconds",
			Buckets:   prometheus.DefBuckets,
		}, []string{"operation", "entity"}),
		StoreErrors: promauto.NewCounterVec(prometheus.CounterOpts{
			Namespace: "lingcdn",
			Subsystem: "control",
			Name:      "store_errors_total",
			Help:      "Total number of store errors",
		}, []string{"operation", "entity"}),
	}

	// Set control up to 1
	m.ControlUp.Set(1)

	return m
}

// RecordHTTPRequest records an HTTP request metric.
func (m *Metrics) RecordHTTPRequest(method, path, status string, duration float64) {
	m.HTTPRequestsTotal.WithLabelValues(method, path, status).Inc()
	m.HTTPRequestDuration.WithLabelValues(method, path).Observe(duration)
}

// RecordGRPCRequest records a gRPC request metric.
func (m *Metrics) RecordGRPCRequest(method, code string, duration float64) {
	m.GRPCRequestsTotal.WithLabelValues(method, code).Inc()
	m.GRPCRequestDuration.WithLabelValues(method).Observe(duration)
}

// RecordStoreOperation records a store operation metric.
func (m *Metrics) RecordStoreOperation(operation, entity string, duration float64, err error) {
	m.StoreOperationsTotal.WithLabelValues(operation, entity).Inc()
	m.StoreOperationDuration.WithLabelValues(operation, entity).Observe(duration)
	if err != nil {
		m.StoreErrors.WithLabelValues(operation, entity).Inc()
	}
}

// RecordConfigPublish records a config publish event.
func (m *Metrics) RecordConfigPublish(err error) {
	m.ConfigPublishTotal.Inc()
	if err != nil {
		m.ConfigPublishErrors.Inc()
	}
}

// RecordPurge records a purge event.
func (m *Metrics) RecordPurge(urlCount int, err error) {
	m.PurgeRequestsTotal.Inc()
	m.PurgeURLsTotal.Add(float64(urlCount))
	if err != nil {
		m.PurgeErrors.Inc()
	}
}

// SetNodesConnected sets the number of connected nodes.
func (m *Metrics) SetNodesConnected(count int) {
	m.NodesConnected.Set(float64(count))
}

// SetNodesRegistered sets the number of registered nodes.
func (m *Metrics) SetNodesRegistered(count int) {
	m.NodesRegistered.Set(float64(count))
}

// SetNodesByStatus sets the number of nodes by status.
func (m *Metrics) SetNodesByStatus(status string, count int) {
	m.NodesByStatus.WithLabelValues(status).Set(float64(count))
}

// SetConfigVersion sets the current config version.
func (m *Metrics) SetConfigVersion(version string) {
	m.ConfigVersion.Reset()
	m.ConfigVersion.WithLabelValues(version).Set(1)
}

// IncGRPCStreams increments the active gRPC streams count.
func (m *Metrics) IncGRPCStreams() {
	m.GRPCStreamActive.Inc()
}

// DecGRPCStreams decrements the active gRPC streams count.
func (m *Metrics) DecGRPCStreams() {
	m.GRPCStreamActive.Dec()
}
