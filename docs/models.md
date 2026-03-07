# Custom Models (`models.json`)

Custom model/provider config is loaded from:

`~/.simple-agent/agent/models.json`

## Supported shape

```json
{
  "providers": {
    "lmstudio": {
      "baseUrl": "http://localhost:1234/v1",
      "api": "openai-completions",
      "apiKey": "lmstudio",
      "models": [
        {
          "id": "qwen/qwen3.5-35b-a3b",
          "name": "Qwen 3.5 35B A3B",
          "input": ["text", "image"],
          "contextWindow": 262144,
          "maxTokens": 16384
        }
      ]
    }
  }
}
```

## Behavior

- Providers in `models.json` are available to client creation and model selector.
- For providers that expose `/v1/models`, live API models are merged with static models.
- Static models are upserted by ID and can enrich metadata (description, vision support, max tokens).
- Providers without an API key use the LM Studio/OpenAI-compatible transport, so you can add named aliases for remote local-model servers.
- Config values support:
  - shell command (`"!command"`)
  - environment variable name (if env var exists)
  - literal fallback

## Example: remote 5090 endpoint

```json
{
  "providers": {
    "ialab": {
      "baseUrl": "http://100.121.50.123:8000/v1",
      "api": "openai-completions",
      "models": [
        {
          "id": "qwen3.5-27b",
          "name": "ialab Qwen3.5-27B Dense",
          "maxTokens": 16384
        },
        {
          "id": "qwopus-27b",
          "name": "ialab Qwopus-27B",
          "maxTokens": 16384
        }
      ]
    }
  }
}
```

Then run:

```bash
simple-agent --provider ialab --model qwen3.5-27b
```

## Code

- Registry: `internal/models/registry.go`
- Main wiring: `cmd/simple-agent/main.go`
- Selector merge: `tui/model_selector.go`
