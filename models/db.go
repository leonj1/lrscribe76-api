package models

import (
	"database/sql"
	"errors"

	_ "github.com/lib/pq"
)

var db *sql.DB
var ErrDBUnavailable = errors.New("database connection is not initialized")

type DB struct {
	*sql.DB
}

func InitDB(connectionString string) error {
	if connectionString == "" {
		db = nil
		return ErrDBUnavailable
	}

	var err error
	db, err = sql.Open("postgres", connectionString)
	if err != nil {
		db = nil
		return err
	}

	if err = db.Ping(); err != nil {
		_ = db.Close()
		db = nil
		return err
	}

	return nil
}

func requireDB() (*sql.DB, error) {
	if db == nil {
		return nil, ErrDBUnavailable
	}

	return db, nil
}

func SetDB(database *sql.DB) *sql.DB {
	previous := db
	db = database
	return previous
}
