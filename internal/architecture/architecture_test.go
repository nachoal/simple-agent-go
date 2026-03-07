package architecture

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

const modulePath = "github.com/nachoal/simple-agent-go"

type importRule struct {
	scope           string
	forbiddenPrefix string
	explanation     string
}

func TestImportBoundaries(t *testing.T) {
	root := repoRoot(t)
	rules := []importRule{
		{scope: modulePath + "/agent", forbiddenPrefix: modulePath + "/tui", explanation: "agent core must not depend on the TUI"},
		{scope: modulePath + "/history", forbiddenPrefix: modulePath + "/tui", explanation: "history must stay UI-agnostic"},
		{scope: modulePath + "/llm", forbiddenPrefix: modulePath + "/tui", explanation: "provider adapters must stay UI-agnostic"},
		{scope: modulePath + "/tools", forbiddenPrefix: modulePath + "/tui", explanation: "tools must stay UI-agnostic"},
		{scope: modulePath + "/config", forbiddenPrefix: modulePath + "/tui", explanation: "config must stay UI-agnostic"},
		{scope: modulePath + "/agent", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "runtime agent code must not depend on private maintainer analysis"},
		{scope: modulePath + "/history", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "history must not depend on private maintainer analysis"},
		{scope: modulePath + "/llm", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "provider adapters must not depend on private maintainer analysis"},
		{scope: modulePath + "/tools", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "tools must not depend on private maintainer analysis"},
		{scope: modulePath + "/tui", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "product UI must not depend on private maintainer analysis"},
		{scope: modulePath + "/cmd/simple-agent", forbiddenPrefix: modulePath + "/internal/codexreport", explanation: "product CLI must not import private maintainer analysis directly"},
	}

	fset := token.NewFileSet()
	var violations []string

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case ".git", "vendor", "analysis":
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".go" || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if parseErr != nil {
			return parseErr
		}

		dir := filepath.Dir(path)
		rel, relErr := filepath.Rel(root, dir)
		if relErr != nil {
			return relErr
		}
		pkgPath := modulePath
		if rel != "." {
			pkgPath += "/" + filepath.ToSlash(rel)
		}

		for _, spec := range file.Imports {
			importPath, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				return unquoteErr
			}
			for _, rule := range rules {
				if strings.HasPrefix(pkgPath, rule.scope) && strings.HasPrefix(importPath, rule.forbiddenPrefix) {
					violations = append(violations, pkgPath+" imports "+importPath+": "+rule.explanation)
				}
			}
		}

		return nil
	})
	if err != nil {
		t.Fatalf("failed to inspect imports: %v", err)
	}

	if len(violations) > 0 {
		t.Fatalf("architecture violations:\n- %s", strings.Join(violations, "\n- "))
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("failed to resolve caller")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
