# Tool Calling Visibility Implementation Design

## Overview

This document outlines the design and implementation strategy for adding real-time tool calling visibility to the Simple Agent Go TUI. The goal is to provide users with better feedback during agent processing by displaying which tools are being called, their parameters, and partial outputs while maintaining a clean and responsive interface.

## Problem Statement

Currently, when users interact with the agent, they only see a "Thinking..." spinner during the entire processing phase. This creates an opaque experience where users cannot tell:
- Whether the agent is actually thinking or executing tools
- Which tools are being called
- If tools are executing in parallel
- What data tools are returning
- If something is stuck or taking longer than expected

## Design Goals

1. **Real-time Visibility**: Show tool execution status as it happens
2. **Non-intrusive Display**: Integrate seamlessly with the existing chat interface
3. **Performance**: No UI lag or blocking during updates
4. **Clarity**: Clear indication of tool names, status, and partial outputs
5. **Concurrency Support**: Handle parallel tool executions gracefully

## Implementation Approaches

### Approach 1: Stream-Based with Inline Tool Status Messages

**Description**: Modify the TUI to use `QueryStream` instead of `Query`, displaying tool events as inline messages in the chat.

**Architecture**:
```go
// Modify bordered.go to use streaming
func (m *BorderedTUI) sendMessage(input string) tea.Cmd {
    return func() tea.Msg {
        ctx := context.Background()
        events, err := m.agent.QueryStream(ctx, input)
        if err != nil {
            return borderedResponseMsg{err: err}
        }
        
        // Forward events to the UI
        go func() {
            for event := range events {
                m.program.Send(toolEventMsg{event: event})
            }
        }()
        
        return startStreamingMsg{}
    }
}
```

**Pros**:
- Leverages existing streaming infrastructure
- Events appear in natural chronological order
- Simple to implement and understand
- No additional UI components needed

**Cons**:
- Tool status messages intermixed with conversation
- Cannot easily update/remove temporary status
- May clutter the conversation history

### Approach 2: Dedicated Tool Status Panel (Split View)

**Description**: Add a dedicated panel (similar to a sidebar or bottom panel) that shows current tool executions.

**Architecture**:
```go
type BorderedTUI struct {
    // ... existing fields ...
    toolStatuses   map[string]ToolStatus  // Track active tools
    showToolPanel  bool                   // Toggle tool panel visibility
    toolPanelWidth int                    // Or height if bottom panel
}

type ToolStatus struct {
    Name      string
    StartTime time.Time
    Status    string  // "running", "completed", "failed"
    Output    string  // First N lines of output
    Progress  float64 // Optional progress indicator
}
```

**View Layout**:
```
┌─────────────────────────┬──────────────────┐
│                         │ Tool Status      │
│   Chat Messages         │ ─────────────    │
│                         │ 🔧 wikipedia     │
│                         │    searching...  │
│                         │                  │
│                         │ 🔧 google_search │
│                         │    3 results     │
└─────────────────────────┴──────────────────┘
│ > [Input Area]                            │
└───────────────────────────────────────────┘
```

**Pros**:
- Clean separation of concerns
- Can show multiple concurrent tools
- Persistent visibility during execution
- Professional appearance like IDEs

**Cons**:
- Reduces available chat space
- More complex layout management
- Requires resize handling for panel

### Approach 3: Ephemeral Status Overlays (Recommended)

**Description**: Display tool status as temporary overlays that appear below the thinking indicator and disappear when complete, leaving only a summary line in the chat.

**Architecture**:
```go
type BorderedTUI struct {
    // ... existing fields ...
    activeTools []ActiveTool  // Currently executing tools
}

type ActiveTool struct {
    ID        string
    Name      string
    Args      map[string]interface{}
    StartTime time.Time
    Output    []string  // Rolling buffer of output lines
    Status    ToolExecutionStatus
}

type ToolExecutionStatus int
const (
    ToolStatusPending ToolExecutionStatus = iota
    ToolStatusRunning
    ToolStatusComplete
    ToolStatusFailed
)
```

