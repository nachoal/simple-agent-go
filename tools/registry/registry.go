package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/nachoal/simple-agent-go/internal/schema"
	"github.com/nachoal/simple-agent-go/internal/validator"
	"github.com/nachoal/simple-agent-go/tools"
)

// ToolFactory is a function that creates a new tool instance
type ToolFactory func() tools.Tool

// Registry manages tool registration and discovery
type Registry struct {
	mu        sync.RWMutex
	tools     map[string]ToolFactory
	generator *schema.Generator
	validator *validator.Validator
}

// New creates a new tool registry
func New() *Registry {
	return &Registry{
		tools:     make(map[string]ToolFactory),
		generator: schema.NewGenerator(),
		validator: validator.New(),
	}
}

// Register registers a tool factory with the given name
func (r *Registry) Register(name string, factory ToolFactory) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.tools[name]; exists {
		return fmt.Errorf("tool '%s' is already registered", name)
	}

	r.tools[name] = factory
	return nil
}

// Get retrieves a tool by name
func (r *Registry) Get(name string) (tools.Tool, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	factory, exists := r.tools[name]
	if !exists {
		return nil, fmt.Errorf("tool '%s' not found", name)
	}

	return factory(), nil
}

// List returns a list of all registered tool names
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.tools))
	for name := range r.tools {
		names = append(names, name)
	}
	return names
}

// GetSchema returns the JSON schema for a tool
func (r *Registry) GetSchema(name string) (map[string]interface{}, error) {
	tool, err := r.Get(name)
	if err != nil {
		return nil, err
	}

	return r.generator.GenerateFunctionSchema(
		tool.Name(),
		tool.Description(),
		tool.Parameters(),
	), nil
}

// GetAllSchemas returns schemas for all registered tools
func (r *Registry) GetAllSchemas() []map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	schemas := make([]map[string]interface{}, 0, len(r.tools))
	
	for name := range r.tools {
		if schema, err := r.GetSchema(name); err == nil {
			schemas = append(schemas, schema)
		}
	}

	return schemas
}

// Execute executes a tool by name with the given parameters
func (r *Registry) Execute(ctx context.Context, name string, params json.RawMessage) (string, error) {
	tool, err := r.Get(name)
	if err != nil {
		return "", err
	}

	// Unmarshal parameters into the tool's parameter struct
	paramStruct := tool.Parameters()
	if err := json.Unmarshal(params, paramStruct); err != nil {
		return "", tools.NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Validate parameters
	if err := r.validator.Validate(paramStruct); err != nil {
		return "", tools.NewToolError("VALIDATION_FAILED", "Parameter validation failed").
			WithDetail("error", err.Error())
	}

	// Execute the tool
	return tool.Execute(ctx, params)
}

// ExecuteToolCall executes a tool call
func (r *Registry) ExecuteToolCall(ctx context.Context, call tools.ToolCall) tools.ToolResult {
	result := tools.ToolResult{
		ID:   call.ID,
		Name: call.Name,
	}

	output, err := r.Execute(ctx, call.Name, call.Arguments)
	if err != nil {
		result.Error = err
	} else {
		result.Result = output
	}

	return result
}

// ExecuteToolCalls executes multiple tool calls concurrently
func (r *Registry) ExecuteToolCalls(ctx context.Context, calls []tools.ToolCall) []tools.ToolResult {
	results := make([]tools.ToolResult, len(calls))
	var wg sync.WaitGroup

	for i, call := range calls {
		wg.Add(1)
		go func(idx int, tc tools.ToolCall) {
			defer wg.Done()
			results[idx] = r.ExecuteToolCall(ctx, tc)
		}(i, call)
	}

	wg.Wait()
	return results
}

// defaultRegistry is the global default registry
var defaultRegistry = New()

// Register registers a tool with the default registry
func Register(name string, factory ToolFactory) error {
	return defaultRegistry.Register(name, factory)
}

// Get retrieves a tool from the default registry
func Get(name string) (tools.Tool, error) {
	return defaultRegistry.Get(name)
}

// List returns all tools in the default registry
func List() []string {
	return defaultRegistry.List()
}

// GetSchema returns the schema for a tool in the default registry
func GetSchema(name string) (map[string]interface{}, error) {
	return defaultRegistry.GetSchema(name)
}

// GetAllSchemas returns all schemas from the default registry
func GetAllSchemas() []map[string]interface{} {
	return defaultRegistry.GetAllSchemas()
}

// Execute executes a tool from the default registry
func Execute(ctx context.Context, name string, params json.RawMessage) (string, error) {
	return defaultRegistry.Execute(ctx, name, params)
}

// ExecuteToolCall executes a tool call using the default registry
func ExecuteToolCall(ctx context.Context, call tools.ToolCall) tools.ToolResult {
	return defaultRegistry.ExecuteToolCall(ctx, call)
}

// ExecuteToolCalls executes multiple tool calls using the default registry
func ExecuteToolCalls(ctx context.Context, calls []tools.ToolCall) []tools.ToolResult {
	return defaultRegistry.ExecuteToolCalls(ctx, calls)
}

// Default returns the default registry instance
func Default() *Registry {
	return defaultRegistry
}