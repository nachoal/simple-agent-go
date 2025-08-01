package toolinit

import (
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// RegisterAll registers all built-in tools
func RegisterAll() {
	// File operations
	registry.Register("file_read", func() tools.Tool {
		return tools.NewFileReadTool()
	})
	
	registry.Register("file_write", func() tools.Tool {
		return tools.NewFileWriteTool()
	})
	
	registry.Register("file_edit", func() tools.Tool {
		return tools.NewFileEditTool()
	})
	
	registry.Register("directory_list", func() tools.Tool {
		return tools.NewDirectoryListTool()
	})
	
	// Utility tools
	registry.Register("calculate", func() tools.Tool {
		return tools.NewCalculateTool()
	})
	
	registry.Register("shell", func() tools.Tool {
		return tools.NewShellTool()
	})
	
	// Search tools
	registry.Register("wikipedia", func() tools.Tool {
		return tools.NewWikipediaTool()
	})
	
	registry.Register("google_search", func() tools.Tool {
		return tools.NewGoogleSearchTool()
	})
	
	// Demo tool for testing
	// Temporarily disabled due to schema issues
	// registry.Register("demo_tool", func() tools.Tool {
	// 	return tools.NewDemoTool()
	// })
}