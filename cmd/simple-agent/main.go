package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/nachoal/simple-agent-go/agent"
	"github.com/nachoal/simple-agent-go/config"
	"github.com/nachoal/simple-agent-go/history"
	"github.com/nachoal/simple-agent-go/internal/harnessllm"
	"github.com/nachoal/simple-agent-go/internal/models"
	"github.com/nachoal/simple-agent-go/internal/resources"
	"github.com/nachoal/simple-agent-go/internal/runlog"
	"github.com/nachoal/simple-agent-go/internal/runtimeprompt"
	"github.com/nachoal/simple-agent-go/internal/selfknowledge"
	"github.com/nachoal/simple-agent-go/internal/toolinit"
	"github.com/nachoal/simple-agent-go/internal/userpaths"
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/llm/anthropic"
	"github.com/nachoal/simple-agent-go/llm/deepseek"
	"github.com/nachoal/simple-agent-go/llm/groq"
	"github.com/nachoal/simple-agent-go/llm/lmstudio"
	"github.com/nachoal/simple-agent-go/llm/minmax"
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
	toolsFlag    string
	maxTokens    int
	timeoutMins  int
	toolsJSON    bool
	doctorJSON   bool
	modelsJSON   bool

	customModelRegistry *models.Registry

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

	modelsCmd = &cobra.Command{
		Use:   "models",
		Short: "Model inspection commands",
	}

	listModelsCmd = &cobra.Command{
		Use:   "list",
		Short: "List available models by provider",
		RunE:  runListModels,
	}

	doctorCmd = &cobra.Command{
		Use:   "doctor",
		Short: "Show machine-readable runtime diagnostics",
		RunE:  runDoctor,
	}
)

