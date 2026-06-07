package metrics

import (
	"errors"
	"net/http"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Registry struct {
	mu  sync.Mutex
	reg prometheus.Registerer
	gat prometheus.Gatherer
	hnd http.Handler
}

func New() *Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		prometheus.NewGoCollector(),
		prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}),
	)
	return newRegistry(reg, reg)
}

var Default = newRegistry(prometheus.DefaultRegisterer, prometheus.DefaultGatherer)

func newRegistry(r prometheus.Registerer, g prometheus.Gatherer) *Registry {
	return &Registry{
		reg: r,
		gat: g,
		hnd: promhttp.HandlerFor(g, promhttp.HandlerOpts{}),
	}
}

func (r *Registry) Counter(name, help string, labels ...string) *prometheus.CounterVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	c := prometheus.NewCounterVec(prometheus.CounterOpts{Name: name, Help: help}, labels)
	return mustOrExisting(r.reg.Register(c), c).(*prometheus.CounterVec)
}

func (r *Registry) Gauge(name, help string, labels ...string) *prometheus.GaugeVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	g := prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: name, Help: help}, labels)
	return mustOrExisting(r.reg.Register(g), g).(*prometheus.GaugeVec)
}

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
	return mustOrExisting(r.reg.Register(h), h).(*prometheus.HistogramVec)
}

func (r *Registry) Summary(name, help string, objectives map[float64]float64, labels ...string) *prometheus.SummaryVec {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := prometheus.NewSummaryVec(prometheus.SummaryOpts{
		Name:       name,
		Help:       help,
		Objectives: objectives,
	}, labels)
	return mustOrExisting(r.reg.Register(s), s).(*prometheus.SummaryVec)
}

func (r *Registry) HTTPHandler() http.Handler { return r.hnd }

// mustOrExisting returns the existing collector on AlreadyRegisteredError so
// that wiring middleware twice does not panic.
func mustOrExisting(err error, fallback prometheus.Collector) prometheus.Collector {
	if err == nil {
		return fallback
	}
	var are prometheus.AlreadyRegisteredError
	if errors.As(err, &are) {
		return are.ExistingCollector
	}
	panic(err)
}
