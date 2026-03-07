package tools

import (
	"os"
	"path/filepath"
	"strings"
)

func currentWorkspaceRoot() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", NewToolError("WORKSPACE_UNAVAILABLE", "Failed to determine current working directory").
			WithDetail("error", err.Error())
	}
	return filepath.Clean(cwd), nil
}

func resolveWorkspacePath(path string) (string, string, error) {
	workspace, err := currentWorkspaceRoot()
	if err != nil {
		return "", "", err
	}

	raw := strings.TrimSpace(path)
	if raw == "" {
		return "", "", NewToolError("VALIDATION_FAILED", "Path cannot be empty")
	}

	clean := filepath.Clean(raw)
	resolved := clean
	if !filepath.IsAbs(clean) {
		resolved = filepath.Join(workspace, clean)
	}
	resolved = filepath.Clean(resolved)

	rel, relErr := filepath.Rel(workspace, resolved)
	if relErr != nil {
		return "", "", NewToolError("PATH_RESOLUTION_FAILED", "Failed to resolve path relative to current working directory").
			WithDetail("path", raw).
			WithDetail("workspace", workspace).
			WithDetail("error", relErr.Error())
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", "", NewToolError("PATH_OUTSIDE_WORKSPACE", "Path must stay within the current working directory").
			WithDetail("path", raw).
			WithDetail("workspace", workspace)
	}

	return resolved, workspace, nil
}

func displayPathForWorkspace(path, workspace string) string {
	if path == "" || workspace == "" {
		return path
	}

	rel, err := filepath.Rel(workspace, path)
	if err != nil {
		return path
	}
	if rel == "." {
		return "."
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return path
	}
	return rel
}
