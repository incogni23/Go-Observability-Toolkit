// Package health provides a composable health-check registry with liveness
// and readiness probes, suitable for Kubernetes and load-balancer integration.
package health

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"sync"
	"time"
)

// Status represents the result of a single check.
type Status string

const (
	StatusUp   Status = "up"
	StatusDown Status = "down"
)

// CheckResult holds the outcome of one named check.
type CheckResult struct {
	Status     Status  `json:"status"`
	Message    string  `json:"message,omitempty"`
	LatencyMs  float64 `json:"latency_ms"` // wall-clock milliseconds, rounded to 3 decimal places
	latencyRaw time.Duration
}

// CheckFunc is the signature for a registered check.
type CheckFunc func(ctx context.Context) CheckResult

// Report is the aggregated response from all checks.
type Report struct {
	Status string                 `json:"status"` // "up" or "down"
	Checks map[string]CheckResult `json:"checks"`
}

// Registry holds named health checks.
type Registry struct {
	mu       sync.RWMutex
	checks   map[string]CheckFunc
	timeout  time.Duration
}

// New creates a Registry with a default per-check timeout of 5 s.
func New(timeout time.Duration) *Registry {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Registry{checks: make(map[string]CheckFunc), timeout: timeout}
}

// Register adds a named check. Calling Register with the same name overwrites the previous check.
func (r *Registry) Register(name string, fn CheckFunc) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.checks[name] = fn
}

// Run executes all checks concurrently and returns a Report.
func (r *Registry) Run(ctx context.Context) Report {
	r.mu.RLock()
	snapshot := make(map[string]CheckFunc, len(r.checks))
	for k, v := range r.checks {
		snapshot[k] = v
	}
	r.mu.RUnlock()

	type result struct {
		name string
		res  CheckResult
	}
	ch := make(chan result, len(snapshot))

	for name, fn := range snapshot {
		go func(n string, f CheckFunc) {
			tCtx, cancel := context.WithTimeout(ctx, r.timeout)
			defer cancel()
			start := time.Now()
			res := f(tCtx)
			elapsed := time.Since(start)
			res.latencyRaw = elapsed
			res.LatencyMs = math.Round(float64(elapsed.Microseconds())/1000*1000) / 1000
			ch <- result{n, res}
		}(name, fn)
	}

	report := Report{
		Status: string(StatusUp),
		Checks: make(map[string]CheckResult, len(snapshot)),
	}
	for range snapshot {
		r := <-ch
		report.Checks[r.name] = r.res
		if r.res.Status == StatusDown {
			report.Status = string(StatusDown)
		}
	}
	return report
}

// LivenessHandler returns an HTTP handler for /healthz (liveness).
// It always returns 200 — the process is alive if it can serve the request.
func (r *Registry) LivenessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "up"})
	}
}

// ReadinessHandler returns an HTTP handler for /readyz (readiness).
// Returns 200 when all checks pass, 503 otherwise.
func (r *Registry) ReadinessHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		report := r.Run(req.Context())
		w.Header().Set("Content-Type", "application/json")
		if report.Status == string(StatusUp) {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(report)
	}
}

// OK is a convenience CheckFunc that always reports up — useful as a placeholder.
func OK(_ context.Context) CheckResult {
	return CheckResult{Status: StatusUp}
}
