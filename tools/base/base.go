package base

// BaseTool provides common functionality for tools
type BaseTool struct {
	ToolName string
	ToolDesc string
}

// Name returns the tool name
func (b *BaseTool) Name() string {
	return b.ToolName
}

// Description returns the tool description
func (b *BaseTool) Description() string {
	return b.ToolDesc
}