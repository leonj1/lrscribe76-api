package routes

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/clerk/clerk-sdk-go/v2"
	"github.com/husobee/vestigo"
	"notes/models"
)

func TestListTranscriptionsReturnsUserTranscriptions(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, time.March, 14, 20, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "title", "audio_url", "content", "status", "created_at",
	}).AddRow(
		1, "user_123", "Patient Visit - March 2026", "https://example.com/audio.webm", "Transcribed text content...", "completed", createdAt,
	)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM transcriptions WHERE `user_id`=? ORDER BY `created_at` DESC",
	)).WithArgs("user_123").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/transcriptions", nil)
	req.Header.Set("Authorization", "Bearer "+mustSignedClerkJWT(t, "user_123"))
	recorder := httptest.NewRecorder()

	ListTranscriptions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var got []models.Transcription
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(got) != 1 {
		t.Fatalf("expected 1 transcription, got %d", len(got))
	}

	if got[0].Id != 1 || got[0].UserId != "user_123" || got[0].Status != "completed" {
		t.Fatalf("unexpected transcription: %+v", got[0])
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestListTranscriptionsReturnsEmptyArrayWhenUserHasNoTranscriptions(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	rows := sqlmock.NewRows([]string{
		"id", "user_id", "title", "audio_url", "content", "status", "created_at",
	})

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM transcriptions WHERE `user_id`=? ORDER BY `created_at` DESC",
	)).WithArgs("user_empty").WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/transcriptions", nil)
	req.Header.Set("Authorization", "Bearer "+mustSignedClerkJWT(t, "user_empty"))
	recorder := httptest.NewRecorder()

	ListTranscriptions(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var got []models.Transcription
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if len(got) != 0 {
		t.Fatalf("expected empty response, got %d items", len(got))
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTranscriptionReturnsCreatedTranscription(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	mock.ExpectExec(regexp.QuoteMeta(
		"INSERT INTO transcriptions (user_id, title, audio_url, content, status, created_at) VALUES (?,?,?,?,?,?)",
	)).WithArgs(
		"user_123",
		"Patient Visit - March 2026",
		"https://example.com/audio.webm",
		"Optional initial transcription text",
		"pending",
		sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(2, 1))

	body := bytes.NewBufferString(`{"userId":"user_123","title":"Patient Visit - March 2026","audioUrl":"https://example.com/audio.webm","content":"Optional initial transcription text"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/transcriptions", body)
	req.Header.Set("Authorization", "Bearer "+mustUnsignedClerkJWT("user_123"))
	recorder := httptest.NewRecorder()

	CreateTranscription(recorder, req)

	if recorder.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, recorder.Code)
	}

	var got models.Transcription
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Id != 2 || got.UserId != "user_123" || got.Status != "pending" {
		t.Fatalf("unexpected transcription: %+v", got)
	}

	if got.CreatedAt.IsZero() {
		t.Fatal("expected createdAt to be set")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestCreateTranscriptionMissingTitleReturnsValidationError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/transcriptions", bytes.NewBufferString(`{"userId":"user_123","audioUrl":"https://example.com/audio.webm"}`))
	req.Header.Set("Authorization", "Bearer "+mustUnsignedClerkJWT("user_123"))
	recorder := httptest.NewRecorder()

	CreateTranscription(recorder, req)

	assertValidationError(t, recorder, "title")
}

func TestCreateTranscriptionMissingAudioURLReturnsValidationError(t *testing.T) {
	req := httptest.NewRequest(http.MethodPost, "/api/transcriptions", bytes.NewBufferString(`{"userId":"user_123","title":"Patient Visit - March 2026"}`))
	req.Header.Set("Authorization", "Bearer "+mustUnsignedClerkJWT("user_123"))
	recorder := httptest.NewRecorder()

	CreateTranscription(recorder, req)

	assertValidationError(t, recorder, "audioUrl")
}

func TestGetTranscriptionReturnsOwnedTranscription(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, time.March, 14, 20, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "title", "audio_url", "content", "status", "created_at",
	}).AddRow(
		1, "user_123", "Patient Visit - March 2026", "https://example.com/audio.webm", "Transcribed text...", "completed", createdAt,
	)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM transcriptions WHERE `id`=?",
	)).WithArgs(int64(1)).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/transcriptions/1", nil)
	vestigo.AddParam(req, "id", "1")
	req = req.WithContext(withSessionClaims(req.Context(), "user_123"))
	recorder := httptest.NewRecorder()

	GetTranscription(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	var got models.Transcription
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Id != 1 || got.UserId != "user_123" {
		t.Fatalf("unexpected transcription: %+v", got)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetTranscriptionReturnsNotFoundForUnknownID(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM transcriptions WHERE `id`=?",
	)).WithArgs(int64(404)).WillReturnError(sql.ErrNoRows)

	req := httptest.NewRequest(http.MethodGet, "/api/transcriptions/404", nil)
	vestigo.AddParam(req, "id", "404")
	req = req.WithContext(withSessionClaims(req.Context(), "user_123"))
	recorder := httptest.NewRecorder()

	GetTranscription(recorder, req)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected status %d, got %d", http.StatusNotFound, recorder.Code)
	}

	assertMessageResponse(t, recorder, "Transcription not found")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func TestGetTranscriptionReturnsUnauthorizedForDifferentOwner(t *testing.T) {
	_, mock, cleanup := setupMockDB(t)
	defer cleanup()

	createdAt := time.Date(2026, time.March, 14, 20, 0, 0, 0, time.UTC)
	rows := sqlmock.NewRows([]string{
		"id", "user_id", "title", "audio_url", "content", "status", "created_at",
	}).AddRow(
		1, "user_other", "Patient Visit - March 2026", "https://example.com/audio.webm", "Transcribed text...", "completed", createdAt,
	)

	mock.ExpectQuery(regexp.QuoteMeta(
		"SELECT `id`, `user_id`, `title`, `audio_url`, `content`, `status`, `created_at` FROM transcriptions WHERE `id`=?",
	)).WithArgs(int64(1)).WillReturnRows(rows)

	req := httptest.NewRequest(http.MethodGet, "/api/transcriptions/1", nil)
	vestigo.AddParam(req, "id", "1")
	req = req.WithContext(withSessionClaims(req.Context(), "user_123"))
	recorder := httptest.NewRecorder()

	GetTranscription(recorder, req)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	assertMessageResponse(t, recorder, "Unauthorized")

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet sql expectations: %v", err)
	}
}

func setupMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock, func()) {
	t.Helper()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("create sqlmock: %v", err)
	}

	previous := models.SetDB(db)
	cleanup := func() {
		models.SetDB(previous)
		db.Close()
	}

	return db, mock, cleanup
}

func assertValidationError(t *testing.T, recorder *httptest.ResponseRecorder, field string) {
	t.Helper()

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, recorder.Code)
	}

	var got validationError
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Message != "Required" || got.Field != field {
		t.Fatalf("unexpected validation error: %+v", got)
	}
}

func assertMessageResponse(t *testing.T, recorder *httptest.ResponseRecorder, want string) {
	t.Helper()

	var got models.MessageResponse
	if err := json.NewDecoder(recorder.Body).Decode(&got); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if got.Message != want {
		t.Fatalf("expected message %q, got %q", want, got.Message)
	}
}

func withSessionClaims(ctx context.Context, subject string) context.Context {
	return clerk.ContextWithSessionClaims(ctx, &clerk.SessionClaims{Subject: subject})
}

func mustUnsignedClerkJWT(userID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	claims := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"` + userID + `"}`))
	return header + "." + claims + "."
}

func mustSignedClerkJWT(t *testing.T, userID string) string {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	publicPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	})

	t.Setenv("CLERK_JWT_KEY", string(publicPEM))
	t.Setenv("CLERK_ISSUER", "")

	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT","kid":"test-key"}`))
	claimsPayload := struct {
		Exp int64  `json:"exp"`
		Sub string `json:"sub"`
	}{
		Exp: time.Now().Add(time.Hour).Unix(),
		Sub: userID,
	}

	claimsJSON, err := json.Marshal(claimsPayload)
	if err != nil {
		t.Fatalf("marshal claims: %v", err)
	}

	claims := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signedContent := header + "." + claims
	hashed := sha256.Sum256([]byte(signedContent))

	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, hashed[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	return signedContent + "." + base64.RawURLEncoding.EncodeToString(signature)
}
