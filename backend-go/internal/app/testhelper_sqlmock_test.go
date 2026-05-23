package app

// Tiny aliases so bulk test files don't all need to import sqlmock.

import (
	"database/sql/driver"

	"github.com/DATA-DOG/go-sqlmock"
)

func sqlmockResult(lastInsertID, rowsAffected int64) driver.Result {
	return sqlmock.NewResult(lastInsertID, rowsAffected)
}

func sqlmockNewRows(cols []string) *sqlmock.Rows {
	return sqlmock.NewRows(cols)
}
