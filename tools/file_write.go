package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// FileWriteParams now uses generic input like Ruby
// The input string should be JSON with 'path' and 'content' fields
type FileWriteParams = base.GenericParams

// FileWriteTool writes content to files
type FileWriteTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *FileWriteTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute writes content to a file
func (t *FileWriteTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Parse the input JSON to get path and content
	var inputParams struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := json.Unmarshal([]byte(args.Input), &inputParams); err != nil {
		return "Error parsing input: " + err.Error() + ". Input must be JSON with 'path' and 'content' fields.", nil
	}

	if inputParams.Path == "" {
		return "Error: path parameter is required", nil
	}

	// Clean the path
	cleanPath := filepath.Clean(inputParams.Path)

	// Always create parent directories (Ruby behavior)
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "Error writing file: " + err.Error(), nil
	}

	// Always overwrite (Ruby behavior)
	if err := os.WriteFile(cleanPath, []byte(inputParams.Content), 0644); err != nil {
		return "Error writing file: " + err.Error(), nil
	}

	return fmt.Sprintf("Successfully wrote to %s", cleanPath), nil
}

