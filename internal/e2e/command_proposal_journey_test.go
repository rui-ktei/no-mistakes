//go:build e2e

package e2e

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

const testDiscoveryPromptMarker = "You are validating a code change by testing it"

// commandProposalScenario writes a fakeagent scenario where the lint and test
// discovery agents report canonical commands, so the run proposes them. testCmd
// is the canonical test command the evidence agent reports.
func commandProposalScenario(t *testing.T, testCmd string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	content := fmt.Sprintf(`actions:
  - match: "Detect the linting and formatting tools"
    text: "lint clean"
    structured:
      findings: []
      summary: "lint clean"
      canonical_command: "golangci-lint run"
      canonical_format_command: "gofmt -w ."
  - match: %q
    text: "tested"
    structured:
      findings: []
      summary: "tested"
      tested: ["ran the suite"]
      testing_summary: "all pass"
      artifacts: []
      canonical_command: %q
  - match: ""
    text: "no issues found"
    structured:
      findings: []
      summary: "no issues found"
      risk_level: low
      risk_rationale: "no risks detected"
      tested: ["fakeagent: simulated test run"]
      testing_summary: "simulated tests passed"
      title: "feat: fakeagent change"
      body: "## Summary\nfakeagent canned PR body"
`, testDiscoveryPromptMarker, testCmd)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write scenario: %v", err)
	}
	return path
}

// TestCommandProposalJourney_ProposesButDoesNotExecute covers task 7.1: a run
// on a repo with commands.test unset on the trusted default branch produces a
// branch commit adding commands.test to .no-mistakes.yaml (pushed upstream),
// but does NOT execute the discovered command within that run.
func TestCommandProposalJourney_ProposesButDoesNotExecute(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed-7-1")
	testCmd := fmt.Sprintf("echo EXECUTED > %s", marker)
	optOut := false
	h := NewHarness(t, SetupOpts{
		Agent:             "claude",
		Scenario:          commandProposalScenario(t, testCmd),
		AllowRepoCommands: &optOut,
	})
	if out, err := h.Run("init"); err != nil {
		t.Fatalf("nm init: %v\n%s", err, out)
	}

	branch := "propose-commands"
	h.CommitChange(branch, branch+".txt", "change to gate\n", "add "+branch+" change")
	h.PushToGate(branch)

	run := h.WaitForRun(branch, 120*time.Second)
	if run.Status != types.RunCompleted {
		t.Fatalf("run did not complete: status=%s error=%v", run.Status, deref(run.Error))
	}

	// The proposed command must NOT have run within the discovering run.
	if _, err := os.Stat(marker); err == nil {
		t.Fatalf("proposed command executed within the discovering run (marker %s exists); a proposal must be inert until merged", marker)
	}

	// The proposal must be committed on the branch and pushed upstream.
	ctx := context.Background()
	out, err := h.runGit(ctx, h.UpstreamDir, "show", branch+":.no-mistakes.yaml")
	if err != nil {
		t.Fatalf("read pushed .no-mistakes.yaml: %v", err)
	}
	yaml := string(out)
	if !strings.Contains(yaml, "commands:") || !strings.Contains(yaml, testCmd) {
		t.Fatalf("pushed .no-mistakes.yaml is missing the proposed commands.test:\n%s", yaml)
	}
	if !strings.Contains(yaml, "golangci-lint run") {
		t.Fatalf("pushed .no-mistakes.yaml is missing the proposed commands.lint:\n%s", yaml)
	}

	// The test step used the discovery path (agent), since no command was configured.
	if !anyInvocationContains(h.AgentInvocations(), testDiscoveryPromptMarker) {
		t.Fatal("expected the test discovery agent to run when commands.test is unset")
	}
}

// TestCommandProposalJourney_ExecutesAfterMerge covers task 7.2: once the
// proposed command is on the trusted default branch, the next run reads it from
// the trusted config and executes it, skipping agent rediscovery.
func TestCommandProposalJourney_ExecutesAfterMerge(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "executed-7-2")
	testCmd := fmt.Sprintf("echo EXECUTED > %s", marker)
	optOut := false
	h := NewHarness(t, SetupOpts{
		Agent:             "claude",
		Scenario:          commandProposalScenario(t, testCmd),
		AllowRepoCommands: &optOut,
	})
	if out, err := h.Run("init"); err != nil {
		t.Fatalf("nm init: %v\n%s", err, out)
	}

	// Simulate the human merge: the proposed command now lives on the trusted
	// default branch (main), so it becomes executed config.
	ctx := context.Background()
	mergedConfig := fmt.Sprintf("ignore_patterns:\n  - 'vendor/**'\nallow_repo_commands: false\ncommands:\n  test: %q\n", testCmd)
	h.CommitChange("main", ".no-mistakes.yaml", mergedConfig, "merge: pin discovered test command")
	if out, err := h.runGit(ctx, h.WorkDir, "push", "origin", "main"); err != nil {
		t.Fatalf("push main: %v\n%s", err, out)
	}

	branch := "consume-merged-command"
	h.CommitChange(branch, branch+".txt", "change to gate\n", "add "+branch+" change")
	h.PushToGate(branch)

	run := h.WaitForRun(branch, 120*time.Second)
	if run.Status != types.RunCompleted {
		t.Fatalf("run did not complete: status=%s error=%v", run.Status, deref(run.Error))
	}

	// The trusted command executed this run.
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("trusted commands.test did not execute (marker %s missing); a merged proposal must run", marker)
	}

	// Rediscovery is skipped: with commands.test configured, the test discovery
	// agent must not run.
	if anyInvocationContains(h.AgentInvocations(), testDiscoveryPromptMarker) {
		t.Fatal("test discovery agent ran even though commands.test is configured on the trusted branch; rediscovery should be skipped")
	}
}
