package pipeline

import (
	"strings"
	"sync"

	"github.com/kunchenguid/no-mistakes/internal/config"
)

// HousekeepingLintResult is the lint assessment produced by the combined
// document+lint housekeeping pass: the document step performs both duties in
// one agent invocation and hands the lint half to the lint step so it does
// not pay a second cold agent pass.
type HousekeepingLintResult struct {
	// FindingsJSON holds the lint-category findings (possibly an empty set)
	// in the same JSON shape the lint step produces itself.
	FindingsJSON string
	// Summary is the housekeeping pass's one-line lint summary.
	Summary string
}

// RunShared carries in-memory run-scoped results one step hands to a later
// step in the same run. It lives on the executor for the run's lifetime and
// is never persisted: on any process boundary the consuming step simply
// falls back to doing its own work.
type RunShared struct {
	mu                 sync.Mutex
	housekeepingLint   *HousekeepingLintResult
	discoveredCommands map[config.CommandField]string
}

// RecordDiscoveredCommand stores the non-empty canonical command a discovery
// agent reported for a commands.* field this run (populated by the test and
// lint steps for fields unset in the effective config, consumed by the push
// step's command-proposal path). Safe to call on a nil receiver.
func (s *RunShared) RecordDiscoveredCommand(field config.CommandField, command string) {
	if s == nil || strings.TrimSpace(command) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.discoveredCommands == nil {
		s.discoveredCommands = map[config.CommandField]string{}
	}
	s.discoveredCommands[field] = command
}

// DiscoveredCommands returns a snapshot of the canonical commands discovery
// agents reported this run, keyed by commands.* field. It returns nil when
// nothing was discovered. Safe to call on a nil receiver.
func (s *RunShared) DiscoveredCommands() map[config.CommandField]string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.discoveredCommands) == 0 {
		return nil
	}
	out := make(map[config.CommandField]string, len(s.discoveredCommands))
	for field, command := range s.discoveredCommands {
		out[field] = command
	}
	return out
}

// SetHousekeepingLint records the combined pass's lint assessment for the
// lint step. It replaces any previous assessment (a document fix round
// re-runs the combined pass and re-stashes a fresh result).
func (s *RunShared) SetHousekeepingLint(result HousekeepingLintResult) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.housekeepingLint = &result
}

// ClearHousekeepingLint discards a previous combined-pass lint assessment
// before a document pass starts, so a later lint step never consumes stale
// findings.
func (s *RunShared) ClearHousekeepingLint() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.housekeepingLint = nil
}

// TakeHousekeepingLint returns and consumes the combined pass's lint
// assessment. The second call returns false so a lint fix round re-assesses
// with its own agent pass instead of trusting a stale result.
func (s *RunShared) TakeHousekeepingLint() (HousekeepingLintResult, bool) {
	if s == nil {
		return HousekeepingLintResult{}, false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.housekeepingLint == nil {
		return HousekeepingLintResult{}, false
	}
	result := *s.housekeepingLint
	s.housekeepingLint = nil
	return result, true
}
