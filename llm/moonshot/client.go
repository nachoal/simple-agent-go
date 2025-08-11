package moonshot

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
	defaultBaseURL = "https://api.moonshot.ai/v1"
	defaultTimeout = 60 * time.Second
	defaultModel   = "moonshot-v1-8k"
)

// Client implements the LLM client interface for Moonshot/Kimi
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new Moonshot client
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
		options.APIKey = os.Getenv("MOONSHOT_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("Moonshot API key not provided")
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

// Chat sends a chat request to Moonshot
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Set default model if not specified
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}

	// Set default temperature (Moonshot prefers lower temperature)
	if request.Temperature == 0 {
		request.Temperature = 0.3
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
		var errResp struct {
			Error struct {
				Message string `json:"message"`
				Type    string `json:"type"`
			} `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil && errResp.Error.Message != "" {
			return nil, fmt.Errorf("Moonshot API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("Moonshot API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response llm.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatStream is not implemented for Moonshot yet
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, fmt.Errorf("streaming not implemented for Moonshot client")
}

// ListModels returns available Moonshot models
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	// Create request for models endpoint (OpenAI-compatible)
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	req.Header.Set("Authorization", "Bearer "+c.options.APIKey)
	req.Header.Set("Content-Type", "application/json")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Moonshot API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response with custom struct to handle Moonshot's permission format
	var response struct {
		Object string `json:"object"`
		Data   []struct {
			ID         string `json:"id"`
			Object     string `json:"object"`
			Created    int64  `json:"created"`
			OwnedBy    string `json:"owned_by"`
			Permission []struct {
				Created      int    `json:"created"`
				ID           string `json:"id"`
				Object       string `json:"object"`
				Organization string `json:"organization"`
				Group        string `json:"group"`
				IsBlocking   bool   `json:"is_blocking"`
			} `json:"permission"`
			Root   string `json:"root"`
			Parent string `json:"parent"`
		} `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to llm.Model format
	models := make([]llm.Model, 0, len(response.Data))
	for _, m := range response.Data {
		model := llm.Model{
			ID:      m.ID,
			Object:  m.Object,
			Created: m.Created,
			OwnedBy: m.OwnedBy,
		}
		models = append(models, model)
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
