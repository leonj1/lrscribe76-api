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
	sqlStatement := fmt.Sprintf(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM %s WHERE `id`=?",
		TranscriptionsTable,
	)

	found := &Transcription{}
	err := db.QueryRow(sqlStatement, id).Scan(
		&found.Id,
		&found.UserId,
		&found.Title,
		&found.AudioUrl,
		&found.Content,
		&found.Status,
		&found.CreatedAt,
	)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, err
		}
		return nil, err
	}

	return found, nil
}
