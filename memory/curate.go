// curate.go — marlowe curation loop.
// Reads queue + proposals, invokes the marlowe persona via the configured
// agent, archives processed inputs, and returns a CurateReport.
//
// Single-writer invariant: only marlowe's session (JEFF_MEMORY_CAN_ADD=1)
// calls WriteCanonical. Curate itself does not write canonical entries; it
// delegates to the agent which uses `jeff memory add`.
package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// AgentRunner abstracts the LLM invocation so tests can mock it.
type AgentRunner interface {
	// Run invokes the agent non-interactively with prompt and extra env vars.
	// Returns the agent's stdout output.
	Run(ctx context.Context, prompt string, env []string) (string, error)
}

// ExecRunner is the default AgentRunner that shells out to an agent binary.
type ExecRunner struct {
	// Command is the agent binary (e.g. "claude").
	Command string
	// BuildArgs returns the full arg list for a non-interactive run, given the prompt.
	// This is typically AgentProvider.BuildCurateArgs.
	BuildArgs func(prompt string) []string
}

// Run executes the agent non-interactively. env entries are appended to the
// current process environment (format: "KEY=VALUE").
func (r ExecRunner) Run(ctx context.Context, prompt string, env []string) (string, error) {
	if r.Command == "" {
		return "", fmt.Errorf("ExecRunner: no command set")
	}
	if r.BuildArgs == nil {
		return "", fmt.Errorf("ExecRunner: no BuildArgs set")
	}
	args := r.BuildArgs(prompt)
	cmd := exec.CommandContext(ctx, r.Command, args...)
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if err != nil {
		return string(out), fmt.Errorf("ExecRunner: %s: %w", r.Command, err)
	}
	return string(out), nil
}

// CurateOptions controls a single curate pass.
type CurateOptions struct {
	Home         string
	Persona      string      // optional: only process this persona's queue entries
	Auto         bool        // auto=true: no interactive prompts; flag conflicts in report only
	Runner       AgentRunner // nil = caller must set (use NewExecRunner in cmd layer)
	SkillContent string      // content of .skills/curation/SKILL.md; loaded by cmd layer
}

// CurateReport summarises the results of a curation pass.
type CurateReport struct {
	Processed   int
	Accepted    int
	Skipped     int // dedupe hits
	Invalidated int
	Flagged     []string // entry names needing user review
	Errors      []error
}

// agentReport mirrors the JSON block marlowe emits at the end of a pass.
type agentReport struct {
	Accepted    int      `json:"accepted"`
	Skipped     int      `json:"skipped"`
	Invalidated int      `json:"invalidated"`
	Flagged     []string `json:"flagged"`
}

// queueItem pairs a SessionQueueEntry with its file path on disk.
type queueItem struct {
	Entry SessionQueueEntry
	Path  string
}

// Curate is the entry point for a curation pass. It:
//  1. Reads queue entries (filtered by Opts.Persona if set).
//  2. For each entry: loads proposals, builds a prompt, invokes Opts.Runner.
//  3. Parses the JSON report emitted by the agent.
//  4. Archives the processed queue entry + proposals.
//  5. Returns an aggregated CurateReport.
//
// The agent (marlowe) writes canonical entries via `jeff memory add`; Curate
// itself does not call WriteCanonical.
func Curate(opts CurateOptions) (CurateReport, error) {
	var report CurateReport

	if opts.Home == "" {
		return report, fmt.Errorf("Curate: Home is required")
	}
	if opts.Runner == nil {
		return report, fmt.Errorf("Curate: Runner is required")
	}

	items, err := listQueueItems(opts.Home)
	if err != nil {
		return report, fmt.Errorf("Curate: list queue: %w", err)
	}

	if opts.Persona != "" {
		filtered := items[:0]
		for _, it := range items {
			if it.Entry.Persona == opts.Persona {
				filtered = append(filtered, it)
			}
		}
		items = filtered
	}

	for _, item := range items {
		proposals, err := ListProposals(opts.Home, item.Entry.Persona, item.Entry.Task)
		if err != nil {
			report.Errors = append(report.Errors,
				fmt.Errorf("load proposals %s/%s: %w", item.Entry.Persona, item.Entry.Task, err))
			continue
		}

		report.Processed++

		prompt := buildCuratePrompt(opts.SkillContent, item.Entry, proposals)
		env := []string{
			"JEFF_MEMORY_CAN_ADD=1",
			"JEFF_HOME=" + opts.Home,
		}

		ctx := context.Background()
		output, runErr := opts.Runner.Run(ctx, prompt, env)
		if runErr != nil {
			report.Errors = append(report.Errors,
				fmt.Errorf("agent run for %s: %w", item.Entry.Task, runErr))
			// Still archive so we don't reprocess indefinitely.
		}

		partial := parseAgentReport(output)
		report.Accepted += partial.Accepted
		report.Skipped += partial.Skipped
		report.Invalidated += partial.Invalidated
		report.Flagged = append(report.Flagged, partial.Flagged...)

		// Archive queue entry.
		if archErr := ArchiveQueueEntry(opts.Home, item.Path); archErr != nil {
			report.Errors = append(report.Errors,
				fmt.Errorf("archive queue entry %s: %w", item.Path, archErr))
		}

		// Archive proposals.
		for _, p := range proposals {
			if archErr := archiveProposal(opts.Home, p.Path); archErr != nil {
				report.Errors = append(report.Errors,
					fmt.Errorf("archive proposal %s: %w", p.Path, archErr))
			}
		}
	}

	return report, nil
}

