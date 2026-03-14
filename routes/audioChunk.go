package routes

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/husobee/vestigo"
)

const (
	maxAudioChunkBytes = 10 << 20
	audioContentType   = "application/octet-stream"
)

type errorResponse struct {
	Error string `json:"error"`
}

type convexUserIDRequest struct {
	UserID string `json:"userId"`
}

func ConvexAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID, err := convexUserIDFromRequest(r)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, err.Error())
			return
		}
		if userID == "" {
			writeJSONError(w, http.StatusUnauthorized, "Not authenticated: no userId provided")
			return
		}

		next(w, r)
	}
}

func AudioChunk(w http.ResponseWriter, r *http.Request) {
	recordingID := vestigo.Param(r, "recordingId")
	if recordingID == "" {
		writeJSONError(w, http.StatusBadRequest, "recordingId is required")
		return
	}

	chunkIndex := strings.TrimSpace(r.Header.Get("X-Chunk-Index"))
	if chunkIndex == "" {
		writeJSONError(w, http.StatusBadRequest, "X-Chunk-Index header is required")
		return
	}
	if _, err := strconv.Atoi(chunkIndex); err != nil {
		writeJSONError(w, http.StatusBadRequest, "X-Chunk-Index header must be an integer")
		return
	}

	audioAPIURL := strings.TrimRight(os.Getenv("AUDIO_API_URL"), "/")
	if audioAPIURL == "" {
		writeJSONError(w, http.StatusInternalServerError, "AUDIO_API_URL is not configured")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxAudioChunkBytes)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		if _, ok := err.(*http.MaxBytesError); ok {
			writeJSONError(w, http.StatusRequestEntityTooLarge, "audio chunk exceeds 10mb")
			return
		}
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	upstreamURL := audioAPIURL + "/api/recordings/" + recordingID + "/chunks"
	req, err := http.NewRequest(http.MethodPost, upstreamURL, bytes.NewReader(body))
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	req.Header.Set(ContentType, audioContentType)
	req.Header.Set("X-Chunk-Index", chunkIndex)
	if userID := strings.TrimSpace(r.Header.Get("X-User-Id")); userID != "" {
		req.Header.Set("X-User-Id", userID)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, err.Error())
		return
	}
	defer resp.Body.Close()

	copyHeaders(w.Header(), resp.Header)
	w.WriteHeader(resp.StatusCode)
	if _, err = io.Copy(w, resp.Body); err != nil {
		http.Error(w, err.Error(), http.StatusBadGateway)
	}
}

func convexUserIDFromRequest(r *http.Request) (string, error) {
	if userID := strings.TrimSpace(r.Header.Get("X-User-Id")); userID != "" {
		return userID, nil
	}

	if r.Body == nil {
		return "", nil
	}

	contentType := r.Header.Get(ContentType)
	if !strings.Contains(strings.ToLower(contentType), JSON) {
		return "", nil
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	r.Body = io.NopCloser(bytes.NewReader(body))

	if len(body) == 0 {
		return "", nil
	}

	var payload convexUserIDRequest
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", nil
	}

	return strings.TrimSpace(payload.UserID), nil
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func writeJSONError(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set(ContentType, JSON)
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(errorResponse{Error: message})
}