func init() {
	// Register all tools
	toolinit.RegisterAll()

	// Global flags
	rootCmd.PersistentFlags().StringVar(&provider, "provider", "", "LLM provider (openai, anthropic, minmax, moonshot, etc)")
	rootCmd.PersistentFlags().StringVar(&model, "model", "", "Model to use")
	rootCmd.PersistentFlags().BoolVarP(&verbose, "verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().BoolVar(&yolo, "yolo", false, "Allow the bash tool to run any command (DANGEROUS)")
	rootCmd.PersistentFlags().StringVar(
		&toolsFlag,
		"tools",
		"",
		"Comma-separated tool names to enable (e.g. read,bash,edit,write). Use 'all' to enable all registered tools.",
	)

	// TUI-specific flags
	rootCmd.Flags().BoolVarP(&continueConv, "continue", "c", false, "Continue last conversation")
	rootCmd.Flags().StringVarP(&resume, "resume", "r", "", "Resume specific session ID or show picker if no ID provided")
	rootCmd.PersistentFlags().StringVar(&customParser, "custom-parser", "", "Enable custom parsing for provider output (e.g., 'lmstudio')")
	rootCmd.PersistentFlags().IntVar(&maxTokens, "max-tokens", 0, "Max tokens per completion (0 = use default: 8192)")
	rootCmd.PersistentFlags().IntVar(&timeoutMins, "timeout", 0, "Per-request timeout in minutes (0 = use default: 10)")

	// Set NoOptDefVal for resume flag - this value is used when -r is provided without an argument
	rootCmd.Flags().Lookup("resume").NoOptDefVal = "picker"

	// Add subcommands
	rootCmd.AddCommand(queryCmd)
	rootCmd.AddCommand(toolsCmd)
	rootCmd.AddCommand(modelsCmd)
	rootCmd.AddCommand(doctorCmd)
	toolsCmd.AddCommand(listToolsCmd)
	modelsCmd.AddCommand(listModelsCmd)
	listToolsCmd.Flags().BoolVar(&toolsJSON, "json", false, "Output tools as JSON")
	listModelsCmd.Flags().BoolVar(&modelsJSON, "json", false, "Output models as JSON")
	doctorCmd.Flags().BoolVar(&doctorJSON, "json", false, "Output diagnostics as JSON")

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

	// Resolve working directory once; runtime resources are cwd-aware.
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	resourceLoader, err := resources.NewLoader(cwd, "")
	if err != nil {
		return fmt.Errorf("failed to initialize resource loader: %w", err)
	}
	selfInfo := selfknowledge.Discover(cwd)

	modelsPath, err := models.DefaultModelsPath()
	if err != nil {
		return fmt.Errorf("failed to resolve models config path: %w", err)
	}
	customModelRegistry = models.NewRegistry(modelsPath)
	if err := customModelRegistry.Reload(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	buildSystemPrompt := func() string {
		return runtimeprompt.Build(agent.DefaultConfig().SystemPrompt, cwd, selfInfo, resourceLoader.Snapshot())
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
	provider = canonicalProvider(provider)

	// Debug: Show loaded provider and model
	if verbose {
		fmt.Printf("Using provider: %s, model: %s\n", provider, model)
	}

	// Create initial LLM client
	providerSetByFlag := cmd.Flags().Changed("provider")
	llmClient, provider, model, fallbackMsg, err := createLLMClientWithStartupFallback(provider, model, !providerSetByFlag)
	if err != nil {
		return fmt.Errorf("failed to create %s client: %w", provider, err)
	}
	if fallbackMsg != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", fallbackMsg)
	}
	defer llmClient.Close()

	// Create all provider clients for model selection.
	providers := make(map[string]llm.Client)
	providerNames := allProviderNames()
	successCount := 0
	for _, name := range providerNames {
		client, err := createLLMClient(name, getDefaultModel(name))
		if err == nil {
			providers[name] = client
			successCount++
		}
	}
	// Ensure currently selected provider is available in selector map.
	if _, ok := providers[strings.ToLower(provider)]; !ok {
		providers[strings.ToLower(provider)] = llmClient
		successCount++
	}
	if verbose {
		fmt.Printf("Loaded %d/%d providers for model selection\n", successCount, len(providerNames))
	}

	// Create agent
	// Determine custom parsers
	enableLMStudioParser := strings.Contains(strings.ToLower(customParser), "lmstudio")

	toolsRaw := strings.TrimSpace(toolsFlag)
	toolsOverride, toolsAll, err := parseToolsOverride(toolsRaw)
	if err != nil {
		return err
	}

	effectiveToolsForHeader := agent.DefaultConfig().Tools
	buildAgentOptions := func(modelName string) []agent.Option {
		opts := []agent.Option{
			agent.WithModel(modelName),
			agent.WithSystemPrompt(buildSystemPrompt()),
			agent.WithMaxIterations(1000),
			agent.WithMaxToolCalls(1000),
			agent.WithTemperature(0.7),
			agent.WithLMStudioParser(enableLMStudioParser),
		}
		if maxTokens > 0 {
			opts = append(opts, agent.WithMaxTokens(maxTokens))
		}
		if timeoutMins > 0 {
			opts = append(opts, agent.WithTimeout(time.Duration(timeoutMins)*time.Minute))
		}
		if toolsRaw != "" {
			if toolsAll {
				opts = append(opts, agent.WithTools(nil)) // empty means "all tools"
			} else {
				opts = append(opts, agent.WithTools(toolsOverride))
			}
		}
		return opts
	}
	if toolsRaw != "" {
		if toolsAll {
			effectiveToolsForHeader = nil
		} else {
			effectiveToolsForHeader = toolsOverride
		}
	}

	agentInstance := agent.New(llmClient, buildAgentOptions(model)...)

	// Initialize history manager
	historyMgr, err := history.NewManager()
	if err != nil {
		return fmt.Errorf("failed to initialize history: %w", err)
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
				llmClient, provider, model, fallbackMsg, err = createLLMClientWithStartupFallback(provider, model, true)
				if err != nil {
					return fmt.Errorf("failed to create %s client: %w", provider, err)
				}
				if fallbackMsg != "" {
					fmt.Fprintf(os.Stderr, "Warning: %s\n", fallbackMsg)
					session.Provider = provider
					session.Model = model
				}
				agentInstance = agent.New(llmClient,
					buildAgentOptions(model)...,
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
			llmClient, provider, model, fallbackMsg, err = createLLMClientWithStartupFallback(provider, model, true)
			if err != nil {
				return fmt.Errorf("failed to create %s client: %w", provider, err)
			}
			if fallbackMsg != "" {
				fmt.Fprintf(os.Stderr, "Warning: %s\n", fallbackMsg)
				session.Provider = provider
				session.Model = model
			}
			agentInstance = agent.New(llmClient,
				buildAgentOptions(model)...,
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
		// Session history includes the original system prompt; ensure it's updated for this run's toolset.
		historyAgent.SetSystemPrompt(buildSystemPrompt())
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
	tui.PrintHeader(provider, model, effectiveToolsForHeader)

	// Create and run TUI (bordered version with providers and history)
	tuiModel := tui.NewBorderedTUIWithHistory(llmClient, historyAgent, provider, model, providers, configManager)
	tuiModel.SetClientFactory(func(providerName, modelName string) (llm.Client, error) {
		return createLLMClient(providerName, modelName)
	})
	tuiModel.SetSystemPromptBuilder(buildSystemPrompt)
	tuiModel.SetStaticModelsLoader(func() map[string][]llm.Model {
		if customModelRegistry == nil {
			return map[string][]llm.Model{}
		}
		return customModelRegistry.StaticModels()
	})
	tuiModel.SetRuntimeReloader(func() error {
		resourceLoader.Reload()
		if customModelRegistry != nil {
			if err := customModelRegistry.Reload(); err != nil {
				return err
			}
		}
		refreshed := make(map[string]llm.Client)
		for _, name := range allProviderNames() {
			client, err := createLLMClient(name, getDefaultModel(name))
			if err == nil {
				refreshed[name] = client
			}
		}
		for name := range providers {
			delete(providers, name)
		}
		for name, client := range refreshed {
			providers[name] = client
		}
		return nil
	})

	p := tea.NewProgram(tuiModel)

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

	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	queryLogger, loggerErr := runlog.New(cwd, "query")
	if loggerErr == nil {
		defer queryLogger.Close()
	}

	resourceLoader, err := resources.NewLoader(cwd, "")
	if err != nil {
		return fmt.Errorf("failed to initialize resource loader: %w", err)
	}
	selfInfo := selfknowledge.Discover(cwd)
	buildSystemPrompt := func() string {
		return runtimeprompt.Build(agent.DefaultConfig().SystemPrompt, cwd, selfInfo, resourceLoader.Snapshot())
	}

	modelsPath, err := models.DefaultModelsPath()
	if err != nil {
		return fmt.Errorf("failed to resolve models config path: %w", err)
	}
	customModelRegistry = models.NewRegistry(modelsPath)
	if err := customModelRegistry.Reload(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	// Get provider and model
	if provider == "" {
		provider = getEnvOrDefault("DEFAULT_PROVIDER", "openai")
	}
	provider = canonicalProvider(provider)
	if model == "" {
		model = getEnvOrDefault("DEFAULT_MODEL", getDefaultModel(provider))
	}

	// Create LLM client
	providerSetByFlag := cmd.Flags().Changed("provider")
	llmClient, _, _, fallbackMsg, err := createLLMClientWithStartupFallback(provider, model, !providerSetByFlag)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}
	if fallbackMsg != "" {
		fmt.Fprintf(os.Stderr, "Warning: %s\n", fallbackMsg)
	}
	defer llmClient.Close()

	// Determine custom parsers
	enableLMStudioParser := strings.Contains(strings.ToLower(customParser), "lmstudio")

	// Create agent
	toolsRaw := strings.TrimSpace(toolsFlag)
	toolsOverride, toolsAll, err := parseToolsOverride(toolsRaw)
	if err != nil {
		return err
	}

	agentOpts := []agent.Option{
		agent.WithModel(model),
		agent.WithSystemPrompt(buildSystemPrompt()),
		agent.WithMaxIterations(1000),
		agent.WithMaxToolCalls(1000),
		agent.WithTemperature(0.7),
		agent.WithLMStudioParser(enableLMStudioParser),
	}
	if maxTokens > 0 {
		agentOpts = append(agentOpts, agent.WithMaxTokens(maxTokens))
	}
	if timeoutMins > 0 {
		agentOpts = append(agentOpts, agent.WithTimeout(time.Duration(timeoutMins)*time.Minute))
	}
	if toolsRaw != "" {
		if toolsAll {
			agentOpts = append(agentOpts, agent.WithTools(nil)) // empty means "all tools"
		} else {
			agentOpts = append(agentOpts, agent.WithTools(toolsOverride))
		}
	}

	agentInstance := agent.New(llmClient, agentOpts...)

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
	runID := fmt.Sprintf("query-%d", time.Now().UnixNano())
	if queryLogger != nil {
		ctx = runlog.WithContext(ctx, queryLogger)
		ctx = runlog.WithMetadata(ctx, runlog.Metadata{
			RunID:     runID,
			Mode:      "query",
			Prompt:    query,
			Provider:  provider,
			Model:     model,
			TracePath: queryLogger.Path(),
		})
		runlog.EventFromContext(ctx, "run_start", nil)
	}
	response, err := agentInstance.Query(ctx, query)
	if err != nil {
		if queryLogger != nil {
			runlog.EventFromContext(ctx, "run_end", map[string]interface{}{
				"status": "error",
				"error":  err.Error(),
			})
		}
		return fmt.Errorf("query failed: %w", err)
	}

	// Print response
	fmt.Println(response.Content)

	if queryLogger != nil {
		fields := map[string]interface{}{
			"status":       "completed",
			"response_len": len(response.Content),
		}
		if response.Usage != nil {
			fields["total_tokens"] = response.Usage.TotalTokens
		}
		runlog.EventFromContext(ctx, "run_end", fields)
	}

	if verbose && response.Usage != nil {
		fmt.Printf("\n[Tokens: %d]\n", response.Usage.TotalTokens)
	}

	return nil
}

func listTools(cmd *cobra.Command, args []string) {
	toolNames := registry.List()

	if toolsJSON {
		sort.Strings(toolNames)
		payload := make([]map[string]string, 0, len(toolNames))
		for _, name := range toolNames {
			tool, err := registry.Get(name)
			if err != nil || tool == nil {
				continue
			}
			payload = append(payload, map[string]string{
				"name":        name,
				"description": tool.Description(),
			})
		}
		data, err := json.MarshalIndent(payload, "", "  ")
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: failed to marshal tools: %v\n", err)
			return
		}
		fmt.Println(string(data))
		return
	}

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

type doctorReport struct {
	Cwd             string   `json:"cwd"`
	ConfigDir       string   `json:"config_dir"`
	AgentDir        string   `json:"agent_dir"`
	HarnessDir      string   `json:"harness_dir"`
	DefaultProvider string   `json:"default_provider"`
	DefaultModel    string   `json:"default_model"`
	RegisteredTools []string `json:"registered_tools"`
	ContextFiles    []string `json:"context_files"`
	PromptFragments []string `json:"prompt_fragments"`
}

type providerModelsReport struct {
	Provider string      `json:"provider"`
	Error    string      `json:"error,omitempty"`
	Models   []llm.Model `json:"models,omitempty"`
}

func runDoctor(cmd *cobra.Command, args []string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return fmt.Errorf("failed to get current directory: %w", err)
	}

	configDir, err := userpaths.ConfigDir()
	if err != nil {
		return err
	}
	agentDir, err := userpaths.AgentDir()
	if err != nil {
		return err
	}
	harnessDir, err := userpaths.HarnessDir(cwd)
	if err != nil {
		return err
	}
	configManager, err := config.NewManager()
	if err != nil {
		return err
	}
	loader, err := resources.NewLoader(cwd, "")
	if err != nil {
		return err
	}
	snapshot := loader.Snapshot()

	report := doctorReport{
		Cwd:             cwd,
		ConfigDir:       configDir,
		AgentDir:        agentDir,
		HarnessDir:      harnessDir,
		DefaultProvider: configManager.GetDefaultProvider(),
		DefaultModel:    configManager.GetDefaultModel(),
		RegisteredTools: registry.List(),
		ContextFiles:    collectLoadedPaths(snapshot.ContextFiles),
		PromptFragments: collectLoadedPaths(snapshot.PromptFragments),
	}
	sort.Strings(report.RegisteredTools)

	if doctorJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal doctor report: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Cwd: %s\n", report.Cwd)
	fmt.Printf("ConfigDir: %s\n", report.ConfigDir)
	fmt.Printf("AgentDir: %s\n", report.AgentDir)
	fmt.Printf("HarnessDir: %s\n", report.HarnessDir)
	fmt.Printf("DefaultProvider: %s\n", report.DefaultProvider)
	fmt.Printf("DefaultModel: %s\n", report.DefaultModel)
	fmt.Printf("RegisteredTools: %d\n", len(report.RegisteredTools))
	for _, path := range report.ContextFiles {
		fmt.Printf("ContextFile: %s\n", path)
	}
	for _, path := range report.PromptFragments {
		fmt.Printf("PromptFragment: %s\n", path)
	}
	return nil
}

func runListModels(cmd *cobra.Command, args []string) error {
	modelsPath, err := models.DefaultModelsPath()
	if err != nil {
		return fmt.Errorf("failed to resolve models config path: %w", err)
	}
	customModelRegistry = models.NewRegistry(modelsPath)
	if err := customModelRegistry.Reload(); err != nil {
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	staticModels := map[string][]llm.Model{}
	if customModelRegistry != nil {
		staticModels = customModelRegistry.StaticModels()
	}

	report := make([]providerModelsReport, 0, len(allProviderNames()))
	for _, providerName := range allProviderNames() {
		entry := providerModelsReport{Provider: providerName}
		if modelsForProvider, ok := staticModels[providerName]; ok && len(modelsForProvider) > 0 {
			entry.Models = modelsForProvider
		} else {
			entry.Models = []llm.Model{{ID: getDefaultModel(providerName)}}
		}
		report = append(report, entry)
	}

	if modelsJSON {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to marshal models report: %w", err)
		}
		fmt.Println(string(data))
		return nil
	}

	for _, entry := range report {
		fmt.Printf("%s:\n", entry.Provider)
		if entry.Error != "" {
			fmt.Printf("  error: %s\n", entry.Error)
			continue
		}
		if len(entry.Models) == 0 {
			fmt.Println("  (no models)")
			continue
		}
		sort.Slice(entry.Models, func(i, j int) bool {
			return entry.Models[i].ID < entry.Models[j].ID
		})
		for _, model := range entry.Models {
			fmt.Printf("  - %s\n", model.ID)
		}
	}

	return nil
}

func collectLoadedPaths(files []resources.LoadedFile) []string {
	out := make([]string, 0, len(files))
	for _, file := range files {
		out = append(out, file.Path)
	}
	return out
}

func truncateForQueryLog(input string, max int) string {
	input = strings.Join(strings.Fields(strings.TrimSpace(input)), " ")
	if max <= 0 || len(input) <= max {
		return input
	}
	if max <= 3 {
		return input[:max]
	}
	return input[:max-3] + "..."
}

func parseToolsOverride(raw string) ([]string, bool, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, false, nil
	}

	if strings.EqualFold(raw, "all") {
		// Empty tool list means "send all registered tools".
		return nil, true, nil
	}

	var parts []string
	if strings.Contains(raw, ",") {
		parts = strings.Split(raw, ",")
	} else {
		parts = strings.Fields(raw)
	}

	out := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))

	for _, p := range parts {
		name := strings.ToLower(strings.TrimSpace(p))
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}

		if _, err := registry.Get(name); err != nil {
			hint := ""
			if name == "shell" {
				hint = " (did you mean \"bash\"?)"
			}
			return nil, false, fmt.Errorf("unknown tool %q%s; run 'simple-agent tools list' to see available tools", name, hint)
		}

		out = append(out, name)
		seen[name] = struct{}{}
	}

	if len(out) == 0 {
		return nil, false, fmt.Errorf("no tools specified in --tools; use e.g. --tools read,bash or --tools all")
	}

	return out, false, nil
}

