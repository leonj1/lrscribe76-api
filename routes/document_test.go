package routes

import (
	"bytes"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGenerateDocument(t *testing.T) {
	token, publicKeyPEM, jwksBody := newClerkTestAuth(t)

	t.Run("valid request with transcription and templateSections returns document metadata", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)
		t.Setenv("CLERK_JWKS_URL", "https://clerk.test/.well-known/jwks.json")
		t.Setenv("REQUESTY_API_KEY", "requesty-test-key")

		restore := mockHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "router.requesty.ai":
				return jsonHTTPResponse(http.StatusOK, map[string]any{
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": `{"sections":[{"name":"Chief Complaint","content":"Patient presents with headaches."}]}`,
							},
						},
					},
				}), nil
			case req.URL.Host == "clerk.test":
				return rawHTTPResponse(http.StatusOK, jwksBody), nil
			default:
				return nil, fmt.Errorf("unexpected outbound request to %s", req.URL.String())
			}
		})
		defer restore()

		body := map[string]any{
			"transcription": "Doctor: Good morning. Patient reports headaches for a week.",
			"sessionId":     "session-123",
			"model":         "openai-responses/gpt-5.4",
			"templateSections": []map[string]any{
				{
					"name":               "Chief Complaint",
					"description":        "Primary reason for the visit",
					"order":              0,
					"adhereToFormatting": false,
					"allowAssessment":    false,
				},
			},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/generate-document", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		GenerateDocument(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp struct {
			Document            string `json:"document"`
			DocumentGeneratedAt int64  `json:"documentGeneratedAt"`
			ModelUsed           string `json:"modelUsed"`
		}
		decodeJSONResponse(t, rec, &resp)

		if resp.Document != "## Chief Complaint\n\nPatient presents with headaches." {
			t.Fatalf("unexpected document: %q", resp.Document)
		}
		if resp.DocumentGeneratedAt <= 0 {
			t.Fatalf("expected documentGeneratedAt to be set, got %d", resp.DocumentGeneratedAt)
		}
		if resp.ModelUsed != "openai-responses/gpt-5.4" {
			t.Fatalf("unexpected modelUsed: %q", resp.ModelUsed)
		}
	})

	t.Run("missing transcription and notes returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"sessionId": "session-123",
			"templateSections": []map[string]any{
				{"name": "Chief Complaint"},
			},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/generate-document", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		GenerateDocument(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "At least one of transcription or notes, plus template sections, are required")
	})

	t.Run("missing templateSections returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"transcription": "Patient reports headaches.",
			"sessionId":     "session-123",
		}

		req := newJSONRequest(t, http.MethodPost, "/api/generate-document", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		GenerateDocument(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "At least one of transcription or notes, plus template sections, are required")
	})

	t.Run("missing sessionId returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"transcription": "Patient reports headaches.",
			"templateSections": []map[string]any{
				{"name": "Chief Complaint"},
			},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/generate-document", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		GenerateDocument(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "sessionId is required")
	})

	t.Run("empty templateSections returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"transcription":    "Patient reports headaches.",
			"sessionId":        "session-123",
			"templateSections": []map[string]any{},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/generate-document", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		GenerateDocument(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "At least one of transcription or notes, plus template sections, are required")
	})
}

func TestRegenerateSection(t *testing.T) {
	token, publicKeyPEM, jwksBody := newClerkTestAuth(t)

	t.Run("valid request returns content string", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)
		t.Setenv("CLERK_JWKS_URL", "https://clerk.test/.well-known/jwks.json")
		t.Setenv("REQUESTY_API_KEY", "requesty-test-key")

		restore := mockHTTPTransport(t, func(req *http.Request) (*http.Response, error) {
			switch {
			case req.URL.Host == "router.requesty.ai":
				return jsonHTTPResponse(http.StatusOK, map[string]any{
					"choices": []any{
						map[string]any{
							"message": map[string]any{
								"content": "Updated chief complaint content.",
							},
						},
					},
				}), nil
			case req.URL.Host == "clerk.test":
				return rawHTTPResponse(http.StatusOK, jwksBody), nil
			default:
				return nil, fmt.Errorf("unexpected outbound request to %s", req.URL.String())
			}
		})
		defer restore()

		body := map[string]any{
			"transcription": "Patient reports headaches.",
			"sessionId":     "session-123",
			"section": map[string]any{
				"name":               "Chief Complaint",
				"description":        "Primary reason for visit",
				"adhereToFormatting": false,
				"allowAssessment":    false,
			},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/regenerate-section", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		RegenerateSection(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected status 200, got %d: %s", rec.Code, rec.Body.String())
		}

		var resp map[string]string
		decodeJSONResponse(t, rec, &resp)
		if resp["content"] != "Updated chief complaint content." {
			t.Fatalf("unexpected content: %q", resp["content"])
		}
	})

	t.Run("missing transcription and notes returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"sessionId": "session-123",
			"section": map[string]any{
				"name": "Chief Complaint",
			},
		}

		req := newJSONRequest(t, http.MethodPost, "/api/regenerate-section", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		RegenerateSection(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "Source content and section are required")
	})

	t.Run("missing section returns 400", func(t *testing.T) {
		t.Setenv("CLERK_JWT_KEY", publicKeyPEM)

		body := map[string]any{
			"transcription": "Patient reports headaches.",
			"sessionId":     "session-123",
		}

		req := newJSONRequest(t, http.MethodPost, "/api/regenerate-section", body)
		req.Header.Set("Authorization", "Bearer "+token)
		rec := httptest.NewRecorder()

		RegenerateSection(rec, req)

		assertJSONError(t, rec, http.StatusBadRequest, "Source content and section are required")
	})
}

