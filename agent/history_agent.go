package agent

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/nachoal/simple-agent-go/history"
)

// HistoryAgent wraps an agent with conversation history support
type HistoryAgent struct {
	Agent
	historyManager *history.Manager
	currentSession *history.Session
}

// NewHistoryAgent creates a new agent with history support
func NewHistoryAgent(agent Agent, historyManager *history.Manager, session *history.Session) *HistoryAgent {
	return &HistoryAgent{
		Agent:          agent,
		historyManager: historyManager,
		currentSession: session,
	}
}

// Query sends a query and saves the conversation to history
func (ha *HistoryAgent) Query(ctx context.Context, query string) (*Response, error) {
	// Add user message to history
	if ha.currentSession != nil {
		ha.currentSession.Messages = append(ha.currentSession.Messages, history.Message{
			Role:      "user",
			Content:   &query,
			Timestamp: time.Now(),
		})
	}
	
	// Execute query
	response, err := ha.Agent.Query(ctx, query)
	
	// Add response to history
	if ha.currentSession != nil && err == nil {
		// Convert tool calls
		var toolCalls []history.ToolCall
		if len(response.ToolCalls) > 0 {
			toolCalls = make([]history.ToolCall, 0)
			// Note: ToolCalls in Response are ToolResult, not the actual calls
			// We'll store the results as part of the message content
		}
		
		ha.currentSession.Messages = append(ha.currentSession.Messages, history.Message{
			Role:      "assistant",
			Content:   &response.Content,
			ToolCalls: toolCalls,
			Timestamp: time.Now(),
		})
		
		// Save session
		if err := ha.historyManager.SaveSession(ha.currentSession); err != nil {
			// Log error but don't fail the query
			fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", err)
			fmt.Fprintf(os.Stderr, "Your conversation may not be saved. Please check disk space and permissions.\n\n")
		}
	}
	
	return response, err
}

// QueryStream sends a query and streams the response while saving to history
func (ha *HistoryAgent) QueryStream(ctx context.Context, query string) (<-chan StreamEvent, error) {
	// Add user message to history
	if ha.currentSession != nil {
		ha.currentSession.Messages = append(ha.currentSession.Messages, history.Message{
			Role:      "user",
			Content:   &query,
			Timestamp: time.Now(),
		})
	}
	
	// Get the stream
	events, err := ha.Agent.QueryStream(ctx, query)
	if err != nil {
		return nil, err
	}
	
	// Create a new channel to intercept events
	intercepted := make(chan StreamEvent, 100)
	
	go func() {
		defer close(intercepted)
		
		var fullContent string
		var toolCalls []history.ToolCall
		
		for event := range events {
			// Forward the event
			intercepted <- event
			
			// Collect content for history
			switch event.Type {
			case EventTypeMessage:
				fullContent += event.Content
			case EventTypeComplete:
				// Save to history when complete
				if ha.currentSession != nil && fullContent != "" {
					ha.currentSession.Messages = append(ha.currentSession.Messages, history.Message{
						Role:      "assistant",
						Content:   &fullContent,
						ToolCalls: toolCalls,
						Timestamp: time.Now(),
					})
					
					// Save session
					if err := ha.historyManager.SaveSession(ha.currentSession); err != nil {
						// Send error event through the stream
						intercepted <- StreamEvent{
							Type:  EventTypeError,
							Error: fmt.Errorf("failed to save conversation history: %w", err),
						}
						// Also log to stderr
						fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", err)
					}
				}
			}
		}
	}()
	
	return intercepted, nil
}

// GetSession returns the current session
func (ha *HistoryAgent) GetSession() *history.Session {
	return ha.currentSession
}

// SetSession updates the current session
func (ha *HistoryAgent) SetSession(session *history.Session) {
	ha.currentSession = session
}

// RestoreMemoryFromSession restores the agent's memory from a session
func (ha *HistoryAgent) RestoreMemoryFromSession(session *history.Session) {
	if session == nil || len(session.Messages) == 0 {
		return
	}
	
	// Convert and restore messages
	llmMessages := ha.historyManager.ConvertToLLMMessages(session.Messages)
	
	// Set the memory directly
	ha.Agent.SetMemory(llmMessages)
	
	// Update current session
	ha.currentSession = session
}