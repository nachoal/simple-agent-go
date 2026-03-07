package runlog

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/nachoal/simple-agent-go/internal/userpaths"
)

type Logger struct {
	path string
	file *os.File
	mu   sync.Mutex
}

type loggerContextKey struct{}
type metaContextKey struct{}

type Metadata struct {
	RunID     string
	Mode      string
	Prompt    string
	Provider  string
	Model     string
	SessionID string
	TracePath string
}

func New(repoRoot, prefix string) (*Logger, error) {
	harnessDir, err := userpaths.HarnessDir(repoRoot)
	if err != nil {
		return nil, err
	}

	runDir := filepath.Join(harnessDir, "runs", time.Now().Format("20060102"))
	if err := os.MkdirAll(runDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create run log directory %q: %w", runDir, err)
	}

	if prefix == "" {
		prefix = "run"
	}
	path := filepath.Join(runDir, fmt.Sprintf("%s_%s_%d.jsonl", prefix, time.Now().Format("150405"), os.Getpid()))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open run log %q: %w", path, err)
	}

	return &Logger{path: path, file: file}, nil
}

func (l *Logger) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

func (l *Logger) Close() error {
	if l == nil || l.file == nil {
		return nil
	}

	l.mu.Lock()
	defer l.mu.Unlock()

	err := l.file.Close()
	l.file = nil
	return err
}

func (l *Logger) Event(kind string, fields map[string]interface{}) {
	if l == nil || l.file == nil {
		return
	}

	record := map[string]interface{}{
		"timestamp": time.Now().Format(time.RFC3339Nano),
		"kind":      kind,
	}
	for k, v := range fields {
		record[k] = v
	}

	data, err := json.Marshal(record)
	if err != nil {
		return
	}

	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	_, _ = l.file.Write(append(data, '\n'))
}

func WithContext(ctx context.Context, logger *Logger) context.Context {
	if logger == nil {
		return ctx
	}
	return context.WithValue(ctx, loggerContextKey{}, logger)
}

func WithMetadata(ctx context.Context, meta Metadata) context.Context {
	return context.WithValue(ctx, metaContextKey{}, meta)
}

func FromContext(ctx context.Context) *Logger {
	if ctx == nil {
		return nil
	}
	logger, _ := ctx.Value(loggerContextKey{}).(*Logger)
	return logger
}

func MetadataFromContext(ctx context.Context) (Metadata, bool) {
	if ctx == nil {
		return Metadata{}, false
	}
	meta, ok := ctx.Value(metaContextKey{}).(Metadata)
	return meta, ok
}

func EventFromContext(ctx context.Context, kind string, fields map[string]interface{}) {
	logger := FromContext(ctx)
	if logger == nil {
		return
	}
	if fields == nil {
		fields = map[string]interface{}{}
	}
	if meta, ok := MetadataFromContext(ctx); ok {
		if meta.RunID != "" {
			fields["run_id"] = meta.RunID
		}
		if meta.Mode != "" {
			fields["mode"] = meta.Mode
		}
		if meta.Provider != "" {
			fields["provider"] = meta.Provider
		}
		if meta.Model != "" {
			fields["model"] = meta.Model
		}
		if meta.SessionID != "" {
			fields["session_id"] = meta.SessionID
		}
		if meta.TracePath != "" {
			fields["trace_path"] = meta.TracePath
		}
		if meta.Prompt != "" {
			fields["prompt_excerpt"] = truncate(meta.Prompt, 512)
		}
	}
	logger.Event(kind, fields)
}

func truncate(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	if max <= 3 {
		return s[:max]
	}
	return s[:max-3] + "..."
}
