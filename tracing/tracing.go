// Package tracing provides lightweight per-request spans (TraceID, SpanID,
// ParentID, tags, and wall-clock duration) without requiring a distributed
// tracing backend. Spans are propagated through context.Context.
package tracing

import (
	"context"
	"time"

	"github.com/google/uuid"
)

type contextKey string

const spanKey contextKey = "obskit_span"

// Span represents a single unit of work.
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

// Duration returns how long the span took. Zero until End is called.
func (s *Span) Duration() time.Duration {
	if s.EndedAt.IsZero() {
		return 0
	}
	return s.EndedAt.Sub(s.StartedAt)
}

// SetTag adds or replaces a tag on the span.
func (s *Span) SetTag(key, value string) {
	if s.Tags == nil {
		s.Tags = make(map[string]string)
	}
	s.Tags[key] = value
}

// End marks the span as finished.
func (s *Span) End() {
	s.EndedAt = time.Now()
}

// Finish is an alias for End that also records an error.
func (s *Span) Finish(err error) {
	s.Err = err
	s.End()
}

// Start creates a new root span and stores it in ctx.
func Start(ctx context.Context, operation string) (context.Context, *Span) {
	parent := FromContext(ctx)
	span := &Span{
		TraceID:   traceID(parent),
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

// FromContext returns the active span or nil.
func FromContext(ctx context.Context) *Span {
	if s, ok := ctx.Value(spanKey).(*Span); ok {
		return s
	}
	return nil
}

func traceID(parent *Span) string {
	if parent != nil {
		return parent.TraceID
	}
	return uuid.NewString()
}
