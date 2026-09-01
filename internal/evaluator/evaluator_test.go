package evaluator

import (
	"context"
	"errors"
	"testing"
)

func TestDeterministicEvaluatorProducesValidatedQuestionShape(t *testing.T) {
	input := Input{
		SchemaVersion: InputSchemaVersion,
		EvidenceBundle: EvidenceBundle{
			Items: []EvidenceItem{{Reference: "E001"}, {Reference: "E002"}},
		},
	}
	selection := ModelSelection{
		PiVersion:     SupportedPiVersion,
		Provider:      "anthropic",
		ModelID:       "claude-test",
		ThinkingLevel: "off",
	}

	result, err := (DeterministicEvaluator{}).Evaluate(context.Background(), input, selection)
	if err != nil {
		t.Fatalf("Evaluate(): %v", err)
	}
	if result.Disposition != DispositionQuestions || len(result.Questions) != 3 {
		t.Fatalf("result = %#v, want exactly three questions", result)
	}
	if got := result.Questions[0].EvidenceReferences; len(got) != 1 || got[0] != "E001" {
		t.Fatalf("Q1 references = %#v, want E001", got)
	}
	if got := result.Questions[1].EvidenceReferences; len(got) != 1 || got[0] != "E002" {
		t.Fatalf("Q2 references = %#v, want E002", got)
	}
}

func TestDeterministicEvaluatorRejectsInvalidSelectionAndCancellation(t *testing.T) {
	input := Input{SchemaVersion: InputSchemaVersion, EvidenceBundle: EvidenceBundle{Items: []EvidenceItem{{Reference: "E001"}}}}
	selection := ModelSelection{PiVersion: SupportedPiVersion, Provider: "-unsafe", ModelID: "model", ThinkingLevel: "off"}
	if _, err := (DeterministicEvaluator{}).Evaluate(context.Background(), input, selection); ContractErrorCodeOf(err) != ContractErrorInvalidInput {
		t.Fatalf("invalid selection error = %v, want invalid_input", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	selection.Provider = "provider"
	if _, err := (DeterministicEvaluator{}).Evaluate(ctx, input, selection); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled Evaluate() error = %v, want context.Canceled", err)
	}
}
