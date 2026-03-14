package clients

import (
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net/http"
	"notes/models"
	"os"
	"strings"
	"time"
)

var ErrUnauthorized = errors.New("unauthorized")

const clerkBackendAPIURL = "https://api.clerk.com/v1"

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwtClaims struct {
	Subject   string `json:"sub"`
	Issuer    string `json:"iss"`
	ExpiresAt int64  `json:"exp"`
	NotBefore int64  `json:"nbf"`
	Email     string `json:"email"`
	FirstName string `json:"given_name"`
	LastName  string `json:"family_name"`
	Picture   string `json:"picture"`
}

type jwksResponse struct {
	Keys []jwk `json:"keys"`
}

type jwk struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	Use string `json:"use"`
	N   string `json:"n"`
	E   string `json:"e"`
	X5c []string `json:"x5c"`
}

type clerkUserResponse struct {
	ID                    string               `json:"id"`
	FirstName             *string              `json:"first_name"`
	LastName              *string              `json:"last_name"`
	ImageURL              *string              `json:"image_url"`
	CreatedAt             *int64               `json:"created_at"`
	UpdatedAt             *int64               `json:"updated_at"`
	PrimaryEmailAddressID *string              `json:"primary_email_address_id"`
	EmailAddresses        []clerkEmailResponse `json:"email_addresses"`
}

type clerkEmailResponse struct {
	ID           string `json:"id"`
	EmailAddress string `json:"email_address"`
}

func GetAuthUser(token string) (*models.AuthUser, error) {
	claims, err := verifyToken(token)
	if err != nil {
		return nil, err
	}

	user := authUserFromClaims(claims)

	secretKey := strings.TrimSpace(os.Getenv("CLERK_SECRET_KEY"))
	if secretKey == "" {
		return user, nil
	}

	clerkUser, err := fetchClerkUser(secretKey, claims.Subject)
	if err != nil {
		return user, nil
	}

	return authUserFromClerkUser(clerkUser), nil
}

func verifyToken(token string) (*jwtClaims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, ErrUnauthorized
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, ErrUnauthorized
	}

	var header jwtHeader
	if err = json.Unmarshal(headerBytes, &header); err != nil {
		return nil, ErrUnauthorized
	}

	if header.Alg != "RS256" {
		return nil, ErrUnauthorized
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, ErrUnauthorized
	}

	var claims jwtClaims
	if err = json.Unmarshal(payloadBytes, &claims); err != nil {
		return nil, ErrUnauthorized
	}

	if claims.Subject == "" || claims.Issuer == "" || claims.ExpiresAt == 0 {
		return nil, ErrUnauthorized
	}

	now := time.Now().Unix()
	if claims.ExpiresAt <= now {
		return nil, ErrUnauthorized
	}

	if claims.NotBefore != 0 && claims.NotBefore > now {
		return nil, ErrUnauthorized
	}

	key, err := fetchVerificationKey(claims.Issuer, header.Kid)
	if err != nil {
		return nil, ErrUnauthorized
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, ErrUnauthorized
	}

	signingInput := parts[0] + "." + parts[1]
	hash := sha256.Sum256([]byte(signingInput))
	if err = rsa.VerifyPKCS1v15(key, crypto.SHA256, hash[:], signature); err != nil {
		return nil, ErrUnauthorized
	}

	return &claims, nil
}

func fetchVerificationKey(issuer string, kid string) (*rsa.PublicKey, error) {
	jwksURL := strings.TrimRight(issuer, "/") + "/.well-known/jwks.json"

	req, err := http.NewRequest(http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, err
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("jwks request failed with status %d", resp.StatusCode)
	}

	var jwks jwksResponse
	if err = json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, err
	}

	selectedKey := selectJWK(jwks.Keys, kid)
	if selectedKey == nil {
		return nil, errors.New("signing key not found")
	}

	return jwkToRSAPublicKey(*selectedKey)
}

func selectJWK(keys []jwk, kid string) *jwk {
	if kid != "" {
		for i := range keys {
			if keys[i].Kid == kid {
				return &keys[i]
			}
		}
	}

	if len(keys) == 1 {
		return &keys[0]
	}

	return nil
}

func jwkToRSAPublicKey(key jwk) (*rsa.PublicKey, error) {
	if len(key.X5c) > 0 {
		pemBytes := []byte("-----BEGIN CERTIFICATE-----\n" + key.X5c[0] + "\n-----END CERTIFICATE-----\n")
		block, _ := pem.Decode(pemBytes)
		if block == nil {
			return nil, errors.New("invalid x5c certificate")
		}

		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, err
		}

		publicKey, ok := cert.PublicKey.(*rsa.PublicKey)
		if !ok {
			return nil, errors.New("certificate public key is not RSA")
		}

		return publicKey, nil
	}

	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, err
	}

	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, err
	}

	modulus := new(big.Int).SetBytes(nBytes)
	exponent := 0
	for _, b := range eBytes {
		exponent = exponent<<8 + int(b)
	}

	if exponent == 0 {
		return nil, errors.New("invalid RSA exponent")
	}

	return &rsa.PublicKey{N: modulus, E: exponent}, nil
}

func fetchClerkUser(secretKey string, userID string) (*clerkUserResponse, error) {
	req, err := http.NewRequest(http.MethodGet, clerkBackendAPIURL+"/users/"+userID, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bearer "+secretKey)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("clerk user request failed with status %d", resp.StatusCode)
	}

	var user clerkUserResponse
	if err = json.NewDecoder(resp.Body).Decode(&user); err != nil {
		return nil, err
	}

	return &user, nil
}

func authUserFromClaims(claims *jwtClaims) *models.AuthUser {
	user := &models.AuthUser{
		ID: claims.Subject,
	}

	if claims.Email != "" {
		user.Email = &claims.Email
	}

	if claims.FirstName != "" {
		user.FirstName = &claims.FirstName
	}

	if claims.LastName != "" {
		user.LastName = &claims.LastName
	}

	if claims.Picture != "" {
		user.ProfileImageURL = &claims.Picture
	}

	return user
}

func authUserFromClerkUser(user *clerkUserResponse) *models.AuthUser {
	return &models.AuthUser{
		ID:              user.ID,
		Email:           primaryEmail(user),
		FirstName:       user.FirstName,
		LastName:        user.LastName,
		ProfileImageURL: user.ImageURL,
		CreatedAt:       user.CreatedAt,
		UpdatedAt:       user.UpdatedAt,
	}
}

func primaryEmail(user *clerkUserResponse) *string {
	if user.PrimaryEmailAddressID != nil {
		for _, email := range user.EmailAddresses {
			if email.ID == *user.PrimaryEmailAddressID {
				emailAddress := email.EmailAddress
				return &emailAddress
			}
		}
	}

	if len(user.EmailAddresses) == 0 {
		return nil
	}

	emailAddress := user.EmailAddresses[0].EmailAddress
	return &emailAddress
}
