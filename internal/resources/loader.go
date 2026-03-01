package resources

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"github.com/nachoal/simple-agent-go/internal/userpaths"
)

var contextFileCandidates = []string{"AGENTS.md", "CLAUDE.md"}

// LoadedFile represents a loaded prompt/context file.
type LoadedFile struct {
	Path    string
	Content string
}

// Snapshot is a point-in-time view of loaded runtime resources.
type Snapshot struct {
	ContextFiles    []LoadedFile
	PromptFragments []LoadedFile
	Diagnostics     []string
}

// Loader discovers and reloads runtime resources used to build system prompts.
type Loader struct {
	cwd      string
	agentDir string

	mu       sync.RWMutex
	snapshot Snapshot
}

// NewLoader creates a resource loader for a working directory.
func NewLoader(cwd, agentDir string) (*Loader, error) {
	if cwd == "" {
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("failed to resolve cwd: %w", err)
		}
	}
	cwd = filepath.Clean(cwd)

	if agentDir == "" {
		var err error
		agentDir, err = userpaths.AgentDir()
		if err != nil {
			return nil, err
		}
	}
	agentDir = filepath.Clean(agentDir)

	l := &Loader{
		cwd:      cwd,
		agentDir: agentDir,
	}
	l.Reload()
	return l, nil
}

// Cwd returns the loader working directory.
func (l *Loader) Cwd() string {
	return l.cwd
}

// AgentDir returns the loader agent configuration directory.
func (l *Loader) AgentDir() string {
	return l.agentDir
}

// Snapshot returns the latest loaded resources.
func (l *Loader) Snapshot() Snapshot {
	l.mu.RLock()
	defer l.mu.RUnlock()

	return cloneSnapshot(l.snapshot)
}

// Reload reloads all resources from disk and returns the fresh snapshot.
func (l *Loader) Reload() Snapshot {
	loaded := Snapshot{
		ContextFiles:    []LoadedFile{},
		PromptFragments: []LoadedFile{},
		Diagnostics:     []string{},
	}

	seen := make(map[string]struct{})

	if f, ok, diag := loadFirstContextFile(l.agentDir); diag != "" {
		loaded.Diagnostics = append(loaded.Diagnostics, diag)
	} else if ok {
		loaded.ContextFiles = append(loaded.ContextFiles, f)
		seen[f.Path] = struct{}{}
	}

	for _, dir := range ancestorDirs(l.cwd) {
		f, ok, diag := loadFirstContextFile(dir)
		if diag != "" {
			loaded.Diagnostics = append(loaded.Diagnostics, diag)
			continue
		}
		if !ok {
			continue
		}
		if _, exists := seen[f.Path]; exists {
			continue
		}
		loaded.ContextFiles = append(loaded.ContextFiles, f)
		seen[f.Path] = struct{}{}
	}

	promptDirs := []string{
		filepath.Join(l.agentDir, "prompts"),
		filepath.Join(l.cwd, ".simple-agent", "prompts"),
	}
	for _, dir := range promptDirs {
		files, diag := loadMarkdownFiles(dir)
		if diag != "" {
			loaded.Diagnostics = append(loaded.Diagnostics, diag)
		}
		loaded.PromptFragments = append(loaded.PromptFragments, files...)
	}

	l.mu.Lock()
	l.snapshot = loaded
	l.mu.Unlock()

	return cloneSnapshot(loaded)
}

func cloneSnapshot(in Snapshot) Snapshot {
	out := Snapshot{
		ContextFiles:    make([]LoadedFile, len(in.ContextFiles)),
		PromptFragments: make([]LoadedFile, len(in.PromptFragments)),
		Diagnostics:     make([]string, len(in.Diagnostics)),
	}
	copy(out.ContextFiles, in.ContextFiles)
	copy(out.PromptFragments, in.PromptFragments)
	copy(out.Diagnostics, in.Diagnostics)
	return out
}

func loadFirstContextFile(dir string) (LoadedFile, bool, string) {
	for _, name := range contextFileCandidates {
		p := filepath.Join(dir, name)
		info, err := os.Stat(p)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return LoadedFile{}, false, fmt.Sprintf("warning: failed to stat %q: %v", p, err)
		}
		if info.IsDir() {
			continue
		}

		data, err := os.ReadFile(p)
		if err != nil {
			return LoadedFile{}, false, fmt.Sprintf("warning: failed to read %q: %v", p, err)
		}
		return LoadedFile{Path: p, Content: string(data)}, true, ""
	}
	return LoadedFile{}, false, ""
}

func loadMarkdownFiles(dir string) ([]LoadedFile, string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ""
		}
		return nil, fmt.Sprintf("warning: failed to read prompt directory %q: %v", dir, err)
	}

	files := make([]LoadedFile, 0, len(entries))
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := filepath.Ext(e.Name())
		if ext != ".md" && ext != ".txt" {
			continue
		}
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		p := filepath.Join(dir, name)
		data, err := os.ReadFile(p)
		if err != nil {
			return files, fmt.Sprintf("warning: failed to read prompt fragment %q: %v", p, err)
		}
		files = append(files, LoadedFile{Path: p, Content: string(data)})
	}
	return files, ""
}

func ancestorDirs(start string) []string {
	start = filepath.Clean(start)
	dirs := []string{}
	seen := make(map[string]struct{})

	current := start
	for {
		if _, ok := seen[current]; ok {
			break
		}
		seen[current] = struct{}{}
		dirs = append(dirs, current)

		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}

	// root -> leaf ordering makes constraints read naturally.
	for i, j := 0, len(dirs)-1; i < j; i, j = i+1, j-1 {
		dirs[i], dirs[j] = dirs[j], dirs[i]
	}
	return dirs
}
