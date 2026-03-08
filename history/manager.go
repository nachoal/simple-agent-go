package history

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
)

// Manager handles conversation history persistence
type Manager struct {
	sessionsDir string
	metaPath    string
	mu          sync.RWMutex
}

// NewManager creates a new history manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	sessionsDir := filepath.Join(homeDir, ".simple-agent", "sessions")

	m := &Manager{
		sessionsDir: sessionsDir,
		metaPath:    filepath.Join(sessionsDir, "meta.json"),
	}

	// Create directory
	if err := os.MkdirAll(m.sessionsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create sessions directory: %w", err)
	}

	// Initialize meta if not exists
	if _, err := os.Stat(m.metaPath); os.IsNotExist(err) {
		if err := m.saveMeta(&MetaIndex{
			Version:   "1.0",
			PathIndex: make(map[string][]string),
		}); err != nil {
			return nil, fmt.Errorf("failed to initialize meta index: %w", err)
		}
	}

	return m, nil
}

// StartSession creates a new session
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
		Metadata: Metadata{
			Tags: []string{},
		},
		Messages: []Message{},
	}

	// Update meta index
	if err := m.updatePathIndex(path, id); err != nil {
		return nil, fmt.Errorf("failed to update path index: %w", err)
	}

	// Persist immediately so empty sessions can still be resumed later.
	if err := m.SaveSession(session); err != nil {
		return nil, fmt.Errorf("failed to persist session: %w", err)
	}

	return session, nil
}

// SaveSession saves a session to disk
func (m *Manager) SaveSession(session *Session) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	session.UpdatedAt = time.Now()

	// Generate title if empty
	if session.Metadata.Title == "" {
		session.Metadata.Title = m.generateTitle(session)
	}

	// Save to file
	data, err := json.MarshalIndent(session, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal session: %w", err)
	}

	filename := filepath.Join(m.sessionsDir, session.ID+".json")
	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write session file: %w", err)
	}

	// Update last session in meta
	meta, err := m.loadMeta()
	if err != nil {
		return fmt.Errorf("failed to load meta: %w", err)
	}

	meta.LastSession = session.ID
	if err := m.saveMeta(meta); err != nil {
		return fmt.Errorf("failed to save meta: %w", err)
	}

	return nil
}

// BeginRun appends and persists a new run record for the session.
func (m *Manager) BeginRun(session *Session, runID, mode, prompt, tracePath string) error {
	if session == nil {
		return nil
	}
	if strings.TrimSpace(runID) == "" {
		runID = fmt.Sprintf("run_%s", generateRandomID(8))
	}

	session.Runs = append(session.Runs, Run{
		ID:        runID,
		Mode:      strings.TrimSpace(mode),
		Prompt:    strings.TrimSpace(prompt),
		TracePath: strings.TrimSpace(tracePath),
		StartedAt: time.Now(),
		Status:    RunStatusRunning,
	})
	session.Metadata.LastRunID = runID
	session.Metadata.LastRunStatus = RunStatusRunning
	session.Metadata.LastRunAt = time.Now()

	return m.SaveSession(session)
}

// FinishRun updates and persists the final status for a run record.
func (m *Manager) FinishRun(session *Session, runID string, status RunStatus, err error) error {
	if session == nil {
		return nil
	}
	if strings.TrimSpace(runID) == "" {
		return nil
	}

	for i := len(session.Runs) - 1; i >= 0; i-- {
		if session.Runs[i].ID != runID {
			continue
		}
		session.Runs[i].FinishedAt = time.Now()
		session.Runs[i].Status = status
		if err != nil {
			session.Runs[i].Error = err.Error()
		} else {
			session.Runs[i].Error = ""
		}
		break
	}

	session.Metadata.LastRunID = runID
	session.Metadata.LastRunStatus = status
	session.Metadata.LastRunAt = time.Now()

	return m.SaveSession(session)
}

