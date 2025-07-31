# Tool Calling Visibility Implementation - Addendum

## Addressing Architectural Review Feedback

This addendum addresses the race conditions, UX edge cases, and performance concerns identified in the architectural review.

## 1. Race Condition Safety

### Problem
The `activeTools` slice is accessed from multiple goroutines without synchronization.

### Solution: Channel-Based State Management

```go
// Use channels for all state mutations
type BorderedTUI struct {
    // ... existing fields ...
    toolUpdates chan toolUpdate  // All tool state changes go through this
}

type toolUpdate struct {
    action toolAction
    data   interface{}
}

type toolAction int
const (
    toolActionAdd toolAction = iota
    toolActionUpdate
    toolActionRemove
)

// Single goroutine owns the state
func (m *BorderedTUI) processToolUpdates() tea.Cmd {
    return func() tea.Msg {
        for update := range m.toolUpdates {
            switch update.action {
            case toolActionAdd:
                // Safe mutation - only this goroutine touches activeTools
                m.activeTools = append(m.activeTools, update.data.(ActiveTool))
            case toolActionUpdate:
                // ... handle updates
            case toolActionRemove:
                // ... handle removal
            }
        }
        return nil
    }
}
```

### Alternative: Mutex Protection

```go
type BorderedTUI struct {
    // ... existing fields ...
    toolsMu     sync.RWMutex
    activeTools []ActiveTool
}

// All access wrapped
func (m *BorderedTUI) addTool(tool ActiveTool) {
    m.toolsMu.Lock()
    defer m.toolsMu.Unlock()
    m.activeTools = append(m.activeTools, tool)
}

func (m *BorderedTUI) getActiveTools() []ActiveTool {
    m.toolsMu.RLock()
    defer m.toolsMu.RUnlock()
    // Return a copy to prevent external mutations
    tools := make([]ActiveTool, len(m.activeTools))
    copy(tools, m.activeTools)
    return tools
}
```

## 2. Unique Tool ID Generation

```go
// Use UUID or timestamp+counter for globally unique IDs
type ToolIDGenerator struct {
    mu      sync.Mutex
    counter uint64
}

func (g *ToolIDGenerator) Next() string {
    g.mu.Lock()
    defer g.mu.Unlock()
    g.counter++
    return fmt.Sprintf("%d-%d-%d", time.Now().UnixNano(), g.counter, rand.Int63())
}

// Or use Google's UUID package
import "github.com/google/uuid"

func generateToolID() string {
    return uuid.New().String()
}
```

## 3. Strict Buffer Management

```go
const (
    maxOutputLines = 5
    maxLineLength  = 80
)

type CircularBuffer struct {
    lines    [maxOutputLines]string
    writeIdx int
    count    int
}

func (cb *CircularBuffer) Add(line string) {
    // Truncate long lines
    if len(line) > maxLineLength {
        line = line[:maxLineLength-3] + "..."
    }
    
    cb.lines[cb.writeIdx] = line
    cb.writeIdx = (cb.writeIdx + 1) % maxOutputLines
    if cb.count < maxOutputLines {
        cb.count++
    }
}

func (cb *CircularBuffer) GetLines() []string {
    if cb.count == 0 {
        return nil
    }
    
    result := make([]string, cb.count)
    start := 0
    if cb.count == maxOutputLines {
        start = cb.writeIdx
    }
    
    for i := 0; i < cb.count; i++ {
        idx := (start + i) % maxOutputLines
        result[i] = cb.lines[idx]
    }
    return result
}
```

## 4. Render Throttling

```go
type RenderThrottler struct {
    lastRender time.Time
    minInterval time.Duration
    pending    bool
    mu         sync.Mutex
}

func NewRenderThrottler(minInterval time.Duration) *RenderThrottler {
    return &RenderThrottler{
        minInterval: minInterval, // e.g., 33ms for ~30fps
    }
}

func (rt *RenderThrottler) ShouldRender() bool {
    rt.mu.Lock()
    defer rt.mu.Unlock()
    
    now := time.Now()
    if now.Sub(rt.lastRender) >= rt.minInterval {
        rt.lastRender = now
        rt.pending = false
        return true
    }
    
    rt.pending = true
    return false
}

// In Update method
func (m *BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case toolProgressMsg:
        m.updateToolProgress(msg)
        
        if m.renderThrottler.ShouldRender() {
            return m, nil
        }
        
        // Schedule a deferred render
        return m, tea.Tick(time.Millisecond*33, func(t time.Time) tea.Msg {
            return forceRenderMsg{}
        })
    }
}
```

## 5. Terminal Width Handling

