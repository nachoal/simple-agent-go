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
	meta, err := m.loadMeta()
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

// ListSessionsForPath returns all sessions for a given path
func (m *Manager) ListSessionsForPath(path string) ([]SessionInfo, error) {
	meta, err := m.loadMeta()
	if err != nil {
		return nil, fmt.Errorf("failed to load meta: %w", err)
	}
	
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
		
		sessions = append(sessions, SessionInfo{
			ID:        session.ID,
			Title:     session.Metadata.Title,
			CreatedAt: session.CreatedAt,
			UpdatedAt: session.UpdatedAt,
			Messages:  len(session.Messages),
			Provider:  session.Provider,
			Model:     session.Model,
		})
	}
	
	// Sort by creation date, newest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	
	return sessions, nil
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
	rand.Read(b)
	for i := range b {
		b[i] = charset[b[i]%byte(len(charset))]
	}
	return string(b)
}