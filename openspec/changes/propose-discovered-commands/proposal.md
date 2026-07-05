## Why

When a repo has no `commands.test` / `commands.lint` / `commands.format` configured, every run spends agent tokens rediscovering how to test, lint, and format the project - the same discovery, repeated indefinitely.
The obvious fix - "let the agent write what it discovered into `.no-mistakes.yaml`" - is unsafe here: those three fields are code-executing selectors run verbatim via `sh -c` with the maintainer's credentials, so they are deliberately read only from the trusted default branch, never the pushed SHA.
This change lets a run *propose* discovered commands as a reviewable commit on the branch, so a human merge to the default branch is what promotes them to trusted, executed config - saving rediscovery tokens on every subsequent run without opening a supply-chain path.

## What Changes

- The lint, test, and format discovery agents gain a structured output field that reports the single canonical command they used (e.g. `go test -race ./...`), distinct from the free-form `tested`/findings text.
- A new pipeline step (or an extension to the existing push/PR flow) collects the canonical commands discovered during a run and, for each currently-unset `commands.*` field, writes a proposed value into the branch's `.no-mistakes.yaml` as a dedicated commit.
- The proposed commit is surfaced in the PR (commit subject plus PR-body note) so the reviewer understands no-mistakes is pinning discovered commands and can accept, edit, or drop them by editing the file before merge.
- Proposals are only ever written for fields that are currently empty in the *effective* config; a field already set (on the default branch, or via `allow_repo_commands`) is never overwritten or re-proposed.
- Proposal writing is opt-outable via config (default on) and is a no-op when there is nothing new to propose, when the branch already contains an equivalent pending proposal, or when the discovery produced no confidently-canonical command.
- No change to how `commands.*` are *read* or *executed*: the trusted-default-branch read path and the `sh -c` verbatim runner are untouched. Discovered strings never become executed config within the run that discovered them; they only take effect after a human merges them to the default branch.

## Capabilities

### New Capabilities
- `command-discovery-persistence`: Defines how a run captures the canonical test/lint/format command an agent discovers, proposes each currently-unset command as a reviewable edit to the branch's `.no-mistakes.yaml`, surfaces it in the PR, and guarantees a proposed command becomes trusted/executed config only through a human-ratified merge to the default branch - never auto-applied on the pushed branch.

### Modified Capabilities
<!-- No existing spec's requirements change; the trust-boundary behavior in the repo config path is preserved, not modified. -->

## Impact

- `internal/config`: `Commands` struct read path unchanged; add awareness of "which command fields are unset in the effective config" for the proposer, and a config toggle for the feature.
- `internal/pipeline/steps/test.go`, `lint.go`, `push.go` (format): extend the discovery agents' JSON schemas (`testFindingsSchema`, `findingsSchema`) with a canonical-command field and thread the captured value out of the step.
- `internal/pipeline` + a new step (proposer): serialize the proposal into `.no-mistakes.yaml`, commit it on the branch, and record it for the PR body. Must run before push/PR and respect the force-push and diff-base invariants.
- `internal/pipeline/steps/pr.go`: include the proposal note in the PR body.
- Trust boundary (`config.EffectiveRepoConfig`, `daemon.loadTrustedRepoConfig`): unchanged - the proposer writes to the pushed branch only; the reader still trusts the default branch only.
- Agent-guidance surfaces (skill body, live `axi` strings, `docs/`): document the propose-on-discovery behavior and that it takes effect after merge.
- Tests: new unit tests for canonical-command capture and the proposer's unset-only / no-duplicate / never-overwrite logic; e2e coverage that a discovered command appears as a branch commit and PR note but does not execute until merged to default.
