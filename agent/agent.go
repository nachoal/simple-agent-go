package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// agent is the main agent implementation
type agent struct {
	client       llm.Client
	config       Config
	memory       *Memory
	toolRegistry *registry.Registry
	mu           sync.RWMutex
}

// New creates a new agent
func New(client llm.Client, opts ...Option) Agent {
	config := DefaultConfig()
	
	// Apply options
	for _, opt := range opts {
		opt(&config)
	}

	a := &agent{
		client: client,
		config: config,
		memory: &Memory{
			Messages: make([]llm.Message, 0),
			MaxSize:  config.MemorySize,
		},
		toolRegistry: registry.Default(),
	}

	// Initialize with system prompt
	if config.SystemPrompt != "" {
		a.memory.Messages = append(a.memory.Messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: config.SystemPrompt,
		})
	}

	return a
}

// Query sends a query and returns the response
func (a *agent) Query(ctx context.Context, query string) (*Response, error) {
	// Add user message to memory
	a.addMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: query,
	})

	// Get available tools if configured
	var availableTools []map[string]interface{}
	if len(a.config.Tools) > 0 {
		for _, toolName := range a.config.Tools {
			if schema, err := a.toolRegistry.GetSchema(toolName); err == nil {
				availableTools = append(availableTools, schema)
			}
		}
	} else {
		// If no specific tools configured, use all available tools
		availableTools = a.toolRegistry.GetAllSchemas()
	}

	// Main agent loop
	var totalUsage llm.Usage
	var allToolResults []tools.ToolResult
	
	for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
		// Create chat request
		request := &llm.ChatRequest{
			Messages:    a.getMessages(),
			Temperature: a.config.Temperature,
			MaxTokens:   a.config.MaxTokens,
			Tools:       availableTools,
			ToolChoice:  "auto",
		}

		// Send request to LLM
		response, err := a.client.Chat(ctx, request)
		if err != nil {
			return nil, fmt.Errorf("LLM request failed: %w", err)
		}

		// Update usage
		if response.Usage != nil {
			totalUsage.PromptTokens += response.Usage.PromptTokens
			totalUsage.CompletionTokens += response.Usage.CompletionTokens
			totalUsage.TotalTokens += response.Usage.TotalTokens
		}

		// Check if we have a response
		if len(response.Choices) == 0 {
			return nil, fmt.Errorf("no response from LLM")
		}

		choice := response.Choices[0]
		message := choice.Message

		// Add assistant message to memory
		a.addMessage(message)

		// Check if we need to execute tools
		if len(message.ToolCalls) > 0 {
			// Execute tools
			toolCalls := make([]tools.ToolCall, len(message.ToolCalls))
			for i, tc := range message.ToolCalls {
				toolCalls[i] = tools.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}
			}

			// Execute tool calls concurrently
			results := a.toolRegistry.ExecuteToolCalls(ctx, toolCalls)
			allToolResults = append(allToolResults, results...)

			// Add tool results to memory
			for _, result := range results {
				content := result.Result
				if result.Error != nil {
					content = fmt.Sprintf("Error: %v", result.Error)
				}

				a.addMessage(llm.Message{
					Role:       llm.RoleTool,
					Content:    content,
					ToolCallID: result.ID,
					Name:       result.Name,
				})
			}

			// Continue to next iteration for LLM to process tool results
			continue
		}

		// We have a final response
		return &Response{
			Content:      message.Content,
			ToolCalls:    allToolResults,
			Usage:        &totalUsage,
			FinishReason: choice.FinishReason,
		}, nil
	}

	return nil, fmt.Errorf("max iterations (%d) reached without completion", a.config.MaxIterations)
}