// LoadSession loads a session from disk
func (m *Manager) LoadSession(id string) (*Session, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	filename := filepath.Join(m.sessionsDir, id+".json")
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read session file: %w", err)
	}

	var session Session
	if err := json.Unmarshal(data, &session); err != nil {
		return nil, fmt.Errorf("failed to unmarshal session: %w", err)
	}

	return &session, nil
}

// GetLastSessionForPath returns the most recent session for a given path
func (m *Manager) GetLastSessionForPath(path string) (*Session, error) {
	m.mu.RLock()
	meta, err := m.loadMeta()
	m.mu.RUnlock()

	if err != nil {
		return nil, fmt.Errorf("failed to load meta: %w", err)
	}

	sessionIDs, ok := meta.PathIndex[path]
	if !ok || len(sessionIDs) == 0 {
		return nil, fmt.Errorf("no sessions found for path: %s", path)
	}

	// Get the most recent (last in list)
	lastID := sessionIDs[len(sessionIDs)-1]
	return m.LoadSession(lastID)
}

// GetLastSession returns the most recently updated session across all paths.
func (m *Manager) GetLastSession() (*Session, error) {
	m.mu.RLock()
	meta, err := m.loadMeta()
	m.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to load meta: %w", err)
	}

	if strings.TrimSpace(meta.LastSession) != "" {
		session, err := m.LoadSession(meta.LastSession)
		if err == nil {
			return session, nil
		}
	}

	sessions, err := m.ListSessions(1)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}

	return m.LoadSession(sessions[0].ID)
}

// ListSessionsForPath returns all sessions for a given path
func (m *Manager) ListSessionsForPath(path string) ([]SessionInfo, error) {
	m.mu.RLock()
	meta, err := m.loadMeta()
	m.mu.RUnlock()

	if err != nil {
		return nil, fmt.Errorf("failed to load meta: %w", err)
	}

	sessionIDs, ok := meta.PathIndex[path]
	if !ok {
		return []SessionInfo{}, nil
	}

	return m.loadSessionInfos(sessionIDs, 0), nil
}

// ListSessions returns recent sessions across all paths, sorted by last update time.
// When limit <= 0, all sessions are returned.
func (m *Manager) ListSessions(limit int) ([]SessionInfo, error) {
	m.mu.RLock()
	meta, err := m.loadMeta()
	m.mu.RUnlock()
	if err != nil {
		return nil, fmt.Errorf("failed to load meta: %w", err)
	}

	seen := make(map[string]struct{})
	ids := make([]string, 0)
	for _, sessionIDs := range meta.PathIndex {
		for _, id := range sessionIDs {
			if _, ok := seen[id]; ok {
				continue
			}
			seen[id] = struct{}{}
			ids = append(ids, id)
		}
	}

	return m.loadSessionInfos(ids, limit), nil
}

// ConvertFromLLMMessages converts LLM messages to history messages
func (m *Manager) ConvertFromLLMMessages(llmMessages []llm.Message) []Message {
	messages := make([]Message, len(llmMessages))
	for i, msg := range llmMessages {
		messages[i] = Message{
			Role:       string(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
			Timestamp:  time.Now(), // We don't have original timestamps
		}

		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			messages[i].ToolCalls = make([]ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				messages[i].ToolCalls[j] = ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: FunctionCall{
						Name:      tc.Function.Name,
						Arguments: string(tc.Function.Arguments),
					},
				}
			}
		}
	}
	return messages
}

// ConvertToLLMMessages converts history messages to LLM messages
func (m *Manager) ConvertToLLMMessages(histMessages []Message) []llm.Message {
	messages := make([]llm.Message, len(histMessages))
	for i, msg := range histMessages {
		messages[i] = llm.Message{
			Role:       llm.Role(msg.Role),
			Content:    msg.Content,
			ToolCallID: msg.ToolCallID,
		}

		// Convert tool calls
		if len(msg.ToolCalls) > 0 {
			messages[i].ToolCalls = make([]llm.ToolCall, len(msg.ToolCalls))
			for j, tc := range msg.ToolCalls {
				messages[i].ToolCalls[j] = llm.ToolCall{
					ID:   tc.ID,
					Type: tc.Type,
					Function: llm.FunctionCall{
						Name:      tc.Function.Name,
						Arguments: json.RawMessage(tc.Function.Arguments),
					},
				}
			}
		}
	}
	return messages
}

