package routes

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
)

const defaultGeminiTranscriptionModel = "google/gemini-3.1-pro-preview"

func fetchAudioAsBase64(ctx context.Context, audioURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, audioURL, nil)
	if err != nil {
		return "", err
	}

	client := http.DefaultClient
	if audioFetchHTTPClientFactory != nil {
		client = audioFetchHTTPClientFactory()
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("audio download failed with status %d", resp.StatusCode)
	}

	reader := io.Reader(resp.Body)
	if maxAudioBytes > 0 {
		reader = io.LimitReader(resp.Body, maxAudioBytes+1)
	}

	audioBytes, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	if maxAudioBytes > 0 && int64(len(audioBytes)) > maxAudioBytes {
		return "", fmt.Errorf("audio download exceeded %d bytes", maxAudioBytes)
	}

	return base64.StdEncoding.EncodeToString(audioBytes), nil
}
