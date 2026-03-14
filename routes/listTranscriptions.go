package routes

import (
	"encoding/json"
	"net/http"
	"notes/models"
)

func ListTranscriptions(w http.ResponseWriter, r *http.Request) {
	userId, err := authenticateClerkJWT(r)
	if err != nil {
		w.Header().Set(ContentType, JSON)
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"message":"Unauthorized"}`))
		return
	}

	var transcription models.Transcription
	transcriptions, err := transcription.AllByUserId(userId)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js, err := json.Marshal(transcriptions)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.Write(js)
}
