package main

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/config"
	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/internal/toolinit"
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/llm/anthropic"
	"github.com/nachoal/simple-agent-go/llm/deepseek"
	"github.com/nachoal/simple-agent-go/llm/groq"
	"github.com/nachoal/simple-agent-go/llm/lmstudio"
	"github.com/nachoal/simple-agent-go/llm/moonshot"
	"github.com/nachoal/simple-agent-go/llm/ollama"
	"github.com/nachoal/simple-agent-go/llm/openai"
	"github.com/nachoal/simple-agent-go/llm/perplexity"
	"github.com/nachoal/simple-agent-go/tools/registry"
	"github.com/nachoal/simple-agent-go/tui"
)

var (
	// Flags
	provider     string
	model        string
	verbose      bool
	yolo         bool
	continueConv bool
	resume       string
	resumeSet    bool
	customParser string

	// Root command
	rootCmd = &cobra.Command{
		Use:   "simple-agent",
		Short: "AI agent with tool support",
		Long:  "Simple Agent Go - A powerful AI agent framework with multiple LLM providers and tool support",
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Enable debug logging if verbose flag is set
			if verbose {
				os.Setenv("SIMPLE_AGENT_DEBUG", "true")
			}

			// Allow unrestricted bash commands if --yolo is set (DANGEROUS)
			if yolo {
				os.Setenv("SIMPLE_AGENT_YOLO", "true")
			}

			// Check if resume flag was explicitly set
			resumeSet = cmd.Flags().Changed("resume")
		},
		RunE: runTUI,
	}

	// Query command for one-shot queries
	queryCmd = &cobra.Command{
		Use:   "query [message]",
		Short: "Send a one-shot query without entering TUI",
		Args:  cobra.MinimumNArgs(1),
		RunE:  runQuery,
	}

	// Tools command
	toolsCmd = &cobra.Command{
		Use:   "tools",
		Short: "Tool management commands",
	}

	// List tools subcommand
	listToolsCmd = &cobra.Command{
		Use:   "list",
		Short: "List available tools",
		Run:   listTools,
	}
)

func init() {
	// Register all tools
	toolinit.RegisterAll()

	// Global flags
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "", "LLM provider (openai, anthropic, moonshot, etc)")
	rootCmd.PersistentFlags().StringVar(&model, "model", "", "Model to use")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&yolo, "yolo", false, "Allow the bash tool to run any command (DANGEROUS)")

	// TUI-specific flags
	rootCmd.Flags().BoolVarP(&continueConv, "continue", "c", false, "Continue last conversation")
	rootCmd.Flags().StringVarP(&resume, "resume", "r", "", "Resume specific session ID or show picker if no ID provided")
	rootCmd.PersistentFlags().StringVar(&customParser, "custom-parser", "", "Enable custom parsing for provider output (e.g., 'lmstudio')")

	// Set NoOptDefVal for resume flag - this value is used when -r is provided without an argument
	rootCmd.Flags().Lookup("resume").NoOptDefVal = "picker"

	// Add subcommands
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(toolsCmd)
	toolsCmd.AddCommand(listToolsCmd)

	// Bind flags to viper
	viper.BindPFlags(rootCmd.PersistentFlags())
}

