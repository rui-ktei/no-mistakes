## Context

`commands.{test,lint,format}` are code-executing selectors: the daemon runs them verbatim through `sh -c` with the maintainer's credentials.
To block a supply-chain RCE, `config.EffectiveRepoConfig` reads those fields only from the trusted default-branch copy of `.no-mistakes.yaml` (resolved to a pinned SHA in `daemon.loadTrustedRepoConfig`), never from the pushed branch - unless the maintainer has opted in with `allow_repo_commands`.

When a field is unset, the step falls back to an agent:
`internal/pipeline/steps/test.go` runs an evidence/discovery agent (`testFindingsSchema`) that reports a free-form `tested []string`;
`internal/pipeline/steps/lint.go` runs a discovery agent (`findingsSchema`) that detects and runs linters/formatters and reports findings only.
Neither reports the single command it settled on, and there is no standalone *format* discovery agent - `push.go` runs `Commands.Format` only when configured, and the lint agent is the one told to apply formatter fixes.

`.no-mistakes.yaml` is parsed by `config.LoadRepo` via `yaml.Unmarshal`. Agent-produced working-tree changes are committed on the branch by `commitAgentFixes` (`internal/pipeline/steps/common_fix.go`). Pushes go through `PushStep`, guarded by the force-push lease in `forcepush.go`.

This change captures the discovered command and proposes it as a reviewable branch edit, letting the human merge be the act that promotes it to trusted, executed config.

## Goals / Non-Goals

**Goals:**
- Capture the single canonical command a test/lint discovery agent used, as structured output.
- Propose that command into the branch's `.no-mistakes.yaml`, but only for fields unset in the trusted/effective config and not already proposed on the branch.
- Surface the proposal in the branch commit and the PR body so a human reviews it.
- Guarantee a proposed command is executed verbatim only after it reaches the default branch through the existing trusted-config read path.
- Preserve the existing `.no-mistakes.yaml` content (comments, key order, unrelated fields) when editing.
- Default the behavior on, with a config switch to disable it.

**Non-Goals:**
- No change to how `commands.*` are read or executed. The trusted read path and `sh -c` runner are untouched.
- No local cross-run command cache (that is the separate "Design B" hint-cache idea; out of scope here).
- No auto-enabling of `allow_repo_commands` and no execution of a proposed command within the discovering run.
- No standalone format discovery agent is introduced; the format command is sourced from the existing lint discovery agent (which already applies formatter fixes), and is only proposed when that agent reports a confidently-canonical format command.

## Decisions

### Capture the canonical command via a dedicated schema field
Add an optional `canonical_command` (test) and `canonical_command` / optional `canonical_format_command` (lint) field to `testFindingsSchema` and `findingsSchema`, with prompt instructions to fill it with the exact reproducible command used, or leave it empty when no single command is confidently canonical.
Thread the captured value out of the step through the `StepOutcome` (a new non-executing field) so the proposer can read it.
Alternative considered: parse it out of the free-form `tested` array. Rejected - `tested` is prose-ish, multi-entry, and unreliable to machine-extract; an explicit field is testable and unambiguous.

### A dedicated proposer action, run after discovery and before push
Implemented as `proposeDiscoveredCommands` (in `command_proposal.go`), invoked at the top of `PushStep.Execute` - after test/lint (so canonical commands exist) and immediately before push stages/commits/pushes.
It reads the run's accumulated canonical commands (from `StepContext.Scratch.DiscoveredCommands`, populated by the test/lint steps and merged by the executor), computes which `commands.*` are proposable, edits `.no-mistakes.yaml` in the worktree, and commits it as a dedicated commit (staging only that file) so the commit rides the branch through the normal lease-guarded push.

**Refinement from the original "dedicated `CommandProposalStep`" decision (discovered during implementation):** a 10th first-class ordered step would force churn in `types.Order()`/`AllSteps()`, the e2e "exactly 9 steps in order" assertion, and TUI backfill, and would surface a near-instant no-op step on the vast majority of runs (repos with commands already configured, or nothing to propose). The design's real motivation for a distinct step - localized testing and keeping push single-purpose - is preserved by isolating all proposer logic in its own file with dedicated unit tests and invoking it via a single call. Every observable spec behavior (dedicated commit, PR note, trust boundary, unset-only, per-branch idempotence) is unchanged.
Alternative considered: a visible ordered `CommandProposalStep`. Rejected for the churn/UX reasons above. Alternative considered: fold the logic inline into `PushStep`'s body. Rejected - a separate file/function keeps testing localized and push single-purpose.

