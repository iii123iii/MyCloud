package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// Spin up an App with just enough wiring to exercise the DLQ admin handlers.
// miniredis gives us an in-process Redis with no external deps; chi router
// is needed so chi.URLParam works for the /{queue}:retry path.
func dlqApp(t *testing.T) (*App, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rc := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rc.Close() })
	return &App{Redis: rc}, mr
}

// requestWithChiParam wraps an httptest request so the route param the
// handler reads via chi.URLParam is populated. Without this, chi.URLParam
// returns "" and the handler can't find the queue.
func requestWithChiParam(method, url, paramKey, paramVal string) *http.Request {
	r := httptest.NewRequest(method, url, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(paramKey, paramVal)
	return r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, rctx))
}

func TestDLQList_EmptyQueues(t *testing.T) {
	a, _ := dlqApp(t)
	w := httptest.NewRecorder()
	a.handleAdminDLQList(w, httptest.NewRequest("GET", "/admin/workers/dlq", nil))

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	var resp struct {
		Data struct {
			DLQ []struct {
				Queue   string   `json:"queue"`
				Depth   int64    `json:"depth"`
				Samples []string `json:"samples"`
			} `json:"dlq"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if len(resp.Data.DLQ) != 3 {
		t.Errorf("expected 3 queues (thumb/extract/hls), got %d", len(resp.Data.DLQ))
	}
	for _, q := range resp.Data.DLQ {
		if q.Depth != 0 {
			t.Errorf("expected empty depth, got %d for %s", q.Depth, q.Queue)
		}
	}
}

func TestDLQList_NonEmptyShowsDepthAndSamples(t *testing.T) {
	a, mr := dlqApp(t)
	// Seed q:thumb:failed with two entries via miniredis directly.
	mr.Lpush("q:thumb:failed", `{"payload":"file-1","error":"decode failed"}`)
	mr.Lpush("q:thumb:failed", `{"payload":"file-2","error":"timeout"}`)

	w := httptest.NewRecorder()
	a.handleAdminDLQList(w, httptest.NewRequest("GET", "/admin/workers/dlq", nil))

	body := w.Body.String()
	if !strings.Contains(body, "\"depth\":2") {
		t.Errorf("expected depth 2 for q:thumb, body: %s", body)
	}
	if !strings.Contains(body, "file-1") || !strings.Contains(body, "file-2") {
		t.Errorf("expected samples to include payloads, got %s", body)
	}
}

func TestDLQRetry_MovesEntriesBack(t *testing.T) {
	a, mr := dlqApp(t)
	// Seed two DLQ entries with the wrapped envelope format the worker writes.
	mr.Lpush("q:thumb:failed", `{"payload":"file-1","error":"e1"}`)
	mr.Lpush("q:thumb:failed", `{"payload":"file-2","error":"e2"}`)

	w := httptest.NewRecorder()
	r := requestWithChiParam("POST", "/admin/workers/dlq/thumb:retry", "queue", "thumb")
	a.handleAdminDLQRetry(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	// :failed should be empty, source queue should have both payloads.
	if got, _ := mr.List("q:thumb:failed"); len(got) != 0 {
		t.Errorf("dlq should be empty, got %v", got)
	}
	source, _ := mr.List("q:thumb")
	if len(source) != 2 {
		t.Errorf("source queue should have 2 entries, got %d", len(source))
	}
	// Entries should be unwrapped — the worker re-sees just the original payload.
	for _, p := range source {
		if strings.Contains(p, "\"error\"") {
			t.Errorf("requeued payload still wrapped: %s", p)
		}
	}
}

func TestDLQRetry_UnknownQueue404(t *testing.T) {
	a, _ := dlqApp(t)
	w := httptest.NewRecorder()
	r := requestWithChiParam("POST", "/admin/workers/dlq/unknown:retry", "queue", "unknown")
	a.handleAdminDLQRetry(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown queue, got %d", w.Code)
	}
}

func TestDLQClear_EmptiesQueue(t *testing.T) {
	a, mr := dlqApp(t)
	mr.Lpush("q:extract:failed", "junk")
	mr.Lpush("q:extract:failed", "more junk")

	w := httptest.NewRecorder()
	r := requestWithChiParam("POST", "/admin/workers/dlq/extract:clear", "queue", "extract")
	a.handleAdminDLQClear(w, r)

	if w.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if got, _ := mr.List("q:extract:failed"); len(got) != 0 {
		t.Errorf("queue should be empty after clear, got %v", got)
	}
}

func TestDLQClear_UnknownQueue404(t *testing.T) {
	a, _ := dlqApp(t)
	w := httptest.NewRecorder()
	r := requestWithChiParam("POST", "/admin/workers/dlq/unknown:clear", "queue", "unknown")
	a.handleAdminDLQClear(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestIsKnownDLQ(t *testing.T) {
	if !isKnownDLQ("q:thumb") {
		t.Errorf("q:thumb should be known")
	}
	if !isKnownDLQ("q:extract") {
		t.Errorf("q:extract should be known")
	}
	if !isKnownDLQ("q:hls") {
		t.Errorf("q:hls should be known")
	}
	if isKnownDLQ("q:made-up") {
		t.Errorf("unknown queue must not be reported as known")
	}
	if isKnownDLQ("") {
		t.Errorf("empty string should not match")
	}
}
