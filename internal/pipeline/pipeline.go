package pipeline

import (
	"context"
	"strings"

	"github.com/kunchenguid/no-mistakes/internal/agent"
	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/db"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

// ResolveIntegrationBranch returns the branch the pipeline rebases, reviews,
// and opens PRs against. Precedence: the per-run override, then the repo's
// persisted base branch, then the auto-detected default branch, then "main".
// The first non-empty value wins. This is intentionally independent of the
// trusted-config root, which always stays on the default branch.
func ResolveIntegrationBranch(runOverride string, repo *db.Repo) string {
	if b := strings.TrimSpace(runOverride); b != "" {
		return b
	}
	if repo != nil {
		if b := strings.TrimSpace(repo.BaseBranch); b != "" {
			return b
		}
		if b := strings.TrimSpace(repo.DefaultBranch); b != "" {
			return b
		}
	}
	return "main"
}

// StepContext provides shared resources to pipeline steps during execution.
type StepContext struct {
	Ctx              context.Context
	Run              *db.Run
	Repo             *db.Repo
	WorkDir          string
	Agent            agent.Agent
	Config           *config.Config
	DB               *db.DB
	Log              func(string) // discrete log line (newline-terminated, user-visible + file)
	LogChunk         func(string) // raw streaming chunk (user-visible + file)
	LogFile          func(string) // file-only log callback (not shown to user)
	Fixing           bool         // true when re-executing after a "fix" action
	PreviousFindings string       // JSON findings from the previous execution (set during fix loop)
	// StepResultID is the DB row ID of the current step's step_results record.
	// Steps use it to query their own round history for multi-round prompts.
	StepResultID string
	Env          []string // extra environment variables for subprocesses (used in tests)
	// UserIntent is a short, possibly-empty summary of what the change author
	// was trying to accomplish, inferred from local agent transcripts. It's
	// surfaced in step prompts so agents have context beyond the diff.
	UserIntent string
	// Scratch is in-memory, non-persisted state shared across the steps of a
	// single run. The executor creates one per run and threads the same pointer
	// into every step's context, so a later step can read what an earlier step
	// discovered (e.g. the command-proposal path reads the canonical commands
	// the test/lint discovery agents reported).
	Scratch *RunScratch
}

// RunScratch is per-run, in-memory state shared across steps. It is never
// persisted; it exists only to pass discovery results forward within a run.
type RunScratch struct {
	// DiscoveredCommands maps a commands.* field name to the canonical command a
	// discovery agent reported for it this run. Populated by the test and lint
	// steps (only for fields unset in the effective config); consumed by the
	// command-proposal path in the push step.
	DiscoveredCommands map[config.CommandField]string
}

// RecordDiscoveredCommand stores a non-empty canonical command for a field.
// Safe to call on a nil scratch (no-op) so steps need not nil-check.
func (s *RunScratch) RecordDiscoveredCommand(field config.CommandField, command string) {
	if s == nil || strings.TrimSpace(command) == "" {
		return
	}
	if s.DiscoveredCommands == nil {
		s.DiscoveredCommands = map[config.CommandField]string{}
	}
	s.DiscoveredCommands[field] = command
}

// IntegrationBranch returns the effective base branch for this run, applying
// the per-run override, the repo's persisted base branch, the default branch,
// and finally "main" in that order.
func (s *StepContext) IntegrationBranch() string {
	var runOverride string
	if s.Run != nil {
		runOverride = s.Run.BaseBranch
	}
	return ResolveIntegrationBranch(runOverride, s.Repo)
}

// StepOutcome is the result of executing a pipeline step.
type StepOutcome struct {
	NeedsApproval bool // whether the step pauses for user action
	AutoFixable   bool
	Findings      string // JSON findings for TUI display (optional)
	ExitCode      int    // process exit code (0 = success)
	PRURL         string // PR/MR URL if this step created or found one
	Skipped       bool   // mark the step as skipped without failing the run
	SkipRemaining bool   // skip all subsequent steps (e.g. empty diff after rebase)
	// FixSummary, when non-empty, is the agent's one-line commit summary for
	// the fix attempt performed during this round. Steps populate it in fix
	// mode so the executor can persist it on the round record and later
	// rounds can reference what was previously attempted.
	FixSummary string

	// DiscoveredCommands carries the canonical command(s) a discovery agent
	// reported this round, keyed by commands.* field. The executor merges these
	// into the run scratch so the command-proposal path can read them. Steps
	// populate it only for fields that were unset in the effective config.
	DiscoveredCommands map[config.CommandField]string

	// DurationOverrideMS, when positive, replaces the wall-clock duration
	// reported for this step. Used by demo mode to show realistic durations
	// without actually waiting.
	DurationOverrideMS int64
}

// Step is the interface that each pipeline step implements.
type Step interface {
	// Name returns the step's identity in the fixed pipeline sequence.
	Name() types.StepName

	// Execute runs the step logic and returns an outcome.
	// A step that returns NeedsApproval=true will pause the pipeline
	// until the user responds with an approval action.
	Execute(sctx *StepContext) (*StepOutcome, error)
}