**Display Flow**:
```
1. Initial state:
   🔄 Thinking...

2. Tool execution starts:
   🔄 Thinking...
   
   📋 Calling wikipedia.search
   └─ query: "Golang concurrency patterns"

3. Tool producing output:
   🔄 Thinking...
   
   📋 wikipedia.search (running 2s)
   └─ Found 3 articles:
       - "Concurrency in Go"
       - "Go Patterns"
       ...

4. Tool completes:
   🔄 Thinking...
   
   ✅ wikipedia.search completed (2.3s)
   
   📋 google_search.query (running 0.5s)
   └─ Searching web...

5. All complete, show in chat:
   Assistant: Based on my research using Wikipedia and Google...
   [Tools used: wikipedia.search, google_search.query]
```

**Implementation Details**:

```go
// New message types for tool events
type toolStartMsg struct {
    toolID   string
    toolName string
    args     string
}

type toolProgressMsg struct {
    toolID string
    output  string
}

type toolCompleteMsg struct {
    toolID   string
    duration time.Duration
    success  bool
}

// Update the Update method
func (m *BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case toolStartMsg:
        m.activeTools = append(m.activeTools, ActiveTool{
            ID:        msg.toolID,
            Name:      msg.toolName,
            StartTime: time.Now(),
            Status:    ToolStatusRunning,
        })
        return m, tickCmd()
        
    case toolProgressMsg:
        for i, tool := range m.activeTools {
            if tool.ID == msg.toolID {
                // Update output buffer (keep last 5 lines)
                m.activeTools[i].Output = append(tool.Output, msg.output)
                if len(m.activeTools[i].Output) > 5 {
                    m.activeTools[i].Output = m.activeTools[i].Output[1:]
                }
                break
            }
        }
        return m, nil
        
    case toolCompleteMsg:
        // Mark tool as complete but keep in list briefly
        for i, tool := range m.activeTools {
            if tool.ID == msg.toolID {
                m.activeTools[i].Status = ToolStatusComplete
                // Remove after a delay
                return m, tea.Sequence(
                    tea.Tick(time.Second, func(t time.Time) tea.Msg {
                        return removeToolMsg{toolID: msg.toolID}
                    }),
                )
            }
        }
    }
}
```

**Pros**:
- Clean, uncluttered interface
- Progressive disclosure of information
- Handles parallel tools elegantly
- Maintains conversation readability
- Similar to modern chat UIs (ChatGPT, Claude)

**Cons**:
- More complex state management
- Requires careful timing for animations
- Need to track tool lifecycle

## Technical Considerations

### 1. Agent Modifications

The agent needs to emit more granular events during tool execution:

```go
// Modify agent/agent.go to emit tool events
func (a *agent) executeTools(ctx context.Context, toolCalls []llm.ToolCall) []tools.ToolResult {
    results := make([]tools.ToolResult, len(toolCalls))
    var wg sync.WaitGroup
    
    for i, tc := range toolCalls {
        wg.Add(1)
        go func(idx int, toolCall llm.ToolCall) {
            defer wg.Done()
            
            // Emit tool start event
            a.emitStreamEvent(StreamEvent{
                Type: EventTypeToolStart,
                Tool: &ToolEvent{
                    Name: toolCall.Function.Name,
                    Args: toolCall.Function.Arguments,
                },
            })
            
            // Execute with progress callback
            tool, _ := a.toolRegistry.Get(toolCall.Function.Name)
            result, err := tool.ExecuteWithProgress(ctx, toolCall.Function.Arguments, 
                func(output string) {
                    a.emitStreamEvent(StreamEvent{
                        Type: EventTypeToolProgress,
                        Tool: &ToolEvent{
                            Name:   toolCall.Function.Name,
                            Result: output,
                        },
                    })
                })
            
            // Emit completion
            a.emitStreamEvent(StreamEvent{
                Type: EventTypeToolResult,
                Tool: &ToolEvent{
                    Name:   toolCall.Function.Name,
                    Result: result,
                    Error:  err,
                },
            })
            
            results[idx] = tools.ToolResult{
                Name:   toolCall.Function.Name,
                Result: result,
                Error:  err,
            }
        }(i, tc)
    }
    
    wg.Wait()
    return results
}
```

### 2. Tool Interface Extension

Add optional progress reporting to tools:

```go
// tools/tool.go
type ProgressReporter func(output string)

type ToolWithProgress interface {
    Tool
    ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error)
}

// Example implementation for a tool
func (t *WikipediaTool) ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error) {
    reporter("Searching Wikipedia...")
    
    // Perform search
    results, err := t.search(params)
    if err != nil {
        return "", err
    }
    
    reporter(fmt.Sprintf("Found %d articles", len(results)))
    
    // Continue processing...
    return t.formatResults(results), nil
}
```

