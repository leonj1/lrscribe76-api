package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
)

type contextKey string

const convexUserIDContextKey contextKey = "convexUserID"

func ConvexAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := extractConvexUserID(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if strings.TrimSpace(userID) == "" {
			writeJSONError(w, http.StatusUnauthorized, "Not authenticated: no userId provided")
			return
		}

		ctx := context.WithValue(r.Context(), convexUserIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func extractConvexUserID(r *http.Request) (string, error) {
	trimmedHeader := strings.TrimSpace(r.Header.Get("X-User-Id"))
	if trimmedHeader != "" {
		return trimmedHeader, nil
	}

	if r.Body == nil {
		return "", nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	r.Body = io.NopCloser(bytes.NewBuffer(body))

	if len(bytes.TrimSpace(body)) == 0 {
		return "", nil
	}

	var payload struct {
		UserID string `json:"userId"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}

	return strings.TrimSpace(payload.UserID), nil
}

func convexUserIDFromContext(ctx context.Context) string {
	userID, _ := ctx.Value(convexUserIDContextKey).(string)
	return userID
}

func writeJSONError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

func writeJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set(ContentType, JSON)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}
