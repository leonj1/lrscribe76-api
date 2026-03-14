package routes

const defaultRequestyModel = "openai-responses/gpt-5.4"

type requestyChatRequest struct {
	Model          string                `json:"model"`
	Messages       []requestyChatMessage `json:"messages"`
	ResponseFormat map[string]string     `json:"response_format,omitempty"`
	Temperature    float64               `json:"temperature,omitempty"`
	MaxTokens      int                   `json:"max_tokens,omitempty"`
}

type requestyChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type requestyChatResponse struct {
	Choices []struct {
		Message struct {
			Content interface{} `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error,omitempty"`
}
