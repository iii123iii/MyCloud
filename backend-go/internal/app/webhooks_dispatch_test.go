package app

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"mycloud/backend-go/internal/config"
)

func TestIsBlockedIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "::1", "10.0.0.1", "172.16.0.1", "192.168.1.1",
		"169.254.169.254", "0.0.0.0", "fc00::1", "fe80::1", "224.0.0.1",
		"::ffff:127.0.0.1", // IPv4-mapped loopback must not slip past
	}
	for _, s := range blocked {
		ip := net.ParseIP(s)
		if ip == nil {
			t.Fatalf("bad test IP %q", s)
		}
		if !isBlockedIP(ip) {
			t.Errorf("%s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if isBlockedIP(net.ParseIP(s)) {
			t.Errorf("%s should be allowed", s)
		}
	}
}

func TestSignWebhookAndHeader(t *testing.T) {
	secret := "shh"
	body := []byte(`{"a":1}`)
	const tnow int64 = 1700000000
	want := func(sec string) string {
		mac := hmac.New(sha256.New, []byte(sec))
		mac.Write([]byte(strconv.FormatInt(tnow, 10)))
		mac.Write([]byte("."))
		mac.Write(body)
		return hex.EncodeToString(mac.Sum(nil))
	}
	if got := signWebhook(secret, tnow, body); got != want(secret) {
		t.Errorf("signWebhook mismatch:\n got %s\nwant %s", got, want(secret))
	}

	// No rotation: exactly one v1.
	h := buildSignatureHeader(webhookRow{secret: secret}, tnow, body)
	if !strings.HasPrefix(h, "t="+strconv.FormatInt(tnow, 10)+",v1=") {
		t.Errorf("header shape wrong: %s", h)
	}
	if strings.Count(h, "v1=") != 1 {
		t.Errorf("expected one v1, got %s", h)
	}

	// Active rotation grace: two v1 values, both verifiable.
	wh := webhookRow{
		secret:            secret,
		prevSecret:        sql.NullString{String: "old", Valid: true},
		prevSecretExpires: sql.NullTime{Time: time.Now().Add(time.Hour), Valid: true},
	}
	h = buildSignatureHeader(wh, tnow, body)
	if strings.Count(h, "v1=") != 2 {
		t.Errorf("expected two v1 during grace, got %s", h)
	}
	if !strings.Contains(h, want("old")) {
		t.Error("previous-secret signature missing during grace window")
	}

	// Expired grace: previous secret dropped.
	wh.prevSecretExpires = sql.NullTime{Time: time.Now().Add(-time.Hour), Valid: true}
	if h = buildSignatureHeader(wh, tnow, body); strings.Count(h, "v1=") != 1 {
		t.Errorf("expired grace should drop previous secret, got %s", h)
	}
}

func TestParseRetryAfter(t *testing.T) {
	if parseRetryAfter("5") != 5*time.Second {
		t.Error("delta-seconds not parsed")
	}
	if parseRetryAfter("") != 0 || parseRetryAfter("-3") != 0 || parseRetryAfter("nonsense") != 0 {
		t.Error("invalid Retry-After should yield 0")
	}
	future := time.Now().Add(30 * time.Second).UTC().Format(http.TimeFormat)
	if d := parseRetryAfter(future); d <= 0 || d > 31*time.Second {
		t.Errorf("HTTP-date Retry-After out of range: %v", d)
	}
}

func TestEventMatches(t *testing.T) {
	if !eventMatches("", "file.upload") || !eventMatches("*", "file.upload") {
		t.Error("empty/star should match all events")
	}
	if !eventMatches("file.upload,share.create", "share.create") {
		t.Error("CSV membership should match")
	}
	if eventMatches("file.upload", "file.delete") {
		t.Error("non-member should not match")
	}
}

func TestNormalizeEvents(t *testing.T) {
	if normalizeEvents(nil) != "*" {
		t.Error("empty selection should mean all (*)")
	}
	if normalizeEvents([]string{"*", "file.upload"}) != "*" {
		t.Error("* should win")
	}
	if normalizeEvents([]string{"file.upload", "file.upload"}) != "file.upload" {
		t.Error("should dedupe")
	}
	if normalizeEvents([]string{"b", "a"}) != "a,b" {
		t.Error("should sort")
	}
}

func TestBuildWebhookBody_Deterministic(t *testing.T) {
	ev := webhookEvent{UserID: "u1", Action: "file.upload", ResourceType: "file", ResourceID: "f1"}
	a := buildWebhookBody("del-1", 1700000000, ev)
	b := buildWebhookBody("del-1", 1700000000, ev)
	if string(a) != string(b) {
		t.Error("body must be deterministic for the same inputs (frozen across retries)")
	}
	if !strings.Contains(string(a), `"event":"file.upload"`) {
		t.Errorf("body missing event field: %s", a)
	}
}

func TestProcessWebhookJob_PayloadHandling(t *testing.T) {
	a := &App{}
	// Malformed payload is an infra/programming error → DLQ (returns error).
	if err := a.processWebhookJob(context.Background(), []byte("not json")); err == nil {
		t.Error("malformed payload should return an error so it dead-letters")
	}
	// Missing user/action is a no-op, not a DLQ condition.
	if err := a.processWebhookJob(context.Background(), []byte(`{"action":"x"}`)); err != nil {
		t.Errorf("empty user_id should be a no-op, got %v", err)
	}
}

func TestSendWebhook_DeliversSignedHeaders(t *testing.T) {
	var sig, event, delivery string
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sig = r.Header.Get("X-MyCloud-Signature")
		event = r.Header.Get("X-MyCloud-Event")
		delivery = r.Header.Get("X-MyCloud-Delivery")
		body, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	a := &App{Config: config.Config{WebhookAllowPrivateTargets: true, WebhookDeliveryTimeoutSecs: 5}}
	a.webhookClient = newWebhookClient(a.Config, a.ssrfControl)

	payload := []byte(`{"hello":"world"}`)
	status, _, err := a.sendWebhook(context.Background(), srv.URL, payload, "t=1,v1=abc", "file.upload", "del-1")
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("status: got %d", status)
	}
	if sig != "t=1,v1=abc" || event != "file.upload" || delivery != "del-1" {
		t.Errorf("headers not forwarded: sig=%q event=%q delivery=%q", sig, event, delivery)
	}
	if string(body) != string(payload) {
		t.Errorf("body mismatch: %q", body)
	}
}

func TestSendWebhook_SSRFBlocksLoopback(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()

	// allow-private = false ⇒ dialing the 127.0.0.1 test server must be refused
	// at dial time by the SSRF Control hook.
	a := &App{Config: config.Config{WebhookAllowPrivateTargets: false, WebhookDeliveryTimeoutSecs: 5}}
	a.webhookClient = newWebhookClient(a.Config, a.ssrfControl)

	if _, _, err := a.sendWebhook(context.Background(), srv.URL, []byte(`{}`), "sig", "evt", "del"); err == nil {
		t.Error("expected SSRF guard to block dialing a loopback target")
	}
}