func createLLMClient(provider, model string) (llm.Client, error) {
	clientOpts := clientOptionsForModel(model)

	if harnessllm.Enabled() {
		return harnessllm.New(clientOpts...)
	}

	normalizedProvider := canonicalProvider(provider)

	if customModelRegistry != nil {
		if cfg, ok := customModelRegistry.Provider(normalizedProvider); ok {
			// If a custom provider is declared, or a built-in provider is overridden
			// with a baseUrl, route requests through the custom configuration.
			if cfg.BaseURL != "" || !models.IsBuiltInProvider(normalizedProvider) {
				return createCustomConfiguredClient(cfg, model)
			}
		}
	}

	switch normalizedProvider {
	case "openai":
		return openai.NewClient(clientOpts...)

	case "anthropic":
		return anthropic.NewClient(clientOpts...)

	case "minmax":
		return minmax.NewClient(clientOpts...)

	case "moonshot":
		return moonshot.NewClient(clientOpts...)

	case "deepseek":
		return deepseek.NewClient(clientOpts...)

	case "perplexity":
		return perplexity.NewClient(clientOpts...)

	case "groq":
		return groq.NewClient(clientOpts...)

	case "lmstudio":
		return lmstudio.NewClient(clientOpts...)

	case "ollama":
		return ollama.NewClient(clientOpts...)

	default:
		return nil, fmt.Errorf("unknown provider: %s", provider)
	}
}

