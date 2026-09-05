package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestNewPiRPCAssessmentEvaluator(t *testing.T) {
	fake := installFakePi(t, "assessment_complete")
	adapter, err := NewPiRPCAssessmentEvaluator(context.Background(), prompts.EvaluatorAnswerAssessmentV1())
	if err != nil {
		t.Fatalf("NewPiRPCAssessmentEvaluator(): %v", err)
	}
	if adapter == nil {
		t.Fatal("NewPiRPCAssessmentEvaluator() = nil, want production adapter")
	}
	if _, err := os.Stat(fake.startsPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("constructor started an assessment process: %v", err)
	}

	if _, err := NewPiRPCAssessmentEvaluator(context.Background(), ""); err == nil {
		t.Fatal("NewPiRPCAssessmentEvaluator(empty prompt) error = nil")
	}
}

func TestPiRPCAssessmentEvaluatorEvaluateAssessment(t *testing.T) {
	selection := syntheticModelSelection()

	t.Run("sends the exact initial assessment in one isolated process", func(t *testing.T) {
		fake := installFakePi(t, "assessment_complete")
		adapter := mustNewFakeAssessmentEvaluator(t)
		input := syntheticInitialAssessmentInput(t)
		result, err := adapter.EvaluateAssessment(context.Background(), input, selection)
		if err != nil {
			t.Fatalf("EvaluateAssessment(): %v", err)
		}
		if result.Disposition != AssessmentDispositionComplete || len(result.Evaluations) != 3 {
			t.Fatalf("result = %#v, want complete Q1/Q2/Q3 assessment", result)
		}
		assertFakeProcessGone(t, fake.pidPath)

		arguments := readFakeArguments(t, fake.argumentsPath)
		for _, argument := range arguments {
			if strings.Contains(argument, "synthetic source") || strings.Contains(argument, "first answer") {
				t.Fatalf("argv contains source or an answer: %#v", arguments)
			}
		}
		request := readFakeWorkerRequest(t, fake.requestsPath, 1)
		message := request.Message
		if request.SystemPrompt != prompts.EvaluatorAnswerAssessmentV1() || request.Model == nil || request.Model.Provider != selection.Provider {
			t.Fatalf("worker request = %#v, want exact assessment prompt and model", request)
		}
		var sent AssessmentInput
		if err := json.Unmarshal([]byte(message), &sent); err != nil {
			t.Fatalf("prompt message is not assessment JSON: %v", err)
		}
		if sent.Stage != AssessmentStageInitialAnswers || sent.Answers[0].Text != "first answer" || sent.EvaluatorInput.EvidenceBundle.Items[0].Content != "synthetic source" {
			t.Fatalf("sent input = %#v, want exact source-bearing initial assessment", sent)
		}
	})

	t.Run("uses a fresh process and exact follow-up stage", func(t *testing.T) {
		fake := installFakePi(t, "assessment_follow_up")
		adapter := mustNewFakeAssessmentEvaluator(t)
		initial := syntheticInitialAssessmentInput(t)
		first, err := adapter.EvaluateAssessment(context.Background(), initial, selection)
		if err != nil || first.Disposition != AssessmentDispositionFollowUp || first.FollowUp == nil {
			t.Fatalf("initial EvaluateAssessment() = (%#v, %v), want F1", first, err)
		}
		followUp, err := NewFollowUpAssessmentInput(initial, *first.FollowUp, "The selected branch returns the expected error.")
		if err != nil {
			t.Fatalf("NewFollowUpAssessmentInput(): %v", err)
		}
		final, err := adapter.EvaluateAssessment(context.Background(), followUp, selection)
		if err != nil || final.Disposition != AssessmentDispositionComplete {
			t.Fatalf("follow-up EvaluateAssessment() = (%#v, %v), want complete", final, err)
		}
		starts, err := os.ReadFile(fake.startsPath)
		if err != nil {
			t.Fatalf("ReadFile(starts): %v", err)
		}
		if got := len(strings.Fields(string(starts))); got != 2 {
			t.Fatalf("assessment process starts = %d, want 2", got)
		}
		request := readFakeWorkerRequest(t, fake.requestsPath, 2)
		var sent AssessmentInput
		if err := json.Unmarshal([]byte(request.Message), &sent); err != nil || sent.Stage != AssessmentStageFollowUpAnswer || sent.FollowUp == nil || sent.FollowUp.Answer == "" {
			t.Fatalf("second prompt = %#v, want exact follow-up input (error = %v)", sent, err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	for _, test := range []struct {
		name     string
		scenario string
		contract ContractErrorCode
	}{
		{name: "rejects malformed assistant JSON", scenario: "assessment_invalid_output", contract: ContractErrorInvalidOutput},
		{name: "rejects an unknown assessment field", scenario: "assessment_invalid_schema", contract: ContractErrorInvalidOutput},
		{name: "rejects an unknown evidence reference", scenario: "assessment_unknown_reference", contract: ContractErrorInvalidOutput},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := installFakePi(t, test.scenario)
			_, err := mustNewFakeAssessmentEvaluator(t).EvaluateAssessment(context.Background(), syntheticInitialAssessmentInput(t), selection)
			if ContractErrorCodeOf(err) != test.contract {
				t.Fatalf("EvaluateAssessment() error = %v, want %s", err, test.contract)
			}
			assertFakeProcessGone(t, fake.pidPath)
		})
	}

	t.Run("enforces the final assistant-text cap before schema parsing", func(t *testing.T) {
		fake := installFakePi(t, "assessment_oversized_output")
		_, err := mustNewFakeAssessmentEvaluator(t).EvaluateAssessment(context.Background(), syntheticInitialAssessmentInput(t), selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	for _, test := range []struct {
		name     string
		scenario string
	}{
		{name: "rejects invalid RPC JSON", scenario: "invalid_json"},
		{name: "rejects an extra response frame", scenario: "extra_frame"},
		{name: "rejects an unknown response shape", scenario: "unknown_response"},
		{name: "keeps child authentication errors opaque", scenario: "auth_failure"},
		{name: "enforces the stdout cap", scenario: "stdout_cap"},
		{name: "enforces the stderr cap", scenario: "stderr_cap"},
		{name: "reports early child exit", scenario: "child_exit"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fake := installFakePi(t, test.scenario)
			_, err := mustNewFakeAssessmentEvaluator(t).EvaluateAssessment(context.Background(), syntheticInitialAssessmentInput(t), selection)
			assertOpaqueRPCFailure(t, err)
			assertFakeProcessGone(t, fake.pidPath)
		})
	}

	t.Run("honors deadline and reaps the process", func(t *testing.T) {
		fake := installFakePi(t, "hang")
		adapter := mustNewFakeAssessmentEvaluator(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := adapter.EvaluateAssessment(ctx, syntheticInitialAssessmentInput(t), selection)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("EvaluateAssessment() error = %v, want deadline exceeded", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("honors cancellation and reaps the process", func(t *testing.T) {
		fake := installFakePi(t, "hang")
		adapter := mustNewFakeAssessmentEvaluator(t)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := adapter.EvaluateAssessment(ctx, syntheticInitialAssessmentInput(t), selection)
			result <- err
		}()
		waitForFakePID(t, fake.pidPath)
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("EvaluateAssessment() error = %v, want context canceled", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("rejects invalid input before spawning", func(t *testing.T) {
		fake := installFakePi(t, "assessment_complete")
		input := syntheticInitialAssessmentInput(t)
		input.Answers[0].Text = ""
		_, err := mustNewFakeAssessmentEvaluator(t).EvaluateAssessment(context.Background(), input, selection)
		if ContractErrorCodeOf(err) != ContractErrorInvalidInput {
			t.Fatalf("EvaluateAssessment() error = %v, want invalid_input", err)
		}
		if _, err := os.Stat(fake.pidPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("assessment process started for invalid input: %v", err)
		}
	})
}

func mustNewFakeAssessmentEvaluator(t *testing.T) *PiRPCAssessmentEvaluator {
	t.Helper()
	adapter, err := NewPiRPCAssessmentEvaluator(context.Background(), prompts.EvaluatorAnswerAssessmentV1())
	if err != nil {
		t.Fatalf("NewPiRPCAssessmentEvaluator(): %v", err)
	}
	return adapter
}

func syntheticInitialAssessmentInput(t *testing.T) AssessmentInput {
	t.Helper()
	result := evidence.Result{
		RepositoryRoot: "/private/synthetic-repository",
		BaseRevision:   strings.Repeat("a", 40),
		HeadRevision:   evidence.WorkingTreeRevision,
		AppliedLimits: evidence.Limits{
			MaxFiles:        5,
			MaxDeclarations: 10,
			MaxExcerptBytes: 4096,
		},
		Files: []evidence.File{{
			Path:         "internal/synthetic.go",
			Status:       evidence.FileModified,
			ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
			Declarations: []evidence.Declaration{{
				Kind:         evidence.DeclarationFunction,
				Name:         "Synthetic",
				Identity:     "Synthetic",
				StartLine:    3,
				EndLine:      3,
				ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
				Excerpt:      "synthetic source",
			}},
		}},
	}
	bundle, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle(): %v", err)
	}
	input, err := NewInput(bundle)
	if err != nil {
		t.Fatalf("NewInput(): %v", err)
	}
	questions, err := ParseQuestionSet([]byte(syntheticQuestionSetJSON()), []string{"E001"})
	if err != nil {
		t.Fatalf("ParseQuestionSet(): %v", err)
	}
	assessmentInput, err := NewInitialAssessmentInput(input, questions, []AssessmentAnswer{
		{QuestionID: "Q1", Text: "first answer"},
		{QuestionID: "Q2", Text: "second answer"},
		{QuestionID: "Q3", Text: "third answer"},
	})
	if err != nil {
		t.Fatalf("NewInitialAssessmentInput(): %v", err)
	}
	return assessmentInput
}
