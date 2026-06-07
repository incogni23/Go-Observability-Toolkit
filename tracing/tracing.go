package tracing

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const spanKey contextKey = "obskit_span"

type Span struct {
	TraceID   string
	SpanID    string
	ParentID  string
	Operation string
	StartedAt time.Time
	EndedAt   time.Time
	Tags      map[string]string
	Err       error
}

func (s *Span) Duration() time.Duration {
	if s.EndedAt.IsZero() {
		return 0
	}
	return s.EndedAt.Sub(s.StartedAt)
}

func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

func (s *Span) End() { s.EndedAt = time.Now() }

func (s *Span) Finish(err error) {
	s.Err = err
	s.End()
}

func Start(ctx context.Context, operation string) (context.Context, *Span) {
	parent := FromContext(ctx)
	span := &Span{
		TraceID:   newTraceID(parent),
		SpanID:    uuid.NewString(),
		Operation: operation,
		StartedAt: time.Now(),
		Tags:      make(map[string]string),
	}
	if parent != nil {
		span.ParentID = parent.SpanID
	}
	return context.WithValue(ctx, spanKey, span), span
}

func FromContext(ctx context.Context) *Span {
	s, _ := ctx.Value(spanKey).(*Span)
	return s
}

// ExtractTraceparent reads a W3C traceparent header and stores the trace-id
// and parent-id in a synthetic parent span so that Start inherits them.
// If the header is absent or malformed the context is returned unchanged.
func ExtractTraceparent(ctx context.Context, h http.Header) context.Context {
	v := h.Get("traceparent")
	if v == "" {
		return ctx
	}
	parts := strings.Split(v, "-")
	if len(parts) != 4 || parts[0] != "00" {
		return ctx
	}
	traceID, parentID := parts[1], parts[2]
	if len(traceID) != 32 || len(parentID) != 16 {
		return ctx
	}
	stub := &Span{TraceID: traceID, SpanID: parentID}
	return context.WithValue(ctx, spanKey, stub)
}

// InjectTraceparent writes a W3C traceparent header derived from the active
// span in ctx. No-ops if there is no active span.
func InjectTraceparent(ctx context.Context, h http.Header) {
	s := FromContext(ctx)
	if s == nil {
		return
	}
	// W3C format: 00-<32-hex traceID>-<16-hex spanID>-01
	traceID := strings.ReplaceAll(s.TraceID, "-", "")
	spanID := strings.ReplaceAll(s.SpanID, "-", "")
	if len(spanID) > 16 {
		spanID = spanID[:16]
	}
	h.Set("traceparent", fmt.Sprintf("00-%s-%s-01", traceID, spanID))
}

func newTraceID(parent *Span) string {
	if parent != nil {
		return parent.TraceID
	}
	return uuid.NewString()
}
