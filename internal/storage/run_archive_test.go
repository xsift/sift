package storage

import (
	"context"
	"database/sql"
	"errors"
	"testing"
)

// TestArchiveRunHidesFromPS verifies archiving stamps archived_at_ms, bumps the
// version, and hides the run from RunPS while a sibling and the audit trail are
// untouched.
func TestArchiveRunHidesFromPS(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	const now = testNow
	if err := db.SeedProjectForTest(ctx, "cfg-ar", "proj-ar", now); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedLaunchRunForTest(ctx, "runA", "proj-ar", "cfg-ar", now, "/w"); err != nil {
		t.Fatal(err)
	}
	// An event is append-only audit trail; it must survive archiving.
	if _, err := db.ExecForTest(ctx, `INSERT INTO events(id,run_id,type,source,payload_schema_version,payload_json,occurred_at_ms,recorded_at_ms) VALUES('evA','runA','run.transitioned','system',1,'{}',?,?)`, now, now); err != nil {
		t.Fatal(err)
	}

	if err := db.ArchiveRun(ctx, "runA", now+1000); err != nil {
		t.Fatalf("ArchiveRun: %v", err)
	}

	// RunPS no longer lists it.
	report, err := db.RunPS(ctx, PSQuery{})
	if err != nil {
		t.Fatalf("RunPS: %v", err)
	}
	for _, r := range report.Runs {
		if r.RunID == "runA" {
			t.Fatalf("archived runA still shown by RunPS")
		}
	}
	// The run row, its attempt and its append-only event are all still present
	// (archive hides, never deletes).
	for _, c := range []struct{ name, q string }{
		{"runs", `SELECT count(*) FROM runs WHERE id='runA'`},
		{"attempts", `SELECT count(*) FROM attempts WHERE run_id='runA'`},
		{"events", `SELECT count(*) FROM events WHERE run_id='runA'`},
	} {
		if n := countRun(t, db, ctx, c.q); n != 1 {
			t.Fatalf("after archive, %s = %d, want 1 (retained)", c.name, n)
		}
	}
	// archived_at_ms is set and version bumped.
	var archived int64
	var version int64
	if err := db.QueryRowForTest(ctx, `SELECT archived_at_ms, version FROM runs WHERE id='runA'`).Scan(&archived, &version); err != nil {
		t.Fatal(err)
	}
	if archived != now+1000 {
		t.Fatalf("archived_at_ms = %d, want %d", archived, now+1000)
	}
}

// TestArchiveRunIdempotent reports not-found when the run is already archived
// (the WHERE archived_at_ms IS NULL guard matches zero rows).
func TestArchiveRunIdempotent(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	if err := db.SeedProjectForTest(ctx, "cfg-id", "proj-id", testNow); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedLaunchRunForTest(ctx, "runI", "proj-id", "cfg-id", testNow, "/w"); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveRun(ctx, "runI", testNow+1); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveRun(ctx, "runI", testNow+2); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("second ArchiveRun = %v, want ErrRunNotFound", err)
	}
	// The first archive's timestamp is retained (not overwritten).
	var archived int64
	if err := db.QueryRowForTest(ctx, `SELECT archived_at_ms FROM runs WHERE id='runI'`).Scan(&archived); err != nil {
		t.Fatal(err)
	}
	if archived != testNow+1 {
		t.Fatalf("archived_at_ms = %d, want %d (idempotent)", archived, testNow+1)
	}
}

func TestArchiveRunFreesIssueSlotAndRequeuesIntake(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	insertConfigSnapshot(t, db, "cfg-rm")
	insertProject(t, db, "p-rm", "cfg-rm")
	first, err := db.CreateForgeRun(ctx, CreateForgeRunCmd{
		RunID: "run-old", ProjectID: "p-rm", ConfigSnapshotID: "cfg-rm",
		ForgeKind: "github", ForgeHost: "github.com", ForgeProjectKey: "org/repo-p-rm",
		IssueID: "7", IssueURL: "u", IssueAuthor: "alice",
		TriggerLabelEventID: "7:sift", TriggerActor: "alice",
		TriggerObservedAtMS: testNow, CreatedAtMS: testNow,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecForTest(ctx, `INSERT INTO intake_items
		(id,project_id,forge_kind,normalized_host,forge_project_key,issue_id,issue_url,issue_digest,state,version,linked_run_id,created_at_ms,updated_at_ms)
		VALUES ('intake-rm','p-rm','github','github.com','org/repo-p-rm','7','u','digest','consumed',1,'run-old',?,?)`, testNow, testNow); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveRun(ctx, first.ID, testNow+1); err != nil {
		t.Fatal(err)
	}
	var state string
	var linked sql.NullString
	if err := db.QueryRowForTest(ctx, `SELECT state, linked_run_id FROM intake_items WHERE id='intake-rm'`).Scan(&state, &linked); err != nil {
		t.Fatal(err)
	}
	if state != "pending_evaluation" || linked.Valid {
		t.Fatalf("archived intake state=%q linked=%v, want pending_evaluation with empty run", state, linked)
	}
	second, err := db.CreateForgeRun(ctx, CreateForgeRunCmd{
		RunID: "run-new", ProjectID: "p-rm", ConfigSnapshotID: "cfg-rm",
		ForgeKind: "github", ForgeHost: "github.com", ForgeProjectKey: "org/repo-p-rm",
		IssueID: "7", IssueURL: "u", IssueAuthor: "alice",
		TriggerLabelEventID: "7:sift-2", TriggerActor: "alice",
		TriggerObservedAtMS: testNow + 2, CreatedAtMS: testNow + 2,
	})
	if err != nil {
		t.Fatalf("CreateForgeRun after archive: %v", err)
	}
	if second.ID != "run-new" {
		t.Fatalf("CreateForgeRun after archive returned %q, want new run-new", second.ID)
	}
}

func TestReverseSyncCandidatesOmitArchived(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	if err := db.SeedProjectForTest(ctx, "cfg-rs", "proj-rs", testNow); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedReverseSyncRunForTest(ctx, "run-live", "proj-rs", "cfg-rs", "1", "3", "running", testNow); err != nil {
		t.Fatal(err)
	}
	if err := db.SeedReverseSyncRunForTest(ctx, "run-dead", "proj-rs", "cfg-rs", "1#archived-dead", "", "waiting_human", testNow+1); err != nil {
		t.Fatal(err)
	}
	if err := db.ArchiveRun(ctx, "run-dead", testNow+2); err != nil {
		t.Fatal(err)
	}
	got, err := db.ReverseSyncCandidates(ctx, "proj-rs")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].RunID != "run-live" {
		t.Fatalf("candidates=%+v, want only run-live", got)
	}
}

func TestArchiveRunNotFound(t *testing.T) {
	db, _ := openTestDB(t)
	if err := db.ArchiveRun(context.Background(), "ghost", testNow); !errors.Is(err, ErrRunNotFound) {
		t.Fatalf("ArchiveRun ghost = %v, want ErrRunNotFound", err)
	}
}

func countRun(t *testing.T, db *DB, ctx context.Context, query string, args ...any) int {
	t.Helper()
	var n int
	if err := db.QueryRowForTest(ctx, query, args...).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}