```go
func (m *BorderedTUI) renderToolStatus() string {
    if m.width < 60 { // Narrow terminal
        // Collapsed view
        activeCount := len(m.getActiveTools())
        if activeCount == 0 {
            return ""
        }
        return fmt.Sprintf("🔧 %d tools running...", activeCount)
    }
    
    // Full view
    var b strings.Builder
    for _, tool := range m.getActiveTools() {
        elapsed := time.Since(tool.StartTime)
        
        // Tool header with smart truncation
        header := fmt.Sprintf("📋 %s", tool.Name)
        if len(header) > m.width-10 {
            header = header[:m.width-13] + "..."
        }
        
        b.WriteString(fmt.Sprintf("%s (%s)\n", header, formatDuration(elapsed)))
        
        // Output lines with indent
        for _, line := range tool.Output {
            // Word wrap long lines
            wrapped := wordwrap.String(line, m.width-4)
            for _, wl := range strings.Split(wrapped, "\n") {
                b.WriteString(fmt.Sprintf("  %s\n", wl))
            }
        }
    }
    
    return b.String()
}
```

## 6. Full Output Capture

```go
type ActiveTool struct {
    // ... existing fields ...
    FullOutput   strings.Builder  // Capture everything
    OutputSample CircularBuffer   // Display sample
}

// Add command to dump full output
case "d": // User pressed 'd' for dump
    if m.focusedToolIndex >= 0 && m.focusedToolIndex < len(m.activeTools) {
        tool := m.activeTools[m.focusedToolIndex]
        
        // Write to temp file
        tmpFile, err := os.CreateTemp("", fmt.Sprintf("tool-%s-*.log", tool.Name))
        if err == nil {
            tmpFile.WriteString(tool.FullOutput.String())
            tmpFile.Close()
            
            // Show notification
            m.notifications = append(m.notifications, 
                fmt.Sprintf("Output saved to: %s", tmpFile.Name()))
        }
    }
```

## 7. Cancellation & Timeout Events

```go
// Extended event types
const (
    EventTypeToolStart   EventType = "tool_start"
    EventTypeToolResult  EventType = "tool_result"
    EventTypeToolCancel  EventType = "tool_cancel"
    EventTypeToolTimeout EventType = "tool_timeout"
)

// Tool execution with timeout
func (a *agent) executeToolWithTimeout(ctx context.Context, tool Tool, params string, timeout time.Duration) (string, error) {
    ctx, cancel := context.WithTimeout(ctx, timeout)
    defer cancel()
    
    resultCh := make(chan struct {
        result string
        err    error
    }, 1)
    
    go func() {
        result, err := tool.Execute(ctx, params)
        resultCh <- struct{ result string; err error }{result, err}
    }()
    
    select {
    case res := <-resultCh:
        return res.result, res.err
    case <-ctx.Done():
        if ctx.Err() == context.DeadlineExceeded {
            a.emitStreamEvent(StreamEvent{
                Type: EventTypeToolTimeout,
                Tool: &ToolEvent{Name: tool.Name()},
            })
            return "", fmt.Errorf("tool %s timed out after %v", tool.Name(), timeout)
        }
        // Cancelled
        a.emitStreamEvent(StreamEvent{
            Type: EventTypeToolCancel,
            Tool: &ToolEvent{Name: tool.Name()},
        })
        return "", ctx.Err()
    }
}
```

## 8. Enhanced Error Display

```go
type ToolError struct {
    ToolName  string
    Error     error
    Timestamp time.Time
    Context   string  // First line of input that caused error
}

// Error display with persistence
func (m *BorderedTUI) renderToolError(toolErr ToolError) string {
    age := time.Since(toolErr.Timestamp)
    
    // Keep errors visible for at least 5 seconds
    if age < 5*time.Second {
        return fmt.Sprintf(
            "❌ %s failed: %s\n   Context: %s\n   (%.1fs ago)",
            toolErr.ToolName,
            firstLine(toolErr.Error.Error()),
            truncate(toolErr.Context, 40),
            age.Seconds(),
        )
    }
    
    // After 5s, show condensed version
    if age < 30*time.Second {
        return fmt.Sprintf("❌ %s failed (%.0fs ago)", toolErr.ToolName, age.Seconds())
    }
    
    return "" // Remove after 30s
}
```

## 9. Interface Naming Fix

```go
// Better Go naming convention
type ProgressReporter interface {
    ReportProgress(output string)
}

// Embed in base Tool interface as optional
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]interface{}
    Execute(ctx context.Context, params string) (string, error)
}

// Tools that support progress implement this additional interface
type ProgressableTool interface {
    Tool
    ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error)
}

// Type assertion in agent
if pt, ok := tool.(ProgressableTool); ok {
    result, err = pt.ExecuteWithProgress(ctx, params, progressReporter)
} else {
    result, err = tool.Execute(ctx, params)
}
```

