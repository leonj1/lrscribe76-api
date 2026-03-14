package models

import (
	"fmt"
	"github.com/kataras/go-errors"
	"time"
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
	sql := fmt.Sprintf("SELECT * from %s", TagsTable)
	rows, err := db.Query(sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	bks := make([]*Tag, 0)

	for rows.Next() {
		tag := new(Tag)
		err := rows.Scan(&tag.Id, &tag.Key, &tag.Value)
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
	var sql string
	if tag.Id == 0 {
		tag.CreateDate = time.Now()
		tag.CreateDate.Format(time.RFC3339)
		sql = fmt.Sprintf("INSERT INTO %s (`key`, `value`, `note_id`, `creator`, `create_date`) VALUES (?,?,?,?,?)", TagsTable)
	} else {
		sql = fmt.Sprintf("UPDATE %s SET `key`=?, `value`=?, `note_id`, `creator`=?, `create_date`=? WHERE `id`=%d", TagsTable, tag.Id)
	}

	res, err := db.Exec(sql, tag.Key, tag.Value, tag.NoteId, tag.Creator, tag.CreateDate)
	if err != nil {
		return nil, err
	}

	if tag.Id == 0 {
		tag.Id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	}

	return &tag, nil
}

func (tag Tag) FindByKeyAndValueAndNoteId(key string, value string, noteId int64) (*[]Tag, error) {
	if key == "" || value == "" || noteId < 1 {
		return nil, errors.New("Please provide key, value, and noteId")
	}

	sql := fmt.Sprintf("select `id`, `note_id`, `key`, `value`, `creator`, `create_date` from %s where `key`=? and `value`=? and `note_id`=?", TagsTable)

	rows, err := db.Query(sql, key, value, noteId)
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

	return &tags, nil
}

func (tag Tag) FindByKeyAndValue(key string, value string) ([]*Tag, error) {
	if key == "" || value == "" {
		return nil, errors.New("Please provide key, and value")
	}

	sql := fmt.Sprintf("select `id`, `note_id`, `key`, `value`, `creator`, `create_date` from %s where `key`=? and `value`=?", TagsTable)

	rows, err := db.Query(sql, key, value)
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

	return tags, nil
}

func (tag Tag) FindByNoteId(noteId int64) (*[]Tag, error) {
	if noteId == 0 {
		return nil, errors.New("NoteId needs to be provided")
	}

	sql := fmt.Sprintf("select `id`, `note_id`, `key`, `value`, `creator`, `create_date` from %s where `note_id`=?", TagsTable)

	rows, err := db.Query(sql, noteId)
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

	return &tags, nil
}
