## Context

The previous change (`ticket_prefix_pattern`) made the gate prepend a work-item id to its commits and PR title, but only when the **branch name** matched the pattern.
Sympli's convention puts `WEB-####` on the branch, the commit, and the PR title, yet branches are frequently named descriptively (`fix/jira-empty-release`) while the ticket lives on the commit subject or PR title instead.
In that case the gate writes `no-mistakes(<step>): ...` commits onto a PR whose author commits read `WEB-####: ...`, producing a mixed, inconsistent history.

Resolution today is a single call: `conventional.ExtractTicket(branch, pattern)` in `fixCommitTicket` (`common_fix.go`) and in `pr.go` for the title.
`ExtractTicket` already runs the regex against an arbitrary string, so it can be reused against any candidate source.

## Goals / Non-Goals

**Goals:**
- Resolve the work-item id from the branch, the author commits, and the PR title, not just the branch.
- One consistent gate-commit subject format that leads with the id and keeps the `no-mistakes(...)` provenance.
- Keep ticket-less changes working exactly as today.
- No database schema change; resolution stays stateless and cheap.

**Non-Goals:**
- Changing the `ticket_prefix_pattern` config surface or its global/per-repo layering.
- Inferring or inventing a ticket when none of the sources carries one.
- Changing author commits; the gate only formats the commits and title it authors.

## Decisions

### Decision 1: Prepend the id, keep the `no-mistakes(...)` body (supersedes the `[no-mistakes/<step>]` form)

Gate commits become `<id>: <existing message>`: `WEB-12345: no-mistakes(document): <summary>`, `WEB-12345: no-mistakes: apply CI fixes`.
This is the user's stated target and is the simplest consistent rule: build the existing `no-mistakes(<step>): <summary>` / `no-mistakes: <text>` subject, then prepend `<id>: ` when an id is resolved.

Alternatives considered:
- Keep the prior `WEB-12345: <summary> [no-mistakes/<step>]` form. Rejected: it drops the `no-mistakes(<step>)` body the user wants to keep and leaves two different formats in the tree depending on which change shipped.
- Two formats (branch source keeps the old form, new sources use the new form). Rejected: the explicit requirement is consistency.

This changes the output of the already-merged branch-only behavior. It is a format change to gate-authored commits only; no consumer parses these subjects, so the blast radius is cosmetic. The existing `conventional` tests that assert the old form will be updated.

### Decision 2: Resolution order branch -> PR title -> first non-gate commit, stateless

First match wins, in that order. Branch stays first so a contributor can pin the id explicitly by naming the branch, and so existing branch-named-ticket behavior is unchanged. The PR title is consulted next, when `sctx.Run.PRURL` is set; in the fixed pipeline order (`... push -> pr -> ci`) that makes the PR-title source reachable for CI-step fix commits but not the earlier steps, so for those steps resolution simply falls through to the commit. The commit source is the first subject in `base..HEAD` (oldest first) not authored by the gate that carries a match; gate-authored subjects (those beginning with `no-mistakes` or already `<id>: no-mistakes`) are skipped.

Alternatives considered:
- Commit before PR title. Rejected: the team's precedence is branch > PR title > commit, treating the human-curated PR title as more authoritative than an individual commit subject.
- Resolve once at run start and persist on the run row. Rejected: needs a schema/migration and the run-start moment cannot see the PR title anyway; the stateless read is cheap and avoids migration.

### Decision 3: Reuse one resolver for commits and the PR title

A single `resolveTicket(sctx)` (with an optional PR-title candidate) feeds both `fixCommitTicket` and the PR-title path in `pr.go`, so the branch/commit/PR-title precedence is identical for commit subjects and the title. `ApplyTicketPrefix` keeps its Conventional-prefix-stripping behavior for the PR title; the commit path just prepends.

## Risks / Trade-offs

- [Author-commit scan picks the wrong id when a branch legitimately spans multiple tickets] -> oldest-first first-match selects the earliest author ticket deterministically; multi-ticket branches are out of convention and already ambiguous. Branch-name precedence lets a contributor pin the id explicitly by naming the branch.
- [A gate commit subject is mistaken for an author subject on a re-run] -> skip subjects beginning with `no-mistakes`/`<id>: no-mistakes`; even if one slipped through it carries the same id, so the result is unchanged.
- [PR-title source only helps at the CI step] -> accepted and documented; by the time a PR exists, a branch/commit-derived id is already applied, so the PR-title source only adds the human-edited-title edge case.
- [Format change to already-shipped behavior] -> cosmetic, no parser depends on it; covered by updated unit tests and an e2e assertion on a ticketed-commit/unticketed-branch run.

## Migration Plan

No data migration. Ship the code change; the next gated run on any repo with `ticket_prefix_pattern` set emits the new format. Rollback is reverting the change; older gate commits already in history are unaffected.

## Open Questions

- None. Resolution precedence is confirmed as branch -> PR title -> first non-gate commit.