func newJSONRequest(t *testing.T, method, target string, body any) *http.Request {
	t.Helper()

	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal request body: %v", err)
	}

	req := httptest.NewRequest(method, target, bytes.NewReader(payload))
	req.Header.Set(ContentType, JSON)
	return req
}

func decodeJSONResponse(t *testing.T, rec *httptest.ResponseRecorder, dest any) {
	t.Helper()

	if err := json.Unmarshal(rec.Body.Bytes(), dest); err != nil {
		t.Fatalf("decode response body: %v\nbody: %s", err, rec.Body.String())
	}
}

func assertJSONError(t *testing.T, rec *httptest.ResponseRecorder, wantStatus int, wantError string) {
	t.Helper()

	if rec.Code != wantStatus {
		t.Fatalf("expected status %d, got %d: %s", wantStatus, rec.Code, rec.Body.String())
	}

	var resp map[string]string
	decodeJSONResponse(t, rec, &resp)
	if resp["error"] != wantError {
		t.Fatalf("expected error %q, got %q", wantError, resp["error"])
	}
}

func mockHTTPTransport(t *testing.T, fn func(*http.Request) (*http.Response, error)) func() {
	t.Helper()

	original := http.DefaultTransport
	http.DefaultTransport = roundTripFunc(fn)
	return func() {
		http.DefaultTransport = original
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func jsonHTTPResponse(status int, body any) *http.Response {
	payload, _ := json.Marshal(body)
	return rawHTTPResponse(status, payload)
}

func rawHTTPResponse(status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func newClerkTestAuth(t *testing.T) (string, string, []byte) {
	t.Helper()

	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate rsa key: %v", err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key: %v", err)
	}

	publicKeyPEM := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyDER,
	}))

	token := mustSignJWT(t, privateKey, map[string]any{
		"sub": "user_123",
		"iss": "https://clerk.test",
		"azp": "http://localhost",
		"exp": time.Now().Add(time.Hour).Unix(),
		"nbf": time.Now().Add(-time.Minute).Unix(),
		"sts": "active",
	})

	jwksBody, err := json.Marshal(map[string]any{
		"keys": []map[string]any{
			{
				"kid": "test-kid",
				"kty": "RSA",
				"alg": "RS256",
				"n":   base64.RawURLEncoding.EncodeToString(privateKey.PublicKey.N.Bytes()),
				"e":   base64.RawURLEncoding.EncodeToString(bigEndianBytes(privateKey.PublicKey.E)),
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal jwks: %v", err)
	}

	return token, publicKeyPEM, jwksBody
}

func mustSignJWT(t *testing.T, privateKey *rsa.PrivateKey, claims map[string]any) string {
	t.Helper()

	headerJSON, err := json.Marshal(map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "test-kid",
	})
	if err != nil {
		t.Fatalf("marshal jwt header: %v", err)
	}

	claimsJSON, err := json.Marshal(claims)
	if err != nil {
		t.Fatalf("marshal jwt claims: %v", err)
	}

	headerSeg := base64.RawURLEncoding.EncodeToString(headerJSON)
	claimsSeg := base64.RawURLEncoding.EncodeToString(claimsJSON)
	signingInput := headerSeg + "." + claimsSeg
	digest := sha256.Sum256([]byte(signingInput))

	signature, err := rsa.SignPKCS1v15(rand.Reader, privateKey, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatalf("sign jwt: %v", err)
	}

	return strings.Join([]string{
		headerSeg,
		claimsSeg,
		base64.RawURLEncoding.EncodeToString(signature),
	}, ".")
}

func bigEndianBytes(value int) []byte {
	if value == 0 {
		return []byte{0}
	}

	var out []byte
	for value > 0 {
		out = append([]byte{byte(value & 0xff)}, out...)
		value >>= 8
	}
	return out
}