// buildCuratePrompt assembles the prompt sent to the marlowe agent.
func buildCuratePrompt(skillContent string, qe SessionQueueEntry, proposals []Proposal) string {
	var sb strings.Builder

	sb.WriteString("You are marlowe — the JEFF memory curator.\n\n")

	if skillContent != "" {
		sb.WriteString("## CURATION SKILL\n\n")
		sb.WriteString(skillContent)
		sb.WriteString("\n\n")
	}

	sb.WriteString("## SESSION TO PROCESS\n\n")
	sb.WriteString(fmt.Sprintf("Task:    %s\n", qe.Task))
	sb.WriteString(fmt.Sprintf("Persona: %s\n", qe.Persona))
	if !qe.EndedAt.IsZero() {
		sb.WriteString(fmt.Sprintf("Ended:   %s\n", qe.EndedAt.Format(time.RFC3339)))
	}
	if len(qe.Repos) > 0 {
		sb.WriteString(fmt.Sprintf("Repos:   %s\n", strings.Join(qe.Repos, ", ")))
	}
	if qe.Reason != "" {
		sb.WriteString(fmt.Sprintf("Reason:  %s\n", qe.Reason))
	}
	sb.WriteString("\n")

	if len(proposals) == 0 {
		sb.WriteString("## PROPOSALS\n\n(none)\n\n")
	} else {
		sb.WriteString("## PROPOSALS\n\n")
		for i, p := range proposals {
			sb.WriteString(fmt.Sprintf("### Proposal %d: %s\n\n", i+1, p.Slug))
			sb.WriteString(fmt.Sprintf("Type: %s\n", p.FM.Type))
			sb.WriteString(fmt.Sprintf("Description: %s\n\n", p.FM.Description))
			sb.WriteString(p.Body)
			sb.WriteString("\n---\n\n")
		}
	}

	sb.WriteString("## YOUR TASK\n\n")
	sb.WriteString("Process each proposal above:\n")
	sb.WriteString("1. Classify scope + bucket + goal alignment (see routing matrix)\n")
	sb.WriteString("2. Check deduplication against existing canonical entries\n")
	sb.WriteString("3. Accept via `jeff memory add`, skip duplicates, soft-invalidate conflicts\n")
	sb.WriteString("4. After all proposals: output a JSON block with the summary:\n\n")
	sb.WriteString("```json\n")
	sb.WriteString(`{"accepted": 0, "skipped": 0, "invalidated": 0, "flagged": []}`)
	sb.WriteString("\n```\n")

	return sb.String()
}

// parseAgentReport extracts the last JSON block from agent output that looks
// like a curation report. Returns zero-value on parse failure (non-fatal).
func parseAgentReport(output string) agentReport {
	var r agentReport
	// Find last occurrence of a JSON object with the expected keys.
	start := strings.LastIndex(output, `{"accepted"`)
	if start == -1 {
		start = strings.LastIndex(output, "{\n  \"accepted\"")
	}
	if start == -1 {
		return r
	}
	end := strings.Index(output[start:], "}")
	if end == -1 {
		return r
	}
	raw := output[start : start+end+1]
	json.Unmarshal([]byte(raw), &r) //nolint:errcheck // best-effort parse
	return r
}

// listQueueItems returns queue entries paired with their file paths.
func listQueueItems(home string) ([]queueItem, error) {
	dir := QueueSessionsRoot(home)
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("listQueueItems: readdir: %w", err)
	}

	var items []queueItem
	for _, de := range ents {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, de.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("listQueueItems: read %s: %w", path, err)
		}
		var e SessionQueueEntry
		if err := json.Unmarshal(data, &e); err != nil {
			return nil, fmt.Errorf("listQueueItems: unmarshal %s: %w", path, err)
		}
		items = append(items, queueItem{Entry: e, Path: path})
	}
	return items, nil
}

// archiveProposal moves a proposal file to archive/<iso-week>/proposals/<persona>/<task>/.
func archiveProposal(home, proposalPath string) error {
	info, err := os.Stat(proposalPath)
	if err != nil {
		return fmt.Errorf("archiveProposal: stat: %w", err)
	}
	week := isoWeek(info.ModTime())

	// Preserve proposals/<persona>/<task>/<file> sub-structure under archive.
	proposalsRoot := ProposalsRoot(home)
	rel, err := filepath.Rel(proposalsRoot, proposalPath)
	if err != nil {
		// Not under proposals root — just archive flat.
		rel = filepath.Base(proposalPath)
	}

	destDir := filepath.Join(ArchiveRoot(home), week, "proposals", filepath.Dir(rel))
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("archiveProposal: mkdir: %w", err)
	}
	dest := filepath.Join(destDir, filepath.Base(proposalPath))
	if err := os.Rename(proposalPath, dest); err != nil {
		return fmt.Errorf("archiveProposal: rename: %w", err)
	}
	return nil
}
