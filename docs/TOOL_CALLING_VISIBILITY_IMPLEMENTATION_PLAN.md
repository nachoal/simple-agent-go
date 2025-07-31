# Tool Calling Visibility - Step-by-Step Implementation Plan

## Overview

This document provides a detailed, sequential implementation plan for adding tool calling visibility to Simple Agent Go. Each phase builds upon the previous one, with clear dependencies and testing checkpoints.

## Implementation Phases

### Phase 1: Core Infrastructure (2-3 days)

#### 1.1 Extend Agent Streaming Events (Day 1)

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

3. Modify `agent.executeTools()` to emit events:
   - Before execution: emit `EventTypeToolStart`
   - During execution: emit `EventTypeToolProgress` (if supported)
   - After execution: emit `EventTypeToolResult`
   - On timeout: emit `EventTypeToolTimeout`
   - On cancellation: emit `EventTypeToolCancel`

4. Implement unique ID generation:
   ```go
   var toolIDCounter uint64
   
   func generateToolID() string {
       id := atomic.AddUint64(&toolIDCounter, 1)
       return fmt.Sprintf("tool-%d-%d", time.Now().UnixNano(), id)
   }
   ```

**Testing:**
- Unit test for ID generation uniqueness
- Unit test for event emission ordering
- Integration test with mock tool

**Rollback:** Feature flag `EnableToolVisibility` in agent config

---

#### 1.2 Tool Progress Interface (Day 1)

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

3. Create adapter function:
   ```go
   func ExecuteTool(ctx context.Context, tool Tool, params string, reporter ProgressReporter) (string, error) {
       if pt, ok := tool.(ProgressableTool); ok && reporter != nil {
           return pt.ExecuteWithProgress(ctx, params, reporter)
       }
       return tool.Execute(ctx, params)
   }
   ```

4. Update agent to use adapter

**Testing:**
- Test adapter with both tool types
- Test nil reporter handling
- Mock progressable tool

---

#### 1.3 Streaming Infrastructure Updates (Day 2)

**Files to modify:**
- `agent/agent.go`
- `agent/stream.go` (if exists, else create)

**Tasks:**
1. Ensure `QueryStream` properly channels events:
   ```go
   func (a *agent) QueryStream(ctx context.Context, query string) (<-chan StreamEvent, error) {
       events := make(chan StreamEvent, 100) // Buffered for performance
       
       go func() {
           defer close(events)
           // Existing logic, but ensure tool events are sent
       }()
       
       return events, nil
   }
   ```

2. Add event buffering for high-frequency progress
3. Implement event deduplication for rapid updates
4. Add context cancellation propagation

**Testing:**
- Stress test with 100+ rapid events
- Test channel closing on context cancel
- Memory leak test with `goleak`

---

### Phase 2: TUI Integration (3-4 days)

#### 2.1 TUI State Management (Day 3)

**Files to modify:**
- `tui/bordered.go`
- `tui/types.go` (create if needed)

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

2. Create circular buffer implementation:
   ```go
   type CircularBuffer struct {
       lines    []string
       maxLines int
       head     int
       size     int
   }
   ```

3. Implement state-only updates in `Update()`:
   - No direct mutations outside Update
   - All external events via tea.Msg

**Testing:**
- Test circular buffer bounds
- Test state consistency
- Race condition test with `-race`

---

#### 2.2 Event Message Types (Day 3)

**Files to modify:**
- `tui/messages.go` (create)

**Tasks:**
1. Define all message types:
   ```go
   type toolEventMsg struct {
       event agent.StreamEvent
   }
   
   type batchCompleteMsg struct {
       tools     []CompletedTool
       timestamp time.Time
   }
   
   type forceRenderMsg struct{}
   
   type removeCompletedToolsMsg struct {
       toolIDs []string
   }
   
   type toggleFullPanelMsg struct{}
   ```

2. Create event subscription command:
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

---

#### 2.3 Update Method Implementation (Day 4)

**Files to modify:**
- `tui/bordered.go`

**Tasks:**
1. Implement comprehensive Update handling:
   ```go
   func (m *BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
       var cmds []tea.Cmd
       
       switch msg := msg.(type) {
       case toolEventMsg:
           cmd := m.handleToolEvent(msg.event)
           cmds = append(cmds, cmd, m.subscribeToStream())
           
       case batchCompleteMsg:
           m.handleBatchComplete(msg)
           
       case forceRenderMsg:
           m.renderPending = false
           
       // ... other cases
       }
       
       return m, tea.Batch(cmds...)
   }
   ```

