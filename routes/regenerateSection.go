package routes

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	regenDefaultRequestyModel = "openai-responses/gpt-5.4"
	requestyChatURL      = "https://router.requesty.ai/v1/chat/completions"
	clerkJWKSPath        = "/.well-known/jwks.json"
	clerkFallbackJWKSURL = "https://api.clerk.com/v1/jwks"
)

var templatePlaceholderPattern = regexp.MustCompile(`{{\s*[^}]+\s*}}`)

type regenerateSectionRequest struct {
	Transcription string           `json:"transcription"`
	Notes         string           `json:"notes"`
	PatientName   string           `json:"patientName"`
	SessionTitle  string           `json:"sessionTitle"`
	SessionID     string           `json:"sessionId"`
	Model         string           `json:"model"`
	Section       *documentSection `json:"section"`
}

type documentSection struct {
	Name                string   `json:"name"`
	Description         string   `json:"description"`
	Examples            []string `json:"examples"`
	AdhereToFormatting  bool     `json:"adhereToFormatting"`
	FormatTemplate      string   `json:"formatTemplate"`
	DoubleSpaceOutput   bool     `json:"doubleSpaceOutput"`
	AllowAssessment     bool     `json:"allowAssessment"`
}

type requestyMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type regenRequestyChatRequest struct {
	Model       string           `json:"model"`
	Messages    []requestyMessage `json:"messages"`
	Temperature float64          `json:"temperature"`
	MaxTokens   int              `json:"max_tokens"`
}

type regenRequestyChatResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type regenClerkJWTHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
}

type regenClerkClaims struct {
	Subject string      `json:"sub"`
	Issuer  string      `json:"iss"`
	AZP     interface{} `json:"azp"`
	EXP     int64       `json:"exp"`
	NBF     int64       `json:"nbf"`
	STS     string      `json:"sts"`
}

type regenClerkJWKS struct {
	Keys []regenClerkJWK `json:"keys"`
}

