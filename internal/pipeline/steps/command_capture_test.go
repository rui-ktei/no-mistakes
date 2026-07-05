package steps

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/agent"
	"github.com/kunchenguid/no-mistakes/internal/config"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

func TestFindings_CanonicalCommandRoundTrip(t *testing.T) {
	t.Parallel()
	raw := `{"findings":[],"summary":"ok","canonical_command":"go test ./...","canonical_format_command":"gofmt -w ."}`
	got, err := types.ParseFindingsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.CanonicalCommand != "go test ./..." {
		t.Errorf("CanonicalCommand = %q", got.CanonicalCommand)
	}
	if got.CanonicalFormatCommand != "gofmt -w ." {
		t.Errorf("CanonicalFormatCommand = %q", got.CanonicalFormatCommand)
	}
	back, err := types.MarshalFindingsJSON(got)
	if err != nil {
		t.Fatal(err)
	}
	round, err := types.ParseFindingsJSON(back)
	if err != nil {
		t.Fatal(err)
	}
	if round.CanonicalCommand != got.CanonicalCommand || round.CanonicalFormatCommand != got.CanonicalFormatCommand {
		t.Errorf("canonical fields lost across marshal round-trip: %+v", round)
	}
}

func TestLintStep_DiscoveryCapturesCanonicalCommands(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(
				`{"findings":[],"summary":"clean","canonical_command":"golangci-lint run","canonical_format_command":"gofmt -w ."}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})

	outcome, err := (&LintStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := outcome.DiscoveredCommands[config.CommandFieldLint]; got != "golangci-lint run" {
		t.Errorf("discovered lint = %q", got)
	}
	if got := outcome.DiscoveredCommands[config.CommandFieldFormat]; got != "gofmt -w ." {
		t.Errorf("discovered format = %q", got)
	}
}

func TestLintStep_DiscoveryEmptyCanonicalYieldsNoProposal(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(`{"findings":[],"summary":"clean"}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})

	outcome, err := (&LintStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.DiscoveredCommands) != 0 {
		t.Errorf("expected no discovered commands, got %v", outcome.DiscoveredCommands)
	}
}

func TestTestStep_DiscoveryCapturesCanonicalCommand(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(
				`{"findings":[],"summary":"ok","tested":["go test ./..."],"testing_summary":"all pass","artifacts":[],"canonical_command":"go test -race ./..."}`)}, nil
		},
	}
	// No configured test command -> evidence agent runs and its canonical command is captured.
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{})

	outcome, err := (&TestStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := outcome.DiscoveredCommands[config.CommandFieldTest]; got != "go test -race ./..." {
		t.Errorf("discovered test = %q", got)
	}
}

func TestTestStep_DiscoverySkippedWhenTestCommandConfigured(t *testing.T) {
	t.Parallel()
	dir, baseSHA, headSHA := setupGitRepo(t)
	gitCmd(t, dir, "checkout", "--detach", headSHA)
	// A configured test command runs deterministically; even if the (intent)
	// evidence agent reports a canonical command, we must not capture it,
	// because the field is already set.
	ag := &mockAgent{
		name: "test",
		runFn: func(ctx context.Context, opts agent.RunOpts) (*agent.Result, error) {
			return &agent.Result{Output: json.RawMessage(
				`{"findings":[],"summary":"ok","tested":[],"testing_summary":"ok","artifacts":[],"canonical_command":"go test -race ./..."}`)}, nil
		},
	}
	sctx := newTestContextWithDBRecords(t, ag, dir, baseSHA, headSHA, config.Commands{Test: "true"})
	sctx.UserIntent = "make the widget render" // forces the evidence agent to run alongside the baseline

	outcome, err := (&TestStep{}).Execute(sctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(outcome.DiscoveredCommands) != 0 {
		t.Errorf("expected no capture when test command is configured, got %v", outcome.DiscoveredCommands)
	}
}
