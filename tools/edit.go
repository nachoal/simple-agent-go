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

type EditParams struct {
	Path    string `json:"path" schema:"required" description:"Path to the file to edit (relative or absolute)"`
	OldText string `json:"oldText" schema:"required" description:"Exact text to find and replace (must match exactly)"`
	NewText string `json:"newText" schema:"required" description:"New text to replace the old text with"`
}

// EditTool edits files by replacing text.
type EditTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *EditTool) Parameters() interface{} {
	return &EditParams{}
}

// Execute edits a file by replacing text.
func (t *EditTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args EditParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	_ = ctx // currently unused

	if args.Path == "" {
		return "", NewToolError("VALIDATION_FAILED", "Path cannot be empty")
	}

	if args.OldText == args.NewText {
		return "", NewToolError("VALIDATION_FAILED", "oldText and newText must be different")
	}

	resolvedPath, workspace, err := resolveWorkspacePath(args.Path)
	if err != nil {
		return "", err
	}
	displayPath := displayPathForWorkspace(resolvedPath, workspace)

	// Check if file exists
	if _, err := os.Stat(resolvedPath); os.IsNotExist(err) {
		// If file doesn't exist, allow creation only when oldText is empty.
		if args.OldText != "" {
			return "", NewToolError("FILE_NOT_FOUND", "File does not exist; oldText must be empty to create it").
				WithDetail("path", displayPath)
		}

		// Create parent directories
		dir := filepath.Dir(resolvedPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", NewToolError("MKDIR_ERROR", "Failed to create parent directories").
				WithDetail("error", err.Error()).
				WithDetail("path", displayPath)
		}

		// Write new file
		if err := os.WriteFile(resolvedPath, []byte(args.NewText), 0644); err != nil {
			return "", NewToolError("WRITE_ERROR", "Failed to create file").
				WithDetail("error", err.Error()).
				WithDetail("path", displayPath)
		}
		return fmt.Sprintf("Successfully created file %s", displayPath), nil
	}

	// Read existing file
	content, err := os.ReadFile(resolvedPath)
	if err != nil {
		return "", NewToolError("READ_ERROR", "Failed to read file").
			WithDetail("error", err.Error()).
			WithDetail("path", displayPath)
	}

	// Check if oldText is empty for existing file
	if args.OldText == "" {
		return "", NewToolError("VALIDATION_FAILED", "Cannot use empty oldText on an existing file").
			WithDetail("path", displayPath)
	}

	// Convert content to string
	fileContent := string(content)

	// Check if oldText exists in file
	if !strings.Contains(fileContent, args.OldText) {
		return "", NewToolError("NOT_FOUND", "oldText not found in file").
			WithDetail("path", displayPath)
	}

	occurrences := strings.Count(fileContent, args.OldText)
	if occurrences > 1 {
		return "", NewToolError("NOT_UNIQUE", "oldText occurs more than once; provide a more specific match").
			WithDetail("path", displayPath).
			WithDetail("occurrences", occurrences)
	}

	// Replace exact match (single occurrence)
	newContent := strings.Replace(fileContent, args.OldText, args.NewText, 1)

	// Write the updated content
	if err := os.WriteFile(resolvedPath, []byte(newContent), 0644); err != nil {
		return "", NewToolError("WRITE_ERROR", "Failed to write file").
			WithDetail("error", err.Error()).
			WithDetail("path", displayPath)
	}

	return fmt.Sprintf("Successfully replaced text in %s", displayPath), nil
}
