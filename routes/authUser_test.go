package routes

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"notes/clients"
	"notes/models"
	"testing"
)

func TestAuthUser_ValidClerkJWTReturnsUserInfo(t *testing.T) {
	originalGetAuthUser := getAuthUser
	t.Cleanup(func() {
		getAuthUser = originalGetAuthUser
	})

	email := "test@example.com"
	firstName := "Test"
	lastName := "User"
	profileImageURL := "https://cdn.example.com/avatar.png"
	createdAt := int64(1710450000000)
	updatedAt := int64(1710453600000)
	expectedUser := &models.AuthUser{
		ID:              "user_2abc123",
		Email:           &email,
		FirstName:       &firstName,
		LastName:        &lastName,
		ProfileImageURL: &profileImageURL,
		CreatedAt:       &createdAt,
		UpdatedAt:       &updatedAt,
	}

	getAuthUser = func(token string) (*models.AuthUser, error) {
		if token != "valid-jwt" {
			t.Fatalf("expected token valid-jwt, got %q", token)
		}

		return expectedUser, nil
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)
	req.Header.Set("Authorization", "Bearer valid-jwt")
	recorder := httptest.NewRecorder()

	AuthUser(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, recorder.Code)
	}

	if contentType := recorder.Header().Get(ContentType); contentType != JSON {
		t.Fatalf("expected content type %q, got %q", JSON, contentType)
	}

	var actualUser models.AuthUser
	if err := json.Unmarshal(recorder.Body.Bytes(), &actualUser); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if actualUser.ID != expectedUser.ID {
		t.Fatalf("expected user id %q, got %q", expectedUser.ID, actualUser.ID)
	}

	assertStringPtrEqual(t, "email", expectedUser.Email, actualUser.Email)
	assertStringPtrEqual(t, "firstName", expectedUser.FirstName, actualUser.FirstName)
	assertStringPtrEqual(t, "lastName", expectedUser.LastName, actualUser.LastName)
	assertStringPtrEqual(t, "profileImageUrl", expectedUser.ProfileImageURL, actualUser.ProfileImageURL)
	assertInt64PtrEqual(t, "createdAt", expectedUser.CreatedAt, actualUser.CreatedAt)
	assertInt64PtrEqual(t, "updatedAt", expectedUser.UpdatedAt, actualUser.UpdatedAt)
}

func TestAuthUser_MissingOrInvalidJWTReturnsUnauthorized(t *testing.T) {
	testCases := []struct {
		name                string
		authorizationHeader string
		mock                func(token string) (*models.AuthUser, error)
	}{
		{
			name:                "missing JWT",
			authorizationHeader: "",
			mock: func(token string) (*models.AuthUser, error) {
				t.Fatalf("getAuthUser should not be called for missing JWT")
				return nil, nil
			},
		},
		{
			name:                "invalid JWT",
			authorizationHeader: "Bearer invalid-jwt",
			mock: func(token string) (*models.AuthUser, error) {
				if token != "invalid-jwt" {
					t.Fatalf("expected token invalid-jwt, got %q", token)
				}

				return nil, clients.ErrUnauthorized
			},
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			originalGetAuthUser := getAuthUser
			t.Cleanup(func() {
				getAuthUser = originalGetAuthUser
			})

			getAuthUser = tc.mock

			req := httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)
			if tc.authorizationHeader != "" {
				req.Header.Set("Authorization", tc.authorizationHeader)
			}
			recorder := httptest.NewRecorder()

			AuthUser(recorder, req)

			assertUnauthorizedResponse(t, recorder)
		})
	}
}

func TestAuthUser_ExpiredJWTReturnsUnauthorized(t *testing.T) {
	originalGetAuthUser := getAuthUser
	t.Cleanup(func() {
		getAuthUser = originalGetAuthUser
	})

	getAuthUser = func(token string) (*models.AuthUser, error) {
		if token != "expired-jwt" {
			t.Fatalf("expected token expired-jwt, got %q", token)
		}

		return nil, errors.New("token expired")
	}

	req := httptest.NewRequest(http.MethodGet, "/api/auth/user", nil)
	req.Header.Set("Authorization", "Bearer expired-jwt")
	recorder := httptest.NewRecorder()

	AuthUser(recorder, req)

	assertUnauthorizedResponse(t, recorder)
}

func assertUnauthorizedResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, recorder.Code)
	}

	if contentType := recorder.Header().Get(ContentType); contentType != JSON {
		t.Fatalf("expected content type %q, got %q", JSON, contentType)
	}

	var actual models.MessageResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &actual); err != nil {
		t.Fatalf("failed to decode response body: %v", err)
	}

	if actual.Message != "Unauthorized" {
		t.Fatalf("expected message %q, got %q", "Unauthorized", actual.Message)
	}
}

func assertStringPtrEqual(t *testing.T, field string, expected *string, actual *string) {
	t.Helper()

	if expected == nil || actual == nil {
		if expected != actual {
			t.Fatalf("expected %s %v, got %v", field, expected, actual)
		}

		return
	}

	if *expected != *actual {
		t.Fatalf("expected %s %q, got %q", field, *expected, *actual)
	}
}

func assertInt64PtrEqual(t *testing.T, field string, expected *int64, actual *int64) {
	t.Helper()

	if expected == nil || actual == nil {
		if expected != actual {
			t.Fatalf("expected %s %v, got %v", field, expected, actual)
		}

		return
	}

	if *expected != *actual {
		t.Fatalf("expected %s %d, got %d", field, *expected, *actual)
	}
}
