package storage

import (
	"context"
	"testing"
)

func TestUnassignedT2FailureRoundTrip(t *testing.T) {
	db, _ := openTestDB(t)
	ctx := context.Background()
	insertConfigSnapshot(t, db, "cfg")
	insertProject(t, db, "p", "cfg")
	if err := db.SeedForgeRunForTest(ctx, "run-t2", "p", "cfg", "2", testNow); err != nil {
		t.Fatal(err)
	}
	if _, err := db.TransitionRun(ctx, "run-t2", 1, DomainCommand{
		To: RunFailed, Source: SourceSystem, FailureReason: "contract_violation", OccurredAtMS: testNow + 1,
	}); err != nil {
		t.Fatal(err)
	}
	row, ok, err := db.UnassignedT2Failure(ctx, "run-t2")
	if err != nil || !ok || row.IssueID != "2" || row.ProjectID != "p" {
		t.Fatalf("lookup = %#v ok=%v err=%v", row, ok, err)
	}
	due, err := db.UnassignedT2Failures(ctx, testNow+1)
	if err != nil || len(due) != 1 || due[0].RunID != "run-t2" {
		t.Fatalf("due = %#v err=%v", due, err)
	}
	if early, err := db.UnassignedT2Failures(ctx, testNow); err != nil || len(early) != 0 {
		t.Fatalf("not-yet-due = %#v err=%v", early, err)
	}
}
