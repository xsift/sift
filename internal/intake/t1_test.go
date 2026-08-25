package intake

import (
	"context"
	"testing"
	"time"

	"github.com/xsift/sift/internal/brain"
	"github.com/xsift/sift/internal/config"
	"github.com/xsift/sift/internal/forge"
	"github.com/xsift/sift/internal/storage"
)

func seedPendingIssue(t *testing.T, db *storage.DB, project Project, issueID string) {
	t.Helper()
	if err := db.PersistIntakeBatch(context.Background(), storage.PersistIntakeBatchCmd{
		ProjectID: project.ID, Stream: "issues", Cursor: "next", NowMS: reconcilerNow,
		Items: []storage.IntakeItemInput{{
			IssueID: issueID, IssueURL: "https://example.test/" + issueID, IssueDigest: "issue-" + issueID,
			ForgeKind: string(project.Ref.Kind), Host: project.Ref.Host, ProjectKey: project.Ref.ProjectKey,
			EventID: "label:" + issueID, EventKind: "trigger_label_added", Actor: "operator",
			ObservedAtMS: reconcilerNow, RawDigest: "event-" + issueID,
		}},
	}); err != nil {
		t.Fatal(err)
	}
}

func t1Evaluator(t *testing.T, db *storage.DB, provider *brain.FakeProvider) *T1Evaluator {
	t.Helper()
	now := time.UnixMilli(reconcilerNow)
	return &T1Evaluator{
		DB: db,
		Brain: brain.NewShell(db, config.Brain{
			Executable: "fake", DailyTokenLimit: 1000, CallTimeout: time.Minute,
			SchemaRetries: 1, MaxInputBytes: 1 << 20, MaxRawOutputBytes: 1 << 20,
		}, provider, func() time.Time { return now }),
		Now: func() time.Time { return now },
	}
}

func TestEvaluateIssueT2FallbackFailsUnassignedRun(t *testing.T) {
	db, project := reconcilerDB(t, "t2-fallback")
	project.T2Agents = []brain.T2AgentCandidate{{ID: "pi", Capabilities: []string{"code"}}}
	seedPendingIssue(t, db, project, "2")
	eval := t1Evaluator(t, db, &brain.FakeProvider{Responses: []brain.FakeResponse{
		{ResultText: brain.ValidT1ResultText(), InputTokens: 1, OutputTokens: 1},
		{SpawnErr: true},
		{SpawnErr: true},
	}})
	if err := eval.EvaluateIssue(context.Background(), project, forge.Issue{
		ID: "2", Title: "t2", Body: "body", Author: "alice", URL: "https://example.test/2",
	}); err != nil {
		t.Fatalf("EvaluateIssue: %v", err)
	}
	var status, reason, agent string
	if err := db.QueryRowForTest(context.Background(), `SELECT status, COALESCE(failure_reason,''), COALESCE(agent_id,'') FROM runs WHERE issue_id='2'`).Scan(&status, &reason, &agent); err != nil {
		t.Fatal(err)
	}
	if status != string(storage.RunFailed) || reason != FailureT2AssignmentUnavailable || agent != "" {
		t.Fatalf("run status=%s reason=%s agent=%q, want failed/%s/empty", status, reason, agent, FailureT2AssignmentUnavailable)
	}
}
