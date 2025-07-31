# Simple Agent Go - History and Resume Feature Implementation Plan

## Overview

This document outlines the implementation plan for adding conversation persistence, history management, and resume capabilities to the Simple Agent Go framework. The feature will allow users to continue their last conversation (`-c/--continue`) or resume specific sessions (`-r/--resume`) from any directory.

## Research Summary

### Claude Code Approach
- **Storage Location**: `~/.claude/projects/`
- **Format**: JSONL (JSON Lines) files with full message history
- **Organization**: By project with session metadata
- **Commands**: `--continue` for last session, `--resume` for picker
- **Features**: Auto-compact, context management, complete context preservation

### Cursor IDE Approach
- **Storage Location**: `%APPDATA%/Cursor/User/workspaceStorage/` (Windows)
- **Format**: SQLite database (`state.vscdb`) per workspace
- **Organization**: MD5 hash folders for each workspace
- **Limitations**: Local only, no cloud sync, workspace-specific

### Gemini CLI Approach
- **Commands**: `/chat save <tag>`, `/chat resume <tag>`, `/chat list`
- **Features**: Manual save/resume with tags, context compression
- **Limitations**: No automatic persistence, requires explicit saves

## Proposed Implementation Approaches

### Approach 1: Claude Code Style - JSONL with Auto-Save

**Storage Structure**:
```
~/.simple-agent/
├── config.json
└── conversations/
    ├── index.json
    └── sessions/
        ├── 20250731_143052_abc123.jsonl
        ├── 20250731_150234_def456.jsonl
        └── ...
```

**Key Features**:
- Automatic saving after each message exchange
- JSONL format for streaming writes and partial reads
- Index file mapping paths to sessions
- Session ID: `YYYYMMDD_HHMMSS_<random>`

**Pros**:
- Simple file format, easy to debug
- Streaming-friendly JSONL
- Human-readable timestamps
- Easy to implement backup/export

**Cons**:
- Multiple files to manage
- Potential for file corruption
- No built-in query capabilities

### Approach 2: SQLite Database - Structured and Queryable

**Database Schema**:
```sql
CREATE TABLE sessions (
    id TEXT PRIMARY KEY,
    created_at TIMESTAMP,
    updated_at TIMESTAMP,
    path TEXT,
    provider TEXT,
    model TEXT,
    metadata JSON
);

CREATE TABLE messages (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    session_id TEXT,
    role TEXT,
    content TEXT,
    tool_calls JSON,
    timestamp TIMESTAMP,
    FOREIGN KEY (session_id) REFERENCES sessions(id)
);

CREATE INDEX idx_sessions_path ON sessions(path);
CREATE INDEX idx_sessions_updated ON sessions(updated_at);
```

**Key Features**:
- Single database file at `~/.simple-agent/history.db`
- Efficient querying and filtering
- Atomic transactions
- Built-in search capabilities

**Pros**:
- ACID compliance, data integrity
- Powerful query capabilities
- Single file to manage
- Efficient storage

**Cons**:
- Binary format, harder to debug
- Requires SQLite dependency
- More complex implementation

### Approach 3: Hybrid - JSON Index with Separate Message Files

**Storage Structure**:
```
~/.simple-agent/
├── config.json
├── conversations/
│   └── index.json
└── messages/
    ├── 2025/
    │   └── 07/
    │       └── 31/
    │           ├── session_143052_abc123.json
    │           └── session_150234_def456.json
```

**Index File** (`conversations/index.json`):
```json
{
  "sessions": [
    {
      "id": "20250731_143052_abc123",
      "path": "/Users/user/projects/myapp",
      "created_at": "2025-07-31T14:30:52Z",
      "updated_at": "2025-07-31T14:45:23Z",
      "provider": "openai",
      "model": "gpt-4",
      "message_count": 12,
      "file_path": "messages/2025/07/31/session_143052_abc123.json"
    }
  ]
}
```

