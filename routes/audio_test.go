package routes

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/husobee/vestigo"
)

const (
	testRecordingID = "550e8400-e29b-41d4-a716-446655440000"
	testSessionID   = "session_123"
	testUserID      = "user_123"
)

func TestAudioStart(t *testing.T) {
	t.Run("valid request returns 201 with recordingId", func(t *testing.T) {
		audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if r.URL.Path != "/api/recordings" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("X-User-Id"); got != testUserID {
				t.Fatalf("unexpected X-User-Id: %q", got)
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}

			if got := body["sessionId"]; got != testSessionID {
				t.Fatalf("unexpected sessionId: %#v", got)
			}
			if got := body["userId"]; got != testUserID {
				t.Fatalf("unexpected userId: %#v", got)
			}
			if got := body["mimeType"]; got != "audio/webm" {
				t.Fatalf("unexpected mimeType: %#v", got)
			}

			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"recordingId":"` + testRecordingID + `","status":"recording"}`))
		}))
		defer audioAPI.Close()

		t.Setenv("AUDIO_API_URL", audioAPI.URL)

		router := vestigo.NewRouter()
		router.Post("/api/audio/start", ConvexAuth(AudioStart))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/start", strings.NewReader(`{"sessionId":"`+testSessionID+`","mimeType":"audio/webm"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusCreated {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if got := body["recordingId"]; got != testRecordingID {
			t.Fatalf("unexpected recordingId: %#v", got)
		}
	})

	t.Run("missing sessionId or mimeType returns 400", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/start", ConvexAuth(AudioStart))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/start", strings.NewReader(`{"userId":"`+testUserID+`"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "sessionId, userId, and mimeType are required")
	})

	t.Run("missing userId with no auth returns 401", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/start", ConvexAuth(AudioStart))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/start", strings.NewReader(`{"sessionId":"`+testSessionID+`","mimeType":"audio/webm"}`))
		req.Header.Set("Content-Type", "application/json")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "Not authenticated: no userId provided")
	})

	t.Run("invalid UUID format for optional id returns 400", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/start", ConvexAuth(AudioStart))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/start", strings.NewReader(`{"sessionId":"`+testSessionID+`","mimeType":"audio/webm","id":"not-a-uuid"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "id must be a valid UUID")
	})
}

func TestAudioChunk(t *testing.T) {
	t.Run("valid binary upload with X-Chunk-Index returns 200", func(t *testing.T) {
		audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if r.URL.Path != "/api/recordings/"+testRecordingID+"/chunks" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("X-Chunk-Index"); got != "0" {
				t.Fatalf("unexpected X-Chunk-Index: %q", got)
			}
			if got := r.Header.Get("X-User-Id"); got != testUserID {
				t.Fatalf("unexpected X-User-Id: %q", got)
			}

			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Fatalf("read request body: %v", err)
			}
			if string(body) != "audio-bytes" {
				t.Fatalf("unexpected body: %q", string(body))
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"chunkIndex":0,"received":true}`))
		}))
		defer audioAPI.Close()

		t.Setenv("AUDIO_API_URL", audioAPI.URL)

		router := vestigo.NewRouter()
		router.Post("/api/audio/chunk/:recordingId", ConvexAuth(AudioChunk))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/chunk/"+testRecordingID, strings.NewReader("audio-bytes"))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-User-Id", testUserID)
		req.Header.Set("X-Chunk-Index", "0")

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "received", true)
	})

	t.Run("missing X-Chunk-Index header returns 400", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/chunk/:recordingId", ConvexAuth(AudioChunk))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/chunk/"+testRecordingID, strings.NewReader("audio-bytes"))
		req.Header.Set("Content-Type", "application/octet-stream")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "X-Chunk-Index header is required")
	})
}

func TestAudioComplete(t *testing.T) {
	t.Run("valid totalChunks returns 200 with status and audioUrl", func(t *testing.T) {
		audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if r.URL.Path != "/api/recordings/"+testRecordingID+"/complete" {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("X-User-Id"); got != testUserID {
				t.Fatalf("unexpected X-User-Id: %q", got)
			}

			var body map[string]any
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatalf("decode request body: %v", err)
			}
			if got := body["totalChunks"]; got != float64(10) {
				t.Fatalf("unexpected totalChunks: %#v", got)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"completed","audioUrl":"https://example.com/audio.webm"}`))
		}))
		defer audioAPI.Close()

		t.Setenv("AUDIO_API_URL", audioAPI.URL)

		router := vestigo.NewRouter()
		router.Post("/api/audio/complete/:recordingId", WithConvexAuth(AudioComplete))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/complete/"+testRecordingID, strings.NewReader(`{"totalChunks":10}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "status", "completed")
		assertJSONField(t, rec.Body.Bytes(), "audioUrl", "https://example.com/audio.webm")
	})

	t.Run("missing totalChunks returns 400", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/complete/:recordingId", WithConvexAuth(AudioComplete))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/complete/"+testRecordingID, strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "totalChunks is required")
	})
}

