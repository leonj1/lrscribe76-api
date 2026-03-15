package models

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

const NotesTable = "notes"

type Note struct {
	Id 		int64
	Note 		string
	Creator		string
	CreateDate 	time.Time
	ExpirationDate 	time.Time
	Tags            *[]Tag
}

func (note Note) ActiveNotes() ([]*Note, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	currentTime := time.Now()
	query := fmt.Sprintf(
		`SELECT id, note, creator, create_date, expiration_date FROM %s WHERE expiration_date IS NULL OR expiration_date > $1`,
		NotesTable,
	)
	rows, err := database.Query(query, currentTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]*Note, 0)

	for rows.Next() {
		note := new(Note)
		var expirationDate sql.NullTime
		err := rows.Scan(&note.Id, &note.Note, &note.Creator, &note.CreateDate, &expirationDate)
		if err != nil {
			return nil, err
		}
		if expirationDate.Valid {
			note.ExpirationDate = expirationDate.Time
		}
		t := new(Tag)
		note.Tags, err = t.FindByNoteId(note.Id)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}

func (note Note) FindIn(ids *[]int64) ([]*Note, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		`SELECT id, note, creator, create_date, expiration_date FROM %s WHERE id = ANY($1)`,
		NotesTable,
	)
	rows, err := database.Query(query, pq.Array(*ids))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]*Note, 0)

	for rows.Next() {
		note := new(Note)
		var expirationDate sql.NullTime
		err := rows.Scan(&note.Id, &note.Note, &note.Creator, &note.CreateDate, &expirationDate)
		if err != nil {
			return nil, err
		}
		if expirationDate.Valid {
			note.ExpirationDate = expirationDate.Time
		}
		t := new(Tag)
		note.Tags, err = t.FindByNoteId(note.Id)
		if err != nil {
			return nil, err
		}
		notes = append(notes, note)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return notes, nil
}