// QueryStream sends a query and streams the response
func (a *agent) QueryStream(ctx context.Context, query string) (<-chan StreamEvent, error) {
	// Add user message to memory
	a.addMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: query,
	})

	// Create event channel
	events := make(chan StreamEvent, 100)

	// Get available tools
	var availableTools []map[string]interface{}
	if len(a.config.Tools) > 0 {
		for _, toolName := range a.config.Tools {
			if schema, err := a.toolRegistry.GetSchema(toolName); err == nil {
				availableTools = append(availableTools, schema)
			}
		}
	} else {
		availableTools = a.toolRegistry.GetAllSchemas()
	}

	// Start streaming goroutine
	go func() {
		defer close(events)

		for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
			// Create chat request
			request := &llm.ChatRequest{
				Messages:    a.getMessages(),
				Temperature: a.config.Temperature,
				MaxTokens:   a.config.MaxTokens,
				Tools:       availableTools,
				ToolChoice:  "auto",
				Stream:      true,
			}

			// Send streaming request to LLM
			streamEvents, err := a.client.ChatStream(ctx, request)
			if err != nil {
				events <- StreamEvent{
					Type:  EventTypeError,
					Error: fmt.Errorf("LLM stream request failed: %w", err),
				}
				return
			}

			// Collect the full response
			var fullContent strings.Builder
			var toolCalls []llm.ToolCall

			// Forward stream events
			for event := range streamEvents {
				if len(event.Choices) > 0 {
					choice := event.Choices[0]
					
					// Handle content delta
					if choice.Delta != nil && choice.Delta.Content != "" {
						fullContent.WriteString(choice.Delta.Content)
						events <- StreamEvent{
							Type:    EventTypeMessage,
							Content: choice.Delta.Content,
						}
					}

					// Handle tool calls
					if choice.Delta != nil && len(choice.Delta.ToolCalls) > 0 {
						toolCalls = append(toolCalls, choice.Delta.ToolCalls...)
					}

					// Check finish reason
					// (finish reason handled later if needed)
				}
			}

			// Create assistant message from collected content
			assistantMsg := llm.Message{
				Role:      llm.RoleAssistant,
				Content:   fullContent.String(),
				ToolCalls: toolCalls,
			}
			a.addMessage(assistantMsg)

			// Execute tools if needed
			if len(toolCalls) > 0 {
				// Convert to tool calls
				calls := make([]tools.ToolCall, len(toolCalls))
				for i, tc := range toolCalls {
					calls[i] = tools.ToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: tc.Function.Arguments,
					}

					// Send tool start event
					events <- StreamEvent{
						Type: EventTypeToolStart,
						Tool: &ToolEvent{
							Name: tc.Function.Name,
							Args: string(tc.Function.Arguments),
						},
					}
				}

				// Execute tools
				results := a.toolRegistry.ExecuteToolCalls(ctx, calls)

				// Send tool results and add to memory
				for _, result := range results {
					content := result.Result
					if result.Error != nil {
						content = fmt.Sprintf("Error: %v", result.Error)
					}

					// Send tool result event
					events <- StreamEvent{
						Type: EventTypeToolResult,
						Tool: &ToolEvent{
							Name:   result.Name,
							Result: content,
							Error:  result.Error,
						},
					}

					// Add to memory
					a.addMessage(llm.Message{
						Role:       llm.RoleTool,
						Content:    content,
						ToolCallID: result.ID,
						Name:       result.Name,
					})
				}

				// Continue to next iteration
				continue
			}

			// Send completion event
			events <- StreamEvent{
				Type: EventTypeComplete,
			}
			return
		}

		// Max iterations reached
		events <- StreamEvent{
			Type:  EventTypeError,
			Error: fmt.Errorf("max iterations (%d) reached", a.config.MaxIterations),
		}
	}()

	return events, nil
}

// Clear clears the conversation memory
func (a *agent) Clear() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.memory.Messages = make([]llm.Message, 0)
	a.memory.TokenCount = 0

	// Re-add system prompt
	if a.config.SystemPrompt != "" {
		a.memory.Messages = append(a.memory.Messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: a.config.SystemPrompt,
		})
	}
}

// GetMemory returns the current conversation memory
func (a *agent) GetMemory() []llm.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messages := make([]llm.Message, len(a.memory.Messages))
	copy(messages, a.memory.Messages)
	return messages
}

// SetSystemPrompt updates the system prompt
func (a *agent) SetSystemPrompt(prompt string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.config.SystemPrompt = prompt

	// Update the first message if it's a system message
	if len(a.memory.Messages) > 0 && a.memory.Messages[0].Role == llm.RoleSystem {
		a.memory.Messages[0].Content = prompt
	} else {
		// Insert system message at the beginning
		a.memory.Messages = append([]llm.Message{{
			Role:    llm.RoleSystem,
			Content: prompt,
		}}, a.memory.Messages...)
	}
}

// addMessage adds a message to memory with size management
func (a *agent) addMessage(msg llm.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.memory.Messages = append(a.memory.Messages, msg)

	// Trim memory if needed (keep system prompt)
	if len(a.memory.Messages) > a.memory.MaxSize {
		systemMsg := a.memory.Messages[0]
		if systemMsg.Role == llm.RoleSystem {
			// Keep system prompt and trim old messages
			trimCount := len(a.memory.Messages) - a.memory.MaxSize
			a.memory.Messages = append([]llm.Message{systemMsg}, a.memory.Messages[trimCount+1:]...)
		} else {
			// No system prompt, just trim
			trimCount := len(a.memory.Messages) - a.memory.MaxSize
			a.memory.Messages = a.memory.Messages[trimCount:]
		}
	}
}

// getMessages returns a copy of messages for API calls
func (a *agent) getMessages() []llm.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messages := make([]llm.Message, len(a.memory.Messages))
	copy(messages, a.memory.Messages)
	return messages
}

// Option is a functional option for configuring the agent
type Option func(*Config)

// WithSystemPrompt sets the system prompt
func WithSystemPrompt(prompt string) Option {
	return func(c *Config) {
		c.SystemPrompt = prompt
	}
}

// WithMaxIterations sets the maximum iterations
func WithMaxIterations(max int) Option {
	return func(c *Config) {
		c.MaxIterations = max
	}
}

// WithTemperature sets the temperature
func WithTemperature(temp float32) Option {
	return func(c *Config) {
		c.Temperature = temp
	}
}

// WithMaxTokens sets the max tokens
func WithMaxTokens(max int) Option {
	return func(c *Config) {
		c.MaxTokens = max
	}
}

// WithTools sets the allowed tools
func WithTools(tools []string) Option {
	return func(c *Config) {
		c.Tools = tools
	}
}

// WithVerbose enables verbose mode
func WithVerbose(verbose bool) Option {
	return func(c *Config) {
		c.Verbose = verbose
	}
}

// WithMemorySize sets the memory size
func WithMemorySize(size int) Option {
	return func(c *Config) {
		c.MemorySize = size
	}
}