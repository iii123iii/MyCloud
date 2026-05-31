package app

import (
	"net/http"
	"strings"
	"testing"
)

func TestCreateMyWebhook_EmptyURL400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	a.handleCreateMyWebhook(w, authedRequest("POST", "/me/webhooks", `{"url":"","events":["*"]}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateMyWebhook_BadScheme400(t *testing.T) {
	a := newTestApp(t)
	a.Config.WebhookAllowPrivateTargets = true // isolate the scheme check from DNS
	w := rec()
	a.handleCreateMyWebhook(w, authedRequest("POST", "/me/webhooks", `{"url":"ftp://example.com","events":["*"]}`))
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for non-http scheme, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestCreateMyWebhook_PrivateTargetBlocked400(t *testing.T) {
	a := newTestApp(t)
	a.Config.WebhookAllowPrivateTargets = false
	w := rec()
	a.handleCreateMyWebhook(w, authedRequest("POST", "/me/webhooks", `{"url":"http://127.0.0.1:9000/hook","events":["*"]}`))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for private target, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "blocked") {
		t.Errorf("expected a 'blocked' message, got %s", w.Body.String())
	}
}

func TestCreateMyWebhook_Success(t *testing.T) {
	a := newTestApp(t)
	a.Config.WebhookAllowPrivateTargets = true // validation short-circuits before any DNS
	a.mock.ExpectExec("INSERT INTO webhooks").WillReturnResult(sqlmockResult(0, 1))
	// refreshWebhookUserActive (is_active defaults true) runs a COUNT then SADDs to miniredis.
	a.mock.ExpectQuery("SELECT COUNT\\(\\*\\) FROM webhooks").
		WillReturnRows(sqlmockNewRows([]string{"c"}).AddRow(1))
	a.mock.ExpectExec(".*").WillReturnResult(sqlmockResult(0, 1)) // writeActivity

	w := rec()
	a.handleCreateMyWebhook(w, authedRequest("POST", "/me/webhooks", `{"url":"http://198.51.100.7/hook","events":["file.upload"]}`))
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "mc_whsec_") {
		t.Error("create response must include the signing secret once")
	}
}

func TestDeleteMyWebhook_NotFound404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("DELETE FROM webhooks").WillReturnResult(sqlmockResult(0, 0))
	w := rec()
	a.handleDeleteMyWebhook(w, withChiParams(authedRequest("DELETE", "/me/webhooks/x", ""), "id", "x"))
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
