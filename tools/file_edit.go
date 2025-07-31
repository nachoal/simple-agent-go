package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// FileEditParams now uses generic input like Ruby
// The input string should be JSON with 'path', 'old_str', and 'new_str' fields
type FileEditParams = base.GenericParams

// FileEditTool edits files by replacing strings
type FileEditTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *FileEditTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute edits a file by replacing strings
func (t *FileEditTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Parse the input JSON to get path, old_str, and new_str
	var inputParams struct {
		Path      string `json:"path"`
		OldString string `json:"old_str"`
		NewString string `json:"new_str"`
	}
	if err := json.Unmarshal([]byte(args.Input), &inputParams); err != nil {
		return "Error parsing input: " + err.Error() + ". Input must be JSON with 'path', 'old_str', and 'new_str' fields.", nil
	}

	if inputParams.Path == "" {
		return "Error: path parameter is required", nil
	}

	if inputParams.OldString == inputParams.NewString {
		return "Error: old_str and new_str must be different", nil
	}

	// Clean the path
	cleanPath := filepath.Clean(inputParams.Path)

	// Check if file exists
	if _, err := os.Stat(cleanPath); os.IsNotExist(err) {
		// Create new file (Ruby behavior)
		if inputParams.OldString != "" {
			return "Error: old_str must be empty when creating new file", nil
		}
		
		// Create parent directories
		dir := filepath.Dir(cleanPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "Error editing file: " + err.Error(), nil
		}
		
		// Write new file
		if err := os.WriteFile(cleanPath, []byte(inputParams.NewString), 0644); err != nil {
			return "Error editing file: " + err.Error(), nil
		}
		return fmt.Sprintf("Successfully created file %s", cleanPath), nil
	}

	// Read existing file
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "Error editing file: " + err.Error(), nil
	}

	// Check if old_str is empty for existing file
	if inputParams.OldString == "" {
		return "Error: Cannot use empty old_str on existing file", nil
	}

	// Convert content to string
	fileContent := string(content)

	// Check if old_str exists in file
	if !strings.Contains(fileContent, inputParams.OldString) {
		return "Error: old_str not found in file", nil
	}

	// Replace all occurrences (Ruby behavior)
	newContent := strings.ReplaceAll(fileContent, inputParams.OldString, inputParams.NewString)

	// Write the updated content
	if err := os.WriteFile(cleanPath, []byte(newContent), 0644); err != nil {
		return "Error editing file: " + err.Error(), nil
	}

	return "OK", nil
}

