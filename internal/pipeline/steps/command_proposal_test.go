package steps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/pipeline"
)

func proposerContext(t *testing.T, cmds config.Commands, discovered map[config.CommandField]string) *pipeline.StepContext {
	t.Helper()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	sctx := newTestContextWithDBRecords(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, cmds)
	sctx.Config.ProposeCommands = true
	sctx.Shared = &pipeline.RunShared{}
	for field, command := range discovered {
		sctx.Shared.RecordDiscoveredCommand(field, command)
	}
	return sctx
}

func repoCommands(t *testing.T, workDir string) config.Commands {
	t.Helper()
	cfg, err := config.LoadRepo(workDir)
	if err != nil {
		t.Fatalf("load repo config: %v", err)
	}
	return cfg.Commands
}

func TestProposeDiscoveredCommands_WritesAndCommitsUnsetField(t *testing.T) {
	t.Parallel()
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test -race ./...",
	})
	beforeHead := sctx.Run.HeadSHA

	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if got := repoCommands(t, sctx.WorkDir).Test; got != "go test -race ./..." {
		t.Fatalf("commands.test = %q, want %q", got, "go test -race ./...")
	}
	if sctx.Run.HeadSHA == beforeHead {
		t.Fatal("expected HeadSHA to advance after proposal commit")
	}
	if msg := lastCommitMessage(t, sctx.WorkDir); !strings.Contains(msg, "propose discovered test command") {
		t.Fatalf("commit message = %q", msg)
	}
	if status := gitStatusPorcelain(t, sctx.WorkDir); status != "" {
		t.Fatalf("expected clean worktree, got %q", status)
	}
}

func TestProposeDiscoveredCommands_OnlyUnsetFields(t *testing.T) {
	t.Parallel()
	// lint is already configured in the effective config; test and format were
	// discovered. Only test and format should be proposed.
	sctx := proposerContext(t, config.Commands{Lint: "golangci-lint run"}, map[config.CommandField]string{
		config.CommandFieldTest:   "go test ./...",
		config.CommandFieldLint:   "should-not-be-proposed",
		config.CommandFieldFormat: "gofmt -w .",
	})
	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := repoCommands(t, sctx.WorkDir)
	if got.Test != "go test ./..." {
		t.Errorf("commands.test = %q", got.Test)
	}
	if got.Format != "gofmt -w ." {
		t.Errorf("commands.format = %q", got.Format)
	}
	if got.Lint != "" {
		t.Errorf("commands.lint should not be proposed for a configured field, got %q", got.Lint)
	}
	if msg := lastCommitMessage(t, sctx.WorkDir); !strings.Contains(msg, "test, format") {
		t.Errorf("commit message should list both proposed fields: %q", msg)
	}
}

func TestProposeDiscoveredCommands_NoOpWhenDisabled(t *testing.T) {
	t.Parallel()
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test ./...",
	})
	sctx.Config.ProposeCommands = false
	before := sctx.Run.HeadSHA

	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sctx.Run.HeadSHA != before {
		t.Fatal("expected no commit when feature disabled")
	}
	if _, err := os.Stat(filepath.Join(sctx.WorkDir, ".no-mistakes.yaml")); !os.IsNotExist(err) {
		t.Fatal("expected no .no-mistakes.yaml written when disabled")
	}
}

func TestProposeDiscoveredCommands_NoOpWhenNothingDiscovered(t *testing.T) {
	t.Parallel()
	sctx := proposerContext(t, config.Commands{}, nil)
	before := sctx.Run.HeadSHA
	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sctx.Run.HeadSHA != before {
		t.Fatal("expected no commit when nothing discovered")
	}
}

