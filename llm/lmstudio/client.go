package lmstudio

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
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
		request.Model = c.options.DefaultModel
	}

	// Create request body
	body, err := json.Marshal(request)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug logging
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "\n[LM Studio] Request URL: %s/chat/completions\n", c.options.BaseURL)
		fmt.Fprintf(os.Stderr, "[LM Studio] Request Body:\n%s\n", string(body))
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

	// Debug logging
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "[LM Studio] Response Status: %d\n", resp.StatusCode)
		fmt.Fprintf(os.Stderr, "[LM Studio] Response Body:\n%s\n", string(respBody))
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

	// Debug log parsed response
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		if len(response.Choices) > 0 && len(response.Choices[0].Message.ToolCalls) > 0 {
			fmt.Fprintf(os.Stderr, "[LM Studio] Parsed %d tool calls\n", len(response.Choices[0].Message.ToolCalls))
			for i, tc := range response.Choices[0].Message.ToolCalls {
				fmt.Fprintf(os.Stderr, "[LM Studio] Tool Call %d: %s with args: %s\n", i, tc.Function.Name, string(tc.Function.Arguments))
			}
		} else {
			fmt.Fprintf(os.Stderr, "[LM Studio] No tool calls in response\n")
		}
	}

	return &response, nil
}

// ChatStream sends a streaming chat request to LM Studio
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

	// Set OwnedBy and vision flag for known vision-capable IDs
	for i := range response.Data {
		if response.Data[i].OwnedBy == "" {
			response.Data[i].OwnedBy = "local"
		}
		if isLMStudioVisionModel(response.Data[i].ID) {
			response.Data[i].SupportsVision = true
			if !strings.Contains(strings.ToLower(response.Data[i].Description), "vision") {
				if response.Data[i].Description == "" {
					response.Data[i].Description = "Vision-capable"
				} else {
					response.Data[i].Description = response.Data[i].Description + " · Vision"
				}
			}
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

// isLMStudioVisionModel marks common LM Studio vision models by ID
func isLMStudioVisionModel(id string) bool {
	n := strings.ToLower(id)
	switch {
	case strings.Contains(n, "gemma-3"), // Google Gemma 3 vision
		strings.Contains(n, "pixtral"), // Mistral Pixtral
		strings.Contains(n, "llava"),
		strings.Contains(n, "bakllava"),
		strings.Contains(n, "moondream"),
		strings.Contains(n, "-vision"):
		return true
	default:
		return false
	}
}

// --- Multimodal helpers (OpenAI-compatible content array) ---

type lmContentPart struct {
	Type     string      `json:"type"`
	Text     string      `json:"text,omitempty"`
	ImageURL *lmImageURL `json:"image_url,omitempty"`
}

type lmImageURL struct {
	URL string `json:"url"`
}

type lmMessage struct {
	Role    string          `json:"role"`
	Content []lmContentPart `json:"content"`
}

type lmChatReq struct {
	Model       string      `json:"model"`
	Messages    []lmMessage `json:"messages"`
	MaxTokens   int         `json:"max_tokens,omitempty"`
	Temperature float64     `json:"temperature,omitempty"`
	Stream      bool        `json:"stream,omitempty"`
}

// encodeImageToDataURL converts an image to data URL format
func (c *Client) encodeImageToDataURL(imagePath string) (string, error) {
	data, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("read image: %w", err)
	}
	mime := "image/jpeg"
	if strings.HasSuffix(strings.ToLower(imagePath), ".png") {
		mime = "image/png"
	} else if strings.HasSuffix(strings.ToLower(imagePath), ".gif") {
		mime = "image/gif"
	} else if strings.HasSuffix(strings.ToLower(imagePath), ".webp") {
		mime = "image/webp"
	}
	b64 := base64.StdEncoding.EncodeToString(data)
	return fmt.Sprintf("data:%s;base64,%s", mime, b64), nil
}

// ChatWithImages sends a prompt + images using LM Studio's OpenAI-compatible API
func (c *Client) ChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (string, error) {
	// Build content array
	parts := []lmContentPart{{Type: "text", Text: prompt}}
	for _, p := range imagePaths {
		var url string
		if strings.HasPrefix(strings.ToLower(p), "data:image/") {
			url = p
		} else {
			var err error
			url, err = c.encodeImageToDataURL(p)
			if err != nil {
				return "", err
			}
		}
		parts = append(parts, lmContentPart{Type: "image_url", ImageURL: &lmImageURL{URL: url}})
	}

	req := lmChatReq{
		Model:    c.options.DefaultModel,
		Messages: []lmMessage{{Role: "user", Content: parts}},
	}
	// Lightweight handling of common opts
	if v, ok := opts["max_tokens"].(int); ok {
		req.MaxTokens = v
	}
	if v, ok := opts["temperature"].(float64); ok {
		req.Temperature = v
	}

	body, err := json.Marshal(req)
	if err != nil {
		return "", err
	}

	httpReq, err := http.NewRequest("POST", c.options.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("LM Studio error: %s", string(b))
	}

	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if len(out.Choices) > 0 {
		return out.Choices[0].Message.Content, nil
	}
	return "", nil
}

// StreamChatWithImages streams chunks for prompt + images
func (c *Client) StreamChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (<-chan string, error) {
	// Build content array
	parts := []lmContentPart{{Type: "text", Text: prompt}}
	for _, p := range imagePaths {
		var url string
		if strings.HasPrefix(strings.ToLower(p), "data:image/") {
			url = p
		} else {
			var err error
			url, err = c.encodeImageToDataURL(p)
			if err != nil {
				return nil, err
			}
		}
		parts = append(parts, lmContentPart{Type: "image_url", ImageURL: &lmImageURL{URL: url}})
	}

	req := lmChatReq{
		Model:    c.options.DefaultModel,
		Messages: []lmMessage{{Role: "user", Content: parts}},
		Stream:   true,
	}
	if v, ok := opts["max_tokens"].(int); ok {
		req.MaxTokens = v
	}
	if v, ok := opts["temperature"].(float64); ok {
		req.Temperature = v
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}

	httpReq, err := http.NewRequest("POST", c.options.BaseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	c.setHeaders(httpReq)
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("LM Studio error: %s", string(b))
	}

	ch := make(chan string)
	go func() {
		defer close(ch)
		defer resp.Body.Close()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()
			if line == "" {
				continue
			}
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			data := strings.TrimPrefix(line, "data: ")
			if data == "[DONE]" {
				return
			}
			var event struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
			}
			if err := json.Unmarshal([]byte(data), &event); err != nil {
				continue
			}
			if len(event.Choices) > 0 && event.Choices[0].Delta.Content != "" {
				ch <- event.Choices[0].Delta.Content
			}
		}
	}()
	return ch, nil
}
