package app

// QR device-linking: log a NEW device (the phone) into an existing account by
// scanning a QR shown on an ALREADY-authenticated browser.
//
//   POST /api/v2/device-link/create        (authed)  browser mints a pairing
//   GET  /api/v2/device-link/{code}        (authed)  browser polls status
//   POST /api/v2/device-link/{code}:approve(authed)  browser approves the phone
//   POST /api/v2/device-link/{code}:deny   (authed)  browser denies the phone
//   POST /api/v2/device-link/claim         (public)  phone claims the code
//   POST /api/v2/device-link/poll          (public)  phone fetches its tokens
//
// Trust model: the browser is the trusted device. The QR carries a one-time
// `code` plus a 256-bit `verifier` whose SHA-256 alone is stored server-side.
// An unauthenticated poller that never scanned the QR can't produce the
// verifier, so knowing the code is useless. Before any token is minted the
// browser shows the phone's reported model + IP and the user must approve —
// this defeats an attacker who merely photographs the QR.
//
// State machine (Redis hash `pair:<code>`):
//   pending -> awaiting_approval (phone claimed) -> approved (tokens minted)
//   -> consumed (phone fetched once); terminal: denied; missing key = expired.

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"

	"mycloud/backend-go/internal/httpapi"
	"mycloud/backend-go/internal/wsHub"
)

const (
	// pairPendingTTL bounds how long an unscanned QR is valid. Short so a
	// photographed-but-unused code expires quickly.
	pairPendingTTL = 60 * time.Second
	// pairApprovalTTL is the window after a scan for the user to approve and
	// the phone to collect its tokens.
	pairApprovalTTL = 120 * time.Second
	// pairTombstoneTTL keeps a short terminal marker (consumed/denied) so the
	// phone's next poll sees the real outcome instead of a bare "expired".
	pairTombstoneTTL = 10 * time.Second
)

func pairKey(code string) string { return "pair:" + code }

// randToken returns 32 bytes of CSPRNG entropy as unpadded base64url (URL- and
// path-safe: alphabet is [A-Za-z0-9-_], so it never collides with the ":approve"
// route suffix).
func randToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func verifierHash(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return hex.EncodeToString(sum[:])
}

// pollConsumeScript atomically delivers the minted tokens exactly once. It
// verifies the verifier hash, and only when state==approved does it return the
// tokens, flip state to "consumed", and drop the tokens field — so a racing
// second poller can never retrieve them.
//
//   KEYS[1] = pair:<code>   ARGV[1] = verifier_hash   ARGV[2] = tombstone ms
//   returns {state} or {"approved", tokensJSON}
var pollConsumeScript = redis.NewScript(`
local key = KEYS[1]
if redis.call('EXISTS', key) == 0 then
  return {'expired'}
end
if redis.call('HGET', key, 'verifier_hash') ~= ARGV[1] then
  return {'bad_verifier'}
end
local state = redis.call('HGET', key, 'state')
if state == 'approved' then
  local tokens = redis.call('HGET', key, 'tokens')
  redis.call('HSET', key, 'state', 'consumed')
  redis.call('HDEL', key, 'tokens')
  redis.call('PEXPIRE', key, tonumber(ARGV[2]))
  return {'approved', tokens}
end
return {state}
`)

// ---- authenticated (browser) handlers ----------------------------------

func (a *App) handleDeviceLinkCreate(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	userID := userIDFrom(r)
	role := userRoleFrom(r)
	code, err := randToken()
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", "Could not generate code")
		return
	}
	verifier, err := randToken()
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", "Could not generate code")
		return
	}
	ctx := r.Context()
	key := pairKey(code)
	if err := a.Redis.HSet(ctx, key, map[string]any{
		"state":         "pending",
		"user_id":       userID,
		"role":          role,
		"verifier_hash": verifierHash(verifier),
		"created_at":    time.Now().UTC().Format(time.RFC3339),
	}).Err(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", "Could not create pairing")
		return
	}
	_ = a.Redis.PExpire(ctx, key, pairPendingTTL).Err()
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"code":       code,
		"verifier":   verifier,
		"url":        a.publicBackendURL(r),
		"expires_in": int(pairPendingTTL.Seconds()),
	}, nil)
}

