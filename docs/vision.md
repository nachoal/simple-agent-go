Vision (Multimodal) Support

Overview
- Providers: Ollama and LM Studio
- Input: Text + images (local file paths)
- Output: Provider returns text as usual

Client helpers
- `llm.MultimodalClient` (optional interface): implemented by Ollama and LM Studio clients.
- Methods:
  - `ChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (string, error)`
  - `StreamChatWithImages(prompt string, imagePaths []string, opts map[string]interface{}) (<-chan string, error)`

Ollama
- Endpoint: `POST /api/chat`
- Images: Base64 strings via message `images` field
- Common vision models: `llava`, `llava:7b-v1.6`, `llava:13b-v1.6`, `llava:34b-v1.6`, `bakllava`, `moondream`
- Usage example:

```go
import (
    "context"
    "fmt"
    "github.com/nachoal/simple-agent-go/llm/ollama"
)

func example() {
    client, _ := ollama.NewClient()
    // Assert multimodal helpers
    if mm, ok := any(client).(interface{ ChatWithImages(string, []string, map[string]interface{}) (string, error) }); ok {
        out, err := mm.ChatWithImages("What is in this image?", []string{"./image.jpg"}, map[string]interface{}{"temperature": 0.7})
        fmt.Println(out, err)
    }
}
```

LM Studio
- Endpoint: `POST /v1/chat/completions` (OpenAI-compatible)
- Images: Data URL (`data:image/<type>;base64,<...>`) via `content` array
- Common vision models: `gemma-3-*`, `pixtral`, `llava*`, `bakllava`, `moondream`
- Usage example:

```go
import (
    "fmt"
    "github.com/nachoal/simple-agent-go/llm/lmstudio"
)

func example() {
    client, _ := lmstudio.NewClient()
    if mm, ok := any(client).(interface{ ChatWithImages(string, []string, map[string]interface{}) (string, error) }); ok {
        out, err := mm.ChatWithImages("Describe the image", []string{"./image.png"}, map[string]interface{}{"max_tokens": 300})
        fmt.Println(out, err)
    }
}
```

Model listing
- The model selector shows a 👁️ indicator for vision-capable models for Ollama and LM Studio.
- Detection is based on common model name patterns. You can still select any model.

Notes
- Ensure providers are running locally:
  - Ollama: `ollama serve` and `ollama pull llava`
  - LM Studio: Start server in Developer tab, load a vision model
- Configure provider URLs via `OLLAMA_URL` and `LM_STUDIO_URL` if needed.