**Pros**:
- Organized by date for easy cleanup
- Fast index lookups
- Separate concerns (metadata vs content)
- Good balance of features

**Cons**:
- More complex directory structure
- Two-step read process
- Potential sync issues between index and files

## Recommended Approach: Enhanced Approach 1 with Smart Features

Based on the research and considering Go's strengths, I recommend an enhanced version of Approach 1 that combines the simplicity of Claude Code's approach with additional features:

### Storage Structure

```
~/.simple-agent/
├── config.json
├── sessions/
│   ├── meta.json           # Global metadata and indexes
│   ├── 20250731_143052_abc123.json
│   ├── 20250731_150234_def456.json
│   └── ...
```

### Session File Format (JSON, not JSONL)

```json
{
  "id": "20250731_143052_abc123",
  "version": "1.0",
  "created_at": "2025-07-31T14:30:52Z",
  "updated_at": "2025-07-31T14:45:23Z",
  "path": "/Users/user/projects/myapp",
  "provider": "openai",
  "model": "gpt-4",
  "metadata": {
    "title": "Auto-generated from first message",
    "tags": [],
    "token_count": 1234
  },
  "messages": [
    {
      "role": "system",
      "content": "You are an AI assistant...",
      "timestamp": "2025-07-31T14:30:52Z"
    },
    {
      "role": "user",
      "content": "Hello, can you help me?",
      "timestamp": "2025-07-31T14:30:55Z"
    },
    {
      "role": "assistant",
      "content": "Of course! I'd be happy to help...",
      "tool_calls": [],
      "timestamp": "2025-07-31T14:31:02Z"
    }
  ]
}
```

### Meta Index File

```json
{
  "version": "1.0",
  "last_session_id": "20250731_143052_abc123",
  "path_index": {
    "/Users/user/projects/myapp": [
      "20250731_143052_abc123",
      "20250731_120234_xyz789"
    ],
    "/Users/user/projects/another": [
      "20250730_090122_qwe456"
    ]
  }
}
```

## Implementation Steps (Pseudocode)

### 1. Create History Package

```go
// history/types.go
type Session struct {
    ID        string    `json:"id"`
    Version   string    `json:"version"`
    CreatedAt time.Time `json:"created_at"`
    UpdatedAt time.Time `json:"updated_at"`
    Path      string    `json:"path"`
    Provider  string    `json:"provider"`
    Model     string    `json:"model"`
    Metadata  Metadata  `json:"metadata"`
    Messages  []Message `json:"messages"`
}

type Metadata struct {
    Title      string   `json:"title"`
    Tags       []string `json:"tags"`
    TokenCount int      `json:"token_count"`
}

type Message struct {
    Role      string     `json:"role"`
    Content   string     `json:"content"`
    ToolCalls []ToolCall `json:"tool_calls,omitempty"`
    Timestamp time.Time  `json:"timestamp"`
}

type MetaIndex struct {
    Version      string              `json:"version"`
    LastSession  string              `json:"last_session_id"`
    PathIndex    map[string][]string `json:"path_index"`
}
```

### 2. Implement History Manager

