//go:build unix

package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/kunchenguid/no-mistakes/internal/paths"
	"github.com/kunchenguid/no-mistakes/internal/types"
)

// TestAgent_EnvOverrideIsolatesNMHome is the regression test for the daemon
// self-gating crash: a daemon-spawned agent inherited the orchestrator's
// NM_HOME, so any no-mistakes CLI it ran (the test step's evidence agent
// builds and exercises the project's own binary) resolved to the SAME
// socket/pid as the orchestrating daemon and tore it down mid-run.
//
// With the EnvOverrides chokepoint, the agent subprocess observes the isolated
// NM_HOME, which resolves to a different socket path - so the agent's CLI dials
// a disposable daemon and can never reenter the orchestrating run.
func TestAgent_EnvOverrideIsolatesNMHome(t *testing.T) {
	dir := t.TempDir()
	orchestratorHome := filepath.Join(dir, "orchestrator")
	ephemeralHome := filepath.Join(dir, "agent-home")
	if err := os.MkdirAll(ephemeralHome, 0o755); err != nil {
		t.Fatalf("mkdir ephemeral home: %v", err)
	}
	t.Setenv("NM_HOME", orchestratorHome)

	capturePath := filepath.Join(dir, "agent-nm-home.txt")
	bin := writeFakeCodex(t, dir, `#!/bin/sh
printf '%s' "$NM_HOME" > "`+capturePath+`"
printf '%s\n' '{"type":"item.completed","item":{"type":"agent_message","text":"{\"ok\":true}"}}'
exit 0
`, "")

	ag, err := NewWithOptions(types.AgentCodex, bin, nil, Options{
		EnvOverrides: map[string]string{"NM_HOME": ephemeralHome},
	})
	if err != nil {
		t.Fatalf("NewWithOptions: %v", err)
	}
	defer ag.Close()

	if _, err := ag.Run(context.Background(), RunOpts{Prompt: "produce a transcript", CWD: t.TempDir()}); err != nil {
		t.Fatalf("Run: %v", err)
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatalf("read captured NM_HOME: %v", err)
	}
	agentHome := string(captured)

	if agentHome == orchestratorHome {
		t.Fatalf("agent NM_HOME = %q, must differ from orchestrator NM_HOME", agentHome)
	}
	if agentHome != ephemeralHome {
		t.Fatalf("agent NM_HOME = %q, want isolated home %q", agentHome, ephemeralHome)
	}

	agentSocket := paths.WithRoot(agentHome).Socket()
	orchestratorSocket := paths.WithRoot(orchestratorHome).Socket()
	if agentSocket == orchestratorSocket {
		t.Fatalf("agent socket %q must differ from orchestrator socket %q; reentrancy is not prevented",
			agentSocket, orchestratorSocket)
	}
}
