## 1. Capture the canonical command from discovery agents

- [x] 1.1 Add an optional `canonical_command` field to `testFindingsSchema` and to the parsed `Findings`/result struct in `internal/pipeline/steps/test.go`; update the evidence/discovery agent prompt to fill it with the exact reproducible test command or leave it empty when none is confidently canonical.
- [x] 1.2 Add optional `canonical_command` (lint) and `canonical_format_command` fields to `findingsSchema` and the parsed struct in `internal/pipeline/steps/lint.go`; update the discovery agent prompt to report the linter and the formatter it ran as two distinct commands, leaving the format field empty when no distinct formatter ran.
- [x] 1.3 Add a non-executing carrier for captured commands to `pipeline.StepOutcome` (`DiscoveredCommands map[config.CommandField]string`) and populate it from test/lint discovery paths only when the field was unset. The executor merges each outcome into a per-run `pipeline.RunScratch` threaded via `StepContext.Scratch`.
- [x] 1.4 Unit-test that the schemas accept/expose the new field and that an empty/absent canonical command yields no captured proposal.

## 2. Config toggle

- [x] 2.1 Add a `propose_commands` setting (default true) to the config types: global default in `GlobalConfig`, overridable by the repo's trusted default-branch `.no-mistakes.yaml` (scoped like `allow_repo_commands`, never read from the pushed branch), following existing precedence in `internal/config/config.go`.
- [x] 2.2 Add it to `defaultConfigYAML` and keep it in sync with the Go default; extend the `TestDefaultConfigYAML_MatchesGoDefaults` guard.
- [x] 2.3 Expose a helper to answer "which `commands.*` fields are unset in the effective config" for the proposer (`Commands.UnsetCommandFields`, plus `Commands.Get`).

## 3. Surgical .no-mistakes.yaml editing

- [x] 3.1 Implement a comment/order-preserving edit (via `yaml.Node`) that sets only the given `commands.*` keys in a repo's `.no-mistakes.yaml`, creating the `commands:` block if absent, leaving unrelated content byte-stable where possible (`config.ProposeCommandsInFile`).
- [x] 3.2 Implement a reader that returns the `commands.*` values currently present in the *branch working-tree* `.no-mistakes.yaml` (distinct from effective config), for the no-duplicate check. Reused the existing `config.LoadRepo(workDir).Commands` (reads the pushed working-tree file), rather than adding a redundant reader.
- [x] 3.3 Unit-test the editor: comments and unrelated keys survive; existing values are not overwritten; new `commands:` block is created correctly.

## 4. Command proposal action

Design refinement (discovered during implementation): rather than a 10th first-class ordered pipeline step - which would force churn in `types.Order()`/`AllSteps()`, the e2e "exactly 9 steps in order" assertion, and TUI backfill, and would surface a near-instant no-op step on the vast majority of runs (repos with commands already configured) - the proposer is implemented as an isolated, unit-tested action (`proposeDiscoveredCommands` in `command_proposal.go`) invoked at the top of `PushStep.Execute`, before it stages/commits/pushes. This preserves every observable spec behavior (dedicated commit, PR note, trust boundary, unset-only, per-branch idempotence) and the design's real intent (localized testing, push stays single-purpose via a single call), while keeping the pipeline at 9 steps.

- [x] 4.1 Implement `proposeDiscoveredCommands` and invoke it after test/lint and immediately before the push operation (top of `PushStep.Execute`).
- [x] 4.2 Compute proposable fields: unset in effective config AND not already equal in the branch file AND backed by a non-empty canonical command; no-op when none (`proposableCommands`).
- [x] 4.3 When enabled and there is something to propose, apply the surgical edit and commit it on the branch (`commitProposedCommands`, staging only `.no-mistakes.yaml`). The PR body note is derived from the branch file vs effective config (`pendingProposedCommands`), which stays accurate across reruns without threading run state to the PR step.
- [x] 4.4 Ensure the action never rewrites history, never sets `allow_repo_commands`, never touches the trusted SHA, and stages only `.no-mistakes.yaml` so it cannot bundle unrelated local changes; the resulting commit rides the branch through `PushStep`'s existing `resolveForcePushDecision`-guarded push (never bypassed).
- [x] 4.5 No-op cleanly when the feature is disabled.

## 5. PR surfacing

- [x] 5.1 In `internal/pipeline/steps/pr.go`, include a short note in the PR body listing the proposed `commands.*` and stating they take effect after merge to the default branch.
- [x] 5.2 Test that the PR body includes the note when a proposal was made and omits it otherwise.

## 6. Trust-boundary regression coverage

- [x] 6.1 Unit/integration test: a proposed command is NOT executed verbatim within the discovering run (proposer leaves the run's effective `commands.*` unset - `TestProposeDiscoveredCommands_DoesNotExecuteWithinRun`).
- [x] 6.2 Test: with the proposal only on the feature branch (not merged), a later run's trusted read still reports the field unset and rediscovers (`TestEffectiveRepoConfig_UnmergedProposalNotExecuted`).
- [x] 6.3 Test: proposer never enables `allow_repo_commands` and never reads/writes the trusted default branch (`TestProposeDiscoveredCommands_NeverWritesTrustBoundaryKeys`; the action only ever touches the worktree `.no-mistakes.yaml`).
- [x] 6.4 Test the per-branch idempotence: a rerun on the same branch does not create a duplicate proposal commit (`TestProposeDiscoveredCommands_IdempotentAcrossReruns`).

## 7. End-to-end

- [x] 7.1 E2E (behind the `e2e` build tag): a run on a repo with `commands.test` unset produces a branch commit adding `commands.test` to `.no-mistakes.yaml` (pushed upstream), but does not execute it in that run (`TestCommandProposalJourney_ProposesButDoesNotExecute`). PR-note surfacing is asserted in the unit tests (`TestProposedCommandsNote_*`), since the e2e file:// origin skips PR creation.
- [x] 7.2 E2E: after merging the proposal to the default branch, the next run reads and executes the command from trusted config and skips rediscovery (`TestCommandProposalJourney_ExecutesAfterMerge`).

## 8. Docs and agent-guidance surfaces

- [x] 8.1 No driving-surface change needed: propose-on-discovery is automatic and adds no gate or `axi` driving action, so the skill body and live `axi` strings are unchanged (`make lint` skill-check confirms no drift). Behavior is documented in the reference docs instead (8.2).
- [x] 8.2 Documented propose-on-discovery and the "takes effect after merge" trust semantics in `docs/reference/repo-config.md` (`propose_commands` field + notes on `commands.{test,lint,format}`), `docs/reference/global-config.md` (`propose_commands` field + example), and `docs/reference/pipeline-steps.md` (push step behavior).
- [x] 8.3 Ran the safe verification sequence: `gofmt`, `make lint`, `go test -race ./...`, and `make e2e` - all green.
