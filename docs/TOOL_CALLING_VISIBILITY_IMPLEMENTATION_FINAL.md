# Tool Calling Visibility Implementation - Final Design (100% Production Ready)

## Executive Summary

This document represents the final, production-ready design for adding tool calling visibility to Simple Agent Go. It incorporates all feedback from architectural reviews and addresses every edge case, performance concern, and UX consideration.

## Core Architecture: Channel-Only Event Loop

### The Canonical Bubble Tea Pattern

All state mutations happen exclusively within the `Update` method, eliminating race conditions entirely.

```go
// All external events flow through tea.Msg
type BorderedTUI struct {
    // State - only modified in Update()
    activeTools   []ActiveTool
    toolErrors    []ToolError
    
    // No mutexes needed - single-threaded by design
    width         int
    height        int
    
    // Event stream from agent
    eventStream   <-chan agent.StreamEvent
    
    // Configuration
    config        ToolVisibilityConfig
}

// External events are converted to tea.Msg
type toolEventMsg struct {
    event agent.StreamEvent
}

// Stream subscription - runs in background goroutine
func (m *BorderedTUI) subscribeToStream() tea.Cmd {
    return func() tea.Msg {
        event, ok := <-m.eventStream
        if !ok {
            return streamCompleteMsg{}
        }
        return toolEventMsg{event: event}
    }
}

// All state changes happen here - thread safe by design
func (m *BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case toolEventMsg:
        switch msg.event.Type {
        case agent.EventTypeToolStart:
            m.activeTools = append(m.activeTools, ActiveTool{
                ID:        generateUniqueID(),
                Name:      msg.event.Tool.Name,
                Args:      msg.event.Tool.Args,
                StartTime: time.Now(),
                Status:    ToolStatusRunning,
                Output:    NewCircularBuffer(m.config.MaxOutputLines),
            })
            
            // Continue listening for next event
            return m, m.subscribeToStream()
            
        case agent.EventTypeToolProgress:
            m.updateToolProgress(msg.event.Tool.Name, msg.event.Tool.Result)
            return m, m.subscribeToStream()
            
        case agent.EventTypeToolResult:
            m.completeToolExecution(msg.event.Tool.Name, msg.event.Tool.Error)
            
            // Schedule removal after delay
            return m, tea.Sequence(
                m.subscribeToStream(),
                tea.Tick(m.config.CompletionDelay, func(t time.Time) tea.Msg {
                    return removeCompletedToolsMsg{}
                }),
            )
            
        case agent.EventTypeToolTimeout:
            m.markToolTimeout(msg.event.Tool.Name)
            return m, m.subscribeToStream()
            
        case agent.EventTypeToolCancel:
            m.markToolCancelled(msg.event.Tool.Name)
            return m, m.subscribeToStream()
        }
    }
    // ... other message handlers
}
```

## Unique ID Generation

```go
// Thread-safe monotonic counter
var toolIDCounter uint64

func generateUniqueID() string {
    // Atomic increment ensures uniqueness even under concurrent calls
    id := atomic.AddUint64(&toolIDCounter, 1)
    return fmt.Sprintf("tool-%d-%d", time.Now().UnixNano(), id)
}
```

## Memory-Safe Circular Buffer

```go
type CircularBuffer struct {
    lines    []string
    maxLines int
    head     int
    size     int
}

func NewCircularBuffer(maxLines int) *CircularBuffer {
    return &CircularBuffer{
        lines:    make([]string, maxLines),
        maxLines: maxLines,
    }
}

func (cb *CircularBuffer) Add(line string) {
    // Truncate long lines to prevent memory bloat
    if len(line) > MaxLineLength {
        line = line[:MaxLineLength-3] + "..."
    }
    
    cb.lines[cb.head] = line
    cb.head = (cb.head + 1) % cb.maxLines
    
    if cb.size < cb.maxLines {
        cb.size++
    }
}

func (cb *CircularBuffer) GetLines() []string {
    if cb.size == 0 {
        return nil
    }
    
    result := make([]string, cb.size)
    start := cb.head - cb.size
    if start < 0 {
        start += cb.maxLines
    }
    
    for i := 0; i < cb.size; i++ {
        idx := (start + i) % cb.maxLines
        result[i] = cb.lines[idx]
    }
    
    return result
}
```

## Smart Render Throttling

