package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateMyToken_EmptyName400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	a.handleCreateMyToken(w, authedRequest("POST", "/me/tokens", `{"name":"  ","scopes":["files:read"]}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateMyToken_UnknownScope400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	a.handleCreateMyToken(w, authedRequest("POST", "/me/tokens", `{"name":"ci","scopes":["files:nuke"]}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for unknown scope, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMyToken_Success(t *testing.T) {
	a := newTestApp(t)
	// PATMaxPerUser is 0 in the test config, so no COUNT precheck runs.
	a.mock.ExpectExec("INSERT INTO personal_access_tokens").WillReturnResult(sqlmockResult(0, 1))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmockResult(0, 1)) // writeActivity

	w := rec()
	a.handleCreateMyToken(w, authedRequest("POST", "/me/tokens", `{"name":"ci","scopes":["files:read","files:read"]}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	body := w.Body.String()
	if !strings.Contains(body, "mc_pat_") {
		t.Error("create response must include the plaintext token once")
	}
	if !strings.Contains(body, "files:read") {
		t.Error("create response should echo the scopes")
	}
	// Critical: the hash must never reach the wire.
	if strings.Contains(body, "token_hash") {
		t.Errorf("response leaked token_hash: %s", body)
	}
}

func TestRevokeMyToken_NotFound404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("UPDATE personal_access_tokens SET revoked_at").WillReturnResult(sqlmockResult(0, 0))
	w := rec()
	a.handleRevokeMyToken(w, withChiParams(authedRequest("DELETE", "/me/tokens/x", ""), "id", "x"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