2. Implement render throttling:
   - 33ms minimum between renders
   - Deferred render scheduling
   - Smart dirty checking

3. Handle tool event state updates:
   - Add tools on start
   - Update progress/output
   - Mark complete/failed
   - Batch completions

**Testing:**
- Mock event sequences
- Test render throttling
- Verify state transitions

---

#### 2.4 Rendering Implementation (Day 4-5)

**Files to modify:**
- `tui/bordered.go`
- `tui/styles.go` (create)

**Tasks:**
1. Create style definitions:
   ```go
   var (
       styleToolRunning   = lipgloss.NewStyle().Foreground(lipgloss.Color("33"))
       styleToolSuccess   = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
       styleToolError     = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
       styleToolTimeout   = lipgloss.NewStyle().Foreground(lipgloss.Color("214"))
       styleToolCancelled = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
   )
   ```

2. Implement responsive rendering:
   ```go
   func (m *BorderedTUI) renderToolOverlay() string {
       if m.width < MinTerminalWidth {
           return m.renderCompactTools()
       }
       if m.width < CompactModeWidth {
           return m.renderSummaryTools()
       }
       return m.renderDetailedTools()
   }
   ```

3. Implement batch completion rendering
4. Add progress bar rendering (optional)
5. Handle argument redaction

**Testing:**
- Test all width breakpoints
- Visual regression tests
- Verify redaction works

---

### Phase 3: Advanced Features (2-3 days)

#### 3.1 Full Panel View (Day 5)

**Files to modify:**
- `tui/panel.go` (create)
- `tui/bordered.go`

**Tasks:**
1. Implement scrollable viewport:
   ```go
   func (m *BorderedTUI) initFullPanel() {
       m.toolViewport = viewport.New(m.width-2, m.height-4)
       m.toolViewport.KeyMap = viewport.KeyMap{
           // Vim-style navigation
       }
   }
   ```

2. Add search functionality:
   - Incremental search
   - Highlight matches
   - Next/prev navigation

3. Implement tool history view
4. Add keybinding ('t' to toggle)

**Testing:**
- Navigation keys work correctly
- Search finds all matches
- Panel resize handling

---

#### 3.2 Configuration & Persistence (Day 6)

**Files to modify:**
- `config/tool_visibility.go` (create)
- `config/config.go`

**Tasks:**
1. Define configuration structure:
   ```go
   type ToolVisibilityConfig struct {
       Version              string        `yaml:"version"`
       Enabled              bool          `yaml:"enabled"`
       MaxOutputLines       int           `yaml:"max_output_lines"`
       ShowArguments        bool          `yaml:"show_arguments"`
       ShowDuration         bool          `yaml:"show_duration"`
       CompletionDelay      time.Duration `yaml:"completion_delay"`
       ErrorPersistence     time.Duration `yaml:"error_persistence"`
       RenderThrottle       time.Duration `yaml:"render_throttle"`
       NarrowTerminalWidth  int           `yaml:"narrow_terminal_width"`
       EnableFullDump       bool          `yaml:"enable_full_dump"`
       TimeoutDuration      time.Duration `yaml:"timeout_duration"`
       ParallelIndicator    string        `yaml:"parallel_indicator"`
       ProgressStyle        string        `yaml:"progress_style"`
   }
   ```

2. Implement config loading with defaults
3. Add theme configuration
4. Implement history persistence (optional)

**Testing:**
- Config loads correctly
- Defaults apply properly
- Invalid config handling

---

#### 3.3 Accessibility & Themes (Day 6)

**Files to create/modify:**
- `tui/themes.go` (create)

**Tasks:**
1. Implement theme system:
   ```go
   type Theme struct {
       Name           string
       ToolRunning    string
       ToolSuccess    string
       ToolError      string
       ToolTimeout    string
       ToolCancelled  string
   }
   ```

2. Add deuteranopia preset
3. Ensure status icons always present
4. Implement theme switching

**Testing:**
- Theme switching works
- Colors apply correctly
- Icons always visible

---

### Phase 4: Integration & Polish (2 days)

#### 4.1 Tool Updates (Day 7)

**Files to modify:**
- Various tools in `tools/`

**Tasks:**
1. Update 2-3 tools to support progress:
   - `download_tool.go` (if exists)
   - `shell_tool.go` 
   - `wikipedia_tool.go`

