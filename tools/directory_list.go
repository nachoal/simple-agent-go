package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// DirectoryListParams now uses generic input like Ruby
// The input string should be JSON with optional 'path' field
type DirectoryListParams = base.GenericParams

// DirectoryListTool lists directory contents
type DirectoryListTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *DirectoryListTool) Parameters() interface{} {
	return &base.GenericParams{}
}

// Execute lists directory contents
func (t *DirectoryListTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args base.GenericParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	// Parse the input JSON to get optional path
	var inputParams struct {
		Path string `json:"path,omitempty"`
	}

	// Handle nil or empty input by defaulting to empty JSON object
	input := strings.TrimSpace(args.Input)
	if input == "" {
		input = "{}"
	}

	if err := json.Unmarshal([]byte(input), &inputParams); err != nil {
		return "Error parsing input: " + err.Error() + ". Input must be JSON. Example: {\"path\": \"directory\"} or {}", nil
	}

	// Default to current directory if no path specified
	path := inputParams.Path
	if path == "" {
		path = "."
	}

	// Clean the path
	cleanPath := filepath.Clean(path)

	// Ruby behavior: use glob to get all files recursively
	pattern := filepath.Join(cleanPath, "**", "*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "Error listing directory: " + err.Error(), nil
	}

	// Also include files in the base directory
	basePattern := filepath.Join(cleanPath, "*")
	baseMatches, err := filepath.Glob(basePattern)
	if err != nil {
		return "Error listing directory: " + err.Error(), nil
	}

	// Combine and deduplicate
	allMatches := append(matches, baseMatches...)
	seen := make(map[string]bool)
	var entries []string

	for _, match := range allMatches {
		if seen[match] {
			continue
		}
		seen[match] = true

		// Skip . and .. entries (Ruby behavior)
		base := filepath.Base(match)
		if base == "." || base == ".." {
			continue
		}

		// Check if it's a directory and add trailing slash
		info, err := os.Stat(match)
		if err == nil && info.IsDir() {
			entries = append(entries, match+"/")
		} else {
			entries = append(entries, match)
		}
	}

	// Sort entries (Ruby behavior)
	sort.Strings(entries)

	// Return as JSON array
	result, err := json.Marshal(entries)
	if err != nil {
		return "Error listing directory: " + err.Error(), nil
	}

	return string(result), nil
}
