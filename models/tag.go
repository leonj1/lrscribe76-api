package models

import (
	"fmt"
	"time"

	"github.com/kataras/go-errors"
)

const TagsTable = "tags"

type Tag struct {
	Id 		int64		`json:"id,string,omitempty"`
	NoteId		int64		`json:"note_id,string,omitempty"`
	Creator		string		`json:"creator,omitempty"`
	Key 		string		`json:"key,omitempty"`
	Value 		string		`json:"value,omitempty"`
	CreateDate 	time.Time
}

func (tag Tag) AllTags() ([]*Tag, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT id, "key", value, note_id, creator, create_date FROM %s`, TagsTable)
	rows, err := database.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bks := make([]*Tag, 0)

	for rows.Next() {
		tag := new(Tag)
		err := rows.Scan(&tag.Id, &tag.Key, &tag.Value, &tag.NoteId, &tag.Creator, &tag.CreateDate)
		if err != nil {
			return nil, err
		}
		bks = append(bks, tag)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return bks, nil
}

func (tag Tag) Save() (*Tag, error){
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	var query string
	if tag.Id == 0 {
		tag.CreateDate = time.Now()
		query = fmt.Sprintf(
			`INSERT INTO %s ("key", value, note_id, creator, create_date) VALUES ($1,$2,$3,$4,$5) RETURNING id`,
			TagsTable,
		)
	} else {
		query = fmt.Sprintf(
			`UPDATE %s SET "key"=$1, value=$2, note_id=$3, creator=$4, create_date=$5 WHERE id=$6`,
			TagsTable,
		)
		_, err = database.Exec(query, tag.Key, tag.Value, tag.NoteId, tag.Creator, tag.CreateDate, tag.Id)
		if err != nil {
			return nil, err
		}

		return &tag, nil
	}

	err = database.QueryRow(query, tag.Key, tag.Value, tag.NoteId, tag.Creator, tag.CreateDate).Scan(&tag.Id)
	if err != nil {
		return nil, err
	}

	return &tag, nil
}

func (tag Tag) FindByKeyAndValueAndNoteId(key string, value string, noteId int64) (*[]Tag, error) {
	if key == "" || value == "" || noteId < 1 {
		return nil, errors.New("Please provide key, value, and noteId")
	}

	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT id, note_id, "key", value, creator, create_date FROM %s WHERE "key"=$1 AND value=$2 AND note_id=$3`, TagsTable)
	rows, err := database.Query(query, key, value, noteId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		t := new(Tag)
		err := rows.Scan(&t.Id, &t.NoteId, &t.Key, &t.Value, &t.Creator, &t.CreateDate)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &tags, nil
}

func (tag Tag) FindByKeyAndValue(key string, value string) ([]*Tag, error) {
	if key == "" || value == "" {
		return nil, errors.New("Please provide key, and value")
	}

	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT id, note_id, "key", value, creator, create_date FROM %s WHERE "key"=$1 AND value=$2`, TagsTable)
	rows, err := database.Query(query, key, value)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	tags := []*Tag{}
	for rows.Next() {
		t := &Tag{}
		err := rows.Scan(&t.Id, &t.NoteId, &t.Key, &t.Value, &t.Creator, &t.CreateDate)
		if err != nil {
			return nil, err
		}
		tags = append(tags, t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return tags, nil
}

func (tag Tag) FindByNoteId(noteId int64) (*[]Tag, error) {
	if noteId == 0 {
		return nil, errors.New("NoteId needs to be provided")
	}

	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(`SELECT id, note_id, "key", value, creator, create_date FROM %s WHERE note_id=$1`, TagsTable)
	rows, err := database.Query(query, noteId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tags []Tag
	for rows.Next() {
		t := new(Tag)
		err := rows.Scan(&t.Id, &t.NoteId, &t.Key, &t.Value, &t.Creator, &t.CreateDate)
		if err != nil {
			return nil, err
		}
		tags = append(tags, *t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return &tags, nil
}
