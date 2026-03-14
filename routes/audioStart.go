package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

var uuidPattern = regexp.MustCompile("(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$")

type audioStartRequest struct {
	SessionID string                 `json:"sessionId"`
	UserID    string                 `json:"userId"`
	MimeType  string                 `json:"mimeType"`
	ID        string                 `json:"id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

func AudioStart(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, "sessionId, userId, and mimeType are required")
		return
	}

	var req audioStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	req.SessionID = strings.TrimSpace(req.SessionID)
	req.UserID = strings.TrimSpace(req.UserID)
	req.MimeType = strings.TrimSpace(req.MimeType)
	req.ID = strings.TrimSpace(req.ID)

	if req.UserID == "" {
		req.UserID = convexUserIDFromContext(r.Context())
	}

	if req.SessionID == "" || req.UserID == "" || req.MimeType == "" {
		writeJSONError(w, http.StatusBadRequest, "sessionId, userId, and mimeType are required")
		return
	}

	if req.ID != "" && !uuidPattern.MatchString(req.ID) {
		writeJSONError(w, http.StatusBadRequest, "id must be a valid UUID")
		return
	}

	audioAPIURL := strings.TrimRight(strings.TrimSpace(os.Getenv("AUDIO_API_URL")), "/")
	if audioAPIURL == "" {
		writeJSONError(w, http.StatusInternalServerError, "AUDIO_API_URL is not configured")
		return
	}

	payload, err := json.Marshal(req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	upstreamReq, err := http.NewRequest(http.MethodPost, audioAPIURL+"/api/recordings", bytes.NewReader(payload))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	upstreamReq.Header.Set(ContentType, JSON)
	upstreamReq.Header.Set("X-User-Id", req.UserID)

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(upstreamReq)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}

	contentType := resp.Header.Get(ContentType)
	if contentType == "" {
		contentType = JSON
	}
	w.Header().Set(ContentType, contentType)
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(body)
}