### "Proposable" = unset in trusted/effective config AND not already on the branch file
Two independent checks, both required:
1. The field is empty in the effective config (so we never overwrite a maintainer's trusted value, and never fight a value already merged to default).
2. The field is not already present with an equal value in the *branch working-tree* `.no-mistakes.yaml`.
Check 2 is essential because `commands.*` are read from the default branch, so a value the proposer wrote to the branch last run is still "unset" in the effective config - without check 2 we would re-propose it on every rerun of the same branch. Detection reads the worktree file directly, not the effective config.

### Edit `.no-mistakes.yaml` surgically, preserving surrounding content
Use a comment-preserving `yaml.Node` round-trip (or a targeted insertion under an existing/created `commands:` block) rather than `yaml.Marshal` of the whole `RepoConfig`, which would drop comments, reorder keys, and materialize defaults.
The edit sets only the proposable command keys and leaves everything else byte-stable where possible.

### Trust boundary is structurally preserved, not merely by convention
The proposer only ever writes to the pushed branch worktree. It never writes to the default branch, never resolves or mutates the trusted SHA, and never sets `allow_repo_commands`.
Because `EffectiveRepoConfig` continues to read `commands.*` from the trusted default branch only, the proposed value is inert until a human merges it - the merge is the ratification. This is enforced by the ordering and by the proposer having no path to the trusted-config read.

### Surface in the PR body
`pr.go` prepends a short "## Proposed Commands" note listing the proposed commands (and that they take effect after merge). It is derived from the branch working-tree `.no-mistakes.yaml` minus the effective config (`pendingProposedCommands`), not from run state, so it stays accurate across reruns (a prior run's proposal already on the branch is still noted) and needs no cross-step state threaded to the PR step. Placed at the top of the body so it survives length-budget truncation.

### Configurability
Add a boolean config `propose_commands` (default true) with a **global default plus trusted-repo override** scope: the global `~/.no-mistakes/config.yaml` sets the default, and a repo's `.no-mistakes.yaml` read from the **trusted default branch** may override it. It is NOT read from the pushed branch, mirroring how `allow_repo_commands` is scoped, so a contributor's branch cannot flip it. This toggle is not itself code-executing (it only decides whether a proposal commit is written, never what runs), so the trusted-default-branch read is a consistency choice, not a strict RCE requirement. When false, canonical capture may still occur internally but the proposer is a no-op. Keep `defaultConfigYAML` and the Go default in sync (mirroring the `ci_timeout` convention and its `TestDefaultConfigYAML_MatchesGoDefaults` guard).

## Risks / Trade-offs

- **Re-proposing across reruns on the same branch** → the branch-file presence check (Decision 3, check 2) makes the proposer idempotent per branch.
- **Editing `.no-mistakes.yaml` clobbers maintainer formatting/comments** → surgical `yaml.Node` edit; add a test asserting unrelated keys and comments survive.
- **Proposal commit interacts badly with rebase / force-push safety** → the proposer runs after rebase and adds a normal commit; it does not rewrite history, must not bypass `resolveForcePushDecision`, and must not bundle unrelated local commits. Reuse the established commit helper and let `PushStep` push as usual.
- **Contributor plants a malicious "test" script the agent discovers and proposes** → the proposal is inert on the branch; it only executes after a human reviews the PR diff and merges it to the default branch. The human merge is exactly the trust gate the design relies on. The proposal note in the PR makes the pinned command visible for that review.
- **A discovered command is noisy/wrong** → it is a proposal, editable or removable in the PR before merge; the merged value (edited or not) is what future runs read.
- **Format has no dedicated discovery agent** → `commands.format` is captured from the lint discovery agent, which already runs formatters. The lint agent prompt is extended to report the canonical format command it used (separate from the lint command); when the two coincide or no distinct formatter was run, the format field is empty and simply not proposed. This keeps format in scope without a new agent and with no behavior regression.
- **Worktree is detached-HEAD** → committing on the branch follows the same path `commitAgentFixes` already uses for agent fixes in that worktree, so no new detached-HEAD handling is introduced.

## Migration Plan

- Additive only: new optional schema fields, a new step, a new default-on config toggle. No DB migration required unless the toggle is persisted per-repo (if so, add an additive `NOT NULL DEFAULT` column mirroring `base_branch`).
- Rollback: disable via config (`propose_commands: false`) or remove the step from the pipeline assembly; the read/execute path is unchanged, so disabling fully reverts behavior.
- Update the three agent-guidance surfaces (skill body, live `axi` strings, `docs/`) to describe propose-on-discovery and the "takes effect after merge" semantics, keeping them in sync per the repo's guidance-surface rule.

## Open Questions

- One commit per run for all proposed fields, or one per field? Proposed: a single commit for all proposable fields to keep branch history clean.
