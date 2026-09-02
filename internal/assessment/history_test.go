package assessment

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestServiceHistoryStoresPiSessionOutsideEvaluatorValues(t *testing.T) {
	store := openHistoryStore(t)
	provenance := validHistoryProvenance()
	provenance.PiSessionID = "session-model-isolation-123"
	service := testServiceWithHistory(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, selection evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		modelValues, err := json.Marshal(struct {
			Input     evaluator.AssessmentInput
			Selection evaluator.ModelSelection
		}{Input: input, Selection: selection})
		if err != nil {
			t.Fatalf("Marshal(model values): %v", err)
		}
		if strings.Contains(string(modelValues), provenance.PiSessionID) {
			t.Fatalf("model-visible values contain Pi Session identity: %s", modelValues)
		}
		reviewed, err := store.ReviewedPiSessionIDs(context.Background(), provenance.CanonicalRoot, []string{provenance.PiSessionID})
		if err != nil || len(reviewed) != 0 {
			t.Fatalf("ReviewedPiSessionIDs(running) = (%#v, %v), want empty", reviewed, err)
		}
		return completeTurn(input), nil
	}), store)

	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection, provenance)
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}
	result, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
	if err != nil || !result.History.Saved {
		t.Fatalf("Submit() = (%#v, %v), want saved completion", result, err)
	}
	reviewed, err := store.ReviewedPiSessionIDs(context.Background(), provenance.CanonicalRoot, []string{provenance.PiSessionID})
	if err != nil || len(reviewed) != 1 || reviewed[0] != provenance.PiSessionID {
		t.Fatalf("ReviewedPiSessionIDs(complete) = (%#v, %v), want Session identity", reviewed, err)
	}
	records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	genericHistory, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("Marshal(generic history): %v", err)
	}
	if strings.Contains(string(genericHistory), provenance.PiSessionID) {
		t.Fatalf("generic history contains Pi Session identity: %s", genericHistory)
	}
}

func TestServiceRejectsInvalidPiSessionProvenanceBeforeEvaluation(t *testing.T) {
	var calls atomic.Int64
	service := testServiceWithHistory(evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		calls.Add(1)
		return evaluator.AssessmentTurn{}, nil
	}), openHistoryStore(t))
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	provenance := validHistoryProvenance()
	provenance.PiSessionID = "private/session"
	if descriptor, err := service.Start(input, questions, selection, provenance); descriptor != (Descriptor{}) || !errors.Is(err, ErrInvalidStart) {
		t.Fatalf("Start(invalid provenance) = (%#v, %v), want ErrInvalidStart", descriptor, err)
	}
	if calls.Load() != 0 {
		t.Fatalf("evaluator calls = %d, want 0", calls.Load())
	}
}

func TestServiceHistoryCompleteLifecycle(t *testing.T) {
	store := openHistoryStore(t)
	provenance := validHistoryProvenance()
	var calls atomic.Int64
	service := testServiceWithHistory(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		calls.Add(1)
		records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
		if err != nil {
			t.Fatalf("List(running): %v", err)
		}
		if len(records) != 1 || records[0].Status != history.StatusRunning {
			t.Fatalf("records before evaluator = %#v, want one running record", records)
		}
		return completeTurn(input), nil
	}), store)

	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection, provenance)
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}
	result, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
	if err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if calls.Load() != 1 || !result.History.Saved || !history.ValidRecordID(result.History.RecordID) || result.History.Reason != "" {
		t.Fatalf("Submit() result = %#v, calls = %d", result, calls.Load())
	}

	records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
	if err != nil {
		t.Fatalf("List(complete): %v", err)
	}
	if len(records) != 1 {
		t.Fatalf("records = %#v, want one complete record", records)
	}
	record := records[0]
	if record.RecordID != result.History.RecordID || record.Status != history.StatusComplete || record.Label != history.LabelPartial || record.FollowUpUsed || len(record.Outcomes) != 3 {
		t.Fatalf("record = %#v, want one direct complete result", record)
	}
	if record.Start.CanonicalRoot != provenance.CanonicalRoot ||
		record.Start.BaseRevision != input.EvidenceBundle.BaseRevision ||
		record.Start.HeadRevision != input.EvidenceBundle.HeadRevision ||
		record.Start.EvidenceManifestSHA256 != input.EvidenceBundle.ManifestSHA256 ||
		record.Start.QuestionSchemaVersion != evaluator.QuestionSetSchemaVersion ||
		record.Start.AssessmentSchemaVersion != evaluator.AssessmentTurnSchemaVersion ||
		record.Start.QuestionPrompt != provenance.QuestionPrompt ||
		record.Start.AssessmentPrompt != provenance.AssessmentPrompt ||
		record.Start.PiVersion != selection.PiVersion ||
		record.Start.Provider != selection.Provider ||
		record.Start.ModelID != selection.ModelID ||
		record.Start.ThinkingLevel != selection.ThinkingLevel {
		t.Fatalf("record provenance = %#v, want server-owned assessment provenance", record.Start)
	}
}

