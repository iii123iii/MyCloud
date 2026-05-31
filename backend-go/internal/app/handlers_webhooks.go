package app

// Per-user outbound webhook management (interactive-session only — in
// patBlockedRoutes).
//
//   GET    /api/v2/me/webhooks                    → list
//   POST   /api/v2/me/webhooks                    → create; returns secret ONCE
//   PATCH  /api/v2/me/webhooks/{id}               → update (url/events/active)
//   DELETE /api/v2/me/webhooks/{id}               → delete
//   GET    /api/v2/me/webhooks/{id}/deliveries    → recent delivery log
//   POST   /api/v2/me/webhooks/{id}:test          → synchronous test delivery
//   POST   /api/v2/me/webhooks/{id}:rotate-secret → rotate secret (grace window)

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"mycloud/backend-go/internal/httpapi"
)

func (a *App) handleListMyWebhooks(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, url, events, is_active, failure_count, last_delivery_at, created_at, updated_at
		FROM webhooks WHERE user_id = ? ORDER BY created_at DESC`, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var id, hookURL, events string
		var isActive bool
		var failureCount int
		var lastDelivery sql.NullTime
		var createdAt, updatedAt time.Time
		if err := rows.Scan(&id, &hookURL, &events, &isActive, &failureCount,
			&lastDelivery, &createdAt, &updatedAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		out = append(out, map[string]any{
			"id":               id,
			"url":              hookURL,
			"events":           splitCSVList(events),
			"is_active":        isActive,
			"failure_count":    failureCount,
			"last_delivery_at": nullTimeJSON(lastDelivery),
			"created_at":       createdAt,
			"updated_at":       updatedAt,
		})
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"webhooks": out}, nil)
}

func (a *App) handleCreateMyWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	var payload struct {
		URL      string   `json:"url"`
		Events   []string `json:"events"`
		IsActive *bool    `json:"is_active"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	payload.URL = strings.TrimSpace(payload.URL)
	if payload.URL == "" {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "URL is required")
		return
	}
	if len(payload.URL) > 2048 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "URL is too long")
		return
	}
	if err := a.validateWebhookURL(r.Context(), payload.URL); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", err.Error())
		return
	}
	events := normalizeEvents(payload.Events)
	isActive := true
	if payload.IsActive != nil {
		isActive = *payload.IsActive
	}
	if a.Config.WebhookMaxPerUser > 0 {
		var count int
		if err := a.DB.QueryRowContext(r.Context(),
			"SELECT COUNT(*) FROM webhooks WHERE user_id = ?", userID).Scan(&count); err != nil {
			respondDBError(w, r, err)
			return
		}
		if count >= a.Config.WebhookMaxPerUser {
			httpapi.Error(w, http.StatusConflict, "limit_reached",
				"You have reached the maximum number of webhooks")
			return
		}
	}
	secret, err := generateWebhookSecret()
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	id := uuid.NewString()
	if _, err := a.DB.ExecContext(r.Context(), `
		INSERT INTO webhooks (id, user_id, url, secret, events, is_active)
		VALUES (?, ?, ?, ?, ?, ?)`,
		id, userID, payload.URL, secret, events, isActive); err != nil {
		respondDBError(w, r, err)
		return
	}
	if isActive {
		a.refreshWebhookUserActive(r.Context(), userID)
	}
	writeActivity(r.Context(), a.DB, &userID, "webhook.create", "webhook", id, clientIP(r),
		map[string]any{"url": payload.URL, "events": events})
	httpapi.JSON(w, http.StatusCreated, map[string]any{
		"id":            id,
		"url":           payload.URL,
		"events":        splitCSVList(events),
		"is_active":     isActive,
		"failure_count": 0,
		"created_at":    time.Now().UTC(),
		// The signing secret is returned exactly once, here.
		"secret": secret,
	}, nil)
}

