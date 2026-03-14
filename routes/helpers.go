package routes

import (
	"encoding/json"
	"net/http"
	"notes/models"
)

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set(ContentType, JSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeUnauthorized(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, models.MessageResponse{Message: "Unauthorized"})
}

func UnauthorizedJSON(w http.ResponseWriter, _ *http.Request) {
	writeUnauthorized(w)
}
