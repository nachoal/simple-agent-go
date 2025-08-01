# Tool Calling Visibility - Implementation Plan

## Overview

This document provides a straightforward implementation plan for adding tool calling visibility to Simple Agent Go. This is a personal tool optimized for macOS/Linux usage.

## Implementation Phases

### Phase 1: Core Infrastructure (1-2 days)

#### 1.1 Extend Agent Streaming Events

**Files to modify:**
- `agent/types.go`
- `agent/agent.go`

**Tasks:**
1. Add new event types to `StreamEvent`:
   ```go
   const (
       EventTypeToolStart    EventType = "tool_start"
       EventTypeToolProgress EventType = "tool_progress" 
       EventTypeToolResult   EventType = "tool_result"
       EventTypeToolTimeout  EventType = "tool_timeout"
       EventTypeToolCancel   EventType = "tool_cancel"
   )
   ```

2. Extend `ToolEvent` structure:
   ```go
   type ToolEvent struct {
       ID       string                 // Unique tool execution ID
       Name     string                 // Tool name
       Args     map[string]interface{} // Parsed arguments
       ArgsRaw  string                 // Raw JSON string
       Result   string                 // Execution result
       Error    error                  // Execution error
       Progress float64                // Progress percentage (0-1)
   }
   ```

3. Modify `agent.executeTools()` to emit events during tool execution

4. Implement unique ID generation:
   ```go
   var toolIDCounter uint64
   
   func generateToolID() string {
       id := atomic.AddUint64(&toolIDCounter, 1)
       return fmt.Sprintf("tool-%d-%d", time.Now().UnixNano(), id)
   }
   ```

---

#### 1.2 Tool Progress Interface

**Files to create/modify:**
- `tools/progress.go` (new)
- `tools/tool.go`

**Tasks:**
1. Create progress reporter interface:
   ```go
   type ProgressReporter interface {
       ReportProgress(message string)
       ReportProgressPercent(message string, percent float64)
   }
   ```

2. Create optional interface:
   ```go
   type ProgressableTool interface {
       Tool
       ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error)
   }
   ```

3. Update agent to check for progressable tools and use them

---

### Phase 2: TUI Integration (2-3 days)

#### 2.1 TUI State Management

**Files to modify:**
- `tui/bordered.go`

**Tasks:**
1. Add tool tracking state:
   ```go
   type BorderedTUI struct {
       // ... existing fields ...
       activeTools      []ActiveTool
       completedTools   []CompletedTool  // For batching
       toolErrors       []ToolError
       eventStream      <-chan agent.StreamEvent
       lastRender       time.Time
       renderPending    bool
       showFullPanel    bool
       toolViewport     viewport.Model
   }
   
   type ActiveTool struct {
       ID         string
       Name       string
       Args       map[string]interface{}
       StartTime  time.Time
       Status     ToolStatus
       Output     *CircularBuffer
       Progress   float64
       LastUpdate time.Time
   }
   ```

2. Create circular buffer for output management
3. Switch from `Query` to `QueryStream` in sendMessage

---

#### 2.2 Event Handling in Update

**Files to modify:**
- `tui/bordered.go`

**Tasks:**
1. Add event subscription:
   ```go
   func (m *BorderedTUI) subscribeToStream() tea.Cmd {
       return func() tea.Msg {
           event, ok := <-m.eventStream
           if !ok {
               return streamCompleteMsg{}
           }
           return toolEventMsg{event: event}
       }
   }
   ```

2. Handle tool events in Update method:
   - Start: Add to activeTools
   - Progress: Update output buffer
   - Complete: Move to completed, schedule removal
   - Error: Add to toolErrors with persistence

3. Implement render throttling (33ms)

---

#### 2.3 Rendering Implementation

**Files to modify:**
- `tui/bordered.go`

**Tasks:**
1. Create tool overlay rendering:
   ```go
   func (m *BorderedTUI) renderToolOverlay() string {
       // Compact mode for width < 80
       if m.width < 80 {
           return fmt.Sprintf("🔧 %d tools running...", len(m.activeTools))
       }
       
       // Full mode
       var b strings.Builder
       for _, tool := range m.activeTools {
           // Render tool status with icon, name, duration
           // Show first 5 lines of output
       }
       return b.String()
   }
   ```

2. Add to main View() after thinking spinner
3. Implement batch completion messages
4. Add argument redaction for sensitive data

