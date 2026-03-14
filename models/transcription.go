package models

import (
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

func (transcription Transcription) Save() (*Transcription, error) {
	if transcription.Id == 0 {
		transcription.Status = "pending"
		transcription.CreatedAt = time.Now().UTC()
		sql := fmt.Sprintf(
			"INSERT INTO %s (user_id, title, audio_url, content, status, created_at) VALUES (?,?,?,?,?,?)",
			TranscriptionsTable,
		)

		res, err := db.Exec(
			sql,
			transcription.UserId,
			transcription.Title,
			transcription.AudioUrl,
			transcription.Content,
			transcription.Status,
			transcription.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		transcription.Id, err = res.LastInsertId()
		if err != nil {
			return nil, err
		}
	} else {
		sql := fmt.Sprintf(
			"UPDATE %s SET user_id=?, title=?, audio_url=?, content=?, status=? WHERE id=%d",
			TranscriptionsTable,
			transcription.Id,
		)

		_, err := db.Exec(
			sql,
			transcription.UserId,
			transcription.Title,
			transcription.AudioUrl,
			transcription.Content,
			transcription.Status,
		)
		if err != nil {
			return nil, err
		}
	}

	return &transcription, nil
}
