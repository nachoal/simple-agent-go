package groq

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
	defaultBaseURL = "https://api.groq.com/openai/v1"
	defaultTimeout = 30 * time.Second // Groq is fast, shorter timeout
	defaultModel   = "mixtral-8x7b-32768"
)

// Client implements the LLM client interface for Groq
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new Groq client
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
		options.APIKey = os.Getenv("GROQ_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("Groq API key not provided")
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

// Chat sends a chat request to Groq
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

	// Execute request with retries
	var response *llm.ChatResponse
	err = c.doWithRetries(ctx, func() error {
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return err
		}
		defer resp.Body.Close()

		// Read response body
		respBody, err := io.ReadAll(resp.Body)
		if err != nil {
			return fmt.Errorf("failed to read response: %w", err)
		}

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Error llm.ErrorResponse `json:"error"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil {
				return fmt.Errorf("Groq API error: %s", errResp.Error.Message)
			}
			return fmt.Errorf("Groq API error: status %d, body: %s", resp.StatusCode, string(respBody))
		}

		// Parse response
		response = &llm.ChatResponse{}
		if err := json.Unmarshal(respBody, response); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		return nil
	})

	return response, err
}

// ChatStream sends a streaming chat request to Groq
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Set default model if not specified
	if request.Model == "" {
		request.Model = c.options.DefaultModel
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
		return nil, fmt.Errorf("Groq API error: status %d, body: %s", resp.StatusCode, string(body))
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

// ListModels returns available Groq models
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
		return nil, fmt.Errorf("Groq API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Data []llm.Model `json:"data"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return response.Data, nil
}

// GetModel returns details about a specific model
func (c *Client) GetModel(ctx context.Context, modelID string) (*llm.Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models/"+modelID, nil)
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
		return nil, fmt.Errorf("Groq API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var model llm.Model
	if err := json.NewDecoder(resp.Body).Decode(&model); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &model, nil
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

// doWithRetries executes a function with retries
func (c *Client) doWithRetries(ctx context.Context, fn func() error) error {
	var lastErr error
	
	for i := 0; i <= c.options.MaxRetries; i++ {
		if i > 0 {
			// Exponential backoff
			delay := time.Duration(i) * time.Second
			select {
			case <-time.After(delay):
			case <-ctx.Done():
				return ctx.Err()
			}
		}

		if err := fn(); err != nil {
			lastErr = err
			// Check if error is retryable
			if strings.Contains(err.Error(), "status 429") || // Rate limit
				strings.Contains(err.Error(), "status 500") || // Server error
				strings.Contains(err.Error(), "status 502") || // Bad gateway
				strings.Contains(err.Error(), "status 503") {  // Service unavailable
				continue
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}