```go
// history/manager.go
type Manager struct {
    sessionsDir string
    metaPath    string
    mu          sync.RWMutex
}

func NewManager() (*Manager, error) {
    homeDir, _ := os.UserHomeDir()
    sessionsDir := filepath.Join(homeDir, ".simple-agent", "sessions")
    
    m := &Manager{
        sessionsDir: sessionsDir,
        metaPath:    filepath.Join(sessionsDir, "meta.json"),
    }
    
    // Create directory
    os.MkdirAll(m.sessionsDir, 0755)
    
    // Initialize meta if not exists
    if _, err := os.Stat(m.metaPath); os.IsNotExist(err) {
        m.saveMeta(&MetaIndex{Version: "1.0", PathIndex: make(map[string][]string)})
    }
    
    return m, nil
}

func (m *Manager) StartSession(path, provider, model string) (*Session, error) {
    // Generate session ID
    id := fmt.Sprintf("%s_%s", 
        time.Now().Format("20060102_150405"),
        generateRandomID(6))
    
    session := &Session{
        ID:        id,
        Version:   "1.0",
        CreatedAt: time.Now(),
        UpdatedAt: time.Now(),
        Path:      path,
        Provider:  provider,
        Model:     model,
        Messages:  []Message{},
    }
    
    // Update meta index
    m.updatePathIndex(path, id)
    
    return session, nil
}

func (m *Manager) SaveSession(session *Session) error {
    session.UpdatedAt = time.Now()
    
    // Save to file
    data, _ := json.MarshalIndent(session, "", "  ")
    filename := filepath.Join(m.sessionsDir, session.ID+".json")
    
    return os.WriteFile(filename, data, 0644)
}

func (m *Manager) LoadSession(id string) (*Session, error) {
    filename := filepath.Join(m.sessionsDir, id+".json")
    data, err := os.ReadFile(filename)
    if err != nil {
        return nil, err
    }
    
    var session Session
    err = json.Unmarshal(data, &session)
    return &session, err
}

func (m *Manager) GetLastSessionForPath(path string) (*Session, error) {
    meta, _ := m.loadMeta()
    
    sessionIDs, ok := meta.PathIndex[path]
    if !ok || len(sessionIDs) == 0 {
        return nil, fmt.Errorf("no sessions found for path: %s", path)
    }
    
    // Get the most recent (last in list)
    lastID := sessionIDs[len(sessionIDs)-1]
    return m.LoadSession(lastID)
}

func (m *Manager) ListSessionsForPath(path string) ([]SessionInfo, error) {
    meta, _ := m.loadMeta()
    
    sessionIDs, ok := meta.PathIndex[path]
    if !ok {
        return []SessionInfo{}, nil
    }
    
    var sessions []SessionInfo
    for _, id := range sessionIDs {
        session, err := m.LoadSession(id)
        if err != nil {
            continue
        }
        
        // Create summary
        title := m.generateTitle(session)
        sessions = append(sessions, SessionInfo{
            ID:        session.ID,
            Title:     title,
            CreatedAt: session.CreatedAt,
            UpdatedAt: session.UpdatedAt,
            Messages:  len(session.Messages),
        })
    }
    
    return sessions, nil
}
```

### 3. Integrate with Agent

```go
// agent/agent.go additions
type AgentWithHistory struct {
    agent.Agent
    historyManager *history.Manager
    currentSession *history.Session
}

func (a *AgentWithHistory) Query(ctx context.Context, query string) (*Response, error) {
    // Add to history before query
    if a.currentSession != nil {
        a.currentSession.Messages = append(a.currentSession.Messages, history.Message{
            Role:      "user",
            Content:   query,
            Timestamp: time.Now(),
        })
    }
    
    // Execute query
    response, err := a.Agent.Query(ctx, query)
    
    // Add response to history
    if a.currentSession != nil && err == nil {
        a.currentSession.Messages = append(a.currentSession.Messages, history.Message{
            Role:      "assistant",
            Content:   response.Content,
            ToolCalls: response.ToolCalls,
            Timestamp: time.Now(),
        })
        
        // Save session
        a.historyManager.SaveSession(a.currentSession)
    }
    
    return response, err
}
```

### 4. Update CLI Commands

