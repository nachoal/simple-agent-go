package toolinit

import (
	"github.com/nachoal/simple-agent-go/tools"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// RegisterAll registers all built-in tools
func RegisterAll() {
	// File operations
	registry.Register("read", func() tools.Tool {
		return tools.NewReadTool()
	})

	registry.Register("write", func() tools.Tool {
		return tools.NewWriteTool()
	})

	registry.Register("edit", func() tools.Tool {
		return tools.NewEditTool()
	})

	registry.Register("directory_list", func() tools.Tool {
		return tools.NewDirectoryListTool()
	})

	// Utility tools
	registry.Register("calculate", func() tools.Tool {
		return tools.NewCalculateTool()
	})

	registry.Register("bash", func() tools.Tool {
		return tools.NewBashTool()
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
