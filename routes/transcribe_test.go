package routes

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	neturl "net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTranscribe_ValidBase64AudioDataReturnsTranscription(t *testing.T) {
	setRequestyAPIKey(t)
	setClerkJWTKey(t)

	var captured requestyMultimodalChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("unexpected method: %s", r.Method)
		}
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		testWriteJSON(t, w, http.StatusOK, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "transcribed text",
					},
				},
			},
		})
	}))
	defer server.Close()

	restore := overrideTranscriptionHooks(server.URL, server.Client())
	defer restore()

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/transcribe", map[string]string{
		"audioData": base64.StdEncoding.EncodeToString([]byte("audio-bytes")),
	})
	rec := httptest.NewRecorder()

	Transcribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response transcribeResponse
	decodeJSONResponse(t, rec.Body, &response)
	if response.Transcription != "transcribed text" {
		t.Fatalf("transcription = %q", response.Transcription)
	}

	if got := captured.Messages[0].Content[1].ImageURL.URL; !strings.HasPrefix(got, "data:audio/webm;base64,") {
		t.Fatalf("audio payload prefix = %q", got)
	}
}

func TestTranscribe_MissingAudioDataReturnsBadRequest(t *testing.T) {
	setClerkJWTKey(t)

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/transcribe", map[string]string{})
	rec := httptest.NewRecorder()

	Transcribe(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	decodeJSONResponse(t, rec.Body, &response)
	if response["error"] != "Audio data is required" {
		t.Fatalf("error = %q", response["error"])
	}
}

func TestTranscribe_CustomMimeTypeIsForwarded(t *testing.T) {
	setRequestyAPIKey(t)
	setClerkJWTKey(t)

	var captured requestyMultimodalChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		testWriteJSON(t, w, http.StatusOK, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "ok",
					},
				},
			},
		})
	}))
	defer server.Close()

	restore := overrideTranscriptionHooks(server.URL, server.Client())
	defer restore()

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/transcribe", map[string]string{
		"audioData": base64.StdEncoding.EncodeToString([]byte("audio-bytes")),
		"mimeType":  "audio/mp4",
	})
	rec := httptest.NewRecorder()

	Transcribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if got := captured.Messages[0].Content[1].ImageURL.URL; !strings.HasPrefix(got, "data:audio/mp4;base64,") {
		t.Fatalf("audio payload prefix = %q", got)
	}
}

func TestTranscribe_DefaultMimeTypeIsAudioWebm(t *testing.T) {
	setRequestyAPIKey(t)
	setClerkJWTKey(t)

	var captured requestyMultimodalChatRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&captured); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		testWriteJSON(t, w, http.StatusOK, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "ok",
					},
				},
			},
		})
	}))
	defer server.Close()

	restore := overrideTranscriptionHooks(server.URL, server.Client())
	defer restore()

	req := authenticatedJSONRequest(t, http.MethodPost, "/api/transcribe", map[string]string{
		"audioData": base64.StdEncoding.EncodeToString([]byte("audio-bytes")),
	})
	rec := httptest.NewRecorder()

	Transcribe(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	if got := captured.Messages[0].Content[1].ImageURL.URL; !strings.HasPrefix(got, "data:audio/webm;base64,") {
		t.Fatalf("audio payload prefix = %q", got)
	}
}

func TestTranscribeFromURL_ValidAudioAPIRecordingIDReturnsTranscription(t *testing.T) {
	setRequestyAPIKey(t)

	audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/recordings/rec-123" {
			t.Fatalf("unexpected audio API path: %s", r.URL.Path)
		}
		testWriteJSON(t, w, http.StatusOK, map[string]string{
			"audioUrl": "https://example.com/audio.webm",
		})
	}))
	defer audioAPI.Close()
	setenv(t, "AUDIO_API_URL", audioAPI.URL)

	var captured map[string]interface{}
	requesty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		decodeJSONResponse(t, r.Body, &captured)
		testWriteJSON(t, w, http.StatusOK, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "from recording id",
					},
				},
			},
		})
	}))
	defer requesty.Close()

	restore := overrideTranscribeFromURLHooks(requesty.URL, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.IPv4(93, 184, 216, 34)}}, nil
	}, nil)
	defer restore()

	req := jsonRequest(t, http.MethodPost, "/api/transcribe-from-url", map[string]string{
		"audioApiRecordingId": "rec-123",
		"mimeType":            "audio/webm",
	})
	rec := httptest.NewRecorder()

	TranscribeFromURL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response transcribeFromURLResponse
	decodeJSONResponse(t, rec.Body, &response)
	if response.Transcription != "from recording id" {
		t.Fatalf("transcription = %q", response.Transcription)
	}

	file := nestedMap(t, captured, "messages", 1, "content", 1, "file")
	if file["file_url"] != "https://example.com/audio.webm" {
		t.Fatalf("file_url = %#v", file["file_url"])
	}
	if file["mime_type"] != "audio/webm" {
		t.Fatalf("mime_type = %#v", file["mime_type"])
	}
}