func main() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// It's okay if .env doesn't exist
		// Only print if the file exists but has an error
		if !os.IsNotExist(err) {
			fmt.Printf("Warning: Error loading .env file: %v\n", err)
		}
	}

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runTUI(cmd *cobra.Command, args []string) error {
	// Enable debug logging if verbose flag is set
	if verbose {
		os.Setenv("SIMPLE_AGENT_DEBUG", "true")
	}

	// Create config manager
	configManager, err := config.NewManager()
	if err != nil {
		return fmt.Errorf("failed to create config manager: %w", err)
	}

	// Get provider and model from config or flags
	if provider == "" {
		// First check config, then env, then default
		provider = configManager.GetDefaultProvider()
		if provider == "" {
			provider = getEnvOrDefault("DEFAULT_PROVIDER", "openai")
		}
	}
	if model == "" {
		model = configManager.GetDefaultModel()
		if model == "" {
			model = getEnvOrDefault("DEFAULT_MODEL", getDefaultModel(provider))
		}
	}

	// Normalize provider name to lowercase for consistency
	provider = strings.ToLower(provider)

	// Debug: Show loaded provider and model
	if verbose {
		fmt.Printf("Using provider: %s, model: %s\n", provider, model)
	}

	// Create initial LLM client
	llmClient, err := createLLMClient(provider, model)
	if err != nil {
		return fmt.Errorf("failed to create %s client: %w", provider, err)
	}
	defer llmClient.Close()

	// Create all provider clients for model selection
	providers := make(map[string]llm.Client)
	providerNames := []string{"openai", "anthropic", "moonshot", "deepseek", "perplexity", "groq", "lmstudio", "ollama"}

	// Debug: count successful providers
	successCount := 0

	for _, name := range providerNames {
		// Skip if it's the same as our current client
		if name == strings.ToLower(provider) {
			providers[name] = llmClient
			successCount++
			continue
		}

		// Try to create client, skip if API key is missing
		client, err := createLLMClient(name, getDefaultModel(name))
		if err == nil {
			providers[name] = client
			successCount++
		}
	}

	// If verbose, show how many providers were loaded
	if verbose {
		fmt.Printf("Loaded %d/%d providers for model selection\n", successCount, len(providerNames))
	}

	// Create agent
	// Determine custom parsers
	enableLMStudioParser := strings.Contains(strings.ToLower(customParser), "lmstudio")

	agentInstance := agent.New(llmClient,
		agent.WithMaxIterations(1000),
		agent.WithMaxToolCalls(1000),
		agent.WithTemperature(0.7),
		agent.WithLMStudioParser(enableLMStudioParser),
	)

	// Initialize history manager
	historyMgr, err := history.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize history: %w", err)
	}

	// Get current working directory
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	var session *history.Session
	var historyAgent *agent.HistoryAgent

	// Handle continue/resume flags
	if continueConv {
		// Try to load last session for this directory
		session, err = historyMgr.GetLastSessionForPath(cwd)
		if err != nil {
			fmt.Printf("No previous conversation found for this directory.\n")
			// Start new session
			session, err = historyMgr.StartSession(cwd, provider, model)
			if err != nil {
				return fmt.Errorf("failed to start session: %w", err)
			}
		} else {
			fmt.Printf("Continuing conversation from %s...\n", session.UpdatedAt.Format("Jan 02 15:04"))
			// Update provider/model from session if different
			if session.Provider != provider || session.Model != model {
				provider = session.Provider
				model = session.Model
				// Recreate client with session's provider/model
				llmClient.Close()
				llmClient, err = createLLMClient(provider, model)
				if err != nil {
					return fmt.Errorf("failed to create %s client: %w", provider, err)
				}
				agentInstance = agent.New(llmClient,
					agent.WithMaxIterations(1000),
					agent.WithMaxToolCalls(1000),
					agent.WithTemperature(0.7),
					agent.WithLMStudioParser(enableLMStudioParser),
				)
			}
		}
	} else if resumeSet {
		// Show session picker if no ID provided, or load specific session
		if resume == "picker" || resume == "list" || resume == "" {
			sessions, err := historyMgr.ListSessionsForPath(cwd)
			if err != nil {
				return fmt.Errorf("failed to list sessions: %w", err)
			}

			if len(sessions) == 0 {
				fmt.Println("No previous conversations found for this directory.")
				// Start new session
				session, err = historyMgr.StartSession(cwd, provider, model)
				if err != nil {
					return fmt.Errorf("failed to start session: %w", err)
				}
			} else {
				// Show session picker
				picker := tui.NewSessionPicker(sessions)
				p := tea.NewProgram(picker)

				pickerModel, err := p.Run()
				if err != nil {
					return fmt.Errorf("failed to run session picker: %w", err)
				}

				// Check if a session was selected
				if verbose {
					fmt.Printf("Picker model type: %T\n", pickerModel)
				}
				if pickerResult, ok := pickerModel.(*tui.SessionPicker); ok {
					if verbose {
						fmt.Printf("Picker result type assertion successful, SelectedSessionID: '%s'\n", pickerResult.SelectedSessionID)
					}
					if pickerResult.SelectedSessionID != "" {
						// Session was selected
						if verbose {
							fmt.Printf("Selected session ID: %s\n", pickerResult.SelectedSessionID)
						}
						session, err = historyMgr.LoadSession(pickerResult.SelectedSessionID)
						if err != nil {
							return fmt.Errorf("failed to load session: %w", err)
						}
						fmt.Printf("Resuming session from %s...\n", session.UpdatedAt.Format("Jan 02 15:04"))
						if verbose {
							fmt.Printf("Session has %d messages\n", len(session.Messages))
						}
					}
				} else {
					if verbose {
						fmt.Printf("Type assertion failed! Model type is: %T\n", pickerModel)
					}
				}

				if session == nil {
					// User cancelled - start new session instead
					session, err = historyMgr.StartSession(cwd, provider, model)
					if err != nil {
						return fmt.Errorf("failed to start session: %w", err)
					}
				}
			}
		} else {
			// Load specific session ID
			session, err = historyMgr.LoadSession(resume)
			if err != nil {
				return fmt.Errorf("failed to load session %s: %w", resume, err)
			}
		}

		// Update provider/model from session
		if session != nil {
			// Always update provider/model from the session
			provider = session.Provider
			model = session.Model
			// Recreate client with session's provider/model
			llmClient.Close()
			llmClient, err = createLLMClient(provider, model)
			if err != nil {
				return fmt.Errorf("failed to create %s client: %w", provider, err)
			}
			agentInstance = agent.New(llmClient,
				agent.WithMaxIterations(1000),
				agent.WithMaxToolCalls(1000),
				agent.WithTemperature(0.7),
				agent.WithLMStudioParser(enableLMStudioParser),
			)
		}
	} else {
		// Start new session
		session, err = historyMgr.StartSession(cwd, provider, model)
		if err != nil {
			return fmt.Errorf("failed to start session: %w", err)
		}
	}

	// Create history-aware agent
	historyAgent = agent.NewHistoryAgent(agentInstance, historyMgr, session)

	// Restore memory if continuing/resuming
	if continueConv || resumeSet {
		historyAgent.RestoreMemoryFromSession(session)
		if verbose && session != nil {
			fmt.Printf("Restored %d messages from session %s\n", len(session.Messages), session.ID)
		}
	}

	// If verbose, show the enhanced system prompt (including tools)
	if verbose {
		// Get the system prompt from the agent's memory which includes tools
		memory := agentInstance.GetMemory()
		if len(memory) > 0 && memory[0].Role == "system" {
			fmt.Println("\n=== ENHANCED SYSTEM PROMPT (with tools) ===")
			// memory[0].Content is a *string; print underlying value
			if memory[0].Content != nil {
				fmt.Println(*memory[0].Content)
			} else {
				fmt.Println("")
			}
		} else {
			fmt.Println("\n=== DEFAULT SYSTEM PROMPT ===")
			fmt.Println(agent.DefaultConfig().SystemPrompt)
			fmt.Println("\n=== AVAILABLE TOOLS ===")
			toolNames := registry.List()
			for _, name := range toolNames {
				tool, _ := registry.Get(name)
				if tool != nil {
					fmt.Printf("- %s: %s\n", name, tool.Description())
				}
			}
		}
		fmt.Println("===================")
	}

	// Print header before starting TUI
	tui.PrintHeader(provider, model)

	// Create and run TUI (bordered version with providers and history)
	p := tea.NewProgram(
		tui.NewBorderedTUIWithHistory(llmClient, historyAgent, provider, model, providers, configManager),
	)

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running TUI: %w", err)
	}

	return nil
}

