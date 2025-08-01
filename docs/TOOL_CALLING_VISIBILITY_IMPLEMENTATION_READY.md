# Tool Calling Visibility - Implementation Ready 🚀

## Quick Reference: Staff Engineer Decisions

### 1. Event Batching
```go
// Batch completions within same frame
type batchCompleteMsg struct {
    tools []CompletedTool
    time  time.Time
}

func (m *BorderedTUI) renderBatchCompletion(tools []CompletedTool) string {
    if len(tools) == 1 {
        return fmt.Sprintf("✅ %s completed", tools[0].Name)
    }
    return fmt.Sprintf("✅ %d tools completed", len(tools))
}
```

### 2. Progress Reporting
```go
const ProgressUpdateInterval = 250 * time.Millisecond

func (m *BorderedTUI) renderProgress(tool ActiveTool) string {
    if m.config.ProgressStyle == "bar" && m.width >= 60 {
        return renderProgressBar(tool.Progress)
    }
    return tool.LastProgressText // Simple text updates
}

func renderProgressBar(pct float64) string {
    const width = 10
    filled := int(pct * width)
    return fmt.Sprintf("[%s%s] %d%%", 
        strings.Repeat("█", filled),
        strings.Repeat("░", width-filled),
        int(pct*100))
}
```

### 3. Argument Redaction
```go
var sensitivePattern = regexp.MustCompile(`(?i)(apikey|secret|token|password)=[^\s]+`)

func Redact(text string) string {
    return sensitivePattern.ReplaceAllStringFunc(text, func(match string) string {
        parts := strings.SplitN(match, "=", 2)
        return parts[0] + "=***REDACTED***"
    })
}

// Tool-level control
type ToolMetadata struct {
    SensitiveArgs bool `json:"sensitive_args"`
}

func (m *BorderedTUI) formatArguments(args string, sensitive bool) string {
    if sensitive {
        return "<arguments hidden>"
    }
    return Redact(args)
}
```

### 4. Full Panel Navigation
```go
// Vim-style keybindings
var fullPanelKeyMap = map[string]tea.Cmd{
    "j":     scrollDown,
    "k":     scrollUp,
    "g g":   scrollTop,
    "G":     scrollBottom,
    "/":     startSearch,
    "n":     nextMatch,
    "N":     prevMatch,
    "q":     closePanel,
}

type searchState struct {
    query       string
    matches     []int
    currentIdx  int
}
```

### 5. History Persistence
```go
const MaxHistoryEvents = 1000

type HistoryManager struct {
    enabled bool
    file    *os.File
    encoder *json.Encoder
    count   int
}

func (h *HistoryManager) LogEvent(event ToolEvent) error {
    if !h.enabled || h.count >= MaxHistoryEvents {
        return nil
    }
    
    h.count++
    return h.encoder.Encode(struct {
        ToolEvent
        Timestamp time.Time `json:"timestamp"`
    }{event, time.Now()})
}

// Enable with: /debug save-history
// View with: /history
```

### 6. Performance Metrics
```go
// Behind --metrics flag
type Metrics struct {
    renderTimes    *histogramVec
    toolExecTimes  *histogramVec
    memoryUsage    *gaugeVec
    lastSample     time.Time
}

func (m *Metrics) RecordRender(duration time.Duration) {
    if metricsEnabled {
        m.renderTimes.Observe(duration.Seconds())
    }
}

// Log to stderr every 5s
func (m *Metrics) StartSampling() {
    ticker := time.NewTicker(5 * time.Second)
    go func() {
        for range ticker.C {
            var ms runtime.MemStats
            runtime.ReadMemStats(&ms)
            fmt.Fprintf(os.Stderr, "[METRICS] Heap: %dMB, Goroutines: %d\n",
                ms.HeapAlloc/1024/1024, runtime.NumGoroutine())
        }
    }()
}
```

