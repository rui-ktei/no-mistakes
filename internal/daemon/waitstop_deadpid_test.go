package daemon

import (
	"errors"
	"os"
	"testing"
	"time"

	"github.com/kunchenguid/no-mistakes/internal/paths"
)

// staleDaemonHome builds an NM_HOME carrying the artifacts a daemon leaves
// behind when its host (a container, a laptop suspend, a reboot) dies without
// running shutdown: a pid file naming a process that no longer exists, plus the
// socket file itself, which nothing is listening on.
func staleDaemonHome(t *testing.T) (*paths.Paths, int) {
	t.Helper()
	p := paths.WithRoot(t.TempDir())
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	deadPID := 999000
	writeDaemonPIDRecord(t, p.PIDFile(), daemonPIDFile{PID: deadPID, StartedAt: time.Now().Add(-time.Hour)})
	if err := os.WriteFile(p.Socket(), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return p, deadPID
}

// TestWaitForDaemonStop_TreatsDeadRecordedPIDAsStopped is the regression for the
// second half of the `make install` upgrade failure: with a managed service
// installed, Stop() delegates to the service manager and then waits here. A
// service manager that reports success without a live daemon (systemctl in a
// container without systemd exits 0) leaves this function polling a socket file
// no one is listening on, so the health check keeps returning an error rather
// than a clean "not alive". After the deadline the PID fallback then failed hard
// with "inspect daemon pid <n>: exit status 1", because `ps` cannot report a
// start time for a process that no longer exists.
//
// A recorded pid that is not running means the daemon is already stopped. That
// is the success case, not an error: there is nothing left to kill.
func TestWaitForDaemonStop_TreatsDeadRecordedPIDAsStopped(t *testing.T) {
	p, deadPID := staleDaemonHome(t)

	restoreHealth := daemonHealthCheck
	restoreRunning := daemonProcessRunning
	restoreStartTime := daemonProcessStartTime
	restoreKill := daemonKillPID
	t.Cleanup(func() {
		daemonHealthCheck = restoreHealth
		daemonProcessRunning = restoreRunning
		daemonProcessStartTime = restoreStartTime
		daemonKillPID = restoreKill
	})

	// A socket file with no listener: the probe cannot complete, so it reports
	// an error rather than a confident "stopped".
	daemonHealthCheck = func(*paths.Paths) (bool, error) {
		return false, errors.New("dial unix: connect: connection refused")
	}
	daemonProcessRunning = func(pid int) (bool, error) {
		if pid != deadPID {
			t.Fatalf("processRunning pid = %d, want %d", pid, deadPID)
		}
		return false, nil
	}
	daemonProcessStartTime = func(pid int) (time.Time, error) {
		return time.Time{}, errors.New("exit status 1")
	}
	daemonKillPID = func(pid int) error {
		t.Fatalf("must not kill pid %d: the process is already gone", pid)
		return nil
	}

	if err := waitForDaemonStop(p); err != nil {
		t.Fatalf("waitForDaemonStop() = %v, want nil for an already-dead daemon", err)
	}
	if _, err := os.Stat(p.PIDFile()); !os.IsNotExist(err) {
		t.Errorf("stale pid file must be cleaned up, stat err = %v", err)
	}
	if _, err := os.Stat(p.Socket()); !os.IsNotExist(err) {
		t.Errorf("stale socket file must be cleaned up, stat err = %v", err)
	}
}

// TestWaitForDaemonStop_DoesNotWaitOutDeadlineForDeadPID keeps the dead-pid exit
// on the fast path. Discovering "already stopped" only after the full stop
// timeout made every `daemon stop` against stale artifacts pay the whole
// deadline before answering.
func TestWaitForDaemonStop_DoesNotWaitOutDeadlineForDeadPID(t *testing.T) {
	p, deadPID := staleDaemonHome(t)

	restoreHealth := daemonHealthCheck
	restoreRunning := daemonProcessRunning
	t.Cleanup(func() {
		daemonHealthCheck = restoreHealth
		daemonProcessRunning = restoreRunning
	})
	daemonHealthCheck = func(*paths.Paths) (bool, error) {
		return false, errors.New("dial unix: connect: connection refused")
	}
	daemonProcessRunning = func(int) (bool, error) { return false, nil }

	start := time.Now()
	if err := waitForDaemonStop(p); err != nil {
		t.Fatalf("waitForDaemonStop() = %v, want nil", err)
	}
	if elapsed := time.Since(start); elapsed > daemonStopTimeout()/2 {
		t.Errorf("waitForDaemonStop took %s; a dead recorded pid must short-circuit the %s deadline",
			elapsed, daemonStopTimeout())
	}
	_ = deadPID
}

// TestWaitForDaemonStop_StillKillsLiveUnresponsiveDaemon guards the other side:
// short-circuiting on a dead pid must not stop the fallback from killing a
// daemon that is genuinely alive but not answering its health check.
func TestWaitForDaemonStop_StillKillsLiveUnresponsiveDaemon(t *testing.T) {
	p := paths.WithRoot(t.TempDir())
	if err := p.EnsureDirs(); err != nil {
		t.Fatal(err)
	}
	livePID := 999001
	startedAt := time.Now().Add(-time.Hour)
	writeDaemonPIDRecord(t, p.PIDFile(), daemonPIDFile{PID: livePID, StartedAt: startedAt})

	restoreHealth := daemonHealthCheck
	restoreRunning := daemonProcessRunning
	restoreStartTime := daemonProcessStartTime
	restoreKill := daemonKillPID
	t.Cleanup(func() {
		daemonHealthCheck = restoreHealth
		daemonProcessRunning = restoreRunning
		daemonProcessStartTime = restoreStartTime
		daemonKillPID = restoreKill
	})
	t.Setenv("NM_TEST_DAEMON_STOP_TIMEOUT", "100ms")

	daemonHealthCheck = func(*paths.Paths) (bool, error) { return true, nil }
	killed := false
	alive := true
	daemonProcessRunning = func(int) (bool, error) { return alive, nil }
	daemonProcessStartTime = func(int) (time.Time, error) { return startedAt, nil }
	daemonKillPID = func(pid int) error {
		if pid != livePID {
			t.Fatalf("killed pid %d, want %d", pid, livePID)
		}
		killed = true
		alive = false
		return nil
	}

	if err := waitForDaemonStop(p); err != nil {
		t.Fatalf("waitForDaemonStop() = %v, want nil after killing the daemon", err)
	}
	if !killed {
		t.Error("a live unresponsive daemon must still be killed by the pid fallback")
	}
}
