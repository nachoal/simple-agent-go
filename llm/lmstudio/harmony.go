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
	// Two formats:
	// 1. Old format: <|channel|>commentary to=functions.tool_name <|constrain|>json<|message|>{"args": "value"}
	// 2. New format: <|channel|>commentary<|message|>{"name": "tool_name", "arguments": {...}}
	
	// Try old format first
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
				// GPT-OSS sends {"path": "...", "content": "..."} directly
				// But our tools expect {"input": "{\"path\": \"...\", \"content\": \"...\"}"}
				if argsJSON != "" {
					// For file tools and other tools expecting "input" parameter,
					// we need to wrap the JSON in an input field
					// All simple-agent tools expect "input" parameter with JSON string
					needsInputWrapper := toolName == "file_write" || toolName == "file_read" || 
										toolName == "file_edit" || toolName == "directory_list" ||
										toolName == "shell" || toolName == "calculate" ||
										toolName == "wikipedia" || toolName == "google_search"
					
					if needsInputWrapper {
						// Wrap the arguments as a JSON string in an "input" field
						wrappedArgs := map[string]interface{}{
							"input": argsJSON,
						}
						if wrapped, err := json.Marshal(wrappedArgs); err == nil {
							toolCall.Function.Arguments = json.RawMessage(wrapped)
						} else {
							// Fallback to original if wrapping fails
							toolCall.Function.Arguments = json.RawMessage(argsJSON)
						}
					} else {
						// For other tools, use the arguments directly
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
				}
				
				response.ToolCalls = append(response.ToolCalls, toolCall)
			}
		}
	} else {
		// Try new format: <|channel|>commentary<|message|>{"name": "tool_name", "arguments": {...}}
		newCommentaryRe := regexp.MustCompile(`(?s)<\|channel\|>commentary(?:\s+[^<]*)?<\|message\|>(.*?)(?:<\|call\|>|<\|end\|>|$)`)
		if matches := newCommentaryRe.FindAllStringSubmatch(content, -1); len(matches) > 0 {
			for _, match := range matches {
				if len(match) > 1 {
					jsonContent := strings.TrimSpace(match[1])
					
					// Try to parse as {"name": "tool_name", "arguments": {...}}
					var toolPayload struct {
						Name      string          `json:"name"`
						Arguments json.RawMessage `json:"arguments"`
					}
					
					if err := json.Unmarshal([]byte(jsonContent), &toolPayload); err == nil && toolPayload.Name != "" {
						// Extract the actual arguments
						var args map[string]interface{}
						if err := json.Unmarshal(toolPayload.Arguments, &args); err == nil {
							// Create tool call
							toolCall := llm.ToolCall{
								ID:   fmt.Sprintf("harmony_%s_%d", toolPayload.Name, len(response.ToolCalls)),
								Type: "function",
								Function: llm.FunctionCall{
									Name: toolPayload.Name,
								},
							}
							
							// All simple-agent tools expect "input" parameter with JSON string
							needsInputWrapper := toolPayload.Name == "file_write" || toolPayload.Name == "file_read" || 
												toolPayload.Name == "file_edit" || toolPayload.Name == "directory_list" ||
												toolPayload.Name == "shell" || toolPayload.Name == "calculate" ||
												toolPayload.Name == "wikipedia" || toolPayload.Name == "google_search"
							
							if needsInputWrapper {
								// Wrap the arguments as a JSON string in an "input" field
								wrappedArgs := map[string]interface{}{
									"input": string(toolPayload.Arguments),
								}
								if wrapped, err := json.Marshal(wrappedArgs); err == nil {
									toolCall.Function.Arguments = json.RawMessage(wrapped)
								} else {
									// Fallback to original if wrapping fails
									toolCall.Function.Arguments = toolPayload.Arguments
								}
							} else {
								// For other tools, use the arguments directly
								toolCall.Function.Arguments = toolPayload.Arguments
							}
							
							response.ToolCalls = append(response.ToolCalls, toolCall)
							response.Commentary = fmt.Sprintf("Tool: %s, Args: %s", toolPayload.Name, string(toolPayload.Arguments))
						}
					}
				}
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