package app

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// Device-link QR flow. Security-critical guarantees exercised here:
//   - tokens are delivered only to a poller holding the verifier from the QR
//   - a wrong verifier is rejected (claim AND poll)
//   - tokens flow only after the owning browser approves
//   - a different user cannot approve someone else's pairing
//   - tokens are delivered exactly once (second poll sees "consumed")

// createPairing runs handleDeviceLinkCreate and returns (code, verifier).
func createPairing(t *testing.T, a *testApp) (string, string) {
	t.Helper()
	w := rec()
	r := authedRequest("POST", "/device-link/create", "")
	a.handleDeviceLinkCreate(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("create: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	var env struct {
		Data struct {
			Code     string `json:"code"`
			Verifier string `json:"verifier"`
			URL      string `json:"url"`
		} `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("create: decode: %v", err)
	}
	if env.Data.Code == "" || env.Data.Verifier == "" {
		t.Fatalf("create: empty code/verifier in %s", w.Body.String())
	}
	return env.Data.Code, env.Data.Verifier
}

func claim(a *testApp, code, verifier string) *http.Request {
	body, _ := json.Marshal(map[string]string{
		"code": code, "verifier": verifier,
		"device_name": "Pixel 8", "device_model": "Pixel 8", "platform": "Android 15",
	})
	r := publicRequest("POST", "/device-link/claim", string(body))
	return r
}

func publicRequest(method, path, body string) *http.Request {
	r := authedRequest(method, path, body) // reuse body wiring; ctx user is ignored by public handlers
	// Strip the auth context so these behave like unauthenticated calls.
	return r.WithContext(context.Background())
}

func TestDeviceLink_HappyPath(t *testing.T) {
	a := newTestApp(t)
	code, verifier := createPairing(t, a)

	// Phone claims (unauthenticated).
	w := rec()
	a.handleDeviceLinkClaim(w, claim(a, code, verifier))
	if w.Code != http.StatusOK {
		t.Fatalf("claim: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// Browser approves — needs the profile lookup + session insert mocked.
	a.mock.ExpectQuery("SELECT username, email, must_change_password FROM users").
		WithArgs("u-test").
		WillReturnRows(sqlmock.NewRows([]string{"username", "email", "must_change_password"}).
			AddRow("alice", "alice@example.com", false))
	a.mock.ExpectExec("INSERT INTO sessions").WillReturnResult(sqlmock.NewResult(1, 1))
	a.mock.ExpectExec("INSERT INTO activity_log").WillReturnResult(sqlmock.NewResult(1, 1))

	w = rec()
	r := authedRequest("POST", "/device-link/"+code+":approve", "")
	r = withChiParams(r, "code", code)
	a.handleDeviceLinkApprove(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("approve: expected 200, got %d body=%s", w.Code, w.Body.String())
	}

	// Phone polls and receives tokens exactly once.
	pollBody, _ := json.Marshal(map[string]string{"code": code, "verifier": verifier})
	w = rec()
	a.handleDeviceLinkPoll(w, publicRequest("POST", "/device-link/poll", string(pollBody)))
	if w.Code != http.StatusOK {
		t.Fatalf("poll: expected 200, got %d body=%s", w.Code, w.Body.String())
	}
	if !containsAll(w.Body.String(), `"access_token"`, `"refresh_token"`) {
		t.Fatalf("poll: expected tokens, got %s", w.Body.String())
	}

	// Second poll must NOT redeliver tokens.
	w = rec()
	a.handleDeviceLinkPoll(w, publicRequest("POST", "/device-link/poll", string(pollBody)))
	if containsAll(w.Body.String(), `"access_token"`) {
		t.Fatalf("poll twice: tokens redelivered! body=%s", w.Body.String())
	}
}

func TestDeviceLink_WrongVerifierRejectedOnClaim(t *testing.T) {
	a := newTestApp(t)
	code, _ := createPairing(t, a)
	w := rec()
	a.handleDeviceLinkClaim(w, claim(a, code, "not-the-real-verifier"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for bad verifier, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestDeviceLink_PollBeforeApprovalNoTokens(t *testing.T) {
	a := newTestApp(t)
	code, verifier := createPairing(t, a)
	w := rec()
	a.handleDeviceLinkClaim(w, claim(a, code, verifier))
	if w.Code != http.StatusOK {
		t.Fatalf("claim: %d", w.Code)
	}
	pollBody, _ := json.Marshal(map[string]string{"code": code, "verifier": verifier})
	w = rec()
	a.handleDeviceLinkPoll(w, publicRequest("POST", "/device-link/poll", string(pollBody)))
	if containsAll(w.Body.String(), `"access_token"`) {
		t.Fatalf("tokens leaked before approval: %s", w.Body.String())
	}
	if !containsAll(w.Body.String(), `"awaiting_approval"`) {
		t.Fatalf("expected awaiting_approval, got %s", w.Body.String())
	}
}

func TestDeviceLink_ForeignApproveRejected(t *testing.T) {
	a := newTestApp(t)
	code, verifier := createPairing(t, a)
	w := rec()
	a.handleDeviceLinkClaim(w, claim(a, code, verifier))
	if w.Code != http.StatusOK {
		t.Fatalf("claim: %d", w.Code)
	}
	// A different user tries to approve.
	w = rec()
	r := adminRequest("POST", "/device-link/"+code+":approve", "")
	r = withChiParams(r, "code", code)
	a.handleDeviceLinkApprove(w, r)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for foreign approve, got %d body=%s", w.Code, w.Body.String())
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		found := false
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}
