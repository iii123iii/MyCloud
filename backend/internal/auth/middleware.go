package auth

import (
	"context"
	"net/http"
	"strings"

	"github.com/iii123iii/mycloud/backend/internal/utils"
)

type contextKey string

const (
	ContextUserID   contextKey = "userId"
	ContextUserRole contextKey = "userRole"
)

// UserIDFromCtx returns the authenticated user's ID from the request context.
func UserIDFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ContextUserID).(string)
	return v
}

// UserRoleFromCtx returns the authenticated user's role from the request context.
func UserRoleFromCtx(ctx context.Context) string {
	v, _ := ctx.Value(ContextUserRole).(string)
	return v
}

// Middleware returns an HTTP middleware that enforces JWT authentication.
// It reads the secret at call time (not construction time) so hot-reloaded
// config is automatically picked up.
func Middleware(secret string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if !strings.HasPrefix(authHeader, "Bearer ") {
				utils.ErrorJSON(w, http.StatusUnauthorized, "Missing or invalid Authorization header")
				return
			}
			tokenStr := authHeader[len("Bearer "):]
			claims, err := VerifyToken(tokenStr, secret)
			if err != nil {
				utils.ErrorJSON(w, http.StatusUnauthorized, "Invalid token: "+err.Error())
				return
			}
			if claims.Type != "access" {
				utils.ErrorJSON(w, http.StatusUnauthorized, "Expected access token")
				return
			}
			ctx := context.WithValue(r.Context(), ContextUserID, claims.UserID)
			ctx = context.WithValue(ctx, ContextUserRole, claims.Role)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AdminMiddleware enforces that the authenticated user has role "admin".
func AdminMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		role := UserRoleFromCtx(r.Context())
		if role != "admin" {
			utils.ErrorJSON(w, http.StatusForbidden, "Admin access required")
			return
		}
		next.ServeHTTP(w, r)
	})
}
