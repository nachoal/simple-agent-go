package lmstudio

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseHarmonyFormat(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		wantFinal    string
		wantAnalysis string
		wantToolName string
		wantToolArgs string
	}{
		{
			name: "Simple greeting response",
			input: `<|channel|>analysis<|message|>The user says "hi". Simple greeting. Respond accordingly.<|end|><|start|>assistant<|channel|>final<|message|>Hello! How can I assist you today?`,
			wantFinal: "Hello! How can I assist you today?",
			wantAnalysis: "The user says \"hi\". Simple greeting. Respond accordingly.",
			wantToolName: "",
			wantToolArgs: "",
		},
		{
			name: "Wikipedia tool call",
			input: `<|channel|>analysis<|message|>We need to perform Wikipedia search. Let's call wikipedia first.<|end|><|start|>assistant<|channel|>commentary to=functions.wikipedia <|constrain|>json<|message|>{"input":"Tunguska incident"}`,
			wantFinal: "",
			wantAnalysis: "We need to perform Wikipedia search. Let's call wikipedia first.",
			wantToolName: "wikipedia",
			wantToolArgs: `{"input":"{\"input\":\"Tunguska incident\"}"}`, // Now wrapped
		},
		{
			name: "Google search tool call",
			input: `<|channel|>commentary to=functions.google_search <|constrain|>json<|message|>{"query":"latest AI news"}`,
			wantFinal: "",
			wantAnalysis: "",
			wantToolName: "google_search",
			wantToolArgs: `{"input":"{\"query\":\"latest AI news\"}"}`, // Now wrapped
		},
		{
			name: "Multiple channels",
			input: `<|channel|>analysis<|message|>Thinking about the problem...<|end|><|channel|>final<|message|>Here's the answer<|end|>`,
			wantFinal: "Here's the answer",
			wantAnalysis: "Thinking about the problem...",
			wantToolName: "",
			wantToolArgs: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseHarmonyFormat(tt.input)
			if err != nil {
				t.Fatalf("ParseHarmonyFormat() error = %v", err)
			}
			
			// Check final content
			if result.Final != tt.wantFinal {
				t.Errorf("Final = %q, want %q", result.Final, tt.wantFinal)
			}
			
			// Check analysis content
			if result.Analysis != tt.wantAnalysis {
				t.Errorf("Analysis = %q, want %q", result.Analysis, tt.wantAnalysis)
			}
			
			// Check tool calls
			if tt.wantToolName != "" {
				if len(result.ToolCalls) == 0 {
					t.Errorf("Expected tool call but got none")
				} else {
					if result.ToolCalls[0].Function.Name != tt.wantToolName {
						t.Errorf("Tool name = %q, want %q", result.ToolCalls[0].Function.Name, tt.wantToolName)
					}
					
					// Compare JSON arguments (normalize whitespace)
					var gotArgs, wantArgs map[string]interface{}
					json.Unmarshal(result.ToolCalls[0].Function.Arguments, &gotArgs)
					json.Unmarshal([]byte(tt.wantToolArgs), &wantArgs)
					
					gotJSON, _ := json.Marshal(gotArgs)
					wantJSON, _ := json.Marshal(wantArgs)
					
					if string(gotJSON) != string(wantJSON) {
						t.Errorf("Tool args = %s, want %s", string(gotJSON), string(wantJSON))
					}
				}
			} else if len(result.ToolCalls) > 0 {
				t.Errorf("Expected no tool calls but got %d", len(result.ToolCalls))
			}
		})
	}
}