func createLLMClientWithStartupFallback(provider, model string, allowFallback bool) (llm.Client, string, string, string, error) {
	normalizedProvider := canonicalProvider(provider)
	chosenModel := strings.TrimSpace(model)
	if chosenModel == "" {
		chosenModel = getDefaultModel(normalizedProvider)
	}

	client, err := createLLMClient(normalizedProvider, chosenModel)
	if err == nil {
		return client, normalizedProvider, chosenModel, "", nil
	}

	if !allowFallback || !isLMStudioProvider(normalizedProvider) {
		return nil, normalizedProvider, chosenModel, "", err
	}

	fallbackProvider := "openai"
	fallbackModel := getDefaultModel(fallbackProvider)
	fallbackClient, fallbackErr := createLLMClient(fallbackProvider, fallbackModel)
	if fallbackErr != nil {
		return nil, normalizedProvider, chosenModel, "", fmt.Errorf(
			"%w (fallback to %s/%s also failed: %v)",
			err,
			fallbackProvider,
			fallbackModel,
			fallbackErr,
		)
	}

	msg := fmt.Sprintf(
		"LM Studio is unavailable for provider %q; using %s (%s) instead",
		normalizedProvider,
		fallbackProvider,
		fallbackModel,
	)
	return fallbackClient, fallbackProvider, fallbackModel, msg, nil
}

