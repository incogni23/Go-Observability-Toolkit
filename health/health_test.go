package health_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/incogni23/obskit/health"
)

func TestAllUp(t *testing.T) {
	r := health.New(0)
	r.Register("a", health.OK)
	r.Register("b", health.OK)
	report := r.Run(context.Background())
	if report.Status != string(health.StatusUp) {
		t.Fatalf("want up, got %s", report.Status)
	}
}

func TestOneDown(t *testing.T) {
	r := health.New(0)
	r.Register("ok", health.OK)
	r.Register("bad", func(_ context.Context) health.CheckResult {
		return health.CheckResult{Status: health.StatusDown, Message: "no connection"}
	})
	report := r.Run(context.Background())
	if report.Status != string(health.StatusDown) {
		t.Fatal("expected overall status down")
	}
}

func TestReadinessHandler_503(t *testing.T) {
	r := health.New(0)
	r.Register("db", func(_ context.Context) health.CheckResult {
		return health.CheckResult{Status: health.StatusDown}
	})
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()
	r.ReadinessHandler()(rr, req)
	if rr.Code != 503 {
		t.Fatalf("want 503, got %d", rr.Code)
	}
}

func TestReadinessHandler_200(t *testing.T) {
	r := health.New(0)
	r.Register("ping", health.OK)
	req := httptest.NewRequest("GET", "/readyz", nil)
	rr := httptest.NewRecorder()
	r.ReadinessHandler()(rr, req)
	if rr.Code != 200 {
		t.Fatalf("want 200, got %d", rr.Code)
	}
	var report health.Report
	if err := json.NewDecoder(rr.Body).Decode(&report); err != nil {
		t.Fatal(err)
	}
	if report.Status != string(health.StatusUp) {
		t.Fatalf("want up in body, got %s", report.Status)
	}
	res := report.Checks["ping"]
	if res.LatencyMs < 0 {
		t.Fatalf("latency_ms must be non-negative, got %f", res.LatencyMs)
	}
}
