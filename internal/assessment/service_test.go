package assessment

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

type evaluatorFunc func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error)

func (function evaluatorFunc) EvaluateAssessment(ctx context.Context, input evaluator.AssessmentInput, selection evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
	return function(ctx, input, selection)
}

func TestServiceStart(t *testing.T) {
	t.Run("retains an owned bounded assessment context", func(t *testing.T) {
		var received evaluator.AssessmentInput
		service := testService(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
			received = input
			return completeTurn(input), nil
		}))
		input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
		descriptor, err := service.Start(input, questions, selection)
		if err != nil {
			t.Fatalf("Start() error = %v", err)
		}
		if !descriptor.Available || !ValidID(descriptor.ID) || descriptor.ExpiresAt != "2026-09-01T12:30:00Z" {
			t.Fatalf("Start() descriptor = %#v, want available deterministic descriptor", descriptor)
		}

		input.EvidenceBundle.Items[0].Content = "mutated"
		questions.Questions[0].Text = "mutated"
		_, err = service.Submit(context.Background(), descriptor.ID, initialSubmission())
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if strings.Contains(received.EvaluatorInput.EvidenceBundle.Items[0].Content, "mutated") ||
			strings.Contains(received.QuestionSet.Questions[0].Text, "mutated") {
			t.Fatalf("retained assessment aliases caller values: %#v", received)
		}
	})

	t.Run("reports explicit unavailable reasons without retaining state", func(t *testing.T) {
		input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
		withoutEvaluator := testService(nil)
		descriptor, err := withoutEvaluator.Start(input, questions, selection)
		if err != nil || descriptor.Available || descriptor.Reason != "evaluator_unavailable" {
			t.Fatalf("Start(no evaluator) = (%#v, %v), want evaluator_unavailable", descriptor, err)
		}
		insufficient := evaluator.QuestionSet{
			SchemaVersion: evaluator.QuestionSetSchemaVersion,
			Disposition:   evaluator.DispositionInsufficientEvidence,
			Questions:     []evaluator.Question{},
		}
		descriptor, err = withoutEvaluator.Start(input, insufficient, selection)
		if err != nil || descriptor.Available || descriptor.Reason != "insufficient_evidence" {
			t.Fatalf("Start(insufficient) = (%#v, %v), want insufficient_evidence", descriptor, err)
		}
	})

	t.Run("rejects invalid context and model selection", func(t *testing.T) {
		service := testService(evaluator.DeterministicAssessmentEvaluator{})
		input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
		input.EvidenceBundle.Items[0].Content = "changed without updating hash"
		if _, err := service.Start(input, questions, selection); !errors.Is(err, ErrInvalidStart) {
			t.Fatalf("Start(invalid input) error = %v, want ErrInvalidStart", err)
		}
		input, questions, selection = validStartContext(t, "func Validate() error { return nil }")
		selection.Provider = "-unsafe"
		if _, err := service.Start(input, questions, selection); !errors.Is(err, ErrInvalidStart) {
			t.Fatalf("Start(invalid selection) error = %v, want ErrInvalidStart", err)
		}
	})

	t.Run("enforces live count and byte capacity without eviction", func(t *testing.T) {
		service := testService(evaluator.DeterministicAssessmentEvaluator{})
		service.maxEntries = 1
		input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
		first, err := service.Start(input, questions, selection)
		if err != nil || !first.Available {
			t.Fatalf("first Start() = (%#v, %v), want available", first, err)
		}
		second, err := service.Start(input, questions, selection)
		if err != nil || second.Available || second.Reason != "capacity" {
			t.Fatalf("second Start() = (%#v, %v), want capacity", second, err)
		}
		if _, err := service.Submit(context.Background(), first.ID, initialSubmission()); err != nil {
			t.Fatalf("first retained assessment was evicted: %v", err)
		}

		service = testService(evaluator.DeterministicAssessmentEvaluator{})
		service.maxBytes = input.EvidenceBundle.ApproximateBytes
		first, err = service.Start(input, questions, selection)
		if err != nil || !first.Available {
			t.Fatalf("byte-limited first Start() = (%#v, %v), want available", first, err)
		}
		second, err = service.Start(input, questions, selection)
		if err != nil || second.Available || second.Reason != "capacity" {
			t.Fatalf("byte-limited second Start() = (%#v, %v), want capacity", second, err)
		}
	})

	t.Run("purges expired entries before admission", func(t *testing.T) {
		service := testService(evaluator.DeterministicAssessmentEvaluator{})
		service.maxEntries = 1
		input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
		first, err := service.Start(input, questions, selection)
		if err != nil || !first.Available {
			t.Fatalf("first Start() = (%#v, %v), want available", first, err)
		}
		service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 30, 0, 1, time.UTC) }
		second, err := service.Start(input, questions, selection)
		if err != nil || !second.Available || second.ID == first.ID {
			t.Fatalf("Start(after expiry) = (%#v, %v), want new available entry", second, err)
		}
		if _, err := service.Submit(context.Background(), first.ID, initialSubmission()); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Submit(expired ID) error = %v, want ErrUnavailable", err)
		}
	})
}