### 7. Accessibility Themes
```yaml
# config.yaml
theme_presets:
  classic:
    tool_running: "33"
    tool_success: "42"
    tool_error: "196"
    
  deuteranopia:
    tool_running: "45"   # yellow-ish
    tool_success: "82"   # light green
    tool_error: "196"    # red (still distinguishable)
    tool_timeout: "208"  # orange-ish
    tool_cancelled: "245"

# Always include status icons
status_icons:
  running: "⏳"
  success: "✅"
  error: "❌"
  timeout: "⏱"
  cancelled: "🚫"
```

### 8. Spinner Integration
```go
func (m *BorderedTUI) updateThinkingState(event StreamEvent) {
    switch event.Type {
    case EventTypeToolStart:
        // Replace spinner with tool overlay
        m.isThinking = false
        m.showingTools = true
        
    case EventTypeComplete:
        // All done - hide everything
        m.isThinking = false
        m.showingTools = false
    }
}

// Verbose mode enhancements
func applyVerboseSettings(config *ToolVisibilityConfig) {
    config.ShowArguments = true
    config.MaxOutputLines = 20
    config.RenderThrottle = 0 // No throttling in verbose
}
```

## Implementation Checklist

### Core Features
- [x] Channel-only event loop in Update()
- [x] Thread-safe state management
- [x] Circular buffer with strict limits
- [x] Render throttling at 30fps
- [x] Context-based cancellation
- [x] Responsive terminal handling

### Staff Engineer Requirements
- [ ] Batch completion messages for same-frame events
- [ ] Text progress updates (250ms interval)
- [ ] Optional progress bars for wide terminals
- [ ] Redact() helper with sensitive patterns
- [ ] SensitiveArgs flag per tool
- [ ] Vim navigation in full panel
- [ ] History persistence (1000 events, opt-in)
- [ ] Metrics behind --metrics flag
- [ ] Deuteranopia color theme
- [ ] Spinner replacement on first tool

### Additional Items
- [ ] Config schema versioning
- [ ] Tool progress implementation guide
- [ ] Cleanup all goroutines on exit
- [ ] goleak.VerifyNone in all tests

## Config Schema Versioning

```go
type ConfigFile struct {
    Version string `yaml:"version"`
    Config  Config `yaml:"config"`
}

const CurrentConfigVersion = "1.0"

func LoadConfig() (*Config, error) {
    // ... load file ...
    
    if cf.Version != CurrentConfigVersion {
        return nil, fmt.Errorf("config version %s not supported (expected %s)",
            cf.Version, CurrentConfigVersion)
    }
    
    return &cf.Config, nil
}
```

## Tool Progress Implementation Guide

```go
// Example: File download tool with progress
type DownloadTool struct {
    BaseTool
}

func (d *DownloadTool) ExecuteWithProgress(ctx context.Context, params string, reporter ProgressReporter) (string, error) {
    var args struct {
        URL  string `json:"url"`
        Path string `json:"path"`
    }
    json.Unmarshal([]byte(params), &args)
    
    // Start download
    resp, err := http.Get(args.URL)
    if err != nil {
        return "", err
    }
    defer resp.Body.Close()
    
    // Report initial status
    reporter.ReportProgress("Starting download...")
    
    // Create file
    out, err := os.Create(args.Path)
    if err != nil {
        return "", err
    }
    defer out.Close()
    
    // Download with progress
    total := resp.ContentLength
    var downloaded int64
    buf := make([]byte, 32*1024)
    
    lastReport := time.Now()
    
    for {
        n, err := resp.Body.Read(buf)
        if n > 0 {
            downloaded += int64(n)
            out.Write(buf[:n])
            
            // Report progress every 250ms
            if time.Since(lastReport) > 250*time.Millisecond {
                pct := float64(downloaded) / float64(total)
                reporter.ReportProgress(fmt.Sprintf("Downloaded %.1f%%", pct*100))
                lastReport = time.Now()
            }
        }
        
        if err == io.EOF {
            break
        }
        if err != nil {
            return "", err
        }
        
        // Check for cancellation
        select {
        case <-ctx.Done():
            return "", ctx.Err()
        default:
        }
    }
    
    return fmt.Sprintf("Downloaded %s to %s", args.URL, args.Path), nil
}
```

## Ready to Implement! 🚀

All design decisions are finalized. The implementation can begin with confidence that all edge cases, performance concerns, and UX considerations have been addressed.