2. Add progress reporting to long operations
3. Implement cancellation support
4. Add SensitiveArgs flags where needed

**Testing:**
- Progress reports correctly
- Cancellation works
- Sensitive args hidden

---

#### 4.2 Testing & Performance (Day 7-8)

**Tasks:**
1. Comprehensive test suite:
   - Unit tests for all components
   - Integration tests
   - Stress tests (100+ parallel tools)
   - Memory leak tests
   - Race condition tests

2. Performance optimization:
   - Profile render performance
   - Optimize event handling
   - Memory usage analysis

3. Add metrics collection (--metrics flag)

---

#### 4.3 Documentation & Examples (Day 8)

**Files to create/modify:**
- `README.md`
- `docs/TOOL_PROGRESS_GUIDE.md`

**Tasks:**
1. Update README with new feature
2. Create tool implementation guide
3. Add configuration examples
4. Document keyboard shortcuts
5. Create troubleshooting guide

---

## Rollout Strategy

### Feature Flags

```go
type FeatureFlags struct {
    ToolVisibility struct {
        Enabled         bool
        ProgressEnabled bool
        FullPanelEnabled bool
    }
}
```

### Gradual Rollout

1. **Alpha**: Internal testing with feature flag
2. **Beta**: Opt-in via config file
3. **GA**: Enabled by default

### Rollback Plan

1. Feature flag to disable instantly
2. Config override to force old behavior
3. Clean removal path if needed

---

## Risk Mitigation

### Technical Risks

1. **Performance Impact**
   - Mitigation: Render throttling, event batching
   - Monitoring: --metrics flag
   - Rollback: Feature flag

2. **Memory Leaks**
   - Mitigation: Circular buffers, goroutine cleanup
   - Testing: goleak in all tests
   - Monitoring: Memory profiling

3. **Race Conditions**
   - Mitigation: Channel-only state updates
   - Testing: -race flag in CI
   - Review: All mutations in Update()

### UX Risks

1. **Visual Clutter**
   - Mitigation: Responsive design, compact modes
   - Testing: Multiple terminal sizes
   - Config: Customizable display options

2. **Performance Perception**
   - Mitigation: Immediate feedback, progress indication
   - Testing: User studies
   - Tuning: Configurable delays

---

## Success Metrics

1. **Technical Metrics**
   - No increase in memory usage > 10%
   - Render performance < 33ms/frame
   - Zero race conditions
   - 95%+ test coverage

2. **User Metrics**
   - Reduced "stuck" perception
   - Increased tool usage visibility
   - Positive feedback on clarity

---

## Timeline Summary

**Total Duration**: 8-10 working days

- Phase 1 (Core): 2-3 days
- Phase 2 (TUI): 3-4 days  
- Phase 3 (Features): 2-3 days
- Phase 4 (Polish): 2 days

**Critical Path**: 
1. Event infrastructure (1.1-1.3)
2. TUI state management (2.1-2.3)
3. Rendering implementation (2.4)

**Parallelizable**:
- Tool progress interface (1.2)
- Configuration (3.2)
- Documentation (4.3)

---

## Dependencies

### External Libraries
- No new dependencies required
- Uses existing: Bubble Tea, Lipgloss, Glamour

### Internal Dependencies
- Agent must support streaming (existing)
- Tools must implement Tool interface (existing)
- Config system must support versioning (new)

---

## Testing Checklist

### Unit Tests
- [ ] ID generation uniqueness
- [ ] Event emission correctness
- [ ] Circular buffer bounds
- [ ] State transitions
- [ ] Render functions
- [ ] Config loading
- [ ] Theme switching

### Integration Tests
- [ ] Full flow: query → tools → display
- [ ] Multiple concurrent tools
- [ ] Cancellation flow
- [ ] Error handling
- [ ] Terminal resize

### Performance Tests
- [ ] 100+ parallel tools
- [ ] Memory usage over time
- [ ] Render performance
- [ ] Event throughput

### Manual Tests
- [ ] Various terminal sizes
- [ ] Different color schemes
- [ ] Keyboard navigation
- [ ] Edge cases (long output, errors)

---

## Sign-off Criteria

1. All tests passing with -race flag
2. No memory leaks (goleak clean)
3. Performance within targets
4. Documentation complete
5. Feature flags working
6. Rollback tested

This implementation plan provides a clear, sequential path to adding tool calling visibility with minimal risk and maximum flexibility.