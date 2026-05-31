package app

// Outbound webhook delivery: the q:webhook worker, the SSRF-guarded HTTP
// client, the per-delivery signing, and the Redis fan-out gate (wh:users).
//
// The event fan-out hot path (publishActivity → fanOutWebhook) does only a
// SISMEMBER + LPUSH; all DB work and HTTP delivery happen here in the worker.

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"

	"mycloud/backend-go/internal/config"
)

const (
	webhookQueue    = "q:webhook"
	webhookUsersKey = "wh:users" // Redis set of user IDs with ≥1 active webhook
	webhookBodyCap  = 16 << 10   // store at most 16 KiB of request body on failure
)

// webhookBackoffDurations is the inter-attempt wait schedule (clamped to the
// per-job deadline). 429 responses override this with Retry-After.
var webhookBackoffDurations = []time.Duration{1 * time.Second, 5 * time.Second, 15 * time.Second}

// webhookEvent is the compact envelope enqueued on q:webhook by fanOutWebhook
// and consumed by processWebhookJob.
type webhookEvent struct {
	UserID       string          `json:"user_id"`
	Action       string          `json:"action"`
	ResourceType string          `json:"resource_type"`
	ResourceID   string          `json:"resource_id"`
	Details      json.RawMessage `json:"details,omitempty"`
	TS           time.Time       `json:"ts"`
}

// webhookRow is a subscription loaded for delivery.
type webhookRow struct {
	id, url, secret, events string
	prevSecret              sql.NullString
	prevSecretExpires       sql.NullTime
}

// newWebhookClient builds the HTTP client used for ALL outbound webhook
// delivery. The dial-time Control hook is the authoritative SSRF defense
// (validates the IP actually being connected to, defeating DNS-rebind);
// Proxy:nil prevents proxy-based bypass; redirects are not followed so a 3xx
// to an internal address can't smuggle a request past the guard.
func newWebhookClient(cfg config.Config, control func(string, string, syscall.RawConn) error) *http.Client {
	timeout := time.Duration(cfg.WebhookDeliveryTimeoutSecs) * time.Second
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         (&net.Dialer{Timeout: timeout, Control: control}).DialContext,
		MaxIdleConns:        10,
		IdleConnTimeout:     30 * time.Second,
		TLSHandshakeTimeout: timeout,
	}
	return &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
}

// ssrfControl runs after DNS resolution and before connect, on the concrete IP
// being dialed. Returning an error aborts the connection.
func (a *App) ssrfControl(network, address string, _ syscall.RawConn) error {
	if a.Config.WebhookAllowPrivateTargets {
		return nil
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		host = address
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return fmt.Errorf("webhook: unresolvable dial address %q", address)
	}
	if isBlockedIP(ip) {
		return fmt.Errorf("webhook target resolves to a blocked address: %s", ip)
	}
	return nil
}

// isBlockedIP reports whether an IP is in a range webhooks must not reach
// unless explicitly allowed. IPv4-mapped IPv6 (::ffff:a.b.c.d) is unmapped
// first so the v4 classifiers apply — otherwise ::ffff:127.0.0.1 would slip
// past IsLoopback.
func isBlockedIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsUnspecified()
}

// processWebhookJob is the q:webhook worker body. It returns an error ONLY for
// infrastructure failures (bad payload, DB unavailable) so those land in the
// DLQ; per-delivery failures are recorded, retried, and auto-disabled inline
// and return nil so one dead endpoint doesn't DLQ the user's other webhooks.
func (a *App) processWebhookJob(ctx context.Context, payload []byte) error {
	var ev webhookEvent
	if err := json.Unmarshal(payload, &ev); err != nil {
		return fmt.Errorf("webhook: bad event payload: %w", err)
	}
	if ev.UserID == "" || ev.Action == "" {
		return nil
	}
	rows, err := a.DB.QueryContext(ctx, `
		SELECT id, url, secret, previous_secret, previous_secret_expires_at, events
		FROM webhooks
		WHERE user_id = ? AND is_active = 1`, ev.UserID)
	if err != nil {
		return fmt.Errorf("webhook: load subscriptions: %w", err)
	}
	var matches []webhookRow
	for rows.Next() {
		var wh webhookRow
		if err := rows.Scan(&wh.id, &wh.url, &wh.secret, &wh.prevSecret, &wh.prevSecretExpires, &wh.events); err != nil {
			rows.Close()
			return fmt.Errorf("webhook: scan subscription: %w", err)
		}
		if eventMatches(wh.events, ev.Action) {
			matches = append(matches, wh)
		}
	}
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return fmt.Errorf("webhook: iterate subscriptions: %w", rowsErr)
	}
	for _, wh := range matches {
		_, _ = a.dispatchToWebhook(ctx, wh, ev, a.Config.WebhookMaxAttempts, true)
	}
	return nil
}

