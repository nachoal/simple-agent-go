package main

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/creack/pty"
)

var ansiRe = regexp.MustCompile(`\x1b\[[0-9;?]*[ -/]*[@-~]`)
var oscRe = regexp.MustCompile(`\x1b\][^\x1b\x07]*(?:\x07|\x1b\\)`)
var sessionResumeRe = regexp.MustCompile(`To resume this session, run simple-agent --resume ([A-Za-z0-9_-]+)`)
var assistantNoneRe = regexp.MustCompile(`Assistant:\s+\(none\)`)
var assistantLastMessageRe = regexp.MustCompile(`Assistant:\s+what was my last user message\?`)
var assistantOKRe = regexp.MustCompile(`Assistant:\s+OK`)
var assistantResumeRe = regexp.MustCompile(`Assistant:\s+reply with ok only`)

func main() {
	binary := flag.String("binary", "", "Path to simple-agent binary")
	flag.Parse()

	if strings.TrimSpace(*binary) == "" {
		*binary = "./simple-agent"
	}
	absBinary, err := filepath.Abs(*binary)
	if err != nil {
		fail(fmt.Errorf("resolve binary path: %w", err))
	}
	*binary = absBinary

	home := mustTempDir()
	defer os.RemoveAll(home)
	launchDir := mustTempDir()
	defer os.RemoveAll(launchDir)
	continueDir := mustTempDir()
	defer os.RemoveAll(continueDir)
	resumeDir := mustTempDir()
	defer os.RemoveAll(resumeDir)

	sessionID, err := firstRun(*binary, home, launchDir)
	if err != nil {
		fail(err)
	}
	if err := continueRun(*binary, home, continueDir, launchDir); err != nil {
		fail(err)
	}
	if err := resumeRun(*binary, home, resumeDir, launchDir, sessionID); err != nil {
		fail(err)
	}

	fmt.Println("tui smoke passed")
}

