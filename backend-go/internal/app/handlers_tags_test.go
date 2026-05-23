package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/go-sql-driver/mysql"
)

func TestHandleCreateTag_EmptyNameReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"  "}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "validation_error") {
		t.Errorf("expected validation_error code")
	}
}

func TestHandleCreateTag_OverlongNameReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"`+strings.Repeat("a", 100)+`"}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for over-64 name, got %d", w.Code)
	}
}

func TestHandleCreateTag_InvalidColorReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"red","color":"not-a-hex"}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "Color") && !strings.Contains(w.Body.String(), "color") {
		t.Errorf("error should mention color, got %s", w.Body.String())
	}
}

// IsDuplicate path: a 1062 MySQL error from the INSERT must surface as 409
// `duplicate`, not the generic 500.
func TestHandleCreateTag_DuplicateReturns409(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO tags").
		WillReturnError(&mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'red' for key 'tags.name'"})
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"red","color":"#ff0000"}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "duplicate") {
		t.Errorf("expected duplicate code")
	}
	// Critical: the response must NOT leak the raw "Duplicate entry 'red'…"
	// MySQL message — the audit caught that exact information disclosure.
	if strings.Contains(w.Body.String(), "tags.name") {
		t.Errorf("response leaked schema text: %s", w.Body.String())
	}
}

// Successful create returns 201 with the new id.
func TestHandleCreateTag_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO tags").
		WillReturnResult(sqlmock.NewResult(0, 1))
	// activity_log insert (writeActivity).
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"red","color":"#ff0000"}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "red") {
		t.Errorf("expected name in body")
	}
}

// Empty color defaults to the muted grey — not a validation error.
func TestHandleCreateTag_EmptyColorDefaults(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO tags").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := authedRequest("POST", "/tags", `{"name":"red"}`)
	a.handleCreateTag(w, r)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
}

// Update with nothing to set returns 400.
func TestHandleUpdateTag_NothingToUpdateReturns400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := authedRequest("PATCH", "/tags/x", `{}`)
	r = withChiParams(r, "id", "x")
	a.handleUpdateTag(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
}
