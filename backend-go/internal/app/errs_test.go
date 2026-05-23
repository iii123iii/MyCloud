package app

import (
	"errors"
	"fmt"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// IsDuplicate is the canonical replacement for
// strings.Contains(err.Error(), "Duplicate") — fragile to driver upgrades
// and localisation. Test that it correctly identifies:
//   - direct MySQLError with number 1062
//   - wrapped MySQLError (via fmt.Errorf %w)
//   - the typed ErrDuplicate sentinel
// And does NOT match:
//   - nil
//   - other MySQL error numbers
//   - generic errors that happen to contain "Duplicate" in their text
func TestIsDuplicate_DirectMySQLError(t *testing.T) {
	err := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry 'foo' for key 'users.email'"}
	if !IsDuplicate(err) {
		t.Errorf("expected IsDuplicate=true for MySQLError 1062")
	}
}

func TestIsDuplicate_WrappedMySQLError(t *testing.T) {
	inner := &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}
	wrapped := fmt.Errorf("insert user: %w", inner)
	if !IsDuplicate(wrapped) {
		t.Errorf("IsDuplicate should unwrap via errors.As")
	}
}

func TestIsDuplicate_OtherMySQLError(t *testing.T) {
	other := &mysql.MySQLError{Number: 1452, Message: "Foreign key fails"}
	if IsDuplicate(other) {
		t.Errorf("IsDuplicate should be false for non-1062")
	}
}

func TestIsDuplicate_NilSafe(t *testing.T) {
	if IsDuplicate(nil) {
		t.Errorf("IsDuplicate(nil) must be false")
	}
}

func TestIsDuplicate_StringMatchDoesntFool(t *testing.T) {
	// Previously, code grepped err.Error() for "Duplicate" — a generic
	// error containing that word would have spoofed it. Verify the typed
	// check refuses to match.
	tricky := errors.New("Duplicate keyword in user query")
	if IsDuplicate(tricky) {
		t.Errorf("IsDuplicate should require the typed MySQLError, not a string match")
	}
}

func TestIsDuplicate_ErrDuplicateSentinel(t *testing.T) {
	if !IsDuplicate(ErrDuplicate) {
		t.Errorf("IsDuplicate should match the explicit sentinel")
	}
	wrapped := fmt.Errorf("ctx: %w", ErrDuplicate)
	if !IsDuplicate(wrapped) {
		t.Errorf("IsDuplicate should match wrapped sentinel")
	}
}

func TestErrFolderNotFound_Identity(t *testing.T) {
	if !errors.Is(ErrFolderNotFound, ErrFolderNotFound) {
		t.Errorf("sentinel self-identity failed")
	}
	if errors.Is(ErrFolderNotFound, ErrSharePassword) {
		t.Errorf("distinct sentinels should not match")
	}
	wrapped := fmt.Errorf("looking up folder: %w", ErrFolderNotFound)
	if !errors.Is(wrapped, ErrFolderNotFound) {
		t.Errorf("wrapped sentinel must unwrap")
	}
}

func TestErrSharePassword_Identity(t *testing.T) {
	wrapped := fmt.Errorf("share access: %w", ErrSharePassword)
	if !errors.Is(wrapped, ErrSharePassword) {
		t.Errorf("wrapped ErrSharePassword should unwrap")
	}
}

func TestErrInvalidFilename_Identity(t *testing.T) {
	wrapped := fmt.Errorf("upload: %w", ErrInvalidFilename)
	if !errors.Is(wrapped, ErrInvalidFilename) {
		t.Errorf("wrapped ErrInvalidFilename should unwrap")
	}
}
