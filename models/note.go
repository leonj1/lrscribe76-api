package models

import (
	"database/sql"
	"errors"
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

func (note Note) AllNotes() ([]*Note, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT id, note, creator, create_date, expiration_date FROM %s`, NotesTable)
	rows, err := database.Query(query)
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

func (note Note) Save() (*Note, error){
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	var query string
	expirationDate := nullableTime(note.ExpirationDate)
	if note.Id == 0 {
		note.CreateDate = time.Now()
		query = fmt.Sprintf(
			`INSERT INTO %s (note, creator, create_date, expiration_date) VALUES ($1,$2,$3,$4) RETURNING id`,
			NotesTable,
		)
	} else {
		query = fmt.Sprintf(
			`UPDATE %s SET note=$1, creator=$2, create_date=$3, expiration_date=$4 WHERE id=$5`,
			NotesTable,
		)
		_, err = database.Exec(query, note.Note, note.Creator, note.CreateDate, expirationDate, note.Id)
		if err != nil {
			return nil, err
		}

		return &note, nil
	}

	err = database.QueryRow(query, note.Note, note.Creator, note.CreateDate, expirationDate).Scan(&note.Id)
	if err != nil {
		return nil, err
	}

	return &note, nil
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

func (note Note) DeleteNodeById(noteId int64) (error) {
	if noteId == 0 {
		return errors.New("NoteId is required")
	}

	database, err := requireDB()
	if err != nil {
		return err
	}

	tx, err := database.Begin()
	if err != nil {
		return err
	}

	if _, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE note_id=$1", TagsTable), noteId); err != nil {
		_ = tx.Rollback()
		return err
	}

	if _, err = tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE id=$1", NotesTable), noteId); err != nil {
		_ = tx.Rollback()
		return err
	}

	return tx.Commit()
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

func nullableTime(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}

	return t
}