func TestServiceHistoryFollowUpUsesOneRecord(t *testing.T) {
	store := openHistoryStore(t)
	provenance := validHistoryProvenance()
	service := testServiceWithHistory(evaluator.DeterministicAssessmentEvaluator{RequestFollowUp: true}, store)
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection, provenance)
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}

	first, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
	if err != nil || first.Turn.Disposition != evaluator.AssessmentDispositionFollowUp {
		t.Fatalf("initial Submit() = (%#v, %v), want follow-up", first, err)
	}
	records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
	if err != nil || len(records) != 1 || records[0].Status != history.StatusRunning || !records[0].FollowUpUsed {
		t.Fatalf("records after F1 = (%#v, %v), want one marked running record", records, err)
	}
	recordID := records[0].RecordID

	final, err := service.Submit(context.Background(), descriptor.ID, Submission{
		Stage:      evaluator.AssessmentStageFollowUpAnswer,
		FollowUpID: "F1",
		Answer:     "The empty-name branch returns ErrEmpty.",
	})
	if err != nil || !final.History.Saved || final.History.RecordID != recordID {
		t.Fatalf("follow-up Submit() = (%#v, %v), want saved original record", final, err)
	}
	records, err = store.List(context.Background(), provenance.CanonicalRoot, 10)
	if err != nil || len(records) != 1 || records[0].Status != history.StatusComplete || !records[0].FollowUpUsed || len(records[0].Outcomes) != 3 {
		t.Fatalf("final records = (%#v, %v), want one completed F1 record", records, err)
	}
}

func TestServiceHistoryFailuresUseSafeCodes(t *testing.T) {
	tests := []struct {
		name      string
		evaluator evaluator.AssessmentEvaluator
		want      history.FailureCode
	}{
		{
			name: "evaluator failure",
			evaluator: evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
				return evaluator.AssessmentTurn{}, errors.New("SECRET_PROVIDER_FAILURE")
			}),
			want: history.FailureEvaluatorFailed,
		},
		{
			name: "invalid output",
			evaluator: evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
				return evaluator.AssessmentTurn{SchemaVersion: 1, Disposition: evaluator.AssessmentDispositionComplete}, nil
			}),
			want: history.FailureEvaluatorInvalidOutput,
		},
		{
			name: "timeout",
			evaluator: evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
				return evaluator.AssessmentTurn{}, context.DeadlineExceeded
			}),
			want: history.FailureEvaluatorTimeout,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			store := openHistoryStore(t)
			provenance := validHistoryProvenance()
			service := testServiceWithHistory(test.evaluator, store)
			input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
			descriptor, err := service.Start(input, questions, selection, provenance)
			if err != nil || !descriptor.Available {
				t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
			}
			if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); err == nil {
				t.Fatal("Submit() error = nil, want evaluator error")
			}
			records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
			if err != nil || len(records) != 1 || records[0].Status != history.StatusFailed || records[0].FailureCode != test.want {
				t.Fatalf("records = (%#v, %v), want safe failure %q", records, err, test.want)
			}
		})
	}
}

