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
	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/llm/anthropic"
	"github.com/nachoal/simple-agent-go/llm/deepseek"
	"github.com/nachoal/simple-agent-go/llm/groq"
	"github.com/nachoal/simple-agent-go/llm/lmstudio"
	"github.com/nachoal/simple-agent-go/llm/moonshot"
	"github.com/nachoal/simple-agent-go/llm/ollama"
	"github.com/nachoal/simple-agent-go/llm/openai"
	"github.com/nachoal/simple-agent-go/llm/perplexity"
	"github.com/nachoal/simple-agent-go/tui"
	"github.com/nachoal/simple-agent-go/internal/toolinit"
	"github.com/nachoal/simple-agent-go/tools/registry"
)

var (
	// Flags
	provider   string
	model      string
	verbose    bool
	continueConv bool
	resume     string
	
	// Root command
	rootCmd = &cobra.Command{
		Use:   "simple-agent",
		Short: "AI agent with tool support",
		Long:  "Simple Agent Go - A powerful AI agent framework with multiple LLM providers and tool support",
		RunE:  runTUI,
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
	
	// TUI-specific flags
	rootCmd.Flags().BoolVarP(&continueConv, "continue", "c", false, "Continue last conversation")
	rootCmd.Flags().StringVarP(&resume, "resume", "r", "", "Resume specific session or show picker if empty")
	
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
	agentInstance := agent.New(llmClient,
		agent.WithMaxIterations(10),
		agent.WithTemperature(0.7),
	)
	
	// Handle continue/resume flags
	if continueConv {
		// TODO: Load last session
		fmt.Println("Loading last conversation...")
	} else if resume != "" {
		// TODO: Load specific session or show picker
		fmt.Printf("Resuming session: %s\n", resume)
	}
	
	// If verbose, show the enhanced system prompt (including tools)
	if verbose {
		// Get the system prompt from the agent's memory which includes tools
		memory := agentInstance.GetMemory()
		if len(memory) > 0 && memory[0].Role == "system" {
			fmt.Println("\n=== ENHANCED SYSTEM PROMPT (with tools) ===")
			fmt.Println(memory[0].Content)
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
		fmt.Println("===================\n")
		fmt.Println("Press Enter to continue...")
		fmt.Scanln()
	}
	
	// Create and run TUI (bordered version with providers)
	p := tea.NewProgram(
		tui.NewBorderedTUIWithProviders(llmClient, agentInstance, provider, model, providers, configManager),
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
	
	// Create agent
	agentInstance := agent.New(llmClient,
		agent.WithMaxIterations(10),
		agent.WithTemperature(0.7),
	)
	
	// If verbose, show the enhanced system prompt (including tools)
	if verbose {
		// Get the system prompt from the agent's memory which includes tools
		memory := agentInstance.GetMemory()
		if len(memory) > 0 && memory[0].Role == "system" {
			fmt.Println("\n=== ENHANCED SYSTEM PROMPT (with tools) ===")
			fmt.Println(memory[0].Content)
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
		fmt.Println("===================\n")
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
		"file_read":      "📄",
		"file_write":     "💾",
		"file_edit":      "📝",
		"directory_list": "📁",
		"shell":          "🖥️",
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