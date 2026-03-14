package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	requestyBaseURL    = "https://router.requesty.ai/v1/chat/completions"
	notDocumentedValue = "Not documented"
)

var placeholderPattern = regexp.MustCompile(`\{\{\s*[^}]+\s*\}\}`)

type generateDocumentRequest struct {
	Transcription    string            `json:"transcription"`
	Notes            string            `json:"notes"`
	PatientName      string            `json:"patientName"`
	SessionTitle     string            `json:"sessionTitle"`
	SessionID        string            `json:"sessionId"`
	Model            string            `json:"model"`
	TemplateSections []templateSection `json:"templateSections"`
}

type templateSection struct {
	Name               string   `json:"name"`
	Description        string   `json:"description"`
	Order              int      `json:"order"`
	Examples           []string `json:"examples"`
	AdhereToFormatting bool     `json:"adhereToFormatting"`
	FormatTemplate     string   `json:"formatTemplate"`
	DoubleSpaceOutput  bool     `json:"doubleSpaceOutput"`
	AllowAssessment    bool     `json:"allowAssessment"`
}

type generateDocumentResponse struct {
	Document            string `json:"document"`
	DocumentGeneratedAt int64  `json:"documentGeneratedAt"`
	ModelUsed           string `json:"modelUsed"`
}

type sectionGenerationResponse struct {
	Sections []generatedSection `json:"sections"`
}

type generatedSection struct {
	Name    string `json:"name"`
	Content string `json:"content"`
}

func GenerateDocument(w http.ResponseWriter, r *http.Request) {
	if _, err := authenticateClerkJWT(r); err != nil {
		writeUnauthorized(w)
		return
	}

	if r.Body == nil {
		writeJSONError(w, http.StatusBadRequest, "Request body is required")
		return
	}

	var payload generateDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, "Invalid JSON body")
		return
	}

	if err := validateGenerateDocumentRequest(payload); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	model := strings.TrimSpace(payload.Model)
	if model == "" {
		model = defaultRequestyModel
	}

	orderedSections := append([]templateSection(nil), payload.TemplateSections...)
	sort.SliceStable(orderedSections, func(i, j int) bool {
		if orderedSections[i].Order == orderedSections[j].Order {
			return orderedSections[i].Name < orderedSections[j].Name
		}
		return orderedSections[i].Order < orderedSections[j].Order
	})

	var placeholderSections []templateSection
	var narrativeSections []templateSection
	for _, section := range orderedSections {
		if isPlaceholderSection(section) {
			placeholderSections = append(placeholderSections, section)
			continue
		}
		narrativeSections = append(narrativeSections, section)
	}

	sectionContent := make(map[string]string, len(orderedSections))
	if len(narrativeSections) > 0 {
		generated, err := generateSectionsWithLLM(r.Context(), payload, narrativeSections, model, false)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for name, content := range generated {
			sectionContent[name] = content
		}
	}

	if len(placeholderSections) > 0 {
		generated, err := generateSectionsWithLLM(r.Context(), payload, placeholderSections, model, true)
		if err != nil {
			writeJSONError(w, http.StatusInternalServerError, err.Error())
			return
		}
		for name, content := range generated {
			sectionContent[name] = content
		}
	}

	document := buildDocument(orderedSections, sectionContent)
	response := generateDocumentResponse{
		Document:            document,
		DocumentGeneratedAt: time.Now().UnixNano() / int64(time.Millisecond),
		ModelUsed:           model,
	}

	writeJSON(w, http.StatusOK, response)
}

func validateGenerateDocumentRequest(payload generateDocumentRequest) error {
	if strings.TrimSpace(payload.SessionID) == "" {
		return errors.New("sessionId is required")
	}

	if strings.TrimSpace(payload.Transcription) == "" && strings.TrimSpace(payload.Notes) == "" {
		return errors.New("At least one of transcription or notes, plus template sections, are required")
	}

	if len(payload.TemplateSections) == 0 {
		return errors.New("At least one of transcription or notes, plus template sections, are required")
	}

	for i, section := range payload.TemplateSections {
		if strings.TrimSpace(section.Name) == "" {
			return fmt.Errorf("templateSections[%d].name is required", i)
		}
		if section.AdhereToFormatting && strings.TrimSpace(section.FormatTemplate) == "" {
			return fmt.Errorf("templateSections[%d].formatTemplate is required when adhereToFormatting is true", i)
		}
	}

	return nil
}

func generateSectionsWithLLM(ctx context.Context, payload generateDocumentRequest, sections []templateSection, model string, placeholders bool) (map[string]string, error) {
	if len(sections) == 0 {
		return map[string]string{}, nil
	}

	systemPrompt := buildSystemPrompt(placeholders)
	userPrompt := buildUserPrompt(payload, sections, placeholders)

	rawContent, err := callRequestyChatCompletion(ctx, model, systemPrompt, userPrompt)
	if err != nil {
		return nil, err
	}

	var parsed sectionGenerationResponse
	if err := json.Unmarshal([]byte(rawContent), &parsed); err != nil {
		return nil, fmt.Errorf("failed to parse LLM response JSON: %w", err)
	}

	result := make(map[string]string, len(sections))
	for _, section := range sections {
		result[section.Name] = ""
	}

	for _, generated := range parsed.Sections {
		name := strings.TrimSpace(generated.Name)
		if name == "" {
			continue
		}
		result[name] = normalizeSectionOutput(generated.Content, sectionByName(sections, name))
	}

	for _, section := range sections {
		if strings.TrimSpace(result[section.Name]) == "" {
			if placeholders && section.AdhereToFormatting {
				result[section.Name] = normalizeSectionOutput(fillTemplateFallback(section.FormatTemplate), section)
				continue
			}
			result[section.Name] = notDocumentedValue
		}
	}

	return result, nil
}

