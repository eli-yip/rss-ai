package mlog

import (
	"context"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/eli-yip/rss-ai/pkg/ctxutil"
)

type TraceIDKey struct{}

// TraceIDCtxKey is the context key under which the per-request trace id lives.
var TraceIDCtxKey = TraceIDKey{}

// CopyTraceID carries the trace id from one context to another — used when a
// background goroutine outlives the originating request.
func CopyTraceID(from, to context.Context) context.Context {
	return context.WithValue(to, TraceIDCtxKey, ctxutil.ValueOr[uint64](from, TraceIDCtxKey, 0))
}

type Logger struct {
	*zap.Logger
	level zap.AtomicLevel
}

func (l *Logger) SetLevel(level zapcore.Level) { l.level.SetLevel(level) }

// New builds a logger that writes to stdout. env "dev" uses a colored console
// encoder for readability; any other env (e.g. "prod") uses JSON so Alloy can
// ship structured lines to Loki. service and env are baked in as base fields.
func New(env string, level zapcore.Level) *Logger {
	encCfg := zap.NewProductionEncoderConfig()
	encCfg.EncodeTime = zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05.000")

	var enc zapcore.Encoder
	if env == "dev" {
		encCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		enc = zapcore.NewConsoleEncoder(encCfg)
	} else {
		enc = zapcore.NewJSONEncoder(encCfg)
	}

	atom := zap.NewAtomicLevelAt(level)
	core := zapcore.NewCore(enc, zapcore.Lock(os.Stdout), atom)
	zl := zap.New(core, zap.AddCaller(), zap.AddStacktrace(zap.ErrorLevel)).
		With(zap.String("service", "rss-ai"), zap.String("env", env))

	return &Logger{Logger: zl, level: atom}
}

// L returns a logger with the request's trace_id attached.
func L(ctx context.Context, logger *Logger) *Logger {
	return &Logger{
		Logger: logger.Logger.With(zap.Uint64("trace_id", ctxutil.ValueOr[uint64](ctx, TraceIDCtxKey, 0))),
		level:  logger.level,
	}
}

// S returns a logger tagged with a service/component name.
func S(logger *Logger, service string) *Logger {
	return &Logger{
		Logger: logger.Logger.With(zap.String("service", service)),
		level:  logger.level,
	}
}

func (l *Logger) With(fields ...zap.Field) *Logger {
	return &Logger{Logger: l.Logger.With(fields...), level: l.level}
}

// NewNop returns a no-op logger for tests.
func NewNop() *Logger {
	return &Logger{Logger: zap.NewNop(), level: zap.NewAtomicLevelAt(zapcore.InfoLevel)}
}
