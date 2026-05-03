// Package database wires GORM to PostgreSQL with sane pool defaults and a
// zap-backed GORM logger. The returned *gorm.DB is the canonical handle
// injected into repository implementations.
package database

import (
	"context"
	"fmt"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
)

// Open opens a PostgreSQL connection pool configured per cfg.
func Open(ctx context.Context, cfg config.PostgresConfig, lg *zap.Logger) (*gorm.DB, error) {
	gormLg := newZapGormLogger(lg)
	db, err := gorm.Open(postgres.Open(cfg.DSN), &gorm.Config{
		Logger:                                   gormLg,
		DisableForeignKeyConstraintWhenMigrating: false,
		PrepareStmt:                              true,
	})
	if err != nil {
		return nil, fmt.Errorf("database: open: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("database: raw handle: %w", err)
	}
	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(cfg.ConnMaxLifetime())

	pingCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := sqlDB.PingContext(pingCtx); err != nil {
		return nil, fmt.Errorf("database: ping: %w", err)
	}
	return db, nil
}

// Close closes the pool underlying db.
func Close(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

type zapGormLogger struct {
	lg            *zap.Logger
	slowThreshold time.Duration
}

func newZapGormLogger(lg *zap.Logger) gormlogger.Interface {
	return &zapGormLogger{lg: lg.WithOptions(zap.AddCallerSkip(3)), slowThreshold: 200 * time.Millisecond}
}

func (l *zapGormLogger) LogMode(gormlogger.LogLevel) gormlogger.Interface { return l }

func (l *zapGormLogger) Info(_ context.Context, msg string, data ...any) {
	l.lg.Sugar().Infof(msg, data...)
}

func (l *zapGormLogger) Warn(_ context.Context, msg string, data ...any) {
	l.lg.Sugar().Warnf(msg, data...)
}

func (l *zapGormLogger) Error(_ context.Context, msg string, data ...any) {
	l.lg.Sugar().Errorf(msg, data...)
}

func (l *zapGormLogger) Trace(_ context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	elapsed := time.Since(begin)
	sql, rows := fc()
	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql", sql),
	}
	switch {
	case err != nil:
		l.lg.Warn("gorm.trace", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold:
		l.lg.Warn("gorm.slow", fields...)
	default:
		l.lg.Debug("gorm.trace", fields...)
	}
}
