package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/nachoal/simple-agent-go/internal/codexreport"
)

func main() {
	repo := flag.String("repo", "", "Repository root to analyze (default: current working directory)")
	codexHome := flag.String("codex-home", "", "Codex home directory (default: $CODEX_HOME or ~/.codex)")
	outDir := flag.String("out-dir", "", "Directory to write generated reports (default: ~/.simple-agent/harness/<repo-slug>/codex-analysis)")
	flag.Parse()

	result, err := codexreport.Run(codexreport.Options{
		RepoRoot:  *repo,
		CodexHome: *codexHome,
		OutDir:    *outDir,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Analyzed %d Codex session files\n", result.ScannedFiles)
	if result.SkippedFiles > 0 {
		fmt.Printf("Skipped %d malformed or unsupported session files\n", result.SkippedFiles)
	}
	fmt.Printf("Included %d relevant sessions (%d primary, %d secondary); excluded %d\n",
		result.IncludedSessions,
		result.PrimarySessionCount,
		result.SecondaryCount,
		result.ExcludedSessions,
	)

	if len(result.ThemeCounts) > 0 {
		fmt.Println("\nTop themes:")
		for i, count := range result.ThemeCounts {
			fmt.Printf("- %s: %d sessions\n", count.Name, count.Sessions)
			if i == 4 {
				break
			}
		}
	}

	fmt.Println("\nWrote:")
	for _, path := range result.GeneratedFiles {
		fmt.Printf("- %s\n", path)
	}
}
