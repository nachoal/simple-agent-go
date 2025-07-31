package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// FileReadParams defines the parameters for the file read tool
type FileReadParams struct {
	Path     string `json:"path" schema:"required" description:"Path to the file to read"`
	Encoding string `json:"encoding,omitempty" schema:"enum:utf-8|ascii|binary" description:"File encoding (default: utf-8)"`
	MaxBytes int    `json:"max_bytes,omitempty" schema:"min:1,max:10485760" description:"Maximum bytes to read (default: 1MB)"`
}

// FileReadTool reads file contents
type FileReadTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *FileReadTool) Parameters() interface{} {
	return &FileReadParams{}
}

// Execute reads a file and returns its contents
func (t *FileReadTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args FileReadParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	if err := Validate(&args); err != nil {
		return "", NewToolError("VALIDATION_FAILED", "Parameter validation failed").
			WithDetail("error", err.Error())
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(args.Path)
	
	// Check if file exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", NewToolError("FILE_NOT_FOUND", "File does not exist").
				WithDetail("path", cleanPath)
		}
		return "", NewToolError("ACCESS_ERROR", "Cannot access file").
			WithDetail("error", err.Error())
	}

	// Check if it's a directory
	if info.IsDir() {
		return "", NewToolError("IS_DIRECTORY", "Path points to a directory, not a file").
			WithDetail("path", cleanPath)
	}

	// Set default max bytes if not specified
	maxBytes := args.MaxBytes
	if maxBytes == 0 {
		maxBytes = 1024 * 1024 // 1MB default
	}

	// Check file size
	if info.Size() > int64(maxBytes) {
		return fmt.Sprintf("File is too large (%d bytes). Only reading first %d bytes.\n", info.Size(), maxBytes), nil
	}

	// Open the file
	file, err := os.Open(cleanPath)
	if err != nil {
		return "", NewToolError("OPEN_ERROR", "Failed to open file").
			WithDetail("error", err.Error())
	}
	defer file.Close()

	// Read the file with size limit
	reader := io.LimitReader(file, int64(maxBytes))
	content, err := io.ReadAll(reader)
	if err != nil {
		return "", NewToolError("READ_ERROR", "Failed to read file").
			WithDetail("error", err.Error())
	}

	// Handle encoding (for now, just return as string)
	// TODO: Add proper encoding support
	result := string(content)

	// Add file info to result
	return fmt.Sprintf("File: %s\nSize: %d bytes\n---\n%s", cleanPath, len(content), result), nil
}