func (a *App) handleUpdateMyWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	var payload struct {
		URL      *string   `json:"url"`
		Events   *[]string `json:"events"`
		IsActive *bool     `json:"is_active"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		httpapi.Error(w, http.StatusBadRequest, "bad_request", "Invalid JSON body")
		return
	}
	sets := make([]string, 0, 4)
	args := make([]any, 0, 5)
	if payload.URL != nil {
		u := strings.TrimSpace(*payload.URL)
		if u == "" || len(u) > 2048 {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", "Invalid URL")
			return
		}
		if err := a.validateWebhookURL(r.Context(), u); err != nil {
			httpapi.Error(w, http.StatusBadRequest, "validation_error", err.Error())
			return
		}
		sets = append(sets, "url=?")
		args = append(args, u)
	}
	if payload.Events != nil {
		sets = append(sets, "events=?")
		args = append(args, normalizeEvents(*payload.Events))
	}
	if payload.IsActive != nil {
		sets = append(sets, "is_active=?")
		args = append(args, *payload.IsActive)
		if *payload.IsActive {
			// Re-enabling clears the failure streak so a previously
			// auto-disabled hook gets a clean slate.
			sets = append(sets, "failure_count=0")
		}
	}
	if len(sets) == 0 {
		httpapi.Error(w, http.StatusBadRequest, "validation_error", "Nothing to update")
		return
	}
	args = append(args, id, userID)
	res, err := a.DB.ExecContext(r.Context(),
		"UPDATE webhooks SET "+strings.Join(sets, ", ")+" WHERE id=? AND user_id=?", args...)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	// Activation state may have changed; reconcile fan-out membership.
	a.refreshWebhookUserActive(r.Context(), userID)
	httpapi.JSON(w, http.StatusOK, map[string]any{"message": "Updated"}, nil)
}

func (a *App) handleDeleteMyWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	res, err := a.DB.ExecContext(r.Context(),
		"DELETE FROM webhooks WHERE id=? AND user_id=?", id, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	a.refreshWebhookUserActive(r.Context(), userID)
	writeActivity(r.Context(), a.DB, &userID, "webhook.delete", "webhook", id, clientIP(r), nil)
	httpapi.NoContent(w)
}

func (a *App) handleListWebhookDeliveries(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	if err := a.ensureOwnWebhook(r.Context(), userID, id); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Webhook not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	rows, err := a.DB.QueryContext(r.Context(), `
		SELECT id, event_type, response_status, error, attempts, status, created_at, delivered_at
		FROM webhook_deliveries WHERE webhook_id = ? ORDER BY created_at DESC LIMIT 50`, id)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	defer rows.Close()
	out := make([]map[string]any, 0)
	for rows.Next() {
		var did, eventType, status string
		var respStatus sql.NullInt64
		var errMsg sql.NullString
		var attempts int
		var createdAt time.Time
		var deliveredAt sql.NullTime
		if err := rows.Scan(&did, &eventType, &respStatus, &errMsg, &attempts, &status, &createdAt, &deliveredAt); err != nil {
			respondDBError(w, r, err)
			return
		}
		row := map[string]any{
			"id":              did,
			"event_type":      eventType,
			"status":          status,
			"attempts":        attempts,
			"created_at":      createdAt,
			"delivered_at":    nullTimeJSON(deliveredAt),
			"response_status": nil,
			"error":           nullStringJSON(errMsg),
		}
		if respStatus.Valid {
			row["response_status"] = respStatus.Int64
		}
		out = append(out, row)
	}
	httpapi.JSON(w, http.StatusOK, map[string]any{"deliveries": out}, nil)
}

func (a *App) handleTestMyWebhook(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	wh, err := a.loadWebhookForUser(r.Context(), userID, id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			httpapi.Error(w, http.StatusNotFound, "not_found", "Webhook not found")
			return
		}
		respondDBError(w, r, err)
		return
	}
	ev := webhookEvent{
		UserID:       userID,
		Action:       "webhook.test",
		ResourceType: "webhook",
		ResourceID:   id,
		Details:      json.RawMessage(`{"message":"This is a test event from MyCloud"}`),
		TS:           time.Now().UTC(),
	}
	// Bypass the queue and the event filter (a "webhook.test" event would
	// otherwise be dropped for a hook not subscribed to it). Single attempt so
	// the result returns inline quickly; bound it to keep the request snappy.
	// updateHealth=false: a manual test must not reset the real failure streak
	// or count toward auto-disabling the webhook.
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	status, derr := a.dispatchToWebhook(ctx, wh, ev, 1, false)
	resp := map[string]any{"ok": derr == nil, "response_status": status}
	if derr != nil {
		resp["error"] = derr.Error()
	}
	httpapi.JSON(w, http.StatusOK, resp, nil)
}

func (a *App) handleRotateMyWebhookSecret(w http.ResponseWriter, r *http.Request) {
	userID := userIDFrom(r)
	id := chi.URLParam(r, "id")
	graceHours := a.Config.WebhookSecretRotationGraceHours
	if graceHours <= 0 {
		graceHours = 24
	}
	newSecret, err := generateWebhookSecret()
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	graceUntil := time.Now().UTC().Add(time.Duration(graceHours) * time.Hour)
	// MySQL evaluates SET left-to-right, so previous_secret captures the OLD
	// secret value before secret is reassigned.
	res, err := a.DB.ExecContext(r.Context(), `
		UPDATE webhooks
		SET previous_secret = secret, previous_secret_expires_at = ?, secret = ?, updated_at = NOW()
		WHERE id = ? AND user_id = ?`, graceUntil, newSecret, id, userID)
	if err != nil {
		respondDBError(w, r, err)
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		httpapi.Error(w, http.StatusNotFound, "not_found", "Webhook not found")
		return
	}
	writeActivity(r.Context(), a.DB, &userID, "webhook.rotate_secret", "webhook", id, clientIP(r),
		map[string]any{"grace_hours": graceHours})
	httpapi.JSON(w, http.StatusOK, map[string]any{
		"secret":      newSecret,
		"grace_hours": graceHours,
	}, nil)
}

// ensureOwnWebhook returns sql.ErrNoRows if the webhook doesn't exist or isn't
// the caller's.
func (a *App) ensureOwnWebhook(ctx context.Context, userID, id string) error {
	var owner string
	if err := a.DB.QueryRowContext(ctx, "SELECT user_id FROM webhooks WHERE id=?", id).Scan(&owner); err != nil {
		return err
	}
	if owner != userID {
		return sql.ErrNoRows
	}
	return nil
}

// loadWebhookForUser fetches the fields needed to dispatch, scoped to the owner.
func (a *App) loadWebhookForUser(ctx context.Context, userID, id string) (webhookRow, error) {
	var wh webhookRow
	err := a.DB.QueryRowContext(ctx, `
		SELECT id, url, secret, previous_secret, previous_secret_expires_at, events
		FROM webhooks WHERE id = ? AND user_id = ?`, id, userID).
		Scan(&wh.id, &wh.url, &wh.secret, &wh.prevSecret, &wh.prevSecretExpires, &wh.events)
	return wh, err
}

// validateWebhookURL enforces scheme + SSRF policy at create/update time for a
// friendly early error. The dial-time guard (ssrfControl) remains the
// authoritative check against DNS rebinding.
func (a *App) validateWebhookURL(ctx context.Context, raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return errors.New("invalid URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return errors.New("URL must use http or https")
	}
	host := u.Hostname()
	if host == "" {
		return errors.New("URL must include a host")
	}
	if a.Config.WebhookAllowPrivateTargets {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if isBlockedIP(ip) {
			return errors.New("URL resolves to a blocked (private/loopback) address")
		}
		return nil
	}
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", host)
	if err != nil {
		return errors.New("could not resolve URL host")
	}
	for _, ip := range ips {
		if isBlockedIP(ip) {
			return errors.New("URL resolves to a blocked (private/loopback) address")
		}
	}
	return nil
}

// normalizeEvents dedupes/sorts the requested action filter. An explicit "*" or
// an empty selection both mean "all of the owner's events" and are stored as "*".
func normalizeEvents(events []string) string {
	seen := map[string]bool{}
	out := make([]string, 0, len(events))
	for _, e := range events {
		e = strings.TrimSpace(e)
		if e == "" {
			continue
		}
		if e == "*" {
			return "*"
		}
		if !seen[e] {
			seen[e] = true
			out = append(out, e)
		}
	}
	if len(out) == 0 {
		return "*"
	}
	sort.Strings(out)
	return strings.Join(out, ",")
}

func splitCSVList(csv string) []string {
	out := []string{}
	for _, p := range strings.Split(csv, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func generateWebhookSecret() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return "mc_whsec_" + base64.RawURLEncoding.EncodeToString(buf), nil
}
