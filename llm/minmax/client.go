package minmax

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
)

const (
	defaultBaseURL = "https://api.minimax.io/v1"
	defaultTimeout = 60 * time.Second
	defaultModel   = "MiniMax-M2.5"
)

func fallbackModels() []llm.Model {
	now := time.Now().Unix()
	return []llm.Model{
		{
			ID:          "MiniMax-M2.5",
			Object:      "model",
			Created:     now,
			OwnedBy:     "minimax",
			Description: "Peak performance model with strong reasoning and coding capabilities.",
		},
		{
			ID:          "MiniMax-M2.5-lightning",
			Object:      "model",
			Created:     now,
			OwnedBy:     "minimax",
			Description: "Faster M2.5 variant optimized for low latency.",
		},
		{
			ID:          "MiniMax-M2.1",
			Object:      "model",
			Created:     now,
			OwnedBy:     "minimax",
			Description: "Strong multilingual model with robust programming performance.",
		},
		{
			ID:          "MiniMax-M2.1-lightning",
			Object:      "model",
			Created:     now,
			OwnedBy:     "minimax",
			Description: "Faster M2.1 variant for responsive interactive workloads.",
		},
		{
			ID:          "MiniMax-M2",
			Object:      "model",
			Created:     now,
			OwnedBy:     "minimax",
			Description: "General-purpose MiniMax model with agentic and reasoning strengths.",
		},
	}
}

// Client implements the LLM client interface for MiniMax.
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new MiniMax client.
func NewClient(opts ...llm.ClientOption) (*Client, error) {
	options := llm.ClientOptions{
		BaseURL:      defaultBaseURL,
		Timeout:      defaultTimeout,
		MaxRetries:   3,
		DefaultModel: defaultModel,
		Headers:      make(map[string]string),
	}

	// Apply options
	for _, opt := range opts {
		opt(&options)
	}

	// Optional base URL override from env for self-hosted/proxy usage.
	if options.BaseURL == defaultBaseURL {
		if envURL := strings.TrimSpace(os.Getenv("MINIMAX_BASE_URL")); envURL != "" {
			options.BaseURL = envURL
		}
	}

	// Get API key from environment if not provided.
	if options.APIKey == "" {
		options.APIKey = strings.TrimSpace(os.Getenv("MINIMAX_API_KEY"))
		if options.APIKey == "" {
			options.APIKey = strings.TrimSpace(os.Getenv("MINMAX_API_KEY"))
		}
		if options.APIKey == "" {
			return nil, fmt.Errorf("MiniMax API key not provided (set MINIMAX_API_KEY or MINMAX_API_KEY)")
		}
	}

	httpClient := &http.Client{
		Timeout: options.Timeout,
	}

	return &Client{
		options:    options,
		httpClient: httpClient,
	}, nil
}

// Chat sends a chat request to MiniMax.
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errResp struct {
			Error llm.ErrorResponse `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("MiniMax API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("MiniMax API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	var response llm.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatStream sends a streaming chat request to MiniMax.
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}
	request.Stream = true

	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MiniMax API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	events := make(chan llm.StreamEvent)
	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				if data == "[DONE]" {
					return
				}

				var event llm.StreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				select {
				case events <- event:
				case <-ctx.Done():
					return
				}
			}
		}
	}()

	return events, nil
}

// ListModels returns available MiniMax models.
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		// Some MiniMax-compatible gateways may not implement /models.
		return fallbackModels(), nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// MiniMax OpenAI-compatible endpoint can return 404 for /models.
		// Fall back to a curated list so model selection still works.
		if resp.StatusCode == http.StatusNotFound {
			return fallbackModels(), nil
		}
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("MiniMax API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Data []llm.Model `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return fallbackModels(), nil
	}

	if len(response.Data) == 0 {
		return fallbackModels(), nil
	}

	return response.Data, nil
}

// GetModel returns details about a specific model.
func (c *Client) GetModel(ctx context.Context, modelID string) (*llm.Model, error) {
	models, err := c.ListModels(ctx)
	if err != nil {
		return nil, err
	}

	for _, model := range models {
		if model.ID == modelID {
			return &model, nil
		}
	}

	return nil, fmt.Errorf("model not found: %s", modelID)
}

// Close cleans up resources.
func (c *Client) Close() error {
	return nil
}

func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.options.APIKey)
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	for k, v := range c.options.Headers {
		req.Header.Set(k, v)
	}
}
