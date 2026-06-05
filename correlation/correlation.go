// Package correlation manages correlation IDs that flow across service boundaries
// via HTTP headers and context values.
package correlation

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

// Header is the canonical HTTP header name used to propagate correlation IDs.
const Header = "X-Correlation-ID"

type contextKey string

const correlationKey contextKey = "obskit_correlation_id"

// New generates a new random correlation ID.
func New() string {
	return uuid.NewString()
}

// WithID stores id in ctx. If id is empty a new one is generated.
func WithID(ctx context.Context, id string) context.Context {
	if id == "" {
		id = New()
	}
	return context.WithValue(ctx, correlationKey, id)
}

// FromContext returns the correlation ID stored in ctx, or an empty string.
func FromContext(ctx context.Context) string {
	if id, ok := ctx.Value(correlationKey).(string); ok {
		return id
	}
	return ""
}

// FromRequest extracts the correlation ID from the request header.
// If absent, a new ID is generated and injected into the request context.
func FromRequest(r *http.Request) (string, *http.Request) {
	id := r.Header.Get(Header)
	if id == "" {
		id = New()
	}
	return id, r.WithContext(WithID(r.Context(), id))
}

// InjectHeader writes the correlation ID from ctx into h.
func InjectHeader(ctx context.Context, h http.Header) {
	if id := FromContext(ctx); id != "" {
		h.Set(Header, id)
	}
}
