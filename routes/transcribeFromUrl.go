package routes

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	neturl "net/url"
	"os"
	"path"
	"strings"
	"time"
)

const (
	requestyChatCompletionsURL         = "https://router.requesty.ai/v1/chat/completions"
	defaultGeminiTranscriptionModel    = "google/gemini-3.1-pro-preview"
	defaultAudioAPIBaseURL             = "https://lrscribe-audio-api-production.up.railway.app"
	transcriptionInstructionPromptText = "Transcribe the provided audio verbatim. Return only the transcription text with no speaker labels, formatting, or extra commentary."
	audioAPIRequestTimeout             = 10 * time.Second
	audioDownloadTimeout               = 2 * time.Minute
	requestyRequestTimeout             = 3 * time.Minute
	maxAudioAPIResponseBytes           = 10 << 20
	maxAudioBytes                      = 50 << 20
	maxRequestyErrorBodyBytes          = 1 << 20
	maxRedirectHops                    = 10
)

var (
	requestyAudioTranscriptionURL = requestyChatCompletionsURL
	requestyAudioHTTPClientFactory = func() *http.Client {
		return &http.Client{Timeout: requestyRequestTimeout}
	}
	audioFetchHTTPClientFactory = newSafeAudioFetchClient
	resolveIPAddrs = func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return net.DefaultResolver.LookupIPAddr(ctx, host)
	}
)

type transcribeFromURLRequest struct {
	AudioAPIRecordingID string `json:"audioApiRecordingId"`
	AudioURL            string `json:"audioUrl"`
	MimeType            string `json:"mimeType"`
}

type transcribeFromURLResponse struct {
	Transcription string `json:"transcription"`
}

type transcriptionAudioAPIResponse struct {
	AudioURL string `json:"audioUrl"`
	URL      string `json:"url"`
}

type requestyTranscriptionResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func TranscribeFromURL(w http.ResponseWriter, r *http.Request) {
	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": "Request body is required"})
		return
	}

	var payload transcribeFromURLRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": "Invalid JSON body"})
		return
	}

	audioURL := strings.TrimSpace(payload.AudioURL)
	recordingID := strings.TrimSpace(payload.AudioAPIRecordingID)
	mimeType := strings.TrimSpace(payload.MimeType)

	if recordingID == "" && audioURL == "" {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": "audioApiRecordingId or audioUrl is required"})
		return
	}

	if mimeType == "" {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": "mimeType is required"})
		return
	}

	if recordingID != "" {
		resolvedAudioURL, err := fetchAudioURLFromRecordingID(r.Context(), recordingID)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		audioURL = resolvedAudioURL
	}

	if err := validateHTTPSURL(r.Context(), audioURL); err != nil {
		writeJSONError(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	transcription, err := transcribeAudioFromURL(r.Context(), audioURL, mimeType)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	writeJSON(w, http.StatusOK, transcribeFromURLResponse{Transcription: transcription})
}

func fetchAudioURLFromRecordingID(ctx context.Context, recordingID string) (string, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(os.Getenv("AUDIO_API_URL")), "/")
	if baseURL == "" {
		baseURL = defaultAudioAPIBaseURL
	}

	requestURL := baseURL + "/api/recordings/" + neturl.PathEscape(recordingID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build audio API request: %w", err)
	}

	if apiKey := strings.TrimSpace(os.Getenv("AUDIO_API_KEY")); apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+apiKey)
		req.Header.Set("X-API-Key", apiKey)
	}

	client := &http.Client{Timeout: audioAPIRequestTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("audio API request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := readBodyWithLimit(resp.Body, maxAudioAPIResponseBytes)
	if err != nil {
		return "", fmt.Errorf("failed to read audio API response: %w", err)
	}

	if resp.StatusCode >= 400 {
		message := strings.TrimSpace(string(body))
		if message == "" {
			return "", fmt.Errorf("audio API request failed with status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("audio API request failed with status %d: %s", resp.StatusCode, message)
	}

	var parsed transcriptionAudioAPIResponse
	if err := json.Unmarshal(body, &parsed); err == nil {
		audioURL := strings.TrimSpace(parsed.AudioURL)
		if audioURL == "" {
			audioURL = strings.TrimSpace(parsed.URL)
		}
		if audioURL != "" {
			return audioURL, nil
		}
	}

	var generic map[string]interface{}
	if err := json.Unmarshal(body, &generic); err == nil {
		if audioURL := extractAudioURLFromMap(generic); audioURL != "" {
			return audioURL, nil
		}
	}

	return "", errors.New("audio API response did not include audioUrl")
}

func extractAudioURLFromMap(payload map[string]interface{}) string {
	for _, key := range []string{"audioUrl", "url"} {
		if value, ok := payload[key].(string); ok && strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}

	for _, key := range []string{"recording", "data"} {
		if nested, ok := payload[key].(map[string]interface{}); ok {
			if audioURL := extractAudioURLFromMap(nested); audioURL != "" {
				return audioURL
			}
		}
	}

	return ""
}

func validateHTTPSURL(ctx context.Context, rawURL string) error {
	parsed, err := neturl.Parse(strings.TrimSpace(rawURL))
	if err != nil {
		return errors.New("audio URL must be a valid https URL")
	}
	if !strings.EqualFold(parsed.Scheme, "https") || strings.TrimSpace(parsed.Host) == "" {
		return errors.New("audio URL must be a valid https URL")
	}

	host := strings.TrimSpace(parsed.Hostname())
	if host == "" {
		return errors.New("audio URL must be a valid https URL")
	}
	if isDisallowedHostname(host) {
		return errors.New("audio URL cannot target a private or internal host")
	}

	if ip := net.ParseIP(host); ip != nil {
		if isDisallowedIP(ip) {
			return errors.New("audio URL cannot target a private or internal host")
		}
		return nil
	}

	resolvedIPs, err := resolveIPAddrs(ctx, host)
	if err != nil {
		return fmt.Errorf("audio URL host lookup failed: %w", err)
	}
	if len(resolvedIPs) == 0 {
		return errors.New("audio URL host did not resolve to an IP address")
	}

	for _, resolved := range resolvedIPs {
		if isDisallowedIP(resolved.IP) {
			return errors.New("audio URL cannot target a private or internal host")
		}
	}

	return nil
}

func transcribeAudioFromURL(ctx context.Context, audioURL, mimeType string) (string, error) {
	transcription, err := callRequestyAudioTranscription(ctx, buildDirectURLTranscriptionPayload(audioURL, mimeType))
	if err == nil {
		return transcription, nil
	}

	audioData, fetchErr := fetchAudioAsBase64(ctx, audioURL)
	if fetchErr != nil {
		return "", fmt.Errorf("direct URL transcription failed: %v; base64 fallback failed: %w", err, fetchErr)
	}

	transcription, fallbackErr := callRequestyAudioTranscription(ctx, buildBase64TranscriptionPayload(audioURL, mimeType, audioData))
	if fallbackErr != nil {
		return "", fmt.Errorf("direct URL transcription failed: %v; base64 fallback failed: %w", err, fallbackErr)
	}

	return transcription, nil
}

func buildDirectURLTranscriptionPayload(audioURL, mimeType string) map[string]interface{} {
	return map[string]interface{}{
		"model": transcriptionModel(),
		"messages": []map[string]interface{}{
			{"role": "system", "content": transcriptionInstructionPromptText},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": "Transcribe this audio file."},
					{
						"type": "file",
						"file": map[string]interface{}{
							"file_url":  audioURL,
							"mime_type": mimeType,
						},
					},
				},
			},
		},
		"temperature": 0,
	}
}

func buildBase64TranscriptionPayload(audioURL, mimeType, audioData string) map[string]interface{} {
	filename := path.Base(strings.TrimSpace(audioURL))
	if filename == "." || filename == "/" || filename == "" {
		filename = "audio"
	}

	return map[string]interface{}{
		"model": transcriptionModel(),
		"messages": []map[string]interface{}{
			{"role": "system", "content": transcriptionInstructionPromptText},
			{
				"role": "user",
				"content": []map[string]interface{}{
					{"type": "text", "text": "Transcribe this audio file."},
					{
						"type": "file",
						"file": map[string]interface{}{
							"filename":  filename,
							"file_data": audioData,
							"mime_type": mimeType,
						},
					},
				},
			},
		},
		"temperature": 0,
	}
}

func transcriptionModel() string {
	model := strings.TrimSpace(os.Getenv("GEMINI_TRANSCRIPTION_MODEL"))
	if model == "" {
		return defaultGeminiTranscriptionModel
	}
	return model
}

func fetchAudioAsBase64(ctx context.Context, audioURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build audio download request: %w", err)
	}

	client := audioFetchHTTPClientFactory()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("audio download failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("audio download failed with status %d", resp.StatusCode)
	}

	if resp.ContentLength > maxAudioBytes {
		return "", fmt.Errorf("audio download exceeded maximum size of %d bytes", maxAudioBytes)
	}

	audioBytes, err := readBodyWithLimit(resp.Body, maxAudioBytes)
	if err != nil {
		return "", fmt.Errorf("failed to read audio download: %w", err)
	}

	if len(audioBytes) == 0 {
		return "", errors.New("audio download was empty")
	}

	return base64.StdEncoding.EncodeToString(audioBytes), nil
}