func TestServiceSubmit(t *testing.T) {
	t.Run("completes once and derives the public label", func(t *testing.T) {
		service := testService(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
			return completeTurn(input), nil
		}))
		descriptor := startAssessment(t, service)
		result, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
		if err != nil {
			t.Fatalf("Submit() error = %v", err)
		}
		if result.Turn.Disposition != evaluator.AssessmentDispositionComplete || result.Label != evaluator.AssessmentLabelPartial {
			t.Fatalf("Submit() = %#v, want complete partial result", result)
		}
		if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("Submit(replay) error = %v, want ErrUnavailable", err)
		}
	})

	t.Run("allows exactly one follow-up and then completes", func(t *testing.T) {
		service := testService(evaluator.DeterministicAssessmentEvaluator{RequestFollowUp: true})
		descriptor := startAssessment(t, service)
		first, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
		if err != nil || first.Turn.Disposition != evaluator.AssessmentDispositionFollowUp || first.Turn.FollowUp == nil {
			t.Fatalf("initial Submit() = (%#v, %v), want F1", first, err)
		}
		service.mu.Lock()
		retained := service.entries[descriptor.ID]
		service.mu.Unlock()
		if retained.input.SchemaVersion != 0 || retained.questions.SchemaVersion != 0 || retained.initial.SchemaVersion != evaluator.AssessmentInputSchemaVersion {
			t.Fatalf("follow-up state retained duplicate runtime context: %#v", retained)
		}
		if _, err := service.Submit(context.Background(), descriptor.ID, Submission{
			Stage:      evaluator.AssessmentStageFollowUpAnswer,
			FollowUpID: "F1",
		}); !errors.Is(err, ErrInvalidSubmission) {
			t.Fatalf("Submit(empty follow-up) error = %v, want ErrInvalidSubmission", err)
		}
		final, err := service.Submit(context.Background(), descriptor.ID, Submission{
			Stage:      evaluator.AssessmentStageFollowUpAnswer,
			FollowUpID: "F1",
			Answer:     "The empty-name branch returns ErrEmpty.",
		})
		if err != nil || final.Turn.Disposition != evaluator.AssessmentDispositionComplete || final.Label != evaluator.AssessmentLabelPartial {
			t.Fatalf("follow-up Submit() = (%#v, %v), want complete partial", final, err)
		}
	})

	t.Run("invalid input and pre-cancelled contexts do not consume state", func(t *testing.T) {
		service := testService(evaluator.DeterministicAssessmentEvaluator{})
		descriptor := startAssessment(t, service)
		invalid := initialSubmission()
		invalid.Answers[0].Text = ""
		if _, err := service.Submit(context.Background(), descriptor.ID, invalid); !errors.Is(err, ErrInvalidSubmission) {
			t.Fatalf("Submit(invalid) error = %v, want ErrInvalidSubmission", err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := service.Submit(ctx, descriptor.ID, initialSubmission()); !errors.Is(err, context.Canceled) {
			t.Fatalf("Submit(cancelled) error = %v, want context.Canceled", err)
		}
		if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); err != nil {
			t.Fatalf("valid Submit() after rejected attempts error = %v", err)
		}
	})

	t.Run("evaluator failures and invalid results make the assessment unavailable", func(t *testing.T) {
		tests := []struct {
			name      string
			evaluator evaluator.AssessmentEvaluator
		}{
			{name: "evaluator error", evaluator: evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
				return evaluator.AssessmentTurn{}, errors.New("synthetic evaluator failure")
			})},
			{name: "invalid turn", evaluator: evaluatorFunc(func(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
				return evaluator.AssessmentTurn{SchemaVersion: 1, Disposition: evaluator.AssessmentDispositionComplete}, nil
			})},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				service := testService(test.evaluator)
				descriptor := startAssessment(t, service)
				if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); err == nil {
					t.Fatal("Submit() error = nil, want evaluator failure")
				}
				if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); !errors.Is(err, ErrUnavailable) {
					t.Fatalf("Submit(after failure) error = %v, want ErrUnavailable", err)
				}
			})
		}
	})

	t.Run("concurrent submission starts one evaluator call", func(t *testing.T) {
		entered := make(chan struct{})
		release := make(chan struct{})
		var calls atomic.Int64
		service := testService(evaluatorFunc(func(_ context.Context, input evaluator.AssessmentInput, _ evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
			calls.Add(1)
			close(entered)
			<-release
			return completeTurn(input), nil
		}))
		descriptor := startAssessment(t, service)
		firstResult := make(chan error, 1)
		go func() {
			_, err := service.Submit(context.Background(), descriptor.ID, initialSubmission())
			firstResult <- err
		}()
		<-entered
		if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); !errors.Is(err, ErrUnavailable) {
			t.Fatalf("concurrent Submit() error = %v, want ErrUnavailable", err)
		}
		close(release)
		if err := <-firstResult; err != nil {
			t.Fatalf("winning Submit() error = %v", err)
		}
		if calls.Load() != 1 {
			t.Fatalf("evaluator calls = %d, want 1", calls.Load())
		}
	})
}

