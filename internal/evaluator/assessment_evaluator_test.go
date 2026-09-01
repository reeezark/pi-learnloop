package evaluator_test

import (
	"context"
	"errors"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

func TestDeterministicAssessmentEvaluatorEvaluateAssessment(t *testing.T) {
	selection := evaluator.ModelSelection{
		PiVersion:     evaluator.SupportedPiVersion,
		Provider:      "anthropic",
		ModelID:       "claude-test",
		ThinkingLevel: "off",
	}
	initial := validInitialAssessmentInput(t)

	t.Run("returns a validated complete result", func(t *testing.T) {
		result, err := (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(context.Background(), initial, selection)
		if err != nil {
			t.Fatalf("EvaluateAssessment() error = %v", err)
		}
		if result.Disposition != evaluator.AssessmentDispositionComplete || len(result.Evaluations) != 3 {
			t.Fatalf("result = %#v, want complete three-question evaluation", result)
		}
		label, err := evaluator.DeriveAssessmentLabel(result)
		if err != nil || label != evaluator.AssessmentLabelPartial {
			t.Fatalf("DeriveAssessmentLabel() = (%q, %v), want partial", label, err)
		}
	})

	t.Run("returns one follow-up only for the initial stage", func(t *testing.T) {
		adapter := evaluator.DeterministicAssessmentEvaluator{RequestFollowUp: true}
		result, err := adapter.EvaluateAssessment(context.Background(), initial, selection)
		if err != nil {
			t.Fatalf("EvaluateAssessment(initial) error = %v", err)
		}
		if result.Disposition != evaluator.AssessmentDispositionFollowUp || result.FollowUp == nil || result.FollowUp.ID != "F1" {
			t.Fatalf("initial result = %#v, want F1", result)
		}

		followUp, err := evaluator.NewFollowUpAssessmentInput(initial, *result.FollowUp, "The empty-name branch returns ErrEmpty.")
		if err != nil {
			t.Fatalf("NewFollowUpAssessmentInput(): %v", err)
		}
		result, err = adapter.EvaluateAssessment(context.Background(), followUp, selection)
		if err != nil {
			t.Fatalf("EvaluateAssessment(follow-up) error = %v", err)
		}
		if result.Disposition != evaluator.AssessmentDispositionComplete {
			t.Fatalf("follow-up result = %#v, want complete result", result)
		}
	})

	t.Run("rejects invalid selection input and context", func(t *testing.T) {
		invalidSelection := selection
		invalidSelection.Provider = "-unsafe"
		if _, err := (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(context.Background(), initial, invalidSelection); evaluator.ContractErrorCodeOf(err) != evaluator.ContractErrorInvalidInput {
			t.Fatalf("invalid selection error = %v, want invalid_input", err)
		}

		invalidInput := initial
		invalidInput.Answers[0].Text = ""
		if _, err := (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(context.Background(), invalidInput, selection); evaluator.ContractErrorCodeOf(err) != evaluator.ContractErrorInvalidInput {
			t.Fatalf("invalid input error = %v, want invalid_input", err)
		}

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(ctx, initial, selection); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancelled evaluation error = %v, want context.Canceled", err)
		}

		if _, err := (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(nil, initial, selection); evaluator.ContractErrorCodeOf(err) != evaluator.ContractErrorInvalidInput {
			t.Fatalf("nil context error = %v, want invalid_input", err)
		}
	})
}
