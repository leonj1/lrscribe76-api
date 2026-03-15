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
		return ErrDBUnavailable
	}

	newDB, err := sql.Open("postgres", connectionString)
	if err != nil {
		return err
	}

	if err = newDB.Ping(); err != nil {
		_ = newDB.Close()
		return err
	}

	oldDB := db
	db = newDB
	if oldDB != nil {
		_ = oldDB.Close()
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
