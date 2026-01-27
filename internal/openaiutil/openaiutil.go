package openaiutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

type Config struct {
	APIKey       string
	Model        string
	Endpoint     string
	MaxTokens    int
	Temperature  float64
	SystemPrompt string
}

type request struct {
	Model               string            `json:"model"`
	MaxCompletionTokens int               `json:"max_completion_tokens,omitempty"`
	Temperature         float64           `json:"temperature,omitempty"`
	Messages            []message         `json:"messages"`
	ResponseFormat      map[string]string `json:"response_format,omitempty"`
	ReasoningEffort     string            `json:"reasoning_effort,omitempty"`
	Verbosity           string            `json:"verbosity,omitempty"`
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type response struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}

func Generate(ctx context.Context, cfg Config, userContent string) (string, error) {
	key := strings.TrimSpace(cfg.APIKey)
	if key == "" {
		key = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if key == "" {
		return "", fmt.Errorf("OPENAI_API_KEY missing; set openai.api_key or environment variable")
	}
	model := strings.TrimSpace(cfg.Model)
	if model == "" {
		model = "gpt-5-nano"
	}
	endpoint := strings.TrimSpace(cfg.Endpoint)
	if endpoint == "" {
		endpoint = "https://api.openai.com/v1/chat/completions"
	}
	systemPrompt := strings.TrimSpace(cfg.SystemPrompt)
	if systemPrompt == "" {
		systemPrompt = "You are a careful analyst. Summarize the provided data without inventing facts."
	}

	reqBody := request{
		Model:               model,
		MaxCompletionTokens: cfg.MaxTokens,
		Messages: []message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userContent},
		},
	}

	if strings.HasPrefix(model, "gpt-5") {
		reqBody.ResponseFormat = map[string]string{"type": "text"}
		reqBody.ReasoningEffort = "minimal"
		reqBody.Verbosity = "low"
	} else if cfg.Temperature > 0 {
		reqBody.Temperature = cfg.Temperature
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return "", fmt.Errorf("marshal OpenAI request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", fmt.Errorf("build OpenAI request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("call OpenAI: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read OpenAI response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("OpenAI HTTP %s: %s", resp.Status, string(body))
	}

	var oaResp response
	if err := json.Unmarshal(body, &oaResp); err != nil {
		return "", fmt.Errorf("parse OpenAI response: %w", err)
	}
	if oaResp.Error != nil {
		return "", fmt.Errorf("OpenAI error: %s", oaResp.Error.Message)
	}
	if len(oaResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI response had no choices")
	}
	return strings.TrimSpace(oaResp.Choices[0].Message.Content), nil
}
