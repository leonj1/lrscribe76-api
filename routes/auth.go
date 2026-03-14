package routes

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

type clerkClaims struct {
	Exp int64  `json:"exp"`
	Iss string `json:"iss"`
	Nbf int64  `json:"nbf"`
	Sub string `json:"sub"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwksDocument struct {
	Keys []jwkKey `json:"keys"`
}

type jwkKey struct {
	Alg string `json:"alg"`
	E   string `json:"e"`
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	N   string `json:"n"`
	Use string `json:"use"`
}

var jwksCache = struct {
	document *jwksDocument
	expiresAt time.Time
	sync.RWMutex
}{}

func authenticateClerkJWT(r *http.Request) (string, error) {
	token, err := bearerToken(r.Header.Get("Authorization"))
	if err != nil {
		return "", err
	}

	header, claims, signedContent, signature, err := parseJWT(token)
	if err != nil {
		return "", err
	}

	if header.Alg != "RS256" {
		return "", errors.New("unauthorized")
	}

	if claims.Sub == "" || claims.Exp == 0 {
		return "", errors.New("unauthorized")
	}

	if configuredIssuer := strings.TrimSpace(os.Getenv("CLERK_ISSUER")); configuredIssuer != "" && claims.Iss != configuredIssuer {
		return "", errors.New("unauthorized")
	}

	now := time.Now().Unix()
	if claims.Exp < now {
		return "", errors.New("unauthorized")
	}

	if claims.Nbf != 0 && claims.Nbf > now {
		return "", errors.New("unauthorized")
	}

	key, err := publicKeyForToken(header.Kid, claims.Iss)
	if err != nil {
		return "", err
	}

	hashed := sha256.Sum256([]byte(signedContent))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature); err != nil {
		return "", errors.New("unauthorized")
	}

	return claims.Sub, nil
}

func bearerToken(authorization string) (string, error) {
	parts := strings.SplitN(authorization, " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || strings.TrimSpace(parts[1]) == "" {
		return "", errors.New("unauthorized")
	}

	return strings.TrimSpace(parts[1]), nil
}

func parseJWT(token string) (*jwtHeader, *clerkClaims, string, []byte, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	claimsBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	header := &jwtHeader{}
	if err := json.Unmarshal(headerBytes, header); err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	claims := &clerkClaims{}
	if err := json.Unmarshal(claimsBytes, claims); err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	return header, claims, parts[0] + "." + parts[1], signature, nil
}

func publicKeyForToken(kid string, issuer string) (*rsa.PublicKey, error) {
	if pemValue := strings.TrimSpace(os.Getenv("CLERK_JWT_KEY")); pemValue != "" {
		return clerkPEMPublicKey()
	}

	if kid == "" {
		return nil, errors.New("unauthorized")
	}

	document, err := loadJWKS(issuer)
	if err != nil {
		return nil, err
	}

	for _, key := range document.Keys {
		if key.Kid == kid {
			return key.publicKey()
		}
	}

	return nil, errors.New("unauthorized")
}

func loadJWKS(issuer string) (*jwksDocument, error) {
	now := time.Now()

	jwksCache.RLock()
	if jwksCache.document != nil && now.Before(jwksCache.expiresAt) {
		document := jwksCache.document
		jwksCache.RUnlock()
		return document, nil
	}
	jwksCache.RUnlock()

	jwksURL, err := clerkJWKSURL(issuer)
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New("unauthorized")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("unauthorized")
	}

	document := &jwksDocument{}
	if err := json.NewDecoder(resp.Body).Decode(document); err != nil {
		return nil, errors.New("unauthorized")
	}

	jwksCache.Lock()
	jwksCache.document = document
	jwksCache.expiresAt = now.Add(5 * time.Minute)
	jwksCache.Unlock()

	return document, nil
}

func clerkJWKSURL(issuer string) (string, error) {
	if configured := strings.TrimSpace(os.Getenv("CLERK_JWKS_URL")); configured != "" {
		return configured, nil
	}

	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		issuer = strings.TrimSpace(os.Getenv("CLERK_ISSUER"))
	}

	if issuer == "" {
		return "", errors.New("unauthorized")
	}

	return strings.TrimRight(issuer, "/") + "/.well-known/jwks.json", nil
}

func (key jwkKey) publicKey() (*rsa.PublicKey, error) {
	if key.Kty != "RSA" || key.N == "" || key.E == "" {
		return nil, errors.New("unauthorized")
	}

	modulusBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	exponentBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	exponent := 0
	for _, b := range exponentBytes {
		exponent = exponent<<8 + int(b)
	}

	if exponent == 0 {
		return nil, errors.New("unauthorized")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(modulusBytes),
		E: exponent,
	}, nil
}

func clerkPEMPublicKey() (*rsa.PublicKey, error) {
	pemValue := strings.TrimSpace(os.Getenv("CLERK_JWT_KEY"))
	if pemValue == "" {
		return nil, errors.New("unauthorized")
	}

	block, _ := pem.Decode([]byte(pemValue))
	if block == nil {
		return nil, errors.New("unauthorized")
	}

	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	rsaKey, ok := publicKey.(*rsa.PublicKey)
	if !ok {
		return nil, errors.New("unauthorized")
	}

	return rsaKey, nil
}
