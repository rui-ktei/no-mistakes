## 1. Ticket resolution

- [x] 1.1 Add a helper in `internal/pipeline/steps` that reads `base..HEAD` commit subjects oldest-first (via the `git` package) and returns the first that matches `ticket_prefix_pattern`, skipping gate-authored subjects (`no-mistakes...` / `<id>: no-mistakes...`).
- [x] 1.2 Add `resolveTicket(sctx, prTitle string)` that applies the branch -> PR-title -> first-non-gate-commit precedence using `conventional.ExtractTicket`, returning "" when the pattern is empty or nothing matches.
- [x] 1.3 Rewrite `fixCommitTicket(sctx)` to call `resolveTicket` (PR title empty for the non-CI paths), keeping the existing nil-guards on `sctx`/`sctx.Config`.

## 2. Commit subject format

- [x] 2.1 Change `deterministicFixCommitMessage` so the base subject is always `no-mistakes(<step>): <summary>` and the resolved id is prepended as `<id>: <base>` when present.
- [x] 2.2 Change `fixedFixCommitMessage` so the base subject is always `no-mistakes: <text>` and the resolved id is prepended when present.
- [x] 2.3 Ensure a subject already leading with the resolved id is not prefixed twice.

## 3. PR title and CI path

- [x] 3.1 Update `pr.go` to source the title's ticket via `resolveTicket` (so an unticketed branch with a ticketed commit still yields a `<id>:` title), keeping `ApplyTicketPrefix`'s Conventional-prefix stripping.
- [x] 3.2 Thread the PR title into the CI fix-commit path (`ci_fix.go` / `commitAndPush`) so CI fix commits can resolve the id from the PR title when branch and commits lack it.

## 4. Tests

- [x] 4.1 Update `internal/conventional/*_test.go` and `internal/pipeline/steps/ticket_commit_test.go` for the new `<id>: no-mistakes(...)` format.
- [x] 4.2 Unit-test `resolveTicket` precedence: branch-only, commit-only, PR-title-only, branch-wins, none, pattern-empty.
- [x] 4.3 Unit-test the author-commit scan: oldest-first first-match and skipping gate-authored subjects.
- [x] 4.4 Add/extend an e2e test for a run whose branch lacks a ticket but whose author commit carries `WEB-####`, asserting gate commits and the PR title lead with the id.

## 5. Verification

- [x] 5.1 `gofmt -w .` and `make lint`.
- [x] 5.2 `go test -race ./...`.
- [x] 5.3 `make e2e` (touches the gate's commit/PR output and recorded fixtures).
- [x] 5.4 `openspec validate ticket-prefix-from-commit-and-pr --strict`.
