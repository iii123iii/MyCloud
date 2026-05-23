package cluster

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// New() sets default RAWTTL.
func TestNew_SetsDefaultRAWTTL(t *testing.T) {
	c := New(nil, nil, nil)
	if c.RAWTTL != 5*time.Second {
		t.Errorf("expected default RAWTTL=5s, got %v", c.RAWTTL)
	}
}

// New() preserves the passed DBs.
func TestNew_AssignsFields(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	rdb, _, _ := sqlmock.New()
	defer rdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()

	c := New(pdb, rdb, rc)
	if c.Primary != pdb {
		t.Errorf("Primary mismatch")
	}
	if c.Replica != rdb {
		t.Errorf("Replica mismatch")
	}
	if c.Redis != rc {
		t.Errorf("Redis mismatch")
	}
}

// Read with nil replica → primary.
func TestRead_NilReplicaReturnsPrimary(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	c := New(pdb, nil, nil)
	if got := c.Read(context.Background(), "u-1"); got != pdb {
		t.Errorf("nil replica should fall back to primary")
	}
}

// Read with empty userID → replica (no sentinel to check).
func TestRead_EmptyUserGoesToReplica(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	rdb, _, _ := sqlmock.New()
	defer rdb.Close()
	c := New(pdb, rdb, nil)
	if got := c.Read(context.Background(), ""); got != rdb {
		t.Errorf("empty userID should hit replica")
	}
}

// Read with sentinel hit → primary.
func TestRead_SentinelHitReturnsPrimary(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	rdb, _, _ := sqlmock.New()
	defer rdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	c := New(pdb, rdb, rc)

	// Set the sentinel for user.
	mr.Set("raw:u-1", "1")
	if got := c.Read(context.Background(), "u-1"); got != pdb {
		t.Errorf("sentinel hit should return primary")
	}
}

// Read with no sentinel → replica.
func TestRead_NoSentinelReturnsReplica(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	rdb, _, _ := sqlmock.New()
	defer rdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	c := New(pdb, rdb, rc)
	if got := c.Read(context.Background(), "u-1"); got != rdb {
		t.Errorf("no sentinel should hit replica")
	}
}

// Read with Redis error → falls back to replica (fail-open).
func TestRead_RedisDownFallsBackToReplica(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	rdb, _, _ := sqlmock.New()
	defer rdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	c := New(pdb, rdb, rc)
	mr.Close() // simulate Redis going away
	if got := c.Read(context.Background(), "u-1"); got != rdb {
		t.Errorf("redis down should fail open to replica")
	}
}

// Write returns primary and sets sentinel.
func TestWrite_SetsSentinelAndReturnsPrimary(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	c := New(pdb, nil, rc)
	got := c.Write(context.Background(), "u-1")
	if got != pdb {
		t.Errorf("Write should always return primary")
	}
	if !mr.Exists("raw:u-1") {
		t.Errorf("sentinel should be set after Write")
	}
}

// Write without redis → still returns primary.
func TestWrite_NilRedisReturnsPrimary(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	c := New(pdb, nil, nil)
	if got := c.Write(context.Background(), "u-1"); got != pdb {
		t.Errorf("nil redis should still return primary")
	}
}

// Write with empty userID skips sentinel.
func TestWrite_EmptyUserIDNoSentinel(t *testing.T) {
	pdb, _, _ := sqlmock.New()
	defer pdb.Close()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer rc.Close()
	c := New(pdb, nil, rc)
	c.Write(context.Background(), "")
	keys := mr.Keys()
	for _, k := range keys {
		if k == "raw:" {
			t.Errorf("empty user should not write a sentinel")
		}
	}
}

// Smoke: real *sql.DB types satisfy the Cluster interface contract.
func TestCluster_SmokeSatisfiesInterface(t *testing.T) {
	var _ = &Cluster{Primary: &sql.DB{}, Replica: nil, Redis: nil}
}