func TestServiceHistoryRecordsCancellationWithoutRetry(t *testing.T) {
	store := openHistoryStore(t)
	provenance := validHistoryProvenance()
	entered := make(chan struct{})
	var calls atomic.Int64
	service := testServiceWithHistory(evaluatorFunc(func(ctx context.Context, _ evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		calls.Add(1)
		close(entered)
		<-ctx.Done()
		return evaluator.AssessmentTurn{}, ctx.Err()
	}), store)
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection, provenance)
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() {
		_, err := service.Submit(ctx, descriptor.ID, initialSubmission())
		result <- err
	}()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("Submit() error = %v, want context.Canceled", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("evaluator calls = %d, want 1", calls.Load())
	}
	records, err := store.List(context.Background(), provenance.CanonicalRoot, 10)
	if err != nil || len(records) != 1 || records[0].Status != history.StatusFailed || records[0].FailureCode != history.FailureEvaluatorFailed {
		t.Fatalf("records after cancellation = (%#v, %v), want one safe failed record", records, err)
	}
}

func TestServiceHistoryFailureDoesNotHideSuccessfulAssessment(t *testing.T) {
	store := openHistoryStore(t)
	var calls atomic.Int64
	service := testServiceWithHistory(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		calls.Add(1)
		if err := store.Close(); err != nil {
			t.Fatalf("Close(history store): %v", err)
		}
		return completeTurn(input), nil
	}), store)
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection, validHistoryProvenance())
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}

	result, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
	if err != nil {
		t.Fatalf("Submit() error = %v, want successful assessment", err)
	}
	if result.Turn.Disposition != evaluator.AssessmentDispositionComplete || result.Label != evaluator.AssessmentLabelPartial || result.History.Saved || result.History.RecordID != "" || result.History.Reason != HistoryStorageUnavailable {
		t.Fatalf("Submit() = %#v, want result plus storage_unavailable", result)
	}
	if calls.Load() != 1 {
		t.Fatalf("evaluator calls = %d, want 1", calls.Load())
	}
}

func TestServiceHistoryPersistsNoExcludedContent(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	service := testServiceWithHistory(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
		turn := completeTurn(input)
		turn.Evaluations[0].Feedback = "SECRET_FEEDBACK_SENTINEL"
		return turn, nil
	}), store)
	input, questions, selection := validStartContext(t, "func Validate() error { /* SECRET_SOURCE_SENTINEL */ return nil }")
	descriptor, err := service.Start(input, questions, selection, validHistoryProvenance())
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}
	submission := initialSubmission()
	submission.Answers[0].Text = "SECRET_ANSWER_SENTINEL"
	if _, err := service.Submit(context.Background(), descriptor.ID, submission); err != nil {
		t.Fatalf("Submit() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	entries, err := os.ReadDir(dataDir)
	if err != nil {
		t.Fatalf("ReadDir(data): %v", err)
	}
	var persisted strings.Builder
	for _, item := range entries {
		content, err := os.ReadFile(filepath.Join(dataDir, item.Name()))
		if err != nil {
			t.Fatalf("ReadFile(%q): %v", item.Name(), err)
		}
		persisted.Write(content)
	}
	for _, excluded := range []string{
		"SECRET_SOURCE_SENTINEL",
		"SECRET_ANSWER_SENTINEL",
		"SECRET_FEEDBACK_SENTINEL",
		"Explain Validate.",
		"pending assessment answer",
	} {
		if strings.Contains(persisted.String(), excluded) {
			t.Fatalf("history storage contains excluded content %q", excluded)
		}
	}
}

func openHistoryStore(t *testing.T) *history.Store {
	t.Helper()
	store, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() {
		if err := store.Close(); err != nil {
			t.Errorf("history.Close(): %v", err)
		}
	})
	return store
}

func testServiceWithHistory(assessmentEvaluator evaluator.AssessmentEvaluator, store *history.Store) *Service {
	service := New(assessmentEvaluator, store)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	var sequence atomic.Int64
	service.newID = func() (string, error) {
		return "as1-" + strings.Repeat("0", 42) + string(rune('0'+sequence.Add(1))), nil
	}
	return service
}

func validHistoryProvenance() Provenance {
	return Provenance{
		CanonicalRoot: "/private/synthetic-repository",
		QuestionPrompt: history.PromptProvenance{
			ID:      "evaluator-question-generation",
			Version: "1.0.0",
			SHA256:  strings.Repeat("1", 64),
		},
		AssessmentPrompt: history.PromptProvenance{
			ID:      "evaluator-answer-assessment",
			Version: "1.0.0",
			SHA256:  strings.Repeat("2", 64),
		},
	}
}