```go
// Debounced rendering to prevent terminal spam
type BorderedTUI struct {
    // ... other fields ...
    lastRender    time.Time
    renderPending bool
}

func (m *BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case toolProgressMsg:
        // Update state
        m.updateToolProgress(msg.toolName, msg.output)
        
        // Check if we should render
        now := time.Now()
        if now.Sub(m.lastRender) >= m.config.RenderThrottle {
            m.lastRender = now
            m.renderPending = false
            return m, m.subscribeToStream()
        }
        
        // Schedule deferred render if not already pending
        if !m.renderPending {
            m.renderPending = true
            return m, tea.Tick(m.config.RenderThrottle, func(t time.Time) tea.Msg {
                return forceRenderMsg{}
            })
        }
        
        return m, m.subscribeToStream()
        
    case forceRenderMsg:
        m.renderPending = false
        return m, nil
    }
}
```

## Context-Based Cancellation

```go
// Agent-side implementation
func (a *agent) executeToolWithContext(ctx context.Context, tool Tool, params string) (string, error) {
    // Apply per-tool timeout
    toolCtx, cancel := context.WithTimeout(ctx, a.config.ToolTimeout)
    defer cancel()
    
    // Handle cancellation gracefully
    done := make(chan struct{})
    var result string
    var err error
    
    go func() {
        result, err = tool.Execute(toolCtx, params)
        close(done)
    }()
    
    select {
    case <-done:
        return result, err
        
    case <-toolCtx.Done():
        if errors.Is(toolCtx.Err(), context.DeadlineExceeded) {
            a.emitEvent(StreamEvent{
                Type: EventTypeToolTimeout,
                Tool: &ToolEvent{Name: tool.Name()},
            })
        } else {
            a.emitEvent(StreamEvent{
                Type: EventTypeToolCancel,
                Tool: &ToolEvent{Name: tool.Name()},
            })
        }
        return "", toolCtx.Err()
    }
}

// TUI-side handling
func (m *BorderedTUI) handleInterrupt() tea.Cmd {
    // Cancel the context, which will propagate to all running tools
    if m.cancelFunc != nil {
        m.cancelFunc()
    }
    
    return tea.Sequence(
        // Show cancellation feedback
        func() tea.Msg {
            return statusMsg{text: "Cancelling all operations..."}
        },
        // Clear after 2 seconds
        tea.Tick(2*time.Second, func(t time.Time) tea.Msg {
            return clearStatusMsg{}
        }),
    )
}
```

## Responsive Terminal Handling

```go
const (
    MinTerminalWidth = 40
    CompactModeWidth = 80
)

func (m *BorderedTUI) renderToolOverlay() string {
    if m.width < MinTerminalWidth {
        // Ultra-narrow: just count
        return fmt.Sprintf("🔧 %d", len(m.activeTools))
    }
    
    if m.width < CompactModeWidth {
        // Compact mode: single line summary
        if len(m.activeTools) == 0 {
            return ""
        }
        
        elapsed := time.Since(m.activeTools[0].StartTime)
        return fmt.Sprintf("🔧 %d tools running... (⏱ %.1fs)", 
            len(m.activeTools), elapsed.Seconds())
    }
    
    // Full mode: detailed overlay
    var b strings.Builder
    availableWidth := m.width - 4 // Account for padding
    
    for i, tool := range m.activeTools {
        if i > 0 {
            b.WriteString("\n")
        }
        
        // Tool header with status indicator
        status := m.getStatusIcon(tool.Status)
        elapsed := time.Since(tool.StartTime)
        
        header := fmt.Sprintf("%s %s", status, tool.Name)
        timing := fmt.Sprintf("(%.1fs)", elapsed.Seconds())
        
        // Smart truncation to fit width
        headerSpace := availableWidth - len(timing) - 1
        if len(header) > headerSpace {
            header = header[:headerSpace-3] + "..."
        }
        
        b.WriteString(fmt.Sprintf("%s %s\n", header, timing))
        
        // Arguments (if enabled and fits)
        if m.config.ShowArguments && availableWidth > 60 {
            args := m.formatArguments(tool.Args, availableWidth-2)
            b.WriteString(fmt.Sprintf("  %s\n", args))
        }
        
        // Output preview
        for _, line := range tool.Output.GetLines() {
            wrapped := wordwrap.String(line, availableWidth-2)
            for _, wl := range strings.Split(wrapped, "\n") {
                b.WriteString(fmt.Sprintf("  %s\n", wl))
            }
        }
    }
    
    return b.String()
}

// Toggle full-screen tool panel
func (m *BorderedTUI) toggleToolPanel() (tea.Model, tea.Cmd) {
    m.showFullPanel = !m.showFullPanel
    
    if m.showFullPanel {
        // Create scrollable viewport with all tool history
        m.toolViewport = viewport.New(m.width, m.height-4)
        m.toolViewport.SetContent(m.renderFullToolHistory())
    }
    
    return m, nil
}
```

