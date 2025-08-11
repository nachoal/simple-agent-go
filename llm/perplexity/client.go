package perplexity

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
	defaultBaseURL = "https://api.perplexity.ai"
	defaultTimeout = 60 * time.Second
	defaultModel   = "llama-3.1-sonar-huge-128k-online"
)

// Client implements the LLM client interface for Perplexity
type Client struct {
	options    llm.ClientOptions
	httpClient *http.Client
}

// PerplexityRequest extends the standard request with Perplexity-specific fields
type PerplexityRequest struct {
	Model            string                   `json:"model"`
	Messages         []llm.Message            `json:"messages"`
	Temperature      float32                  `json:"temperature,omitempty"`
	MaxTokens        int                      `json:"max_tokens,omitempty"`
	TopP             float32                  `json:"top_p,omitempty"`
	Stream           bool                     `json:"stream,omitempty"`
	Tools            []map[string]interface{} `json:"tools,omitempty"`
	ToolChoice       interface{}              `json:"tool_choice,omitempty"`
	FrequencyPenalty float32                  `json:"frequency_penalty,omitempty"`
	PresencePenalty  float32                  `json:"presence_penalty,omitempty"`
	// Perplexity-specific fields
	SearchDomainFilter  []string `json:"search_domain_filter,omitempty"`
	ReturnCitations     bool     `json:"return_citations,omitempty"`
	ReturnImages        bool     `json:"return_images,omitempty"`
	ReturnRelated       bool     `json:"return_related_questions,omitempty"`
	SearchRecencyFilter string   `json:"search_recency_filter,omitempty"`
}

// NewClient creates a new Perplexity client
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
		options.APIKey = os.Getenv("PERPLEXITY_API_KEY")
		if options.APIKey == "" {
			return nil, fmt.Errorf("Perplexity API key not provided")
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

// Chat sends a chat request to Perplexity
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	// Convert to Perplexity request
	perplexityReq := c.convertRequest(request)

	// Create request body
	body, err := json.Marshal(perplexityReq)
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
			return nil, fmt.Errorf("Perplexity API error: %s", errResp.Error.Message)
		}
		return nil, fmt.Errorf("Perplexity API error: status %d, body: %s", resp.StatusCode, string(respBody))
	}

	// Parse response (Perplexity uses OpenAI-compatible format)
	var response llm.ChatResponse
	if err := json.Unmarshal(respBody, &response); err != nil {
		return nil, fmt.Errorf("failed to parse response: %w", err)
	}

	// Perplexity might include citations and other metadata in the response
	// These would be in the message content or as tool calls

	return &response, nil
}

// ChatStream is not implemented for Perplexity yet
func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	return nil, fmt.Errorf("streaming not implemented for Perplexity client")
}

// ListModels returns available Perplexity models
func (c *Client) ListModels(ctx context.Context) ([]llm.Model, error) {
	// Perplexity doesn't have a models endpoint, return hardcoded list
	models := []llm.Model{
		{
			ID:          "llama-3.1-sonar-small-128k-online",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Small Llama 3.1 Sonar model with online search (128k context)",
		},
		{
			ID:          "llama-3.1-sonar-large-128k-online",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Large Llama 3.1 Sonar model with online search (128k context)",
		},
		{
			ID:          "llama-3.1-sonar-huge-128k-online",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Huge Llama 3.1 Sonar model with online search (128k context)",
		},
		{
			ID:          "llama-3.1-sonar-small-128k-chat",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Small Llama 3.1 Sonar model for chat (128k context)",
		},
		{
			ID:          "llama-3.1-sonar-large-128k-chat",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Large Llama 3.1 Sonar model for chat (128k context)",
		},
		{
			ID:          "llama-3.1-8b-instruct",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Llama 3.1 8B instruct model",
		},
		{
			ID:          "llama-3.1-70b-instruct",
			Object:      "model",
			Created:     time.Now().Unix(),
			OwnedBy:     "perplexity",
			Description: "Llama 3.1 70B instruct model",
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

// convertRequest converts from standard format to Perplexity format
func (c *Client) convertRequest(req *llm.ChatRequest) *PerplexityRequest {
	perplexityReq := &PerplexityRequest{
		Model:            req.Model,
		Messages:         req.Messages,
		Temperature:      req.Temperature,
		MaxTokens:        req.MaxTokens,
		TopP:             req.TopP,
		Stream:           req.Stream,
		Tools:            req.Tools,
		ToolChoice:       req.ToolChoice,
		FrequencyPenalty: req.FrequencyPenalty,
		PresencePenalty:  req.PresencePenalty,
		// Enable Perplexity-specific features by default for online models
		ReturnCitations: true,
		ReturnRelated:   true,
	}

	if perplexityReq.Model == "" {
		perplexityReq.Model = c.options.DefaultModel
	}

	// Add search recency filter for online models
	if isOnlineModel(perplexityReq.Model) {
		perplexityReq.SearchRecencyFilter = "week" // Default to recent results
	}

	return perplexityReq
}

// isOnlineModel checks if the model supports online search
func isOnlineModel(model string) bool {
	return len(model) > 6 && model[len(model)-6:] == "online"
}

// SearchOptions provides additional search configuration for Perplexity
type SearchOptions struct {
	// Domains to restrict search to
	Domains []string
	// Recency filter: "day", "week", "month", "year"
	Recency string
	// Whether to return citations
	ReturnCitations bool
	// Whether to return images
	ReturnImages bool
	// Whether to return related questions
	ReturnRelated bool
}

// ChatWithSearch sends a chat request with search options
func (c *Client) ChatWithSearch(ctx context.Context, request *llm.ChatRequest, searchOpts SearchOptions) (*llm.ChatResponse, error) {
	// Convert to Perplexity request
	perplexityReq := c.convertRequest(request)

	// Apply search options
	if len(searchOpts.Domains) > 0 {
		perplexityReq.SearchDomainFilter = searchOpts.Domains
	}
	if searchOpts.Recency != "" {
		perplexityReq.SearchRecencyFilter = searchOpts.Recency
	}
	perplexityReq.ReturnCitations = searchOpts.ReturnCitations
	perplexityReq.ReturnImages = searchOpts.ReturnImages
	perplexityReq.ReturnRelated = searchOpts.ReturnRelated

	// Use the standard chat method with the modified request
	modifiedReq := &llm.ChatRequest{
		Model:            perplexityReq.Model,
		Messages:         perplexityReq.Messages,
		Temperature:      perplexityReq.Temperature,
		MaxTokens:        perplexityReq.MaxTokens,
		TopP:             perplexityReq.TopP,
		Stream:           perplexityReq.Stream,
		Tools:            perplexityReq.Tools,
		ToolChoice:       perplexityReq.ToolChoice,
		FrequencyPenalty: perplexityReq.FrequencyPenalty,
		PresencePenalty:  perplexityReq.PresencePenalty,
	}

	return c.Chat(ctx, modifiedReq)
}
