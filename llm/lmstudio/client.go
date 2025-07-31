package lmstudio

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
	defaultBaseURL = "http://localhost:1234/v1"
	defaultTimeout = 120 * time.Second // Longer timeout for local models
	defaultModel   = "local-model"
)

// Client implements the LLM client interface for LM Studio
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new LM Studio client
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

	// Check for custom base URL from environment
	if options.BaseURL == defaultBaseURL {
		if envURL := os.Getenv("LM_STUDIO_URL"); envURL != "" {
			options.BaseURL = envURL
		}
	}

	// Create HTTP client
	httpClient := &http.Client{
		Timeout: options.Timeout,
	}

	client := &Client{
		options:    options,
		httpClient: httpClient,
	}

	// Check connection
	if err := client.checkConnection(context.Background()); err != nil {
		return nil, fmt.Errorf("failed to connect to LM Studio at %s: %w", options.BaseURL, err)
	}

	return client, nil
}

// checkConnection verifies the LM Studio server is running
func (c *Client) checkConnection(ctx context.Context) error {
	// Try to list models to check connection
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models", nil)
	if err != nil {
		return err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("server not reachable: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("server returned status %d", resp.StatusCode)
	}

	return nil
}

// Chat sends a chat request to LM Studio
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Set default model if not specified
	if request.Model == "" {
		// Try to get the first available model
		models, err := c.ListModels(ctx)
		if err == nil && len(models) > 0 {
			request.Model = models[0].ID
		} else {
			request.Model = c.options.DefaultModel
		}
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
			Error llm.ErrorResponse `json:"error"`
		}
		if err := json.Unmarshal(respBody, &errResp); err == nil {
			return nil, fmt.Errorf("LM Studio error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("LM Studio error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response
	var response llm.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &response, nil
}

// ChatStream sends a streaming chat request to LM Studio
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Set default model if not specified
	if request.Model == "" {
		// Try to get the first available model
		models, err := c.ListModels(ctx)
		if err == nil && len(models) > 0 {
			request.Model = models[0].ID
		} else {
			request.Model = c.options.DefaultModel
		}
	}

	// Enable streaming
	request.Stream = true

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
	req.Header.Set("Accept", "text/event-stream")

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Create event channel
	events := make(chan llm.StreamEvent)

	// Start goroutine to read stream
	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Parse SSE event
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")
				
				// Check for end of stream
				if data == "[DONE]" {
					return
				}

				// Parse event
				var event llm.StreamEvent
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue // Skip invalid events
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

// ListModels returns available models in LM Studio
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	c.setHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Data []llm.Model `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Add description for local models
	for i := range response.Data {
		response.Data[i].Description = "Local model running in LM Studio"
		if response.Data[i].OwnedBy == "" {
			response.Data[i].OwnedBy = "local"
		}
	}

	return response.Data, nil
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
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	// LM Studio doesn't require authentication
	// But add custom headers if provided
	for k, v := range c.options.Headers {
		req.Header.Set(k, v)
	}
}