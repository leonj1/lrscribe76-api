package routes

import (
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"github.com/husobee/vestigo"
	"net/http"
	"notes/models"
	"strconv"
	"strings"
)

type transcriptionTokenClaims struct {
	Sub string `json:"sub"`
}

type errorMessage struct {
	Message string `json:"message"`
}

func GetTranscription(w http.ResponseWriter, r *http.Request) {
	userId, err := clerkUserIDFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorMessage{Message: "Unauthorized"})
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

		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if found.UserId != userId {
		writeJSON(w, http.StatusUnauthorized, errorMessage{Message: "Unauthorized"})
		return
	}

	writeJSON(w, http.StatusOK, found)
}

func clerkUserIDFromRequest(r *http.Request) (string, error) {
	authorizationHeader := r.Header.Get("Authorization")
	if authorizationHeader == "" {
		return "", errors.New("missing authorization header")
	}

	headerParts := strings.SplitN(authorizationHeader, " ", 2)
	if len(headerParts) != 2 || !strings.EqualFold(headerParts[0], "Bearer") {
		return "", errors.New("invalid authorization header")
	}

	tokenParts := strings.Split(headerParts[1], ".")
	if len(tokenParts) < 2 {
		return "", errors.New("invalid jwt")
	}

	payload, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if err != nil {
		return "", err
	}

	var claims transcriptionTokenClaims
	if err = json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	if claims.Sub == "" {
		return "", errors.New("missing sub claim")
	}

	return claims.Sub, nil
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
