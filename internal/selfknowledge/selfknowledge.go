package selfknowledge

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Info describes where simple-agent-go documentation and source can be found.
type Info struct {
	RootDir    string
	ReadmePath string
	DocsDir    string
	KeyFiles   []string
}

// Discover locates simple-agent-go docs/source roots from cwd, env, and binary path.
func Discover(cwd string) Info {
	candidates := make([]string, 0, 4)

	if envRoot := strings.TrimSpace(os.Getenv("SIMPLE_AGENT_SOURCE_DIR")); envRoot != "" {
		candidates = append(candidates, envRoot)
	}

	if cwd == "" {
		if wd, err := os.Getwd(); err == nil {
			cwd = wd
		}
	}
	if cwd != "" {
		candidates = append(candidates, upwardCandidates(cwd)...)
	}

	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		candidates = append(candidates, exeDir, filepath.Dir(exeDir))
	}

	if _, thisFile, _, ok := runtime.Caller(0); ok {
		srcRoot := filepath.Clean(filepath.Join(filepath.Dir(thisFile), "..", ".."))
		candidates = append(candidates, srcRoot)
	}

	root := ""
	seen := make(map[string]struct{})
	for _, c := range candidates {
		c = filepath.Clean(c)
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		if isSimpleAgentRoot(c) {
			root = c
			break
		}
	}

	if root == "" && cwd != "" {
		root = filepath.Clean(cwd)
	}

	info := Info{RootDir: root}
	if root == "" {
		return info
	}

	readme := filepath.Join(root, "README.md")
	if existsFile(readme) {
		info.ReadmePath = readme
	}

	docsDir := filepath.Join(root, "docs")
	if existsDir(docsDir) {
		info.DocsDir = docsDir
	}

	keyCandidates := []string{
		"README.md",
		"docs/vision.md",
		"cmd/simple-agent/main.go",
		"agent/agent.go",
		"tui/bordered.go",
		"llm/lmstudio/client.go",
		"internal/toolinit/init.go",
	}
	for _, rel := range keyCandidates {
		p := filepath.Join(root, rel)
		if existsFile(p) {
			info.KeyFiles = append(info.KeyFiles, p)
		}
	}

	return info
}

// BuildPromptSection returns a compact instruction block for self-documentation.
func BuildPromptSection(info Info) string {
	if info.RootDir == "" {
		return ""
	}

	var b strings.Builder
	b.WriteString("Simple Agent self-documentation (read when user asks about simple-agent-go internals, models, tools, or architecture):\n")
	if info.ReadmePath != "" {
		b.WriteString(fmt.Sprintf("- README: %s\n", info.ReadmePath))
	}
	if info.DocsDir != "" {
		b.WriteString(fmt.Sprintf("- Docs directory: %s\n", info.DocsDir))
	}
	if len(info.KeyFiles) > 0 {
		b.WriteString("- Key source files:\n")
		for _, p := range info.KeyFiles {
			b.WriteString(fmt.Sprintf("  - %s\n", p))
		}
	}
	b.WriteString("- When answering self-referential questions, read docs/source first and cite concrete file paths.\n")
	return strings.TrimSpace(b.String())
}

func upwardCandidates(start string) []string {
	start = filepath.Clean(start)
	out := []string{}
	cur := start
	for {
		out = append(out, cur)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return out
}

func isSimpleAgentRoot(dir string) bool {
	return existsFile(filepath.Join(dir, "README.md")) &&
		existsFile(filepath.Join(dir, "cmd", "simple-agent", "main.go")) &&
		existsDir(filepath.Join(dir, "agent")) &&
		existsDir(filepath.Join(dir, "tools"))
}

func existsFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func existsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
