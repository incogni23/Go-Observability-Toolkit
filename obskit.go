package obskit

import (
	"context"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/incogni23/obskit/correlation"
	"github.com/incogni23/obskit/health"
	"github.com/incogni23/obskit/logger"
	"github.com/incogni23/obskit/metrics"
	"github.com/incogni23/obskit/middleware"
	"github.com/incogni23/obskit/tracing"
)

type Config struct {
	// LogLevel is one of debug, info, warn, error. Default: "info".
	LogLevel string
	// LogJSON selects JSON output. Default: true. Pass LogJSON(false) for console.
	LogJSON      *bool
	LogCaller    bool
	CheckTimeout time.Duration
}

type Obskit struct {
	Logger  *logger.Logger
	Metrics *metrics.Registry
	Health  *health.Registry
}

var registerOnce sync.Once

func New(cfg Config) *Obskit {
	if cfg.LogLevel == "" {
		cfg.LogLevel = "info"
	}
	json := true
	if cfg.LogJSON != nil {
		json = *cfg.LogJSON
	}
	log := logger.Must(logger.New(logger.Config{
		Level:     cfg.LogLevel,
		JSON:      json,
		AddCaller: cfg.LogCaller,
	}))
	registerOnce.Do(func() {
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
			func(ctx context.Context) zap.Field {
				if s := tracing.FromContext(ctx); s != nil {
					return zap.String("span_id", s.SpanID)
				}
				return zap.Field{}
			},
		)
	})
	return &Obskit{
		Logger:  log,
		Metrics: metrics.New(),
		Health:  health.New(cfg.CheckTimeout),
	}
}

func LogJSON(v bool) *bool { return &v }

func (o *Obskit) Middleware(h http.Handler) http.Handler {
	return middleware.Chain(h,
		middleware.Correlation,
		middleware.Tracing,
		middleware.Logging(o.Logger),
		middleware.Metrics(o.Metrics),
	)
}
