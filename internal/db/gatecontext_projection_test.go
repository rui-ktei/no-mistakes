package db

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// writeLegacyGateRegistry creates a database whose runs and step_results tables
// carry only the columns the very first schema shipped, i.e. none of the
// additive migrations. It models the on-disk state of a user who upgrades the
// binary before anything has reopened the database read-write.
func writeLegacyGateRegistry(t *testing.T, withAgentPID bool) string {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "state.sqlite")
	legacyDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatalf("open legacy db: %v", err)
	}
	defer legacyDB.Close()
	agentPIDColumn := ""
	agentPIDValue := ""
	if withAgentPID {
		agentPIDColumn = ", agent_pid INTEGER"
		agentPIDValue = ", 4242"
	}
	if _, err := legacyDB.Exec(`
		CREATE TABLE repos (
			id TEXT PRIMARY KEY,
			working_path TEXT NOT NULL UNIQUE,
			upstream_url TEXT NOT NULL,
			fork_url TEXT,
			default_branch TEXT NOT NULL DEFAULT 'main',
			created_at INTEGER NOT NULL
		);
		CREATE TABLE runs (
			id TEXT PRIMARY KEY,
			repo_id TEXT NOT NULL,
			branch TEXT NOT NULL,
			head_sha TEXT NOT NULL,
			base_sha TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			pr_url TEXT,
			error TEXT,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);
		CREATE TABLE step_results (
			id TEXT PRIMARY KEY,
			run_id TEXT NOT NULL,
			step_name TEXT NOT NULL,
			step_order INTEGER NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending'` + agentPIDColumn + `
		);
		INSERT INTO repos (id, working_path, upstream_url, default_branch, created_at)
		VALUES ('repo-1', '/work/repo', 'git@github.com:parent/repo.git', 'main', 1);
		INSERT INTO runs (id, repo_id, branch, head_sha, base_sha, status, created_at, updated_at)
		VALUES ('run-active', 'repo-1', 'refs/heads/feature', 'aaa', 'bbb', 'running', 10, 10),
		       ('run-done', 'repo-1', 'refs/heads/old', 'ccc', 'ddd', 'completed', 5, 5);
		INSERT INTO step_results (id, run_id, step_name, step_order, status` + func() string {
		if withAgentPID {
			return ", agent_pid"
		}
		return ""
	}() + `)
		VALUES ('step-1', 'run-active', 'review', 1, 'running'` + agentPIDValue + `);
	`); err != nil {
		t.Fatalf("create legacy gate registry: %v", err)
	}
	return dbPath
}

// TestGetActiveRunIdentities_ReadsPreMigrationSchema is the regression for the
// upgrade crash where `no-mistakes daemon stop` (as `make install` runs it) died
// with "no such column: submitted_head_sha". The gate-context classifier
// consults the registry through OpenReadOnly, which deliberately never migrates,
// so its queries must project only long-stable columns instead of the full
// current column list.
func TestGetActiveRunIdentities_ReadsPreMigrationSchema(t *testing.T) {
	d, err := OpenReadOnly(writeLegacyGateRegistry(t, true))
	if err != nil {
		t.Fatalf("open legacy db read-only: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	identities, err := d.GetActiveRunIdentities()
	if err != nil {
		t.Fatalf("get active run identities on pre-migration schema: %v", err)
	}
	if len(identities) != 1 {
		t.Fatalf("got %d active run identities, want 1 (only the running row)", len(identities))
	}
	if identities[0].ID != "run-active" || identities[0].RepoID != "repo-1" {
		t.Fatalf("got identity %+v, want {ID:run-active RepoID:repo-1}", identities[0])
	}
}

func TestGetActiveStepIdentitiesByRun_ReadsPreMigrationSchema(t *testing.T) {
	d, err := OpenReadOnly(writeLegacyGateRegistry(t, true))
	if err != nil {
		t.Fatalf("open legacy db read-only: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	steps, err := d.GetActiveStepIdentitiesByRun("run-active")
	if err != nil {
		t.Fatalf("get active step identities on pre-migration schema: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d step identities, want 1", len(steps))
	}
	if steps[0].StepName != "review" || steps[0].AgentPID != 4242 {
		t.Fatalf("got step %+v, want {StepName:review AgentPID:4242}", steps[0])
	}
}

// TestGetActiveStepIdentitiesByRun_ToleratesMissingAgentPIDColumn covers the
// oldest databases, where agent_pid itself predates its migration. A missing
// column degrades to the same zero the code already produces for a NULL pid,
// rather than failing the whole authorization check.
func TestGetActiveStepIdentitiesByRun_ToleratesMissingAgentPIDColumn(t *testing.T) {
	d, err := OpenReadOnly(writeLegacyGateRegistry(t, false))
	if err != nil {
		t.Fatalf("open legacy db read-only: %v", err)
	}
	t.Cleanup(func() { d.Close() })

	steps, err := d.GetActiveStepIdentitiesByRun("run-active")
	if err != nil {
		t.Fatalf("get active step identities without agent_pid column: %v", err)
	}
	if len(steps) != 1 {
		t.Fatalf("got %d step identities, want 1", len(steps))
	}
	if steps[0].StepName != "review" || steps[0].AgentPID != 0 {
		t.Fatalf("got step %+v, want {StepName:review AgentPID:0}", steps[0])
	}
}

// TestGetActiveStepIdentitiesByRun_FiltersToActiveStatuses keeps the projection
// honest about which statuses count as an active agent step, so narrowing the
// query cannot silently widen what the classifier treats as a live step.
func TestGetActiveStepIdentitiesByRun_FiltersToActiveStatuses(t *testing.T) {
	d := openTestDB(t)
	repo, err := d.InsertRepo("/work/repo", "git@github.com:parent/repo.git", "main")
	if err != nil {
		t.Fatal(err)
	}
	run, err := d.InsertRun(repo.ID, "refs/heads/feature", "aaa", "bbb")
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		step   string
		status string
	}{
		{"review", "running"},
		{"test", "fixing"},
		{"lint", "awaiting_approval"},
		{"document", "fix_review"},
		{"rebase", "completed"},
		{"push", "pending"},
		{"pr", "failed"},
		{"ci", "skipped"},
	} {
		if _, err := d.sql.Exec(
			`INSERT INTO step_results (id, run_id, step_name, step_order, status) VALUES (?, ?, ?, 1, ?)`,
			"step-"+tc.step, run.ID, tc.step, tc.status,
		); err != nil {
			t.Fatal(err)
		}
	}
	steps, err := d.GetActiveStepIdentitiesByRun(run.ID)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, s := range steps {
		got[string(s.StepName)] = true
	}
	for _, want := range []string{"review", "test", "lint", "document"} {
		if !got[want] {
			t.Errorf("active step %q missing from projection", want)
		}
	}
	for _, notWant := range []string{"rebase", "push", "pr", "ci"} {
		if got[notWant] {
			t.Errorf("inactive step %q must not be reported as active", notWant)
		}
	}
}