type regenClerkJWK struct {
	Kid string `json:"kid"`
	Kty string `json:"kty"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func RegenerateSection(w http.ResponseWriter, r *http.Request) {
	if _, err := regenAuthenticateClerkJWT(r); err != nil {
		writeJSONError(w, http.StatusUnauthorized, "Unauthorized")
		return
	}

	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, "Source content and section are required")
		return
	}

	var req regenerateSectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	if !hasSourceContent(req.Transcription, req.Notes) || req.Section == nil || strings.TrimSpace(req.Section.Name) == "" {
		writeJSONError(w, http.StatusBadRequest, "Source content and section are required")
		return
	}

	content, err := requestRegeneratedSection(r.Context(), req)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}

	w.Header().Set(ContentType, JSON)
	json.NewEncoder(w).Encode(map[string]string{
		"content": content,
	})
}

func requestRegeneratedSection(ctx context.Context, req regenerateSectionRequest) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("REQUESTY_API_KEY"))
	if apiKey == "" {
		return "", errors.New("REQUESTY_API_KEY is not configured")
	}

	body := regenRequestyChatRequest{
		Model:       resolveRequestyModel(req.Model),
		Messages:    buildSectionMessages(req),
		Temperature: 0.2,
		MaxTokens:   800,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, requestyChatURL, bytes.NewReader(payload))
	if err != nil {
		return "", err
	}

	httpReq.Header.Set(ContentType, JSON)
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: 45 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return "", fmt.Errorf("requesty returned %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var completion regenRequestyChatResponse
	if err := json.Unmarshal(respBody, &completion); err != nil {
		return "", err
	}

	content := extractRequestyContent(completion)
	if content == "" {
		content = strings.TrimSpace(req.Section.FormatTemplate)
	}

	content = postProcessSectionContent(content, req.Section)
	if content == "" {
		return "", errors.New("requesty returned an empty section")
	}

	return content, nil
}

func buildSectionMessages(req regenerateSectionRequest) []requestyMessage {
	return []requestyMessage{
		{
			Role:    "system",
			Content: buildSectionSystemPrompt(req.Section),
		},
		{
			Role:    "user",
			Content: buildSectionUserPrompt(req),
		},
	}
}

func buildSectionSystemPrompt(section *documentSection) string {
	lines := []string{
		"You generate one section of a medical document from source material.",
		"Return only the section body content. Do not include headings, markdown, code fences, or commentary.",
		"Use only information present in the source transcription or notes.",
		"If information is missing, state that it was not documented instead of inventing details.",
	}

	if section.AllowAssessment {
		lines = append(lines, "Clinical assessment is allowed when it is directly supported by the source.")
	} else {
		lines = append(lines, "Do not add diagnoses, assessment, impression, or plan content unless it is explicitly documented in the source.")
	}

	if section.AdhereToFormatting && strings.TrimSpace(section.FormatTemplate) != "" {
		lines = append(lines,
			"Follow the provided formatting template exactly.",
			"Replace every {{placeholder}} with the best source-supported value or the exact phrase 'Not documented'.",
			"Do not leave unresolved placeholders in the response.",
		)
	}

	if section.DoubleSpaceOutput {
		lines = append(lines, "Use double spacing between paragraphs or list items in the final output.")
	}

	return strings.Join(lines, "\n")
}

func buildSectionUserPrompt(req regenerateSectionRequest) string {
	parts := []string{
		fmt.Sprintf("Section name: %s", strings.TrimSpace(req.Section.Name)),
	}

	if description := strings.TrimSpace(req.Section.Description); description != "" {
		parts = append(parts, fmt.Sprintf("Section description: %s", description))
	}

	if patientName := strings.TrimSpace(req.PatientName); patientName != "" {
		parts = append(parts, fmt.Sprintf("Patient name: %s", patientName))
	}

	if sessionTitle := strings.TrimSpace(req.SessionTitle); sessionTitle != "" {
		parts = append(parts, fmt.Sprintf("Session title: %s", sessionTitle))
	}

	if sessionID := strings.TrimSpace(req.SessionID); sessionID != "" {
		parts = append(parts, fmt.Sprintf("Session ID: %s", sessionID))
	}

	if len(req.Section.Examples) > 0 {
		parts = append(parts, "Examples:")
		for _, example := range req.Section.Examples {
			example = strings.TrimSpace(example)
			if example == "" {
				continue
			}
			parts = append(parts, "- "+example)
		}
	}

	if req.Section.AdhereToFormatting && strings.TrimSpace(req.Section.FormatTemplate) != "" {
		parts = append(parts, "Formatting template:")
		parts = append(parts, req.Section.FormatTemplate)
	}

	if transcription := strings.TrimSpace(req.Transcription); transcription != "" {
		parts = append(parts, "Transcription:")
		parts = append(parts, transcription)
	}

	if notes := strings.TrimSpace(req.Notes); notes != "" {
		parts = append(parts, "Notes:")
		parts = append(parts, notes)
	}

	return strings.Join(parts, "\n\n")
}

func postProcessSectionContent(content string, section *documentSection) string {
	content = strings.TrimSpace(content)
	content = strings.Trim(content, "`")

	if section.AdhereToFormatting && strings.TrimSpace(section.FormatTemplate) != "" {
		content = replaceTemplatePlaceholders(content)
		if content == "" {
			content = replaceTemplatePlaceholders(section.FormatTemplate)
		}
	}

	if section.DoubleSpaceOutput {
		content = applyDoubleSpacing(content)
	}

	return strings.TrimSpace(content)
}

func extractRequestyContent(resp regenRequestyChatResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}

	return stringifyMessageContent(resp.Choices[0].Message.Content)
}

func stringifyMessageContent(content interface{}) string {
	switch v := content.(type) {
	case string:
		return strings.TrimSpace(v)
	case []interface{}:
		var parts []string
		for _, item := range v {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			text, _ := entry["text"].(string)
			if strings.TrimSpace(text) != "" {
				parts = append(parts, strings.TrimSpace(text))
			}
		}
		return strings.TrimSpace(strings.Join(parts, "\n"))
	default:
		return ""
	}
}

func replaceTemplatePlaceholders(content string) string {
	return templatePlaceholderPattern.ReplaceAllStringFunc(content, func(string) string {
		return "Not documented"
	})
}

func applyDoubleSpacing(content string) string {
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")
	var filtered []string
	lastBlank := false
	for _, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if strings.TrimSpace(trimmed) == "" {
			if !lastBlank {
				filtered = append(filtered, "")
			}
			lastBlank = true
			continue
		}

		if len(filtered) > 0 && !lastBlank {
			filtered = append(filtered, "")
		}
		filtered = append(filtered, trimmed)
		lastBlank = false
	}

	return strings.TrimSpace(strings.Join(filtered, "\n"))
}

func resolveRequestyModel(model string) string {
	model = strings.TrimSpace(model)
	if model != "" {
		return model
	}

	model = strings.TrimSpace(os.Getenv("REQUESTY_MODEL"))
	if model != "" {
		return model
	}

	return regenDefaultRequestyModel
}

