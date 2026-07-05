---
title: Repo Config Reference
description: All fields for .no-mistakes.yaml.
---

Per-repo configuration lives in `.no-mistakes.yaml` at the root of your repository.

:::caution[Security: code-executing fields are read from the default branch]
`commands.*` execute arbitrary shell on the daemon host via `sh -c` / `cmd.exe /c`, and `agent` selects which process launches there (including ordered fallback lists and `acp:` targets) with the maintainer's credentials. To prevent a supply-chain attack where a contributor lands a hostile value on a gated branch, the daemon always reads **`commands` and `agent` from your default branch** (e.g. `origin/main`), never from the pushed SHA, and reads them at the exact commit a fresh fetch resolved (so a stale `origin/<default>` ref cannot serve a value the live default branch removed). If the fetch fails, both fields are forced empty — the run proceeds on built-in defaults rather than falling back to a potentially stale or hostile copy. Commit the `commands` and `agent` you want the gate to run to your default branch. Non-executing fields (`ignore_patterns`, `auto_fix`, `intent`, `test`) are still read from the pushed branch.

If you genuinely want per-branch `commands` and `agent` (for example, a single-developer repo where you trust your own feature branches), opt in with [`allow_repo_commands: true`](#allow_repo_commands) in this same file on your default branch. This re-enables the previous behavior with eyes open. The switch is read only from the trusted default-branch copy, so a contributor cannot self-enable it from a pushed branch.
:::

```yaml
# .no-mistakes.yaml

agent: codex

commands:
  lint: "golangci-lint run ./..."
  test: "go test -race ./..."
  format: "gofmt -w ."

ignore_patterns:
  - "*.generated.go"
  - "vendor/**"

ticket_prefix_pattern: 'WEB-\d+'

auto_fix:
  rebase: 3
  review: 3
  test: 3
  document: 3
  lint: 5
  ci: 3

intent:
  enabled: true
  threshold: 0.2
  slack_days: 3
  disabled_readers: []

test:
  evidence:
    store_in_repo: true
    dir: .no-mistakes/evidence
```

## Fields

### agent

Override the default agent for this repo and its setup-wizard suggestions.

| | |
|---|---|
| Type | `string` or `string[]` |
| Values | `auto`, `claude`, `codex`, `rovodev`, `opencode`, `pi`, `copilot`, `acp:<target>` |
| Default | Inherits from global config |

`auto` resolves to the first supported native agent found on `PATH` in this order: `claude`, `codex`, `opencode`, `acli` with `rovodev` support, `pi`, then `copilot`.
`acp:<target>` uses the user-installed `acpx` binary configured in global config.
ACP agents are opt-in and are not considered by `agent: auto`.

You can also set an ordered fallback list:

```yaml
agent: [codex, claude]
```

The list is filtered to entries available to the daemon at run startup, and the first available entry becomes the primary agent. If a pipeline invocation fails because that agent process cannot start or exits with an error, no-mistakes retries that invocation with the next available fallback. Structured findings and schema/output validation problems do not trigger fallback. This per-repo `agent` value, including every fallback entry, is still read from the trusted default-branch `.no-mistakes.yaml` unless `allow_repo_commands` is enabled there.

### allow_repo_commands

Opt in to honoring the code-executing selection fields (`commands.{test,lint,format}` and `agent`) from a contributor's pushed branch instead of the trusted default-branch copy.

| | |
|---|---|
| Type | `bool` |
| Default | `false` |

This field is itself read **only from the trusted default-branch copy** of `.no-mistakes.yaml`, never from the pushed SHA, so a contributor cannot self-enable it by setting it on a feature branch. By default the daemon reads `commands` and `agent` from your default branch (e.g. `origin/main`) so a pushed SHA cannot inject shell or pick the launched agent on the daemon host. Leave this `false` for any repo that accepts contributions. Set it to `true` only for a single-developer environment where you trust every branch you push (for example, a personal repo gated by your own daemon).

### commands.test

Explicit test command. Run via the platform shell - `sh -c` on POSIX, `cmd.exe /c` on Windows.

| | |
|---|---|
| Type | `string` |
| Default | Empty (agent auto-detects tests and evidence checks) |

When set, the test step runs this exact command first as the baseline and checks the exit code.
When empty, the agent detects and runs relevant tests itself, and - unless [`propose_commands`](#propose_commands) is disabled - proposes the canonical command it used as an edit to this file (see [propose_commands](#propose_commands)).
When user intent is available, the agent may still run after a successful baseline command to gather evidence-oriented validation.

### commands.lint

Explicit lint command. Run via the platform shell - `sh -c` on POSIX, `cmd.exe /c` on Windows.

| | |
|---|---|
| Type | `string` |
| Default | Empty (agent auto-detects) |

When set, the lint step runs this exact command and checks the exit code.
When empty, the agent detects relevant linters and formatters, applies safe fixes, reruns the relevant checks, commits any agent changes, and reports only unresolved issues. Unless [`propose_commands`](#propose_commands) is disabled, it also proposes the canonical lint command (and the canonical `commands.format` command, when it ran a distinct formatter) as an edit to this file.

### commands.format

Formatter command run before the push step commits agent fixes.

| | |
|---|---|
| Type | `string` |
| Default | Empty (no separate push-step formatter) |

This does not prevent empty `commands.lint` from detecting and running formatters during the lint step.
When empty, the lint discovery agent reports the canonical formatter it ran, which - unless [`propose_commands`](#propose_commands) is disabled - is proposed as an edit to this file.

### propose_commands

Whether a run proposes the canonical `commands.{test,lint,format}` a discovery agent used as an edit to this file, so that a human merge to your default branch promotes them to trusted, executed config and future runs skip rediscovery.

| | |
|---|---|
| Type | `bool` |
| Default | `true` (inherits the [global `propose_commands`](/reference/global-config/#propose_commands)) |

When a run has no configured command for a field, a discovery agent works out how to test, lint, or format the project. With this on, the run writes the single canonical command it settled on into a dedicated commit on the branch's `.no-mistakes.yaml` (only for fields that are currently unset and not already proposed on the branch), and the pull request notes the proposal.

The proposed command is **inert until you merge it to the default branch**: `commands.*` are read only from the trusted default-branch copy, so the discovering run never executes the proposal, and later runs keep rediscovering until the merge lands. Review the proposed command in the PR and edit or drop it before merging if it is wrong; the merged value is what future runs execute. This never enables [`allow_repo_commands`](#allow_repo_commands) and never widens the trust boundary.

Like `allow_repo_commands`, this switch is read only from the trusted default-branch copy of `.no-mistakes.yaml`, so a contributor cannot flip it from a pushed branch. Set it to `false` to turn the behavior off for a repo.

### Command process lifetime

All configured `commands.*` entries are scoped to their step.
After no-mistakes starts one of these commands, it terminates any remaining child processes from that command when the command exits, fails, or the step is cancelled.
Do not rely on a configured command to leave a background server or watcher running after it returns; keep that service inside the command lifetime or start it outside no-mistakes.

### ignore_patterns

Paths to exclude from review and documentation checks.

| | |
|---|---|
| Type | `string[]` |
| Default | Empty (no ignores) |

Pattern matching rules:

| Pattern | Rule |
|---|---|
| `*.generated.go` | No slash - matches by basename |
| `vendor/**` | Ends with `/**` - matches entire subtree |
| `some/path/file.go` | Contains a slash - full path glob |

### ticket_prefix_pattern

Opt in to a work-item title/commit convention for this repo instead of conventional commits.
When set to a regexp, no-mistakes resolves the work-item id by matching the pattern against the branch name first, then the PR title (when a PR exists), then the first non-gate author commit subject on the branch (oldest first).
The first source that produces a match supplies the id (e.g. `WEB-12345`), which is then prepended to the PR title and to the commit subjects the gate authors during fixes.

| | |
|---|---|
| Type | `string` (regexp) |
| Default | Inherits from global (default off - conventional-commit formatting) |

When an id is resolved, it leads the PR title (with any conventional `type(scope): ` prefix stripped so the result is not double-prefixed), and authored fix commits use `<ticket>: no-mistakes(<step>): <summary>` instead of `no-mistakes(<step>): <summary>`.
When no source carries a match, the gate falls back to conventional commits, so ticket-less changes still work.
A non-empty value here overrides the global `ticket_prefix_pattern`.
This is a non-executing field, so it is read from the pushed branch (unlike `commands` and `agent`).

### auto_fix

Override auto-fix attempt limits for specific steps. Fields not set here inherit from global config.

| | |
|---|---|
| Type | `object` |

| Field | Type | Default |
|---|---|---|
| `auto_fix.rebase` | `int` | Inherits from global (default `3`) |
| `auto_fix.review` | `int` | Inherits from global (default `0`) |
| `auto_fix.test` | `int` | Inherits from global (default `3`) |
| `auto_fix.document` | `int` | Inherits from global (default `3`) |
| `auto_fix.lint` | `int` | Inherits from global (default `3`) |
| `auto_fix.ci` | `int` | Inherits from global (default `3`) |

Set to `0` to disable the follow-up auto-fix loop for a step (findings require manual approval).
The document step attempts documentation fixes during its initial pass, so unresolved documentation findings pause for approval instead of using an automatic follow-up loop.
For empty `commands.lint`, the agent still attempts safe fixes during the initial lint pass; unresolved lint findings then pause for approval instead of starting another automatic fix loop.

`auto_fix.ci` covers the CI step's CI failure and merge-conflict auto-fix attempts.

Legacy alias: `auto_fix.babysit`.

### intent

Override transcript-based user-intent extraction settings for this repo.
Fields not set here inherit from global config and then the built-in defaults.

| Field | Type | Default |
|---|---|---|
| `intent.enabled` | `bool` | Inherits from global (default `true`) |
| `intent.threshold` | `float` | Inherits from global (default `0.2`) |
| `intent.slack_days` | `int` | Inherits from global (default `3`) |
| `intent.disabled_readers` | `string[]` | Adds to globally disabled readers |

Valid `disabled_readers` values are `claude`, `codex`, `opencode`, `rovodev`, `pi`, and `copilot`.

### test.evidence

Override where evidence artifacts from the test step are stored.
Fields not set here inherit from global config and then the built-in defaults.

| Field | Type | Default |
|---|---|---|
| `test.evidence.store_in_repo` | `bool` | Inherits from global (default `false`) |
| `test.evidence.dir` | `string` | Inherits from global (default `.no-mistakes/evidence`) |

By default, test evidence stays in a temporary directory keyed by run ID and is referenced by local path.
Set `store_in_repo: true` to write evidence under `<dir>/<branch-slug>` inside the worktree so push can commit and publish it with the branch.
Branch slashes become nested directories, unsafe branch characters are replaced, and an empty branch slug falls back to the run ID.
If `dir` is absolute, escapes the worktree, points into `.git`, crosses a symlink, or is ignored by Git, no-mistakes falls back to temporary evidence storage for that run.