// dispatchToWebhook delivers one event to one webhook with bounded retries,
// recording a webhook_deliveries row and, when updateHealth is true, updating
// the webhook's persistent failure state. The worker passes updateHealth=true;
// the synchronous :test endpoint passes false so a manual test can't reset the
// real consecutive-failure streak or count toward auto-disabling the hook. The
// signed body is built ONCE and reused across attempts so retries carry an
// identical signature (receivers doing (delivery_id, signature) replay-
// protection would otherwise reject them). Returns the last HTTP status and a
// non-nil error on final failure.
func (a *App) dispatchToWebhook(ctx context.Context, wh webhookRow, ev webhookEvent, maxAttempts int, updateHealth bool) (int, error) {
	deliveryID := uuid.NewString()
	t := time.Now().Unix()
	body := buildWebhookBody(deliveryID, t, ev)
	sigHeader := buildSignatureHeader(wh, t, body)

	_, _ = a.DB.ExecContext(ctx,
		`INSERT INTO webhook_deliveries (id, webhook_id, event_type, status, attempts) VALUES (?, ?, ?, 'pending', 0)`,
		deliveryID, wh.id, ev.Action)

	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var (
		status   int
		sendErr  error
		attempts int
	)
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		attempts = attempt
		var retryAfter string
		status, retryAfter, sendErr = a.sendWebhook(ctx, wh.url, body, sigHeader, ev.Action, deliveryID)
		if sendErr == nil && status >= 200 && status < 300 {
			a.recordDeliverySuccess(ctx, deliveryID, wh.id, attempt, status, updateHealth)
			a.Metrics.IncWebhookDelivery("success")
			return status, nil
		}
		retryable := sendErr != nil || status >= 500 || status == http.StatusTooManyRequests
		if !retryable || attempt == maxAttempts {
			break
		}
		a.Metrics.IncWebhookDelivery("retry")
		if !webhookBackoffWait(ctx, attempt, status, retryAfter) {
			break
		}
	}
	failMsg := webhookFailMsg(status, sendErr)
	a.recordDeliveryFailure(ctx, deliveryID, wh.id, attempts, status, body, failMsg, updateHealth)
	a.Metrics.IncWebhookDelivery("failed")
	return status, fmt.Errorf("webhook %s delivery failed: %s", wh.id, failMsg)
}

func (a *App) sendWebhook(ctx context.Context, url string, body []byte, sigHeader, event, deliveryID string) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "MyCloud-Webhook/1.0")
	req.Header.Set("X-MyCloud-Event", event)
	req.Header.Set("X-MyCloud-Delivery", deliveryID)
	req.Header.Set("X-MyCloud-Signature", sigHeader)
	resp, err := a.webhookClient.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	// Drain a little so the connection can be reused; cap to avoid a hostile
	// receiver streaming forever.
	_, _ = io.CopyN(io.Discard, resp.Body, 4<<10)
	return resp.StatusCode, resp.Header.Get("Retry-After"), nil
}

// buildWebhookBody renders the canonical delivery payload. Called once per
// delivery; the bytes are then frozen across retries.
func buildWebhookBody(deliveryID string, t int64, ev webhookEvent) []byte {
	var details any
	if len(ev.Details) > 0 {
		details = ev.Details
	}
	body, _ := json.Marshal(map[string]any{
		"id":    deliveryID,
		"event": ev.Action,
		"ts":    t,
		"data": map[string]any{
			"user_id":       ev.UserID,
			"action":        ev.Action,
			"resource_type": ev.ResourceType,
			"resource_id":   ev.ResourceID,
			"details":       details,
		},
	})
	return body
}

