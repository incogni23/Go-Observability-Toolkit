// Package metrics exposes Prometheus-backed counters, gauges, histograms, and
// a default HTTP handler for the /metrics endpoint.
package metrics

import (
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Registry wraps a Prometheus registry with convenience factory methods.
type Registry struct {
	mu  sync.Mutex
	reg prometheus.Registerer
	gat prometheus.Gatherer
	hnd http.Handler
}

// New creates an isolated Registry with Go runtime and process collectors
// pre-registered. Prefer this over Default in libraries and tests to avoid
// polluting the global prometheus.DefaultRegisterer.
func New() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	return newRegistry(reg, reg)
}

// Default is a package-level Registry backed by prometheus.DefaultRegisterer /
// prometheus.DefaultGatherer — the same registry that the standard
// promhttp.Handler() exposes. Use it when you want obskit metrics to appear
// alongside any other Prometheus instrumentation already in the process.
var Default = newRegistry(prometheus.DefaultRegisterer, prometheus.DefaultGatherer)

func newRegistry(r prometheus.Registerer, g prometheus.Gatherer) *Registry {
	return &Registry{
		reg: r,
		gat: g,
		hnd: promhttp.HandlerFor(g, promhttp.HandlerOpts{}),
	}
}

// Counter registers and returns a *prometheus.CounterVec.
func (r *Registry) Counter(name, help string, labels ...string) *prometheus.CounterVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	r.reg.MustRegister(c)
	return c
}

// Gauge registers and returns a *prometheus.GaugeVec.
func (r *Registry) Gauge(name, help string, labels ...string) *prometheus.GaugeVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	r.reg.MustRegister(g)
	return g
}

// Histogram registers and returns a *prometheus.HistogramVec.
// Pass nil buckets to use prometheus.DefBuckets.
func (r *Registry) Histogram(name, help string, buckets []float64, labels ...string) *prometheus.HistogramVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	h := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    help,
		Buckets: buckets,
	}, labels)
	r.reg.MustRegister(h)
	return h
}

// Summary registers and returns a *prometheus.SummaryVec.
func (r *Registry) Summary(name, help string, objectives map[float64]float64, labels ...string) *prometheus.SummaryVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       name,
		Help:       help,
		Objectives: objectives,
	}, labels)
	r.reg.MustRegister(s)
	return s
}

// HTTPHandler returns the Prometheus /metrics HTTP handler for this registry.
func (r *Registry) HTTPHandler() http.Handler { return r.hnd }