func runQuery(cmd *cobra.Command, args []string) error {
	// Enable debug logging if verbose flag is set
	if verbose {
		os.Setenv("SIMPLE_AGENT_DEBUG", "true")
	}

	query := strings.Join(args, " ")

	// Get provider and model
	if provider == "" {
		provider = getEnvOrDefault("DEFAULT_PROVIDER", "openai")
	}
	if model == "" {
		model = getEnvOrDefault("DEFAULT_MODEL", getDefaultModel(provider))
	}

	// Create LLM client
	llmClient, err := createLLMClient(provider, model)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}
	defer llmClient.Close()

	// Determine custom parsers
	enableLMStudioParser := strings.Contains(strings.ToLower(customParser), "lmstudio")

	// Create agent
	agentInstance := agent.New(llmClient,
		agent.WithMaxIterations(1000),
		agent.WithMaxToolCalls(1000),
		agent.WithTemperature(0.7),
		agent.WithLMStudioParser(enableLMStudioParser),
	)

	// If verbose, show the enhanced system prompt (including tools)
	if verbose {
		// Get the system prompt from the agent's memory which includes tools
		memory := agentInstance.GetMemory()
		if len(memory) > 0 && memory[0].Role == "system" {
			fmt.Println("\n=== ENHANCED SYSTEM PROMPT (with tools) ===")
			if memory[0].Content != nil {
				fmt.Println(*memory[0].Content)
			} else {
				fmt.Println("")
			}
		} else {
			fmt.Println("\n=== DEFAULT SYSTEM PROMPT ===")
			fmt.Println(agent.DefaultConfig().SystemPrompt)
			fmt.Println("\n=== AVAILABLE TOOLS ===")
			toolNames := registry.List()
			for _, name := range toolNames {
				tool, _ := registry.Get(name)
				if tool != nil {
					fmt.Printf("- %s: %s\n", name, tool.Description())
				}
			}
		}
		fmt.Println("===================")
	}

	// Execute query
	ctx := context.Background()
	response, err := agentInstance.Query(ctx, query)
	if err != nil {
		return fmt.Errorf("query failed: %w", err)
	}

	// Print response
	fmt.Println(response.Content)

	if verbose && response.Usage != nil {
		fmt.Printf("\n[Tokens: %d]\n", response.Usage.TotalTokens)
	}

	return nil
}

