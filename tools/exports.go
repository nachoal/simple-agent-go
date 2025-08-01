package tools

import (
	"net/http"
	"os"
	"time"
	
	"github.com/nachoal/simple-agent-go/tools/base"
)

// Export tool constructors to avoid import cycles
// These are implemented in their respective files but exported here

// NewFileReadTool creates a new file read tool
func NewFileReadTool() Tool {
	return &FileReadTool{
		BaseTool: base.BaseTool{
			ToolName: "file_read",
			ToolDesc: "Read the contents of a file. Input must be JSON with 'path' field. Example: {\"path\": \"file.txt\"}",
		},
	}
}

// NewFileWriteTool creates a new file write tool
func NewFileWriteTool() Tool {
	return &FileWriteTool{
		BaseTool: base.BaseTool{
			ToolName: "file_write",
			ToolDesc: "Write content to a file, creating it if it doesn't exist. This overwrites the entire file content. Input should be a JSON string with 'path' and 'content' fields.",
		},
	}
}

// NewFileEditTool creates a new file edit tool  
func NewFileEditTool() Tool {
	return &FileEditTool{
		BaseTool: base.BaseTool{
			ToolName: "file_edit",
			ToolDesc: "Edit a file by replacing old_str with new_str. Input must be JSON with 'path', 'old_str', and 'new_str' fields. Example: {\"path\": \"file.txt\", \"old_str\": \"old\", \"new_str\": \"new\"}",
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

// NewShellTool creates a new shell tool
func NewShellTool() Tool {
	// Default allowed commands for safety
	allowedCommands := []string{
		"ls", "cat", "grep", "find", "echo", "pwd", "date",
		"wc", "sort", "head", "tail", "awk", "sed", "cut",
		"diff", "file", "which", "env", "printenv",
	}
	
	return &ShellTool{
		BaseTool: base.BaseTool{
			ToolName: "shell",
			ToolDesc: "Execute shell commands safely with timeout and output capture",
		},
		allowedCommands: allowedCommands,
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

// NewDemoTool creates a new demo tool for testing
func NewDemoTool() Tool {
	return &DemoTool{
		BaseTool: base.BaseTool{
			ToolName: "demo_tool",
			ToolDesc: "Demo tool for testing tool visibility. Supports operations: fast, slow, error. Can simulate long-running tasks with progress.",
		},
	}
}