func firstRun(binary, home, launchDir string) (string, error) {
	cmd := exec.Command(binary)
	cmd.Dir = launchDir
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"TERM=xterm-256color",
		"SIMPLE_AGENT_FAKE_LLM=slow-stream",
		"DEFAULT_PROVIDER=openai",
		"DEFAULT_MODEL=harness-fake-model",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return "", fmt.Errorf("start pty: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	reader := newBufferingReader(ptmx)
	if err := reader.waitFor("Tools:", 10*time.Second); err != nil {
		return "", err
	}

	if err := typeAndEnter(ptmx, "hello"); err != nil {
		return "", err
	}
	time.Sleep(350 * time.Millisecond)
	if err := typeAndEnter(ptmx, "/cancel"); err != nil {
		return "", err
	}
	if err := reader.waitFor("Cancellation requested.", 10*time.Second); err != nil {
		return "", err
	}

	if err := typeAndEnter(ptmx, "what was my last user message?"); err != nil {
		return "", err
	}
	if err := reader.waitForRegexp(assistantNoneRe, 10*time.Second); err != nil {
		return "", err
	}

	if _, err := ptmx.Write([]byte{3}); err != nil {
		return "", err
	}
	if err := waitProcess(cmd, 10*time.Second); err != nil {
		return "", err
	}
	if err := reader.waitFor("To resume this session, run simple-agent --resume", 10*time.Second); err != nil {
		return "", err
	}

	sessionID := extractSessionID(reader.text())
	if sessionID == "" {
		return "", fmt.Errorf("failed to extract session id from output:\n%s", reader.text())
	}

	return sessionID, nil
}

func continueRun(binary, home, continueDir, launchDir string) error {
	cmd := exec.Command(binary, "--continue")
	cmd.Dir = continueDir
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"TERM=xterm-256color",
		"SIMPLE_AGENT_FAKE_LLM=echo",
		"DEFAULT_PROVIDER=openai",
		"DEFAULT_MODEL=harness-fake-model",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return fmt.Errorf("start continue pty: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	reader := newBufferingReader(ptmx)
	if err := reader.waitFor("Continuing session", 10*time.Second); err != nil {
		return err
	}

	if err := typeAndEnter(ptmx, "what was my last user message?"); err != nil {
		return err
	}
	if err := reader.waitForRegexp(assistantLastMessageRe, 10*time.Second); err != nil {
		return err
	}

	if err := typeAndEnter(ptmx, "/system"); err != nil {
		return err
	}
	if err := reader.waitFor(launchDir, 10*time.Second); err != nil {
		return err
	}
	time.Sleep(250 * time.Millisecond)

	if err := typeAndEnter(ptmx, "reply with ok only"); err != nil {
		return err
	}
	if err := reader.waitForRegexp(assistantOKRe, 10*time.Second); err != nil {
		return err
	}

	if _, err := ptmx.Write([]byte{3}); err != nil {
		return err
	}
	return waitProcess(cmd, 10*time.Second)
}

func resumeRun(binary, home, resumeDir, launchDir, sessionID string) error {
	cmd := exec.Command(binary, "--resume", sessionID)
	cmd.Dir = resumeDir
	cmd.Env = append(os.Environ(),
		"HOME="+home,
		"USERPROFILE="+home,
		"TERM=xterm-256color",
		"SIMPLE_AGENT_FAKE_LLM=echo",
		"DEFAULT_PROVIDER=openai",
		"DEFAULT_MODEL=harness-fake-model",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 24, Cols: 80})
	if err != nil {
		return fmt.Errorf("start resume pty: %w", err)
	}
	defer func() { _ = ptmx.Close() }()

	reader := newBufferingReader(ptmx)
	if err := reader.waitFor("Resuming session", 10*time.Second); err != nil {
		return err
	}
	if err := reader.waitFor(launchDir, 10*time.Second); err != nil {
		return err
	}

	if err := typeAndEnter(ptmx, "what was my last user message?"); err != nil {
		return err
	}
	if err := reader.waitForRegexp(assistantResumeRe, 10*time.Second); err != nil {
		return err
	}

	if _, err := ptmx.Write([]byte{3}); err != nil {
		return err
	}
	return waitProcess(cmd, 10*time.Second)
}

type bufferingReader struct {
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func newBufferingReader(r io.Reader) *bufferingReader {
	br := &bufferingReader{done: make(chan struct{})}
	go func() {
		defer close(br.done)
		_, _ = io.Copy(br, r)
	}()
	return br
}

func (b *bufferingReader) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *bufferingReader) waitFor(substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text := b.text()
		if strings.Contains(text, substr) {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %q in output:\n%s", substr, b.text())
}

func (b *bufferingReader) waitForRegexpAfter(pattern *regexp.Regexp, after string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text := b.text()
		if len(text) >= len(after) {
			if pattern.FindStringIndex(text[len(after):]) != nil {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %q after prompt in output:\n%s", pattern.String(), b.text())
}

func (b *bufferingReader) waitForRegexp(pattern *regexp.Regexp, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		text := b.text()
		if pattern.FindStringIndex(text) != nil {
			return nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for %q in output:\n%s", pattern.String(), b.text())
}

func (b *bufferingReader) text() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	text := b.buf.String()
	text = oscRe.ReplaceAllString(text, "")
	text = ansiRe.ReplaceAllString(text, "")
	return text
}

func mustTempDir() string {
	dir, err := os.MkdirTemp("", "simple-agent-tui-smoke-*")
	if err != nil {
		fail(err)
	}
	return dir
}

func waitProcess(cmd *exec.Cmd, timeout time.Duration) error {
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(timeout):
		_ = cmd.Process.Signal(syscall.SIGKILL)
		return fmt.Errorf("timed out waiting for process exit")
	}
}

func typeAndEnter(w io.Writer, text string) error {
	if _, err := w.Write([]byte(text)); err != nil {
		return err
	}
	time.Sleep(120 * time.Millisecond)
	_, err := w.Write([]byte{'\r'})
	return err
}

func extractSessionID(output string) string {
	matches := sessionResumeRe.FindStringSubmatch(output)
	if len(matches) != 2 {
		return ""
	}
	return matches[1]
}

func fail(err error) {
	fmt.Fprintf(os.Stderr, "Error: %v\n", err)
	os.Exit(1)
}
