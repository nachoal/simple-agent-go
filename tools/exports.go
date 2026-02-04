package tools

import (
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/tools/base"
)

// Export tool constructors to avoid import cycles
// These are implemented in their respective files but exported here

// NewReadTool creates a new read tool.
func NewReadTool() Tool {
	return &ReadTool{
		BaseTool: base.BaseTool{
			ToolName: "read",
			ToolDesc: "Read the contents of a file. Supports optional offset/limit for large files. Example: {\"path\":\"file.txt\",\"offset\":1,\"limit\":200}",
		},
	}
}

// NewWriteTool creates a new write tool.
func NewWriteTool() Tool {
	return &WriteTool{
		BaseTool: base.BaseTool{
			ToolName: "write",
			ToolDesc: "Create or overwrite a file. Creates parent directories. Example: {\"path\":\"file.txt\",\"content\":\"hello\"}",
		},
	}
}

// NewEditTool creates a new edit tool.
func NewEditTool() Tool {
	return &EditTool{
		BaseTool: base.BaseTool{
			ToolName: "edit",
			ToolDesc: "Edit a file by replacing exact oldText with newText (must be unique). Example: {\"path\":\"file.txt\",\"oldText\":\"old\",\"newText\":\"new\"}",
		},
	}
}

// NewDirectoryListTool creates a new directory list tool
func NewDirectoryListTool() Tool {
	return &DirectoryListTool{
		BaseTool: base.BaseTool{
			ToolName: "directory_list",
			ToolDesc: "List files and directories. Input must be JSON with optional 'path' field. Example: {\"path\": \"directory\"} or {} for current directory.",
		},
	}
}

// NewCalculateTool creates a new calculate tool
func NewCalculateTool() Tool {
	return &CalculateTool{
		BaseTool: base.BaseTool{
			ToolName: "calculate",
			ToolDesc: "Evaluates mathematical expressions with support for basic operators (+, -, *, /, %, **) and parentheses.",
		},
	}
}

// NewBashTool creates a new bash tool.
func NewBashTool() Tool {
	yolo := strings.EqualFold(os.Getenv("SIMPLE_AGENT_YOLO"), "true") ||
		os.Getenv("SIMPLE_AGENT_YOLO") == "1" ||
		strings.EqualFold(os.Getenv("SIMPLE_AGENT_YOLO"), "yes")

	// Default allowed commands for safety
	allowedCommands := []string{
		"ls", "cat", "grep", "find", "echo", "pwd", "date",
		"wc", "sort", "head", "tail", "awk", "sed", "cut",
		"diff", "file", "which", "env", "printenv",
	}

	desc := "Execute bash commands safely with timeout and output capture. Example: {\"command\":\"ls -la\",\"timeout\":30}"
	if yolo {
		desc = "Execute bash commands (UNSAFE: --yolo enabled; any command allowed) with timeout and output capture. Example: {\"command\":\"ls -la\",\"timeout\":30}"
	}

	return &BashTool{
		BaseTool: base.BaseTool{
			ToolName: "bash",
			ToolDesc: desc,
		},
		allowedCommands: allowedCommands,
		allowAll:        yolo,
	}
}

// NewWikipediaTool creates a new Wikipedia search tool
func NewWikipediaTool() Tool {
	return &WikipediaTool{
		BaseTool: base.BaseTool{
			ToolName: "wikipedia",
			ToolDesc: "Searches Wikipedia for the given query and returns the snippet of the most relevant article match.",
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// NewGoogleSearchTool creates a new Google search tool
func NewGoogleSearchTool() Tool {
	return &GoogleSearchTool{
		BaseTool: base.BaseTool{
			ToolName: "google_search",
			ToolDesc: "Performs a Google search using Custom Search API and returns detailed results including titles, URLs, descriptions, and metadata for up to 10 results.",
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey:         os.Getenv("GOOGLE_API_KEY"),
		searchEngineID: os.Getenv("GOOGLE_CX"),
	}
}
