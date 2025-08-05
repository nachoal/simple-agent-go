package lmstudio

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/nachoal/simple-agent-go/llm"
)

// HarmonyResponse represents the parsed Harmony format response
type HarmonyResponse struct {
	Analysis   string            // Content from analysis channel
	Final      string            // Content from final channel (user-visible)
	Commentary string            // Content from commentary channel (tool calls)
	ToolCalls  []llm.ToolCall   // Parsed tool calls
}

// ParseHarmonyFormat parses GPT-OSS Harmony format output
func ParseHarmonyFormat(content string) (*HarmonyResponse, error) {
	response := &HarmonyResponse{}
	
	// Extract analysis channel (make the regex more greedy and handle multiline)
	analysisRe := regexp.MustCompile(`(?s)<\|channel\|>analysis<\|message\|>(.*?)(?:<\|end\|>|<\|start\|>|$)`)
	if matches := analysisRe.FindStringSubmatch(content); len(matches) > 1 {
		response.Analysis = strings.TrimSpace(matches[1])
	}
	
	// Extract final channel (user-visible content)
	// Use (?s) flag to make . match newlines for multiline content
	finalRe := regexp.MustCompile(`(?s)<\|channel\|>final<\|message\|>(.*?)(?:<\|return\|>|<\|end\|>|$)`)
	if matches := finalRe.FindStringSubmatch(content); len(matches) > 1 {
		response.Final = strings.TrimSpace(matches[1])
	}
	
	// Extract commentary channel (tool calls)
	// Format: <|channel|>commentary to=functions.tool_name <|constrain|>json<|message|>{"args": "value"}
	commentaryRe := regexp.MustCompile(`(?s)<\|channel\|>commentary\s+to=functions\.(\w+).*?<\|message\|>(.*?)(?:<\|call\|>|<\|end\|>|$)`)
	if matches := commentaryRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for _, match := range matches {
			if len(match) > 2 {
				toolName := match[1]
				argsJSON := strings.TrimSpace(match[2])
				response.Commentary = fmt.Sprintf("Tool: %s, Args: %s", toolName, argsJSON)
				
				// Parse tool call
				toolCall := llm.ToolCall{
					ID:   fmt.Sprintf("harmony_%s_%d", toolName, len(response.ToolCalls)),
					Type: "function",
					Function: llm.FunctionCall{
						Name: toolName,
					},
				}
				
				// Convert the arguments to the expected format
				// GPT-OSS might send {"input": "value"} or other formats
				if argsJSON != "" {
					// Try to parse and re-encode to ensure valid JSON
					var parsedArgs map[string]interface{}
					if err := json.Unmarshal([]byte(argsJSON), &parsedArgs); err == nil {
						if reencoded, err := json.Marshal(parsedArgs); err == nil {
							toolCall.Function.Arguments = json.RawMessage(reencoded)
						} else {
							// If re-encoding fails, use the original
							toolCall.Function.Arguments = json.RawMessage(argsJSON)
						}
					} else {
						// If parsing fails, try to use as-is (might be valid JSON we can't parse)
						toolCall.Function.Arguments = json.RawMessage(argsJSON)
					}
				}
				
				response.ToolCalls = append(response.ToolCalls, toolCall)
			}
		}
	}
	
	// If no explicit final channel but we have content without tags, use it as final
	if response.Final == "" && !strings.Contains(content, "<|channel|>") {
		response.Final = strings.TrimSpace(content)
	}
	
	return response, nil
}

// ConvertToStandardResponse converts Harmony response to standard format
func ConvertToStandardResponse(harmony *HarmonyResponse) string {
	// Return the final content for display
	if harmony.Final != "" {
		return harmony.Final
	}
	// Don't return analysis content as it's internal reasoning
	// Return empty string if no final content
	return ""
}

// IsHarmonyFormat checks if the content contains Harmony format markers
func IsHarmonyFormat(content string) bool {
	return strings.Contains(content, "<|channel|>") || 
	       strings.Contains(content, "<|message|>") ||
	       strings.Contains(content, "<|end|>") ||
	       strings.Contains(content, "<|start|>")
}

// ExtractToolCallsFromCommentary extracts tool calls from commentary channel
// This handles various formats that GPT-OSS might use
func ExtractToolCallsFromCommentary(content string) []llm.ToolCall {
	var toolCalls []llm.ToolCall
	
	// Pattern 1: <|channel|>commentary to=functions.tool_name <|constrain|>json<|message|>{...}
	pattern1 := regexp.MustCompile(`to=functions\.(\w+).*?<\|message\|>\s*(\{[^}]+\})`)
	
	// Pattern 2: Alternative format that might appear
	pattern2 := regexp.MustCompile(`"name":\s*"(\w+)".*?"arguments":\s*(\{[^}]+\})`)
	
	// Try pattern 1 first
	if matches := pattern1.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		for i, match := range matches {
			if len(match) > 2 {
				toolName := match[1]
				argsJSON := match[2]
				
				toolCall := llm.ToolCall{
					ID:   fmt.Sprintf("harmony_call_%d", i),
					Type: "function",
					Function: llm.FunctionCall{
						Name:      toolName,
						Arguments: json.RawMessage(argsJSON),
					},
				}
				toolCalls = append(toolCalls, toolCall)
			}
		}
	} else if matches := pattern2.FindAllStringSubmatch(content, -1); len(matches) > 0 {
		// Fallback to pattern 2
		for i, match := range matches {
			if len(match) > 2 {
				toolName := match[1]
				argsJSON := match[2]
				
				toolCall := llm.ToolCall{
					ID:   fmt.Sprintf("harmony_call_%d", i),
					Type: "function",
					Function: llm.FunctionCall{
						Name:      toolName,
						Arguments: json.RawMessage(argsJSON),
					},
				}
				toolCalls = append(toolCalls, toolCall)
			}
		}
	}
	
	return toolCalls
}