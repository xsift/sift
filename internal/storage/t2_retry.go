package storage

import (
	"context"
	"database/sql"
	"errors"
)

// UnassignedT2Failure is a failed Run that never received an agent. The
// intake evaluator retries T2 assignment for these rows; they still occupy
// the issue slot until assignment succeeds or the operator archives.
type UnassignedT2Failure struct {
	RunID       string
	ProjectID   string
	IssueID     string
	Version     int64
	UpdatedAtMS int64
}

const unassignedT2SQL = `SELECT id,project_id,issue_id,version,updated_at_ms FROM runs
WHERE archived_at_ms IS NULL AND status='failed' AND failure_reason='contract_violation'
AND (agent_id IS NULL OR agent_id='')
AND NOT EXISTS (SELECT 1 FROM attempts WHERE run_id=runs.id)`

// UnassignedT2Failures lists assignment failures that are due for another T2
// attempt (updated_at_ms <= readyBeforeMS).
func (d *DB) UnassignedT2Failures(ctx context.Context, readyBeforeMS int64) ([]UnassignedT2Failure, error) {
	rows, err := d.db.QueryContext(ctx, unassignedT2SQL+` AND updated_at_ms<=? ORDER BY updated_at_ms,id`, readyBeforeMS)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []UnassignedT2Failure
	for rows.Next() {
		var row UnassignedT2Failure
		if err := rows.Scan(&row.RunID, &row.ProjectID, &row.IssueID, &row.Version, &row.UpdatedAtMS); err != nil {
			return nil, err
		}
		out = append(out, row)
	}
	return out, rows.Err()
}

// UnassignedT2Failure looks up one run. ok is false when the run is not an
// unassigned T2 failure (wrong status, already assigned, or has attempts).
func (d *DB) UnassignedT2Failure(ctx context.Context, runID string) (UnassignedT2Failure, bool, error) {
	var row UnassignedT2Failure
	err := d.db.QueryRowContext(ctx, unassignedT2SQL+` AND id=?`, runID).Scan(
		&row.RunID, &row.ProjectID, &row.IssueID, &row.Version, &row.UpdatedAtMS)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return UnassignedT2Failure{}, false, nil
		}
		return UnassignedT2Failure{}, false, err
	}
	return row, true, nil
}
