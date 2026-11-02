package ai

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// APIInvoker calls an OpenAI-compatible chat completions endpoint. It is the
// direct-API transport: the caller owns the agent loop and tools, and this type
// only sends one prompt and returns one answer.
type APIInvoker struct {
	Endpoint  string
	APIKey    string
	Model     string
	System    string
	Headers   map[string]string
	MaxTokens int
	// HTTPClient is optional; nil uses http.DefaultClient.
	HTTPClient *http.Client
}

// APIError is a non-2xx response from the endpoint.
type APIError struct {
	StatusCode int
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("api error (%d)", e.StatusCode)
	}
	return fmt.Sprintf("api error (%d): %s", e.StatusCode, body)
}

// Invoke sends one chat completion request. The request model wins over the
// invoker default, so the picker can override a configured model.
func (a APIInvoker) Invoke(ctx context.Context, r Request) (Result, error) {
	endpoint := strings.TrimRight(a.Endpoint, "/")
	if endpoint == "" {
		return Result{}, errors.New("api invoker: endpoint is empty")
	}
	model := r.Model
	if model == "" {
		model = a.Model
	}
	if model == "" {
		return Result{}, errors.New("api invoker: model is empty")
	}

	messages := make([]apiMessage, 0, 2)
	if a.System != "" {
		messages = append(messages, apiMessage{Role: "system", Content: a.System})
	}
	messages = append(messages, apiMessage{Role: "user", Content: r.Prompt})
	body, _ := json.Marshal(apiRequest{
		Model:     model,
		Messages:  messages,
		MaxTokens: a.MaxTokens,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return Result{}, fmt.Errorf("api invoker: create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if a.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+a.APIKey)
	}
	for key, value := range a.Headers {
		req.Header.Set(key, value)
	}

	client := a.HTTPClient
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, fmt.Errorf("api invoker: request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return Result{}, fmt.Errorf("api invoker: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, &APIError{StatusCode: resp.StatusCode, Body: string(data)}
	}

	var parsed apiResponse
	if err := json.Unmarshal(data, &parsed); err != nil {
		return Result{}, fmt.Errorf("api invoker: parse response: %w", err)
	}
	if parsed.Error != nil {
		return Result{}, fmt.Errorf("api invoker: %s", parsed.Error.Message)
	}
	if len(parsed.Choices) == 0 {
		return Result{}, errors.New("api invoker: empty response")
	}
	outModel := parsed.Model
	if outModel == "" {
		outModel = model
	}
	return Result{
		Output: parsed.Choices[0].Message.Content,
		Model:  outModel,
		Usage: Usage{
			InputTokens:  parsed.Usage.PromptTokens,
			OutputTokens: parsed.Usage.CompletionTokens,
		},
	}, nil
}

type apiMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type apiRequest struct {
	Model     string       `json:"model"`
	Messages  []apiMessage `json:"messages"`
	MaxTokens int          `json:"max_tokens,omitempty"`
}

type apiResponse struct {
	Model   string `json:"model"`
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
	Usage struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
	} `json:"usage"`
	Error *struct {
		Message string `json:"message"`
	} `json:"error"`
}