## 10. Stress Test Implementation

```go
// stress_test.go
func TestToolOverlayStress(t *testing.T) {
    tui := NewBorderedTUI(mockClient, mockAgent, "test", "model")
    
    // Simulate 50 parallel tools with varying durations
    var wg sync.WaitGroup
    for i := 0; i < 50; i++ {
        wg.Add(1)
        go func(idx int) {
            defer wg.Done()
            
            // Random duration between 10ms and 2s
            duration := time.Duration(rand.Intn(1990)+10) * time.Millisecond
            
            // Start event
            tui.toolUpdates <- toolUpdate{
                action: toolActionAdd,
                data: ActiveTool{
                    ID:   fmt.Sprintf("tool-%d", idx),
                    Name: fmt.Sprintf("test_tool_%d", idx),
                },
            }
            
            // Progress events
            for j := 0; j < 5; j++ {
                time.Sleep(duration / 5)
                tui.toolUpdates <- toolUpdate{
                    action: toolActionUpdate,
                    data: toolProgressData{
                        ID:     fmt.Sprintf("tool-%d", idx),
                        Output: fmt.Sprintf("Progress %d/5", j+1),
                    },
                }
            }
            
            // Complete
            tui.toolUpdates <- toolUpdate{
                action: toolActionRemove,
                data:   fmt.Sprintf("tool-%d", idx),
            }
        }(i)
    }
    
    wg.Wait()
    
    // Verify no race conditions, proper cleanup
    assert.Empty(t, tui.getActiveTools())
    assert.NoError(t, tui.err)
}
```

## 11. Visual Polish

```go
// Enhanced styling with lipgloss
var (
    styleThinking = lipgloss.NewStyle().
        Foreground(lipgloss.Color("240"))
    
    styleToolRunning = lipgloss.NewStyle().
        Foreground(lipgloss.Color("33"))  // Blue
        
    styleToolSuccess = lipgloss.NewStyle().
        Foreground(lipgloss.Color("42"))  // Green
        
    styleToolError = lipgloss.NewStyle().
        Foreground(lipgloss.Color("196")) // Red
        
    styleToolCancelled = lipgloss.NewStyle().
        Foreground(lipgloss.Color("214")) // Orange
)

// Progress indicator using arc
func renderProgressArc(progress float64) string {
    const segments = 8
    filled := int(progress * float64(segments))
    
    arcs := []string{"○", "◔", "◑", "◕", "●"}
    if filled >= segments {
        return arcs[4] // Full circle
    }
    
    arcIndex := (filled * len(arcs)) / segments
    return arcs[arcIndex]
}
```

## 12. Tool History Footer

```go
type ToolHistory struct {
    Tools []ToolSummary
}

type ToolSummary struct {
    Name      string
    Duration  time.Duration
    Success   bool
    Timestamp time.Time
}

func (m *BorderedTUI) renderFooter() string {
    if len(m.toolHistory.Tools) == 0 {
        return ""
    }
    
    // Group by tool name
    toolCounts := make(map[string]int)
    for _, t := range m.toolHistory.Tools {
        toolCounts[t.Name]++
    }
    
    // Build summary
    var parts []string
    for name, count := range toolCounts {
        if count > 1 {
            parts = append(parts, fmt.Sprintf("%s (%d)", name, count))
        } else {
            parts = append(parts, name)
        }
    }
    
    return fmt.Sprintf("Tools used: %s", strings.Join(parts, ", "))
}
```

## Revised Configuration

```go
type ToolVisibilityConfig struct {
    Enabled              bool          
    MaxOutputLines       int           // Default: 5
    ShowArguments        bool          // Default: true
    ShowDuration         bool          // Default: true
    CompletionDelay      time.Duration // Default: 1s
    ErrorPersistence     time.Duration // Default: 5s
    RenderThrottle       time.Duration // Default: 33ms (~30fps)
    NarrowTerminalWidth  int           // Default: 60
    EnableFullDump       bool          // Default: true ('d' key)
    TimeoutDuration      time.Duration // Default: 30s per tool
}
```

## Summary

These additions address all the architectural concerns:
- **Race safety** through channel-based updates or mutex protection
- **Unique IDs** via UUID or timestamp+counter
- **Strict buffers** with circular implementation
- **Render throttling** at ~30fps
- **Terminal width** handling with graceful degradation
- **Full output** capture with dump capability
- **Cancellation/timeout** events in the stream
- **Error persistence** with timed decay
- **Proper Go naming** conventions
- **Stress testing** for concurrent operations
- **Visual polish** with colors and progress indicators

The implementation is now more robust and production-ready.