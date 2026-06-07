package middleware

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/incogni23/obskit/correlation"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/metrics"
	"github.com/incogni23/obskit/tracing"
)

// responseWriter captures status and bytes while delegating Flusher, Hijacker,
// and Unwrap so SSE, WebSockets, and ResponseController all work correctly.
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

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("obskit: underlying ResponseWriter does not implement http.Hijacker")
	}
	return h.Hijack()
}

func (rw *responseWriter) Unwrap() http.ResponseWriter { return rw.ResponseWriter }

func routePattern(r *http.Request) string {
	if p := r.Pattern; p != "" {
		return p
	}
	return r.URL.Path
}

func Correlation(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, r := correlation.FromRequest(r)
		w.Header().Set(correlation.Header, id)
		next.ServeHTTP(w, r)
	})
}

// Tracing starts a span and propagates W3C traceparent on inbound requests.
func Tracing(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := tracing.ExtractTraceparent(r.Context(), r.Header)
		ctx, span := tracing.Start(ctx, fmt.Sprintf("%s %s", r.Method, r.URL.Path))
		defer span.End()
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Logging stores log in the request context so handlers can call
// logger.FromContext, then logs each completed request.
func Logging(log *logger.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rw := &responseWriter{ResponseWriter: w, status: http.StatusOK}
			r = r.WithContext(log.WithContext(r.Context()))
			next.ServeHTTP(rw, r)
			logger.FromContext(r.Context()).Info("http request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", rw.status),
				zap.Duration("duration", time.Since(start)),
			)
		})
	}
}

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

// Transport returns an http.RoundTripper that injects X-Correlation-ID and
// W3C traceparent into outbound requests, propagating context across service
// boundaries.
func Transport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return roundTripFunc(func(r *http.Request) (*http.Response, error) {
		r = r.Clone(r.Context())
		correlation.InjectHeader(r.Context(), r.Header)
		tracing.InjectTraceparent(r.Context(), r.Header)
		return base.RoundTrip(r)
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func Chain(h http.Handler, mws ...func(http.Handler) http.Handler) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}
