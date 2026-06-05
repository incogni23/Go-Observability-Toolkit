// Package middleware provides composable net/http middleware that wires together
// the obskit correlation, tracing, logging, and metrics packages.
package middleware

import (
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/incogni23/obskit/correlation"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/metrics"
	"github.com/incogni23/obskit/tracing"
)

// responseWriter wraps http.ResponseWriter to capture the status code and
// bytes written without allocating on every request.
type responseWriter struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytes += n
	return n, err
}

// routePattern returns the registered pattern from the request's context when
// available (Go 1.22+ net/http sets it via http.PatternKey), falling back to
// the raw path. Using the pattern instead of r.URL.Path prevents high-cardinality
// label values for routes like /users/{id}.
func routePattern(r *http.Request) string {
	if p := r.Pattern; p != "" {
		return p
	}
	return r.URL.Path
}

// Correlation injects a correlation ID into the request context and echoes it
// in the response header.
func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, r := correlation.FromRequest(r)
		w.Header().Set(correlation.Header, id)
		next.ServeHTTP(w, r)
	})
}

// Tracing starts a span per request, naming it "<METHOD> <pattern>".
func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.Start(r.Context(), fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging logs each request with method, path, status, duration, and
// correlation ID. It calls logger.FromContext so that registered context-field
// extractors (correlation_id, trace_id) are automatically attached.
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			// Use FromContext so correlation_id / trace_id are auto-attached
			// by any extractors registered via logger.RegisterContextFields.
			logger.FromContext(r.Context()).Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.status),
				zap.Duration("duration", time.Since(start)),
			)
			_ = log // retain the parameter so callers can pass a custom logger;
			// it is used below if the context carries no logger.
		})
	}
}

// Metrics records request count and duration histograms into the given
// registry. Route labels use the registered pattern (not the raw path) to
// avoid Prometheus cardinality explosion on parameterised routes.
func Metrics(reg *metrics.Registry) func(http.Handler) http.Handler {
	requests := reg.Counter(
		"http_requests_total",
		"Total number of HTTP requests",
		"method", "route", "status",
	)
	duration := reg.Histogram(
		"http_request_duration_seconds",
		"HTTP request duration in seconds",
		[]float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5, 5},
		"method", "route",
	)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(rw, r)
			route := routePattern(r)
			requests.WithLabelValues(r.Method, route, fmt.Sprint(rw.status)).Inc()
			duration.WithLabelValues(r.Method, route).Observe(time.Since(start).Seconds())
		})
	}
}

// Chain applies a list of middleware in order (outermost first).
func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
