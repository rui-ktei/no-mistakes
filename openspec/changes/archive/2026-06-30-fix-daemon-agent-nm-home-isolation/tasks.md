## 1. Reproduce (TDD, failing first)

- [x] 1.1 Add a regression test that spawns a daemon-pipeline agent (fake-agent fixture, mirroring `internal/agent/reap_unix_test.go` and the `NM_CAPTURE_NM_HOME_FILE` capture in `internal/daemon/service_test.go`) and asserts the agent's observed `NM_HOME` differs from the orchestrator's. Confirm it FAILS on `main` (today the agent inherits the daemon's `NM_HOME`).
- [x] 1.2 Strengthen the assertion: derive the socket path from the captured agent `NM_HOME` (`<root>/socket`) and assert it differs from the orchestrator's socket - proving reentrancy is actually prevented, not just that a var changed.

## 2. Env override plumbing

- [x] 2.1 Add an env override to agent construction: an `NMHome string` or generic `EnvOverrides map[string]string` on `agent.Options` (`internal/agent/agent.go`).
- [x] 2.2 Apply the override at the shared native-agent env build (after `cmd.Env = gitSafeEnv(opts.CWD)` in `claude.go`, `codex.go`, `pi.go`, `copilot.go`, `acpx.go`, and opencode/rovodev), with the override replacing any inherited `NM_HOME`. Prefer a single helper in `internal/agent/env.go` so all backends share one code path.
- [x] 2.3 Make the test from 1.1/1.2 pass.

## 3. Ephemeral home lifecycle in the daemon

- [x] 3.1 In `internal/daemon/manager.go`, create a per-run ephemeral agent home (existing, writable `0o755` dir, e.g. `<NM_HOME>/agent-homes/<runID>` or a tempdir) before constructing the agent, and pass it via `agent.NewWithOptions`.
- [x] 3.2 Remove the ephemeral home when the run ends (success/failure/cancel), wired into existing run teardown; best-effort and idempotent.
- [x] 3.3 Test: ephemeral home exists before agent spawn and is gone after the run reaches a terminal state.

## 4. Full verification

- [x] 4.1 `gofmt -w .`, `make lint`, `go test -race ./...`.
- [x] 4.2 Confirm the existing reaping regressions still pass (`TestCodexAgent_Run_ReapsLeakedGrandchildOnCleanExit`, `TestRunShellCommandWithEnv_ReapsGrandchildOnCleanExit`, `TestTerminateShellCommandGroup_*`).
- [x] 4.3 End-to-end: gate a trivial change in the no-mistakes repo itself WITHOUT `--skip test` and confirm the daemon survives the test step. Run `make e2e` separately, outside any gated pipeline run (per AGENTS.md).

## 5. Docs / cleanup

- [x] 5.1 Note the isolation in `AGENTS.md` Context/Concurrency section (daemon-spawned agents get an ephemeral `NM_HOME`; why) so the invariant is not regressed.
- [x] 5.2 Update the memory note `daemon-evidence-agent-reentrancy` to reflect the fix once landed.
- [x] 5.3 Remove the standing `--skip test` workaround from the local dogfooding flow.
