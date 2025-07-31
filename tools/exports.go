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
			ToolDesc: "Read the contents of a file from the filesystem",
		},
	}
}

// NewFileWriteTool creates a new file write tool
func NewFileWriteTool() Tool {
	return &FileWriteTool{
		BaseTool: base.BaseTool{
			ToolName: "file_write",
			ToolDesc: "Write content to a file, creating it if it doesn't exist",
		},
	}
}

// NewFileEditTool creates a new file edit tool  
func NewFileEditTool() Tool {
	return &FileEditTool{
		BaseTool: base.BaseTool{
			ToolName: "file_edit",
			ToolDesc: "Edit a file by replacing strings within it",
		},
	}
}

// NewDirectoryListTool creates a new directory list tool
func NewDirectoryListTool() Tool {
	return &DirectoryListTool{
		BaseTool: base.BaseTool{
			ToolName: "directory_list",
			ToolDesc: "List the contents of a directory with optional filtering",
		},
	}
}

// NewCalculateTool creates a new calculate tool
func NewCalculateTool() Tool {
	return &CalculateTool{
		BaseTool: base.BaseTool{
			ToolName: "calculate",
			ToolDesc: "Evaluate mathematical expressions safely",
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
			ToolDesc: "Search Wikipedia for information on any topic",
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
			ToolDesc: "Search Google and get detailed results with titles, URLs, and descriptions",
		},
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
		apiKey:         os.Getenv("GOOGLE_API_KEY"),
		searchEngineID: os.Getenv("GOOGLE_CX"),
	}
}