func TestServiceClose(t *testing.T) {
	service := testService(evaluator.DeterministicAssessmentEvaluator{})
	descriptor := startAssessment(t, service)
	service.Close()
	service.Close()
	if _, err := service.Submit(context.Background(), descriptor.ID, initialSubmission()); !errors.Is(err, ErrClosed) {
		t.Fatalf("Submit(after Close) error = %v, want ErrClosed", err)
	}
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	if _, err := service.Start(input, questions, selection); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start(after Close) error = %v, want ErrClosed", err)
	}
	if len(service.entries) != 0 || service.retainedBytes != 0 {
		t.Fatalf("Close() retained (%d entries, %d bytes)", len(service.entries), service.retainedBytes)
	}
	withoutEvaluator := testService(nil)
	withoutEvaluator.Close()
	if _, err := withoutEvaluator.Start(input, questions, selection); !errors.Is(err, ErrClosed) {
		t.Fatalf("Start(after Close without evaluator) error = %v, want ErrClosed", err)
	}
}

func testService(assessmentEvaluator evaluator.AssessmentEvaluator) *Service {
	service := New(assessmentEvaluator)
	service.now = func() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }
	var sequence atomic.Int64
	service.newID = func() (string, error) {
		return fmt.Sprintf("as1-%043d", sequence.Add(1)), nil
	}
	return service
}

func startAssessment(t *testing.T, service *Service) Descriptor {
	t.Helper()
	input, questions, selection := validStartContext(t, "func Validate() error { return nil }")
	descriptor, err := service.Start(input, questions, selection)
	if err != nil || !descriptor.Available {
		t.Fatalf("Start() = (%#v, %v), want available", descriptor, err)
	}
	return descriptor
}

func validStartContext(t *testing.T, excerpt string) (evaluator.Input, evaluator.QuestionSet, evaluator.ModelSelection) {
	t.Helper()
	result := evidence.Result{
		RepositoryRoot: "/private/synthetic-repository",
		BaseRevision:   strings.Repeat("a", 40),
		HeadRevision:   evidence.WorkingTreeRevision,
		AppliedLimits: evidence.Limits{
			MaxFiles:        5,
			MaxDeclarations: 10,
			MaxExcerptBytes: len(excerpt) + 1,
		},
		Files: []evidence.File{{
			Path:         "internal/validate.go",
			Status:       evidence.FileModified,
			ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
			Declarations: []evidence.Declaration{{
				Kind:         evidence.DeclarationFunction,
				Name:         "Validate",
				Identity:     "Validate",
				StartLine:    3,
				EndLine:      3,
				ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
				Excerpt:      excerpt,
			}},
		}},
	}
	bundle, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle(): %v", err)
	}
	input, err := evaluator.NewInput(bundle)
	if err != nil {
		t.Fatalf("NewInput(): %v", err)
	}
	questions, err := evaluator.ParseQuestionSet([]byte(`{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"Explain Validate.","evidence_references":["E001"]},{"id":"Q2","kind":"code_specific","text":"Which branch matters?","evidence_references":["E001"]},{"id":"Q3","kind":"go_backend","text":"How should this be tested?","evidence_references":[]}]}`), []string{"E001"})
	if err != nil {
		t.Fatalf("ParseQuestionSet(): %v", err)
	}
	return input, questions, evaluator.ModelSelection{
		PiVersion:     evaluator.SupportedPiVersion,
		Provider:      "provider",
		ModelID:       "model",
		ThinkingLevel: "off",
	}
}

func initialSubmission() Submission {
	return Submission{
		Stage: evaluator.AssessmentStageInitialAnswers,
		Answers: []evaluator.AssessmentAnswer{
			{QuestionID: "Q1", Text: "It returns nil for the selected branch."},
			{QuestionID: "Q2", Text: "The edge case follows the error branch."},
			{QuestionID: "Q3", Text: "A table-driven test covers both cases."},
		},
	}
}

func completeTurn(input evaluator.AssessmentInput) evaluator.AssessmentTurn {
	return evaluator.AssessmentTurn{
		SchemaVersion: evaluator.AssessmentTurnSchemaVersion,
		Disposition:   evaluator.AssessmentDispositionComplete,
		Evaluations: []evaluator.QuestionEvaluation{
			{QuestionID: "Q1", Verdict: evaluator.AssessmentVerdictDemonstrated, Feedback: "The answer identifies the selected behavior.", EvidenceReferences: append([]string(nil), input.QuestionSet.Questions[0].EvidenceReferences...)},
			{QuestionID: "Q2", Verdict: evaluator.AssessmentVerdictPartial, Feedback: "The answer omits one selected edge path.", EvidenceReferences: append([]string(nil), input.QuestionSet.Questions[1].EvidenceReferences...)},
			{QuestionID: "Q3", Verdict: evaluator.AssessmentVerdictNotDemonstrated, Feedback: "The answer needs a clearer testing explanation.", EvidenceReferences: []string{}},
		},
	}
}
