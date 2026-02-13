package tui

import (
	"strings"
	"testing"
)

func TestSplitThinkingTrace(t *testing.T) {
	content := "<think>\ninternal reasoning\n</think>\n\nFinal answer."
	trace, final := splitThinkingTrace(content)

	if trace != "internal reasoning" {
		t.Fatalf("unexpected trace: %q", trace)
	}
	if final != "Final answer." {
		t.Fatalf("unexpected final content: %q", final)
	}
}

func TestRenderAssistantMessageWithThinkingTrace(t *testing.T) {
	content := "<think>plan</think>\nDone."
	rendered := renderAssistantMessage(nil, content)

	if !strings.Contains(rendered, "<thinking traces>") {
		t.Fatalf("expected thinking trace start tag, got: %q", rendered)
	}
	if !strings.Contains(rendered, "</thinking traces>") {
		t.Fatalf("expected thinking trace end tag, got: %q", rendered)
	}
	if !strings.Contains(rendered, "plan") {
		t.Fatalf("expected thinking content in output, got: %q", rendered)
	}
	if !strings.Contains(rendered, "Done.") {
		t.Fatalf("expected final content in output, got: %q", rendered)
	}
}

func TestWrapThinkingTraceWrapsLongLine(t *testing.T) {
	longLine := strings.Repeat("word ", 30)
	wrapped := wrapThinkingTrace(strings.TrimSpace(longLine))

	if !strings.Contains(wrapped, "\n") {
		t.Fatalf("expected wrapped output to contain a newline, got: %q", wrapped)
	}
}

func TestFormatArgumentsTruncatesLongValues(t *testing.T) {
	m := &BorderedTUI{}
	args := map[string]interface{}{
		"raw": strings.Repeat("x", 400),
	}

	got := m.formatArguments(args)
	if !strings.Contains(got, "…") {
		t.Fatalf("expected truncated ellipsis in args output, got: %q", got)
	}
}
