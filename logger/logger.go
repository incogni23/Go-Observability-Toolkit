// Package logger provides structured, leveled logging built on top of zap.
// When a logger is retrieved via FromContext it automatically attaches the
// correlation ID and trace ID stored in that context as log fields.
package logger

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const loggerKey contextKey = "obskit_logger"

// ctxFields is a function type so the logger package can pull correlation and
// trace values without importing those packages (which would create a cycle).
// Register your extractors once at process startup via RegisterContextFields.
var ctxFields []func(context.Context) zap.Field

// RegisterContextFields appends field extractors that are called every time
// FromContext returns a logger. Use this to auto-attach correlation IDs, trace
// IDs, or any other per-request value from context.
func RegisterContextFields(fn ...func(context.Context) zap.Field) {
	ctxFields = append(ctxFields, fn...)
}

// Logger wraps zap.Logger with context-aware helpers.
type Logger struct {
	z *zap.Logger
}

// Config controls logger initialisation.
type Config struct {
	Level      string // debug, info, warn, error  (default: "info")
	JSON       bool   // true = JSON lines, false = console  (default: true)
	AddCaller  bool
	StackTrace bool
}

// New creates a Logger from Config.
func New(cfg Config) (*Logger, error) {
	if cfg.Level == "" {
		cfg.Level = "info"
	}

	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.InfoLevel
	}

	var zapCfg zap.Config
	if cfg.JSON {
		zapCfg = zap.NewProductionConfig()
	} else {
		zapCfg = zap.NewDevelopmentConfig()
	}
	zapCfg.Level = zap.NewAtomicLevelAt(level)
	zapCfg.DisableCaller = !cfg.AddCaller
	if !cfg.StackTrace {
		zapCfg.DisableStacktrace = true
	}

	z, err := zapCfg.Build()
	if err != nil {
		return nil, err
	}
	return &Logger{z: z}, nil
}

// Default returns a ready-to-use JSON/info logger.
func Default() *Logger {
	l, _ := New(Config{Level: "info", JSON: true, AddCaller: true})
	return l
}

// WithContext stores the logger in ctx so child components can retrieve it.
func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

// FromContext retrieves the logger stored by WithContext and enriches it with
// any fields registered via RegisterContextFields (e.g. correlation_id,
// trace_id). Falls back to Default() if no logger is in ctx.
func FromContext(ctx context.Context) *Logger {
	base := Default()
	if l, ok := ctx.Value(loggerKey).(*Logger); ok {
		base = l
	}
	if len(ctxFields) == 0 {
		return base
	}
	fields := make([]zap.Field, 0, len(ctxFields))
	for _, fn := range ctxFields {
		if f := fn(ctx); f.Key != "" {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return base
	}
	return &Logger{z: base.z.With(fields...)}
}

// With returns a child logger with permanent fields.
func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{z: l.z.With(fields...)}
}

// WithFields returns a child logger enriched with key-value pairs.
func (l *Logger) WithFields(keysAndValues ...any) *Logger {
	return &Logger{z: l.z.Sugar().With(keysAndValues...).Desugar()}
}

func (l *Logger) Debug(msg string, fields ...zap.Field) { l.z.Debug(msg, fields...) }
func (l *Logger) Info(msg string, fields ...zap.Field)  { l.z.Info(msg, fields...) }
func (l *Logger) Warn(msg string, fields ...zap.Field)  { l.z.Warn(msg, fields...) }
func (l *Logger) Error(msg string, fields ...zap.Field) { l.z.Error(msg, fields...) }
func (l *Logger) Fatal(msg string, fields ...zap.Field) { l.z.Fatal(msg, fields...) }

// Sync flushes buffered log entries.
func (l *Logger) Sync() error { return l.z.Sync() }

// Zap exposes the underlying *zap.Logger for interop.
func (l *Logger) Zap() *zap.Logger { return l.z }
