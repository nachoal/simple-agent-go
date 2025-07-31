package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// FileWriteParams defines the parameters for the file write tool
type FileWriteParams struct {
	Path      string `json:"path" schema:"required" description:"Path to the file to write"`
	Content   string `json:"content" schema:"required" description:"Content to write to the file"`
	Mode      string `json:"mode,omitempty" schema:"enum:overwrite|append" description:"Write mode (default: overwrite)"`
	CreateDir bool   `json:"create_dir,omitempty" description:"Create parent directories if they don't exist"`
}

// FileWriteTool writes content to files
type FileWriteTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *FileWriteTool) Parameters() interface{} {
	return &FileWriteParams{}
}

// Execute writes content to a file
func (t *FileWriteTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args FileWriteParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	if err := Validate(&args); err != nil {
		return "", NewToolError("VALIDATION_FAILED", "Parameter validation failed").
			WithDetail("error", err.Error())
	}

	// Clean the path
	cleanPath := filepath.Clean(args.Path)

	// Create parent directories if requested
	if args.CreateDir {
		dir := filepath.Dir(cleanPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", NewToolError("DIR_CREATE_ERROR", "Failed to create parent directories").
				WithDetail("error", err.Error()).
				WithDetail("path", dir)
		}
	}

	// Determine file flags based on mode
	flags := os.O_WRONLY | os.O_CREATE
	if args.Mode == "append" {
		flags |= os.O_APPEND
	} else {
		flags |= os.O_TRUNC
	}

	// Open/create the file
	file, err := os.OpenFile(cleanPath, flags, 0644)
	if err != nil {
		return "", NewToolError("OPEN_ERROR", "Failed to open file for writing").
			WithDetail("error", err.Error()).
			WithDetail("path", cleanPath)
	}
	defer file.Close()

	// Write the content
	bytesWritten, err := file.WriteString(args.Content)
	if err != nil {
		return "", NewToolError("WRITE_ERROR", "Failed to write to file").
			WithDetail("error", err.Error())
	}

	// Sync to ensure data is written to disk
	if err := file.Sync(); err != nil {
		return "", NewToolError("SYNC_ERROR", "Failed to sync file to disk").
			WithDetail("error", err.Error())
	}

	mode := "overwritten"
	if args.Mode == "append" {
		mode = "appended"
	}

	return fmt.Sprintf("Successfully %s %d bytes to file: %s", mode, bytesWritten, cleanPath), nil
}

