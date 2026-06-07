package logger

import (
	"context"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type contextKey string

const loggerKey contextKey = "obskit_logger"

var (
	ctxMu     sync.RWMutex
	ctxFields []func(context.Context) zap.Field

	defaultOnce   sync.Once
	defaultLogger *Logger
)

func RegisterContextFields(fn ...func(context.Context) zap.Field) {
	ctxMu.Lock()
	ctxFields = append(ctxFields, fn...)
	ctxMu.Unlock()
}

type Logger struct {
	z *zap.Logger
}

type Config struct {
	Level      string
	JSON       bool
	AddCaller  bool
	StackTrace bool
}

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

// Must panics if err is non-nil. Intended for use at process startup:
//
//	log := logger.Must(logger.New(cfg))
func Must(l *Logger, err error) *Logger {
	if err != nil {
		panic("obskit/logger: " + err.Error())
	}
	return l
}

func Default() *Logger {
	defaultOnce.Do(func() {
		defaultLogger = Must(New(Config{Level: "info", JSON: true, AddCaller: true}))
	})
	return defaultLogger
}

func (l *Logger) WithContext(ctx context.Context) context.Context {
	return context.WithValue(ctx, loggerKey, l)
}

func FromContext(ctx context.Context) *Logger {
	base := Default()
	if l, ok := ctx.Value(loggerKey).(*Logger); ok {
		base = l
	}
	ctxMu.RLock()
	fns := ctxFields
	ctxMu.RUnlock()
	if len(fns) == 0 {
		return base
	}
	fields := make([]zap.Field, 0, len(fns))
	for _, fn := range fns {
		if f := fn(ctx); f.Key != "" {
			fields = append(fields, f)
		}
	}
	if len(fields) == 0 {
		return base
	}
	return &Logger{z: base.z.With(fields...)}
}

func (l *Logger) With(fields ...zap.Field) *Logger     { return &Logger{z: l.z.With(fields...)} }
func (l *Logger) WithFields(kv ...any) *Logger         { return &Logger{z: l.z.Sugar().With(kv...).Desugar()} }
func (l *Logger) Debug(msg string, f ...zap.Field)     { l.z.Debug(msg, f...) }
func (l *Logger) Info(msg string, f ...zap.Field)      { l.z.Info(msg, f...) }
func (l *Logger) Warn(msg string, f ...zap.Field)      { l.z.Warn(msg, f...) }
func (l *Logger) Error(msg string, f ...zap.Field)     { l.z.Error(msg, f...) }
func (l *Logger) Fatal(msg string, f ...zap.Field)     { l.z.Fatal(msg, f...) }
func (l *Logger) Sync() error                          { return l.z.Sync() }
func (l *Logger) Zap() *zap.Logger                     { return l.z }
