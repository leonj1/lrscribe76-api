package routes

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/husobee/vestigo"
)

const defaultConvexURL = "https://robust-labrador-493.convex.cloud"

type audioTriggerInterimRequest struct {
	TotalChunks int    `json:"totalChunks"`
	SessionID   string `json:"sessionId"`
	MIMEType    string `json:"mimeType"`
}

type audioTriggerInterimResponse struct {
	NewRecordingID string `json:"newRecordingId"`
}

type audioRecordingCreateRequest struct {
	ID        string                 `json:"id"`
	SessionID string                 `json:"sessionId"`
	UserID    string                 `json:"userId"`
	MIMEType  string                 `json:"mimeType"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type audioCompleteResponse struct {
	Status   string `json:"status"`
	AudioURL string `json:"audioUrl"`
}

type convexFunctionRequest struct {
	Path   string      `json:"path"`
	Args   interface{} `json:"args"`
	Format string      `json:"format,omitempty"`
}

type convexSession struct {
	ID            string `json:"_id"`
	Transcription string `json:"transcription"`
}

func AudioTriggerInterim(w http.ResponseWriter, r *http.Request) {
	recordingID := vestigo.Param(r, "recordingId")
	if recordingID == "" {
		writeJSONError(w, http.StatusBadRequest, "recordingId is required")
		return
	}

	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, "totalChunks and sessionId are required")
		return
	}

	var payload audioTriggerInterimRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	payload.SessionID = strings.TrimSpace(payload.SessionID)
	payload.MIMEType = strings.TrimSpace(payload.MIMEType)
	if payload.TotalChunks == 0 || payload.SessionID == "" {
		writeJSONError(w, http.StatusBadRequest, "totalChunks and sessionId are required")
		return
	}
	if payload.MIMEType == "" {
		payload.MIMEType = defaultTranscriptionMIMEType
	}

	audioAPIURL := strings.TrimSpace(os.Getenv("AUDIO_API_URL"))
	if audioAPIURL == "" {
		http.Error(w, "AUDIO_API_URL is not configured", http.StatusInternalServerError)
		return
	}

	completeBody, err := json.Marshal(map[string]int{"totalChunks": payload.TotalChunks})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	completed, routeErr := completeAudioRecording(r.Context(), audioAPIURL, recordingID, ConvexUserID(r), completeBody)
	if routeErr != nil {
		writeJSONError(w, routeErr.status, routeErr.message)
		return
	}

	newRecordingID, err := newUUID()
	if err != nil {
		http.Error(w, "failed to generate recording ID", http.StatusInternalServerError)
		return
	}

	startPayload := audioRecordingCreateRequest{
		ID:        newRecordingID,
		SessionID: payload.SessionID,
		UserID:    ConvexUserID(r),
		MIMEType:  payload.MIMEType,
	}
	if routeErr := startAudioRecording(r.Context(), audioAPIURL, ConvexUserID(r), startPayload); routeErr != nil {
		writeJSONError(w, routeErr.status, routeErr.message)
		return
	}

	writeJSON(w, http.StatusOK, audioTriggerInterimResponse{NewRecordingID: newRecordingID})

	go transcribeAndAppendSegment(payload.SessionID, ConvexUserID(r), payload.MIMEType, completed.AudioURL)
}

type routeError struct {
	status  int
	message string
}

func completeAudioRecording(ctx context.Context, audioAPIURL, recordingID, userID string, requestBody []byte) (*audioCompleteResponse, *routeError) {
	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/recordings/%s/complete", audioAPIURL, recordingID),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return nil, &routeError{status: http.StatusInternalServerError, message: err.Error()}
	}

	req.Header.Set(ContentType, JSON)
	req.Header.Set("X-User-Id", userID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, &routeError{status: http.StatusBadGateway, message: err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, &routeError{status: http.StatusBadGateway, message: err.Error()}
	}

	if resp.StatusCode >= 400 {
		return nil, &routeError{status: resp.StatusCode, message: extractErrorMessage(body, "failed to complete recording")}
	}

	var completed audioCompleteResponse
	if err := json.Unmarshal(body, &completed); err != nil {
		return nil, &routeError{status: http.StatusBadGateway, message: "invalid response from audio API complete endpoint"}
	}

	return &completed, nil
}

func startAudioRecording(ctx context.Context, audioAPIURL, userID string, payload audioRecordingCreateRequest) *routeError {
	requestBody, err := json.Marshal(payload)
	if err != nil {
		return &routeError{status: http.StatusInternalServerError, message: err.Error()}
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodPost,
		fmt.Sprintf("%s/api/recordings", audioAPIURL),
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return &routeError{status: http.StatusInternalServerError, message: err.Error()}
	}

	req.Header.Set(ContentType, JSON)
	req.Header.Set("X-User-Id", userID)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &routeError{status: http.StatusBadGateway, message: err.Error()}
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return &routeError{status: http.StatusBadGateway, message: err.Error()}
	}

	if resp.StatusCode >= 400 {
		return &routeError{status: resp.StatusCode, message: extractErrorMessage(body, "failed to start new recording")}
	}

	return nil
}

func transcribeAndAppendSegment(sessionID, userID, mimeType, audioURL string) {
	if strings.TrimSpace(audioURL) == "" {
		log.Printf("trigger interim: missing audio URL for session %s", sessionID)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	audioData, err := fetchAudioAsBase64(ctx, audioURL)
	if err != nil {
		log.Printf("trigger interim: failed to fetch audio for session %s: %v", sessionID, err)
		return
	}

	transcription, err := callRequestyTranscriptionWithContext(ctx, audioData, mimeType)
	if err != nil {
		log.Printf("trigger interim: failed to transcribe audio for session %s: %v", sessionID, err)
		return
	}

	if err := appendTranscriptionToConvex(ctx, sessionID, userID, transcription); err != nil {
		log.Printf("trigger interim: failed to append transcription for session %s: %v", sessionID, err)
	}
}

func appendTranscriptionToConvex(ctx context.Context, sessionID, userID, transcription string) error {
	baseURL := strings.TrimSpace(os.Getenv("VITE_CONVEX_URL"))
	if baseURL == "" {
		baseURL = defaultConvexURL
	}
	baseURL = strings.TrimRight(baseURL, "/")

	session, err := convexGetSession(ctx, baseURL, sessionID)
	if err != nil {
		return err
	}

	existing := strings.TrimSpace(session.Transcription)
	addition := strings.TrimSpace(transcription)
	if addition == "" {
		return nil
	}

	combined := addition
	if existing != "" {
		combined = existing + "\n\n" + addition
	}

	return convexUpdateSessionTranscription(ctx, baseURL, session.ID, userID, combined)
}

func convexGetSession(ctx context.Context, baseURL, sessionID string) (*convexSession, error) {
	var session convexSession
	if err := callConvexFunction(ctx, baseURL, "/api/query", "recordingSessions:get", map[string]string{"id": sessionID}, &session); err != nil {
		return nil, err
	}

	if strings.TrimSpace(session.ID) == "" {
		session.ID = sessionID
	}

	return &session, nil
}

func convexUpdateSessionTranscription(ctx context.Context, baseURL, sessionID, userID, transcription string) error {
	var response interface{}
	return callConvexFunction(ctx, baseURL, "/api/mutation", "recordingSessions:update", map[string]string{
		"id":            sessionID,
		"userId":        userID,
		"transcription": transcription,
	}, &response)
}

func callConvexFunction(ctx context.Context, baseURL, endpoint, path string, args interface{}, out interface{}) error {
	requestBody, err := json.Marshal(convexFunctionRequest{
		Path:   path,
		Args:   args,
		Format: "json",
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+endpoint, bytes.NewReader(requestBody))
	if err != nil {
		return err
	}

	req.Header.Set(ContentType, JSON)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	if resp.StatusCode >= 400 {
		return fmt.Errorf("convex request failed with status %d: %s", resp.StatusCode, extractErrorMessage(body, "request failed"))
	}

	var wrapped struct {
		Status string          `json:"status"`
		Value  json.RawMessage `json:"value"`
		Error  string          `json:"errorMessage"`
	}
	if err := json.Unmarshal(body, &wrapped); err != nil {
		return err
	}

	if wrapped.Error != "" {
		return fmt.Errorf("convex error: %s", wrapped.Error)
	}

	if len(wrapped.Value) == 0 || out == nil {
		return nil
	}

	return json.Unmarshal(wrapped.Value, out)
}

func extractErrorMessage(body []byte, fallback string) string {
	var payload map[string]interface{}
	if err := json.Unmarshal(body, &payload); err != nil {
		return fallback
	}

	for _, key := range []string{"error", "message"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return value
		}
	}

	return fallback
}

func newUUID() (string, error) {
	var data [16]byte
	if _, err := rand.Read(data[:]); err != nil {
		return "", err
	}

	data[6] = (data[6] & 0x0f) | 0x40
	data[8] = (data[8] & 0x3f) | 0x80

	return fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		data[0], data[1], data[2], data[3],
		data[4], data[5],
		data[6], data[7],
		data[8], data[9],
		data[10], data[11], data[12], data[13], data[14], data[15],
	), nil
}