func buildSystemPrompt(placeholders bool) string {
	if placeholders {
		return strings.Join([]string{
			"You generate medical document sections as strict JSON.",
			"Return only valid JSON with the shape {\"sections\":[{\"name\":\"Section Name\",\"content\":\"...\"}]}.",
			"These sections use formatting templates with placeholders. Preserve the template structure exactly and replace each placeholder with the best grounded value from the source material.",
			"Use \"" + notDocumentedValue + "\" when a placeholder value is unavailable.",
			"Do not include markdown fences or commentary.",
		}, "\n")
	}

	return strings.Join([]string{
		"You generate medical document sections as strict JSON.",
		"Return only valid JSON with the shape {\"sections\":[{\"name\":\"Section Name\",\"content\":\"...\"}]}.",
		"Ground every section in the supplied transcription and notes.",
		"Do not hallucinate facts. If information is unavailable, say \"" + notDocumentedValue + "\".",
		"Do not include markdown fences or commentary.",
	}, "\n")
}

func buildUserPrompt(payload generateDocumentRequest, sections []templateSection, placeholders bool) string {
	var builder strings.Builder
	builder.WriteString("Generate the requested medical document sections.\n")
	builder.WriteString("Patient Name: ")
	builder.WriteString(orFallback(payload.PatientName, "Unknown"))
	builder.WriteString("\nSession Title: ")
	builder.WriteString(orFallback(payload.SessionTitle, "Untitled Session"))
	builder.WriteString("\nSession ID: ")
	builder.WriteString(payload.SessionID)
	builder.WriteString("\n\nTranscription:\n")
	builder.WriteString(orFallback(strings.TrimSpace(payload.Transcription), notDocumentedValue))
	builder.WriteString("\n\nNotes:\n")
	builder.WriteString(orFallback(strings.TrimSpace(payload.Notes), notDocumentedValue))
	builder.WriteString("\n\nSections:\n")

	for _, section := range sections {
		builder.WriteString("- Name: ")
		builder.WriteString(section.Name)
		builder.WriteString("\n  Description: ")
		builder.WriteString(orFallback(section.Description, notDocumentedValue))
		builder.WriteString("\n  Allow Assessment: ")
		builder.WriteString(boolText(section.AllowAssessment))
		builder.WriteString("\n  Double Space Output: ")
		builder.WriteString(boolText(section.DoubleSpaceOutput))
		if placeholders {
			builder.WriteString("\n  Format Template:\n")
			builder.WriteString(section.FormatTemplate)
		}
		if len(section.Examples) > 0 {
			builder.WriteString("\n  Examples:\n")
			for _, example := range section.Examples {
				builder.WriteString("  - ")
				builder.WriteString(example)
				builder.WriteString("\n")
			}
		} else {
			builder.WriteString("\n  Examples: none\n")
		}
		if !section.AllowAssessment {
			builder.WriteString("  Constraint: do not add diagnosis, impression, or unsupported clinical assessment.\n")
		}
		if placeholders {
			builder.WriteString("  Constraint: keep the output aligned to the provided format template.\n")
		}
	}

	return builder.String()
}

func callRequestyChatCompletion(ctx context.Context, model, systemPrompt, userPrompt string) (string, error) {
	apiKey := strings.TrimSpace(os.Getenv("REQUESTY_API_KEY"))
	if apiKey == "" {
		return "", errors.New("REQUESTY_API_KEY is required")
	}

	body := requestyChatRequest{
		Model: model,
		Messages: []requestyChatMessage{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: map[string]string{"type": "json_object"},
		Temperature:    0.2,
	}

	payload, err := json.Marshal(body)
	if err != nil {
		return "", fmt.Errorf("failed to encode LLM request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, requestyBaseURL, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("failed to build LLM request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set(ContentType, JSON)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("requesty request failed: %w", err)
	}
	defer resp.Body.Close()

	var parsed requestyChatResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return "", fmt.Errorf("failed to decode LLM response: %w", err)
	}

	if resp.StatusCode >= 400 {
		if parsed.Error != nil && parsed.Error.Message != "" {
			return "", fmt.Errorf("requesty error: %s", parsed.Error.Message)
		}
		return "", fmt.Errorf("requesty error: status %d", resp.StatusCode)
	}

	if len(parsed.Choices) == 0 {
		return "", errors.New("requesty returned no choices")
	}

	content := strings.TrimSpace(extractRequestyContent(parsed))
	if content == "" {
		return "", errors.New("requesty returned empty content")
	}

	return content, nil
}

func buildDocument(sections []templateSection, content map[string]string) string {
	parts := make([]string, 0, len(sections))
	for _, section := range sections {
		body := strings.TrimSpace(content[section.Name])
		if body == "" {
			body = notDocumentedValue
		}
		parts = append(parts, fmt.Sprintf("## %s\n\n%s", section.Name, body))
	}
	return strings.Join(parts, "\n\n")
}

func normalizeSectionOutput(content string, section templateSection) string {
	content = strings.TrimSpace(content)
	if content == "" {
		return ""
	}
	if section.DoubleSpaceOutput {
		lines := strings.Split(content, "\n")
		return strings.Join(lines, "\n\n")
	}
	return content
}

func sectionByName(sections []templateSection, name string) templateSection {
	for _, section := range sections {
		if section.Name == name {
			return section
		}
	}
	return templateSection{Name: name}
}

func isPlaceholderSection(section templateSection) bool {
	return section.AdhereToFormatting && strings.TrimSpace(section.FormatTemplate) != ""
}

func orFallback(value, fallback string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fallback
	}
	return trimmed
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func fillTemplateFallback(formatTemplate string) string {
	return placeholderPattern.ReplaceAllString(formatTemplate, notDocumentedValue)
}
