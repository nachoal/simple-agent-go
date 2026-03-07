package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
	"github.com/nachoal/simple-agent-go/llm/lmstudio"
)

func main() {
	provider := flag.String("provider", "lmstudio", "Provider to canary")
	model := flag.String("model", "", "Optional model override")
	timeout := flag.Int("timeout-seconds", 45, "Outer timeout in seconds")
	inference := flag.Bool("inference", false, "Also run a tiny inference canary")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(*timeout)*time.Second)
	defer cancel()

	switch strings.ToLower(strings.TrimSpace(*provider)) {
	case "lmstudio", "lm-studio":
		if err := runLMStudioCanary(ctx, strings.TrimSpace(*model), *inference); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
			os.Exit(1)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: unsupported live canary provider %q\n", *provider)
		os.Exit(1)
	}

	fmt.Print("CANARY_OK\n")
}

func runLMStudioCanary(ctx context.Context, model string, inference bool) error {
	client, err := lmstudio.NewClient()
	if err != nil {
		return err
	}
	defer client.Close()

	models, err := client.ListModels(ctx)
	if err != nil {
		return err
	}
	if len(models) == 0 {
		return fmt.Errorf("lmstudio returned no models")
	}

	if !inference {
		return nil
	}

	if model == "" {
		model = models[0].ID
	}
	resp, err := client.Chat(ctx, &llm.ChatRequest{
		Model: model,
		Messages: []llm.Message{{
			Role:    llm.RoleUser,
			Content: llm.StringPtr("Reply with the single token CANARY_OK and nothing else."),
		}},
		MaxTokens: 16,
	})
	if err != nil {
		return err
	}
	if len(resp.Choices) == 0 || resp.Choices[0].Message.Content == nil {
		return fmt.Errorf("lmstudio inference returned no content")
	}
	if !strings.Contains(strings.ToUpper(*resp.Choices[0].Message.Content), "CANARY_OK") {
		return fmt.Errorf("lmstudio inference did not return CANARY_OK: %q", *resp.Choices[0].Message.Content)
	}
	return nil
}
