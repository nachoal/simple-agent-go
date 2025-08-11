package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// FileReadParams now uses generic input like Ruby
// The input string should be JSON with 'path' field
type FileReadParams = base.GenericParams

// FileReadTool reads file contents
type FileReadTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *FileReadTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute reads a file and returns its contents
func (t *FileReadTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Parse the input JSON to get the path
	var inputParams struct {
		Path string `json:"path"`
	}
	if err := json.Unmarshal([]byte(args.Input), &inputParams); err != nil {
		return "Error parsing input: " + err.Error() + ". Input must be JSON with 'path' field. Example: {\"path\": \"file.txt\"}", nil
	}

	if inputParams.Path == "" {
		return "Error: path parameter is required. Input must be JSON like: {\"path\": \"file.txt\"}", nil
	}

	// Clean and validate the path
	cleanPath := filepath.Clean(inputParams.Path)

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

	// Ruby doesn't limit file size, just read the whole file
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "Error reading file: " + err.Error(), nil
	}

	// Return the content directly (Ruby behavior)
	return string(content), nil
}
