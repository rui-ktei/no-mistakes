# command-discovery-persistence Specification

## Purpose

Define how a run captures the canonical test, lint, and format command an agent discovers, proposes each currently-unset `commands.*` value as a reviewable edit to the branch's `.no-mistakes.yaml`, surfaces it in the pull request, and guarantees a proposed command becomes trusted and executed config only through a human-ratified merge to the default branch - never auto-applied on the pushed branch.

## Requirements

### Requirement: Discovery agents report a canonical command

When a run has no configured `commands.test`, `commands.lint`, or `commands.format`, a discovery agent SHALL report, as a dedicated structured output field, the single canonical shell command it used to test, lint, or format the project.
The test command SHALL be sourced from the test discovery agent; the lint and format commands SHALL both be sourced from the lint discovery agent, which already detects and runs both linters and formatters, reporting them as two distinct fields.
The canonical command SHALL be the exact command a maintainer could paste into `.no-mistakes.yaml` to reproduce the check, distinct from the free-form `tested` array and from prose findings.
When the agent cannot identify a single confidently-reproducible command (for example it ran an ad-hoc or multi-step sequence, or nothing conclusive), it SHALL report an empty canonical command rather than guess.

#### Scenario: Test discovery yields a canonical command
- **WHEN** a repo with no `commands.test` runs and the discovery agent runs `go test -race ./...` as the project's tests
- **THEN** the step SHALL surface a canonical test command equal to `go test -race ./...`
- **AND** that value SHALL be available to the proposal step

#### Scenario: No confidently-canonical command
- **WHEN** the discovery agent performs ad-hoc or non-reproducible verification and cannot name a single command
- **THEN** the canonical command SHALL be empty
- **AND** no proposal SHALL be made for that field

#### Scenario: Format command sourced from the lint agent
- **WHEN** a repo with no `commands.format` runs and the lint discovery agent runs a distinct formatter (for example `gofmt -w .`) alongside the linter
- **THEN** the lint step SHALL surface a canonical format command distinct from the canonical lint command
- **AND** when no distinct formatter was run, or the formatter coincides with the lint command, the canonical format command SHALL be empty

#### Scenario: Command already configured
- **WHEN** a repo already has `commands.test` set in the effective config
- **THEN** the test discovery agent SHALL NOT run for the purpose of proposing a command
- **AND** no canonical test command SHALL be captured

### Requirement: Discovered commands are proposed only for unset fields

The run SHALL propose a discovered command into the branch's `.no-mistakes.yaml` only for a `commands.*` field that is currently empty in the effective config.
A field that is already set - whether from the trusted default branch, or honored from the pushed branch under `allow_repo_commands` - SHALL NOT be overwritten, and SHALL NOT be re-proposed.
When no field is both unset and backed by a non-empty canonical command, the proposal step SHALL be a no-op.

#### Scenario: Only unset fields are proposed
- **WHEN** a run has `commands.lint` set but `commands.test` and `commands.format` unset, and canonical commands are discovered for test and format
- **THEN** the proposal SHALL add `commands.test` and `commands.format` to `.no-mistakes.yaml`
- **AND** the proposal SHALL NOT modify `commands.lint`

#### Scenario: Nothing to propose
- **WHEN** every `commands.*` field is already set, or no unset field has a canonical command
- **THEN** the proposal step SHALL make no change and create no commit

#### Scenario: Existing proposal not duplicated
- **WHEN** the branch's `.no-mistakes.yaml` already contains a proposed value for an unset field equal to the newly discovered command
- **THEN** the proposal step SHALL NOT create a duplicate commit or a redundant change for that field

### Requirement: A proposal is a reviewable branch commit surfaced in the PR

When there is at least one command to propose, the run SHALL write the proposed values into the branch's `.no-mistakes.yaml` and commit them as a dedicated commit on the branch.
The commit and the pull request SHALL make the proposal visible to a human reviewer, so that the reviewer can accept, edit, or remove the proposed commands by editing the file before merge.
The proposal SHALL be written to the effective integration branch's working tree in a way consistent with the run's existing rebase, diff-base, and force-push safety invariants; it SHALL NOT bypass the force-push lease or bundle unrelated local commits.

#### Scenario: Proposal appears on the branch and PR
- **WHEN** a run discovers a canonical test command for a repo with `commands.test` unset
- **THEN** the branch SHALL gain a commit that adds `commands.test` to `.no-mistakes.yaml`
- **AND** the pull request SHALL note that no-mistakes proposed pinning the discovered command

#### Scenario: Reviewer edits before merge
- **WHEN** a reviewer edits or deletes the proposed `commands.*` value in the PR before merging
- **THEN** the merged default branch SHALL reflect the reviewer's edited value, not the original proposal

### Requirement: Proposed commands become trusted and executed only after merge to the default branch

A proposed command SHALL NOT alter the trusted config read path or be executed verbatim within the run that discovered it.
A discovered command SHALL become an executed `commands.*` value only after it is present on the auto-detected default branch, read through the existing trusted-config path.
A contributor SHALL NOT be able to cause a discovered command to execute on the pushed branch, and the proposal mechanism SHALL NOT enable `allow_repo_commands` or otherwise widen the trust boundary.

#### Scenario: Proposed command does not execute in the discovering run
- **WHEN** a run proposes `commands.test` on the branch
- **THEN** that run SHALL NOT execute the proposed command verbatim as a configured command
- **AND** the run's test behavior SHALL be unchanged from the no-command-configured discovery path

#### Scenario: Trusted read path unchanged
- **WHEN** a later run starts after the proposal commit is still only on the feature branch (not merged)
- **THEN** the trusted config read SHALL still find `commands.test` unset
- **AND** the later run SHALL rediscover rather than execute the unmerged proposal

#### Scenario: Executed only after human merge
- **WHEN** the proposal commit has been merged into the default branch
- **THEN** the next run SHALL read the command from the trusted default-branch config
- **AND** SHALL execute it verbatim, skipping agent rediscovery for that field

### Requirement: Proposal writing is configurable and defaults on

The propose-on-discovery behavior SHALL be controllable via configuration and SHALL default to enabled.
When disabled, discovery SHALL behave exactly as before this change: the canonical command MAY still be captured internally, but no `.no-mistakes.yaml` edit or commit SHALL be produced.

#### Scenario: Feature disabled
- **WHEN** the propose-on-discovery setting is disabled and a run discovers a canonical command for an unset field
- **THEN** no `.no-mistakes.yaml` change or proposal commit SHALL be created

#### Scenario: Default enabled
- **WHEN** no propose-on-discovery setting is specified
- **THEN** the behavior SHALL be enabled
