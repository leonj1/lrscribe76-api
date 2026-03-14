package models

import (
	"errors"
	"fmt"
	"time"
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
	currentTime := time.Now()
	currentTime.Format(time.RFC3339)
	sql := fmt.Sprintf("SELECT `id`, `note`, `creator`, `create_date`, `expiration_date` from %s", NotesTable)
	rows, err := db.Query(sql, currentTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]*Note, 0)

	for rows.Next() {
		note := new(Note)
		err := rows.Scan(&note.Id, &note.Note, &note.Creator, &note.CreateDate, &note.ExpirationDate)
		if err != nil {
			return nil, err
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
	var sql string
	if note.Id == 0 {
		note.CreateDate = time.Now()
		note.CreateDate.Format(time.RFC3339)
		sql = fmt.Sprintf("INSERT INTO %s (note, creator, create_date, expiration_date) VALUES (?,?,?,?)", NotesTable)
	} else {
		sql = fmt.Sprintf("UPDATE %s SET note=?, creator=?, create_date=?, expiration_date=? WHERE id=%d", NotesTable, note.Id)
	}

	res, err := db.Exec(sql, note.Note, note.Creator, note.CreateDate, note.ExpirationDate)
	if err != nil {
		return nil, err
	}

	if note.Id == 0 {
		note.Id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	}

	return &note, nil
}

func (note Note) ActiveNotes() ([]*Note, error) {
	currentTime := time.Now()
	currentTime.Format(time.RFC3339)
	sql := fmt.Sprintf("SELECT `id`, `note`, `creator`, `create_date`, `expiration_date` from %s where `expiration_date` = '0000-00-00 00:00:00' or expiration_date > ?", NotesTable)
	rows, err := db.Query(sql, currentTime)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]*Note, 0)

	for rows.Next() {
		note := new(Note)
		err := rows.Scan(&note.Id, &note.Note, &note.Creator, &note.CreateDate, &note.ExpirationDate)
		if err != nil {
			return nil, err
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

	sql := fmt.Sprintf("DELETE FROM %s where note_id=?", TagsTable)
	_, err := db.Exec(sql, noteId)
	if err != nil {
		return err
	}

	sql = fmt.Sprintf("DELETE FROM %s where `id`=?", NotesTable)
	_, err = db.Exec(sql, noteId)
	if err != nil {
		return err
	}

	return nil
}

func (note Note) FindIn(ids *[]int64) ([]*Note, error) {
	currentTime := time.Now()
	currentTime.Format(time.RFC3339)
	sql := fmt.Sprintf("SELECT `id`, `note`, `creator`, `create_date`, `expiration_date` from %s where `id` in (?)", NotesTable)
	rows, err := db.Query(sql, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	notes := make([]*Note, 0)

	for rows.Next() {
		note := new(Note)
		err := rows.Scan(&note.Id, &note.Note, &note.Creator, &note.CreateDate, &note.ExpirationDate)
		if err != nil {
			return nil, err
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
