package repository

import (
	"errors"
	"strings"

	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// isUniqueViolation reports whether err represents a PostgreSQL unique_violation (23505)
// or a generic string that looks like one. We keep both paths so the code stays
// correct even if GORM's underlying driver changes.
func isUniqueViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	msg := err.Error()
	return strings.Contains(msg, "duplicate key") || strings.Contains(msg, "unique constraint")
}

// isFKViolation reports whether err represents a FK violation (23503).
func isFKViolation(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23503"
	}
	return false
}

// notFound is kept around so future repositories can share the same helper
// instead of inlining errors.Is(err, gorm.ErrRecordNotFound).
func notFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}
