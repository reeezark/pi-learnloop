package history_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestOpenInitializesAndReopensProtectedStore(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	assertPermission(t, dataDir, 0o700)
	assertPermission(t, filepath.Join(dataDir, "history.db"), 0o600)

	store, err = history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open(existing store) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(existing store) error = %v", err)
	}
}

func TestOpenMarksRunningAttemptInterrupted(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	start := validStart(repositoryRoot)
	start.StartedAt = time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	recordID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open(existing store) error = %v", err)
	}
	defer store.Close()
	records, err := store.List(ctx, repositoryRoot, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 1 || records[0].RecordID != recordID || records[0].Status != history.StatusInterrupted || records[0].FinishedAt == nil {
		t.Fatalf("recovered records = %#v, want one interrupted record", records)
	}
	if records[0].FinishedAt.Before(start.StartedAt) || records[0].FailureCode != "" || records[0].Label != "" || len(records[0].Outcomes) != 0 {
		t.Fatalf("interrupted record contains invalid terminal data: %#v", records[0])
	}
}

func TestTerminalUpdatesAreIdempotentAndRejectConflicts(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	start := validStart(filepath.Join(t.TempDir(), "repository"))
	completion := history.Completion{
		FinishedAt: start.StartedAt.Add(time.Minute),
		Label:      history.LabelUnderstood,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictDemonstrated},
		},
	}
	recordID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Complete(ctx, recordID, completion); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if err := store.Complete(ctx, recordID, completion); err != nil {
		t.Fatalf("Complete(idempotent) error = %v", err)
	}
	conflict := completion
	conflict.Label = history.LabelPartial
	if err := store.Complete(ctx, recordID, conflict); !errors.Is(err, history.ErrConflict) {
		t.Fatalf("Complete(conflict) error = %v, want ErrConflict", err)
	}
	if err := store.Fail(ctx, recordID, history.Failure{FinishedAt: completion.FinishedAt, Code: history.FailureEvaluatorFailed}); !errors.Is(err, history.ErrConflict) {
		t.Fatalf("Fail(completed record) error = %v, want ErrConflict", err)
	}
	if err := store.MarkFollowUp(ctx, recordID); !errors.Is(err, history.ErrConflict) {
		t.Fatalf("MarkFollowUp(completed record) error = %v, want ErrConflict", err)
	}

	failedID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create(failed record) error = %v", err)
	}
	failure := history.Failure{FinishedAt: start.StartedAt.Add(2 * time.Minute), Code: history.FailureEvaluatorTimeout}
	if err := store.Fail(ctx, failedID, failure); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if err := store.Fail(ctx, failedID, failure); err != nil {
		t.Fatalf("Fail(idempotent) error = %v", err)
	}
	failure.Code = history.FailureEvaluatorFailed
	if err := store.Fail(ctx, failedID, failure); !errors.Is(err, history.ErrConflict) {
		t.Fatalf("Fail(conflict) error = %v, want ErrConflict", err)
	}
	if err := store.Complete(ctx, failedID, completion); !errors.Is(err, history.ErrConflict) {
		t.Fatalf("Complete(failed record) error = %v, want ErrConflict", err)
	}
}

func TestListIsRepositoryScopedNewestFirstAndBounded(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	first := validStart(repositoryRoot)
	second := first
	second.StartedAt = first.StartedAt.Add(time.Minute)
	firstID, err := store.Create(ctx, first)
	if err != nil {
		t.Fatalf("Create(first) error = %v", err)
	}
	secondID, err := store.Create(ctx, second)
	if err != nil {
		t.Fatalf("Create(second) error = %v", err)
	}
	other := validStart(filepath.Join(t.TempDir(), "other-repository"))
	if _, err := store.Create(ctx, other); err != nil {
		t.Fatalf("Create(other repository) error = %v", err)
	}

	records, err := store.List(ctx, repositoryRoot, 1)
	if err != nil {
		t.Fatalf("List(limit 1) error = %v", err)
	}
	if len(records) != 1 || records[0].RecordID != secondID {
		t.Fatalf("List(limit 1) = %#v, want newest record %q", records, secondID)
	}
	records, err = store.List(ctx, repositoryRoot, 2)
	if err != nil {
		t.Fatalf("List(limit 2) error = %v", err)
	}
	if len(records) != 2 || records[0].RecordID != secondID || records[1].RecordID != firstID {
		t.Fatalf("List(limit 2) = %#v, want newest-first repository records", records)
	}
	if _, err := store.List(ctx, repositoryRoot, 0); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("List(limit 0) error = %v, want ErrInvalid", err)
	}
	if _, err := store.List(ctx, repositoryRoot, 51); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("List(limit 51) error = %v, want ErrInvalid", err)
	}
}

func TestCompleteAttemptSurvivesReopen(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	start := validStart(repositoryRoot)
	recordID, err := store.Create(context.Background(), start)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if !history.ValidRecordID(recordID) {
		t.Fatalf("Create() record ID = %q, want lr1 record ID", recordID)
	}
	if err := store.MarkFollowUp(context.Background(), recordID); err != nil {
		t.Fatalf("MarkFollowUp() error = %v", err)
	}
	finishedAt := start.StartedAt.Add(2 * time.Minute)
	completion := history.Completion{
		FinishedAt: finishedAt,
		Label:      history.LabelPartial,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictPartial},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictNotDemonstrated},
		},
	}
	if err := store.Complete(context.Background(), recordID, completion); err != nil {
		t.Fatalf("Complete() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open(existing store) error = %v", err)
	}
	defer store.Close()
	records, err := store.List(context.Background(), repositoryRoot, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("List() records = %#v, want one record", records)
	}
	record := records[0]
	if record.RecordID != recordID || record.Status != history.StatusComplete || record.Label != history.LabelPartial || !record.FollowUpUsed {
		t.Fatalf("persisted terminal identity = %#v, want completed follow-up record", record)
	}
	if record.FinishedAt == nil || !record.FinishedAt.Equal(finishedAt) {
		t.Fatalf("persisted FinishedAt = %v, want %v", record.FinishedAt, finishedAt)
	}
	if record.Start != start {
		t.Fatalf("persisted Start = %#v, want %#v", record.Start, start)
	}
	if !reflect.DeepEqual(record.Outcomes, completion.Outcomes) {
		t.Fatalf("persisted Outcomes = %#v, want %#v", record.Outcomes, completion.Outcomes)
	}
}

func validStart(repositoryRoot string) history.Start {
	return history.Start{
		CanonicalRoot:           repositoryRoot,
		StartedAt:               time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		BaseRevision:            strings.Repeat("a", 40),
		HeadRevision:            "WORKTREE",
		EvidenceManifestSHA256:  strings.Repeat("b", 64),
		QuestionSchemaVersion:   1,
		AssessmentSchemaVersion: 1,
		QuestionPrompt: history.PromptProvenance{
			ID:      "evaluator-question-generation",
			Version: "1.0.0",
			SHA256:  strings.Repeat("c", 64),
		},
		AssessmentPrompt: history.PromptProvenance{
			ID:      "evaluator-answer-assessment",
			Version: "1.0.0",
			SHA256:  strings.Repeat("d", 64),
		},
		PiVersion:     "0.84.3",
		Provider:      "anthropic",
		ModelID:       "claude-sonnet-4",
		ThinkingLevel: "medium",
	}
}

func assertPermission(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat(%q) error = %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("permission(%q) = %#o, want %#o", path, got, want)
	}
}
