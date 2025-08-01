package tools

import "context"

// ProgressReporter allows tools to report execution progress
type ProgressReporter interface {
	// ReportProgress reports a text progress update
	ReportProgress(message string)
	
	// ReportProgressPercent reports progress with a percentage (0-1)
	ReportProgressPercent(message string, percent float64)
}

// ProgressableTool is an optional interface for tools that support progress reporting
type ProgressableTool interface {
	Tool
	// ExecuteWithProgress executes the tool with progress reporting
	ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error)
}