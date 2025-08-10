package tui

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"
	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/config"
	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

// BorderedTUI is a minimal TUI that matches the Python bordered_interface.py
type BorderedTUI struct {
	agent         agent.Agent
	llmClient     llm.Client
	provider      string
	model         string
	textarea      textarea.Model
	historyForAgent []llm.Message // Keep history only for agent context, not UI
	width         int
	height        int
	isThinking    bool
	typing        strings.Builder // For future streaming support
	err           error
	initialized   bool // Track if we've received the first WindowSizeMsg
	
	// Providers for model selection
	providers      map[string]llm.Client
	configManager  *config.Manager
	
	// Glamour renderer
	renderer *glamour.TermRenderer
	
	// Spinner for thinking state
	spinner spinner.Model
	
	// Tool execution tracking
	activeTools    map[string]*ActiveTool
	completedTools []CompletedTool
	toolErrors     []ToolError
	showingTools   bool
	lastRender     time.Time
	renderPending  bool
	toolEventChan  chan agent.StreamEvent
	toolsUsedInLastQuery map[string]time.Duration
	
	// Border style for input
    borderStyle   lipgloss.Style

    // In-app modal: model selector
    showModelSelector bool
    selector          *ModelSelector
}


// ActiveTool represents a currently executing tool
type ActiveTool struct {
	ID               string
	Name             string
	Args             map[string]interface{}
	StartTime        time.Time
	Status           ToolStatus
	Output           *CircularBuffer
	Progress         float64
	LastProgressText string
	LastUpdate       time.Time
}


// CompletedTool represents a completed tool execution
type CompletedTool struct {
	ID           string
	Name         string
	CompletedAt  time.Time
	Success      bool
	OutputSample string
}

// ToolError represents a tool error
type ToolError struct {
	ID      string
	Name    string
	Error   error
	Time    time.Time
	Details string
}

// CircularBuffer manages output with a size limit
type CircularBuffer struct {
	lines    []string
	maxLines int
	total    int
}

// NewCircularBuffer creates a new circular buffer
func NewCircularBuffer(maxLines int) *CircularBuffer {
	return &CircularBuffer{
		lines:    make([]string, 0, maxLines),
		maxLines: maxLines,
	}
}

// Add adds a line to the buffer
func (cb *CircularBuffer) Add(line string) {
	cb.total++
	if len(cb.lines) < cb.maxLines {
		cb.lines = append(cb.lines, line)
	} else {
		// Shift and add
		copy(cb.lines, cb.lines[1:])
		cb.lines[len(cb.lines)-1] = line
	}
}

// GetLines returns current lines
func (cb *CircularBuffer) GetLines() []string {
	return cb.lines
}

// HasOverflow returns true if more lines were added than capacity
func (cb *CircularBuffer) HasOverflow() bool {
	return cb.total > cb.maxLines
}

// NewBorderedTUI creates a new bordered TUI
func NewBorderedTUI(llmClient llm.Client, agentInstance agent.Agent, provider, model string) *BorderedTUI {
	ta := textarea.New()
	ta.Placeholder = ""
	ta.ShowLineNumbers = false
	ta.Prompt = "" // Remove the prompt since we'll handle it in the border
	ta.CharLimit = 0
	ta.SetHeight(1) // Start with single line
	ta.Focus()
	
	// Completely transparent styles - unset all backgrounds and borders
	transparentStyle := lipgloss.NewStyle().
		UnsetBackground().
		UnsetBorderBackground().
		UnsetBorderStyle()
	
	// Apply transparent styles to all textarea style states
	ta.FocusedStyle.Base = transparentStyle
	ta.FocusedStyle.Text = transparentStyle
	ta.FocusedStyle.Placeholder = transparentStyle
	ta.FocusedStyle.Prompt = transparentStyle
	ta.FocusedStyle.CursorLine = transparentStyle
	
	ta.BlurredStyle.Base = transparentStyle
	ta.BlurredStyle.Text = transparentStyle
	ta.BlurredStyle.Placeholder = transparentStyle
	ta.BlurredStyle.Prompt = transparentStyle
	ta.BlurredStyle.CursorLine = transparentStyle
	
	// Disable newlines - Enter will send the message
	ta.KeyMap.InsertNewline.SetEnabled(false)
	
	// Set initial width (will be updated by WindowSizeMsg)
	ta.SetWidth(74) // Default width minus borders/padding
	
	// Simple glamour renderer
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(74),
	)
	
	// Initialize spinner
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("75")) // Same color as model
	
	// Border style for input
	borderStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("15"))
	
	tui := &BorderedTUI{
		agent:       agentInstance,
		llmClient:   llmClient,
		provider:    provider,
		model:       model,
		textarea:    ta,
		historyForAgent: []llm.Message{},
		width:       80, // Default terminal width
		initialized: false,
		renderer:    renderer,
		spinner:     s,
		activeTools:    make(map[string]*ActiveTool),
		completedTools: []CompletedTool{},
		toolErrors:     []ToolError{},
		lastRender:     time.Now(),
		toolsUsedInLastQuery: make(map[string]time.Duration),
		borderStyle: borderStyle,
	}
	
	return tui
}