func (a *App) handleDeviceLinkStatus(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	code := chi.URLParam(r, "code")
	fields, err := a.Redis.HGetAll(r.Context(), pairKey(code)).Result()
	if err != nil || len(fields) == 0 {
		httpapi.JSON(w, http.StatusOK, map[string]any{"state": "expired"}, nil)
		return
	}
	// One user must never inspect another user's pairing record.
	if fields["user_id"] != userIDFrom(r) {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Pairing not found")
		return
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"state":        fields["state"],
		"device_name":  fields["device_name"],
		"device_model": fields["device_model"],
		"device_ip":    fields["device_ip"],
		"platform":     fields["platform"],
	}, nil)
}

func (a *App) handleDeviceLinkApprove(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	userID := userIDFrom(r)
	code := chi.URLParam(r, "code")
	ctx := r.Context()
	key := pairKey(code)
	fields, err := a.Redis.HGetAll(ctx, key).Result()
	if err != nil || len(fields) == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Pairing expired or not found")
		return
	}
	if fields["user_id"] != userID {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Pairing not found")
		return
	}
	if fields["state"] != "awaiting_approval" {
		httpapi.Error(w, http.StatusConflict, "invalid_state", "No device is waiting for approval")
		return
	}

	tokens, err := a.Auth.IssuePair(userID, fields["role"])
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", err.Error())
		return
	}
	// Enrich with profile fields so the phone gets the same payload as /auth/login.
	var username, email string
	var mustChange bool
	_ = a.DB.QueryRowContext(ctx,
		"SELECT username, email, must_change_password FROM users WHERE id=?", userID).
		Scan(&username, &email, &mustChange)
	tokens["username"] = username
	tokens["email"] = email
	tokens["must_change_password"] = mustChange

	// Record the session against the PHONE's reported identity (captured at
	// claim time), not the approving browser's request.
	jti, _ := tokens["access_jti"].(string)
	expires, _ := tokens["access_expires_at"].(time.Time)
	if expires.IsZero() {
		expires = time.Now().Add(time.Duration(a.Config.AccessTokenTTL) * time.Second)
	}
	a.recordSessionWithDevice(ctx, jti, userID, fields["device_name"], fields["device_ua"], fields["device_ip"], expires)

	blob, err := json.Marshal(tokens)
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "token_error", "Could not encode tokens")
		return
	}
	if err := a.Redis.HSet(ctx, key, map[string]any{
		"state":  "approved",
		"tokens": string(blob),
	}).Err(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", "Could not approve pairing")
		return
	}
	_ = a.Redis.PExpire(ctx, key, pairApprovalTTL).Err()
	writeActivity(ctx, a.DB, &userID, "user.device_link_approve", "user", userID, clientIP(r),
		map[string]any{"device_name": fields["device_name"]})
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "approved"}, nil)
}

func (a *App) handleDeviceLinkDeny(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	userID := userIDFrom(r)
	code := chi.URLParam(r, "code")
	ctx := r.Context()
	key := pairKey(code)
	owner, err := a.Redis.HGet(ctx, key, "user_id").Result()
	if err != nil || owner == "" {
		// Already gone — nothing to deny.
		httpapi.NoContent(w)
		return
	}
	if owner != userID {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Pairing not found")
		return
	}
	// Leave a short tombstone so the phone's next poll reports "denied".
	_ = a.Redis.HSet(ctx, key, "state", "denied").Err()
	_ = a.Redis.HDel(ctx, key, "tokens").Err()
	_ = a.Redis.PExpire(ctx, key, pairTombstoneTTL).Err()
	httpapi.NoContent(w)
}

// ---- public (phone) handlers -------------------------------------------

type deviceClaimPayload struct {
	Code        string `json:"code"`
	Verifier    string `json:"verifier"`
	DeviceName  string `json:"device_name"`
	DeviceModel string `json:"device_model"`
	Platform    string `json:"platform"`
}

