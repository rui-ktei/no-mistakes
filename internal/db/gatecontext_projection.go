package db

import (
	"fmt"

	"github.com/kunchenguid/no-mistakes/internal/types"
)

// The gate-context classifier reads the registry through OpenReadOnly, which
// deliberately performs no migration so pre-authorization classification cannot
// write to disk. A freshly upgraded binary therefore meets whatever schema the
// previous version left behind, and any query naming a column added by a later
// additive migration fails outright - the upgrade crash where `daemon stop`
// reported "no such column: submitted_head_sha".
//
// The projections below exist to keep that authorization path independent of
// schema drift: each selects only the columns it actually consumes, all of which
// have existed since the initial schema. Widening one of these queries to a full
// row read (or to any migration-added column) reintroduces the crash, so keep
// them narrow and add new fields to the richer accessors instead.

// ActiveRunIdentity is the minimal active-run projection the gate-context
// classifier consumes.
type ActiveRunIdentity struct {
	ID     string
	RepoID string
}

// GetActiveRunIdentities returns the identity of every pending or running run,
// newest first, without reading any migration-added column.
func (d *DB) GetActiveRunIdentities() ([]ActiveRunIdentity, error) {
	rows, err := d.sql.Query(
		`SELECT id, repo_id FROM runs WHERE status IN (?, ?) ORDER BY created_at DESC, id DESC`,
		types.RunPending, types.RunRunning,
	)
	if err != nil {
		return nil, fmt.Errorf("get active run identities: %w", err)
	}
	defer rows.Close()

	var identities []ActiveRunIdentity
	for rows.Next() {
		var identity ActiveRunIdentity
		if err := rows.Scan(&identity.ID, &identity.RepoID); err != nil {
			return nil, fmt.Errorf("scan active run identity: %w", err)
		}
		identities = append(identities, identity)
	}
	return identities, rows.Err()
}

// ActiveStepIdentity is the minimal active-step projection the gate-context
// classifier consumes.
type ActiveStepIdentity struct {
	StepName types.StepName
	AgentPID int
}

// activeStepStatuses are the step statuses that mean an agent may currently be
// running under the step. It mirrors gatecontext's activeStepStatus so the
// projection cannot silently widen what counts as a live step.
var activeStepStatuses = []types.StepStatus{
	types.StepStatusRunning,
	types.StepStatusFixing,
	types.StepStatusAwaitingApproval,
	types.StepStatusFixReview,
}

// GetActiveStepIdentitiesByRun returns the active steps of one run. agent_pid is
// itself migration-added, so on a database that predates it the pid degrades to
// the same zero the code already produces for a NULL pid rather than failing the
// whole authorization check.
func (d *DB) GetActiveStepIdentitiesByRun(runID string) ([]ActiveStepIdentity, error) {
	hasAgentPID, err := d.hasColumn("step_results", "agent_pid")
	if err != nil {
		return nil, fmt.Errorf("inspect step_results schema: %w", err)
	}
	pidColumn := "NULL"
	if hasAgentPID {
		pidColumn = "agent_pid"
	}
	args := []any{runID}
	placeholders := ""
	for i, status := range activeStepStatuses {
		if i > 0 {
			placeholders += ", "
		}
		placeholders += "?"
		args = append(args, status)
	}
	rows, err := d.sql.Query(
		`SELECT step_name, `+pidColumn+` FROM step_results WHERE run_id = ? AND status IN (`+placeholders+`) ORDER BY step_order`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("get active step identities: %w", err)
	}
	defer rows.Close()

	var steps []ActiveStepIdentity
	for rows.Next() {
		var step ActiveStepIdentity
		var pid *int64
		if err := rows.Scan(&step.StepName, &pid); err != nil {
			return nil, fmt.Errorf("scan active step identity: %w", err)
		}
		if pid != nil {
			step.AgentPID = int(*pid)
		}
		steps = append(steps, step)
	}
	return steps, rows.Err()
}

func (d *DB) hasColumn(table, column string) (bool, error) {
	rows, err := d.sql.Query(`SELECT COUNT(*) FROM pragma_table_info(?) WHERE name = ?`, table, column)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	if !rows.Next() {
		return false, rows.Err()
	}
	var count int
	if err := rows.Scan(&count); err != nil {
		return false, err
	}
	return count > 0, rows.Err()
}
