// Package metrics owns the Prometheus client registration for MyCloud.
//
// Everything that admin operators care about for capacity planning is
// surfaced as a labelled metric: HTTP request rate/latency by route, DB pool
// utilisation, prepared-statement count, queue depths, active WebSocket
// sessions. Handlers and workers reach into this package to bump counters
// and observe histograms.
//
// We deliberately use a custom registry instead of the default global one so
// the /metrics endpoint exposes only MyCloud's own series — no Go runtime
// metrics by default, which keeps the scrape payload small. To opt back in,
// the registry exposes Reg directly so a caller can register
// collectors.NewGoCollector() if desired.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
)

// Metrics groups every collector behind a single struct so dependents accept
// one parameter and the wiring stays explicit. Nil-safety on individual
// observers is the caller's responsibility — every method nil-guards `m`
// itself so tests that omit the metrics layer don't NPE.
type Metrics struct {
	Reg *prometheus.Registry

	RequestsTotal    *prometheus.CounterVec
	RequestDuration  *prometheus.HistogramVec
	DBQueryDuration  *prometheus.HistogramVec
	StmtCacheSize    prometheus.Gauge
	QueueDepth       *prometheus.GaugeVec
	DLQPushed        *prometheus.CounterVec
	WSConnections    prometheus.Gauge
	WSMessagesTotal  *prometheus.CounterVec
}

// New constructs a registered Metrics instance. Repeated calls would
// double-register; only call once at app startup.
func New() *Metrics {
	reg := prometheus.NewRegistry()
	// Always include Go runtime + process metrics — operators expect heap,
	// gc, fd-count etc. when triaging. The cardinality is fixed and small.
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	m := &Metrics{
		Reg: reg,
		RequestsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mycloud",
				Subsystem: "http",
				Name:      "requests_total",
				Help:      "Total HTTP requests served, partitioned by method, route template, and status class (2xx/3xx/4xx/5xx).",
			},
			[]string{"method", "route", "status"},
		),
		RequestDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mycloud",
				Subsystem: "http",
				Name:      "request_duration_seconds",
				Help:      "HTTP request handling latency.",
				// Buckets tuned for a file-storage app: from 5 ms up to 10 s.
				// Large uploads stream out — they intentionally land in the
				// long tail and that's fine; downloads of cached metadata
				// should sit in the first three buckets.
				Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5, 10},
			},
			[]string{"method", "route"},
		),
		DBQueryDuration: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: "mycloud",
				Subsystem: "db",
				Name:      "query_duration_seconds",
				Help:      "Database query latency, partitioned by query name (set by the call site, e.g. \"list_files\").",
				Buckets:   []float64{.001, .005, .01, .025, .05, .1, .25, .5, 1, 2.5},
			},
			[]string{"query"},
		),
		StmtCacheSize: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "mycloud",
				Subsystem: "db",
				Name:      "stmt_cache_size",
				Help:      "Number of distinct prepared statements currently held by the cache.",
			},
		),
		QueueDepth: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: "mycloud",
				Subsystem: "queue",
				Name:      "depth",
				Help:      "Approximate depth of each Redis-backed job queue, sampled periodically.",
			},
			[]string{"queue"},
		),
		DLQPushed: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mycloud",
				Subsystem: "queue",
				Name:      "dlq_pushed_total",
				Help:      "Total jobs pushed to a dead-letter queue, partitioned by source queue. Sustained increase = a worker is dropping every job; investigate the corresponding :failed list.",
			},
			[]string{"queue"},
		),
		WSConnections: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: "mycloud",
				Subsystem: "ws",
				Name:      "connections",
				Help:      "Currently open WebSocket sync connections.",
			},
		),
		WSMessagesTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: "mycloud",
				Subsystem: "ws",
				Name:      "messages_total",
				Help:      "WebSocket messages sent or received, partitioned by direction (in/out) and topic prefix.",
			},
			[]string{"direction", "topic_prefix"},
		),
	}
	reg.MustRegister(
		m.RequestsTotal,
		m.RequestDuration,
		m.DBQueryDuration,
		m.StmtCacheSize,
		m.QueueDepth,
		m.DLQPushed,
		m.WSConnections,
		m.WSMessagesTotal,
	)
	return m
}

// ObserveHTTPRequest is the canonical shorthand for the access-log
// middleware to record a single request. method and route are low-cardinality
// strings (HTTP verb + chi route pattern). statusClass is "2xx"/"4xx" etc;
// the raw status would explode label cardinality on adversarial inputs.
//
// Nil-receiver-safe: tests can pass a nil *Metrics to skip recording.
func (m *Metrics) ObserveHTTPRequest(method, route, statusClass string, durationSeconds float64) {
	if m == nil {
		return
	}
	m.RequestsTotal.WithLabelValues(method, route, statusClass).Inc()
	m.RequestDuration.WithLabelValues(method, route).Observe(durationSeconds)
}

// StatusClass collapses an HTTP status into a 2xx/3xx/4xx/5xx string so
// label cardinality stays bounded.
func StatusClass(status int) string {
	switch {
	case status < 200:
		return "1xx"
	case status < 300:
		return "2xx"
	case status < 400:
		return "3xx"
	case status < 500:
		return "4xx"
	default:
		return "5xx"
	}
}