// NewBorderedTUIWithProviders creates a new bordered TUI with provider and config support
func NewBorderedTUIWithProviders(llmClient llm.Client, agentInstance agent.Agent, provider, model string, providers map[string]llm.Client, configManager *config.Manager) *BorderedTUI {
	tui := NewBorderedTUI(llmClient, agentInstance, provider, model)
	tui.providers = providers
	tui.configManager = configManager
	return tui
}

// NewBorderedTUIWithHistory creates a new bordered TUI with history support
func NewBorderedTUIWithHistory(llmClient llm.Client, historyAgent *agent.HistoryAgent, provider, model string, providers map[string]llm.Client, configManager *config.Manager) *BorderedTUI {
	tui := NewBorderedTUI(llmClient, historyAgent, provider, model)
	tui.providers = providers
	tui.configManager = configManager
	
	// Print history immediately before TUI starts
	if historyAgent != nil {
		session := historyAgent.GetSession()
		if session != nil && len(session.Messages) > 0 {
			// Debug output
			if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
				fmt.Fprintf(os.Stderr, "[TUI] Found %d messages in session %s\n", len(session.Messages), session.ID)
			}
			
			// Print history messages to stdout
			for _, msg := range session.Messages {
				// Skip system messages
				if msg.Role == "system" {
					continue
				}
				
				content := ""
				if msg.Content != nil {
					content = *msg.Content
				}
				
				// Also populate historyForAgent for context
				tui.historyForAgent = append(tui.historyForAgent, llm.Message{
					Role:    llm.Role(msg.Role),
					Content: &content,
				})
				
				// Print to stdout
				switch msg.Role {
				case "user":
					fmt.Println(renderUserMessage(content))
				case "assistant":
					fmt.Println(renderAssistantMessage(tui.renderer, content))
				}
				fmt.Println() // Empty line between messages
			}
		}
	}
	
	return tui
}

// Helper functions for rendering messages to stdout with styling
func renderUserMessage(content string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("15"))
	return fmt.Sprintf("👤 You: %s", style.Render(content))
}

func renderAssistantMessage(renderer *glamour.TermRenderer, content string) string {
	if renderer != nil {
		rendered, err := renderer.Render(content)
		if err == nil {
			return fmt.Sprintf("🤖 Assistant:\n%s", strings.TrimRight(rendered, "\n"))
		}
	}
	// Fallback without glamour
	return fmt.Sprintf("🤖 Assistant: %s", content)
}

func renderCommandMessage(content string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return style.Render(content)
}

func renderErrorMessage(content string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	return style.Render(fmt.Sprintf("❌ %s", content))
}

func renderToolMessage(content string) string {
	style := lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Italic(true)
	return style.Render(content)
}

