package redisclient

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Client wraps go-redis for token blacklisting and rate limiting.
type Client struct {
	rdb *redis.Client
}

func New(host, port string) *Client {
	rdb := redis.NewClient(&redis.Options{
		Addr:         fmt.Sprintf("%s:%s", host, port),
		DialTimeout:  2 * time.Second,
		ReadTimeout:  2 * time.Second,
		WriteTimeout: 2 * time.Second,
	})
	return &Client{rdb: rdb}
}

// Setex stores a key with TTL.
func (c *Client) Setex(ctx context.Context, key string, ttl time.Duration) error {
	return c.rdb.Set(ctx, key, "1", ttl).Err()
}

// Exists returns true if key is present.
func (c *Client) Exists(ctx context.Context, key string) bool {
	n, err := c.rdb.Exists(ctx, key).Result()
	return err == nil && n > 0
}

// Incr atomically increments a counter and sets TTL if it's the first increment.
// Uses a Lua script to make INCR + EXPIRE atomic (no race window).
var rateLimitScript = redis.NewScript(`
local v = redis.call('INCR', KEYS[1])
if v == 1 then redis.call('EXPIRE', KEYS[1], ARGV[1]) end
return v
`)

func (c *Client) Incr(ctx context.Context, key string, ttlSeconds int) (int64, error) {
	res, err := rateLimitScript.Run(ctx, c.rdb, []string{key}, ttlSeconds).Int64()
	return res, err
}
