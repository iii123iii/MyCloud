package app

import (
	"net/http"
	"strings"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── admin demotion / disable guards ────────────────────────────────────────

// Self-disable must be blocked — locks the admin out of their own account.
func TestHandleAdminUpdateUser_SelfDisableBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-admin").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	w := rec()
	r := adminRequest("PATCH", "/admin/users/u-admin", `{"is_active":false}`)
	r = withChiParams(r, "id", "u-admin")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no_self_disable") {
		t.Errorf("expected no_self_disable code, got %s", w.Body.String())
	}
}

// Peer admin cannot be disabled — only the admin themselves (which is also
// blocked above).
func TestHandleAdminUpdateUser_PeerAdminDisableBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-peer-admin").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	w := rec()
	r := adminRequest("PATCH", "/admin/users/u-peer-admin", `{"is_active":false}`)
	r = withChiParams(r, "id", "u-peer-admin")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no_admin_disable") {
		t.Errorf("expected no_admin_disable code")
	}
}

// Cross-admin demotion blocked — the "demote then disable" attack path.
func TestHandleAdminUpdateUser_PeerAdminDemoteBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-peer").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	w := rec()
	r := adminRequest("PATCH", "/admin/users/u-peer", `{"role":"user"}`)
	r = withChiParams(r, "id", "u-peer")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "no_admin_demote") {
		t.Errorf("expected no_admin_demote code")
	}
}

// Last admin self-demote blocked — would leave zero admins.
func TestHandleAdminUpdateUser_LastAdminSelfDemoteBlocked(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-admin").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	a.mock.ExpectQuery("SELECT COUNT.\\*. FROM users WHERE role='admin'").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	w := rec()
	r := adminRequest("PATCH", "/admin/users/u-admin", `{"role":"user"}`)
	r = withChiParams(r, "id", "u-admin")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d body=%s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "last_admin") {
		t.Errorf("expected last_admin code")
	}
}

// Self-demote ALLOWED when another active admin exists.
func TestHandleAdminUpdateUser_SelfDemoteAllowedWhenMultipleAdmins(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("u-admin").
		WillReturnRows(sqlmock.NewRows([]string{"role"}).AddRow("admin"))
	a.mock.ExpectQuery("SELECT COUNT.\\*. FROM users WHERE role='admin'").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	// After guard passes, handler runs the dynamic UPDATE for "role".
	a.mock.ExpectExec("UPDATE users SET").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := adminRequest("PATCH", "/admin/users/u-admin", `{"role":"user"}`)
	r = withChiParams(r, "id", "u-admin")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// Target not found returns 404.
func TestHandleAdminUpdateUser_TargetNotFound404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT role FROM users WHERE id=").
		WithArgs("ghost").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := adminRequest("PATCH", "/admin/users/ghost", `{"is_active":false}`)
	r = withChiParams(r, "id", "ghost")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// Empty body rejected.
func TestHandleAdminUpdateUser_EmptyBody400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("PATCH", "/admin/users/x", `{}`)
	r = withChiParams(r, "id", "x")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// Malformed JSON rejected.
func TestHandleAdminUpdateUser_MalformedJSON400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("PATCH", "/admin/users/x", `{not-json`)
	r = withChiParams(r, "id", "x")
	a.handleAdminUpdateUser(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// requireAdmin middleware rejects non-admin role.
func TestRequireAdmin_NonAdminReturns403(t *testing.T) {
	a := newTestApp(t)
	handler := a.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	w := rec()
	r := authedRequest("GET", "/admin/users", "")
	handler.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "forbidden") {
		t.Errorf("expected forbidden code, got %s", w.Body.String())
	}
}

func TestRequireAdmin_AdminPassesThrough(t *testing.T) {
	a := newTestApp(t)
	hit := false
	handler := a.requireAdmin(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(200)
	}))
	w := rec()
	r := adminRequest("GET", "/admin/users", "")
	handler.ServeHTTP(w, r)
	if !hit {
		t.Errorf("admin should pass through")
	}
}

// requirePasswordCurrent middleware: must_change_password=true blocks all
// routes except the exempt list.
func TestRequirePasswordCurrent_BlocksProtectedRoute(t *testing.T) {
	a := newTestApp(t)
	hit := false
	handler := a.requirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(200)
	}))
	w := rec()
	r := authedRequest("GET", "/api/v2/files", "")
	// Inject must_change_password=true into ctx.
	r = r.WithContext(contextWithMustChange(r, true))
	handler.ServeHTTP(w, r)
	if hit {
		t.Errorf("handler should not be reached when must_change_password=true")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "password_change_required") {
		t.Errorf("expected password_change_required code")
	}
}

func TestRequirePasswordCurrent_AllowsWhenFlagFalse(t *testing.T) {
	a := newTestApp(t)
	hit := false
	handler := a.requirePasswordCurrent(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		w.WriteHeader(200)
	}))
	w := rec()
	r := authedRequest("GET", "/api/v2/files", "")
	handler.ServeHTTP(w, r)
	if !hit {
		t.Errorf("handler should be reached when flag is false")
	}
}
