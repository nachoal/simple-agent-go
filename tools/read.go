package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/nachoal/simple-agent-go/tools/base"
)

const (
	defaultReadMaxLines = 2000
	defaultReadMaxBytes = 50 * 1024
)

type ReadParams struct {
	Path   string `json:"path" schema:"required" description:"Path to the file to read (relative or absolute)"`
	Offset int    `json:"offset,omitempty" description:"Line number to start reading from (1-indexed)"`
	Limit  int    `json:"limit,omitempty" description:"Maximum number of lines to read"`
}

// ReadTool reads file contents.
type ReadTool struct {
	base.BaseTool
}

// Parameters returns the parameters struct
func (t *ReadTool) Parameters() interface{} {
	return &ReadParams{}
}

func truncateUTF8Head(s string, maxBytes int) (string, bool) {
	if maxBytes <= 0 {
		return "", true
	}
	if len(s) <= maxBytes {
		return s, false
	}

	var b bytes.Buffer
	b.Grow(maxBytes)
	truncated := false

	for i := 0; i < len(s); {
		r, size := utf8.DecodeRuneInString(s[i:])
		if r == utf8.RuneError && size == 1 {
			// Invalid UTF-8 byte; skip it to avoid producing invalid output.
			i++
			truncated = true
			continue
		}
		if b.Len()+size > maxBytes {
			truncated = true
			break
		}
		b.WriteRune(r)
		i += size
	}

	return b.String(), truncated
}

// Execute reads a file and returns its contents (with truncation + offset/limit).
func (t *ReadTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args ReadParams
	if err := json.Unmarshal(params, &args); err != nil {
		return "", NewToolError("INVALID_PARAMS", "Failed to parse parameters").
			WithDetail("error", err.Error())
	}

	if args.Path == "" {
		return "", NewToolError("VALIDATION_FAILED", "Path cannot be empty")
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

	// Read file
	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return "", NewToolError("READ_ERROR", "Error reading file").
			WithDetail("error", err.Error())
	}

	_ = ctx // currently unused

	text := string(content)
	// Normalize line endings to \n
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	totalLines := len(lines)
	if totalLines == 0 {
		return "", nil
	}

	startLine := 1
	if args.Offset > 0 {
		startLine = args.Offset
	}
	if startLine < 1 {
		startLine = 1
	}
	if startLine > totalLines {
		return "", NewToolError("INVALID_OFFSET", "Offset is beyond end of file").
			WithDetail("offset", startLine).
			WithDetail("total_lines", totalLines)
	}

	limit := args.Limit
	if limit <= 0 {
		limit = defaultReadMaxLines
	}

	endLine := startLine + limit - 1
	if endLine > totalLines {
		endLine = totalLines
	}

	selected := strings.Join(lines[startLine-1:endLine], "\n")
	selected, bytesTruncated := truncateUTF8Head(selected, defaultReadMaxBytes)

	output := selected
	if endLine < totalLines || bytesTruncated {
		nextOffset := endLine + 1
		if nextOffset <= totalLines {
			if bytesTruncated {
				output += fmt.Sprintf("\n\n[Output truncated at %dKB. Showing lines %d-%d of %d. Use offset=%d to continue.]", defaultReadMaxBytes/1024, startLine, endLine, totalLines, nextOffset)
			} else {
				output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d. Use offset=%d to continue.]", startLine, endLine, totalLines, nextOffset)
			}
		} else {
			output += fmt.Sprintf("\n\n[Showing lines %d-%d of %d.]", startLine, endLine, totalLines)
		}
	}

	return output, nil
}