### 3. Streaming Infrastructure

Enhance the streaming to support bidirectional communication:

```go
// agent/stream.go
type StreamManager struct {
    events   chan StreamEvent
    commands chan StreamCommand
    agent    *agent
}

type StreamCommand struct {
    Type    StreamCommandType
    Payload interface{}
}

func (sm *StreamManager) Start(ctx context.Context, query string) {
    go func() {
        // Process query and emit events
        response := sm.agent.processWithEvents(ctx, query, sm.events)
        sm.events <- StreamEvent{
            Type:    EventTypeComplete,
            Content: response.Content,
        }
        close(sm.events)
    }()
}
```

### 4. Bubble Tea Event Handling

Use Bubble Tea's subscription model for real-time updates:

```go
// Subscribe to agent events
func (m *BorderedTUI) subscribeToAgentEvents() tea.Cmd {
    return func() tea.Msg {
        // This runs in a goroutine
        for event := range m.eventChannel {
            m.program.Send(agentEventMsg{event: event})
        }
        return nil
    }
}
```

## Edge Cases and Error Handling

### 1. Rapid Tool Calls
When multiple tools are called in quick succession, ensure the UI doesn't flicker:
- Batch updates within a time window (e.g., 100ms)
- Use animation transitions for smooth appearance/disappearance

### 2. Long-Running Tools
For tools that take significant time:
- Show elapsed time counter
- Provide timeout indicators
- Allow user to see more detailed progress

### 3. Failed Tools
Clear indication of failures:
- Red color or ❌ icon for failed tools
- Show error summary (not full stack traces)
- Maintain in view briefly before removal

### 4. Parallel Tool Execution
When tools run concurrently:
- Stack tool status displays vertically
- Show which tools are running simultaneously
- Indicate when all tools are complete

### 5. Terminal Resize
Handle terminal resize gracefully:
- Truncate tool output to fit available space
- Maintain tool status visibility
- Reflow text appropriately

## Implementation Plan

### Phase 1: Core Infrastructure
1. Extend agent to use streaming for all queries
2. Add tool progress events to StreamEvent types
3. Implement basic event emission in agent

### Phase 2: TUI Integration
1. Modify BorderedTUI to use QueryStream
2. Add tool status tracking data structures
3. Implement basic tool status display

### Phase 3: Enhanced Display
1. Add animations and transitions
2. Implement output buffering and truncation
3. Add parallel execution indicators

### Phase 4: Polish
1. Add configuration options (show/hide tool panel)
2. Implement keyboard shortcuts for tool view
3. Add tool execution history

## Testing Strategy

### Unit Tests
- Test event emission from agent
- Test message ordering and buffering
- Test error scenarios

### Integration Tests
- Test complete flow from query to display
- Test parallel tool execution
- Test UI responsiveness under load

### Manual Testing
- Test with various terminal sizes
- Test with long-running tools
- Test with tools that produce lots of output

## Performance Considerations

1. **Event Buffering**: Implement a circular buffer for tool outputs to prevent memory growth
2. **Render Optimization**: Only re-render changed portions of the UI
3. **Goroutine Management**: Properly manage goroutines to prevent leaks
4. **Channel Buffering**: Use buffered channels for event flow to prevent blocking

## Configuration Options

```go
type ToolVisibilityConfig struct {
    Enabled              bool          // Toggle feature on/off
    MaxOutputLines       int           // Max lines to show per tool
    ShowArguments        bool          // Show tool arguments
    ShowDuration         bool          // Show execution time
    CompletionDelay      time.Duration // How long to show completed tools
    ParallelIndicator    string        // Symbol for parallel execution
    TruncateOutput       bool          // Truncate long outputs
    OutputTruncateLength int           // Character limit for output
}
```

## Conclusion

The recommended approach (#3 - Ephemeral Status Overlays) provides the best balance of functionality, user experience, and implementation complexity. It offers:

- Clean, uncluttered interface that doesn't interfere with conversation flow
- Real-time visibility into tool execution
- Support for parallel tool execution
- Progressive disclosure of information
- Familiar UX pattern from modern AI chat interfaces

This approach requires moderate changes to the agent's streaming infrastructure and the TUI's update cycle, but results in a professional, informative interface that significantly improves the user experience during tool execution.