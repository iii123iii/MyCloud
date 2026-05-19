package app

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/httpapi"
)

// peekJSONField extracts a single top-level string field from a JSON request
// body, then restores r.Body so the underlying handler can re-read it.
// Returns "" on any error.
func peekJSONField(r *http.Request, field string) string {
	if r.Body == nil {
		return ""
	}
	// Cap the read at 64 KiB — login/register payloads are tiny.
	buf, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		return ""
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	var m map[string]json.RawMessage
	if err := json.Unmarshal(buf, &m); err != nil {
		return ""
	}
	raw, ok := m[field]
	if !ok {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(s))
}

// rateLimitScript atomically increments a counter and returns (count, ttl).
// On the first hit (count==1) it sets the expiry to window seconds.
//
// Returns [current_count, ttl_seconds].
var rateLimitScript = redis.NewScript(`
local key = KEYS[1]
local window = tonumber(ARGV[1])
local n = redis.call('INCR', key)
if n == 1 then
    redis.call('EXPIRE', key, window)
end
local ttl = redis.call('TTL', key)
if ttl < 0 then
    redis.call('EXPIRE', key, window)
    ttl = window
end
return {n, ttl}
`)

// keyFn extracts the bucket key for a given request. Receives the configured
// bucket name so multiple buckets can share a keyer without collision.
type keyFn func(r *http.Request, bucket string) string

// rateLimit returns middleware enforcing capacity hits per window for the bucket.
// If Redis is unavailable, the middleware fails open (logs nothing — we'd rather
// serve traffic than block on a Redis blip).
func (a *App) rateLimit(bucket string, capacity int, window time.Duration, key keyFn) func(http.Handler) http.Handler {
	if capacity <= 0 || window <= 0 {
		// Disabled — pass through.
		return func(next http.Handler) http.Handler { return next }
	}
	windowSec := int(window.Seconds())
	if windowSec < 1 {
		windowSec = 1
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			id := key(r, bucket)
			if id == "" {
				next.ServeHTTP(w, r)
				return
			}
			ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
			defer cancel()
			res, err := rateLimitScript.Run(ctx, a.Redis, []string{"rl:" + bucket + ":" + id}, windowSec).Result()
			if err != nil {
				// Fail open on Redis failure.
				next.ServeHTTP(w, r)
				return
			}
			arr, ok := res.([]any)
			if !ok || len(arr) < 2 {
				next.ServeHTTP(w, r)
				return
			}
			count, _ := arr[0].(int64)
			ttl, _ := arr[1].(int64)
			if int(count) > capacity {
				w.Header().Set("Retry-After", strconv.FormatInt(ttl, 10))
				// Record the block so admins can spot attacks.
				uid := userIDFrom(r)
				var uidPtr *string
				if uid != "" {
					uidPtr = &uid
				}
				writeActivity(r.Context(), a.DB, uidPtr, "rate_limit_block", "system", "", clientIP(r),
					map[string]any{"bucket": bucket, "key": id})
				httpapi.Error(w, http.StatusTooManyRequests, "rate_limited",
					fmt.Sprintf("Too many requests. Retry in %ds.", ttl))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// keyByIP buckets requests by remote IP.
func keyByIP(r *http.Request, _ string) string { return clientIP(r) }

// keyByLoginUsername buckets by lowercased email/username from the JSON body.
// We sneak the body out by parsing it WITHOUT consuming the underlying request
// body — instead the middleware peeks at a JSON field we expect.
//
// NOTE: this requires the caller to use json.NewDecoder against r.Body AFTER
// we restore it. Since the handler still uses decodeJSON which reads fresh,
// we duplicate the body buffer here.
func keyByLoginField(field string) keyFn {
	return func(r *http.Request, _ string) string {
		v := peekJSONField(r, field)
		if v == "" {
			return ""
		}
		return v
	}
}
