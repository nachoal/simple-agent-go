package harnessllm

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/llm"
)

const envName = "SIMPLE_AGENT_FAKE_LLM"

type Client struct {
	mode         string
	defaultModel string
}

func Enabled() bool {
	return strings.TrimSpace(os.Getenv(envName)) != ""
}

func New(opts ...llm.ClientOption) (*Client, error) {
	options := llm.ClientOptions{DefaultModel: "harness-fake-model"}
	for _, opt := range opts {
		opt(&options)
	}
	mode := strings.TrimSpace(os.Getenv(envName))
	if mode == "" {
		mode = "echo"
	}
	return &Client{
		mode:         mode,
		defaultModel: options.DefaultModel,
	}, nil
}

func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
	content := c.replyText(request.Messages)
	return &llm.ChatResponse{
		Model: request.Model,
		Choices: []llm.Choice{{
			Message: llm.Message{
				Role:    llm.RoleAssistant,
				Content: llm.StringPtr(content),
			},
			FinishReason: "stop",
		}},
		Usage: &llm.Usage{PromptTokens: 1, CompletionTokens: 1, TotalTokens: 2},
	}, nil
}

func (c *Client) ChatStream(ctx context.Context, request *llm.ChatRequest) (<-chan llm.StreamEvent, error) {
	ch := make(chan llm.StreamEvent, 8)
	go func() {
		defer close(ch)
		content := c.replyText(request.Messages)
		chunks := []string{content}
		delay := 0 * time.Millisecond
		if c.mode == "slow-stream" {
			delay = 300 * time.Millisecond
			chunks = []string{"stub ", "stream ", "response ", "that ", "should ", "be ", "interruptible ", "before ", "it ", "finishes"}
			if strings.Contains(strings.ToLower(lastUserMessage(request.Messages)), "what was my last user message") {
				chunks = []string{content}
			}
		}

		for _, chunk := range chunks {
			if delay > 0 {
				select {
				case <-ctx.Done():
					return
				case <-time.After(delay):
				}
			}
			select {
			case <-ctx.Done():
				return
			case ch <- llm.StreamEvent{
				Choices: []llm.Choice{{
					Delta: &llm.Message{
						Content: llm.StringPtr(chunk),
					},
				}},
			}:
			}
		}
	}()
	return ch, nil
}

func (c *Client) ListModels(context.Context) ([]llm.Model, error) {
	return []llm.Model{{
		ID:      c.defaultModel,
		OwnedBy: "harness",
	}}, nil
}

func (c *Client) GetModel(context.Context, string) (*llm.Model, error) {
	return &llm.Model{ID: c.defaultModel, OwnedBy: "harness"}, nil
}

func (c *Client) Close() error { return nil }

func (c *Client) replyText(messages []llm.Message) string {
	current := lastUserMessage(messages)
	prev := previousUserMessage(messages)
	lower := strings.ToLower(strings.TrimSpace(current))

	switch {
	case strings.Contains(lower, "reply with the single token canary_ok"):
		return "CANARY_OK"
	case strings.Contains(lower, "reply with ok only"):
		return "OK"
	case strings.Contains(lower, "what was my last user message"):
		if strings.TrimSpace(prev) == "" {
			return "(none)"
		}
		return prev
	default:
		return fmt.Sprintf("stub:%s", strings.TrimSpace(current))
	}
}

func lastUserMessage(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == llm.RoleUser && messages[i].Content != nil {
			return *messages[i].Content
		}
	}
	return ""
}

func previousUserMessage(messages []llm.Message) string {
	foundCurrent := false
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != llm.RoleUser || messages[i].Content == nil {
			continue
		}
		if !foundCurrent {
			foundCurrent = true
			continue
		}
		return *messages[i].Content
	}
	return ""
}
