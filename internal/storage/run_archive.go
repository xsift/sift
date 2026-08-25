package storage

import (
	"context"
	"errors"
	"fmt"
)

// ErrRunNotFound is returned by run-removal operations when no run row matched
// the id (including the already-archived case, where the idempotency guard
// matches zero rows).
var ErrRunNotFound = errors.New("storage: run not found")

// ArchiveRun is the storage half of `sift rm`. A run's audit trail is
// append-only (events, budget_entries, ledger_entries, gate_evaluations, ...) so
// it cannot be hard-deleted without orphaning those immutable rows; archiving
// instead stamps archived_at_ms so `sift ps` hides the run while every event,
// ledger entry and metric is retained verbatim. The same transaction lets the
// issue slot go (partial unique index) and requeues a consumed intake item so
// the next tick can open a new Run without rewriting sqlite by hand.
//
// The WHERE archived_at_ms IS NULL guard makes a second archive on the same run
// match zero rows, so the operation is idempotent from the caller's view
// (re-archiving reports ErrRunNotFound). The run's optimistic-concurrency
// version is bumped so a concurrent lifecycle transition observes the change.
func (d *DB) ArchiveRun(ctx context.Context, runID string, nowMS int64) error {
	if runID == "" {
		return errors.New("storage: archive run requires a run id")
	}
	if nowMS <= 0 {
		return errors.New("storage: archive run requires a timestamp")
	}
	tx, err := d.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("storage: archive run: %w", err)
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx, `UPDATE runs SET archived_at_ms=?, version=version+1, updated_at_ms=? WHERE id=? AND archived_at_ms IS NULL`, nowMS, nowMS, runID)
	if err != nil {
		return fmt.Errorf("storage: archive run: %w", err)
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n == 0 {
		return ErrRunNotFound
	}
	if _, err := tx.ExecContext(ctx, `UPDATE intake_items SET state='pending_evaluation', linked_run_id=NULL, version=version+1, updated_at_ms=? WHERE linked_run_id=? AND state='consumed'`, nowMS, runID); err != nil {
		return fmt.Errorf("storage: requeue archived intake: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("storage: archive run: %w", err)
	}
	return nil
}

// IsRunArchived reports whether the run is archived. It is a read helper for the
// rm handler to distinguish "not found" from "already archived" when needed.
func (d *DB) IsRunArchived(ctx context.Context, runID string) (bool, error) {
	var archived any
	err := d.db.QueryRowContext(ctx, `SELECT archived_at_ms FROM runs WHERE id=?`, runID).Scan(&archived)
	if err != nil {
		return false, err
	}
	return archived != nil, nil
}
