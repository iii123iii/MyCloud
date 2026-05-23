package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestForDeterministic(t *testing.T) {
	a := For(1024, "2026-05-20T10:00:00Z")
	b := For(1024, "2026-05-20T10:00:00Z")
	if a != b {
		t.Errorf("For should be deterministic: %q != %q", a, b)
	}
}

func TestForChangesOnInputs(t *testing.T) {
	base := For(1024, "2026-05-20T10:00:00Z")
	if For(1025, "2026-05-20T10:00:00Z") == base {
		t.Error("ETag should change when size changes")
	}
	if For(1024, "2026-05-20T10:00:01Z") == base {
		t.Error("ETag should change when modified time changes")
	}
}

func TestForLengthPrefixedAvoidsCollisions(t *testing.T) {
	// Without length-prefix, "12" + ":" + "34" and "1" + ":" + "234" both
	// stream "12:34" / "1:234" — different. With the prefix, even
	// adversarial pairs that happen to land on the same body stay distinct.
	if For(12, "34") == For(1, "234") {
		t.Error("collision between size=12 mtime=34 and size=1 mtime=234")
	}
}

func TestCheckIfNoneMatchHits(t *testing.T) {
	tag := For(100, "x")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/file", nil)
	r.Header.Set("If-None-Match", tag)

	if !CheckIfNoneMatch(w, r, tag) {
		t.Fatal("expected 304 response (hit)")
	}
	if w.Code != http.StatusNotModified {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotModified)
	}
	if got := w.Header().Get("ETag"); got != tag {
		t.Errorf("ETag header = %q, want %q", got, tag)
	}
}

func TestCheckIfNoneMatchMisses(t *testing.T) {
	tag := For(100, "x")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/file", nil)
	r.Header.Set("If-None-Match", `"different"`)

	if CheckIfNoneMatch(w, r, tag) {
		t.Fatal("expected miss")
	}
	if got := w.Header().Get("ETag"); got != tag {
		t.Errorf("ETag header = %q, want %q", got, tag)
	}
}

func TestCheckIfNoneMatchWildcard(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/file", nil)
	r.Header.Set("If-None-Match", "*")

	if !CheckIfNoneMatch(w, r, `"any"`) {
		t.Fatal("expected wildcard hit")
	}
	if w.Code != http.StatusNotModified {
		t.Errorf("wildcard status = %d, want %d", w.Code, http.StatusNotModified)
	}
}

func TestCheckIfNoneMatchAlwaysSetsHeader(t *testing.T) {
	tag := For(100, "x")
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/file", nil)

	if CheckIfNoneMatch(w, r, tag) {
		t.Fatal("expected miss with no client header")
	}
	if got := w.Header().Get("ETag"); got != tag {
		t.Errorf("ETag header = %q, want %q (should be set even on miss)", got, tag)
	}
}
