package routes

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"notes/models"
	"strings"
)

type transcriptionClaims struct {
	Sub string `json:"sub"`
}

type validationError struct {
	Message string `json:"message"`
	Field   string `json:"field"`
}

func CreateTranscription(w http.ResponseWriter, r *http.Request) {
	var transcription models.Transcription
	if r.Body == nil {
		writeValidationError(w, "userId")
		return
	}

	err := json.NewDecoder(r.Body).Decode(&transcription)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	field := requiredTranscriptionField(transcription)
	if field != "" {
		writeValidationError(w, field)
		return
	}

	authenticatedUserId, err := clerkUserIDFromRequest(r)
	if err != nil || authenticatedUserId == "" {
		writeUnauthorized(w)
		return
	}

	if authenticatedUserId != transcription.UserId {
		writeUnauthorized(w)
		return
	}

	saved, err := transcription.Save()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	js, err := json.Marshal(saved)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(http.StatusCreated)
	w.Write(js)
}

func requiredTranscriptionField(transcription models.Transcription) string {
	if strings.TrimSpace(transcription.UserId) == "" {
		return "userId"
	}
	if strings.TrimSpace(transcription.Title) == "" {
		return "title"
	}
	if strings.TrimSpace(transcription.AudioUrl) == "" {
		return "audioUrl"
	}

	return ""
}

func writeValidationError(w http.ResponseWriter, field string) {
	js, err := json.Marshal(validationError{
		Message: "Required",
		Field:   field,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(http.StatusBadRequest)
	w.Write(js)
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set(ContentType, JSON)
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"message":"Unauthorized"}`))
}

func clerkUserIDFromRequest(r *http.Request) (string, error) {
	authHeader := r.Header.Get("Authorization")
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return "", http.ErrNoCookie
	}

	tokenParts := strings.Split(strings.TrimPrefix(authHeader, "Bearer "), ".")
	if len(tokenParts) < 2 {
		return "", http.ErrNoCookie
	}

	payload, err := base64.RawURLEncoding.DecodeString(tokenParts[1])
	if err != nil {
		return "", err
	}

	var claims transcriptionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return "", err
	}

	return claims.Sub, nil
}
