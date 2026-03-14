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

func (transcription Transcription) AllByUserId(userId string) ([]*Transcription, error) {
	sql := fmt.Sprintf(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM %s WHERE `user_id`=? ORDER BY `created_at` DESC",
		TranscriptionsTable,
	)

	rows, err := db.Query(sql, userId)
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
