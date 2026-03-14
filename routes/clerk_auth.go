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

const defaultClerkJWKSURL = "https://api.clerk.com/v1/jwks"

type clerkClaims struct {
	Exp int64  `json:"exp"`
	Iss string `json:"iss"`
	Nbf int64  `json:"nbf"`
	Sts string `json:"sts,omitempty"`
	Sub string `json:"sub"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

type jwksDocument struct {
	Keys []clerkJWK `json:"keys"`
}

type clerkJWK struct {
	Alg string   `json:"alg"`
	E   string   `json:"e"`
	Kid string   `json:"kid"`
	Kty string   `json:"kty"`
	N   string   `json:"n"`
	Use string   `json:"use"`
	X5c []string `json:"x5c,omitempty"`
}

type jwksCache struct {
	mu        sync.RWMutex
	keys      map[string]*rsa.PublicKey
	expiresAt time.Time
}

var clerkKeyCache = &jwksCache{}

func authenticateClerkJWT(r *http.Request) (string, error) {
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return "", errors.New("unauthorized")
	}

	header, claims, signedContent, signature, err := parseJWT(token)
	if err != nil {
		return "", err
	}

	if header.Alg != "RS256" {
		return "", errors.New("unauthorized")
	}
	if claims.Sub == "" || claims.Exp == 0 || strings.EqualFold(claims.Sts, "pending") {
		return "", errors.New("unauthorized")
	}

	now := time.Now().Unix()
	if claims.Exp < now || (claims.Nbf != 0 && claims.Nbf > now) {
		return "", errors.New("unauthorized")
	}

	if configuredIssuer := strings.TrimSpace(os.Getenv("CLERK_ISSUER")); configuredIssuer != "" && claims.Iss != configuredIssuer {
		return "", errors.New("unauthorized")
	}

	key, err := publicKeyForToken(r.Context(), header.Kid, claims.Iss)
	if err != nil {
		return "", err
	}

	hashed := sha256.Sum256([]byte(signedContent))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, hashed[:], signature); err != nil {
		return "", errors.New("unauthorized")
	}

	return claims.Sub, nil
}

func bearerToken(header string) string {
	parts := strings.SplitN(strings.TrimSpace(header), " ", 2)
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		return ""
	}

	return strings.TrimSpace(parts[1])
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

	var header jwtHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	var claims clerkClaims
	if err := json.Unmarshal(claimsBytes, &claims); err != nil {
		return nil, nil, "", nil, errors.New("unauthorized")
	}

	return &header, &claims, parts[0] + "." + parts[1], signature, nil
}

func publicKeyForToken(ctx context.Context, kid, issuer string) (*rsa.PublicKey, error) {
	if pemValue := strings.TrimSpace(os.Getenv("CLERK_JWT_KEY")); pemValue != "" {
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

	if strings.TrimSpace(kid) == "" {
		return nil, errors.New("unauthorized")
	}

	return clerkKeyCache.getKey(ctx, kid, issuer)
}

func (c *jwksCache) getKey(ctx context.Context, keyID, issuer string) (*rsa.PublicKey, error) {
	now := time.Now()

	c.mu.RLock()
	if c.keys != nil && now.Before(c.expiresAt) {
		if key := c.keys[keyID]; key != nil {
			c.mu.RUnlock()
			return key, nil
		}
	}
	c.mu.RUnlock()

	c.mu.Lock()
	defer c.mu.Unlock()

	if c.keys != nil && now.Before(c.expiresAt) {
		if key := c.keys[keyID]; key != nil {
			return key, nil
		}
	}

	keys, err := fetchClerkJWKS(ctx, issuer)
	if err != nil {
		return nil, err
	}

	c.keys = keys
	c.expiresAt = time.Now().Add(15 * time.Minute)

	key := c.keys[keyID]
	if key == nil {
		return nil, errors.New("unauthorized")
	}

	return key, nil
}

func fetchClerkJWKS(ctx context.Context, issuer string) (map[string]*rsa.PublicKey, error) {
	jwksURL, err := clerkJWKSURL(issuer)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	if secret := strings.TrimSpace(os.Getenv("CLERK_SECRET_KEY")); secret != "" {
		req.Header.Set("Authorization", "Bearer "+secret)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.New("unauthorized")
	}
	defer resp.Body.Close()

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, errors.New("unauthorized")
	}

	var document jwksDocument
	if err := json.NewDecoder(resp.Body).Decode(&document); err != nil {
		return nil, errors.New("unauthorized")
	}

	keys := make(map[string]*rsa.PublicKey, len(document.Keys))
	for _, jwk := range document.Keys {
		key, err := parseRSAPublicKey(jwk)
		if err != nil {
			continue
		}
		keys[jwk.Kid] = key
	}

	if len(keys) == 0 {
		return nil, errors.New("unauthorized")
	}

	return keys, nil
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
		return defaultClerkJWKSURL, nil
	}

	return strings.TrimRight(issuer, "/") + "/.well-known/jwks.json", nil
}

func parseRSAPublicKey(jwk clerkJWK) (*rsa.PublicKey, error) {
	if len(jwk.X5c) > 0 {
		der, err := base64.StdEncoding.DecodeString(jwk.X5c[0])
		if err == nil {
			cert, err := x509.ParseCertificate(der)
			if err == nil {
				if key, ok := cert.PublicKey.(*rsa.PublicKey); ok {
					return key, nil
				}
			}
		}
	}

	if jwk.Kty != "RSA" || jwk.N == "" || jwk.E == "" {
		return nil, errors.New("unauthorized")
	}

	modulusBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, errors.New("unauthorized")
	}

	exponentBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
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
