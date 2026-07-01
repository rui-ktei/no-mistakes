## ADDED Requirements

### Requirement: Pipeline agents run with an isolated NM_HOME

A daemon-spawned pipeline agent subprocess SHALL run with an `NM_HOME` distinct from the orchestrating daemon's `NM_HOME`.
The isolated `NM_HOME` SHALL point at an existing, writable directory dedicated to that run, so that any `no-mistakes` CLI command the agent runs resolves to a different socket and pid than the orchestrator.
The override SHALL apply to every agent backend and every pipeline step that spawns an agent, set once at agent construction.

#### Scenario: Agent environment is redirected
- **WHEN** the daemon constructs the agent for a run and spawns it
- **THEN** the agent subprocess SHALL observe an `NM_HOME` value different from the daemon's
- **AND** that value SHALL be an existing, writable directory

#### Scenario: Agent CLI cannot reach the orchestrator
- **WHEN** an agent subprocess runs the project's own `no-mistakes` CLI
- **THEN** the CLI SHALL resolve its daemon socket and pid from the isolated `NM_HOME`
- **AND** that socket path SHALL differ from the orchestrating daemon's socket path
- **AND** the orchestrating run SHALL NOT be stopped, restarted, or cancelled by that CLI invocation

#### Scenario: Daemon home is unchanged
- **WHEN** pipeline agents are isolated
- **THEN** the orchestrating daemon SHALL continue to use its own `NM_HOME`
- **AND** `NM_HOME` resolution precedence in `internal/paths` SHALL be unchanged

### Requirement: Ephemeral agent home lifecycle

The isolated agent home SHALL be created when the run's agent is constructed and removed when the run ends, whether the run succeeds, fails, or is cancelled.
Cleanup SHALL be best-effort and idempotent; a leftover empty directory SHALL NOT cause an error.

#### Scenario: Created for the run
- **WHEN** a run starts and its agent is constructed
- **THEN** the isolated agent home directory SHALL exist before the agent is spawned

#### Scenario: Removed on run end
- **WHEN** a run reaches a terminal state (success, failure, or cancel)
- **THEN** the isolated agent home for that run SHALL be removed
- **AND** a failure to remove it SHALL NOT fail the run

### Requirement: Test step runs without the skip workaround

With agent isolation in place, gating the no-mistakes repository itself SHALL be able to run the `test` step (including its evidence-gathering agent) without `--skip test`.

#### Scenario: Self-gating completes the test step
- **WHEN** no-mistakes gates its own repository and the `test` step spawns the evidence agent
- **THEN** the evidence agent's `no-mistakes` invocations SHALL NOT terminate the orchestrating daemon
- **AND** the `test` step SHALL complete on its own merits rather than crashing the daemon