func TestTranscribeFromURL_ValidAudioURLFallbackReturnsTranscription(t *testing.T) {
	setRequestyAPIKey(t)

	audio := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/audio.webm" {
			t.Fatalf("unexpected audio path: %s", r.URL.Path)
		}
		_, _ = w.Write([]byte("fallback-audio"))
	}))
	defer audio.Close()

	var payloads []map[string]interface{}
	requesty := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]interface{}
		decodeJSONResponse(t, r.Body, &payload)
		payloads = append(payloads, payload)

		if len(payloads) == 1 {
			testWriteJSON(t, w, http.StatusInternalServerError, map[string]interface{}{
				"error": map[string]string{"message": "direct upload unsupported"},
			})
			return
		}

		testWriteJSON(t, w, http.StatusOK, map[string]interface{}{
			"choices": []map[string]interface{}{
				{
					"message": map[string]interface{}{
						"content": "fallback transcription",
					},
				},
			},
		})
	}))
	defer requesty.Close()

	audioClient := rewriteClient(t, audio.Client(), audio.URL)
	restore := overrideTranscribeFromURLHooks(requesty.URL, func(ctx context.Context, host string) ([]net.IPAddr, error) {
		return []net.IPAddr{{IP: net.IPv4(93, 184, 216, 34)}}, nil
	}, func() *http.Client {
		return audioClient
	})
	defer restore()

	req := jsonRequest(t, http.MethodPost, "/api/transcribe-from-url", map[string]string{
		"audioUrl": "https://example.com/audio.webm",
		"mimeType": "audio/webm",
	})
	rec := httptest.NewRecorder()

	TranscribeFromURL(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response transcribeFromURLResponse
	decodeJSONResponse(t, rec.Body, &response)
	if response.Transcription != "fallback transcription" {
		t.Fatalf("transcription = %q", response.Transcription)
	}

	if len(payloads) != 2 {
		t.Fatalf("requesty payload count = %d", len(payloads))
	}

	directFile := nestedMap(t, payloads[0], "messages", 1, "content", 1, "file")
	if directFile["file_url"] != "https://example.com/audio.webm" {
		t.Fatalf("direct file_url = %#v", directFile["file_url"])
	}

	fallbackFile := nestedMap(t, payloads[1], "messages", 1, "content", 1, "file")
	if fallbackFile["mime_type"] != "audio/webm" {
		t.Fatalf("fallback mime_type = %#v", fallbackFile["mime_type"])
	}
	encoded, _ := fallbackFile["file_data"].(string)
	if encoded == "" {
		t.Fatal("fallback file_data was empty")
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode fallback file_data: %v", err)
	}
	if string(decoded) != "fallback-audio" {
		t.Fatalf("fallback audio = %q", string(decoded))
	}
}

func TestTranscribeFromURL_MissingAudioInputsReturnsBadRequest(t *testing.T) {
	req := jsonRequest(t, http.MethodPost, "/api/transcribe-from-url", map[string]string{
		"mimeType": "audio/webm",
	})
	rec := httptest.NewRecorder()

	TranscribeFromURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	decodeJSONResponse(t, rec.Body, &response)
	if response["error"] != "audioApiRecordingId or audioUrl is required" {
		t.Fatalf("error = %q", response["error"])
	}
}

func TestTranscribeFromURL_NonHTTPSAudioURLReturnsBadRequest(t *testing.T) {
	req := jsonRequest(t, http.MethodPost, "/api/transcribe-from-url", map[string]string{
		"audioUrl": "http://example.com/audio.webm",
		"mimeType": "audio/webm",
	})
	rec := httptest.NewRecorder()

	TranscribeFromURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	decodeJSONResponse(t, rec.Body, &response)
	if response["error"] != "audio URL must be a valid https URL" {
		t.Fatalf("error = %q", response["error"])
	}
}

func TestTranscribeFromURL_InvalidURLFormatReturnsBadRequest(t *testing.T) {
	req := jsonRequest(t, http.MethodPost, "/api/transcribe-from-url", map[string]string{
		"audioUrl": "://not-a-url",
		"mimeType": "audio/webm",
	})
	rec := httptest.NewRecorder()

	TranscribeFromURL(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}

	var response map[string]string
	decodeJSONResponse(t, rec.Body, &response)
	if response["error"] != "audio URL must be a valid https URL" {
		t.Fatalf("error = %q", response["error"])
	}
}

func overrideTranscriptionHooks(requestyURL string, client *http.Client) func() {
	prevURL := requestyTranscriptionURL
	prevClient := requestyTranscriptionHTTPClient

	requestyTranscriptionURL = requestyURL
	requestyTranscriptionHTTPClient = client

	return func() {
		requestyTranscriptionURL = prevURL
		requestyTranscriptionHTTPClient = prevClient
	}
}

