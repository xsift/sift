package intake

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"time"

	"github.com/xsift/sift/internal/brain"
	"github.com/xsift/sift/internal/forge"
	"github.com/xsift/sift/internal/schema"
	"github.com/xsift/sift/internal/storage"
	"github.com/xsift/sift/internal/worktree"
)

type T1Evaluator struct {
	DB    *storage.DB
	Brain *brain.Shell
	Now   func() time.Time
}

// FailureT2AssignmentUnavailable is the closed runs.failure_reason used when
// T1 already created a queued Run but T2 could not assign an agent. The CHECK
// list has no T2-specific token, so this reuses contract_violation.
const FailureT2AssignmentUnavailable = "contract_violation"

// T2RetryAfter is how long a failed unassigned Run waits before the daemon
// retries T2. sift retry ignores this and tries immediately.
const T2RetryAfter = time.Minute

// EvaluateIssue wires a normalized Forge Issue into T1. Provider disabled or
// unavailable is intentionally not a drop: the shell's deterministic fallback
// is persisted as ready and the Issue is enqueued through PersistIntakeDecision.
func (e *T1Evaluator) EvaluateIssue(ctx context.Context, project Project, issue forge.Issue) error {
	item, err := e.DB.FindPendingIntake(ctx, project.ID, issue.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil // already consumed or not yet intake'd
		}
		return err
	}
	input, err := brain.BuildT1Input(brain.T1Input{Forge: brain.T1Forge{Kind: string(project.Ref.Kind), Host: project.Ref.Host, ProjectKey: project.Ref.ProjectKey}, Issue: brain.T1Issue{ID: issue.ID, Title: issue.Title, Body: issue.Body, Author: issue.Author, URL: issue.URL, Labels: issue.Labels}, KnownCandidates: []brain.T1Candidate{}})
	if err != nil {
		return err
	}
	now := e.now()
	result, err := e.Brain.Call(ctx, brain.T1Contract(nil), brain.CallParams{Scope: "intake", SubjectKey: fmt.Sprintf("forge:%s:%s:%s:issue:%s", project.Ref.Kind, project.Ref.Host, project.Ref.ProjectKey, issue.ID), ProjectID: project.ID, Input: input})
	if err != nil {
		return err
	}
	var out struct {
		Disposition string   `json:"disposition"`
		Questions   []string `json:"questions"`
		Possible    *string  `json:"possible_duplicate_run_id"`
		Rationale   string   `json:"rationale"`
	}
	if err = json.Unmarshal(result.Output, &out); err != nil {
		return err
	}
	q, _ := json.Marshal(out.Questions)
	runID := storage.NewID()
	if err := e.DB.PersistIntakeDecision(ctx, storage.IntakeDecisionCmd{IntakeID: item.ID, AssessmentID: storage.NewID(), LogicalCallID: result.CallID, ExpectedVersion: item.Version, Disposition: out.Disposition, QuestionsJSON: string(q), PossibleDuplicateRunID: out.Possible, Rationale: out.Rationale, NowMS: now.UnixMilli(), RunID: runID}); err != nil {
		return err
	}
	if out.Disposition != string(brain.T1Ready) {
		return nil
	}
	return e.assignT2(ctx, project, issue, runID)
}