// ConvertToResumeMessages restores only the conversational transcript that is
// useful across process restarts. Raw tool-call scaffolding and tool outputs
// are intentionally omitted because many OpenAI-compatible local models
// degrade when replaying historical tool protocol messages.
func (m *Manager) ConvertToResumeMessages(histMessages []Message) []llm.Message {
	messages := make([]llm.Message, 0, len(histMessages))
	for _, msg := range histMessages {
		switch msg.Role {
		case "system", "user":
			if msg.Content == nil {
				continue
			}
			messages = append(messages, llm.Message{
				Role:    llm.Role(msg.Role),
				Content: llm.StringPtr(*msg.Content),
			})
		case "assistant":
			if msg.Content == nil {
				continue
			}
			content := strings.TrimSpace(*msg.Content)
			if content == "" {
				continue
			}
			messages = append(messages, llm.Message{
				Role:    llm.RoleAssistant,
				Content: llm.StringPtr(*msg.Content),
			})
		}
	}
	return messages
}

// Private methods

func (m *Manager) loadMeta() (*MetaIndex, error) {
	data, err := os.ReadFile(m.metaPath)
	if err != nil {
		return nil, err
	}

	var meta MetaIndex
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}

	return &meta, nil
}

func (m *Manager) saveMeta(meta *MetaIndex) error {
	data, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(m.metaPath, data, 0644)
}

func (m *Manager) updatePathIndex(path, sessionID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	meta, err := m.loadMeta()
	if err != nil {
		return err
	}

	if meta.PathIndex == nil {
		meta.PathIndex = make(map[string][]string)
	}

	// Append session ID to path index
	meta.PathIndex[path] = append(meta.PathIndex[path], sessionID)

	return m.saveMeta(meta)
}

func (m *Manager) loadSessionInfos(sessionIDs []string, limit int) []SessionInfo {
	sessions := make([]SessionInfo, 0, len(sessionIDs))
	for _, id := range sessionIDs {
		session, err := m.LoadSession(id)
		if err != nil {
			continue
		}
		sessions = append(sessions, sessionInfoFromSession(session))
	}

	sort.Slice(sessions, func(i, j int) bool {
		if sessions[i].UpdatedAt.Equal(sessions[j].UpdatedAt) {
			return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
		}
		return sessions[i].UpdatedAt.After(sessions[j].UpdatedAt)
	})

	if limit > 0 && len(sessions) > limit {
		return sessions[:limit]
	}

	return sessions
}

func sessionInfoFromSession(session *Session) SessionInfo {
	return SessionInfo{
		ID:            session.ID,
		Title:         session.Metadata.Title,
		CreatedAt:     session.CreatedAt,
		UpdatedAt:     session.UpdatedAt,
		Path:          session.Path,
		Messages:      len(session.Messages),
		Provider:      session.Provider,
		Model:         session.Model,
		LastRunStatus: session.Metadata.LastRunStatus,
	}
}

func (m *Manager) generateTitle(session *Session) string {
	// Find first user message
	for _, msg := range session.Messages {
		if msg.Role == "user" && msg.Content != nil {
			// Take first 50 characters or until newline
			content := *msg.Content
			if idx := strings.IndexByte(content, '\n'); idx != -1 {
				content = content[:idx]
			}
			if len(content) > 50 {
				content = content[:47] + "..."
			}
			return content
		}
	}

	// Fallback to timestamp
	return fmt.Sprintf("Session %s", session.CreatedAt.Format("Jan 02 15:04"))
}

func generateRandomID(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	if _, err := rand.Read(b); err != nil {
		// Fall back to time-based seed if crypto/rand fails
		// This should be extremely rare
		panic(fmt.Sprintf("crypto/rand failed: %v", err))
	}
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}