func overrideTranscribeFromURLHooks(
	requestyURL string,
	resolver func(context.Context, string) ([]net.IPAddr, error),
	audioClientFactory func() *http.Client,
) func() {
	prevURL := requestyAudioTranscriptionURL
	prevRequestyFactory := requestyAudioHTTPClientFactory
	prevAudioFactory := audioFetchHTTPClientFactory
	prevResolver := resolveIPAddrs

	requestyAudioTranscriptionURL = requestyURL
	requestyAudioHTTPClientFactory = func() *http.Client {
		return http.DefaultClient
	}
	audioFetchHTTPClientFactory = newSafeAudioFetchClient
	resolveIPAddrs = resolver

	if audioClientFactory != nil {
		audioFetchHTTPClientFactory = audioClientFactory
	}

	return func() {
		requestyAudioTranscriptionURL = prevURL
		requestyAudioHTTPClientFactory = prevRequestyFactory
		audioFetchHTTPClientFactory = prevAudioFactory
		resolveIPAddrs = prevResolver
	}
}

func rewriteClient(t *testing.T, base *http.Client, target string) *http.Client {
	t.Helper()

	parsed, err := neturl.Parse(target)
	if err != nil {
		t.Fatalf("parse target URL: %v", err)
	}

	return &http.Client{
		Timeout: base.Timeout,
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			clone := req.Clone(req.Context())
			urlCopy := *clone.URL
			urlCopy.Scheme = parsed.Scheme
			urlCopy.Host = parsed.Host
			clone.URL = &urlCopy
			return base.Transport.RoundTrip(clone)
		}),
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func authenticatedJSONRequest(t *testing.T, method, target string, payload interface{}) *http.Request {
	t.Helper()

	req := jsonRequest(t, method, target, payload)
	req.Header.Set("Authorization", "Bearer "+clerkJWT(t))
	return req
}

func jsonRequest(t *testing.T, method, target string, payload interface{}) *http.Request {
	t.Helper()

	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(body))
	req.Header.Set(ContentType, JSON)
	return req
}

func decodeJSONResponse(t *testing.T, body io.Reader, target interface{}) {
	t.Helper()
	if err := json.NewDecoder(body).Decode(target); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
}

func testWriteJSON(t *testing.T, w http.ResponseWriter, status int, payload interface{}) {
	t.Helper()

	w.Header().Set(ContentType, JSON)
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode JSON response: %v", err)
	}
}

func nestedMap(t *testing.T, payload map[string]interface{}, path ...interface{}) map[string]interface{} {
	t.Helper()

	var current interface{} = payload
	for _, segment := range path {
		switch key := segment.(type) {
		case string:
			m, ok := current.(map[string]interface{})
			if !ok {
				t.Fatalf("expected map at %v, got %T", path, current)
			}
			current = m[key]
		case int:
			slice, ok := current.([]interface{})
			if !ok {
				t.Fatalf("expected slice at %v, got %T", path, current)
			}
			current = slice[key]
		default:
			t.Fatalf("unsupported path segment %T", segment)
		}
	}

	result, ok := current.(map[string]interface{})
	if !ok {
		t.Fatalf("expected final map, got %T", current)
	}
	return result
}

func setRequestyAPIKey(t *testing.T) {
	t.Helper()
	setenv(t, "REQUESTY_API_KEY", "test-requesty-key")
}

func setClerkJWTKey(t *testing.T) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}

	publicDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	publicPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER})
	setenv(t, "CLERK_JWT_KEY", string(publicPEM))
	setenv(t, "TEST_CLERK_PRIVATE_KEY_B64", base64.StdEncoding.EncodeToString(x509.MarshalPKCS1PrivateKey(privateKey)))
}

func clerkJWT(t *testing.T) string {
	t.Helper()

	privateKeyDER, err := base64.StdEncoding.DecodeString(os.Getenv("TEST_CLERK_PRIVATE_KEY_B64"))
	if err != nil {
		t.Fatalf("decode test private key: %v", err)
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(privateKeyDER)
	if err != nil {
		t.Fatalf("parse test private key: %v", err)
	}

	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}
	claims := map[string]interface{}{
		"sub": "user_test",
		"nbf": time.Now().Add(-1 * time.Minute).Unix(),
		"exp": time.Now().Add(1 * time.Hour).Unix(),
	}

	return signJWT(t, privateKey, header, claims)
}

func signJWT(t *testing.T, privateKey *rsa.PrivateKey, header, claims interface{}) string {
	t.Helper()

	headerJSON, err := json.Marshal(header)
	if err != nil {
		t.Fatalf("marshal JWT header: %v", err)
	}
	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal JWT claims: %v", err)
	}

	signingInput := base64.RawURLEncoding.EncodeToString(headerJSON) + "." + base64.RawURLEncoding.EncodeToString(claimsJSON)
	digest := sha256.Sum256([]byte(signingInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign JWT: %v", err)
	}

	return signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)
}

func setenv(t *testing.T, key, value string) {
	t.Helper()

	previousValue, existed := os.LookupEnv(key)
	if err := os.Setenv(key, value); err != nil {
		t.Fatalf("setenv %s: %v", key, err)
	}

	t.Cleanup(func() {
		var err error
		if existed {
			err = os.Setenv(key, previousValue)
		} else {
			err = os.Unsetenv(key)
		}
		if err != nil {
			t.Fatalf("restore env %s: %v", key, err)
		}
	})
}
