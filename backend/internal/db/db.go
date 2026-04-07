package db

import (
	"database/sql"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

// Open creates a *sql.DB connected to MariaDB/MySQL with a bounded connection pool.
func Open(host, port, name, user, password string) (*sql.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&loc=UTC",
		user, password, host, port, name)
	db, err := sql.Open("mysql", dsn)
	if err != nil {
		return nil, err
	}
	// Target ~30 total connections (matches C++ backend behaviour).
	db.SetMaxOpenConns(30)
	db.SetMaxIdleConns(10)
	return db, nil
}
