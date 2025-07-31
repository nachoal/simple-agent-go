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

// DirectoryListParams defines the parameters for the directory list tool
type DirectoryListParams struct {
	Path        string `json:"path" schema:"required" description:"Path to the directory to list"`
	Pattern     string `json:"pattern,omitempty" description:"Glob pattern to filter files (e.g., *.go)"`
	Recursive   bool   `json:"recursive,omitempty" description:"List files recursively"`
	ShowHidden  bool   `json:"show_hidden,omitempty" description:"Show hidden files (starting with .)"`
	ShowDetails bool   `json:"show_details,omitempty" description:"Show file details (size, permissions, etc.)"`
	MaxEntries  int    `json:"max_entries,omitempty" schema:"min:1,max:10000" description:"Maximum entries to return (default: 1000)"`
}

// DirectoryListTool lists directory contents
type DirectoryListTool struct {
	base.BaseTool
}


// Parameters returns the parameters struct
func (t *DirectoryListTool) Parameters() interface{} {
	return &DirectoryListParams{}
}

// Execute lists directory contents
func (t *DirectoryListTool) Execute(ctx context.Context, params json.RawMessage) (string, error) {
	var args DirectoryListParams
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

	// Check if directory exists
	info, err := os.Stat(cleanPath)
	if err != nil {
		if os.IsNotExist(err) {
			return "", NewToolError("DIR_NOT_FOUND", "Directory does not exist").
				WithDetail("path", cleanPath)
		}
		return "", NewToolError("ACCESS_ERROR", "Cannot access directory").
			WithDetail("error", err.Error())
	}

	if !info.IsDir() {
		return "", NewToolError("NOT_DIRECTORY", "Path is not a directory").
			WithDetail("path", cleanPath)
	}

	// Set default max entries
	maxEntries := args.MaxEntries
	if maxEntries == 0 {
		maxEntries = 1000
	}

	var entries []string
	count := 0

	if args.Recursive {
		// Walk directory recursively
		err = filepath.Walk(cleanPath, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // Skip entries with errors
			}

			// Check if we've reached max entries
			if count >= maxEntries {
				return filepath.SkipAll
			}

			// Skip hidden files if not requested
			if !args.ShowHidden && strings.HasPrefix(filepath.Base(path), ".") {
				if info.IsDir() && path != cleanPath {
					return filepath.SkipDir
				}
				return nil
			}

			// Apply pattern filter if specified
			if args.Pattern != "" {
				matched, err := filepath.Match(args.Pattern, filepath.Base(path))
				if err != nil || !matched {
					return nil
				}
			}

			// Format entry
			entry := t.formatEntry(path, info, cleanPath, args.ShowDetails)
			entries = append(entries, entry)
			count++

			return nil
		})
	} else {
		// List directory non-recursively
		dirEntries, err := os.ReadDir(cleanPath)
		if err != nil {
			return "", NewToolError("READ_ERROR", "Failed to read directory").
				WithDetail("error", err.Error())
		}

		for _, entry := range dirEntries {
			if count >= maxEntries {
				break
			}

			// Skip hidden files if not requested
			if !args.ShowHidden && strings.HasPrefix(entry.Name(), ".") {
				continue
			}

			// Apply pattern filter if specified
			if args.Pattern != "" {
				matched, err := filepath.Match(args.Pattern, entry.Name())
				if err != nil || !matched {
					continue
				}
			}

			// Get file info
			info, err := entry.Info()
			if err != nil {
				continue
			}

			// Format entry
			fullPath := filepath.Join(cleanPath, entry.Name())
			entryStr := t.formatEntry(fullPath, info, cleanPath, args.ShowDetails)
			entries = append(entries, entryStr)
			count++
		}
	}

	// Build result
	result := fmt.Sprintf("Directory: %s\n", cleanPath)
	result += fmt.Sprintf("Total entries: %d", count)
	if count >= maxEntries {
		result += fmt.Sprintf(" (limited to %d)", maxEntries)
	}
	result += "\n\n"

	if len(entries) == 0 {
		result += "No entries found"
		if args.Pattern != "" {
			result += fmt.Sprintf(" matching pattern '%s'", args.Pattern)
		}
	} else {
		result += strings.Join(entries, "\n")
	}

	return result, nil
}

func (t *DirectoryListTool) formatEntry(path string, info os.FileInfo, basePath string, showDetails bool) string {
	relPath, _ := filepath.Rel(basePath, path)
	if relPath == "." {
		relPath = filepath.Base(path)
	}

	if !showDetails {
		if info.IsDir() {
			return relPath + "/"
		}
		return relPath
	}

	// Format with details
	typeChar := "-"
	if info.IsDir() {
		typeChar = "d"
	} else if info.Mode()&os.ModeSymlink != 0 {
		typeChar = "l"
	}

	size := t.formatSize(info.Size())
	modTime := info.ModTime().Format("Jan 02 15:04")
	perms := info.Mode().Perm().String()

	return fmt.Sprintf("%s%s %10s %s %s", typeChar, perms, size, modTime, relPath)
}

func (t *DirectoryListTool) formatSize(bytes int64) string {
	const (
		KB = 1024
		MB = KB * 1024
		GB = MB * 1024
	)

	switch {
	case bytes >= GB:
		return fmt.Sprintf("%.1fG", float64(bytes)/GB)
	case bytes >= MB:
		return fmt.Sprintf("%.1fM", float64(bytes)/MB)
	case bytes >= KB:
		return fmt.Sprintf("%.1fK", float64(bytes)/KB)
	default:
		return fmt.Sprintf("%dB", bytes)
	}
}

