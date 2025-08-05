# GPT-OSS Harmony Format Issue & Solution

## The Problem

GPT-OSS models output responses in OpenAI's Harmony format, which contains channel tags that break llama.cpp's chat parser:

```
<|channel|>analysis<|message|>Thinking about the problem...<|start|>assistant<|channel|>final<|message|>Here's the answer
```

When llama.cpp encounters `<|start|>assistant` within a response, it crashes with:
```
libc++abi: terminating due to uncaught exception of type std::runtime_error: Unexpected content at end of input
```

## Current Status

1. **llama.cpp PR #15091** adds GPT-OSS support but has known template parsing issues
2. **Community workarounds** (no `--jinja` flag) produce raw Harmony output, unusable for chat UIs
3. **"Fixed" templates** only fix input formatting, not output parsing
4. **Server crashes** in TUI mode when GPT-OSS outputs Harmony format

## Proposed Solution: Harmony Parser in simple-agent

Add a Harmony format parser to simple-agent-go that:

1. **Detects GPT-OSS models** (when provider is "llamacpp" and model contains "gpt-oss")
2. **Intercepts responses** before standard parsing
3. **Extracts channels**:
   - `<|channel|>analysis<|message|>` → Reasoning/thinking content
   - `<|channel|>final<|message|>` → User-visible response
   - `<|channel|>commentary<|message|>` → Tool calls
4. **Converts to standard format** that simple-agent expects

## Implementation Steps

### 1. Create Harmony Parser (`llm/llamacpp/harmony.go`)
```go
package llamacpp

import (
    "regexp"
    "strings"
)

type HarmonyResponse struct {
    Analysis   string
    Final      string
    Commentary string
    ToolCalls  []ToolCall
}

func ParseHarmonyFormat(content string) (*HarmonyResponse, error) {
    // Extract channel contents with regex
    analysisRe := regexp.MustCompile(`<\|channel\|>analysis<\|message\|>(.*?)(?:<\|start\|>|<\|end\|>|$)`)
    finalRe := regexp.MustCompile(`<\|channel\|>final<\|message\|>(.*?)(?:<\|return\|>|<\|end\|>|$)`)
    commentaryRe := regexp.MustCompile(`<\|channel\|>commentary.*?<\|message\|>(.*?)(?:<\|call\|>|<\|end\|>|$)`)
    
    // Parse and return structured response
}

func ConvertToStandardResponse(harmony *HarmonyResponse) string {
    // Return clean final content for display
    return harmony.Final
}
```

### 2. Modify llamacpp Client (`llm/llamacpp/client.go`)

In the `Chat` method, add Harmony detection:
```go
func (c *Client) Chat(ctx context.Context, request *llm.ChatRequest) (*llm.ChatResponse, error) {
    // ... existing code ...
    
    // After getting response
    if strings.Contains(c.model, "gpt-oss") && strings.Contains(chatResp.Choices[0].Message.Content, "<|channel|>") {
        harmony, err := ParseHarmonyFormat(chatResp.Choices[0].Message.Content)
        if err == nil {
            // Convert Harmony format to standard response
            chatResp.Choices[0].Message.Content = harmony.Final
            // Store reasoning if field exists
            // Convert commentary to tool calls if present
        }
    }
    
    return &chatResp, nil
}
```

### 3. Enable Tools for GPT-OSS

Once Harmony parsing works, re-enable tools in `cmd/simple-agent/main.go`:
```go
// Remove the tool disable for GPT-OSS
// The Harmony parser will handle tool calls from commentary channel
```

## Testing

1. Start server without `--jinja` to get raw Harmony output:
```bash
~/bin/llama-cpp-latest/llama-server \
    --model ~/.cache/lm-studio/models/ggml-org/gpt-oss-120b-GGUF/gpt-oss-120b-mxfp4-00001-of-00003.gguf \
    --host 0.0.0.0 --port 8080 -c 8192 -fa
```

2. Test with simple-agent:
```bash
./simple-agent --provider llamacpp --model gpt-oss-120b
```

## Files to Modify

1. `/Users/ia/code/projects/simple-agents/simple-agent-go/llm/llamacpp/harmony.go` (NEW)
2. `/Users/ia/code/projects/simple-agents/simple-agent-go/llm/llamacpp/client.go` (MODIFY Chat/ChatStream)
3. `/Users/ia/code/projects/simple-agents/simple-agent-go/cmd/simple-agent/main.go` (RE-ENABLE tools)

## Expected Outcome

- GPT-OSS works in TUI mode without crashes
- Clean responses without visible Harmony tags
- Tool calling support through commentary channel parsing
- Reasoning available in separate field

## Alternative Approaches

1. **Wait for llama.cpp fix** - PR #15091 discussion suggests ongoing work
2. **Use different model** - Other models work fine with current setup
3. **Post-process in TUI** - Less clean but isolates changes to UI layer

## References

- llama.cpp PR #15091: https://github.com/ggml-org/llama.cpp/pull/15091
- OpenAI Harmony: https://github.com/openai/harmony
- Fixed template attempt: https://huggingface.co/DevQuasar/openai.gpt-oss-20b-GGUF/resolve/main/gpt-oss_fixed.jinja