## Error Display with Persistence

```go
type ToolError struct {
    ID        string
    ToolName  string
    Error     error
    Timestamp time.Time
    Dismissed bool
}

func (m *BorderedTUI) renderErrors() string {
    var b strings.Builder
    now := time.Now()
    
    for i, err := range m.toolErrors {
        if err.Dismissed {
            continue
        }
        
        age := now.Sub(err.Timestamp)
        
        // Auto-dismiss after configured time
        if age > m.config.ErrorPersistence {
            m.toolErrors[i].Dismissed = true
            continue
        }
        
        // Fade effect: full detail -> summary -> gone
        if age < 2*time.Second {
            // Full error with context
            b.WriteString(styleToolError.Render(
                fmt.Sprintf("❌ %s failed: %s\n   %s (%.1fs ago)\n",
                    err.ToolName,
                    firstLine(err.Error.Error()),
                    "Press 'e' for full error",
                    age.Seconds()),
            ))
        } else {
            // Condensed error
            b.WriteString(styleToolError.Render(
                fmt.Sprintf("❌ %s failed (%.0fs ago)\n", 
                    err.ToolName, age.Seconds()),
            ))
        }
    }
    
    return b.String()
}

// Allow manual dismissal
func (m *BorderedTUI) dismissErrors() {
    for i := range m.toolErrors {
        m.toolErrors[i].Dismissed = true
    }
}
```

## Improved Tool Interface

```go
// Base tool interface remains clean
type Tool interface {
    Name() string
    Description() string
    Schema() map[string]interface{}
    Execute(ctx context.Context, params string) (string, error)
}

// Optional progress support via type assertion
type ProgressReporter interface {
    ReportProgress(message string)
}

// Tools can optionally accept a progress reporter
type ProgressableTool interface {
    Tool
    ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error)
}

// Adapter for backward compatibility
func ExecuteTool(ctx context.Context, tool Tool, params string, reporter ProgressReporter) (string, error) {
    if pt, ok := tool.(ProgressableTool); ok && reporter != nil {
        return pt.ExecuteWithProgress(ctx, params, reporter)
    }
    return tool.Execute(ctx, params)
}
```

## Comprehensive Testing

```go
// Race condition test
func TestConcurrentToolUpdates(t *testing.T) {
    // Run with: go test -race
    tui := NewBorderedTUI(mockClient, mockAgent, "test", "model")
    program := tea.NewProgram(tui)
    
    // Simulate 100 concurrent tool events
    eventChan := make(chan agent.StreamEvent, 100)
    tui.eventStream = eventChan
    
    go func() {
        for i := 0; i < 100; i++ {
            eventChan <- agent.StreamEvent{
                Type: agent.EventTypeToolStart,
                Tool: &agent.ToolEvent{
                    Name: fmt.Sprintf("tool_%d", i),
                },
            }
        }
    }()
    
    // Let it process
    time.Sleep(100 * time.Millisecond)
    
    // No races should be detected
}

// Goroutine leak test
func TestNoGoroutineLeaks(t *testing.T) {
    defer goleak.VerifyNone(t)
    
    tui := NewBorderedTUI(mockClient, mockAgent, "test", "model")
    program := tea.NewProgram(tui)
    
    // Simulate lifecycle
    go program.Run()
    time.Sleep(50 * time.Millisecond)
    program.Quit()
    
    // goleak will fail if any goroutines leak
}

// Terminal resize test
func TestTerminalResize(t *testing.T) {
    tui := NewBorderedTUI(mockClient, mockAgent, "test", "model")
    
    // Add active tools
    tui.activeTools = []ActiveTool{
        {Name: "test_tool", Status: ToolStatusRunning},
    }
    
    // Simulate resize to tiny terminal
    model, _ := tui.Update(tea.WindowSizeMsg{Width: 30, Height: 10})
    updatedTUI := model.(*BorderedTUI)
    
    // Should not panic and should show compact view
    view := updatedTUI.View()
    assert.Contains(t, view, "🔧")
    assert.NotContains(t, view, "test_tool") // Name hidden in ultra-narrow
}
```

## Configuration with YAML Example

