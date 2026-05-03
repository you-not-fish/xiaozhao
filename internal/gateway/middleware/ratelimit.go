package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/xiaozhao/xiaozhao/internal/api/response"
	"github.com/xiaozhao/xiaozhao/internal/gateway/httpctx"
	"github.com/xiaozhao/xiaozhao/internal/pkg/config"
	"github.com/xiaozhao/xiaozhao/internal/pkg/errcode"
)

// RateLimit enforces per-user and per-IP fixed-window limits using Redis
// INCR + EXPIRE. A missing Redis (rate limit disabled or startup failure) is
// treated as fail-open; we log but do not reject traffic because that would
// make the whole API unavailable on a cache blip.
func RateLimit(rdb *redis.Client, cfg config.RateLimitConfig) func(http.Handler) http.Handler {
	if !cfg.Enabled || rdb == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	window := time.Minute

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			bucket := time.Now().Truncate(window).Unix()

			if cfg.PerUserPerMinute > 0 {
				if uid, err := httpctx.UserID(ctx); err == nil && uid != "" {
					key := fmt.Sprintf("rl:u:%s:%d", uid, bucket)
					if over, retryAfter := exceed(ctx, rdb, key, cfg.PerUserPerMinute, window); over {
						writeRateLimited(ctx, w, retryAfter)
						return
					}
				}
			}
			if cfg.PerIPPerMinute > 0 {
				ip := clientIPForRateLimit(r)
				if ip != "" {
					key := fmt.Sprintf("rl:ip:%s:%d", ip, bucket)
					if over, retryAfter := exceed(ctx, rdb, key, cfg.PerIPPerMinute, window); over {
						writeRateLimited(ctx, w, retryAfter)
						return
					}
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// exceed returns whether the key is over the limit for the current window.
// On a Redis error we return (false, 0) so the chain is fail-open.
func exceed(ctx context.Context, rdb *redis.Client, key string, limit int, window time.Duration) (bool, time.Duration) {
	pipe := rdb.Pipeline()
	incr := pipe.Incr(ctx, key)
	pipe.Expire(ctx, key, window)
	if _, err := pipe.Exec(ctx); err != nil {
		return false, 0
	}
	if incr.Val() > int64(limit) {
		ttl, err := rdb.TTL(ctx, key).Result()
		if err != nil || ttl < 0 {
			ttl = window
		}
		return true, ttl
	}
	return false, 0
}

func writeRateLimited(ctx context.Context, w http.ResponseWriter, retryAfter time.Duration) {
	if retryAfter > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
	}
	response.WriteError(ctx, w, errcode.New(errcode.CodeRateLimited, "rate limit exceeded"))
}

func clientIPForRateLimit(r *http.Request) string {
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// Take the first hop.
		if idx := strings.IndexByte(v, ','); idx > 0 {
			return strings.TrimSpace(v[:idx])
		}
		return strings.TrimSpace(v)
	}
	if v := r.Header.Get("X-Real-Ip"); v != "" {
		return strings.TrimSpace(v)
	}
	// Strip port from RemoteAddr.
	addr := r.RemoteAddr
	if idx := strings.LastIndexByte(addr, ':'); idx > 0 {
		return addr[:idx]
	}
	return addr
}
