package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

const (
	requestyChatURL = "https://router.requesty.ai/v1/chat/completions"
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
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Examples           []string `json:"examples"`
	AdhereToFormatting bool     `json:"adhereToFormatting"`
	FormatTemplate     string   `json:"formatTemplate"`
	DoubleSpaceOutput  bool     `json:"doubleSpaceOutput"`
	AllowAssessment    bool     `json:"allowAssessment"`
}

func RegenerateSection(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticateClerkJWT(r); err != nil {
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

	body := requestyChatRequest{
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

	var completion requestyChatResponse
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

func buildSectionMessages(req regenerateSectionRequest) []requestyChatMessage {
	return []requestyChatMessage{
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

func extractRequestyContent(resp requestyChatResponse) string {
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

	return defaultRequestyModel
}

func hasSourceContent(transcription string, notes string) bool {
	return strings.TrimSpace(transcription) != "" || strings.TrimSpace(notes) != ""
}