// PrintHeader prints the TUI header to stdout before the TUI starts
func PrintHeader(provider, model string) {
	// Colors matching Python version
	headerStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("15")).Bold(true)
	modelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // #5B9BD5
	toolsStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("80"))  // #4ECDC4
	cmdStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))   // #6B7280
	
	verboseIndicator := ""
	if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
		verboseStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("196")).Bold(true) // Red
		verboseIndicator = " | " + verboseStyle.Render("[VERBOSE]")
	}
	
	header1 := fmt.Sprintf("%s | Model: %s | Provider: %s%s",
		headerStyle.Render("Simple Agent Go"),
		modelStyle.Render(model),
		modelStyle.Render(provider),
		verboseIndicator)
	
	toolCount := len(registry.List())
	header2 := fmt.Sprintf("%s | %s",
		toolsStyle.Render(fmt.Sprintf("Loaded %d tools", toolCount)),
		cmdStyle.Render("Commands: /help, /tools, /model, /status, /system, /verbose, /clear, /exit"))
	
	fmt.Println(header1)
	fmt.Println(header2)
	fmt.Println() // Empty line after header
}

// replayHistory prints historical messages to stdout for --continue support
func replayHistory(session *history.Session, renderer *glamour.TermRenderer) tea.Cmd {
	return func() tea.Msg {
		if session == nil || len(session.Messages) == 0 {
			return nil
		}
		
		for _, msg := range session.Messages {
			// Skip system messages
			if msg.Role == "system" {
				continue
			}
			
			content := ""
			if msg.Content != nil {
				content = *msg.Content
			}
			
			switch msg.Role {
			case "user":
				tea.Println(renderUserMessage(content))
			case "assistant":
				tea.Println(renderAssistantMessage(renderer, content))
			}
			tea.Println() // Empty line between messages
		}
		
		return nil
	}
}

func (m BorderedTUI) Init() tea.Cmd {
	// Initialize with a default width if not set
	if m.width == 0 {
		m.width = 80 // Default terminal width
	}
	
	// Just start the textarea blink
	return textarea.Blink
}



