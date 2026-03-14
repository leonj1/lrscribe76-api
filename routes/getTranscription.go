package routes

import (
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/husobee/vestigo"
	"notes/models"
)

type errorMessage struct {
	Message string `json:"message"`
}

func UnauthorizedJSON(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusUnauthorized, errorMessage{Message: "Unauthorized"})
}

func GetTranscription(w http.ResponseWriter, r *http.Request) {
	claims, ok := clerk.SessionClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.Subject == "" {
		UnauthorizedJSON(w, r)
		return
	}

	id := vestigo.Param(r, "id")
	if id == "" {
		http.Error(w, "Invalid transcription id", http.StatusBadRequest)
		return
	}

	transcriptionId, err := strconv.ParseInt(id, 10, 64)
	if err != nil {
		http.Error(w, "Invalid transcription id", http.StatusBadRequest)
		return
	}

	var transcription models.Transcription
	found, err := transcription.FindById(transcriptionId)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusNotFound, errorMessage{Message: "Transcription not found"})
			return
		}

		log.Printf("Error fetching transcription %d: %v", transcriptionId, err)
		writeJSON(w, http.StatusInternalServerError, errorMessage{Message: "Internal server error"})
		return
	}

	if found.UserId != claims.Subject {
		writeJSON(w, http.StatusForbidden, errorMessage{Message: "Forbidden"})
		return
	}

	writeJSON(w, http.StatusOK, found)
}

func writeJSON(w http.ResponseWriter, statusCode int, payload interface{}) {
	js, err := json.Marshal(payload)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(statusCode)
	w.Write(js)
}
