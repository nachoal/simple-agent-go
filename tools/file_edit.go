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

// FileEditParams defines the parameters for the file edit tool
type FileEditParams struct {
	Path        string `json:"path" schema:"required" description:"Path to the file to edit"`
	OldString   string `json:"old_string" schema:"required" description:"The exact string to replace"`
	NewString   string `json:"new_string" schema:"required" description:"The new string to replace with"`
	ReplaceAll  bool   `json:"replace_all,omitempty" description:"Replace all occurrences (default: false, only first)"`
	IgnoreCase  bool   `json:"ignore_case,omitempty" description:"Ignore case when searching"`
	CreateBackup bool  `json:"create_backup,omitempty" description:"Create a backup before editing"`
}

// FileEditTool edits files by replacing strings
type FileEditTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *FileEditTool) Parameters() interface{} {
	return &FileEditParams{}
}

// Execute edits a file by replacing strings
func (t *FileEditTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args FileEditParams
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

	// Read the file
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", NewToolError("FILE_NOT_FOUND", "File does not exist").
				WithDetail("path", cleanPath)
		}
		return "", NewToolError("READ_ERROR", "Failed to read file").
			WithDetail("error", err.Error())
	}

	// Create backup if requested
	if args.CreateBackup {
		backupPath := cleanPath + ".bak"
		if err := os.WriteFile(backupPath, content, 0644); err != nil {
			return "", NewToolError("BACKUP_ERROR", "Failed to create backup").
				WithDetail("error", err.Error()).
				WithDetail("backup_path", backupPath)
		}
	}

	// Convert content to string for manipulation
	fileContent := string(content)
	originalContent := fileContent

	// Perform the replacement
	var replacementCount int
	if args.IgnoreCase {
		// Case-insensitive replacement
		if args.ReplaceAll {
			lower := strings.ToLower(fileContent)
			searchLower := strings.ToLower(args.OldString)
			
			// Count occurrences
			replacementCount = strings.Count(lower, searchLower)
			
			// Perform case-insensitive replace all
			for {
				idx := strings.Index(strings.ToLower(fileContent), searchLower)
				if idx == -1 {
					break
				}
				fileContent = fileContent[:idx] + args.NewString + fileContent[idx+len(args.OldString):]
			}
		} else {
			// Replace first occurrence case-insensitively
			idx := strings.Index(strings.ToLower(fileContent), strings.ToLower(args.OldString))
			if idx != -1 {
				fileContent = fileContent[:idx] + args.NewString + fileContent[idx+len(args.OldString):]
				replacementCount = 1
			}
		}
	} else {
		// Case-sensitive replacement
		if args.ReplaceAll {
			replacementCount = strings.Count(fileContent, args.OldString)
			fileContent = strings.ReplaceAll(fileContent, args.OldString, args.NewString)
		} else {
			if strings.Contains(fileContent, args.OldString) {
				fileContent = strings.Replace(fileContent, args.OldString, args.NewString, 1)
				replacementCount = 1
			}
		}
	}

	// Check if any replacements were made
	if replacementCount == 0 {
		return "", NewToolError("STRING_NOT_FOUND", "The specified string was not found in the file").
			WithDetail("search_string", args.OldString).
			WithDetail("path", cleanPath)
	}

	// Check if content actually changed
	if fileContent == originalContent {
		return "No changes were made to the file", nil
	}

	// Write the modified content back
	if err := os.WriteFile(cleanPath, []byte(fileContent), 0644); err != nil {
		return "", NewToolError("WRITE_ERROR", "Failed to write changes to file").
			WithDetail("error", err.Error())
	}

	result := fmt.Sprintf("Successfully edited file: %s\n", cleanPath)
	result += fmt.Sprintf("Replaced %d occurrence(s) of '%s' with '%s'", 
		replacementCount, args.OldString, args.NewString)
	
	if args.CreateBackup {
		result += fmt.Sprintf("\nBackup created: %s.bak", cleanPath)
	}

	return result, nil
}