func listTools(cmd *cobra.Command, args []string) {
	toolNames := registry.List()

	fmt.Println("Available tools:")

	// Define icons for tools
	icons := map[string]string{
		"calculate":      "🧮",
		"read":           "📄",
		"write":          "💾",
		"edit":           "📝",
		"directory_list": "📁",
		"bash":           "🖥️",
		"wikipedia":      "📚",
		"google_search":  "🔍",
	}

	// Sort tools by name for consistent output
	sort.Strings(toolNames)

	// Display tools
	for _, name := range toolNames {
		tool, err := registry.Get(name)
		if err != nil {
			continue
		}

		icon := icons[name]
		if icon == "" {
			icon = "🔧" // Default icon
		}

		// Format name with padding
		paddedName := fmt.Sprintf("%-15s", name)
		fmt.Printf("  %s %s - %s\n", icon, paddedName, tool.Description())
	}
}

func createLLMClient(provider, model string) (llm.Client, error) {
	switch strings.ToLower(provider) {
	case "openai":
		return openai.NewClient(llm.WithModel(model))

	case "anthropic", "claude":
		return anthropic.NewClient(llm.WithModel(model))

	case "moonshot", "kimi":
		return moonshot.NewClient(llm.WithModel(model))

	case "deepseek":
		return deepseek.NewClient(llm.WithModel(model))

	case "perplexity":
		return perplexity.NewClient(llm.WithModel(model))

	case "groq":
		return groq.NewClient(llm.WithModel(model))

	case "lmstudio", "lm-studio":
		return lmstudio.NewClient(llm.WithModel(model))

	case "ollama":
		return ollama.NewClient(llm.WithModel(model))

	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func getDefaultModel(provider string) string {
	defaults := map[string]string{
		"openai":     "gpt-4-turbo-preview",
		"anthropic":  "claude-3-opus-20240229",
		"moonshot":   "moonshot-v1-8k",
		"deepseek":   "deepseek-chat",
		"perplexity": "llama-3.1-sonar-huge-128k-online",
		"groq":       "mixtral-8x7b-32768",
		"lmstudio":   "local-model",
		"ollama":     "llama2",
	}

	if model, ok := defaults[strings.ToLower(provider)]; ok {
		return model
	}
	return "default"
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
