-- Frozen execution site for a pre-start HITL assignment. Approve consumes
-- this row to create attempt 1; it is not a live attempt and cannot launch.
CREATE TABLE run_launch_sites (
    run_id TEXT PRIMARY KEY REFERENCES runs(id),
    worktree_path TEXT NOT NULL,
    branch_name TEXT NOT NULL,
    base_ref TEXT NOT NULL,
    base_sha TEXT NOT NULL,
    created_at_ms INTEGER NOT NULL
);
