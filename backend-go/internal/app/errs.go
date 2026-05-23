package app

// Typed error sentinels used across the app package. Centralising these
// kills the string-matching anti-pattern that previously sprinkled
// `strings.Contains(err.Error(), "folder not found")` and friends through
// the handler files — fragile on driver upgrades, broken by error
// wrapping, and undiscoverable in a code search. Use errors.Is to test.

import (
	"errors"

	"github.com/go-sql-driver/mysql"
)

// ErrFolderNotFound is returned by helpers that look up a folder by ID +
// user and find no row (or one that's soft-deleted). Distinguished from
// generic db_error so handlers can surface a 400/404 instead of 500.
var ErrFolderNotFound = errors.New("folder not found")

// ErrSharePassword is returned when a share resolution fails because the
// supplied X-Share-Password didn't match. Distinguished from a generic
// 401 so the rate limiter can count it separately if we want.
var ErrSharePassword = errors.New("share password mismatch")

// ErrDuplicate signals that an INSERT hit a UNIQUE / PRIMARY KEY conflict.
// Handlers translate this to 409 Conflict.
var ErrDuplicate = errors.New("duplicate row")

// IsDuplicate inspects err for a MariaDB / MySQL ER_DUP_ENTRY (1062). Wraps
// the typed-assertion dance so handlers can write
//
//	if app.IsDuplicate(err) { … }
//
// instead of strings.Contains(err.Error(), "Duplicate"). The latter breaks
// silently when the driver upgrades its error text or localises it.
func IsDuplicate(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrDuplicate) {
		return true
	}
	var mErr *mysql.MySQLError
	if errors.As(err, &mErr) {
		return mErr.Number == 1062
	}
	return false
}