func TestIsHarmonyFormat(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{
			name:  "Harmony format with channels",
			input: `<|channel|>analysis<|message|>Some content`,
			want:  true,
		},
		{
			name:  "Harmony format with end tag",
			input: `Some content<|end|>`,
			want:  true,
		},
		{
			name:  "Regular text",
			input: `This is just regular text without harmony tags`,
			want:  false,
		},
		{
			name:  "JSON content",
			input: `{"name": "tool", "arguments": {}}`,
			want:  false,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsHarmonyFormat(tt.input); got != tt.want {
				t.Errorf("IsHarmonyFormat() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestConvertToStandardResponse(t *testing.T) {
	tests := []struct {
		name     string
		harmony  *HarmonyResponse
		expected string
	}{
		{
			name: "Final content present",
			harmony: &HarmonyResponse{
				Final:    "This is the final response",
				Analysis: "This is analysis",
			},
			expected: "This is the final response",
		},
		{
			name: "Only analysis present",
			harmony: &HarmonyResponse{
				Final:    "",
				Analysis: "This is only analysis",
			},
			expected: "", // Analysis is internal reasoning, not returned as content
		},
		{
			name: "Empty response",
			harmony: &HarmonyResponse{
				Final:    "",
				Analysis: "",
			},
			expected: "",
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := ConvertToStandardResponse(tt.harmony)
			if result != tt.expected {
				t.Errorf("ConvertToStandardResponse() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestMultilineFinalContent(t *testing.T) {
	// Test that multiline markdown content in final channel is properly extracted
	input := `<|channel|>analysis<|message|>Processing the request...<|end|><|start|>assistant<|channel|>final<|message|>## Tunguska Incident Report

| Date | Location | Energy |
|------|----------|--------|
| June 30, 1908 | Siberia | 10-15 MT |

The event was caused by an **asteroid airburst** at 5-10km altitude.

### Key Evidence:
- No impact crater
- Tree-fall pattern
- Eyewitness accounts<|end|>`
	
	result, err := ParseHarmonyFormat(input)
	if err != nil {
		t.Fatalf("Failed to parse multiline content: %v", err)
	}
	
	// Check that we got the full multiline content
	if !strings.Contains(result.Final, "## Tunguska Incident Report") {
		t.Error("Missing markdown header")
	}
	if !strings.Contains(result.Final, "| June 30, 1908 | Siberia | 10-15 MT |") {
		t.Error("Missing table content")
	}
	if !strings.Contains(result.Final, "### Key Evidence:") {
		t.Error("Missing subheader")
	}
	if !strings.Contains(result.Final, "- No impact crater") {
		t.Error("Missing bullet points")
	}
}

func TestRealWorldExample(t *testing.T) {
	// Test with the actual example from the user
	input := `<|channel|>analysis<|message|>We need to perform Wikipedia search and Google search. Then synthesize info and provide a report on what really happened (the Tunguska event). We'll use wikipedia tool for snippet and google_search for results. Then combine.

Let's call wikipedia first.<|end|><|start|>assistant<|channel|>commentary to=functions.wikipedia <|constrain|>json<|message|>{"input":"Tunguska incident"}`
	
	result, err := ParseHarmonyFormat(input)
	if err != nil {
		t.Fatalf("Failed to parse real-world example: %v", err)
	}
	
	// Check that we extracted the analysis
	if !strings.Contains(result.Analysis, "Wikipedia search and Google search") {
		t.Errorf("Analysis doesn't contain expected content: %q", result.Analysis)
	}
	
	// Check that we extracted the tool call
	if len(result.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
	}
	
	if result.ToolCalls[0].Function.Name != "wikipedia" {
		t.Errorf("Expected wikipedia tool, got %q", result.ToolCalls[0].Function.Name)
	}
	
	// Check the arguments - now should be wrapped in "input" field
	var args map[string]interface{}
	err = json.Unmarshal(result.ToolCalls[0].Function.Arguments, &args)
	if err != nil {
		t.Fatalf("Failed to unmarshal tool arguments: %v", err)
	}
	
	// The arguments should now be wrapped: {"input": "{\"input\":\"Tunguska incident\"}"}
	if inputStr, ok := args["input"].(string); !ok {
		t.Errorf("Expected 'input' field to be a string, got %T", args["input"])
	} else if inputStr != `{"input":"Tunguska incident"}` {
		t.Errorf("Expected input JSON string '{\"input\":\"Tunguska incident\"}', got %q", inputStr)
	}
}

func TestFileWriteToolWrapper(t *testing.T) {
	// Test file_write tool gets properly wrapped
	input := `<|channel|>commentary to=functions.file_write <|constrain|>json<|message|>{"path":"test.txt","content":"hello world"}`
	
	result, err := ParseHarmonyFormat(input)
	if err != nil {
		t.Fatalf("Failed to parse file_write example: %v", err)
	}
	
	// Check that we have a tool call
	if len(result.ToolCalls) != 1 {
		t.Fatalf("Expected 1 tool call, got %d", len(result.ToolCalls))
	}
	
	if result.ToolCalls[0].Function.Name != "file_write" {
		t.Errorf("Expected file_write tool, got %q", result.ToolCalls[0].Function.Name)
	}
	
	// Check the arguments are wrapped in "input" field
	var args map[string]interface{}
	err = json.Unmarshal(result.ToolCalls[0].Function.Arguments, &args)
	if err != nil {
		t.Fatalf("Failed to unmarshal tool arguments: %v", err)
	}
	
	// The arguments should be wrapped: {"input": "{\"path\":\"test.txt\",\"content\":\"hello world\"}"}
	if inputStr, ok := args["input"].(string); !ok {
		t.Errorf("Expected 'input' field to be a string, got %T", args["input"])
	} else if inputStr != `{"path":"test.txt","content":"hello world"}` {
		t.Errorf("Expected wrapped JSON string, got %q", inputStr)
	}
}