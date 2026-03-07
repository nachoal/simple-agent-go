package codexreport

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/nachoal/simple-agent-go/internal/userpaths"
)

type Options struct {
	RepoRoot  string
	CodexHome string
	OutDir    string
}

type Result struct {
	GeneratedAt         time.Time        `json:"generated_at"`
	RepoRoot            string           `json:"repo_root"`
	CodexHome           string           `json:"codex_home"`
	ScannedFiles        int              `json:"scanned_files"`
	SkippedFiles        int              `json:"skipped_files"`
	ParsedSessions      int              `json:"parsed_sessions"`
	IncludedSessions    int              `json:"included_sessions"`
	ExcludedSessions    int              `json:"excluded_sessions"`
	IncludedByClass     []Count          `json:"included_by_class"`
	ThemeCounts         []Count          `json:"theme_counts"`
	PetitionCounts      []Count          `json:"petition_counts"`
	FailureCounts       []Count          `json:"failure_counts"`
	TopTools            []Count          `json:"top_tools"`
	LoopIncidents       []LoopIncident   `json:"loop_incidents"`
	Sessions            []SessionSummary `json:"sessions"`
	Excluded            []SessionSummary `json:"excluded"`
	Recommendations     []Recommendation `json:"recommendations"`
	GeneratedFiles      []string         `json:"generated_files"`
	PrimarySessionCount int              `json:"primary_session_count"`
	SecondaryCount      int              `json:"secondary_session_count"`
}

type SessionSummary struct {
	ID                 string    `json:"id"`
	File               string    `json:"file"`
	StartedAt          time.Time `json:"started_at,omitempty"`
	LastEventAt        time.Time `json:"last_event_at,omitempty"`
	Classification     string    `json:"classification"`
	FirstUserMessage   string    `json:"first_user_message"`
	UserTurns          int       `json:"user_turns"`
	AgentMessages      int       `json:"agent_messages"`
	TaskStarted        int       `json:"task_started"`
	TaskCompleted      int       `json:"task_completed"`
	Themes             []string  `json:"themes"`
	RepeatedPetitions  []string  `json:"repeated_petitions"`
	FailureSignals     []string  `json:"failure_signals"`
	LoopIncidentCount  int       `json:"loop_incident_count"`
	RelevanceReasons   []string  `json:"relevance_reasons"`
	OtherProjectPaths  []string  `json:"other_project_paths,omitempty"`
	ToolNames          []string  `json:"tool_names,omitempty"`
	RepresentativeUser []string  `json:"representative_user,omitempty"`
}

type Count struct {
	Name     string `json:"name"`
	Sessions int    `json:"sessions"`
	Mentions int    `json:"mentions"`
}

type LoopIncident struct {
	SessionID        string `json:"session_id"`
	SessionFile      string `json:"session_file"`
	AgentClaim       string `json:"agent_claim"`
	UserCorrection   string `json:"user_correction"`
	Classification   string `json:"classification"`
	PreviousClaimIdx int    `json:"previous_claim_idx"`
}

type Recommendation struct {
	Title    string `json:"title"`
	Why      string `json:"why"`
	Proposal string `json:"proposal"`
}

type lineEnvelope struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type sessionMetaPayload struct {
	ID        string    `json:"id"`
	Timestamp string    `json:"timestamp"`
	Cwd       string    `json:"cwd"`
	Git       *gitState `json:"git"`
}

type turnContextPayload struct {
	TurnID string    `json:"turn_id"`
	Cwd    string    `json:"cwd"`
	Model  string    `json:"model"`
	Git    *gitState `json:"git"`
}

type gitState struct {
	RepositoryURL string `json:"repository_url"`
}

type eventMsgPayload struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type responseMessagePayload struct {
	Type    string            `json:"type"`
	Role    string            `json:"role"`
	Content []responseContent `json:"content"`
}

type responseContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type functionCallPayload struct {
	Type      string          `json:"type"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type conversationMessage struct {
	Role string
	Text string
}

type toolInvocation struct {
	Name    string
	Command string
	Workdir string
}

type sessionAccumulator struct {
	FilePath             string
	RelativeFile         string
	ID                   string
	StartedAt            time.Time
	LastEventAt          time.Time
	SessionCwd           string
	TurnCwds             map[string]struct{}
	GitRepoURL           string
	EventConversation    []conversationMessage
	ResponseConversation []conversationMessage
	ToolInvocations      []toolInvocation
	TaskStarted          int
	TaskCompleted        int
}

type matchDefinition struct {
	Name    string
	Phrases []string
}

var (
	otherProjectPattern = regexp.MustCompile(`/Users/[^/\s]+/code/(?:projects|tmp|experiments)/[^\s"'` + "`" + `]+`)
)

var themeDefinitions = []matchDefinition{
	{
		Name: "provider-reliability",
		Phrases: []string{
			"lmstudio", "provider", "model", "startup", "fallback",
			"tool call", "tool calls", "parser", "kimi", "moonshot",
		},
	},
	{
		Name: "observability-and-tracing",
		Phrases: []string{
			"verbose", "trace", "traces", "debug", "logs", "instrumentation",
			"history", "session", "session trasability", "full verbose logs",
		},
	},
	{
		Name: "autonomy-and-recovery",
		Phrases: []string{
			"continue", "cancel", "escape", "esc", "stuck", "hang", "hung",
			"loop", "panic", "retry", "timeout", "all times",
		},
	},
	{
		Name: "verification-pressure",
		Phrases: []string{
			"did you test", "validate", "smoke", "check again",
			"open a browser", "click around", "works all times", "verify",
		},
	},
	{
		Name: "commit-hygiene",
		Phrases: []string{
			"atomic conventional commit", "atomic conventional commits",
			"checkpoint", "commit and push", "worktree is clean",
		},
	},
	{
		Name: "self-knowledge-and-self-improve",
		Phrases: []string{
			"/improve", "read and analyae its own docs", "read and analyze its own docs",
			"update itself", "self-documentation", "self-improve",
		},
	},
}

var repeatedPetitionDefinitions = []matchDefinition{
	{
		Name:    "atomic-conventional-commits",
		Phrases: []string{"atomic conventional commit", "atomic conventional commits"},
	},
	{
		Name:    "verification-repeat",
		Phrases: []string{"did you test", "validate", "smoke", "all times", "check again"},
	},
	{
		Name:    "browser-level-validation",
		Phrases: []string{"open a browser", "click around", "use the app from the user perspective"},
	},
	{
		Name:    "logs-and-traces",
		Phrases: []string{"full verbose logs", "trace", "verbose", "instrumentation"},
	},
	{
		Name:    "continue-and-cancel-semantics",
		Phrases: []string{"continue", "esc", "cancel", "how does pi does the esc dance"},
	},
}

var failureDefinitions = []matchDefinition{
	{
		Name:    "hangs-and-timeouts",
		Phrases: []string{"hang", "hung", "timeout", "stuck"},
	},
	{
		Name:    "panic-crashes",
		Phrases: []string{"panic", "program experienced a panic", "maximum update depth exceeded"},
	},
	{
		Name:    "malformed-tool-calls",
		Phrases: []string{"not working", "tool call", "tool calls", "arg_value", "parser", "raw malformed"},
	},
	{
		Name:    "missing-observability",
		Phrases: []string{"full verbose logs", "instrumentation", "debug logs", "trace"},
	},
	{
		Name:    "premature-completion",
		Phrases: []string{"did you test", "check again", "still", "why did this fail"},
	},
}

var completionClaimPhrases = []string{
	"tests are clean",
	"repo checks are clean",
	"build succeeded",
	"smoke check",
	"worktree is clean",
	"created one atomic conventional commit",
	"committed the remaining changes",
	"the commit is in place",
	"verification now",
	"the repo checks are clean",
	"i ran the rebuilt binary",
	"what was tested",
}

var correctionPhrases = []string{
	"did you test",
	"still",
	"not working",
	"why did this fail",
	"panic",
	"hang",
	"hung",
	"stuck",
	"check again",
	"continue",
	"all times",
	"full verbose logs",
	"open a browser",
}

var boilerplatePrefixes = []string{
	"# agents.md instructions",
	"<environment_context>",
	"<instructions>",
}

func Run(opts Options) (Result, error) {
	normalized, err := normalizeOptions(opts)
	if err != nil {
		return Result{}, err
	}

	accumulators, scanned, skipped, err := loadSessions(normalized.CodexHome)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		GeneratedAt:  time.Now(),
		RepoRoot:     normalized.RepoRoot,
		CodexHome:    normalized.CodexHome,
		ScannedFiles: scanned,
		SkippedFiles: skipped,
	}

	themeSessions := make(map[string]int)
	themeMentions := make(map[string]int)
	petitionSessions := make(map[string]int)
	petitionMentions := make(map[string]int)
	failureSessions := make(map[string]int)
	failureMentions := make(map[string]int)
	toolSessions := make(map[string]int)
	toolMentions := make(map[string]int)
	classCounts := map[string]int{}

	for _, acc := range accumulators {
		summary, loops := summarizeSession(acc, normalized.RepoRoot)
		result.ParsedSessions++

		if summary.Classification == "primary" || summary.Classification == "secondary" {
			result.IncludedSessions++
			result.Sessions = append(result.Sessions, summary)
			result.LoopIncidents = append(result.LoopIncidents, loops...)
			classCounts[summary.Classification]++

			for _, name := range summary.Themes {
				themeSessions[name]++
			}
			for name, mentions := range matchDefinitions(joinEvidence(acc), themeDefinitions) {
				themeMentions[name] += mentions
			}

			for _, name := range summary.RepeatedPetitions {
				petitionSessions[name]++
			}
			for name, mentions := range matchDefinitions(userMessagesOnly(acc), repeatedPetitionDefinitions) {
				petitionMentions[name] += mentions
			}

			for _, name := range summary.FailureSignals {
				failureSessions[name]++
			}
			for name, mentions := range matchDefinitions(joinEvidence(acc), failureDefinitions) {
				failureMentions[name] += mentions
			}

			for _, tool := range summary.ToolNames {
				toolSessions[tool]++
			}
			for _, tool := range acc.ToolInvocations {
				name := canonicalToolName(tool.Name)
				toolMentions[name]++
			}
			continue
		}

		result.ExcludedSessions++
		result.Excluded = append(result.Excluded, summary)
	}

	result.Sessions = sortSessions(result.Sessions)
	result.Excluded = sortSessions(result.Excluded)
	result.LoopIncidents = sortLoopIncidents(result.LoopIncidents)
	result.ThemeCounts = toCounts(themeSessions, themeMentions)
	result.PetitionCounts = toCounts(petitionSessions, petitionMentions)
	result.FailureCounts = toCounts(failureSessions, failureMentions)
	result.TopTools = toCounts(toolSessions, toolMentions)
	result.IncludedByClass = []Count{
		{Name: "primary", Sessions: classCounts["primary"], Mentions: classCounts["primary"]},
		{Name: "secondary", Sessions: classCounts["secondary"], Mentions: classCounts["secondary"]},
	}
	result.PrimarySessionCount = classCounts["primary"]
	result.SecondaryCount = classCounts["secondary"]
	result.Recommendations = buildRecommendations(result)

	files, err := writeArtifacts(result, normalized.OutDir)
	if err != nil {
		return Result{}, err
	}
	result.GeneratedFiles = files
	return result, nil
}

func normalizeOptions(opts Options) (Options, error) {
	repoRoot := strings.TrimSpace(opts.RepoRoot)
	if repoRoot == "" {
		wd, err := os.Getwd()
		if err != nil {
			return Options{}, fmt.Errorf("failed to resolve repo root: %w", err)
		}
		repoRoot = wd
	}
	repoRoot, err := filepath.Abs(repoRoot)
	if err != nil {
		return Options{}, fmt.Errorf("failed to resolve repo root: %w", err)
	}

	codexHome := strings.TrimSpace(opts.CodexHome)
	if codexHome == "" {
		codexHome = os.Getenv("CODEX_HOME")
	}
	if codexHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return Options{}, fmt.Errorf("failed to resolve home directory: %w", err)
		}
		codexHome = filepath.Join(home, ".codex")
	}
	codexHome, err = filepath.Abs(codexHome)
	if err != nil {
		return Options{}, fmt.Errorf("failed to resolve codex home: %w", err)
	}

	outDir := strings.TrimSpace(opts.OutDir)
	if outDir == "" {
		harnessDir, err := userpaths.HarnessDir(repoRoot)
		if err != nil {
			return Options{}, err
		}
		outDir = filepath.Join(harnessDir, "codex-analysis")
	}
	outDir, err = filepath.Abs(outDir)
	if err != nil {
		return Options{}, fmt.Errorf("failed to resolve output directory: %w", err)
	}

	return Options{
		RepoRoot:  filepath.Clean(repoRoot),
		CodexHome: filepath.Clean(codexHome),
		OutDir:    filepath.Clean(outDir),
	}, nil
}

func loadSessions(codexHome string) ([]sessionAccumulator, int, int, error) {
	dirs := []string{
		filepath.Join(codexHome, "sessions"),
		filepath.Join(codexHome, "archived_sessions"),
	}

	files := make([]string, 0, 128)
	for _, dir := range dirs {
		err := filepath.WalkDir(dir, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".jsonl" && ext != ".json" {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			return nil, 0, 0, fmt.Errorf("failed to scan %q: %w", dir, err)
		}
	}
	sort.Strings(files)

	accumulators := make([]sessionAccumulator, 0, len(files))
	skipped := 0
	for _, path := range files {
		acc, err := parseSessionFile(codexHome, path)
		if err != nil {
			skipped++
			continue
		}
		if acc.ID == "" {
			continue
		}
		accumulators = append(accumulators, acc)
	}

	return accumulators, len(files), skipped, nil
}

func parseSessionFile(codexHome, path string) (sessionAccumulator, error) {
	if filepath.Ext(path) == ".json" {
		return parseLegacySessionFile(codexHome, path)
	}

	f, err := os.Open(path)
	if err != nil {
		return sessionAccumulator{}, err
	}
	defer f.Close()

	acc := sessionAccumulator{
		FilePath:     path,
		TurnCwds:     map[string]struct{}{},
		RelativeFile: path,
	}
	if rel, err := filepath.Rel(codexHome, path); err == nil {
		acc.RelativeFile = rel
	}

	reader := bufio.NewReader(f)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return sessionAccumulator{}, err
		}

		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var envelope lineEnvelope
			if unmarshalErr := json.Unmarshal(line, &envelope); unmarshalErr == nil {
				applyEnvelope(&acc, envelope)
			}
		}

		if err == io.EOF {
			break
		}
	}

	return acc, nil
}

func parseLegacySessionFile(codexHome, path string) (sessionAccumulator, error) {
	f, err := os.Open(path)
	if err != nil {
		return sessionAccumulator{}, err
	}
	defer f.Close()

	acc := sessionAccumulator{
		FilePath:     path,
		TurnCwds:     map[string]struct{}{},
		RelativeFile: path,
	}
	if rel, err := filepath.Rel(codexHome, path); err == nil {
		acc.RelativeFile = rel
	}

	decoder := json.NewDecoder(f)
	for {
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			if err == io.EOF {
				break
			}
			return sessionAccumulator{}, err
		}
		applyLegacyValue(&acc, raw)
	}

	return acc, nil
}

func applyEnvelope(acc *sessionAccumulator, envelope lineEnvelope) {
	if ts := parseTime(envelope.Timestamp); !ts.IsZero() {
		if acc.StartedAt.IsZero() {
			acc.StartedAt = ts
		}
		acc.LastEventAt = ts
	}

	switch envelope.Type {
	case "session_meta":
		var payload sessionMetaPayload
		if json.Unmarshal(envelope.Payload, &payload) != nil {
			return
		}
		if payload.ID != "" {
			acc.ID = payload.ID
		}
		if ts := parseTime(payload.Timestamp); !ts.IsZero() {
			acc.StartedAt = ts
		}
		if payload.Cwd != "" {
			acc.SessionCwd = payload.Cwd
			acc.TurnCwds[payload.Cwd] = struct{}{}
		}
		if payload.Git != nil && payload.Git.RepositoryURL != "" {
			acc.GitRepoURL = payload.Git.RepositoryURL
		}
	case "turn_context":
		var payload turnContextPayload
		if json.Unmarshal(envelope.Payload, &payload) != nil {
			return
		}
		if payload.Cwd != "" {
			acc.TurnCwds[payload.Cwd] = struct{}{}
		}
		if acc.GitRepoURL == "" && payload.Git != nil && payload.Git.RepositoryURL != "" {
			acc.GitRepoURL = payload.Git.RepositoryURL
		}
	case "event_msg":
		var payload eventMsgPayload
		if json.Unmarshal(envelope.Payload, &payload) != nil {
			return
		}
		text := strings.TrimSpace(payload.Message)
		switch payload.Type {
		case "user_message":
			if text != "" {
				acc.EventConversation = append(acc.EventConversation, conversationMessage{Role: "user", Text: text})
			}
		case "agent_message":
			if text != "" {
				acc.EventConversation = append(acc.EventConversation, conversationMessage{Role: "assistant", Text: text})
			}
		case "task_started":
			acc.TaskStarted++
		case "task_complete":
			acc.TaskCompleted++
		}
	case "response_item":
		if bytes.Contains(envelope.Payload, []byte(`"type":"message"`)) {
			var payload responseMessagePayload
			if json.Unmarshal(envelope.Payload, &payload) == nil {
				text := collectResponseText(payload.Content)
				if text != "" {
					acc.ResponseConversation = append(acc.ResponseConversation, conversationMessage{Role: payload.Role, Text: text})
				}
				return
			}
		}
		if bytes.Contains(envelope.Payload, []byte(`"type":"function_call"`)) {
			var payload functionCallPayload
			if json.Unmarshal(envelope.Payload, &payload) != nil {
				return
			}
			acc.ToolInvocations = append(acc.ToolInvocations, decodeToolInvocations(payload.Name, payload.Arguments)...)
		}
	}
}

type legacyWrapper struct {
	Session *struct {
		Timestamp string `json:"timestamp"`
		ID        string `json:"id"`
	} `json:"session"`
	Items []json.RawMessage `json:"items"`
}

func applyLegacyValue(acc *sessionAccumulator, raw json.RawMessage) {
	var wrapper legacyWrapper
	if json.Unmarshal(raw, &wrapper) == nil && (wrapper.Session != nil || len(wrapper.Items) > 0) {
		if wrapper.Session != nil {
			if wrapper.Session.ID != "" {
				acc.ID = wrapper.Session.ID
			}
			if ts := parseTime(wrapper.Session.Timestamp); !ts.IsZero() {
				acc.StartedAt = ts
				acc.LastEventAt = ts
			}
		}
		for _, item := range wrapper.Items {
			applyLegacyItem(acc, item)
		}
		return
	}

	applyLegacyItem(acc, raw)
}

func applyLegacyItem(acc *sessionAccumulator, raw json.RawMessage) {
	var kind struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(raw, &kind) != nil {
		return
	}

	if kind.Type == "message" {
		var payload responseMessagePayload
		if json.Unmarshal(raw, &payload) == nil {
			text := collectResponseText(payload.Content)
			if text != "" {
				acc.ResponseConversation = append(acc.ResponseConversation, conversationMessage{Role: payload.Role, Text: text})
			}
			return
		}
	}

	if kind.Type == "function_call" {
		var payload functionCallPayload
		if json.Unmarshal(raw, &payload) == nil {
			acc.ToolInvocations = append(acc.ToolInvocations, decodeToolInvocations(payload.Name, payload.Arguments)...)
		}
	}
}

func collectResponseText(items []responseContent) string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		if item.Text == "" {
			continue
		}
		lines = append(lines, strings.TrimSpace(item.Text))
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func decodeToolInvocations(name string, raw json.RawMessage) []toolInvocation {
	name = canonicalToolName(name)
	args := decodeJSONObject(raw)
	if name == "parallel" {
		toolUses, ok := args["tool_uses"].([]interface{})
		if !ok {
			return []toolInvocation{{Name: name}}
		}
		invocations := make([]toolInvocation, 0, len(toolUses))
		for _, item := range toolUses {
			entry, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			recipient, _ := entry["recipient_name"].(string)
			params, _ := entry["parameters"].(map[string]interface{})
			command, workdir := extractExecFields(canonicalToolName(recipient), params)
			invocations = append(invocations, toolInvocation{
				Name:    canonicalToolName(recipient),
				Command: command,
				Workdir: workdir,
			})
		}
		if len(invocations) > 0 {
			return invocations
		}
	}

	command, workdir := extractExecFields(name, args)
	return []toolInvocation{{
		Name:    name,
		Command: command,
		Workdir: workdir,
	}}
}

func extractExecFields(name string, args map[string]interface{}) (string, string) {
	if name != "exec_command" {
		return "", ""
	}

	command, _ := args["cmd"].(string)
	workdir, _ := args["workdir"].(string)
	return command, workdir
}

func decodeJSONObject(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return map[string]interface{}{}
	}

	var direct map[string]interface{}
	if json.Unmarshal(raw, &direct) == nil {
		return direct
	}

	var encoded string
	if json.Unmarshal(raw, &encoded) != nil {
		return map[string]interface{}{}
	}

	encoded = strings.TrimSpace(encoded)
	if encoded == "" {
		return map[string]interface{}{}
	}

	if json.Unmarshal([]byte(encoded), &direct) == nil {
		return direct
	}

	return map[string]interface{}{}
}

func summarizeSession(acc sessionAccumulator, repoRoot string) (SessionSummary, []LoopIncident) {
	conversation := chooseConversation(acc)
	userMessages := filterMessagesByRole(conversation, "user")
	agentMessages := filterMessagesByRole(conversation, "assistant")
	firstUser := ""
	if len(userMessages) > 0 {
		firstUser = userMessages[0]
	}

	reasons, classification, others := classifySession(acc, repoRoot, firstUser)
	themes, petitions, failures := detectSummarySignals(acc, conversation)
	loops := detectLoopIncidents(acc, conversation, classification)
	toolNames := uniqueToolNames(acc.ToolInvocations)

	summary := SessionSummary{
		ID:                 acc.ID,
		File:               acc.RelativeFile,
		StartedAt:          acc.StartedAt,
		LastEventAt:        acc.LastEventAt,
		Classification:     classification,
		FirstUserMessage:   shortenText(firstUser, 180),
		UserTurns:          len(userMessages),
		AgentMessages:      len(agentMessages),
		TaskStarted:        acc.TaskStarted,
		TaskCompleted:      acc.TaskCompleted,
		Themes:             themes,
		RepeatedPetitions:  petitions,
		FailureSignals:     failures,
		LoopIncidentCount:  len(loops),
		RelevanceReasons:   reasons,
		OtherProjectPaths:  others,
		ToolNames:          toolNames,
		RepresentativeUser: sampleRepresentativeUserMessages(userMessages),
	}
	return summary, loops
}

func chooseConversation(acc sessionAccumulator) []conversationMessage {
	if len(acc.EventConversation) > 0 {
		return compactConversation(acc.EventConversation)
	}
	filtered := make([]conversationMessage, 0, len(acc.ResponseConversation))
	for _, item := range acc.ResponseConversation {
		if item.Role == "user" && isBoilerplateInput(item.Text) {
			continue
		}
		filtered = append(filtered, item)
	}
	return compactConversation(filtered)
}

func compactConversation(in []conversationMessage) []conversationMessage {
	out := make([]conversationMessage, 0, len(in))
	for _, item := range in {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		if len(out) > 0 && out[len(out)-1].Role == item.Role && out[len(out)-1].Text == text {
			continue
		}
		out = append(out, conversationMessage{Role: item.Role, Text: text})
	}
	return out
}

func classifySession(acc sessionAccumulator, repoRoot, firstUser string) ([]string, string, []string) {
	repoRoot = filepath.Clean(repoRoot)
	markers := repoMarkers(repoRoot)
	conversation := chooseConversation(acc)
	textEvidence := make([]string, 0, len(conversation)+len(acc.ToolInvocations)*2)
	for _, item := range conversation {
		textEvidence = append(textEvidence, item.Text)
	}
	for _, tool := range acc.ToolInvocations {
		if tool.Command != "" {
			textEvidence = append(textEvidence, tool.Command)
		}
		if tool.Workdir != "" {
			textEvidence = append(textEvidence, tool.Workdir)
		}
	}
	userText := strings.ToLower(strings.Join(userMessagesOnly(acc), "\n"))

	reasons := []string{}
	otherProjects := collectOtherProjectRefs(textEvidence, repoRoot)

	cwdMatch := acc.SessionCwd == repoRoot
	if cwdMatch {
		reasons = append(reasons, "session cwd matches repo root")
	}

	turnCwdMatch := false
	for cwd := range acc.TurnCwds {
		if filepath.Clean(cwd) == repoRoot {
			turnCwdMatch = true
			break
		}
	}
	if turnCwdMatch && !cwdMatch {
		reasons = append(reasons, "turn context cwd matches repo root")
	}

	repoPathMentionCount := 0
	repoSpecificTextCount := 0
	repoCommandCount := 0
	for _, text := range textEvidence {
		lower := strings.ToLower(text)
		if strings.Contains(text, repoRoot) {
			repoPathMentionCount++
		}
		if containsAny(lower, markers) {
			repoSpecificTextCount++
		}
	}
	for _, tool := range acc.ToolInvocations {
		commandLower := strings.ToLower(tool.Command)
		if strings.Contains(tool.Command, repoRoot) || containsAny(commandLower, markers) {
			repoCommandCount++
		}
		if filepath.Clean(tool.Workdir) == repoRoot && repoSpecificTextCount > 0 {
			repoCommandCount++
		}
	}
	if repoPathMentionCount > 0 {
		reasons = append(reasons, fmt.Sprintf("repo path mentioned %d times", repoPathMentionCount))
	}
	if repoSpecificTextCount > 0 {
		reasons = append(reasons, fmt.Sprintf("repo-specific markers mentioned %d times", repoSpecificTextCount))
	}
	if repoCommandCount > 0 {
		reasons = append(reasons, fmt.Sprintf("repo-scoped commands found %d times", repoCommandCount))
	}

	if len(otherProjects) > 0 && repoSpecificTextCount == 0 && repoCommandCount == 0 {
		reasons = append(reasons, "other project paths dominate without repo-specific markers")
		return reasons, "cross_project", otherProjects
	}

	if repoSpecificTextCount > 0 && (repoCommandCount > 0 || repoPathMentionCount > 0 || cwdMatch || turnCwdMatch) {
		return reasons, "primary", otherProjects
	}

	if (cwdMatch || turnCwdMatch) && (repoPathMentionCount > 0 || repoSpecificTextCount > 0 || containsAny(userText, markers)) {
		reasons = append(reasons, "repo context present but weaker than direct command evidence")
		return reasons, "secondary", otherProjects
	}

	if cwdMatch || turnCwdMatch {
		reasons = append(reasons, "cwd matches but no direct repo evidence")
		return reasons, "path_only", otherProjects
	}

	if firstUser != "" && containsAny(strings.ToLower(firstUser), markers) {
		reasons = append(reasons, "first user turn mentions repo by name")
		return reasons, "secondary", otherProjects
	}

	reasons = append(reasons, "no direct repo evidence")
	return reasons, "excluded", otherProjects
}

func repoMarkers(repoRoot string) []string {
	name := strings.ToLower(filepath.Base(repoRoot))
	markers := []string{name, repoRoot}

	base := strings.TrimSuffix(name, "-go")
	base = strings.TrimSuffix(base, "-cli")
	if base != "" && base != name {
		markers = append(markers, base, "./"+base, "cmd/"+base)
	}

	parts := strings.Split(base, "-")
	if len(parts) >= 2 {
		firstTwo := strings.Join(parts[:2], "-")
		markers = append(markers, firstTwo, strings.Join(parts[:2], " "), "./"+firstTwo, "cmd/"+firstTwo)
	}

	deduped := make([]string, 0, len(markers))
	seen := map[string]struct{}{}
	for _, marker := range markers {
		marker = strings.TrimSpace(marker)
		if marker == "" {
			continue
		}
		if _, ok := seen[marker]; ok {
			continue
		}
		seen[marker] = struct{}{}
		deduped = append(deduped, marker)
	}
	return deduped
}

func detectSummarySignals(acc sessionAccumulator, conversation []conversationMessage) ([]string, []string, []string) {
	evidence := joinEvidence(acc)
	userOnly := userMessagesOnly(acc)
	themes := sortedMatchedNames(matchDefinitions(evidence, themeDefinitions))
	petitions := sortedMatchedNames(matchDefinitions(userOnly, repeatedPetitionDefinitions))
	failures := sortedMatchedNames(matchDefinitions(evidence, failureDefinitions))

	if len(failures) == 0 && len(conversation) > 0 {
		for _, item := range conversation {
			if item.Role != "user" {
				continue
			}
			lower := strings.ToLower(item.Text)
			if containsAny(lower, correctionPhrases) {
				failures = append(failures, "premature-completion")
				break
			}
		}
	}

	return themes, petitions, failures
}

func detectLoopIncidents(acc sessionAccumulator, conversation []conversationMessage, classification string) []LoopIncident {
	incidents := []LoopIncident{}
	lastClaim := ""
	lastClaimIdx := -1

	for i, item := range conversation {
		lower := strings.ToLower(item.Text)
		switch item.Role {
		case "assistant":
			if containsAny(lower, completionClaimPhrases) {
				lastClaim = shortenText(item.Text, 220)
				lastClaimIdx = i
			}
		case "user":
			if lastClaim == "" {
				continue
			}
			if !containsAny(lower, correctionPhrases) {
				continue
			}
			incidents = append(incidents, LoopIncident{
				SessionID:        acc.ID,
				SessionFile:      acc.RelativeFile,
				AgentClaim:       lastClaim,
				UserCorrection:   shortenText(item.Text, 220),
				Classification:   classification,
				PreviousClaimIdx: lastClaimIdx,
			})
			lastClaim = ""
			lastClaimIdx = -1
		}
	}

	return incidents
}

func joinEvidence(acc sessionAccumulator) []string {
	evidence := make([]string, 0, len(acc.EventConversation)+len(acc.ResponseConversation)+len(acc.ToolInvocations)+2)
	if acc.SessionCwd != "" {
		evidence = append(evidence, acc.SessionCwd)
	}
	if acc.GitRepoURL != "" {
		evidence = append(evidence, acc.GitRepoURL)
	}
	for _, item := range chooseConversation(acc) {
		evidence = append(evidence, item.Text)
	}
	for _, tool := range acc.ToolInvocations {
		evidence = append(evidence, canonicalToolName(tool.Name))
		if tool.Command != "" {
			evidence = append(evidence, tool.Command)
		}
		if tool.Workdir != "" {
			evidence = append(evidence, tool.Workdir)
		}
	}
	return evidence
}

func userMessagesOnly(acc sessionAccumulator) []string {
	conversation := chooseConversation(acc)
	return filterMessagesByRole(conversation, "user")
}

func filterMessagesByRole(messages []conversationMessage, role string) []string {
	out := []string{}
	for _, item := range messages {
		if item.Role == role {
			out = append(out, item.Text)
		}
	}
	return out
}

func matchDefinitions(texts []string, defs []matchDefinition) map[string]int {
	matches := make(map[string]int, len(defs))
	for _, text := range texts {
		lower := strings.ToLower(text)
		for _, def := range defs {
			for _, phrase := range def.Phrases {
				if strings.Contains(lower, phrase) {
					matches[def.Name]++
				}
			}
		}
	}
	return matches
}

func sortedMatchedNames(in map[string]int) []string {
	names := make([]string, 0, len(in))
	for name, count := range in {
		if count > 0 {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}

func collectOtherProjectRefs(texts []string, repoRoot string) []string {
	seen := map[string]struct{}{}
	for _, text := range texts {
		for _, match := range otherProjectPattern.FindAllString(text, -1) {
			cleaned := strings.TrimRight(match, ".,:;)]}")
			if strings.HasPrefix(cleaned, repoRoot) {
				continue
			}
			seen[cleaned] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for value := range seen {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func uniqueToolNames(invocations []toolInvocation) []string {
	seen := map[string]struct{}{}
	for _, tool := range invocations {
		name := canonicalToolName(tool.Name)
		if name == "" {
			continue
		}
		seen[name] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

func sampleRepresentativeUserMessages(messages []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 3)
	for _, msg := range messages {
		trimmed := shortenText(msg, 180)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
		if len(out) == 3 {
			break
		}
	}
	return out
}

func buildRecommendations(result Result) []Recommendation {
	recs := []Recommendation{
		{
			Title:    "Add Shared JSONL Traces Across TUI and Query",
			Why:      fmt.Sprintf("Observability petitions appeared in %d included sessions, and users explicitly asked for fuller logs.", countSessions(result.PetitionCounts, "logs-and-traces")),
			Proposal: "Write structured JSONL run traces for every turn, tool call, timeout, cancellation, and final status across both `query` and TUI paths. Store raw provider summaries plus replay metadata, not only stderr diagnostics.",
		},
		{
			Title:    "Build a Transcript-Derived Eval Harness",
			Why:      fmt.Sprintf("Verification pressure showed up in %d sessions, and repeated regressions came from hangs, malformed tool calls, and panic paths.", countSessions(result.ThemeCounts, "verification-pressure")),
			Proposal: "Promote the top historical failures into fixtures under `evals/` and gate changes on replay-style checks: malformed streamed tool calls, startup fallback, cancel/continue semantics, panic repros, and bash hang detection.",
		},
		{
			Title:    "Model Interrupted and Incomplete Turns Explicitly",
			Why:      fmt.Sprintf("%d loop incidents were detected where the user had to push the agent back into the task after an apparent completion claim.", len(result.LoopIncidents)),
			Proposal: "Persist `completed`, `failed`, `cancelled`, `timed_out`, and `interrupted` turn states. Make continue/resume semantics consume that state so the next turn can inherit unfinished context instead of pretending the previous run finished cleanly.",
		},
		{
			Title:    "Add Watchdogs for Interactive or Hanging Tools",
			Why:      fmt.Sprintf("Failure reports for hangs and timeouts appeared across %d sessions.", countSessions(result.FailureCounts, "hangs-and-timeouts")),
			Proposal: "Detect commands likely to request interactivity, enforce outer watchdogs, and record timeout causes directly in session history and traces so autonomous loops fail fast instead of stalling silently.",
		},
		{
			Title:    "Encode Entropy Controls in Repo Automation",
			Why:      fmt.Sprintf("Commit hygiene and verification petitions repeated throughout the corpus, including %d sessions explicitly asking for atomic conventional commits.", countSessions(result.PetitionCounts, "atomic-conventional-commits")),
			Proposal: "Add normal CI for build/test/smoke/evals, keep change scopes small, and generate a repo-local analysis report from Codex history so entropy is measured instead of discovered ad hoc in future sessions.",
		},
	}
	return recs
}

func writeArtifacts(result Result, outDir string) ([]string, error) {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create output directory %q: %w", outDir, err)
	}

	files := []struct {
		path    string
		content []byte
	}{
		{path: filepath.Join(outDir, "summary.md"), content: []byte(renderSummaryMarkdown(result))},
		{path: filepath.Join(outDir, "action-plan.md"), content: []byte(renderActionPlanMarkdown(result))},
	}

	reportJSON, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal report json: %w", err)
	}
	files = append(files, struct {
		path    string
		content []byte
	}{path: filepath.Join(outDir, "report.json"), content: append(reportJSON, '\n')})

	sessionLines := make([][]byte, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		line, err := json.Marshal(session)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal session summary: %w", err)
		}
		sessionLines = append(sessionLines, append(line, '\n'))
	}
	files = append(files, struct {
		path    string
		content []byte
	}{path: filepath.Join(outDir, "relevant-sessions.jsonl"), content: bytes.Join(sessionLines, nil)})

	written := make([]string, 0, len(files))
	for _, file := range files {
		if err := os.WriteFile(file.path, file.content, 0644); err != nil {
			return nil, fmt.Errorf("failed to write %q: %w", file.path, err)
		}
		written = append(written, file.path)
	}

	return written, nil
}

func renderSummaryMarkdown(result Result) string {
	var b strings.Builder
	b.WriteString("# Codex Session Summary\n\n")
	b.WriteString(fmt.Sprintf("Generated: %s\n\n", result.GeneratedAt.Format(time.RFC3339)))
	b.WriteString(fmt.Sprintf("- Repo root: `%s`\n", result.RepoRoot))
	b.WriteString(fmt.Sprintf("- Codex home: `%s`\n", result.CodexHome))
	b.WriteString(fmt.Sprintf("- Scanned files: `%d`\n", result.ScannedFiles))
	b.WriteString(fmt.Sprintf("- Skipped malformed files: `%d`\n", result.SkippedFiles))
	b.WriteString(fmt.Sprintf("- Included sessions: `%d` (`%d` primary, `%d` secondary)\n", result.IncludedSessions, result.PrimarySessionCount, result.SecondaryCount))
	b.WriteString(fmt.Sprintf("- Excluded sessions: `%d`\n\n", result.ExcludedSessions))

	b.WriteString("## Common Themes\n\n")
	for _, count := range limitCounts(result.ThemeCounts, 6) {
		b.WriteString(fmt.Sprintf("- `%s`: %d sessions, %d mentions\n", count.Name, count.Sessions, count.Mentions))
	}
	b.WriteString("\n## Repeated Petitions\n\n")
	for _, count := range limitCounts(result.PetitionCounts, 6) {
		b.WriteString(fmt.Sprintf("- `%s`: %d sessions, %d mentions\n", count.Name, count.Sessions, count.Mentions))
	}
	b.WriteString("\n## Debugging Gone Wrong\n\n")
	for _, count := range limitCounts(result.FailureCounts, 6) {
		b.WriteString(fmt.Sprintf("- `%s`: %d sessions, %d mentions\n", count.Name, count.Sessions, count.Mentions))
	}

	b.WriteString("\n## Loops That Should Have Continued\n\n")
	if len(result.LoopIncidents) == 0 {
		b.WriteString("- No loop incidents detected by the current heuristic.\n")
	} else {
		for _, incident := range limitLoopIncidents(result.LoopIncidents, 8) {
			b.WriteString(fmt.Sprintf("- `%s`: agent claimed `%s`; user came back with `%s`\n",
				incident.SessionID,
				shortenText(incident.AgentClaim, 80),
				shortenText(incident.UserCorrection, 80),
			))
		}
	}

	b.WriteString("\n## Relevant Sessions\n\n")
	for _, session := range limitSessions(result.Sessions, 12) {
		b.WriteString(fmt.Sprintf("- `%s` `%s`: %s\n", session.ID, session.Classification, session.FirstUserMessage))
	}
	return b.String()
}

func renderActionPlanMarkdown(result Result) string {
	var b strings.Builder
	b.WriteString("# Harness Action Plan\n\n")
	b.WriteString("This plan is generated from Codex session history for this repository and turns the dominant failure patterns into harness work.\n\n")
	for i, rec := range result.Recommendations {
		b.WriteString(fmt.Sprintf("## Phase %d: %s\n\n", i+1, rec.Title))
		b.WriteString(fmt.Sprintf("- Why: %s\n", rec.Why))
		b.WriteString(fmt.Sprintf("- Proposal: %s\n\n", rec.Proposal))
	}
	return b.String()
}

func toCounts(sessionCounts, mentionCounts map[string]int) []Count {
	names := make([]string, 0, len(sessionCounts))
	for name := range sessionCounts {
		names = append(names, name)
	}
	sort.Strings(names)

	counts := make([]Count, 0, len(names))
	for _, name := range names {
		counts = append(counts, Count{
			Name:     name,
			Sessions: sessionCounts[name],
			Mentions: mentionCounts[name],
		})
	}

	sort.SliceStable(counts, func(i, j int) bool {
		if counts[i].Sessions == counts[j].Sessions {
			if counts[i].Mentions == counts[j].Mentions {
				return counts[i].Name < counts[j].Name
			}
			return counts[i].Mentions > counts[j].Mentions
		}
		return counts[i].Sessions > counts[j].Sessions
	})
	return counts
}

func sortSessions(in []SessionSummary) []SessionSummary {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].StartedAt.Equal(in[j].StartedAt) {
			return in[i].ID < in[j].ID
		}
		return in[i].StartedAt.Before(in[j].StartedAt)
	})
	return in
}

func sortLoopIncidents(in []LoopIncident) []LoopIncident {
	sort.SliceStable(in, func(i, j int) bool {
		if in[i].SessionFile == in[j].SessionFile {
			return in[i].PreviousClaimIdx < in[j].PreviousClaimIdx
		}
		return in[i].SessionFile < in[j].SessionFile
	})
	return in
}

func limitCounts(in []Count, max int) []Count {
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func limitSessions(in []SessionSummary, max int) []SessionSummary {
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func limitLoopIncidents(in []LoopIncident, max int) []LoopIncident {
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func canonicalToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	if idx := strings.LastIndex(name, "."); idx >= 0 && idx < len(name)-1 {
		return name[idx+1:]
	}
	return name
}

func isBoilerplateInput(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, prefix := range boilerplatePrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func containsAny(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}

func shortenText(text string, max int) string {
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if max <= 0 || len(text) <= max {
		return text
	}
	if max <= 3 {
		return text[:max]
	}
	return text[:max-3] + "..."
}

func parseTime(value string) time.Time {
	if strings.TrimSpace(value) == "" {
		return time.Time{}
	}
	ts, err := time.Parse(time.RFC3339Nano, value)
	if err == nil {
		return ts
	}
	ts, err = time.Parse(time.RFC3339, value)
	if err == nil {
		return ts
	}
	return time.Time{}
}

func countSessions(counts []Count, name string) int {
	for _, count := range counts {
		if count.Name == name {
			return count.Sessions
		}
	}
	return 0
}
