// Package obskit is a zero-ceremony observability toolkit for Go services.
// It bundles structured logging, correlation IDs, request tracing, Prometheus
// metrics, and health checks — everything most production services need.
//
// Quick-start:
//
//	obs := obskit.New(obskit.Config{})
//	http.Handle("/metrics", obs.Metrics.HTTPHandler())
//	http.Handle("/healthz", obs.Health.LivenessHandler())
//	http.Handle("/readyz",  obs.Health.ReadinessHandler())
//	http.Handle("/", obs.Middleware(yourHandler))
package obskit

import (
	"context"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/incogni23/obskit/correlation"
	"github.com/incogni23/obskit/health"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/metrics"
	"github.com/incogni23/obskit/middleware"
	"github.com/incogni23/obskit/tracing"
)

// Config holds top-level configuration for the toolkit.
// All fields are optional; zero values produce safe, production-ready defaults.
type Config struct {
	// LogLevel is one of debug, info, warn, error. Default: "info".
	LogLevel string
	// LogJSON controls output format. Default: true (JSON lines).
	// Set to false for human-readable console output during development.
	LogJSON *bool
	// LogCaller includes caller file:line in log output.
	LogCaller bool
	// CheckTimeout is the per-health-check deadline. Default: 5 s.
	CheckTimeout time.Duration
}

// Obskit is the root object — it owns all sub-components.
type Obskit struct {
	Logger  *logger.Logger
	Metrics *metrics.Registry
	Health  *health.Registry
}

// New initialises the toolkit from cfg, applying defaults for unset fields.
func New(cfg Config) *Obskit {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	json := true
	if cfg.LogJSON != nil {
		json = *cfg.LogJSON
	}

	log, _ := logger.New(logger.Config{
		Level:     cfg.LogLevel,
		JSON:      json,
		AddCaller: cfg.LogCaller,
	})

	// Wire context-field extractors so logger.FromContext automatically attaches
	// correlation_id and trace_id without any manual zap.Field plumbing.
	logger.RegisterContextFields(
		func(ctx context.Context) zap.Field {
			if id := correlation.FromContext(ctx); id != "" {
				return zap.String("correlation_id", id)
			}
			return zap.Field{}
		},
		func(ctx context.Context) zap.Field {
			if s := tracing.FromContext(ctx); s != nil {
				return zap.String("trace_id", s.TraceID)
			}
			return zap.Field{}
		},
	)

	return &Obskit{
		Logger:  log,
		Metrics: metrics.New(),
		Health:  health.New(cfg.CheckTimeout),
	}
}

// LogJSON is a convenience helper to get a *bool for Config.LogJSON.
//
//	obskit.New(obskit.Config{LogJSON: obskit.LogJSON(false)})
func LogJSON(v bool) *bool { return &v }

// Middleware wraps h with correlation, tracing, structured logging, and metrics.
func (o *Obskit) Middleware(h http.Handler) http.Handler {
	return middleware.Chain(h,
		middleware.Correlation,
		middleware.Tracing,
		middleware.Logging(o.Logger),
		middleware.Metrics(o.Metrics),
	)
}
