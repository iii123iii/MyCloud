// Package cluster splits a logical database into a write-primary and an
// optional read-replica. Handlers call Read(ctx) for SELECTs and Write(ctx)
// for everything else; the routing logic returns the right *sql.DB.
//
// Falls back to the primary when no replica is configured, so every
// existing handler "just works" without changes; the migration to call
// Read() vs Write() is gradual.
//
// Read-after-write: writes set a short-lived Redis key keyed by the user's
// ID. Reads check the key and force-promote to the primary while it's hot.
// This prevents the "I uploaded a file and don't see it" footgun without
// requiring full strong consistency.
package cluster

import (
	"context"
	"database/sql"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cluster wires a primary + optional replica with a Redis-backed
// read-after-write sentinel.
type Cluster struct {
	Primary *sql.DB
	Replica *sql.DB // nil → all reads go to primary
	Redis   *redis.Client
	// RAWTTL is how long after a write a user's subsequent reads are pinned
	// to the primary. 5 s is a generous default — typical replica lag on
	// the same network is under 1 s.
	RAWTTL time.Duration
}

// New constructs a Cluster. Pass nil replica for single-instance mode.
func New(primary, replica *sql.DB, redisClient *redis.Client) *Cluster {
	return &Cluster{
		Primary: primary,
		Replica: replica,
		Redis:   redisClient,
		RAWTTL:  5 * time.Second,
	}
}

// Read returns the right pool for a SELECT. The ctx-attached userIDKey is
// checked against the Redis sentinel; on hit, primary is returned so the
// user's own recent writes are visible.
func (c *Cluster) Read(ctx context.Context, userID string) *sql.DB {
	if c.Replica == nil {
		return c.Primary
	}
	if userID == "" {
		return c.Replica
	}
	// Check the read-after-write sentinel. Errors fail open to the replica;
	// we'd rather serve a slightly stale read than 500 the page on a Redis
	// blip.
	if c.Redis != nil {
		if n, err := c.Redis.Exists(ctx, "raw:"+userID).Result(); err == nil && n > 0 {
			return c.Primary
		}
	}
	return c.Replica
}

// Write returns the primary and marks the user's reads to bypass the
// replica for RAWTTL. Call this BEFORE running the actual UPDATE/INSERT/
// DELETE so a concurrent read on the same user sees the sentinel in time.
func (c *Cluster) Write(ctx context.Context, userID string) *sql.DB {
	if c.Redis != nil && userID != "" {
		_ = c.Redis.Set(ctx, "raw:"+userID, 1, c.RAWTTL).Err()
	}
	return c.Primary
}