func (m BorderedTUI) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
    var cmds []tea.Cmd
    var cmd tea.Cmd

    // Update spinner if we're thinking
    if m.isThinking {
        m.spinner, cmd = m.spinner.Update(msg)
        cmds = append(cmds, cmd)
    }

    // If model selector modal is active, route all messages to it,
    // except confirmation/cancellation messages which the parent handles.
    if m.showModelSelector && m.selector != nil {
        switch msg.(type) {
        case selectorConfirmMsg, selectorCancelMsg:
            // Let parent handle below
        default:
            var scmd tea.Cmd
            var child tea.Model
            child, scmd = m.selector.Update(msg)
            if sel, ok := child.(*ModelSelector); ok {
                m.selector = sel
            }
            return m, scmd
        }
    }


	switch msg := msg.(type) {
	case modelSelectedMsg:
		// Model was selected, update our state
		m.provider = msg.provider
		m.model = msg.model
		
		// Save to config if we have a config manager
		if m.configManager != nil {
			if err := m.configManager.SetDefaults(msg.provider, msg.model); err != nil {
				// We'll print the error with the success message
				m.err = fmt.Errorf("failed to save config: %w", err)
			}
		}
		
		// Update the agent with the new model
		if newClient, ok := m.providers[msg.provider]; ok {
			m.llmClient = newClient
			m.agent = agent.New(newClient,
				agent.WithMaxIterations(10),
				agent.WithTemperature(0.7),
			)
		}
		
		// Print model switch message
		m.textarea.Focus()
		return m, tea.Printf("%s\n\n", renderCommandMessage(fmt.Sprintf("Switched to %s - %s", msg.provider, msg.model)))
		
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		
		// Update textarea width to match terminal minus borders and padding
		// The -6 accounts for: border (2) + padding (2) + some margin (2)
		textareaWidth := m.width - 6
		if textareaWidth < 20 {
			textareaWidth = 20 // Minimum width
		}
		m.textarea.SetWidth(textareaWidth)
		
		// Update border style width
		m.borderStyle = m.borderStyle.Width(m.width - 2)
		
		// Adjust height based on content
		m.adjustTextareaHeight()
		
		// Mark as initialized but don't clear screen for native scrollback
		m.initialized = true
		return m, nil
		
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyCtrlC, tea.KeyCtrlQ, tea.KeyEsc:
			return m, tea.Quit
			
		case tea.KeyCtrlL:
			// Clear history for agent context
			m.historyForAgent = []llm.Message{}
			// Clear screen command will clear the terminal
			return m, tea.ClearScreen
			
		case tea.KeyEnter:
			// Send the message on Enter
			if !m.isThinking {
				value := m.textarea.Value()
				if strings.TrimSpace(value) != "" {
					// Print user message to stdout
					cmds = append(cmds, tea.Printf("%s\n\n", renderUserMessage(value)))
					
					// Add to history for agent context
					m.historyForAgent = append(m.historyForAgent, llm.Message{
						Role:    llm.RoleUser,
						Content: &value,
					})
					
					// Clear input and reset height
					m.textarea.Reset()
					m.textarea.SetHeight(1)
					m.textarea.Blur()
					
					// Send to agent
					m.isThinking = true
					m.showingTools = false
					
					// Create event channel and store it
					m.toolEventChan = make(chan agent.StreamEvent, 100)
					
					cmds = append(cmds, m.sendMessage(value))
					cmds = append(cmds, m.spinner.Tick)
					cmds = append(cmds, m.listenForToolEvents())
				}
			}
			return m, tea.Batch(cmds...)
		}
		
	case toolEventMsg:
		// Handle tool events
		if os.Getenv("SIMPLE_AGENT_DEBUG") == "true" {
			fmt.Fprintf(os.Stderr, "[TUI] Received tool event: %s\n", msg.event.Type)
			if msg.event.Tool != nil {
				fmt.Fprintf(os.Stderr, "[TUI] Tool: %s (ID: %s)\n", msg.event.Tool.Name, msg.event.Tool.ID)
			}
		}
		
		switch msg.event.Type {
		case agent.EventTypeToolStart:
			if msg.event.Tool != nil {
				// Keep showing "Thinking..." when tools start - show continuous indicator
				m.isThinking = true
				m.showingTools = true
				
				// Add to active tools
				m.activeTools[msg.event.Tool.ID] = &ActiveTool{
					ID:        msg.event.Tool.ID,
					Name:      msg.event.Tool.Name,
					Args:      msg.event.Tool.Args,
					StartTime: time.Now(),
					Status:    ToolStatusRunning,
					Output:    NewCircularBuffer(20),
				}
				
				// Track tool usage
				m.toolsUsedInLastQuery[msg.event.Tool.Name] = 0
				
				// Print tool start message immediately
				argStr := m.formatArguments(msg.event.Tool.Args)
				toolStartMsg := fmt.Sprintf("🔧 Calling tool: %s %s", msg.event.Tool.Name, argStr)
				cmds = append(cmds, tea.Printf("%s\n", renderToolMessage(toolStartMsg)))
			}
			
		case agent.EventTypeToolProgress:
			if msg.event.Tool != nil && m.activeTools[msg.event.Tool.ID] != nil {
				tool := m.activeTools[msg.event.Tool.ID]
				tool.Progress = msg.event.Tool.Progress
				tool.LastProgressText = msg.event.Tool.Message
				tool.LastUpdate = time.Now()
			}
			
		case agent.EventTypeToolResult:
			if msg.event.Tool != nil {
				// Move from active to completed
				if activeTool := m.activeTools[msg.event.Tool.ID]; activeTool != nil {
					delete(m.activeTools, msg.event.Tool.ID)
					
					// Add to completed
					completedTool := CompletedTool{
						ID:           msg.event.Tool.ID,
						Name:         activeTool.Name,
						CompletedAt:  time.Now(),
						Success:      msg.event.Tool.Error == nil,
						OutputSample: msg.event.Tool.Result,
					}
					m.completedTools = append(m.completedTools, completedTool)
					
					// Update duration in tracking
					duration := time.Since(activeTool.StartTime)
					m.toolsUsedInLastQuery[activeTool.Name] = duration
					
					// Print tool completion message immediately
					if msg.event.Tool.Error != nil {
						// Track error
						m.toolErrors = append(m.toolErrors, ToolError{
							ID:    msg.event.Tool.ID,
							Name:  activeTool.Name,
							Error: msg.event.Tool.Error,
							Time:  time.Now(),
						})
						
						errorMsg := fmt.Sprintf("❌ Tool %s failed: %v", activeTool.Name, msg.event.Tool.Error)
						cmds = append(cmds, tea.Printf("%s\n", renderToolMessage(errorMsg)))
					} else {
						// Print success message with duration
						successMsg := fmt.Sprintf("✅ Tool %s completed in %v", activeTool.Name, duration.Round(time.Millisecond))
						cmds = append(cmds, tea.Printf("%s\n", renderToolMessage(successMsg)))
					}
				}
			}
		}
		
		// Continue listening for more events with any accumulated commands
		cmds = append(cmds, m.listenForToolEvents())
		return m, tea.Batch(cmds...)
		
	case borderedResponseMsg:
		m.isThinking = false
		m.showingTools = false
		
		// Reset for next query
		m.toolsUsedInLastQuery = make(map[string]time.Duration)
		m.activeTools = make(map[string]*ActiveTool)
		m.completedTools = []CompletedTool{}
		
		// Handle special command cases
		if msg.isQuit {
			return m, tea.Quit
		}
		
		if msg.isClear {
			// Clear history for agent context
			m.historyForAgent = []llm.Message{}
			m.textarea.Focus()
			return m, tea.ClearScreen
		}
		
        if msg.isModelSelect {
            // Show in-app model selector modal
            m.selector = NewModelSelector(m.providers, nil)
            // Initialize selector size to match current TUI
            if m.selector != nil {
                m.selector.width = m.width
                m.selector.height = m.height
                m.selector.list.SetSize(m.width, m.height)
            }
            m.showModelSelector = true
            m.textarea.Blur()
            // Enter alt screen and trigger selector Init to load models
            return m, tea.Batch(tea.EnterAltScreen, m.selector.Init())
        }
        // Handle normal messages
        if msg.err != nil {
            // Print error message
            return m, tea.Printf("%s\n\n", renderErrorMessage(fmt.Sprintf("Error: %v", msg.err)))
        } else if msg.content != "" {
            if msg.isCommand {
                // Print command output
                m.textarea.Focus()
                return m, tea.Printf("%s\n\n", renderCommandMessage(msg.content))
            } else {
                // Print assistant message
                content := msg.content
                m.historyForAgent = append(m.historyForAgent, llm.Message{
                    Role:    llm.RoleAssistant,
                    Content: &content,
                })
                m.textarea.Focus()
                return m, tea.Printf("%s\n\n", renderAssistantMessage(m.renderer, msg.content))
            }
        }
        m.textarea.Focus()
        return m, nil

    case selectorCancelMsg:
        // Close selector, refocus input
        m.showModelSelector = false
        m.selector = nil
        m.textarea.Focus()
        return m, tea.ExitAltScreen

    case selectorConfirmMsg:
        // Apply selected provider/model
        m.provider = msg.provider
        m.model = msg.model
		// Save to config if available
		if m.configManager != nil {
			if err := m.configManager.SetDefaults(msg.provider, msg.model); err != nil {
				m.err = fmt.Errorf("failed to save config: %w", err)
			}
		}
		// Update agent client if available
		if newClient, ok := m.providers[msg.provider]; ok {
			m.llmClient = newClient
			m.agent = agent.New(newClient,
				agent.WithMaxIterations(10),
				agent.WithTemperature(0.7),
			)
		}
        // Close selector and refocus
        m.showModelSelector = false
        m.selector = nil
        m.textarea.Focus()
        return m, tea.Batch(tea.ExitAltScreen, tea.Printf("%s\n\n", renderCommandMessage(fmt.Sprintf("Switched to %s - %s", msg.provider, msg.model))))

    }

    // Update textarea
    oldValue := m.textarea.Value()
    m.textarea, cmd = m.textarea.Update(msg)
    cmds = append(cmds, cmd)
	
	// Check if content changed to adjust height
	if oldValue != m.textarea.Value() {
		m.adjustTextareaHeight()
	}

    return m, tea.Batch(cmds...)
}