func hasSourceContent(transcription string, notes string) bool {
	return strings.TrimSpace(transcription) != "" || strings.TrimSpace(notes) != ""
}

func regenAuthenticateClerkJWT(r *http.Request) (string, error) {
	token := regenBearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return "", errors.New("missing bearer token")
	}

	headerSeg, payloadSeg, signatureSeg, err := regenSplitJWT(token)
	if err != nil {
		return "", err
	}

	var header regenClerkJWTHeader
	if err := regenDecodeJWTSegment(headerSeg, &header); err != nil {
		return "", err
	}

	if header.Alg != "RS256" || header.Kid == "" {
		return "", errors.New("invalid clerk jwt header")
	}

	var claims regenClerkClaims
	if err := regenDecodeJWTSegment(payloadSeg, &claims); err != nil {
		return "", err
	}

	if claims.STS == "pending" {
		return "", errors.New("pending clerk session")
	}

	if err := regenValidateJWTTimeClaims(claims); err != nil {
		return "", err
	}

	publicKey, err := regenFetchClerkPublicKey(r.Context(), claims.Issuer, header.Kid)
	if err != nil {
		return "", err
	}

	signed := headerSeg + "." + payloadSeg
	signature, err := base64.RawURLEncoding.DecodeString(signatureSeg)
	if err != nil {
		return "", err
	}

	hash := sha256.Sum256([]byte(signed))
	if err := rsa.VerifyPKCS1v15(publicKey, crypto.SHA256, hash[:], signature); err != nil {
		return "", err
	}

	if strings.TrimSpace(claims.Subject) == "" {
		return "", errors.New("missing clerk subject")
	}

	return claims.Subject, nil
}

func regenBearerToken(header string) string {
	if !strings.HasPrefix(strings.TrimSpace(header), "Bearer ") {
		return ""
	}

	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

func regenSplitJWT(token string) (string, string, string, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return "", "", "", errors.New("invalid jwt structure")
	}
	return parts[0], parts[1], parts[2], nil
}

func regenDecodeJWTSegment(segment string, dest interface{}) error {
	decoded, err := base64.RawURLEncoding.DecodeString(segment)
	if err != nil {
		return err
	}
	return json.Unmarshal(decoded, dest)
}

func regenValidateJWTTimeClaims(claims regenClerkClaims) error {
	now := time.Now().Unix()
	if claims.NBF > 0 && now < claims.NBF {
		return errors.New("jwt not active")
	}
	if claims.EXP > 0 && now >= claims.EXP {
		return errors.New("jwt expired")
	}
	return nil
}

func regenFetchClerkPublicKey(ctx context.Context, issuer string, kid string) (*rsa.PublicKey, error) {
	jwksURL := regenBuildClerkJWKSURL(issuer)
	keys, err := regenFetchClerkJWKS(ctx, jwksURL, "")
	if err != nil && strings.TrimSpace(os.Getenv("CLERK_SECRET_KEY")) != "" {
		keys, err = regenFetchClerkJWKS(ctx, clerkFallbackJWKSURL, os.Getenv("CLERK_SECRET_KEY"))
	}
	if err != nil {
		return nil, err
	}

	for _, key := range keys {
		if key.Kid != kid || key.Kty != "RSA" {
			continue
		}

		return regenJwkToPublicKey(key)
	}

	return nil, errors.New("clerk jwk not found")
}

func regenBuildClerkJWKSURL(issuer string) string {
	issuer = strings.TrimSpace(issuer)
	if issuer == "" {
		return clerkFallbackJWKSURL
	}

	return strings.TrimRight(issuer, "/") + clerkJWKSPath
}

func regenFetchClerkJWKS(ctx context.Context, url string, secret string) ([]regenClerkJWK, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	if strings.TrimSpace(secret) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(secret))
	}

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode >= http.StatusBadRequest {
		return nil, fmt.Errorf("clerk jwks returned %d", resp.StatusCode)
	}

	var jwks regenClerkJWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, err
	}

	return jwks.Keys, nil
}

func regenJwkToPublicKey(jwk regenClerkJWK) (*rsa.PublicKey, error) {
	nb, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, err
	}

	eb, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, err
	}

	exponent := 0
	for _, b := range eb {
		exponent = exponent<<8 + int(b)
	}

	if exponent == 0 {
		return nil, errors.New("invalid rsa exponent")
	}

	return &rsa.PublicKey{
		N: new(big.Int).SetBytes(nb),
		E: exponent,
	}, nil
}