---

### Phase 3: Advanced Features (1-2 days)

#### 3.1 Full Panel View

**Tasks:**
1. Add 't' key handler to toggle full panel
2. Create scrollable viewport with tool history
3. Implement vim-style navigation (j/k/g/G)
4. Add search with '/' key

---

#### 3.2 Configuration

**Files to create:**
- `config/tool_visibility.go`

**Tasks:**
1. Add configuration struct:
   ```go
   type ToolVisibilityConfig struct {
       Enabled              bool          
       MaxOutputLines       int           // Default: 5
       ShowArguments        bool          // Default: true
       ShowDuration         bool          // Default: true
       CompletionDelay      time.Duration // Default: 1.5s
       ErrorPersistence     time.Duration // Default: 5s
       RenderThrottle       time.Duration // Default: 33ms
   }
   ```

2. Load from YAML config file
3. Apply defaults if not specified

---

#### 3.3 Tool Updates

**Files to modify:**
- 2-3 example tools in `tools/`

**Tasks:**
1. Add progress support to a long-running tool (e.g., shell, download)
2. Mark tools with sensitive arguments
3. Test cancellation handling

---

### Phase 4: Polish & Testing (1 day)

#### 4.1 Visual Polish

**Tasks:**
1. Add color themes (including deuteranopia-friendly)
2. Ensure status icons always visible
3. Fine-tune animations and transitions
4. Handle edge cases (very long output, many parallel tools)

---

#### 4.2 Manual Testing Guide

**Test Scenarios:**

1. **Basic Tool Execution**
   - Run a query that uses 1 tool
   - Verify tool name appears below "Thinking..."
   - Verify output preview shows (max 5 lines)
   - Verify completion message appears briefly

2. **Parallel Tools**
   - Query: "Search Wikipedia for Go programming and also search Google for Go tutorials"
   - Verify both tools show simultaneously
   - Verify parallel indicator (⧉) appears
   - Verify batch completion if they finish together

3. **Long-Running Tools**
   - Execute a shell command with sleep
   - Verify duration counter updates
   - Test Ctrl+C cancellation
   - Verify timeout handling (30s default)

4. **Error Handling**
   - Trigger a tool error (bad arguments)
   - Verify error shows in red for 5s
   - Verify error details visible
   - Test 'e' key to dismiss

5. **Terminal Resize**
   - Start with wide terminal
   - Resize to < 80 chars
   - Verify switches to compact mode
   - Resize back, verify full mode returns

6. **Full Panel**
   - Press 't' to open full panel
   - Test j/k navigation
   - Test '/' search
   - Press 'q' to close

7. **Output Overflow**
   - Run tool that produces 20+ lines
   - Verify only 5 lines shown
   - Verify 'd' key dumps to temp file

8. **Sensitive Arguments**
   - Run tool with API key in args
   - Verify key is redacted (***REDACTED***)

**Performance Checks:**
- No UI lag during updates
- Memory usage stable over time
- Smooth scrolling in full panel

---

## Implementation Order

1. **Day 1**: Core infrastructure (1.1, 1.2)
2. **Day 2**: Basic TUI integration (2.1, 2.2)
3. **Day 3**: Rendering and polish (2.3, 4.1)
4. **Day 4**: Advanced features (3.1, 3.2)
5. **Day 5**: Tool updates and testing (3.3, 4.2)

## Key Decisions Made

1. **No feature flags** - Direct implementation
2. **No automated tests** - Manual testing only
3. **macOS/Linux only** - No Windows considerations
4. **Channel-only updates** - Prevents race conditions
5. **33ms render throttle** - Smooth 30fps updates
6. **5-line output preview** - Balances info vs clutter

## Configuration Example

```yaml
# ~/.simple-agent/config.yaml
tool_visibility:
  enabled: true
  max_output_lines: 5
  show_arguments: true
  show_duration: true
  completion_delay: 1500ms
  error_persistence: 5s
  render_throttle: 33ms
  
theme: classic  # or 'deuteranopia'
```

## Success Criteria

- [x] Tools show real-time status during execution
- [x] No UI blocking or lag
- [x] Parallel tools display correctly
- [x] Errors are visible but not intrusive
- [x] Works smoothly on macOS and Linux
- [x] Clean, minimal aesthetic maintained

This plan focuses on getting the feature working well for personal use, without the overhead of enterprise features.