func (m BorderedTUI) View() string {
    // Render modal selector if active
    if m.showModelSelector && m.selector != nil {
        return m.selector.View()
    }
    var b strings.Builder
	
	// Only show the live region: streaming content (future) + spinner + input box
	
	// Show any streaming content (for future implementation)
	if m.typing.Len() > 0 {
		b.WriteString(m.typing.String())
		b.WriteString("\n\n")
	}
	
	// Show thinking indicator with spinner
	if m.isThinking {
		b.WriteString(fmt.Sprintf("%s Thinking...\n\n", m.spinner.View()))
	} else {
		// When not thinking, add extra spacing based on textarea height
		// This prevents border overlap when printing messages
		extraLines := m.textarea.Height()
		if extraLines > 1 {
			// Add extra newlines for multi-line input to push border down
			for i := 0; i < extraLines; i++ {
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}
	
	// Create model info string that will appear above the input box
	// Use gray for labels and blue for the actual model/provider names
	grayStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	blueStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("75"))  // Same blue as header
	
	modelInfo := fmt.Sprintf("%s %s %s %s",
		grayStyle.Render("Model:"),
		blueStyle.Render(m.model),
		grayStyle.Render("| Provider:"),
		blueStyle.Render(m.provider))
	
	// Calculate the right alignment padding
	// The border box width is m.width - 2, so we align to that
	boxWidth := m.width - 2
	if boxWidth < 40 {
		boxWidth = 40 // Minimum width
	}
	
	// Calculate the actual width of the rendered text (without ANSI codes)
	plainTextWidth := len("Model: ") + len(m.model) + len(" | Provider: ") + len(m.provider)
	
	// Right-align the model info to match the box width
	paddingNeeded := boxWidth - plainTextWidth
	if paddingNeeded > 0 {
		modelInfo = strings.Repeat(" ", paddingNeeded) + modelInfo
	}
	
	// Add the model info line above the input box
	b.WriteString(modelInfo)
	b.WriteString("\n")
	
	// Input area with border and prompt
	inputContent := m.textarea.View()
	// Add the prompt prefix
	promptedInput := "> " + inputContent
	
	// Style the input box with border
	styledInput := m.borderStyle.
		PaddingLeft(1).
		PaddingRight(1).
		Render(promptedInput)
	b.WriteString(styledInput)
	b.WriteString("\n") // Ensure cursor moves to next line after box
	
	return b.String()
}

func (m *BorderedTUI) sendMessage(input string) tea.Cmd {
	return func() tea.Msg {
		// Handle commands
		if strings.HasPrefix(input, "/") {
			return m.handleCommand(input)
		}
		
		// Query the agent with event channel in context
		ctx := context.Background()
		if m.toolEventChan != nil {
			ctx = context.WithValue(ctx, "toolEventChan", m.toolEventChan)
		}
		
		response, err := m.agent.Query(ctx, input)
		
		// Close the event channel after query completes
		if m.toolEventChan != nil {
			close(m.toolEventChan)
		}
		
		if err != nil {
			return borderedResponseMsg{err: err}
		}
		
		return borderedResponseMsg{content: response.Content}
	}
}

func (m *BorderedTUI) handleCommand(cmd string) borderedResponseMsg {
	switch strings.ToLower(strings.TrimSpace(cmd)) {
	case "/exit", "/quit":
		// Return a special message type that will trigger quit
		return borderedResponseMsg{content: "", isQuit: true}
	case "/clear":
		// Return a special message type that will trigger clear
		return borderedResponseMsg{content: "", isClear: true}
	case "/help":
		help := `Commands:
  /help    - Show this help
  /tools   - List available tools
  /model   - Change model interactively
  /status  - Show current model and provider
  /system  - Show system prompt
  /verbose - Toggle verbose/debug mode
  /clear   - Clear chat history
  /exit    - Exit application

Keyboard shortcuts:
  Ctrl+C - Quit
  Ctrl+L - Clear chat
  Enter  - Send message`
		return borderedResponseMsg{content: help, isCommand: true}
	case "/tools":
		var toolsBuilder strings.Builder
		toolsBuilder.WriteString("Available tools:\n")
		
		// Get all tools from registry
		toolNames := registry.List()
		for _, name := range toolNames {
			tool, err := registry.Get(name)
			if err != nil {
				continue
			}
			// Format: tool_name - description
			toolsBuilder.WriteString(fmt.Sprintf("  %-15s - %s\n", name, tool.Description()))
		}
		
		return borderedResponseMsg{content: strings.TrimRight(toolsBuilder.String(), "\n"), isCommand: true}
	case "/model":
		// Check if providers are available
		if m.providers == nil || len(m.providers) == 0 {
			return borderedResponseMsg{content: "Model selection not available (no providers configured)", isCommand: true}
		}
		
		// Return a special message that will trigger model selection
		return borderedResponseMsg{content: "", isModelSelect: true}
	case "/status":
		// Show current model and provider status
		statusMsg := fmt.Sprintf("📊 Current Configuration:\n  Provider: %s\n  Model: %s", m.provider, m.model)
		return borderedResponseMsg{content: statusMsg, isCommand: true}
	case "/system":
		// Show the current system prompt with tools
		messages := m.agent.GetMemory()
		if len(messages) > 0 && messages[0].Role == "system" {
			return borderedResponseMsg{
				content: fmt.Sprintf("**Current System Prompt (including tools):**\n\n%s", messages[0].Content),
				isCommand: true,
			}
		}
		// Fallback to default if no system message found
		systemPrompt := agent.DefaultConfig().SystemPrompt
		return borderedResponseMsg{
			content: fmt.Sprintf("**Default System Prompt:**\n\n%s", systemPrompt),
			isCommand: true,
		}
	case "/verbose":
		// Toggle verbose mode
		currentDebug := os.Getenv("SIMPLE_AGENT_DEBUG")
		if currentDebug == "true" {
			os.Unsetenv("SIMPLE_AGENT_DEBUG")
			return borderedResponseMsg{content: "Verbose mode: OFF", isCommand: true}
		} else {
			os.Setenv("SIMPLE_AGENT_DEBUG", "true")
			return borderedResponseMsg{content: "Verbose mode: ON\nDebug output will be shown in the terminal", isCommand: true}
		}
	default:
		return borderedResponseMsg{content: fmt.Sprintf("Unknown command: %s", cmd), isCommand: true}
	}
}

type borderedResponseMsg struct {
	content       string
	err           error
	isQuit        bool
	isClear       bool
	isCommand     bool // Flag to indicate this is a command response
	isModelSelect bool // Flag to trigger model selection
}

// modelSelectedMsg is sent when a model is selected
type modelSelectedMsg struct {
	provider string
	model    string
}

// toolEventMsg carries tool execution events
type toolEventMsg struct {
	event agent.StreamEvent
}


// adjustTextareaHeight dynamically adjusts the textarea height based on content
func (m *BorderedTUI) adjustTextareaHeight() {
	content := m.textarea.Value()
	if content == "" {
		m.textarea.SetHeight(1)
		return
	}
	
	// Count lines needed considering word wrapping
	lines := 1
	currentLineLength := 0
	textareaWidth := m.width - 8 // Account for borders, padding, and prompt
	if textareaWidth < 20 {
		textareaWidth = 20
	}
	
	for _, char := range content {
		if char == '\n' {
			lines++
			currentLineLength = 0
		} else {
			currentLineLength++
			if currentLineLength >= textareaWidth {
				lines++
				currentLineLength = 0
			}
		}
	}
	
	// Set height with a reasonable maximum
	maxHeight := 10
	if lines > maxHeight {
		lines = maxHeight
	}
	
	m.textarea.SetHeight(lines)
}

// formatArguments formats tool arguments for display
func (m *BorderedTUI) formatArguments(args map[string]interface{}) string {
	if len(args) == 0 {
		return ""
	}
	
	var parts []string
	for k, v := range args {
		parts = append(parts, fmt.Sprintf("%s=%v", k, v))
	}
	
	return fmt.Sprintf("(%s)", strings.Join(parts, ", "))
}

// listenForToolEvents creates a command that listens for tool events
func (m *BorderedTUI) listenForToolEvents() tea.Cmd {
	return func() tea.Msg {
		if m.toolEventChan == nil {
			return nil
		}
		
		event, ok := <-m.toolEventChan
		if !ok {
			// Channel closed
			return nil
		}
		
		return toolEventMsg{event: event}
	}
}