func callRequestyAudioTranscription(ctx context.Context, payload map[string]interface{}) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("REQUESTY_API_KEY"))
	if apiKey == "" {
		return "", errors.New("REQUESTY_API_KEY is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to encode transcription request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestyAudioTranscriptionURL, bytes.NewReader(body))
	if err != nil {
		return "", fmt.Errorf("failed to build transcription request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set(ContentType, JSON)

	client := requestyAudioHTTPClientFactory()
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesty transcription request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, readErr := readBodyWithLimit(resp.Body, maxRequestyErrorBodyBytes)
		if readErr != nil {
			return "", fmt.Errorf("requesty error: status %d (failed to read error body: %v)", resp.StatusCode, readErr)
		}

		var parsed requestyTranscriptionResponse
		if err := json.Unmarshal(body, &parsed); err == nil && parsed.Error != nil && strings.TrimSpace(parsed.Error.Message) != "" {
			return "", fmt.Errorf("requesty error: %s", parsed.Error.Message)
		}

		rawBody := strings.TrimSpace(string(body))
		if rawBody == "" {
			return "", fmt.Errorf("requesty error: status %d", resp.StatusCode)
		}
		return "", fmt.Errorf("requesty error: status %d: %s", resp.StatusCode, rawBody)
	}

	var parsed requestyTranscriptionResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode transcription response: %w", err)
	}

	transcription := extractTranscriptionText(parsed)
	if transcription == "" {
		return "", errors.New("requesty returned empty transcription")
	}

	return transcription, nil
}

func extractTranscriptionText(response requestyTranscriptionResponse) string {
	if len(response.Choices) == 0 {
		return ""
	}

	return strings.TrimSpace(extractTextContent(response.Choices[0].Message.Content))
}

func extractTextContent(content interface{}) string {
	switch value := content.(type) {
	case string:
		return value
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			part := strings.TrimSpace(extractTextContent(item))
			if part != "" {
				parts = append(parts, part)
			}
		}
		return strings.Join(parts, "\n")
	case map[string]interface{}:
		if text, ok := value["text"].(string); ok && strings.TrimSpace(text) != "" {
			return text
		}
		if inner, ok := value["content"]; ok {
			return extractTextContent(inner)
		}
	}

	return ""
}

func readBodyWithLimit(reader io.Reader, limit int64) ([]byte, error) {
	limitedReader := &io.LimitedReader{R: reader, N: limit + 1}
	body, err := io.ReadAll(limitedReader)
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("response exceeded maximum size of %d bytes", limit)
	}
	return body, nil
}

func isDisallowedHostname(host string) bool {
	normalized := strings.ToLower(strings.TrimSpace(host))
	switch normalized {
	case "", "localhost", "localhost.localdomain", "0.0.0.0":
		return true
	default:
		return false
	}
}

func isDisallowedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}

	return ip.IsLoopback() ||
		ip.IsUnspecified() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() ||
		ip.IsInterfaceLocalMulticast()
}

func newSafeAudioFetchClient() *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}

		if isDisallowedHostname(host) {
			return nil, errors.New("audio URL cannot target a private or internal host")
		}

		resolvedIPs, err := resolveIPAddrs(ctx, host)
		if err != nil {
			return nil, err
		}
		if len(resolvedIPs) == 0 {
			return nil, errors.New("audio URL host did not resolve to an IP address")
		}
		for _, resolved := range resolvedIPs {
			if isDisallowedIP(resolved.IP) {
				return nil, errors.New("audio URL cannot target a private or internal host")
			}
		}

		dialer := &net.Dialer{Timeout: 30 * time.Second}
		var lastErr error
		for _, resolved := range resolvedIPs {
			targetAddress := net.JoinHostPort(resolved.IP.String(), port)
			conn, err := dialer.DialContext(ctx, network, targetAddress)
			if err == nil {
				return conn, nil
			}
			lastErr = err
		}

		if lastErr == nil {
			lastErr = errors.New("audio URL host did not resolve to a dialable IP address")
		}

		return nil, lastErr
	}

	return &http.Client{
		Timeout:   audioDownloadTimeout,
		Transport: transport,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= maxRedirectHops {
				return fmt.Errorf("stopped after %d redirects", maxRedirectHops)
			}
			return validateHTTPSURL(req.Context(), req.URL.String())
		},
	}
}
