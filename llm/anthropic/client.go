package anthropic

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
	defaultBaseURL = "https://api.anthropic.com/v1"
	defaultTimeout = 60 * time.Second
	defaultModel   = "claude-3-opus-20240229"
	apiVersion     = "2023-06-01"
)

// Client implements the LLM client interface for Anthropic
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// AnthropicMessage represents a message in Anthropic's format
type AnthropicMessage struct {
	Role    string      `json:"role"`
	Content interface{} `json:"content"`
}

// AnthropicRequest represents a request to Anthropic's API
type AnthropicRequest struct {
	Model         string             `json:"model"`
	Messages      []AnthropicMessage `json:"messages"`
	MaxTokens     int                `json:"max_tokens"`
	Temperature   float32            `json:"temperature,omitempty"`
	TopP          float32            `json:"top_p,omitempty"`
	TopK          int                `json:"top_k,omitempty"`
	Stream        bool               `json:"stream,omitempty"`
	System        string             `json:"system,omitempty"`
	Tools         []AnthropicTool    `json:"tools,omitempty"`
	ToolChoice    interface{}        `json:"tool_choice,omitempty"`
	StopSequences []string           `json:"stop_sequences,omitempty"`
}

// AnthropicTool represents a tool in Anthropic's format
type AnthropicTool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"input_schema"`
}

// AnthropicResponse represents a response from Anthropic's API
type AnthropicResponse struct {
	ID           string                  `json:"id"`
	Type         string                  `json:"type"`
	Role         string                  `json:"role"`
	Content      []AnthropicContentBlock `json:"content"`
	Model        string                  `json:"model"`
	StopReason   string                  `json:"stop_reason"`
	StopSequence string                  `json:"stop_sequence,omitempty"`
	Usage        AnthropicUsage          `json:"usage"`
}

// AnthropicContentBlock represents a content block in the response
type AnthropicContentBlock struct {
	Type    string          `json:"type"`
	Text    string          `json:"text,omitempty"`
	ID      string          `json:"id,omitempty"`
	Name    string          `json:"name,omitempty"`
	Input   json.RawMessage `json:"input,omitempty"`
	ToolUse string          `json:"tool_use_id,omitempty"`
	Content string          `json:"content,omitempty"`
}

// AnthropicUsage represents token usage
type AnthropicUsage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

// NewClient creates a new Anthropic client
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
		options.APIKey = os.Getenv("ANTHROPIC_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("Anthropic API key not provided")
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

// Chat sends a chat request to Anthropic
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Convert to Anthropic format
	anthropicReq := c.convertRequest(request)

	// Create request body
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Debug logging
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "\n[Anthropic] Request URL: %s/messages\n", c.options.BaseURL)
		fmt.Fprintf(os.Stderr, "[Anthropic] Request Body:\n%s\n", string(body))
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/messages", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.setHeaders(req)
	req.Header.Set("Content-Type", "application/json")

	// Execute request with retries
	var anthropicResp AnthropicResponse
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

		// Debug logging
		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[Anthropic] Response Status: %d\n", resp.StatusCode)
			fmt.Fprintf(os.Stderr, "[Anthropic] Response Body:\n%s\n", string(respBody))
		}

		// Check for errors
		if resp.StatusCode != http.StatusOK {
			var errResp struct {
				Error struct {
					Type    string `json:"type"`
					Message string `json:"message"`
				} `json:"error"`
			}
			if err := json.Unmarshal(respBody, &errResp); err == nil {
				return fmt.Errorf("Anthropic API error: %s", errResp.Error.Message)
			}
			return fmt.Errorf("Anthropic API error: status %d, body: %s", resp.StatusCode, string(respBody))
		}

		// Parse response
		if err := json.Unmarshal(respBody, &anthropicResp); err != nil {
			return fmt.Errorf("failed to parse response: %w", err)
		}

		return nil
	})

	if err != nil {
		return nil, err
	}

	// Convert to standard format
	response := c.convertResponse(&anthropicResp)

	// Debug log parsed response
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		if len(response.Choices) > 0 && len(response.Choices[0].Message.ToolCalls) > 0 {
			fmt.Fprintf(os.Stderr, "[Anthropic] Parsed %d tool calls\n", len(response.Choices[0].Message.ToolCalls))
			for i, tc := range response.Choices[0].Message.ToolCalls {
				fmt.Fprintf(os.Stderr, "[Anthropic] Tool Call %d: %s with args: %s\n", i, tc.Function.Name, string(tc.Function.Arguments))
			}
		} else {
			fmt.Fprintf(os.Stderr, "[Anthropic] No tool calls in response\n")
		}
	}

	return response, nil
}

// ChatStream sends a streaming chat request to Anthropic
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	// Convert to Anthropic format
	anthropicReq := c.convertRequest(request)
	anthropicReq.Stream = true

	// Create request body
	body, err := json.Marshal(anthropicReq)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Create HTTP request
	req, err := http.NewRequestWithContext(ctx, "POST", c.options.BaseURL+"/messages", bytes.NewReader(body))
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
		return nil, fmt.Errorf("Anthropic API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Create event channel
	events := make(chan llm.StreamEvent)

	// Start goroutine to read stream
	go func() {
		defer close(events)
		defer resp.Body.Close()

		scanner := bufio.NewScanner(resp.Body)
		var currentMessage strings.Builder

		for scanner.Scan() {
			line := scanner.Text()

			// Skip empty lines
			if line == "" {
				continue
			}

			// Parse SSE event
			if strings.HasPrefix(line, "data: ") {
				data := strings.TrimPrefix(line, "data: ")

				var event map[string]interface{}
				if err := json.Unmarshal([]byte(data), &event); err != nil {
					continue
				}

				// Convert Anthropic stream event to standard format
				if event["type"] == "content_block_delta" {
					delta, _ := event["delta"].(map[string]interface{})
					if text, ok := delta["text"].(string); ok {
						currentMessage.WriteString(text)

						streamEvent := llm.StreamEvent{
							ID:      event["id"].(string),
							Object:  "chat.completion.chunk",
							Created: time.Now().Unix(),
							Model:   anthropicReq.Model,
							Choices: []llm.Choice{
								{
									Index: 0,
									Delta: &llm.Message{
										Content: llm.StringPtr(text),
									},
								},
							},
						}

						select {
						case events <- streamEvent:
						case <-ctx.Done():
							return
						}
					}
				} else if event["type"] == "message_stop" {
					// Send final event with finish reason
					streamEvent := llm.StreamEvent{
						ID:      event["id"].(string),
						Object:  "chat.completion.chunk",
						Created: time.Now().Unix(),
						Model:   anthropicReq.Model,
						Choices: []llm.Choice{
							{
								Index:        0,
								FinishReason: "stop",
							},
						},
					}

					select {
					case events <- streamEvent:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()

	return events, nil
}

// ListModels returns available Anthropic models
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	// Create request for models endpoint
	req, err := http.NewRequestWithContext(ctx, "GET", c.options.BaseURL+"/models", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	// Set headers
	c.setHeaders(req)

	// Execute request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to execute request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Anthropic API error: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var response struct {
		Data []struct {
			ID          string `json:"id"`
			Type        string `json:"type"`
			DisplayName string `json:"display_name"`
			CreatedAt   string `json:"created_at"`
		} `json:"data"`
		HasMore bool   `json:"has_more"`
		FirstID string `json:"first_id"`
		LastID  string `json:"last_id"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	// Convert to llm.Model format
	models := make([]llm.Model, 0, len(response.Data))
	for _, m := range response.Data {
		model := llm.Model{
			ID:          m.ID,
			Object:      "model",
			OwnedBy:     "anthropic",
			Description: m.DisplayName,
		}
		// Parse created_at timestamp if needed
		if t, err := time.Parse(time.RFC3339, m.CreatedAt); err == nil {
			model.Created = t.Unix()
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
	req.Header.Set("x-api-key", c.options.APIKey)
	req.Header.Set("anthropic-version", apiVersion)
	req.Header.Set("User-Agent", "simple-agent-go/1.0")

	// Add custom headers
	for k, v := range c.options.Headers {
		req.Header.Set(k, v)
	}
}

// convertRequest converts from standard format to Anthropic format
func (c *Client) convertRequest(req *llm.ChatRequest) *AnthropicRequest {
	anthropicReq := &AnthropicRequest{
		Model:       req.Model,
		MaxTokens:   req.MaxTokens,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		Stream:      req.Stream,
	}

	if anthropicReq.Model == "" {
		anthropicReq.Model = c.options.DefaultModel
	}

	if anthropicReq.MaxTokens == 0 {
		anthropicReq.MaxTokens = 4096
	}

	// Convert messages
	var messages []AnthropicMessage
	var systemMessage string

	for _, msg := range req.Messages {
		switch msg.Role {
		case llm.RoleSystem:
			systemMessage = llm.GetStringValue(msg.Content)
		case llm.RoleUser:
			messages = append(messages, AnthropicMessage{
				Role:    "user",
				Content: llm.GetStringValue(msg.Content),
			})
		case llm.RoleAssistant:
			// Handle tool calls
			if len(msg.ToolCalls) > 0 {
				var content []AnthropicContentBlock

				// Add text if present
				if msg.Content != nil && *msg.Content != "" {
					content = append(content, AnthropicContentBlock{
						Type: "text",
						Text: *msg.Content,
					})
				}

				// Add tool calls
				for _, toolCall := range msg.ToolCalls {
					content = append(content, AnthropicContentBlock{
						Type:  "tool_use",
						ID:    toolCall.ID,
						Name:  toolCall.Function.Name,
						Input: toolCall.Function.Arguments,
					})
				}

				messages = append(messages, AnthropicMessage{
					Role:    "assistant",
					Content: content,
				})
			} else {
				messages = append(messages, AnthropicMessage{
					Role:    "assistant",
					Content: llm.GetStringValue(msg.Content),
				})
			}
		case llm.RoleTool:
			// Tool responses
			messages = append(messages, AnthropicMessage{
				Role: "user",
				Content: []AnthropicContentBlock{
					{
						Type:    "tool_result",
						ToolUse: msg.ToolCallID,
						Content: *msg.Content,
					},
				},
			})
		}
	}

	anthropicReq.Messages = messages
	if systemMessage != "" {
		anthropicReq.System = systemMessage
	}

	// Convert tools
	if len(req.Tools) > 0 {
		var tools []AnthropicTool
		for _, tool := range req.Tools {
			if fn, ok := tool["function"].(map[string]interface{}); ok {
				tools = append(tools, AnthropicTool{
					Name:        fn["name"].(string),
					Description: fn["description"].(string),
					InputSchema: fn["parameters"].(map[string]interface{}),
				})
			}
		}
		anthropicReq.Tools = tools
	}

	return anthropicReq
}

// convertResponse converts from Anthropic format to standard format
func (c *Client) convertResponse(resp *AnthropicResponse) *llm.ChatResponse {
	// Build message content and tool calls
	var content strings.Builder
	var toolCalls []llm.ToolCall

	for _, block := range resp.Content {
		switch block.Type {
		case "text":
			content.WriteString(block.Text)
		case "tool_use":
			toolCalls = append(toolCalls, llm.ToolCall{
				ID:   block.ID,
				Type: "function",
				Function: llm.FunctionCall{
					Name:      block.Name,
					Arguments: block.Input,
				},
			})
		}
	}

	// Determine finish reason
	finishReason := "stop"
	if resp.StopReason == "tool_use" {
		finishReason = "tool_calls"
	} else if resp.StopReason == "max_tokens" {
		finishReason = "length"
	}

	return &llm.ChatResponse{
		ID:      resp.ID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   resp.Model,
		Choices: []llm.Choice{
			{
				Index: 0,
				Message: llm.Message{
					Role:      llm.RoleAssistant,
					Content:   llm.StringPtr(content.String()),
					ToolCalls: toolCalls,
				},
				FinishReason: finishReason,
			},
		},
		Usage: &llm.Usage{
			PromptTokens:     resp.Usage.InputTokens,
			CompletionTokens: resp.Usage.OutputTokens,
			TotalTokens:      resp.Usage.InputTokens + resp.Usage.OutputTokens,
		},
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
