package tracing_test

import (
	"context"
	"testing"
	"time"

	"github.com/incogni23/obskit/tracing"
)

func TestStartCreatesSpan(t *testing.T) {
	ctx, span := tracing.Start(context.Background(), "my-op")
	if span == nil {
		t.Fatal("span is nil")
	}
	if span.Operation != "my-op" {
		t.Fatalf("want my-op, got %s", span.Operation)
	}
	if tracing.FromContext(ctx) != span {
		t.Fatal("span not stored in context")
	}
}

func TestChildSpan_inheritsTraceID_and_setsParentID(t *testing.T) {
	rootCtx, root := tracing.Start(context.Background(), "root")
	if root.TraceID == "" {
		t.Fatal("root TraceID must not be empty")
	}

	// Start the child from the context that holds the root span.
	_, child := tracing.Start(rootCtx, "child")

	if child.TraceID != root.TraceID {
		t.Fatalf("child TraceID %q must match root %q", child.TraceID, root.TraceID)
	}
	if child.ParentID != root.SpanID {
		t.Fatalf("child ParentID %q must equal root SpanID %q", child.ParentID, root.SpanID)
	}
	if child.SpanID == root.SpanID {
		t.Fatal("child and root must have distinct SpanIDs")
	}
}

func TestIndependentRoots_haveDifferentTraceIDs(t *testing.T) {
	_, a := tracing.Start(context.Background(), "a")
	_, b := tracing.Start(context.Background(), "b")
	if a.TraceID == b.TraceID {
		t.Fatal("independent roots must not share a TraceID")
	}
}

func TestSpanDuration(t *testing.T) {
	_, span := tracing.Start(context.Background(), "timed")
	time.Sleep(5 * time.Millisecond)
	span.End()
	if span.Duration() < 5*time.Millisecond {
		t.Fatalf("duration too short: %v", span.Duration())
	}
}

func TestFinishRecordsError(t *testing.T) {
	_, span := tracing.Start(context.Background(), "err-op")
	span.Finish(context.DeadlineExceeded)
	if span.Err != context.DeadlineExceeded {
		t.Fatalf("want DeadlineExceeded, got %v", span.Err)
	}
	if span.EndedAt.IsZero() {
		t.Fatal("Finish must set EndedAt")
	}
}

func TestSetTag(t *testing.T) {
	_, span := tracing.Start(context.Background(), "tagged")
	span.SetTag("user_id", "u-42")
	if span.Tags["user_id"] != "u-42" {
		t.Fatalf("tag not set: %v", span.Tags)
	}
}