func TestAudioTriggerInterim(t *testing.T) {
	t.Run("valid request returns 200 with newRecordingId", func(t *testing.T) {
		var completeCalls int32
		var startCalls int32

		audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch {
			case r.Method == http.MethodPost && r.URL.Path == "/api/recordings/"+testRecordingID+"/complete":
				atomic.AddInt32(&completeCalls, 1)

				if got := r.Header.Get("X-User-Id"); got != testUserID {
					t.Fatalf("unexpected complete X-User-Id: %q", got)
				}

				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode complete body: %v", err)
				}
				if got := body["totalChunks"]; got != float64(5) {
					t.Fatalf("unexpected totalChunks: %#v", got)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"status":"completed","audioUrl":""}`))
			case r.Method == http.MethodPost && r.URL.Path == "/api/recordings":
				atomic.AddInt32(&startCalls, 1)

				if got := r.Header.Get("X-User-Id"); got != testUserID {
					t.Fatalf("unexpected start X-User-Id: %q", got)
				}

				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatalf("decode start body: %v", err)
				}
				if got := body["sessionId"]; got != testSessionID {
					t.Fatalf("unexpected sessionId: %#v", got)
				}
				if got := body["userId"]; got != testUserID {
					t.Fatalf("unexpected userId: %#v", got)
				}
				if got := body["mimeType"]; got != "audio/webm" {
					t.Fatalf("unexpected mimeType: %#v", got)
				}
				if got := body["id"]; got == "" || got == nil {
					t.Fatalf("expected new recording id in body, got %#v", got)
				}

				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusCreated)
				_, _ = w.Write([]byte(`{"status":"recording"}`))
			default:
				t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
			}
		}))
		defer audioAPI.Close()

		t.Setenv("AUDIO_API_URL", audioAPI.URL)

		router := vestigo.NewRouter()
		router.Post("/api/audio/trigger-interim/:recordingId", WithConvexAuth(AudioTriggerInterim))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/trigger-interim/"+testRecordingID, strings.NewReader(`{"totalChunks":5,"sessionId":"`+testSessionID+`","mimeType":"audio/webm"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}

		var body map[string]any
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response body: %v", err)
		}
		if got := body["newRecordingId"]; got == "" || got == nil {
			t.Fatalf("expected newRecordingId, got %#v", got)
		}
		if got := atomic.LoadInt32(&completeCalls); got != 1 {
			t.Fatalf("expected complete endpoint to be called once, got %d", got)
		}
		if got := atomic.LoadInt32(&startCalls); got != 1 {
			t.Fatalf("expected start endpoint to be called once, got %d", got)
		}
	})

	t.Run("missing totalChunks or sessionId returns 400", func(t *testing.T) {
		router := vestigo.NewRouter()
		router.Post("/api/audio/trigger-interim/:recordingId", WithConvexAuth(AudioTriggerInterim))

		req := httptest.NewRequest(http.MethodPost, "/api/audio/trigger-interim/"+testRecordingID, strings.NewReader(`{"mimeType":"audio/webm"}`))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "error", "totalChunks and sessionId are required")
	})
}

func TestAudioStatus(t *testing.T) {
	t.Run("valid recordingId returns 200 with status info", func(t *testing.T) {
		audioAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Fatalf("unexpected method: %s", r.Method)
			}
			if r.URL.Path != "/api/recordings/"+testRecordingID {
				t.Fatalf("unexpected path: %s", r.URL.Path)
			}
			if got := r.Header.Get("X-User-Id"); got != testUserID {
				t.Fatalf("unexpected X-User-Id: %q", got)
			}

			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"recordingId":"` + testRecordingID + `","status":"completed","audioUrl":"https://example.com/audio.webm","totalChunks":10,"createdAt":"2026-03-14T20:00:00.000Z"}`))
		}))
		defer audioAPI.Close()

		t.Setenv("AUDIO_API_URL", audioAPI.URL)

		router := vestigo.NewRouter()
		router.Get("/api/audio/status/:recordingId", ConvexAuth(AudioStatus))

		req := httptest.NewRequest(http.MethodGet, "/api/audio/status/"+testRecordingID, nil)
		req.Header.Set("X-User-Id", testUserID)

		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("unexpected status: %d body=%s", rec.Code, rec.Body.String())
		}
		assertJSONField(t, rec.Body.Bytes(), "recordingId", testRecordingID)
		assertJSONField(t, rec.Body.Bytes(), "status", "completed")
		assertJSONField(t, rec.Body.Bytes(), "audioUrl", "https://example.com/audio.webm")
	})
}

func assertJSONField(t *testing.T, body []byte, field string, want any) {
	t.Helper()

	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode json body: %v body=%s", err, string(body))
	}

	if got := payload[field]; got != want {
		t.Fatalf("unexpected %s: got=%#v want=%#v body=%s", field, got, want, string(body))
	}
}
