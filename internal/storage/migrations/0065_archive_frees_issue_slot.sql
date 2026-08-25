-- Archived runs no longer occupy the intake idempotency slot, so `sift rm`
-- can be followed by a new Run for the same forge issue.
DROP INDEX runs_intake_idempotency;
CREATE UNIQUE INDEX runs_intake_idempotency
    ON runs (forge_kind, forge_host, forge_project_key, issue_id)
    WHERE issue_id IS NOT NULL AND archived_at_ms IS NULL;
