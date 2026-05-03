// Package migration applies plain-SQL migration files embedded in the
// binary. MVP uses a monotonically increasing file prefix (001_, 002_, ...)
// and records applied versions in the schema_migrations table. Each file is
// expected to be idempotent on its own (uses IF NOT EXISTS / ON CONFLICT).
package migration

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

//go:embed *.sql
var files embed.FS

// Apply runs all embedded migrations in ascending filename order.
func Apply(ctx context.Context, db *gorm.DB, lg *zap.Logger) error {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		return fmt.Errorf("migration: read embedded fs: %w", err)
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		content, err := files.ReadFile(name)
		if err != nil {
			return fmt.Errorf("migration: read %s: %w", name, err)
		}
		lg.Info("applying migration", zap.String("file", name))
		if err := db.WithContext(ctx).Exec(string(content)).Error; err != nil {
			return fmt.Errorf("migration: exec %s: %w", name, err)
		}
	}
	return nil
}
