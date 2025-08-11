package openai

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
	defaultBaseURL = "https://api.openai.com/v1"
	defaultTimeout = 60 * time.Second
	defaultModel   = "gpt-4"
)

// Client implements the LLM client interface for OpenAI
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// NewClient creates a new OpenAI client
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
		options.APIKey = os.Getenv("OPENAI_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("OpenAI API key not provided")
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

// Chat sends a chat request to OpenAI
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Set default model if not specified
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}

	// Create the request for OpenAI API
	openAIReq := c.buildOpenAIRequest(request)

	// Create request body
	body, err := json.Marshal(openAIReq)
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
				return fmt.Errorf("OpenAI API error: %s", errResp.Error.Message)
			}
			return fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(respBody))
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

// ChatStream sends a streaming chat request to OpenAI
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Set default model if not specified
	if request.Model == "" {
		request.Model = c.options.DefaultModel
	}

	// Enable streaming
	request.Stream = true

	// Create the request for OpenAI API
	openAIReq := c.buildOpenAIRequest(request)

	// Create request body
	body, err := json.Marshal(openAIReq)
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
		return nil, fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(body))
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

// ListModels returns available OpenAI models
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
		return nil, fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(body))
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
		return nil, fmt.Errorf("OpenAI API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var model llm.Model
	if err := json.NewDecoder(resp.Body).Decode(&model); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	return &model, nil
}

// Close cleans up resources
func (c *Client) Close() error {
	// Nothing to clean up for HTTP client
	return nil
}

// setHeaders sets common headers for requests
func (c *Client) setHeaders(req *http.Request) {
	req.Header.Set("Authorization", "Bearer "+c.options.APIKey)
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	if c.options.Organization != "" {
		req.Header.Set("OpenAI-Organization", c.options.Organization)
	}

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
				strings.Contains(err.Error(), "status 503") { // Service unavailable
				continue
			}
			return err
		}

		return nil
	}

	return fmt.Errorf("max retries exceeded: %w", lastErr)
}

// buildOpenAIRequest creates an OpenAI-specific request from the generic ChatRequest
// It handles model-specific parameter differences for o3 models:
// - Uses max_completion_tokens instead of max_tokens
// - Only supports temperature of 1 (default)
// - Excludes unsupported parameters like top_p, frequency_penalty, and presence_penalty
func (c *Client) buildOpenAIRequest(request *llm.ChatRequest) map[string]interface{} {
	// Create a map from the request
	reqMap := make(map[string]interface{})

	// Always include these fields
	reqMap["model"] = request.Model
	reqMap["messages"] = request.Messages

	// Handle temperature based on model
	modelLower := strings.ToLower(request.Model)
	isO3Model := strings.HasPrefix(modelLower, "o3") || modelLower == "o3-mini"

	if request.Temperature > 0 {
		// O3 models only support temperature of 1
		if isO3Model && request.Temperature != 1.0 {
			// Silently use the default temperature of 1 for o3 models
			// We don't include it in the request since 1 is the default
		} else if !isO3Model {
			// For non-o3 models, include the temperature
			reqMap["temperature"] = request.Temperature
		}
	}

	// O3 models may have restrictions on other parameters too
	if request.TopP > 0 && !isO3Model {
		reqMap["top_p"] = request.TopP
	}
	if request.Stream {
		reqMap["stream"] = request.Stream
	}
	if len(request.Tools) > 0 {
		reqMap["tools"] = request.Tools
	}
	if request.ToolChoice != nil {
		reqMap["tool_choice"] = request.ToolChoice
	}
	if request.ResponseFormat != nil {
		reqMap["response_format"] = request.ResponseFormat
	}

	// O3 models may not support penalty parameters
	if request.FrequencyPenalty > 0 && !isO3Model {
		reqMap["frequency_penalty"] = request.FrequencyPenalty
	}
	if request.PresencePenalty > 0 && !isO3Model {
		reqMap["presence_penalty"] = request.PresencePenalty
	}
	if len(request.Stop) > 0 {
		reqMap["stop"] = request.Stop
	}

	// Handle max_tokens vs max_completion_tokens based on model
	if request.MaxTokens > 0 {
		if isO3Model {
			reqMap["max_completion_tokens"] = request.MaxTokens
		} else {
			reqMap["max_tokens"] = request.MaxTokens
		}
	}

	return reqMap
}
