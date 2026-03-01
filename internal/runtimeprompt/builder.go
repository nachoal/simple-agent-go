package runtimeprompt

import (
	"fmt"
	"strings"

	"github.com/nachoal/simple-agent-go/internal/resources"
	"github.com/nachoal/simple-agent-go/internal/selfknowledge"
)

const maxEmbeddedCharsPerFile = 12000

// Build creates an enriched system prompt from base prompt + runtime resources.
func Build(basePrompt string, info selfknowledge.Info, snapshot resources.Snapshot) string {
	basePrompt = strings.TrimSpace(basePrompt)
	var b strings.Builder
	b.WriteString(basePrompt)

	if section := strings.TrimSpace(selfknowledge.BuildPromptSection(info)); section != "" {
		b.WriteString("\n\n")
		b.WriteString(section)
	}

	if len(snapshot.ContextFiles) > 0 {
		b.WriteString("\n\nProject context files (follow these instructions):\n")
		for _, f := range snapshot.ContextFiles {
			b.WriteString(fmt.Sprintf("\n## %s\n\n", f.Path))
			b.WriteString(truncateForPrompt(f.Content, maxEmbeddedCharsPerFile))
			b.WriteString("\n")
		}
	}

	if len(snapshot.PromptFragments) > 0 {
		b.WriteString("\nAdditional prompt fragments:\n")
		for _, f := range snapshot.PromptFragments {
			b.WriteString(fmt.Sprintf("\n## %s\n\n", f.Path))
			b.WriteString(truncateForPrompt(f.Content, maxEmbeddedCharsPerFile))
			b.WriteString("\n")
		}
	}

	if len(snapshot.Diagnostics) > 0 {
		b.WriteString("\nResource diagnostics:\n")
		for _, d := range snapshot.Diagnostics {
			b.WriteString("- ")
			b.WriteString(d)
			b.WriteString("\n")
		}
	}

	return b.String()
}

func truncateForPrompt(content string, maxChars int) string {
	if maxChars <= 0 {
		return ""
	}
	if len(content) <= maxChars {
		return content
	}
	trimmed := strings.TrimSpace(content[:maxChars])
	return trimmed + "\n\n[Truncated for prompt safety.]"
}
