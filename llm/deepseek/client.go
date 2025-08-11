package deepseek

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
)

const (
	defaultBaseURL = "https://api.deepseek.com"
	defaultTimeout = 60 * time.Second
	defaultModel   = "deepseek-chat"
)

// Client implements the LLM client interface for DeepSeek
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new DeepSeek client
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

	// Get API key from environment if not provided
	if options.APIKey == "" {
		options.APIKey = os.Getenv("DEEPSEEK_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("DeepSeek API key not provided")
		}
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: options.Timeout,
	}

	return &Client{
		options:    options,
		httpClient: httpClient,
	}, nil
}

// Chat sends a chat request to DeepSeek
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Set default model if not specified
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}

	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Read response body
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		// Try to parse JSON error
		var jsonError map[string]interface{}
		if err := json.Unmarshal(respBody, &jsonError); err == nil {
			if detail, ok := jsonError["detail"].(string); ok {
				return nil, fmt.Errorf("DeepSeek API error: %s", detail)
			}
			if errMsg, ok := jsonError["error"].(map[string]interface{}); ok {
				if msg, ok := errMsg["message"].(string); ok {
					return nil, fmt.Errorf("DeepSeek API error: %s", msg)
				}
			}
		}
		return nil, fmt.Errorf("DeepSeek API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response llm.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatStream is not implemented for DeepSeek yet
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, fmt.Errorf("streaming not implemented for DeepSeek client")
}

// ListModels returns available DeepSeek models
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	// DeepSeek doesn't have a models endpoint, return hardcoded list
	models := []llm.Model{
		{
			ID:          "deepseek-chat",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "deepseek",
			Description: "DeepSeek Chat model optimized for dialogue",
		},
		{
			ID:          "deepseek-coder",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "deepseek",
			Description: "DeepSeek Coder model optimized for code generation",
		},
	}

	return models, nil
}

// GetModel returns details about a specific model
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

// Close cleans up resources
func (c *Client) Close() error {
	return nil
}

// setHeaders sets common headers for requests
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.options.APIKey)
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	// Add custom headers
	for k, v := range c.options.Headers {
		req.Header.Set(k, v)
	}
}
