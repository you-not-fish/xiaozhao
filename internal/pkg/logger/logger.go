// Package logger provides a globally reachable *zap.Logger built from
// config.LogConfig. The logger is structured (JSON or console) and attaches
// caller information when requested. Contextual loggers can be derived via
// WithRequestID / WithTenant / WithUser to carry trace-related fields.
package logger

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"

	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
)

type ctxKey struct{}

var loggerCtxKey = ctxKey{}

// New builds a *zap.Logger from cfg.
func New(cfg config.LogConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		return nil, fmt.Errorf("parse log level: %w", err)
	}

	encoderCfg := zap.NewProductionEncoderConfig()
	encoderCfg.TimeKey = "ts"
	encoderCfg.EncodeTime = zapcore.ISO8601TimeEncoder
	encoderCfg.EncodeDuration = zapcore.MillisDurationEncoder

	var encoder zapcore.Encoder
	switch cfg.Encoding {
	case "console", "":
		encoderCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(encoderCfg)
	case "json":
		encoderCfg.EncodeLevel = zapcore.LowercaseLevelEncoder
		encoder = zapcore.NewJSONEncoder(encoderCfg)
	default:
		return nil, fmt.Errorf("unsupported log encoding: %s", cfg.Encoding)
	}

	core := zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(writer())), level)
	opts := []zap.Option{zap.AddStacktrace(zapcore.ErrorLevel)}
	if cfg.Caller {
		opts = append(opts, zap.AddCaller(), zap.AddCallerSkip(0))
	}
	return zap.New(core, opts...), nil
}

// From returns the logger stored in ctx, falling back to the no-op logger.
func From(ctx context.Context) *zap.Logger {
	if l, ok := ctx.Value(loggerCtxKey).(*zap.Logger); ok && l != nil {
		return l
	}
	return zap.NewNop()
}

// Into stores lg in ctx for later retrieval with From.
func Into(ctx context.Context, lg *zap.Logger) context.Context {
	return context.WithValue(ctx, loggerCtxKey, lg)
}
