// Package repository holds GORM-backed implementations of the domain
// repository interfaces. These types own the mapping between plain domain
// structs (internal/domain) and GORM table rows.
package repository

import (
	"encoding/json"
	"time"
)

// jsonMap is a thin wrapper for storing map[string]any in a JSONB column.
type jsonMap map[string]any

// MarshalJSON / UnmarshalJSON are not needed because map[string]any already
// marshals to/from JSON correctly. GORM recognizes JSONB when the column
// type is declared explicitly in the struct tag.

func marshalSettings(m map[string]any) ([]byte, error) {
	if m == nil {
		return []byte(`{}`), nil
	}
	return json.Marshal(m)
}

func unmarshalSettings(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return map[string]any{}, nil
	}
	m := make(map[string]any)
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, err
	}
	return m, nil
}

// now returns the current wall-clock time; centralized so tests can override.
var now = func() time.Time { return time.Now() }
