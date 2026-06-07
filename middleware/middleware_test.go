package middleware_test

import (
	"bufio"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/incogni23/obskit/correlation"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/metrics"
	"github.com/incogni23/obskit/middleware"
	"github.com/incogni23/obskit/tracing"
)

var noop = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
})

func TestCorrelation_echoesExistingID(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlation.Header, "my-id")
	rr := httptest.NewRecorder()
	middleware.Correlation(noop).ServeHTTP(rr, req)
	if got := rr.Header().Get(correlation.Header); got != "my-id" {
		t.Fatalf("want my-id, got %s", got)
	}
}

func TestCorrelation_generatesIfAbsent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	middleware.Correlation(noop).ServeHTTP(rr, req)
	if got := rr.Header().Get(correlation.Header); got == "" {
		t.Fatal("expected generated correlation ID in response header")
	}
}

func TestCorrelation_injectsIntoContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlation.Header, "ctx-id")
	var got string
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = correlation.FromContext(r.Context())
	})
	middleware.Correlation(handler).ServeHTTP(httptest.NewRecorder(), req)
	if got != "ctx-id" {
		t.Fatalf("want ctx-id, got %s", got)
	}
}

func TestTracing_createsSpan(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/ping", nil)
	var span *tracing.Span
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		span = tracing.FromContext(r.Context())
	})
	middleware.Tracing(handler).ServeHTTP(httptest.NewRecorder(), req)
	if span == nil {
		t.Fatal("expected span in context")
	}
	if span.TraceID == "" || span.SpanID == "" {
		t.Fatalf("span missing IDs: %+v", span)
	}
}

func TestTracing_extractsTraceparent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("traceparent", "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01")
	var span *tracing.Span
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		span = tracing.FromContext(r.Context())
	})
	middleware.Tracing(handler).ServeHTTP(httptest.NewRecorder(), req)
	if span.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
		t.Fatalf("want propagated trace ID, got %s", span.TraceID)
	}
}

func TestLogging_storesLoggerInContext(t *testing.T) {
	log := logger.Must(logger.New(logger.Config{Level: "debug", JSON: true}))
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	var ctxLog *logger.Logger
	handler := http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		ctxLog = logger.FromContext(r.Context())
	})
	middleware.Logging(log)(handler).ServeHTTP(httptest.NewRecorder(), req)
	if ctxLog == nil {
		t.Fatal("expected logger in context")
	}
}

func TestMetrics_doesNotPanicOnDoubleWire(t *testing.T) {
	reg := metrics.New()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("double-wiring panicked: %v", r)
		}
	}()
	h := middleware.Metrics(reg)(noop)
	h = middleware.Metrics(reg)(h)
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
}

func TestChain_order(t *testing.T) {
	var order []string
	mw := func(label string) func(http.Handler) http.Handler {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				order = append(order, label)
				next.ServeHTTP(w, r)
			})
		}
	}
	h := middleware.Chain(noop, mw("a"), mw("b"), mw("c"))
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/", nil))
	if strings.Join(order, ",") != "a,b,c" {
		t.Fatalf("want a,b,c got %v", order)
	}
}

func TestTransport_injectsCorrelationAndTraceparent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("x-got-correlation", r.Header.Get(correlation.Header))
		w.Header().Set("x-got-traceparent", r.Header.Get("traceparent"))
	}))
	defer srv.Close()

	ctx := correlation.WithID(context.Background(), "transport-test")
	ctx, span := tracing.Start(ctx, "op")
	defer span.End()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	client := &http.Client{Transport: middleware.Transport(nil)}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	if got := resp.Header.Get("x-got-correlation"); got != "transport-test" {
		t.Fatalf("correlation not propagated: %q", got)
	}
	if got := resp.Header.Get("x-got-traceparent"); got == "" {
		t.Fatal("traceparent not propagated")
	}
}

type hijackRecorder struct {
	*httptest.ResponseRecorder
	hijacked bool
}

func (h *hijackRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h.hijacked = true
	return nil, nil, nil
}

func TestResponseWriter_delegatesHijacker(t *testing.T) {
	hr := &hijackRecorder{ResponseRecorder: httptest.NewRecorder()}
	var hijacked bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hj, ok := w.(http.Hijacker); ok {
			hj.Hijack()
			hijacked = true
		}
	})
	log := logger.Must(logger.New(logger.Config{Level: "info", JSON: true}))
	middleware.Logging(log)(handler).ServeHTTP(hr, httptest.NewRequest(http.MethodGet, "/", nil))
	if !hijacked || !hr.hijacked {
		t.Fatal("Hijack not delegated through responseWriter")
	}
}