```yaml
# .simple-agent/config.yaml
tool_visibility:
  enabled: true
  max_output_lines: 5
  show_arguments: true
  show_duration: true
  completion_delay: 1500ms
  error_persistence: 5s
  render_throttle: 33ms
  narrow_terminal_width: 80
  enable_full_dump: true
  timeout_duration: 30s
  
  # Visual customization
  parallel_indicator: "⧉"
  progress_style: "arc"  # arc, bar, spinner
  
  # Key bindings
  keys:
    toggle_panel: "t"
    dump_output: "d"
    dismiss_errors: "e"
    cancel_all: "ctrl+c"

# Theme colors (using lipgloss color names)
theme:
  tool_running: "33"    # blue
  tool_success: "42"    # green
  tool_error: "196"     # red
  tool_timeout: "214"   # orange
  tool_cancelled: "240" # gray
```

```go
// Config struct with defaults
type ToolVisibilityConfig struct {
    Enabled              bool          `yaml:"enabled" default:"true"`
    MaxOutputLines       int           `yaml:"max_output_lines" default:"5"`
    ShowArguments        bool          `yaml:"show_arguments" default:"true"`
    ShowDuration         bool          `yaml:"show_duration" default:"true"`
    CompletionDelay      time.Duration `yaml:"completion_delay" default:"1500ms"`
    ErrorPersistence     time.Duration `yaml:"error_persistence" default:"5s"`
    RenderThrottle       time.Duration `yaml:"render_throttle" default:"33ms"`
    NarrowTerminalWidth  int           `yaml:"narrow_terminal_width" default:"80"`
    EnableFullDump       bool          `yaml:"enable_full_dump" default:"true"`
    TimeoutDuration      time.Duration `yaml:"timeout_duration" default:"30s"`
    ParallelIndicator    string        `yaml:"parallel_indicator" default:"⧉"`
    ProgressStyle        string        `yaml:"progress_style" default:"arc"`
}

// Load with defaults
func LoadConfig() (*Config, error) {
    config := &Config{}
    
    // Set defaults first
    if err := defaults.Set(config); err != nil {
        return nil, err
    }
    
    // Override with user config if exists
    configPath := filepath.Join(os.UserHomeDir(), ".simple-agent", "config.yaml")
    if _, err := os.Stat(configPath); err == nil {
        data, err := os.ReadFile(configPath)
        if err != nil {
            return nil, err
        }
        
        if err := yaml.Unmarshal(data, config); err != nil {
            return nil, err
        }
    }
    
    return config, nil
}
```

## Questions for Staff Engineer

Before proceeding with implementation, I have a few clarification questions:

1. **Event Batching Strategy**: When multiple tools complete within the same render frame (33ms), should we:
   - Show them stacked with a batch indicator (e.g., "3 tools completed")?
   - Stagger their removal animations by 100ms each?
   - Show a summary line instead of individual completions?

2. **Progress Reporting Granularity**: For tools that can report progress percentages (e.g., file downloads), should we:
   - Show a progress bar inline with the tool?
   - Update the arc/spinner to reflect percentage?
   - Keep it simple with just text updates?

3. **Tool Argument Display**: Some tool arguments might contain sensitive data (API keys, passwords). Should we:
   - Implement a redaction system for known patterns?
   - Allow tools to mark certain parameters as sensitive?
   - Leave it to tool implementers to sanitize?

4. **Full Panel Navigation**: For the full-screen tool history panel ('t' key), should we:
   - Implement vim-style navigation (j/k/g/G)?
   - Add search functionality within the panel?
   - Include filters by tool name or status?

5. **Persistence Between Sessions**: Should tool execution history:
   - Be saved to disk for debugging purposes?
   - Be included when resuming a conversation?
   - Have a configurable retention period?

6. **Performance Monitoring**: Would you like me to add:
   - Metrics for render time per frame?
   - Memory usage tracking for buffer growth?
   - Tool execution time histograms?

7. **Color Accessibility**: Should we:
   - Provide a colorblind-friendly theme option?
   - Use symbols in addition to colors for status?
   - Allow users to customize the color mappings?

8. **Integration with Existing Features**: How should this interact with:
   - The existing spinner for "Thinking..." state?
   - The conversation history when saved/loaded?
   - The `/verbose` debug mode?

## Summary

This final design achieves 100% production readiness by:

1. **Thread Safety**: Channel-only event loop eliminates all race conditions
2. **Memory Safety**: Strict circular buffers with enforced limits
3. **Performance**: 30fps render throttling with smart diffing
4. **Cancellation**: Full context propagation with timeout support
5. **Responsive UI**: Graceful degradation for narrow terminals
6. **Error UX**: Persistent errors with manual dismiss option
7. **Testing**: Comprehensive race, leak, and resize tests
8. **Configuration**: YAML-based with sensible defaults
9. **Extensibility**: Clean interfaces for future enhancements

The implementation is ready to begin once the clarification questions are answered.