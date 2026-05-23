package httpapi

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
)

// JSON writes the canonical envelope: {"data": ..., "meta": ...}. Callers
// that hand it nil meta should get a body with no meta key.
func TestJSON_DataOnly(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, 200, map[string]any{"x": 1}, nil)
	var got map[string]any
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := got["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %T", got["data"])
	}
	if data["x"].(float64) != 1 {
		t.Errorf("data.x wrong: %v", data["x"])
	}
	if _, hasMeta := got["meta"]; hasMeta {
		t.Errorf("nil meta should not appear in envelope")
	}
}

// With meta, both keys appear.
func TestJSON_DataAndMeta(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, 200, map[string]any{"x": 1}, map[string]any{"total": 100})
	body := w.Body.String()
	if !strings.Contains(body, `"data"`) || !strings.Contains(body, `"meta"`) {
		t.Errorf("expected both data and meta keys, got %s", body)
	}
}

// JSON content-type must be application/json with charset so browsers parse
// correctly and the response is gzipped by middleware.Compress.
func TestJSON_SetsContentType(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, 200, nil, nil)
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "application/json") {
		t.Errorf("expected application/json content type, got %q", ct)
	}
}

func TestJSON_StatusCodePropagates(t *testing.T) {
	w := httptest.NewRecorder()
	JSON(w, 201, map[string]any{}, nil)
	if w.Code != 201 {
		t.Errorf("status should propagate: got %d", w.Code)
	}
}

// Error writes the canonical error envelope: {"error": {code, message}}.
func TestError_Envelope(t *testing.T) {
	w := httptest.NewRecorder()
	Error(w, 400, "bad_request", "missing field")
	var got struct {
		Error struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Error.Code != "bad_request" {
		t.Errorf("code wrong: %q", got.Error.Code)
	}
	if got.Error.Message != "missing field" {
		t.Errorf("message wrong: %q", got.Error.Message)
	}
}

func TestError_StatusCodePropagates(t *testing.T) {
	for _, status := range []int{400, 401, 403, 404, 409, 413, 429, 500, 503} {
		w := httptest.NewRecorder()
		Error(w, status, "x", "y")
		if w.Code != status {
			t.Errorf("Error(%d) wrote %d", status, w.Code)
		}
	}
}

// NoContent writes 204 with an empty body.
func TestNoContent(t *testing.T) {
	w := httptest.NewRecorder()
	NoContent(w)
	if w.Code != 204 {
		t.Errorf("expected 204, got %d", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Errorf("body should be empty, got %q", w.Body.String())
	}
}
