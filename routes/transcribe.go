package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
)

const (
	defaultTranscriptionMIMEType = "audio/webm"
)

var (
	requestyTranscriptionURL        = requestyBaseURL
	requestyTranscriptionHTTPClient = http.DefaultClient
)

type transcribeRequest struct {
	AudioData string `json:"audioData"`
	MIMEType  string `json:"mimeType"`
}

type transcribeResponse struct {
	Transcription string `json:"transcription"`
}

type requestyMultimodalChatRequest struct {
	Model    string                          `json:"model"`
	Messages []requestyMultimodalChatMessage `json:"messages"`
}

type requestyMultimodalChatMessage struct {
	Role    string                      `json:"role"`
	Content []requestyMultimodalContent `json:"content"`
}

type requestyMultimodalContent struct {
	Type     string                   `json:"type"`
	Text     string                   `json:"text,omitempty"`
	ImageURL *requestyContentImageURL `json:"image_url,omitempty"`
}

type requestyContentImageURL struct {
	URL string `json:"url"`
}

type requestyMultimodalChatResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func Transcribe(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticateClerkJWT(r); err != nil {
		writeUnauthorized(w)
		return
	}

	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, "Audio data is required")
		return
	}

	var payload transcribeRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	audioData := strings.TrimSpace(payload.AudioData)
	if audioData == "" {
		writeJSONError(w, http.StatusBadRequest, "Audio data is required")
		return
	}

	mimeType := strings.TrimSpace(payload.MIMEType)
	if mimeType == "" {
		mimeType = defaultTranscriptionMIMEType
	}

	transcription, err := callRequestyTranscription(r, audioData, mimeType)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, transcribeResponse{Transcription: transcription})
}

func callRequestyTranscription(r *http.Request, audioData, mimeType string) (string, error) {
	return callRequestyTranscriptionWithContext(r.Context(), audioData, mimeType)
}

func callRequestyTranscriptionWithContext(ctx context.Context, audioData, mimeType string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("REQUESTY_API_KEY"))
	if apiKey == "" {
		return "", errors.New("REQUESTY_API_KEY is required")
	}

	model := strings.TrimSpace(os.Getenv("GEMINI_TRANSCRIPTION_MODEL"))
	if model == "" {
		model = defaultGeminiTranscriptionModel
	}

	body := requestyMultimodalChatRequest{
		Model: model,
		Messages: []requestyMultimodalChatMessage{
			{
				Role: "user",
				Content: []requestyMultimodalContent{
					{
						Type: "text",
						Text: "Transcribe this audio and return only the transcription text.",
					},
					{
						Type: "image_url",
						ImageURL: &requestyContentImageURL{
							URL: fmt.Sprintf("data:%s;base64,%s", mimeType, audioData),
						},
					},
				},
			},
		},
	}

	requestBody, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to encode transcription request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestyTranscriptionURL, bytes.NewReader(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to build transcription request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set(ContentType, JSON)

	resp, err := requestyTranscriptionHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesty request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed requestyMultimodalChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode transcription response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("requesty error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("requesty error: status %d", resp.StatusCode)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("requesty returned no choices")
	}

	transcription := strings.TrimSpace(extractMessageContentText(parsed.Choices[0].Message.Content))
	if transcription == "" {
		return "", errors.New("requesty returned empty content")
	}

	return transcription, nil
}

func extractMessageContentText(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			part, ok := item.(map[string]interface{})
			if !ok {
				continue
			}

			text, _ := part["text"].(string)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.Join(parts, "\n")
	default:
		return ""
	}
}
