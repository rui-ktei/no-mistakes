## Why

The gate currently derives the work-item id (e.g. `WEB-12345`) only from the branch name.
When a contributor follows a ticketed convention on their commit subject or the PR title but not the branch name (a common case - branches are often named `fix/...` while the commit reads `WEB-12345: ...`), the gate-authored fix commits fall back to `no-mistakes(<step>): ...` and the PR ends up with a mix of `WEB-12345:` author commits and unticketed `no-mistakes(...)` commits.
That inconsistency is exactly what work-item conventions exist to prevent, and it makes the gate's commits look foreign in the PR history.

## What Changes

- Broaden work-item id resolution beyond the branch name to also consult, in order: the branch name, the PR title (when a PR exists), then the first non-gate commit subject on the branch that carries the id.
- Unify the gate-authored commit subject format so that when a ticket is resolved from any source, the id is **prepended** to the existing message verbatim: `WEB-12345: no-mistakes(<step>): <summary>` for step fixes, and `WEB-12345: no-mistakes: apply agent fixes` / `WEB-12345: no-mistakes: apply CI fixes` for the push and CI fixes.
- **BREAKING (format only):** this supersedes the previous ticket format `WEB-12345: <summary> [no-mistakes/<step>]`. Gate commits now keep their `no-mistakes(<step>):` body and lead with the id, so author and gate commits share one consistent `WEB-12345:` prefix.
- Apply the same resolver to the generated PR title, so a branch without a ticket still produces a `WEB-12345:`-prefixed PR title when the commit subject or PR title carries one.
- When no source carries a ticket, behavior is unchanged: gate commits stay `no-mistakes(<step>): ...` and the PR title stays Conventional Commits, so ticket-less small changes keep working.

## Capabilities

### New Capabilities

- `ticket-prefix`: how the gate resolves the work-item id from the branch, commits, and PR title, and how it applies that id to gate-authored commit subjects and the PR title.

### Modified Capabilities

<!-- none: openspec/specs/ is empty; the prior branch-only behavior was never captured as a spec. -->

## Impact

- `internal/conventional/title.go` - ticket extraction helpers (`ExtractTicket`, `ApplyTicketPrefix`).
- `internal/pipeline/steps/common_fix.go` - `fixCommitTicket`, `deterministicFixCommitMessage`, `fixedFixCommitMessage` (the gate-authored commit subjects).
- `internal/pipeline/steps/pr.go` - PR title construction.
- `internal/pipeline/steps/ci_fix.go`, `push.go` - reuse the unified resolver for their fix commits.
- New stateless git read of `base..HEAD` author commit subjects; no database schema change.
- Config surface (`ticket_prefix_pattern`) is unchanged - the same pattern is now matched against more sources.
