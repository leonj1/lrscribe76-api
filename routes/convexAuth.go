package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
)

type convexUserIDContextKey string

const convexUserIDKey convexUserIDContextKey = "convexUserID"

func WithConvexAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := r.Header.Get("X-User-Id")
		if userID == "" && r.Body != nil {
			body, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			var payload map[string]interface{}
			if len(body) > 0 {
				if err := json.Unmarshal(body, &payload); err != nil {
					http.Error(w, err.Error(), http.StatusBadRequest)
					return
				}

				if value, ok := payload["userId"].(string); ok {
					userID = value
				}
			}

			r.Body = io.NopCloser(bytes.NewReader(body))
		}

		if userID == "" {
			w.Header().Set(ContentType, JSON)
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"Not authenticated: no userId provided"}`))
			return
		}

		ctx := context.WithValue(r.Context(), convexUserIDKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func ConvexUserID(r *http.Request) string {
	userID, _ := r.Context().Value(convexUserIDKey).(string)
	return userID
}