func isLMStudioProvider(provider string) bool {
	return canonicalProvider(provider) == "lmstudio"
}

func getDefaultModel(provider string) string {
	normalizedProvider := canonicalProvider(provider)

	if customModelRegistry != nil {
		if cfg, ok := customModelRegistry.Provider(normalizedProvider); ok && len(cfg.Models) > 0 {
			// models.json list order defines preference.
			return cfg.Models[0].ID
		}
	}

	defaults := map[string]string{
		"openai":     "gpt-4-turbo-preview",
		"anthropic":  "claude-3-opus-20240229",
		"minmax":     "MiniMax-M2.5",
		"moonshot":   "moonshot-v1-8k",
		"deepseek":   "deepseek-chat",
		"perplexity": "llama-3.1-sonar-huge-128k-online",
		"groq":       "mixtral-8x7b-32768",
		"lmstudio":   "local-model",
		"ollama":     "llama2",
	}

	if model, ok := defaults[normalizedProvider]; ok {
		return model
	}
	return "default"
}

func canonicalProvider(provider string) string {
	normalized := models.NormalizeProvider(provider)
	switch normalized {
	case "claude":
		return "anthropic"
	case "minimax":
		return "minmax"
	case "kimi":
		return "moonshot"
	default:
		return normalized
	}
}

