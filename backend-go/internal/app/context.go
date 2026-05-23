package app

import (
	"context"
	"net/http"
	"sync"
)

type ctxKey string

const (
	userIDKey       ctxKey = "userID"
	userRoleKey     ctxKey = "userRole"
	userIDHolderKey ctxKey = "userIDHolder"
	userJTIKey      ctxKey = "userJTI"
	// mustChangePwKey carries the user's must_change_password flag as
	// loaded by requireAuth, so requirePasswordCurrent can gate routes
	// without a second DB hit.
	mustChangePwKey ctxKey = "userMustChangePw"
)

// userIDHolder is a mutable slot the access-log middleware seeds at the top of
// the chain so requireAuth (which lives deeper) can publish the authenticated
// user back up to the logger. A pointer is shared by reference across the
// chi-style "build a new request" middleware pattern, where each layer's
// r.WithContext copies the *http.Request but the underlying holder is the
// same allocation.
type userIDHolder struct {
	mu sync.Mutex
	id string
}

func (h *userIDHolder) set(id string) {
	if h == nil {
		return
	}
	h.mu.Lock()
	h.id = id
	h.mu.Unlock()
}

func (h *userIDHolder) get() string {
	if h == nil {
		return ""
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.id
}

func withUser(ctx context.Context, userID, role string) context.Context {
	// Publish to the access-log holder (no-op if access-log middleware didn't
	// seed one, e.g. unit tests).
	if h, ok := ctx.Value(userIDHolderKey).(*userIDHolder); ok {
		h.set(userID)
	}
	ctx = context.WithValue(ctx, userIDKey, userID)
	ctx = context.WithValue(ctx, userRoleKey, role)
	return ctx
}

func userIDFrom(r *http.Request) string {
	v, _ := r.Context().Value(userIDKey).(string)
	return v
}

func userRoleFrom(r *http.Request) string {
	v, _ := r.Context().Value(userRoleKey).(string)
	return v
}

// jtiFrom returns the access token's JTI for the current request, or "" if
// requireAuth hasn't run (public routes). Used by /me/sessions handlers to
// (a) tag the caller's own session row and (b) refuse self-revoke.
func jtiFrom(r *http.Request) string {
	v, _ := r.Context().Value(userJTIKey).(string)
	return v
}

// userIDHolderFrom returns the slot if present (the access-log middleware
// installs one). Used by handlers that want to update the "current user" for
// log purposes without going through requireAuth — e.g. a future
// impersonation flow.
func userIDHolderFrom(ctx context.Context) *userIDHolder {
	h, _ := ctx.Value(userIDHolderKey).(*userIDHolder)
	return h
}
