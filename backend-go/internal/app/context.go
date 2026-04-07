package app

import (
	"context"
	"net/http"
)

type ctxKey string

const (
	userIDKey   ctxKey = "userID"
	userRoleKey ctxKey = "userRole"
)

func withUser(ctx context.Context, userID, role string) context.Context {
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
