package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Same inputs produce the same ETag — the whole point.
func TestFor_Stable(t *testing.T) {
	a := For(1024, "2026-05-22T10:00:00Z")
	b := For(1024, "2026-05-22T10:00:00Z")
	if a != b {
		t.Errorf("ETag not stable: %q vs %q", a, b)
	}
}

// Different size → different ETag.
func TestFor_DiffersOnSize(t *testing.T) {
	a := For(1024, "2026-05-22T10:00:00Z")
	b := For(1025, "2026-05-22T10:00:00Z")
	if a == b {
		t.Errorf("ETag should differ when size changes")
	}
}

// Different mtime → different ETag.
func TestFor_DiffersOnModified(t *testing.T) {
	a := For(1024, "2026-05-22T10:00:00Z")
	b := For(1024, "2026-05-22T10:00:01Z")
	if a == b {
		t.Errorf("ETag should differ when modified time changes")
	}
}

// Length-prefix protection: distinct (size, mtime) pairs that concatenate
// to the same string still hash differently.
func TestFor_LengthPrefixedAvoidsCollision(t *testing.T) {
	a := For(12, "34") // "12:34"
	b := For(1, "234") // "1:234"
	if a == b {
		t.Errorf("collision: %q == %q (length-prefix protection broken)", a, b)
	}
}

// Strong ETag — wrapped in quotes, no W/ prefix.
func TestFor_StrongETagFormat(t *testing.T) {
	tag := For(100, "x")
	if !strings.HasPrefix(tag, `"`) || !strings.HasSuffix(tag, `"`) {
		t.Errorf("ETag should be quoted: %q", tag)
	}
	if strings.HasPrefix(tag, `W/`) {
		t.Errorf("ETag should be strong, not weak: %q", tag)
	}
}

// 304 when If-None-Match matches.
func TestCheckIfNoneMatch_Match(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	tag := For(1, "x")
	r.Header.Set("If-None-Match", tag)
	hit := CheckIfNoneMatch(w, r, tag)
	if !hit {
		t.Errorf("expected hit=true")
	}
	if w.Code != http.StatusNotModified {
		t.Errorf("expected 304, got %d", w.Code)
	}
	if w.Header().Get("ETag") != tag {
		t.Errorf("ETag should be set on 304")
	}
}

// No match → caller proceeds with body.
func TestCheckIfNoneMatch_NoMatch(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	tag := For(1, "x")
	r.Header.Set("If-None-Match", `"different"`)
	hit := CheckIfNoneMatch(w, r, tag)
	if hit {
		t.Errorf("expected hit=false")
	}
	// ETag still set so the next request can be conditional.
	if w.Header().Get("ETag") != tag {
		t.Errorf("ETag should be set even on miss")
	}
}

// Wildcard If-None-Match: * matches any representation.
func TestCheckIfNoneMatch_Wildcard(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", "*")
	hit := CheckIfNoneMatch(w, r, For(1, "x"))
	if !hit || w.Code != http.StatusNotModified {
		t.Errorf("wildcard should produce 304")
	}
}

// CSV list of ETags — match any.
func TestCheckIfNoneMatch_CSVMatch(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	tag := For(1, "x")
	r.Header.Set("If-None-Match", `"other-1", `+tag+`, "other-2"`)
	hit := CheckIfNoneMatch(w, r, tag)
	if !hit {
		t.Errorf("expected hit on CSV match")
	}
}

// Absent header — no conditional behaviour.
func TestCheckIfNoneMatch_Absent(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	tag := For(1, "x")
	if CheckIfNoneMatch(w, r, tag) {
		t.Errorf("absent header should not match")
	}
	if w.Header().Get("ETag") != tag {
		t.Errorf("ETag should still be advertised")
	}
}