func (a *App) handleDeviceLinkClaim(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	var p deviceClaimPayload
	if err := decodeJSON(r, &p); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if p.Code == "" || p.Verifier == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "code and verifier are required")
		return
	}
	ctx := r.Context()
	key := pairKey(p.Code)
	fields, err := a.Redis.HGetAll(ctx, key).Result()
	if err != nil || len(fields) == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Pairing expired or not found")
		return
	}
	// Constant-time compare of the verifier hash.
	want := []byte(fields["verifier_hash"])
	got := []byte(verifierHash(p.Verifier))
	if len(want) != len(got) || subtle.ConstantTimeCompare(want, got) != 1 {
		httpapi.Error(w, http.StatusForbidden, "invalid_verifier", "Invalid pairing code")
		return
	}
	if fields["state"] != "pending" {
		httpapi.Error(w, http.StatusConflict, "already_claimed", "This code has already been used")
		return
	}

	deviceName := truncate(p.DeviceName, 120)
	if deviceName == "" {
		deviceName = "New device"
	}
	if err := a.Redis.HSet(ctx, key, map[string]any{
		"state":        "awaiting_approval",
		"device_name":  deviceName,
		"device_model": truncate(p.DeviceModel, 120),
		"platform":     truncate(p.Platform, 80),
		"device_ip":    clientIP(r),
		"device_ua":    truncate(r.UserAgent(), 500),
	}).Err(); err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", "Could not claim pairing")
		return
	}
	_ = a.Redis.PExpire(ctx, key, pairApprovalTTL).Err()

	// Tell the browser (over its already-open WS) that a device is waiting so
	// it can render the approval card without polling.
	if a.Hub != nil {
		userID := fields["user_id"]
		_ = a.Hub.Broker().Publish(ctx, "user:"+userID, wsHub.Envelope{
			Type:  "device_link_scanned",
			Topic: "user:" + userID,
			Data: map[string]any{
				"code":         p.Code,
				"device_name":  deviceName,
				"device_model": truncate(p.DeviceModel, 120),
				"device_ip":    clientIP(r),
				"platform":     truncate(p.Platform, 80),
			},
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"state": "awaiting_approval"}, nil)
}

type devicePollPayload struct {
	Code     string `json:"code"`
	Verifier string `json:"verifier"`
}

func (a *App) handleDeviceLinkPoll(w http.ResponseWriter, r *http.Request) {
	if a.Redis == nil {
		httpapi.Error(w, http.StatusServiceUnavailable, "unavailable", "Device linking is unavailable")
		return
	}
	var p devicePollPayload
	if err := decodeJSON(r, &p); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	if p.Code == "" || p.Verifier == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "code and verifier are required")
		return
	}
	ctx := r.Context()
	res, err := pollConsumeScript.Run(ctx, a.Redis,
		[]string{pairKey(p.Code)}, verifierHash(p.Verifier),
		pairTombstoneTTL.Milliseconds()).Result()
	if err != nil {
		httpapi.Error(w, http.StatusInternalServerError, "redis_error", "Could not poll pairing")
		return
	}
	arr, _ := res.([]any)
	if len(arr) == 0 {
		httpapi.JSON(w, http.StatusOK, map[string]any{"state": "expired"}, nil)
		return
	}
	state, _ := arr[0].(string)
	switch state {
	case "bad_verifier":
		httpapi.Error(w, http.StatusForbidden, "invalid_verifier", "Invalid pairing code")
		return
	case "approved":
		blob, _ := arr[1].(string)
		// Notify the browser that the link completed so it can clear the QR.
		userID := a.Redis.HGet(ctx, pairKey(p.Code), "user_id").Val()
		if a.Hub != nil && userID != "" {
			_ = a.Hub.Broker().Publish(ctx, "user:"+userID, wsHub.Envelope{
				Type:  "device_link_consumed",
				Topic: "user:" + userID,
				Data:  map[string]any{"code": p.Code},
			})
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		// blob is the already-encoded token map; wrap it in the standard
		// {data:...} envelope so the client unwraps it like /auth/login.
		_, _ = w.Write([]byte(`{"data":` + blob + `}`))
		return
	default:
		// pending | awaiting_approval | denied | consumed | expired
		httpapi.JSON(w, http.StatusOK, map[string]any{"state": state}, nil)
		return
	}
}

// truncate trims s to at most n bytes. Used to bound client-supplied strings
// before they reach Redis / the DB.
func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

// recordSessionWithDevice writes a session row using an explicit, client-
// supplied device name plus the device's own UA/IP (captured at QR-claim time),
// rather than the approving browser's request metadata. deviceName takes
// precedence as the human label; the UA-derived label is kept as a fallback.
func (a *App) recordSessionWithDevice(ctx context.Context, jti, userID, deviceName, ua, ip string, expires time.Time) {
	if jti == "" {
		return
	}
	label := deviceLabelFromUA(ua)
	var nameCol any
	if deviceName != "" {
		nameCol = deviceName
	} else {
		nameCol = sql.NullString{}
	}
	_, _ = a.DB.ExecContext(ctx, `
		INSERT INTO sessions (jti, user_id, device_label, device_name, user_agent, ip_address, expires_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
		    last_seen_at = NOW(),
		    device_name  = VALUES(device_name),
		    user_agent   = VALUES(user_agent),
		    ip_address   = VALUES(ip_address),
		    expires_at   = VALUES(expires_at)`,
		jti, userID, label, nameCol, ua, ip, expires)
}
