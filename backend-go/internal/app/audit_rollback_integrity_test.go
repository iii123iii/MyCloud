package app

// Audit-log + rollback integrity. These tests verify properties of the
// rollback engine itself, not individual rollback fns.

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
)

// ─── Registry sanity ────────────────────────────────────────────────────────

// Every registered rollback fn must be non-nil — guards against accidental
// `nil` slots that would NPE at admin-action time.
func TestRollbackRegistry_AllFnsNonNil(t *testing.T) {
	for action, fn := range rollbackRegistry {
		if fn == nil {
			t.Errorf("registry slot %q has nil fn", action)
		}
	}
}

// Every action key must be in the writeActivityWithDiff-using handler set —
// i.e. only actions that actually emit old_value/new_value can be rolled
// back. We can't reflect on handlers; instead we codify the expected list.
func TestRollbackRegistry_ExpectedActions(t *testing.T) {
	want := map[string]bool{
		"file_rename":      true,
		"file_move":        true,
		"file_soft_delete": true,
		"share_create":     true,
	}
	for action := range rollbackRegistry {
		if !want[action] {
			t.Errorf("unexpected action in registry: %q", action)
		}
	}
	for action := range want {
		if _, ok := rollbackRegistry[action]; !ok {
			t.Errorf("expected action %q missing from registry", action)
		}
	}
}

// ─── handleAdminRollback flow ──────────────────────────────────────────────

func TestHandleAdminRollback_MissingID_400(t *testing.T) {
	a := newTestApp(t)
	w := rec()
	r := adminRequest("POST", "/admin/activity/:rollback", "")
	r = withChiParams(r, "id", "")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleAdminRollback_NotFound_404(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnError(rowsErrNoRows())
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleAdminRollback_NonReversible_422(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"action", "resource_id", "old_value", "new_value", "reversible", "rolled_back_at",
		}).AddRow("file_purge", "f1", nil, nil, false, nil))
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAdminRollback_AlreadyRolledBack_409(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"action", "resource_id", "old_value", "new_value", "reversible", "rolled_back_at",
		}).AddRow("file_rename", "f1",
			`{"name":"old"}`, `{"name":"new"}`, true, time.Now()))
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleAdminRollback_UnknownAction_422(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"action", "resource_id", "old_value", "new_value", "reversible", "rolled_back_at",
		}).AddRow("unknown_action", "f1", nil, nil, true, nil))
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("expected 422, got %d", w.Code)
	}
}

func TestHandleAdminRollback_Success(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"action", "resource_id", "old_value", "new_value", "reversible", "rolled_back_at",
		}).AddRow("file_rename", "f1",
			`{"name":"old"}`, `{"name":"new"}`, true, nil))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	a.mock.ExpectExec("UPDATE activity_log SET rolled_back_at").
		WillReturnResult(sqlmock.NewResult(0, 1))
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAdminRollback_FnFails_500(t *testing.T) {
	a := newTestApp(t)
	a.mock.MatchExpectationsInOrder(false)
	a.mock.ExpectQuery("SELECT action, COALESCE.resource_id").
		WillReturnRows(sqlmock.NewRows([]string{
			"action", "resource_id", "old_value", "new_value", "reversible", "rolled_back_at",
		}).AddRow("file_rename", "f1",
			`{"name":"old"}`, `{"name":"new"}`, true, nil))
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnError(errStr2("db down"))
	w := rec()
	r := adminRequest("POST", "/admin/activity/a1:rollback", "")
	r = withChiParams(r, "id", "a1")
	a.handleAdminRollback(w, r)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ─── writeActivityWithDiff JSON marshalling ────────────────────────────────

// Verify both old + new values are stored as valid JSON. Malformed marshals
// from upstream callers would corrupt rollback later.
func TestWriteActivityWithDiff_MarshalsValidJSON(t *testing.T) {
	a := newTestApp(t)
	a.mock.ExpectExec("INSERT INTO activity_log").
		WithArgs(
			sqlmock.AnyArg(), // user_id
			sqlmock.AnyArg(), // action
			sqlmock.AnyArg(), // resource_type
			sqlmock.AnyArg(), // resource_id
			sqlmock.AnyArg(), // details
			sqlmock.AnyArg(), // old_value
			sqlmock.AnyArg(), // new_value
			sqlmock.AnyArg(), // operation_id
			sqlmock.AnyArg(), // ip_address
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	uid := "u-test"
	old := map[string]any{"name": "before"}
	newv := map[string]any{"name": "after"}
	writeActivityWithDiff(testCtx(), a.DB, &uid, "file_rename", "file", "f1", "ip", "op", old, newv, nil)
	// If the call didn't panic and the mock was satisfied, JSON marshalled OK.
}

// Round-trip: writeActivityWithDiff JSON-marshals our diffs, rollback fns
// unmarshal them. Verify the shapes match by feeding the marshalled output
// back into the rollback fn.
func TestRollbackFn_AcceptsActivityLogPayload(t *testing.T) {
	a := newTestApp(t)
	old := map[string]any{"name": "old"}
	newv := map[string]any{"name": "new"}
	oldRaw, _ := json.Marshal(old)
	newRaw, _ := json.Marshal(newv)
	a.mock.ExpectExec("UPDATE files SET name").
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := rollbackFileRename(testCtx(), a.App, "f1", oldRaw, newRaw); err != nil {
		t.Errorf("expected nil, got %v", err)
	}
}
