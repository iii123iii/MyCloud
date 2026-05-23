package app

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// Helper: spin up an App with miniredis + a sqlmock DB. Rate-limit blocks
// write a row to activity_log via the DB — without a non-nil DB the
// middleware panics. The default regexp matcher with ".*" lets any number
// of activity_log inserts pass through unchecked.
func rlApp(t *testing.T, capacity int, window time.Duration) (*App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	mock.MatchExpectationsInOrder(false)
	// Accept any number of activity_log writes — we don't validate them in
	// these tests.
	for i := 0; i < 100; i++ {
		mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	}
	t.Cleanup(func() { _ = rc.Close(); db.Close() })
	return &App{Redis: rc, DB: db}, mr
}

func okHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
}

// Disabled limiter (capacity 0) passes through every request — used when
// the config sets the bucket to "off".
func TestRateLimit_Disabled(t *testing.T) {
	a, _ := rlApp(t, 0, time.Minute)
	mw := a.rateLimit("test", 0, time.Minute, keyByIP)
	h := mw(okHandler())
	for i := 0; i < 5; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusOK {
			t.Errorf("disabled limiter blocked request: %d", w.Code)
		}
	}
}

// Empty key (e.g. unauth user-id keyer on a public request) passes through
// without consuming bucket capacity.
func TestRateLimit_EmptyKeyPassesThrough(t *testing.T) {
	a, _ := rlApp(t, 1, time.Minute)
	mw := a.rateLimit("test", 1, time.Minute, func(r *http.Request, _ string) string { return "" })
	h := mw(okHandler())
	for i := 0; i < 10; i++ {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
		if w.Code != http.StatusOK {
			t.Errorf("empty-key request blocked: %d", w.Code)
		}
	}
}

// Within capacity: every request succeeds. At capacity+1: 429.
func TestRateLimit_BlocksAfterCapacity(t *testing.T) {
	a, _ := rlApp(t, 3, time.Minute)
	mw := a.rateLimit("test", 3, time.Minute, keyByIP)
	h := mw(okHandler())

	req := func() *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "1.2.3.4:1000"
		h.ServeHTTP(w, r)
		return w
	}

	for i := 0; i < 3; i++ {
		w := req()
		if w.Code != http.StatusOK {
			t.Errorf("request %d should succeed, got %d", i+1, w.Code)
		}
	}
	w := req()
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("4th request should be 429, got %d", w.Code)
	}
	if ra := w.Header().Get("Retry-After"); ra == "" {
		t.Errorf("429 response must set Retry-After")
	} else if n, err := strconv.Atoi(ra); err != nil || n <= 0 {
		t.Errorf("Retry-After should be a positive int, got %q", ra)
	}
	body := w.Body.String()
	if !strings.Contains(body, "rate_limited") {
		t.Errorf("expected rate_limited code in body, got %s", body)
	}
}

// Distinct IPs get independent buckets — no cross-talk.
func TestRateLimit_PerKeyBucket(t *testing.T) {
	a, _ := rlApp(t, 2, time.Minute)
	mw := a.rateLimit("test", 2, time.Minute, keyByIP)
	h := mw(okHandler())

	for _, ip := range []string{"1.1.1.1:80", "2.2.2.2:80"} {
		for i := 0; i < 2; i++ {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/", nil)
			r.RemoteAddr = ip
			h.ServeHTTP(w, r)
			if w.Code != http.StatusOK {
				t.Errorf("ip %s req %d blocked: %d", ip, i+1, w.Code)
			}
		}
	}
}

// keyByIP delegates to clientIP — verify it handles X-Forwarded-For so the
// limiter sees the actual client IP, not the proxy's.
func TestKeyByIP_UsesXForwardedFor(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "10.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := keyByIP(r, "bucket"); got != "203.0.113.5" {
		t.Errorf("expected XFF value, got %q", got)
	}
}

// Fail-open: if Redis is unreachable, the limiter must NOT block. Better to
// serve unrestrictedly than to make the whole API depend on Redis.
func TestRateLimit_FailsOpenOnRedisError(t *testing.T) {
	mr := miniredis.RunT(t)
	cli := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer cli.Close()
	a := &App{Redis: cli}
	mw := a.rateLimit("test", 1, time.Minute, keyByIP)
	h := mw(okHandler())

	// Close miniredis to simulate Redis going away mid-traffic.
	mr.Close()
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "1.2.3.4:1"
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected fail-open OK, got %d", w.Code)
	}
}

// Bucket separation: same key in two different buckets doesn't share state.
func TestRateLimit_DistinctBucketsIndependent(t *testing.T) {
	a, _ := rlApp(t, 1, time.Minute)
	bucketA := a.rateLimit("a", 1, time.Minute, keyByIP)
	bucketB := a.rateLimit("b", 1, time.Minute, keyByIP)
	ha := bucketA(okHandler())
	hb := bucketB(okHandler())

	mk := func() *http.Request {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = "1.2.3.4:1"
		return r
	}

	w1 := httptest.NewRecorder()
	ha.ServeHTTP(w1, mk())
	if w1.Code != http.StatusOK {
		t.Errorf("bucket A first req should pass, got %d", w1.Code)
	}
	// Second hit on the same bucket would block — but on a DIFFERENT bucket
	// it must still pass.
	w2 := httptest.NewRecorder()
	hb.ServeHTTP(w2, mk())
	if w2.Code != http.StatusOK {
		t.Errorf("bucket B should be independent, got %d", w2.Code)
	}
}
