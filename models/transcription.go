package models

import (
	"database/sql"
	"fmt"
	"time"
)

const TranscriptionsTable = "transcriptions"

type Transcription struct {
	Id        int64     `json:"id"`
	UserId    string    `json:"userId"`
	Title     string    `json:"title"`
	AudioUrl  string    `json:"audioUrl"`
	Content   string    `json:"content"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"createdAt"`
}

func (transcription Transcription) FindById(id int64) (*Transcription, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	sqlStatement := fmt.Sprintf(
		"SELECT id, user_id, title, audio_url, content, status, created_at FROM %s WHERE id=$1",
		TranscriptionsTable,
	)

	found := &Transcription{}
	var title sql.NullString
	var audioURL sql.NullString
	var content sql.NullString
	var status sql.NullString
	err = database.QueryRow(sqlStatement, id).Scan(
		&found.Id,
		&found.UserId,
		&title,
		&audioURL,
		&content,
		&status,
		&found.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}

	found.Title = title.String
	found.AudioUrl = audioURL.String
	found.Content = content.String
	found.Status = status.String

	return found, nil
}

func (transcription Transcription) Save() (*Transcription, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	if transcription.Id == 0 {
		transcription.Status = "pending"
		transcription.CreatedAt = time.Now().UTC()
		query := fmt.Sprintf(
			"INSERT INTO %s (user_id, title, audio_url, content, status, created_at) VALUES ($1,$2,$3,$4,$5,$6) RETURNING id",
			TranscriptionsTable,
		)

		err = database.QueryRow(
			query,
			transcription.UserId,
			transcription.Title,
			transcription.AudioUrl,
			transcription.Content,
			transcription.Status,
			transcription.CreatedAt,
		).Scan(&transcription.Id)
		if err != nil {
			return nil, err
		}
	} else {
		query := fmt.Sprintf(
			"UPDATE %s SET user_id=$1, title=$2, audio_url=$3, content=$4, status=$5 WHERE id=$6",
			TranscriptionsTable,
		)

		_, err = database.Exec(
			query,
			transcription.UserId,
			transcription.Title,
			transcription.AudioUrl,
			transcription.Content,
			transcription.Status,
			transcription.Id,
		)
		if err != nil {
			return nil, err
		}
	}

	return &transcription, nil
}

func (transcription Transcription) AllByUserId(userId string) ([]*Transcription, error) {
	database, err := requireDB()
	if err != nil {
		return nil, err
	}

	query := fmt.Sprintf(
		"SELECT id, user_id, title, audio_url, content, status, created_at FROM %s WHERE user_id=$1 ORDER BY created_at DESC",
		TranscriptionsTable,
	)

	rows, err := database.Query(query, userId)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transcriptions := make([]*Transcription, 0)
	for rows.Next() {
		t := new(Transcription)
		var title sql.NullString
		var audioUrl sql.NullString
		var content sql.NullString
		var status sql.NullString

		err := rows.Scan(&t.Id, &t.UserId, &title, &audioUrl, &content, &status, &t.CreatedAt)
		if err != nil {
			return nil, err
		}

		t.Title = title.String
		t.AudioUrl = audioUrl.String
		t.Content = content.String
		t.Status = status.String
		transcriptions = append(transcriptions, t)
	}

	if err = rows.Err(); err != nil {
		return nil, err
	}

	return transcriptions, nil
}
