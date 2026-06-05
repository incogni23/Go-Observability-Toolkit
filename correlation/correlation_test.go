package correlation_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/incogni23/obskit/correlation"
)

func TestRoundTrip(t *testing.T) {
	ctx := correlation.WithID(context.Background(), "abc-123")
	if got := correlation.FromContext(ctx); got != "abc-123" {
		t.Fatalf("want abc-123, got %s", got)
	}
}

func TestGeneratesIfMissing(t *testing.T) {
	ctx := correlation.WithID(context.Background(), "")
	if id := correlation.FromContext(ctx); id == "" {
		t.Fatal("expected a generated ID")
	}
}

func TestFromRequest_usesHeader(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set(correlation.Header, "existing-id")
	id, _ := correlation.FromRequest(req)
	if id != "existing-id" {
		t.Fatalf("want existing-id, got %s", id)
	}
}

func TestFromRequest_generatesIfAbsent(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	id, _ := correlation.FromRequest(req)
	if id == "" {
		t.Fatal("expected a generated correlation ID")
	}
}

func TestInjectHeader(t *testing.T) {
	ctx := correlation.WithID(context.Background(), "inject-me")
	h := make(http.Header)
	correlation.InjectHeader(ctx, h)
	if got := h.Get(correlation.Header); got != "inject-me" {
		t.Fatalf("want inject-me, got %s", got)
	}
}
