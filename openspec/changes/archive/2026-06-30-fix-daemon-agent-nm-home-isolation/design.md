## Context

A run's agent is constructed once in `internal/daemon/manager.go` (`agent.NewWithOptions(cfg.Agent, ...)`) and handed to every step via `StepContext.Agent`.
Each native backend builds its subprocess environment as `cmd.Env = gitSafeEnv(opts.CWD)` (`internal/agent/claude.go` and siblings), where `gitSafeEnv` -> `git.NonInteractiveEnv` does `append(os.Environ(), ...)` (`internal/git/env.go`).
So the agent inherits the daemon's full environment, including `NM_HOME` (set at `cmd/no-mistakes/main.go` for the daemon process).
A `no-mistakes` CLI run by the agent calls `paths.New()` (`internal/paths/paths.go`), reads that inherited `NM_HOME`, and derives the **same** `socket`/`daemon.pid` as the orchestrator (`Paths.Socket()` = `<root>/socket`).
Confirmed isolation lever: a different `NM_HOME` yields a different socket and pid, so the agent's CLI would dial a different (disposable) daemon.

## Goals / Non-Goals

**Goals:**
- A daemon-spawned pipeline agent can never reenter the orchestrating run via the project's own CLI.
- One chokepoint covering all agent backends and all steps.
- Ephemeral home created per run and cleaned up deterministically.
- No change to `NM_HOME` precedence or to the daemon's own home.

**Non-Goals:**
- Sandboxing the agent's filesystem or network beyond `NM_HOME`.
- Preventing an agent from running `no-mistakes` at all (it may legitimately want to; it just must hit a throwaway home).
- Changing what the test-step evidence agent is asked to do.

## Decisions

**Isolate at construction, not per call.**
Isolation is a property of "this agent was spawned by the daemon for a run", so set it once when the run's agent is built (`manager.go`) rather than at each `sctx.Agent.Run` site. Carry it on `agent.Options` (and through to each native runner's env assembly). Steps stay unchanged. This mirrors how `startNativeAgentCommand` centralizes the subprocess lifecycle.

**Override mechanism: an env override applied where `cmd.Env` is built.**
Add an `NMHome string` (or generic `EnvOverrides map[string]string`) to `agent.Options`; in the shared env path, after `append(os.Environ(), ...)`, set/replace `NM_HOME=<ephemeral>` (override wins over the inherited value). A generic map is more future-proof; a single `NMHome` field is the minimum. Decide during apply, leaning to the generic map keyed by `NM_HOME` so the same hook can isolate other state later.

**Ephemeral home is a real, writable, per-run directory.**
A throwaway daemon may actually start there if the agent runs `no-mistakes daemon ...`, so the dir must exist and be writable (`0o755`). Path under the orchestrator state, e.g. `<NM_HOME>/agent-homes/<runID>`, or `os.MkdirTemp`. Created when the agent is constructed; removed when the run ends (success/failure/cancel) alongside existing run teardown. Cleanup is best-effort and idempotent; a leftover dir is harmless.

**Daemon's own home unchanged.**
`paths.New()` and `cmd/no-mistakes/main.go` are untouched. Only the child agent's environment is redirected.

## Risks / Trade-offs

- **Agent assumes orchestrator state**: if any backend relied on reading the orchestrator's `NM_HOME`, it would now see an empty home. Reviewed: agents operate on the worktree, not no-mistakes state, so this is safe; assert via test.
- **Disk**: an ephemeral home per run is tiny (empty unless the agent starts a daemon). Cleanup on run end bounds it; a crash leaves an empty dir, swept on next start if desired.
- **Cross-platform**: the override is pure env, so Windows/unix identical; no `SysProcAttr` interaction. The existing process-group reaping (`ConfigureShellCommand`) still reaps any throwaway daemon the agent spawned in its group.
- **Test realism**: the regression test should assert both that `NM_HOME` differs and that it resolves to a different socket path, not merely that the var is set, to prove true reentrancy prevention.