func TestProposeDiscoveredCommands_IdempotentAcrossReruns(t *testing.T) {
	t.Parallel()
	// The branch file already carries the proposal (as a prior run would have
	// left it); a rerun that rediscovers the same command must not re-propose.
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test ./...",
	})
	if err := os.WriteFile(filepath.Join(sctx.WorkDir, ".no-mistakes.yaml"),
		[]byte("commands:\n  test: go test ./...\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, sctx.WorkDir, "add", "-A")
	gitCmd(t, sctx.WorkDir, "commit", "-m", "seed proposal")
	before := gitCmd(t, sctx.WorkDir, "rev-parse", "HEAD")

	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if after := gitCmd(t, sctx.WorkDir, "rev-parse", "HEAD"); after != before {
		t.Fatal("expected no new commit when branch already carries the proposal")
	}
}

func TestProposeDiscoveredCommands_CommitsOnlyProposalFile(t *testing.T) {
	t.Parallel()
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test ./...",
	})
	// An unrelated dirty file must NOT be bundled into the proposal commit.
	if err := os.WriteFile(filepath.Join(sctx.WorkDir, "unrelated.txt"), []byte("dirty"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	files := gitCmd(t, sctx.WorkDir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD")
	if files != ".no-mistakes.yaml" {
		t.Fatalf("proposal commit changed %q, want only .no-mistakes.yaml", files)
	}
	if status := gitStatusPorcelain(t, sctx.WorkDir); !strings.Contains(status, "unrelated.txt") {
		t.Fatalf("expected unrelated.txt to remain uncommitted, status=%q", status)
	}
}

func TestProposeDiscoveredCommands_NeverWritesTrustBoundaryKeys(t *testing.T) {
	t.Parallel()
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test ./...",
	})
	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(sctx.WorkDir, ".no-mistakes.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "allow_repo_commands") {
		t.Fatalf("proposer must never write allow_repo_commands:\n%s", text)
	}
	if strings.Contains(text, "agent:") {
		t.Fatalf("proposer must never write an agent selector:\n%s", text)
	}
}

func TestProposedCommandsNote_PresentForPendingAbsentWhenMerged(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	if err := os.WriteFile(filepath.Join(dir, ".no-mistakes.yaml"),
		[]byte("commands:\n  test: go test ./...\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Pending: effective config has no test command, branch file does.
	pending := newTestContext(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, config.Commands{})
	note := proposedCommandsNote(pending)
	if !strings.Contains(note, "## Proposed Commands") {
		t.Fatalf("expected proposed-commands note, got %q", note)
	}
	if !strings.Contains(note, "commands.test") || !strings.Contains(note, "go test ./...") {
		t.Fatalf("note missing the proposed command: %q", note)
	}
	if !strings.Contains(note, "after this PR merges") {
		t.Fatalf("note should explain merge-to-take-effect semantics: %q", note)
	}

	// Merged: the effective (trusted) config already carries it, so nothing is pending.
	merged := newTestContext(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, config.Commands{Test: "go test ./..."})
	if note := proposedCommandsNote(merged); note != "" {
		t.Fatalf("expected no note once command is in effective config, got %q", note)
	}
}

func TestProposeDiscoveredCommands_DoesNotExecuteWithinRun(t *testing.T) {
	t.Parallel()
	// The proposal must not become executed config within the discovering run:
	// the effective config the run drives on stays unchanged (commands are only
	// executed from the trusted default-branch read, unaffected here).
	sctx := proposerContext(t, config.Commands{}, map[config.CommandField]string{
		config.CommandFieldTest: "go test ./...",
	})
	if err := proposeDiscoveredCommands(sctx); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sctx.Config.Commands.Test != "" {
		t.Fatalf("proposer mutated the run's effective commands.test to %q; it must stay unset", sctx.Config.Commands.Test)
	}
}

func TestProposedCommandsNote_EmptyWithoutBranchFile(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	sctx := newTestContext(t, &mockAgent{name: "test"}, dir, baseSHA, headSHA, config.Commands{})
	if note := proposedCommandsNote(sctx); note != "" {
		t.Fatalf("expected empty note without branch .no-mistakes.yaml, got %q", note)
	}
}
