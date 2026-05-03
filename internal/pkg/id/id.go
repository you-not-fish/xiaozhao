// Package id produces prefixed, k-sortable identifiers using ULID under the
// hood. Format: "{prefix}_{26-char-base32-ulid}", e.g. "user_01HQZX8K4AJ5YB...".
// The prefix carries the entity type, which aids log/trace readability and
// protects against cross-resource ID reuse.
package id

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/oklog/ulid/v2"
)

// Prefix enumerates resource ID prefixes used across the service.
type Prefix string

const (
	PrefixUser         Prefix = "user"
	PrefixOrg          Prefix = "org"
	PrefixProject      Prefix = "proj"
	PrefixConversation Prefix = "conv"
	PrefixMessage      Prefix = "msg"
	PrefixResponse     Prefix = "resp"
	PrefixEvent        Prefix = "evt"
	PrefixToolCall     Prefix = "call"
	PrefixToolDef      Prefix = "tool"
	PrefixFile         Prefix = "file"
	PrefixKnowledge    Prefix = "kb"
	PrefixDocument     Prefix = "doc"
	PrefixChunk        Prefix = "chk"
	PrefixTrace        Prefix = "trace"
	PrefixSpan         Prefix = "span"
	PrefixAudit        Prefix = "aud"
	PrefixRequest      Prefix = "req"
	PrefixSecret       Prefix = "sec"
	PrefixFeedback     Prefix = "fb"
)

var (
	mu      sync.Mutex
	entropy = ulid.Monotonic(rand.Reader, 0)
)

// New returns a new prefixed identifier for the given resource.
func New(p Prefix) string {
	mu.Lock()
	defer mu.Unlock()
	u := ulid.MustNew(ulid.Timestamp(time.Now()), entropy)
	return string(p) + "_" + u.String()
}

// Parse splits a prefixed identifier into its prefix and raw ULID parts.
func Parse(s string) (Prefix, ulid.ULID, error) {
	idx := strings.LastIndex(s, "_")
	if idx <= 0 || idx == len(s)-1 {
		return "", ulid.ULID{}, errors.New("id: malformed, expected {prefix}_{ulid}")
	}
	prefix := Prefix(s[:idx])
	raw := s[idx+1:]
	u, err := ulid.Parse(raw)
	if err != nil {
		return "", ulid.ULID{}, fmt.Errorf("id: parse ulid: %w", err)
	}
	return prefix, u, nil
}

// HasPrefix checks whether s is a prefixed id with the expected prefix.
func HasPrefix(s string, want Prefix) bool {
	p, _, err := Parse(s)
	return err == nil && p == want
}
