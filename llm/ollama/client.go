package ollama

import (
	"bufio"
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
	defaultBaseURL = "http://localhost:11434"
	defaultTimeout = 120 * time.Second // Longer timeout for local models
	defaultModel   = "llama2"
)

// Client implements the LLM client interface for Ollama
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// OllamaMessage represents a message in Ollama's format
type OllamaMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// OllamaRequest represents a request to Ollama's API
type OllamaRequest struct {
	Model    string          `json:"model"`
	Messages []OllamaMessage `json:"messages"`
	Stream   bool            `json:"stream"`
	Options  map[string]interface{} `json:"options,omitempty"`
}

// OllamaResponse represents a response from Ollama's API
type OllamaResponse struct {
	Model     string         `json:"model"`
	CreatedAt time.Time      `json:"created_at"`
	Message   OllamaMessage  `json:"message"`
	Done      bool           `json:"done"`
	TotalDuration   int64    `json:"total_duration,omitempty"`
	LoadDuration    int64    `json:"load_duration,omitempty"`
	PromptEvalCount int      `json:"prompt_eval_count,omitempty"`
	EvalCount       int      `json:"eval_count,omitempty"`
	EvalDuration    int64    `json:"eval_duration,omitempty"`
}

// OllamaStreamResponse for streaming
type OllamaStreamResponse struct {
	Model     string        `json:"model"`
	CreatedAt time.Time     `json:"created_at"`
	Message   OllamaMessage `json:"message"`
	Done      bool          `json:"done"`
}

// NewClient creates a new Ollama client
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
		if envURL := os.Getenv("OLLAMA_URL"); envURL != "" {
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
		return nil, fmt.Errorf("failed to connect to Ollama at %s: %w", options.BaseURL, err)
	}

	return client, nil
}

// checkConnection verifies the Ollama server is running
func (c *Client) checkConnection(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/api/tags", nil)
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

// Chat sends a chat request to Ollama
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Convert to Ollama format
	ollamaReq := c.convertRequest(request)

	// Create request body
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/api/chat", bytes.NewReader(body))
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
		return nil, fmt.Errorf("Ollama error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse Ollama response
	var ollamaResp OllamaResponse
	if err := json.Unmarshal(respBody, &ollamaResp); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to standard format
	return c.convertResponse(&ollamaResp, request.Model), nil
}

// ChatStream sends a streaming chat request to Ollama
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Convert to Ollama format
	ollamaReq := c.convertRequest(request)
	ollamaReq.Stream = true

	// Create request body
	body, err := json.Marshal(ollamaReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/api/chat", bytes.NewReader(body))
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

	// Check for errors
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Ollama error: status %d, body: %s", resp.StatusCode, string(body))
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

			// Parse JSON response
			var streamResp OllamaStreamResponse
			if err := json.Unmarshal([]byte(line), &streamResp); err != nil {
				continue
			}

			// Convert to standard stream event
			event := llm.StreamEvent{
				ID:      fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
				Object:  "chat.completion.chunk",
				Created: streamResp.CreatedAt.Unix(),
				Model:   streamResp.Model,
				Choices: []llm.Choice{
					{
						Index: 0,
						Delta: &llm.Message{
							Content: llm.StringPtr(streamResp.Message.Content),
						},
					},
				},
			}

			// Set finish reason if done
			if streamResp.Done {
				event.Choices[0].FinishReason = "stop"
			}

			select {
			case events <- event:
			case <-ctx.Done():
				return
			}

			if streamResp.Done {
				return
			}
		}
	}()

	return events, nil
}

// ListModels returns available models in Ollama
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/api/tags", nil)
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
		return nil, fmt.Errorf("Ollama error: status %d, body: %s", resp.StatusCode, string(body))
	}

	var response struct {
		Models []struct {
			Name       string    `json:"name"`
			ModifiedAt time.Time `json:"modified_at"`
			Size       int64     `json:"size"`
			Digest     string    `json:"digest"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Convert to standard model format
	models := make([]llm.Model, len(response.Models))
	for i, model := range response.Models {
		models[i] = llm.Model{
			ID:          model.Name,
			Object:      "model",
			Created:     model.ModifiedAt.Unix(),
			OwnedBy:     "ollama",
			Description: fmt.Sprintf("Local model (%s)", formatBytes(model.Size)),
		}
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
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	// Ollama doesn't require authentication
	// But add custom headers if provided
	for k, v := range c.options.Headers {
		req.Header.Set(k, v)
	}
}

// convertRequest converts from standard format to Ollama format
func (c *Client) convertRequest(req *llm.ChatRequest) *OllamaRequest {
	ollamaReq := &OllamaRequest{
		Model:  req.Model,
		Stream: req.Stream,
		Options: make(map[string]interface{}),
	}

	if ollamaReq.Model == "" {
		ollamaReq.Model = c.options.DefaultModel
	}

	// Convert messages
	for _, msg := range req.Messages {
		role := string(msg.Role)
		// Ollama uses "system", "user", "assistant"
		if msg.Role == llm.RoleTool {
			// Combine tool responses into user messages
			role = "user"
		}

		ollamaReq.Messages = append(ollamaReq.Messages, OllamaMessage{
			Role:    role,
			Content: llm.GetStringValue(msg.Content),
		})
	}

	// Convert parameters to Ollama options
	if req.Temperature > 0 {
		ollamaReq.Options["temperature"] = req.Temperature
	}
	if req.MaxTokens > 0 {
		ollamaReq.Options["num_predict"] = req.MaxTokens
	}
	if req.TopP > 0 {
		ollamaReq.Options["top_p"] = req.TopP
	}

	return ollamaReq
}

// convertResponse converts from Ollama format to standard format
func (c *Client) convertResponse(resp *OllamaResponse, model string) *llm.ChatResponse {
	return &llm.ChatResponse{
		ID:      fmt.Sprintf("ollama-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: resp.CreatedAt.Unix(),
		Model:   model,
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:    llm.RoleAssistant,
					Content: llm.StringPtr(resp.Message.Content),
				},
				FinishReason: "stop",
			},
		},
		Usage: &llm.Usage{
			PromptTokens:     resp.PromptEvalCount,
			CompletionTokens: resp.EvalCount,
			TotalTokens:      resp.PromptEvalCount + resp.EvalCount,
		},
	}
}

// formatBytes formats bytes to human readable string
func formatBytes(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1f GB", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1f MB", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1f KB", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}