// RetryAssignment re-runs T2 on a failed unassigned Run. The issue slot stays
// on this Run; the operator does not need to archive or re-label.
func (e *T1Evaluator) RetryAssignment(ctx context.Context, project Project, issue forge.Issue, runID string) error {
	run, err := e.DB.Run(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != storage.RunFailed || run.AgentID != "" || run.FailureReason != FailureT2AssignmentUnavailable {
		return nil
	}
	if _, err := e.DB.TransitionRun(ctx, runID, run.Version, storage.DomainCommand{
		To: storage.RunQueued, Source: storage.SourceSystem, OccurredAtMS: e.now().UnixMilli(),
	}); err != nil {
		return err
	}
	return e.assignT2(ctx, project, issue, runID)
}

func (e *T1Evaluator) assignT2(ctx context.Context, project Project, issue forge.Issue, runID string) error {
	now := e.now()
	run, err := e.DB.Run(ctx, runID)
	if err != nil {
		return err
	}
	candidateIDs := make([]string, 0, len(project.T2Agents))
	for _, candidate := range project.T2Agents {
		candidateIDs = append(candidateIDs, candidate.ID)
	}
	t2Input, err := brain.BuildT2Input(brain.T2Input{
		RunID:           runID,
		Issue:           brain.T2Issue{Title: issue.Title, Body: issue.Body, URL: issue.URL},
		CandidateAgents: project.T2Agents,
		BaseContext:     brain.T2BaseContext{},
	})
	if err != nil {
		return e.failUnassigned(ctx, runID, now)
	}
	t2result, err := e.Brain.Call(ctx, brain.T2Contract(candidateIDs), brain.CallParams{
		Scope: "run", SubjectKey: "run:" + runID, ProjectID: project.ID, RunID: runID, Input: t2Input,
	})
	if err != nil {
		_ = e.failUnassigned(ctx, runID, now)
		return err
	}
	if t2result.Status != storage.BrainCallValid || len(t2result.Output) == 0 {
		return e.failUnassigned(ctx, runID, now)
	}
	var t2out brain.T2Output
	if err := schema.Decode(t2result.Output, &t2out, schema.Closed); err != nil {
		return e.failUnassigned(ctx, runID, now)
	}
	if t2out.Kind == nil || t2out.Agent == nil || t2out.HITLBeforeStart == nil || t2out.Goals == nil || t2out.Rationale == nil {
		return e.failUnassigned(ctx, runID, now)
	}
	backend := project.AgentBackends[*t2out.Agent]
	if backend == "" {
		backend = "process"
	}
	assignment := storage.CommitT2AssignmentCmd{
		RunID: runID, ExpectedVersion: run.Version, Kind: string(*t2out.Kind), AgentID: *t2out.Agent,
		HITLBeforeStart: *t2out.HITLBeforeStart, Backend: backend, NowMS: now.UnixMilli(),
	}
	var worktrees *worktree.Manager
	var created worktree.Worktree
	if project.Repo != "" {
		worktrees, err = worktree.NewManager(project.Repo, filepath.Join(project.Repo, ".sift-worktrees"))
		if err != nil {
			_ = e.failUnassigned(ctx, runID, now)
			return err
		}
		created, err = worktrees.Create(ctx, runID, 1, "HEAD", "sift/"+runID)
		if err != nil {
			_ = e.failUnassigned(ctx, runID, now)
			return err
		}
		canonical, digest, assembleErr := brain.AssembleTaskSpec(brain.TaskSpecParams{
			Title: issue.Title, Body: issue.Body, SourceURL: issue.URL, Goals: *t2out.Goals,
			PolicyHash: project.ID, Kind: *t2out.Kind, Agent: *t2out.Agent,
			HITLBeforeStart: *t2out.HITLBeforeStart, LogicalCallID: t2result.CallID,
			PromptVersion: brain.T2Asset().PromptVersion,
		})
		if assembleErr != nil {
			_ = worktrees.Remove(ctx, created)
			_ = e.failUnassigned(ctx, runID, now)
			return assembleErr
		}
		assignment.TaskSpecID = storage.NewID()
		assignment.TaskSpecJSON = canonical
		assignment.TaskSpecDigest = digest
		assignment.InitialAttempt = &storage.InitialAttemptSpec{
			WorktreePath: created.Path, BranchName: created.Branch, BaseRef: created.Base, BaseSHA: created.Base,
		}
	}
	if _, err = e.DB.CommitT2Assignment(ctx, assignment); err != nil {
		if worktrees != nil && created.Path != "" {
			_ = worktrees.Remove(ctx, created)
		}
		_ = e.failUnassigned(ctx, runID, now)
		return err
	}
	return nil
}

func (e *T1Evaluator) failUnassigned(ctx context.Context, runID string, now time.Time) error {
	run, err := e.DB.Run(ctx, runID)
	if err != nil {
		return err
	}
	if run.Status != storage.RunQueued {
		return nil
	}
	_, err = e.DB.TransitionRun(ctx, runID, run.Version, storage.DomainCommand{
		To: storage.RunFailed, Source: storage.SourceSystem,
		FailureReason: FailureT2AssignmentUnavailable, OccurredAtMS: now.UnixMilli(),
	})
	return err
}

func (e *T1Evaluator) now() time.Time {
	if e.Now != nil {
		if n := e.Now(); !n.IsZero() {
			return n
		}
	}
	return time.UnixMilli(1)
}
