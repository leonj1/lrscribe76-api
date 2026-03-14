package routes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/husobee/vestigo"
)

const convexUserIDContextKey = "convexUserID"

type convexAuthBody struct {
	UserID string `json:"userId"`
}

type audioStatusResponse struct {
	RecordingID string `json:"recordingId"`
	Status      string `json:"status"`
	AudioURL    string `json:"audioUrl"`
	TotalChunks int    `json:"totalChunks"`
	CreatedAt   string `json:"createdAt"`
}

func ConvexAuthMiddleware(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := getConvexUserID(r)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, map[string]string{"message": "Unauthorized"})
			return
		}

		ctx := context.WithValue(r.Context(), convexUserIDContextKey, userID)
		next(w, r.WithContext(ctx))
	}
}

func AudioStatus(w http.ResponseWriter, r *http.Request) {
	recordingID := strings.TrimSpace(vestigo.Param(r, "recordingId"))
	if recordingID == "" {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": "recordingId is required"})
		return
	}

	audioAPIURL := strings.TrimSpace(os.Getenv("AUDIO_API_URL"))
	if audioAPIURL == "" {
		writeJSONError(w, http.StatusInternalServerError, map[string]string{"error": "AUDIO_API_URL is required"})
		return
	}

	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		fmt.Sprintf("%s/api/recordings/%s", strings.TrimRight(audioAPIURL, "/"), url.PathEscape(recordingID)),
		nil,
	)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, map[string]string{"error": "failed to build audio API request"})
		return
	}

	if userID, ok := r.Context().Value(convexUserIDContextKey).(string); ok && strings.TrimSpace(userID) != "" {
		req.Header.Set("X-User-Id", userID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, map[string]string{"error": "audio API request failed"})
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var errorPayload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorPayload); err == nil && len(errorPayload) > 0 {
			writeJSON(w, resp.StatusCode, errorPayload)
			return
		}

		writeJSONError(w, resp.StatusCode, map[string]string{"error": fmt.Sprintf("audio API returned status %d", resp.StatusCode)})
		return
	}

	var payload audioStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadGateway, map[string]string{"error": "failed to decode audio API response"})
		return
	}

	writeJSON(w, http.StatusOK, payload)
}

func getConvexUserID(r *http.Request) (string, error) {
	if userID := strings.TrimSpace(r.Header.Get("X-User-Id")); userID != "" {
		return userID, nil
	}

	if r.Body == nil {
		return "", errors.New("missing convex user id")
	}

	var body convexAuthBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		if errors.Is(err, io.EOF) {
			return "", errors.New("missing convex user id")
		}
		return "", err
	}

	userID := strings.TrimSpace(body.UserID)
	if userID == "" {
		return "", errors.New("missing convex user id")
	}

	return userID, nil
}
