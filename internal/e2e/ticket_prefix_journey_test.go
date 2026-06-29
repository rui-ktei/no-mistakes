//go:build e2e

package e2e

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

// TestTicketPrefixFromAuthorCommit drives a run whose branch name carries no
// work-item id but whose author commit subject does, with ticket_prefix_pattern
// configured. The gate must resolve the id from the commit and lead its
// authored commit subject with it.
func TestTicketPrefixFromAuthorCommit(t *testing.T) {
	h := NewHarness(t, SetupOpts{Agent: "claude", Scenario: ticketPrefixScenario(t)})

	if out, err := h.Run("init"); err != nil {
		t.Fatalf("nm init: %v\n%s", err, out)
	}

	const branch = "fix-empty-release"
	h.CommitChange(branch, "release.txt", "release fix\n", "WEB-12345: fix empty releases")
	repoCfg := "ignore_patterns:\n  - '*.generated.go'\n  - 'vendor/**'\nticket_prefix_pattern: 'WEB-\\d+'\n"
	originalHead := h.CommitChange(branch, ".no-mistakes.yaml", repoCfg, "configure ticket prefix")

	h.PushToGate(branch)
	run := h.WaitForRun(branch, 60*time.Second)
	if run.Status != types.RunCompleted {
		t.Fatalf("ticket-prefix run did not complete: status=%s error=%v", run.Status, deref(run.Error))
	}
	if run.HeadSHA == originalHead {
		t.Fatalf("expected push step to add a gate commit, head remained %s", run.HeadSHA)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	subjects, err := h.runGit(ctx, h.UpstreamDir, "log", "--format=%s", "main..refs/heads/"+branch)
	if err != nil {
		t.Fatalf("read %s upstream commit subjects: %v\n%s", branch, err, subjects)
	}

	sawGateCommit := false
	for _, subject := range strings.Split(strings.TrimSpace(string(subjects)), "\n") {
		subject = strings.TrimSpace(subject)
		if !strings.Contains(subject, "no-mistakes") {
			continue
		}
		sawGateCommit = true
		if !strings.HasPrefix(subject, "WEB-12345: no-mistakes") {
			t.Fatalf("gate commit %q must lead with the resolved id %q", subject, "WEB-12345")
		}
	}
	if !sawGateCommit {
		t.Fatalf("expected at least one gate-authored commit on %s, got subjects:\n%s", branch, strings.TrimSpace(string(subjects)))
	}
}

func ticketPrefixScenario(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.yaml")
	content := `actions:
  - match: "branch: fix-empty-release"
    text: "agent edited a file"
    edits:
      - path: "ticket-edit.txt"
        new: "agent edited\n"
    structured:
      findings: []
      summary: "no issues found"
      risk_level: low
      risk_rationale: "agent edit is deterministic"
      tested:
        - "fakeagent: simulated test run"
      testing_summary: "simulated tests passed"
      artifacts: []
  - text: "no issues found"
    structured:
      findings: []
      summary: "no issues found"
      risk_level: low
      risk_rationale: "no risks detected in the diff"
      tested:
        - "fakeagent: simulated test run"
      testing_summary: "simulated tests passed"
      title: "feat: fakeagent change"
      body: "## Summary\nfakeagent canned PR body"
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write ticket-prefix scenario: %v", err)
	}
	return path
}
