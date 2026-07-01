## Why

When no-mistakes gates its own repository, the `test` step's evidence-gathering agent (a child of the daemon) inherits the orchestrating daemon's `NM_HOME`.
The agent builds and runs the project's own `no-mistakes` CLI to "produce a real end-user transcript", and because `NM_HOME` is shared, those CLI invocations target the **same** daemon socket / pid / gate that is running the pipeline - tearing it down mid-run.
The run fails with `daemon crashed during execution` (test step `duration_ms: 0`); it reproduces deterministically with ample free RAM (not OOM, not stale binaries).
Both prior PRs had to route around it with `--skip test`.

## What Changes

- Give every **pipeline-spawned agent subprocess an isolated `NM_HOME`** that points at a throwaway, per-run directory, so any `no-mistakes` command the agent runs hits a disposable daemon and can never reach the orchestrator.
- Create the ephemeral agent home when a run's agent is constructed and remove it when the run ends (success, failure, or cancel).
- Apply the override at a single chokepoint (agent construction / the native-agent env builder) so it covers all agent backends and all steps, not just the test step.
- Leave `NM_HOME` resolution itself unchanged (`internal/paths`): the daemon still uses its own home; only the agent child's environment is redirected.

## Capabilities

### New Capabilities
- `pipeline-agent-isolation`: A daemon-spawned pipeline agent runs with an isolated, ephemeral `NM_HOME` distinct from the orchestrating daemon's, so any `no-mistakes` CLI the agent invokes targets a disposable daemon (its own socket/pid) and cannot stop, restart, or otherwise reenter the orchestrating run; the isolated home is created per run and cleaned up when the run ends.

### Modified Capabilities
<!-- None: no existing specs in openspec/specs/. -->

## Impact

- `internal/agent`: extend the agent construction options (`agent.Options`) and/or `RunOpts` with an env override; apply it where each native runner sets `cmd.Env` (`claude.go`, `codex.go`, `pi.go`, `copilot.go`, `acpx.go`, opencode/rovodev) via the shared `env.go` builder.
- `internal/daemon/manager.go`: create the per-run ephemeral agent home, pass it into `agent.NewWithOptions`, and tear it down on run completion.
- `internal/paths`: a helper for the ephemeral agent-home path is acceptable; no change to `NM_HOME` precedence.
- Tests: a regression test asserting a daemon-spawned agent's `NM_HOME` differs from the orchestrator's and resolves to a distinct socket; reuse the env-capture fake-agent pattern (`service_test.go` `NM_CAPTURE_NM_HOME_FILE`, `reap_unix_test.go`).
- Unblocks running the `test` step when gating no-mistakes itself; removes the standing `--skip test` workaround.
