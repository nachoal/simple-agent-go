package agent

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/internal/runlog"
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

	runID := ha.beginRun(ctx, "query", query)

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
		if saveErr := ha.historyManager.FinishRun(ha.currentSession, runID, history.RunStatusCompleted, nil); saveErr != nil {
			// Log error but don't fail the query
			fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", saveErr)
			fmt.Fprintf(os.Stderr, "Your conversation may not be saved. Please check disk space and permissions.\n\n")
		}
	} else if err != nil && ha.currentSession != nil {
		// Query failed - rollback to initial state
		ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
		if saveErr := ha.historyManager.FinishRun(ha.currentSession, runID, statusFromRunError(ctx, err), err); saveErr != nil {
			fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", saveErr)
		}
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

	runID := ha.beginRun(ctx, "query", query)

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
		runFinalized := false

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
					if err := ha.historyManager.FinishRun(ha.currentSession, runID, history.RunStatusCompleted, nil); err != nil {
						// Send error event through the stream
						intercepted <- StreamEvent{
							Type:  EventTypeError,
							Error: fmt.Errorf("failed to save conversation history: %w", err),
						}
						// Also log to stderr
						fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", err)
					}
					runFinalized = true
				}
			case EventTypeError:
				// Stream failed - rollback the session
				if ha.currentSession != nil && !streamSucceeded {
					ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
					if err := ha.historyManager.FinishRun(ha.currentSession, runID, statusFromRunError(ctx, event.Error), event.Error); err != nil {
						fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", err)
					}
					runFinalized = true
				}
			}
		}

		// If stream ended without completion or error, rollback
		if !streamSucceeded && ha.currentSession != nil {
			ha.currentSession.Messages = ha.currentSession.Messages[:initialMessageCount]
			if !runFinalized {
				runErr := ctx.Err()
				status := statusFromRunError(ctx, runErr)
				if runErr == nil {
					status = history.RunStatusInterrupted
				}
				if err := ha.historyManager.FinishRun(ha.currentSession, runID, status, runErr); err != nil {
					fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to save conversation history: %v\n", err)
				}
			}
		}
	}()

	return intercepted, nil
}

func (ha *HistoryAgent) beginRun(ctx context.Context, fallbackMode, prompt string) string {
	if ha.currentSession == nil || ha.historyManager == nil {
		return ""
	}

	runID := ""
	mode := fallbackMode
	tracePath := ""
	if meta, ok := runlog.MetadataFromContext(ctx); ok {
		runID = meta.RunID
		if strings.TrimSpace(meta.Mode) != "" {
			mode = meta.Mode
		}
		tracePath = meta.TracePath
	}

	if err := ha.historyManager.BeginRun(ha.currentSession, runID, mode, prompt, tracePath); err != nil {
		fmt.Fprintf(os.Stderr, "\n[WARNING] Failed to start conversation run history: %v\n", err)
	}
	if meta, ok := runlog.MetadataFromContext(ctx); ok && strings.TrimSpace(meta.RunID) != "" {
		return meta.RunID
	}
	return ha.currentSession.Metadata.LastRunID
}

func statusFromRunError(ctx context.Context, err error) history.RunStatus {
	if ctx != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return history.RunStatusTimedOut
		}
		if errors.Is(ctx.Err(), context.Canceled) {
			return history.RunStatusCancelled
		}
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return history.RunStatusTimedOut
	}
	if errors.Is(err, context.Canceled) {
		return history.RunStatusCancelled
	}
	if err != nil {
		return history.RunStatusFailed
	}
	return history.RunStatusCompleted
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
