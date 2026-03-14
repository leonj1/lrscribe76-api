package routes

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"

	"github.com/husobee/vestigo"
)

type audioStatusResponse struct {
	RecordingID string `json:"recordingId"`
	Status      string `json:"status"`
	AudioURL    string `json:"audioUrl"`
	TotalChunks int    `json:"totalChunks"`
	CreatedAt   string `json:"createdAt"`
}

func AudioStatus(w http.ResponseWriter, r *http.Request) {
	recordingID := strings.TrimSpace(vestigo.Param(r, "recordingId"))
	if recordingID == "" {
		writeJSONError(w, http.StatusBadRequest, "recordingId is required")
		return
	}

	audioAPIURL := strings.TrimSpace(os.Getenv("AUDIO_API_URL"))
	if audioAPIURL == "" {
		writeJSONError(w, http.StatusInternalServerError, "AUDIO_API_URL is required")
		return
	}

	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		fmt.Sprintf("%s/api/recordings/%s", strings.TrimRight(audioAPIURL, "/"), url.PathEscape(recordingID)),
		nil,
	)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to build audio API request")
		return
	}

	userID := convexUserIDFromContext(r.Context())
	if userID != "" {
		req.Header.Set("X-User-Id", userID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "audio API request failed")
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		var errorPayload map[string]interface{}
		if err := json.NewDecoder(resp.Body).Decode(&errorPayload); err == nil && len(errorPayload) > 0 {
			writeJSON(w, resp.StatusCode, errorPayload)
			return
		}

		writeJSONError(w, resp.StatusCode, fmt.Sprintf("audio API returned status %d", resp.StatusCode))
		return
	}

	var payload audioStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadGateway, "failed to decode audio API response")
		return
	}

	writeJSON(w, http.StatusOK, payload)
}