// buildSignatureHeader produces "t=<unix>,v1=<hex>[,v1=<hex>]". The extra v1 is
// the previous secret during a rotation grace window, so receivers holding
// either secret can verify.
func buildSignatureHeader(wh webhookRow, t int64, body []byte) string {
	sig := "t=" + strconv.FormatInt(t, 10) + ",v1=" + signWebhook(wh.secret, t, body)
	if wh.prevSecret.Valid && wh.prevSecret.String != "" &&
		(!wh.prevSecretExpires.Valid || wh.prevSecretExpires.Time.After(time.Now())) {
		sig += ",v1=" + signWebhook(wh.prevSecret.String, t, body)
	}
	return sig
}

// signWebhook computes hex(HMAC-SHA256(secret, "<t>.<body>")). Signing the
// timestamp (not just trusting body.ts) lets receivers reject stale events.
func signWebhook(secret string, t int64, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(strconv.FormatInt(t, 10)))
	mac.Write([]byte("."))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) recordDeliverySuccess(ctx context.Context, deliveryID, webhookID string, attempts, status int, updateHealth bool) {
	_, _ = a.DB.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = 'success', response_status = ?, attempts = ?, delivered_at = NOW()
		WHERE id = ?`, status, attempts, deliveryID)
	// A manual :test delivery records its own delivery row but must not touch
	// the webhook's health state — otherwise a test would silently reset a real
	// consecutive-failure streak and stop a broken hook from auto-disabling.
	if !updateHealth {
		return
	}
	_, _ = a.DB.ExecContext(ctx,
		"UPDATE webhooks SET failure_count = 0, last_delivery_at = NOW() WHERE id = ?", webhookID)
}

func (a *App) recordDeliveryFailure(ctx context.Context, deliveryID, webhookID string, attempts, status int, body []byte, errMsg string, updateHealth bool) {
	var respStatus any
	if status > 0 {
		respStatus = status
	}
	// Store the request body only on failure (capped) so the table stays small
	// for high-volume users; successes keep just status + timing.
	stored := body
	if len(stored) > webhookBodyCap {
		stored = stored[:webhookBodyCap]
	}
	_, _ = a.DB.ExecContext(ctx, `
		UPDATE webhook_deliveries
		SET status = 'failed', response_status = ?, error = ?, attempts = ?, request_body = ?, delivered_at = NOW()
		WHERE id = ?`, respStatus, truncateStr(errMsg, 512), attempts, string(stored), deliveryID)
	// See recordDeliverySuccess: a :test delivery records its attempt but must
	// not advance the failure counter or auto-disable the webhook.
	if !updateHealth {
		return
	}
	a.handleWebhookFailure(ctx, webhookID)
}

// handleWebhookFailure increments the failure counter and auto-disables a hook
// that has failed too many times in a row, pruning the fan-out set if that was
// the user's last active webhook.
func (a *App) handleWebhookFailure(ctx context.Context, webhookID string) {
	_, _ = a.DB.ExecContext(ctx,
		"UPDATE webhooks SET failure_count = failure_count + 1, last_delivery_at = NOW() WHERE id = ?", webhookID)
	limit := a.Config.WebhookAutoDisableAfter
	if limit <= 0 {
		return
	}
	var count int
	var userID string
	if err := a.DB.QueryRowContext(ctx,
		"SELECT failure_count, user_id FROM webhooks WHERE id = ?", webhookID).Scan(&count, &userID); err != nil {
		return
	}
	if count >= limit {
		_, _ = a.DB.ExecContext(ctx, "UPDATE webhooks SET is_active = 0 WHERE id = ?", webhookID)
		a.refreshWebhookUserActive(ctx, userID)
	}
}

// fanOutWebhook is called from publishActivity on every activity write. It does
// the minimum work on the request path: a gated SISMEMBER + LPUSH. The worker
// resolves subscriptions and delivers.
func (a *App) fanOutWebhook(ctx context.Context, userID, action, resourceType, resourceID string, details []byte) {
	if a.Redis == nil || userID == "" || !a.webhookUserHasActive(ctx, userID) {
		return
	}
	ev := webhookEvent{
		UserID:       userID,
		Action:       action,
		ResourceType: resourceType,
		ResourceID:   resourceID,
		TS:           time.Now().UTC(),
	}
	if len(details) > 0 && string(details) != "null" {
		ev.Details = json.RawMessage(details)
	}
	raw, err := json.Marshal(ev)
	if err != nil {
		return
	}
	_ = a.Redis.LPush(ctx, webhookQueue, raw).Err()
}

// webhookUserHasActive gates the fan-out. Fails closed (returns false) on a
// Redis error — webhooks are best-effort and skipping an enqueue is preferable
// to erroring the originating request.
func (a *App) webhookUserHasActive(ctx context.Context, userID string) bool {
	if a.Redis == nil {
		return false
	}
	ok, err := a.Redis.SIsMember(ctx, webhookUsersKey, userID).Result()
	if err != nil {
		return false
	}
	return ok
}

// refreshWebhookUserActive reconciles a single user's membership in wh:users
// after a webhook CRUD change or auto-disable.
func (a *App) refreshWebhookUserActive(ctx context.Context, userID string) {
	if a.Redis == nil || userID == "" {
		return
	}
	var count int
	if err := a.DB.QueryRowContext(ctx,
		"SELECT COUNT(*) FROM webhooks WHERE user_id = ? AND is_active = 1", userID).Scan(&count); err != nil {
		return
	}
	if count > 0 {
		_ = a.Redis.SAdd(ctx, webhookUsersKey, userID).Err()
	} else {
		_ = a.Redis.SRem(ctx, webhookUsersKey, userID).Err()
	}
}

// rebuildWebhookUserSet reconstructs wh:users from the DB. Mandatory at boot
// (Redis is ephemeral — without this, webhooks silently stop after a Redis
// restart) and run each maintenance tick as a backstop. Built into a temp key
// then RENAMEd so an in-flight reader never sees a half-populated set.
func (a *App) rebuildWebhookUserSet(ctx context.Context) {
	if a.Redis == nil {
		return
	}
	rows, err := a.DB.QueryContext(ctx, "SELECT DISTINCT user_id FROM webhooks WHERE is_active = 1")
	if err != nil {
		return
	}
	defer rows.Close()
	tmp := webhookUsersKey + ":rebuild"
	_ = a.Redis.Del(ctx, tmp).Err()
	pipe := a.Redis.Pipeline()
	n := 0
	for rows.Next() {
		var uid string
		if err := rows.Scan(&uid); err != nil {
			return
		}
		pipe.SAdd(ctx, tmp, uid)
		n++
	}
	if err := rows.Err(); err != nil {
		return
	}
	if n == 0 {
		// No active webhooks anywhere; clear the live set.
		_ = a.Redis.Del(ctx, webhookUsersKey).Err()
		_ = a.Redis.Del(ctx, tmp).Err()
		return
	}
	if _, err := pipe.Exec(ctx); err != nil {
		return
	}
	_ = a.Redis.Rename(ctx, tmp, webhookUsersKey).Err()
}

func eventMatches(events, action string) bool {
	events = strings.TrimSpace(events)
	if events == "" || events == "*" {
		return true
	}
	for _, e := range strings.Split(events, ",") {
		if strings.TrimSpace(e) == action {
			return true
		}
	}
	return false
}

// webhookBackoffWait sleeps before the next attempt, honoring Retry-After on
// 429 and clamping to the remaining per-job deadline. Returns false if there's
// no budget left or the context was cancelled.
func webhookBackoffWait(ctx context.Context, attempt, status int, retryAfter string) bool {
	idx := attempt - 1
	if idx >= len(webhookBackoffDurations) {
		idx = len(webhookBackoffDurations) - 1
	}
	wait := webhookBackoffDurations[idx]
	if status == http.StatusTooManyRequests {
		if ra := parseRetryAfter(retryAfter); ra > 0 {
			wait = ra
		}
	}
	if dl, ok := ctx.Deadline(); ok {
		if rem := time.Until(dl) - time.Second; rem < wait {
			wait = rem
		}
	}
	if wait <= 0 {
		return false
	}
	select {
	case <-time.After(wait):
		return true
	case <-ctx.Done():
		return false
	}
}

// parseRetryAfter accepts both delta-seconds and an HTTP-date.
func parseRetryAfter(v string) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if secs, err := strconv.Atoi(v); err == nil {
		if secs < 0 {
			return 0
		}
		return time.Duration(secs) * time.Second
	}
	if t, err := http.ParseTime(v); err == nil {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

func webhookFailMsg(status int, err error) string {
	if err != nil {
		return err.Error()
	}
	return fmt.Sprintf("HTTP %d", status)
}

func truncateStr(s string, max int) string {
	if len(s) > max {
		return s[:max]
	}
	return s
}