```go
// cmd/simple-agent/main.go modifications
func runTUI(cmd *cobra.Command, args []string) error {
    // ... existing setup ...
    
    // Initialize history manager
    historyMgr, err := history.NewManager()
    if err != nil {
        return fmt.Errorf("failed to initialize history: %w", err)
    }
    
    // Get current working directory
    cwd, _ := os.Getwd()
    
    var session *history.Session
    
    // Handle continue flag
    if continueConv {
        session, err = historyMgr.GetLastSessionForPath(cwd)
        if err != nil {
            fmt.Printf("No previous conversation found for this directory\n")
            // Start new session
            session, _ = historyMgr.StartSession(cwd, provider, model)
        } else {
            // Restore messages to agent
            for _, msg := range session.Messages {
                // Convert and add to agent memory
            }
        }
    } else if resume != "" {
        // Show session picker or load specific session
        if resume == "list" {
            sessions, _ := historyMgr.ListSessionsForPath(cwd)
            // Display session picker UI
            selectedSession := showSessionPicker(sessions)
            session = historyMgr.LoadSession(selectedSession.ID)
        } else {
            // Load specific session ID
            session, err = historyMgr.LoadSession(resume)
        }
    } else {
        // Start new session
        session, _ = historyMgr.StartSession(cwd, provider, model)
    }
    
    // Create agent with history
    agentWithHistory := &AgentWithHistory{
        Agent:          agentInstance,
        historyManager: historyMgr,
        currentSession: session,
    }
    
    // ... rest of TUI setup ...
}
```

### 5. Add Session Picker UI

```go
// tui/session_picker.go
type SessionPicker struct {
    sessions []history.SessionInfo
    selected int
    done     bool
}

func NewSessionPicker(sessions []history.SessionInfo) *SessionPicker {
    return &SessionPicker{
        sessions: sessions,
        selected: 0,
    }
}

func (p *SessionPicker) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    switch msg := msg.(type) {
    case tea.KeyMsg:
        switch msg.String() {
        case "up", "k":
            if p.selected > 0 {
                p.selected--
            }
        case "down", "j":
            if p.selected < len(p.sessions)-1 {
                p.selected++
            }
        case "enter":
            p.done = true
            return p, tea.Quit
        case "esc", "q":
            return p, tea.Quit
        }
    }
    return p, nil
}

func (p *SessionPicker) View() string {
    if len(p.sessions) == 0 {
        return "No previous conversations found for this directory."
    }
    
    var b strings.Builder
    b.WriteString("Select a conversation to resume:\n\n")
    
    for i, session := range p.sessions {
        cursor := "  "
        if i == p.selected {
            cursor = "> "
        }
        
        b.WriteString(fmt.Sprintf("%s%s - %s (%d messages)\n",
            cursor,
            session.CreatedAt.Format("Jan 02 15:04"),
            session.Title,
            session.Messages))
    }
    
    b.WriteString("\n[↑/↓] Navigate  [Enter] Select  [Esc] Cancel")
    return b.String()
}
```

## Additional Features to Consider

1. **Auto-cleanup**: Remove sessions older than X days
2. **Export functionality**: Export conversations to Markdown/PDF
3. **Search**: Full-text search across all conversations
4. **Compression**: Compress old sessions to save space
5. **Sync**: Optional cloud sync (future enhancement)
6. **Privacy**: Encryption for sensitive conversations

## Benefits of Recommended Approach

1. **Simplicity**: Single JSON file per session, easy to implement and debug
2. **Performance**: Fast meta index for path lookups
3. **Flexibility**: Easy to add features like search, export, tags
4. **Go-idiomatic**: Uses standard library, no external dependencies
5. **Human-readable**: JSON format is easy to inspect and migrate
6. **Atomic writes**: Each session is a separate file, reducing corruption risk

## Migration Path

If we need to migrate to a different approach later (e.g., SQLite for better performance with large histories), the JSON format makes it easy to write migration scripts.

## Security Considerations

1. Store files with 0600 permissions (user read/write only)
2. Consider optional encryption for sensitive conversations
3. Implement session expiry/cleanup policies
4. Sanitize file paths to prevent directory traversal

## Testing Strategy

1. Unit tests for history manager operations
2. Integration tests for CLI commands
3. Stress tests with large conversation histories
4. Edge cases: corrupted files, missing indexes, concurrent access

This implementation plan provides a solid foundation for conversation persistence while maintaining the simplicity and performance characteristics that make Simple Agent Go successful.