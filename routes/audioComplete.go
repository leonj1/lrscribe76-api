package routes

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/husobee/vestigo"
)

type audioCompleteRequest struct {
	TotalChunks int `json:"totalChunks"`
}

func AudioComplete(w http.ResponseWriter, r *http.Request) {
	recordingID := vestigo.Param(r, "recordingId")
	if recordingID == "" {
		http.Error(w, "recordingId is required", http.StatusBadRequest)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	var payload audioCompleteRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if payload.TotalChunks == 0 {
		w.Header().Set(ContentType, JSON)
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"totalChunks is required"}`))
		return
	}

	audioAPIURL := os.Getenv("AUDIO_API_URL")
	if audioAPIURL == "" {
		http.Error(w, "AUDIO_API_URL is not configured", http.StatusInternalServerError)
		return
	}

	req, err := http.NewRequest(
		http.MethodPost,
		fmt.Sprintf("%s/api/recordings/%s/complete", audioAPIURL, recordingID),
		bytes.NewReader(body),
	)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	req.Header.Set(ContentType, JSON)
	req.Header.Set("X-User-Id", ConvexUserID(r))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)
}
