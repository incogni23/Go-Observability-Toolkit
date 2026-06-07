package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

type Status string

const (
	StatusUp   Status = "up"
	StatusDown Status = "down"
)

type CheckResult struct {
	Status    Status  `json:"status"`
	Message   string  `json:"message,omitempty"`
	LatencyMs float64 `json:"latency_ms"`
}

type CheckFunc func(ctx context.Context) CheckResult

type Report struct {
	Status string                 `json:"status"`
	Checks map[string]CheckResult `json:"checks"`
}

type Registry struct {
	mu      sync.RWMutex
	checks  map[string]CheckFunc
	timeout time.Duration
}

func New(timeout time.Duration) *Registry {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Registry{checks: make(map[string]CheckFunc), timeout: timeout}
}

func (r *Registry) Register(name string, fn CheckFunc) {
	r.mu.Lock()
	r.checks[name] = fn
	r.mu.Unlock()
}

func (r *Registry) Run(ctx context.Context) Report {
	r.mu.RLock()
	snapshot := make(map[string]CheckFunc, len(r.checks))
	for k, v := range r.checks {
		snapshot[k] = v
	}
	r.mu.RUnlock()

	type entry struct {
		name string
		res  CheckResult
	}
	ch := make(chan entry, len(snapshot))

	for name, fn := range snapshot {
		go func(n string, f CheckFunc) {
			tCtx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			start := time.Now()
			res := f(tCtx)
			res.LatencyMs = float64(time.Since(start).Microseconds()) / 1000
			ch <- entry{n, res}
		}(name, fn)
	}

	report := Report{
		Status: string(StatusUp),
		Checks: make(map[string]CheckResult, len(snapshot)),
	}
	for range snapshot {
		e := <-ch
		report.Checks[e.name] = e.res
		if e.res.Status == StatusDown {
			report.Status = string(StatusDown)
		}
	}
	return report
}

func (r *Registry) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "up"})
	}
}

func (r *Registry) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		report := r.Run(req.Context())
		w.Header().Set("Content-Type", "application/json")
		if report.Status != string(StatusUp) {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	}
}

func OK(_ context.Context) CheckResult {
	return CheckResult{Status: StatusUp}
}
