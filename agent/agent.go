package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// Tool ID generation
var toolIDCounter uint64

func generateToolID() string {
	id := atomic.AddUint64(&toolIDCounter, 1)
	return fmt.Sprintf("tool-%d-%d", time.Now().UnixNano(), id)
}

// agent is the main agent implementation
type agent struct {
	client          llm.Client
	config          Config
	memory          *Memory
	toolRegistry    *registry.Registry
	mu              sync.RWMutex
	progressHandler func(ProgressEvent)
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
		toolRegistry:    registry.Default(),
		progressHandler: config.progressHandler,
	}

	// Initialize with system prompt
	if config.SystemPrompt != "" {
		// Get tool information to enhance the system prompt
		toolInfo := a.getToolListForPrompt()
		enhancedPrompt := config.SystemPrompt
		if toolInfo != "" {
			enhancedPrompt = config.SystemPrompt + "\n\n" + toolInfo
		}

		a.memory.Messages = append(a.memory.Messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: llm.StringPtr(enhancedPrompt),
		})
	}

	return a
}

// Query sends a query and returns the response
func (a *agent) Query(ctx context.Context, query string) (*Response, error) {
	// Add user message to memory
	a.addMessage(llm.Message{
		Role:    llm.RoleUser,
		Content: llm.StringPtr(query),
	})

	// Extract stream channel (if any) once
	var streamChan chan<- StreamEvent
	if ch, ok := ctx.Value("toolEventChan").(chan StreamEvent); ok {
		streamChan = ch // nil if UI isn't streaming
	}

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
	toolChoice := "auto"
	totalToolCalls := 0

	for iteration := 0; iteration < a.config.MaxIterations; iteration++ {
		// Emit progress event for iteration
		a.emitProgress(ProgressEvent{
			Type:      ProgressEventIteration,
			Iteration: iteration + 1,
			Max:       a.config.MaxIterations,
		})

		// Keep allowing tool calls to enable multi-tool chains.
		// We'll rely on max iterations and model behavior to avoid loops.
		// toolChoice remains "auto" unless explicitly changed elsewhere.

		// Create chat request
		request := &llm.ChatRequest{
			Messages:    a.getMessages(),
			Temperature: a.config.Temperature,
			MaxTokens:   a.config.MaxTokens,
			TopP:        a.config.TopP,
			ExtraBody:   a.config.ExtraBody,
			Tools:       availableTools,
			ToolChoice:  toolChoice,
		}

		// Debug log available tools
		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" && len(availableTools) > 0 {
			fmt.Fprintf(os.Stderr, "\n[Agent] Sending %d tools to LLM:\n", len(availableTools))
			for _, tool := range availableTools {
				if fn, ok := tool["function"].(map[string]interface{}); ok {
					fmt.Fprintf(os.Stderr, "[Agent] - %s: %s\n", fn["name"], fn["description"])
				}
			}
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

		// Check if we need to parse tool calls from content (for LMStudio/Moonshot)
		if len(message.ToolCalls) == 0 && message.Content != nil && *message.Content != "" {
			if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "\n[Agent] No native tool calls found, attempting to parse from content:\n%s\n", *message.Content)
			}

			// Try to parse tool calls from content
			toolCalls := a.parseToolCallsFromContent(*message.Content)
			if len(toolCalls) > 0 {
				if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
					fmt.Fprintf(os.Stderr, "[Agent] Parsed %d tool calls from content\n", len(toolCalls))
					for i, tc := range toolCalls {
						fmt.Fprintf(os.Stderr, "[Agent] Tool Call %d: %s with args: %s\n", i, tc.Function.Name, string(tc.Function.Arguments))
					}
				}
				message.ToolCalls = toolCalls
				message.Content = nil // Clear content if we found tool calls
			} else if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "[Agent] No tool calls could be parsed from content\n")
			}
		}

		// If the message has tool calls, ensure content is not nil.
		// Some models require `content` to be an empty string if `tool_calls` are present.
		if len(message.ToolCalls) > 0 && message.Content == nil {
			message.Content = llm.StringPtr("")
		}

		// Normalize tool arguments to canonical JSON objects before persisting or executing.
		normalizeLLMToolCalls(message.ToolCalls)

		// Add assistant message to memory
		a.addMessage(message)

		// Check if we need to execute tools
		if len(message.ToolCalls) > 0 {
			if a.config.MaxToolCalls > 0 && totalToolCalls+len(message.ToolCalls) > a.config.MaxToolCalls {
				return nil, fmt.Errorf("max tool calls (%d) reached without completion", a.config.MaxToolCalls)
			}
			totalToolCalls += len(message.ToolCalls)
			// Emit progress event for tool calls
			a.emitProgress(ProgressEvent{
				Type:      ProgressEventToolCallsStart,
				ToolCount: len(message.ToolCalls),
			})

			// Execute tools
			toolCalls := make([]tools.ToolCall, len(message.ToolCalls))
			for i, tc := range message.ToolCalls {
				toolCalls[i] = tools.ToolCall{
					ID:        tc.ID,
					Name:      tc.Function.Name,
					Arguments: tc.Function.Arguments,
				}

				// Emit progress event for individual tool call
				a.emitProgress(ProgressEvent{
					Type:     ProgressEventToolCall,
					ToolName: tc.Function.Name,
				})
			}

			// Execute tool calls with events if channel provided
			results := a.executeToolsWithEvents(ctx, toolCalls, streamChan)
			allToolResults = append(allToolResults, results...)

			// Add tool results to memory
			for _, result := range results {
				content := result.Result
				if result.Error != nil {
					content = fmt.Sprintf("Error: %v", result.Error)
				}

				a.addMessage(llm.Message{
					Role:       llm.RoleTool,
					Content:    llm.StringPtr(content),
					ToolCallID: result.ID,
				})
			}

			// Continue to next iteration for LLM to process tool results
			// Reset tool choice for next iteration
			toolChoice = "auto"
			continue
		}

		// Check if we have empty content
		if message.Content == nil || (message.Content != nil && strings.TrimSpace(*message.Content) == "") {
			// Emit no tools event
			a.emitProgress(ProgressEvent{
				Type: ProgressEventNoTools,
			})

			// Model returned empty content, prompt for response
			a.addMessage(llm.Message{
				Role:    llm.RoleUser,
				Content: llm.StringPtr("Please provide your response based on the information gathered."),
			})
			toolChoice = "none"
			continue
		}

		// We have a final response
		content := ""
		if message.Content != nil {
			content = *message.Content
		}
		return &Response{
			Content:      content,
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
		Content: llm.StringPtr(query),
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
		totalToolCalls := 0

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
					if choice.Delta != nil && choice.Delta.Content != nil && *choice.Delta.Content != "" {
						fullContent.WriteString(*choice.Delta.Content)
						events <- StreamEvent{
							Type:    EventTypeMessage,
							Content: *choice.Delta.Content,
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
			contentStr := fullContent.String()
			var contentPtr *string
			if contentStr != "" || len(toolCalls) == 0 {
				contentPtr = &contentStr
			}
			assistantMsg := llm.Message{
				Role:      llm.RoleAssistant,
				Content:   contentPtr,
				ToolCalls: toolCalls,
			}
			normalizeLLMToolCalls(assistantMsg.ToolCalls)
			a.addMessage(assistantMsg)

			// Execute tools if needed
			if len(toolCalls) > 0 {
				if a.config.MaxToolCalls > 0 && totalToolCalls+len(toolCalls) > a.config.MaxToolCalls {
					events <- StreamEvent{
						Type:  EventTypeError,
						Error: fmt.Errorf("max tool calls (%d) reached", a.config.MaxToolCalls),
					}
					return
				}
				totalToolCalls += len(toolCalls)
				// Convert to tool calls
				calls := make([]tools.ToolCall, len(toolCalls))
				for i, tc := range toolCalls {
					args, normalizedArgs := llm.NormalizeToolArguments(tc.Function.Arguments)

					calls[i] = tools.ToolCall{
						ID:        tc.ID,
						Name:      tc.Function.Name,
						Arguments: normalizedArgs,
					}

					// Send tool start event
					events <- StreamEvent{
						Type: EventTypeToolStart,
						Tool: &ToolEvent{
							Name:    tc.Function.Name,
							Args:    args,
							ArgsRaw: string(normalizedArgs),
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
							ID:     result.ID,
							Name:   result.Name,
							Result: content,
							Error:  result.Error,
						},
					}

					// Add to memory
					a.addMessage(llm.Message{
						Role:       llm.RoleTool,
						Content:    llm.StringPtr(content),
						ToolCallID: result.ID,
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

	// Re-add system prompt with tool list
	if a.config.SystemPrompt != "" {
		// Get tool information to enhance the system prompt
		toolInfo := a.getToolListForPrompt()
		enhancedPrompt := a.config.SystemPrompt
		if toolInfo != "" {
			enhancedPrompt = a.config.SystemPrompt + "\n\n" + toolInfo
		}

		a.memory.Messages = append(a.memory.Messages, llm.Message{
			Role:    llm.RoleSystem,
			Content: llm.StringPtr(enhancedPrompt),
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

	// Get tool information to enhance the system prompt
	toolInfo := a.getToolListForPrompt()
	enhancedPrompt := prompt
	if toolInfo != "" {
		enhancedPrompt = prompt + "\n\n" + toolInfo
	}

	// Update the first message if it's a system message
	if len(a.memory.Messages) > 0 && a.memory.Messages[0].Role == llm.RoleSystem {
		a.memory.Messages[0].Content = llm.StringPtr(enhancedPrompt)
	} else {
		// Insert system message at the beginning
		a.memory.Messages = append([]llm.Message{{
			Role:    llm.RoleSystem,
			Content: llm.StringPtr(enhancedPrompt),
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

// getMessages returns a copy of messages for API calls, ensuring compatibility.
func (a *agent) getMessages() []llm.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()

	messages := make([]llm.Message, len(a.memory.Messages))
	copy(messages, a.memory.Messages)

	// Compatibility fix for models that require a non-nil content field for tool calls.
	for i := range messages {
		if messages[i].Role == llm.RoleAssistant && len(messages[i].ToolCalls) > 0 && messages[i].Content == nil {
			messages[i].Content = llm.StringPtr("")
		}
	}

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

// WithMaxToolCalls sets the maximum tool calls
func WithMaxToolCalls(max int) Option {
	return func(c *Config) {
		c.MaxToolCalls = max
	}
}

// WithTemperature sets the temperature
func WithTemperature(temp float32) Option {
	return func(c *Config) {
		c.Temperature = temp
	}
}

// WithTopP sets top-p
func WithTopP(topP float32) Option {
	return func(c *Config) {
		c.TopP = topP
	}
}

// WithExtraBody sets provider-specific extra body parameters
func WithExtraBody(extra map[string]interface{}) Option {
	return func(c *Config) {
		c.ExtraBody = extra
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

// WithProgressHandler sets a progress handler function
func WithProgressHandler(handler func(ProgressEvent)) Option {
	return func(c *Config) {
		// Store in a temporary field that we'll extract
		c.progressHandler = handler
	}
}

// WithLMStudioParser enables/disables parsing of LM Studio channel-markup tool calls
func WithLMStudioParser(enabled bool) Option {
	return func(c *Config) {
		c.EnableLMStudioParser = enabled
	}
}

// SetRequestParams updates the per-request model parameters.
func (a *agent) SetRequestParams(params RequestParams) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.Temperature = params.Temperature
	a.config.TopP = params.TopP
	if params.ExtraBody == nil {
		a.config.ExtraBody = nil
		return
	}
	clone := make(map[string]interface{}, len(params.ExtraBody))
	for k, v := range params.ExtraBody {
		clone[k] = v
	}
	a.config.ExtraBody = clone
}

// GetRequestParams returns the current per-request model parameters.
func (a *agent) GetRequestParams() RequestParams {
	a.mu.RLock()
	defer a.mu.RUnlock()
	var extra map[string]interface{}
	if a.config.ExtraBody != nil {
		extra = make(map[string]interface{}, len(a.config.ExtraBody))
		for k, v := range a.config.ExtraBody {
			extra[k] = v
		}
	}
	return RequestParams{
		Temperature: a.config.Temperature,
		TopP:        a.config.TopP,
		ExtraBody:   extra,
	}
}

// emitProgress emits a progress event if a handler is set
func (a *agent) emitProgress(event ProgressEvent) {
	if a.progressHandler != nil {
		a.progressHandler(event)
	}
}

// getToolListForPrompt generates a formatted list of available tools for the system prompt
func (a *agent) getToolListForPrompt() string {
	if a.toolRegistry == nil {
		return ""
	}

	var toolInfo strings.Builder
	toolInfo.WriteString("Available tools:\n\n")

	// Prefer the configured toolset, otherwise list everything registered.
	toolNames := a.config.Tools
	if len(toolNames) == 0 {
		toolNames = a.toolRegistry.List()
	}

	// Sort for consistent ordering
	sort.Strings(toolNames)

	for _, name := range toolNames {
		tool, err := a.toolRegistry.Get(name)
		if err != nil {
			continue
		}

		toolInfo.WriteString(fmt.Sprintf("- %s: %s\n", name, tool.Description()))
	}

	toolInfo.WriteString("\n")
	toolInfo.WriteString("When you need to use a tool, respond with a JSON object in this format:\n")
	toolInfo.WriteString(`{"name": "tool_name", "arguments": {"param1": "value1", "param2": "value2"}}`)
	toolInfo.WriteString("\n\nDo not include any other text when calling a tool, just the JSON object.")

	return toolInfo.String()
}

// SetMemory sets the conversation memory
func (a *agent) SetMemory(messages []llm.Message) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.memory.Messages = make([]llm.Message, len(messages))
	copy(a.memory.Messages, messages)

	// Update token count if needed
	// TODO: Implement token counting
	a.memory.TokenCount = 0
}

// parseToolCallsFromContent attempts to parse tool calls from content
// This is for compatibility with providers that return tool calls in content
func (a *agent) parseToolCallsFromContent(content string) []llm.ToolCall {
	var toolCalls []llm.ToolCall

	// 0) LM Studio / channel-markup compatibility (gated by config)
	// Example: "<|start|>assistant<|channel|>commentary to=functions.google_search <|constrain|>json<|message|>{\"input\":\"Tunguska incident\"}"
	// Extract tool name after "to=functions." and JSON after "<|message|>"
	if a.config.EnableLMStudioParser && strings.Contains(content, "to=functions.") && strings.Contains(content, "<|message|>") {
		// Tool name
		name := ""
		if i := strings.Index(content, "to=functions."); i >= 0 {
			start := i + len("to=functions.")
			// read until whitespace
			j := start
			for j < len(content) && !unicode.IsSpace(rune(content[j])) {
				j++
			}
			if j > start {
				name = content[start:j]
			}
		}

		// JSON arguments after <|message|>
		argsJSON := ""
		if k := strings.Index(content, "<|message|>"); k >= 0 {
			payload := strings.TrimSpace(content[k+len("<|message|>"):])

			// Try to extract first balanced JSON object
			// Simple brace-balancing scanner
			depth := 0
			started := false
			var b strings.Builder
			for _, ch := range payload {
				if !started {
					if ch == '{' {
						started = true
						depth = 1
						b.WriteRune(ch)
					}
					continue
				} else {
					b.WriteRune(ch)
					if ch == '{' {
						depth++
					} else if ch == '}' {
						depth--
						if depth == 0 {
							break
						}
					}
				}
			}
			if started {
				argsJSON = b.String()
			}
		}

		if name != "" && argsJSON != "" {
			// Validate JSON
			var tmp interface{}
			if json.Unmarshal([]byte(argsJSON), &tmp) == nil {
				id := fmt.Sprintf("call_%d_%d", time.Now().Unix(), rand.Intn(1000))
				_, normalizedArgs := llm.NormalizeToolArguments(json.RawMessage(argsJSON))
				toolCalls = append(toolCalls, llm.ToolCall{
					ID:   id,
					Type: "function",
					Function: llm.FunctionCall{
						Name:      name,
						Arguments: normalizedArgs,
					},
				})
				return toolCalls
			}
		}
	}

	// Try to parse as single JSON object
	var singleCall struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		ID        string          `json:"id,omitempty"`
	}

	content = strings.TrimSpace(content)
	if err := json.Unmarshal([]byte(content), &singleCall); err == nil && singleCall.Name != "" {
		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[Agent] Successfully parsed single JSON tool call\n")
		}

		id := singleCall.ID
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", time.Now().Unix(), rand.Intn(1000))
		}
		_, normalizedArgs := llm.NormalizeToolArguments(singleCall.Arguments)

		toolCalls = append(toolCalls, llm.ToolCall{
			ID:   id,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      singleCall.Name,
				Arguments: normalizedArgs,
			},
		})
		return toolCalls
	} else if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" && err != nil {
		fmt.Fprintf(os.Stderr, "[Agent] Failed to parse as single JSON: %v\n", err)
	}

	// Try multiple JSON objects with regex
	jsonPattern := regexp.MustCompile(`\{"name":\s*"([^"]+)",\s*"arguments":\s*(\{[^}]*\})(?:,\s*"id":\s*"([^"]+)")?\}`)
	matches := jsonPattern.FindAllStringSubmatch(content, -1)

	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		fmt.Fprintf(os.Stderr, "[Agent] Regex pattern found %d matches\n", len(matches))
	}

	for _, match := range matches {
		name := match[1]
		args := json.RawMessage(match[2])
		id := ""
		if len(match) > 3 {
			id = match[3]
		}
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", time.Now().Unix(), rand.Intn(1000))
		}

		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[Agent] Regex match found tool: %s with args: %s\n", name, string(args))
		}
		_, normalizedArgs := llm.NormalizeToolArguments(args)

		toolCalls = append(toolCalls, llm.ToolCall{
			ID:   id,
			Type: "function",
			Function: llm.FunctionCall{
				Name:      name,
				Arguments: normalizedArgs,
			},
		})
	}

	return toolCalls
}

func normalizeLLMToolCalls(toolCalls []llm.ToolCall) {
	for i := range toolCalls {
		_, normalizedArgs := llm.NormalizeToolArguments(toolCalls[i].Function.Arguments)
		toolCalls[i].Function.Arguments = normalizedArgs
	}
}

// executeToolsWithEvents executes tools and emits events without streaming
func (a *agent) executeToolsWithEvents(ctx context.Context, calls []tools.ToolCall, eventChan chan<- StreamEvent) []tools.ToolResult {
	results := make([]tools.ToolResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc tools.ToolCall) {
			defer wg.Done()

			// Generate unique ID if not present
			if tc.ID == "" {
				tc.ID = generateToolID()
			}

			args, normalizedArgs := llm.NormalizeToolArguments(tc.Arguments)
			tc.Arguments = normalizedArgs

			// Print to stderr in query mode (no event channel)
			if eventChan == nil {
				fmt.Fprintf(os.Stderr, "🔧 Calling tool: %s\n", tc.Name)
			}

			// Emit tool start event if channel provided
			if eventChan != nil {
				if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
					fmt.Fprintf(os.Stderr, "[Agent] Sending tool start event for %s (ID: %s)\n", tc.Name, tc.ID)
				}
				select {
				case eventChan <- StreamEvent{
					Type: EventTypeToolStart,
					Tool: &ToolEvent{
						ID:      tc.ID,
						Name:    tc.Name,
						Args:    args,
						ArgsRaw: string(normalizedArgs),
					},
				}:
				case <-ctx.Done():
					return
				}
			}

			// Execute the tool
			startTime := time.Now()
			result := a.toolRegistry.ExecuteToolCall(ctx, tc)
			duration := time.Since(startTime)
			results[idx] = result

			// Print completion in query mode
			if eventChan == nil {
				fmt.Fprintf(os.Stderr, "🔧 %s completed in %v\n", tc.Name, duration)
			}

			// Emit tool result event if channel provided
			if eventChan != nil {
				eventType := EventTypeToolResult
				if result.Error != nil {
					// Distinguish cancel/timeout from generic errors when possible.
					if toolErr, ok := result.Error.(*tools.ToolError); ok {
						switch toolErr.Code {
						case "EXECUTION_CANCELLED":
							eventType = EventTypeToolCancel
						case "EXECUTION_TIMEOUT":
							eventType = EventTypeToolTimeout
						}
					}
					if eventType == EventTypeToolResult {
						lowerErr := strings.ToLower(result.Error.Error())
						switch {
						case strings.Contains(lowerErr, "context canceled"), strings.Contains(lowerErr, "cancelled"):
							eventType = EventTypeToolCancel
						case strings.Contains(lowerErr, "deadline exceeded"), strings.Contains(lowerErr, "timed out"):
							eventType = EventTypeToolTimeout
						}
					}
				}

				select {
				case eventChan <- StreamEvent{
					Type: eventType,
					Tool: &ToolEvent{
						ID:      tc.ID,
						Name:    tc.Name,
						Args:    args,
						ArgsRaw: string(normalizedArgs),
						Result:  result.Result,
						Error:   result.Error,
					},
				}:
				case <-ctx.Done():
					return
				}
			}
		}(i, call)
	}

	wg.Wait()
	return results
}