func allProviderNames() []string {
	base := []string{"openai", "anthropic", "minmax", "moonshot", "deepseek", "perplexity", "groq", "lmstudio", "ollama"}
	seen := make(map[string]struct{}, len(base))
	for _, name := range base {
		seen[name] = struct{}{}
	}

	if customModelRegistry != nil {
		for _, cfg := range customModelRegistry.Providers() {
			name := canonicalProvider(cfg.Name)
			if name == "" {
				continue
			}
			if _, ok := seen[name]; ok {
				continue
			}
			seen[name] = struct{}{}
			base = append(base, name)
		}
	}

	sort.Strings(base)
	return base
}

func clientOptionsForModel(model string) []llm.ClientOption {
	opts := []llm.ClientOption{llm.WithModel(model)}
	timeout := time.Duration(timeoutMins) * time.Minute
	if timeout <= 0 {
		timeout = agent.DefaultConfig().Timeout
	}
	if timeout > 0 {
		opts = append(opts, llm.WithTimeout(timeout))
	}
	return opts
}

func createCustomConfiguredClient(cfg models.ProviderConfig, model string) (llm.Client, error) {
	if cfg.BaseURL == "" {
		return nil, fmt.Errorf("provider %q requires baseUrl in models.json", cfg.Name)
	}
	if cfg.API != "" && !strings.EqualFold(cfg.API, "openai-completions") {
		return nil, fmt.Errorf("provider %q api %q is unsupported (supported: openai-completions)", cfg.Name, cfg.API)
	}

	headers := make(map[string]string)
	for k, v := range cfg.Headers {
		resolved := models.ResolveConfigValue(v)
		if resolved != "" {
			headers[k] = resolved
		}
	}

	apiKey := models.ResolveConfigValue(cfg.APIKey)
	if cfg.AuthHeader {
		if apiKey == "" {
			return nil, fmt.Errorf("provider %q has authHeader=true but no apiKey value", cfg.Name)
		}
		if _, ok := headers["Authorization"]; !ok {
			headers["Authorization"] = "Bearer " + apiKey
		}
	}

	normalized := canonicalProvider(cfg.Name)
	if normalized == "lmstudio" || apiKey == "" {
		opts := append(clientOptionsForModel(model), llm.WithBaseURL(cfg.BaseURL))
		if len(headers) > 0 {
			opts = append(opts, llm.WithHeaders(headers))
		}
		return lmstudio.NewClient(opts...)
	}

	opts := append(clientOptionsForModel(model),
		llm.WithBaseURL(cfg.BaseURL),
		llm.WithAPIKey(apiKey),
	)
	if len(headers) > 0 {
		opts = append(opts, llm.WithHeaders(headers))
	}
	return openai.NewClient(opts...)
}

func getEnvOrDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
