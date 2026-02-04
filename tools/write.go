package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nachoal/simple-agent-go/tools/base"
)

type WriteParams struct {
	Path    string `json:"path" schema:"required" description:"Path to the file to write (relative or absolute)"`
	Content string `json:"content" schema:"required" description:"Content to write to the file"`
}

// WriteTool writes content to files.
type WriteTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *WriteTool) Parameters() interface{} {
	return &WriteParams{}
}

// Execute writes content to a file.
func (t *WriteTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args WriteParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	_ = ctx // currently unused

	if args.Path == "" {
		return "", NewToolError("VALIDATION_FAILED", "Path cannot be empty")
	}

	// Clean the path
	cleanPath := filepath.Clean(args.Path)

	// Always create parent directories.
	dir := filepath.Dir(cleanPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", NewToolError("MKDIR_ERROR", "Failed to create parent directories").
			WithDetail("error", err.Error()).
			WithDetail("path", cleanPath)
	}

	// Always overwrite.
	if err := os.WriteFile(cleanPath, []byte(args.Content), 0644); err != nil {
		return "", NewToolError("WRITE_ERROR", "Failed to write file").
			WithDetail("error", err.Error()).
			WithDetail("path", cleanPath)
	}

	return fmt.Sprintf("Successfully wrote %d bytes to %s", len(args.Content), cleanPath), nil
}
