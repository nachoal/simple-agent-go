package agent

import (
	"context"
	"fmt"
	"os"

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
	// Remember the initial message count to rollback on failure
	initialMessageCount := 0
	if ha.currentSession != nil {
		initialMessageCount = len(ha.currentSession.Messages)
	}
	
	// Execute query first
	response, err := ha.Agent.Query(ctx, query)
	
	// If successful, update history with the complete conversation
	if err == nil && ha.currentSession != nil {
		// Get the complete memory from the agent (includes all tool interactions)
		agentMemory := ha.Agent.GetMemory()
		
		// Convert and store all new messages since our last save
		// We need to sync our session with the agent's memory
		ha.currentSession.Messages = ha.historyManager.ConvertFromLLMMessages(agentMemory)
		
		// Save session with complete history
		if saveErr := ha.historyManager.SaveSession(ha.currentSession); saveErr != nil {
			// Log error but don't fail the query
			fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", saveErr)
			fmt.Fprintf(os.Stderr, "Your conversation may not be saved. Please check disk space and permissions.\n\n")
		}
	} else if err != nil && ha.currentSession != nil {
		// Query failed - rollback to initial state
		ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
	}
	
	return response, err
}

// QueryStream sends a query and streams the response while saving to history
func (ha *HistoryAgent) QueryStream(ctx context.Context, query string) (<-chan StreamEvent, error) {
	// Remember the initial message count to rollback on failure
	initialMessageCount := 0
	if ha.currentSession != nil {
		initialMessageCount = len(ha.currentSession.Messages)
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
		
		streamSucceeded := false
		
		for event := range events {
			// Forward the event
			intercepted <- event
			
			// Check for completion or error
			switch event.Type {
			case EventTypeComplete:
				streamSucceeded = true
				// Get the complete memory from the agent (includes all tool interactions)
				if ha.currentSession != nil {
					agentMemory := ha.Agent.GetMemory()
					ha.currentSession.Messages = ha.historyManager.ConvertFromLLMMessages(agentMemory)
					
					// Save session with complete history
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
			case EventTypeError:
				// Stream failed - rollback the session
				if ha.currentSession != nil && !streamSucceeded {
					ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
				}
			}
		}
		
		// If stream ended without completion or error, rollback
		if !streamSucceeded && ha.currentSession != nil {
			ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
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