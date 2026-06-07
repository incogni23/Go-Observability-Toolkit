package correlation

import (
	"context"
	"net/http"

	"github.com/google/uuid"
)

const Header = "X-Correlation-ID"

type contextKey string

const correlationKey contextKey = "obskit_correlation_id"

func New() string { return uuid.NewString() }

func WithID(ctx context.Context, id string) context.Context {
	if id == "" {
		id = New()
	}
	return context.WithValue(ctx, correlationKey, id)
}

func FromContext(ctx context.Context) string {
	id, _ := ctx.Value(correlationKey).(string)
	return id
}

func FromRequest(r *http.Request) (string, *http.Request) {
	id := r.Header.Get(Header)
	if id == "" {
		id = New()
	}
	return id, r.WithContext(WithID(r.Context(), id))
}

func InjectHeader(ctx context.Context, h http.Header) {
	if id := FromContext(ctx); id != "" {
		h.Set(Header, id)
	}
}
