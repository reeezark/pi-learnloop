package evaluator

import (
	"context"
	"testing"

	"github.com/reeezark/pi-learnloop/agent/prompts"
)

func TestVersionedPiRPCAdaptersSelectPromptsFromValidatedInputVersion(t *testing.T) {
	questions := &PiRPCEvaluator{systemPrompt: "question-v1", systemPromptV2: "question-v2"}
	if got, err := questions.questionPrompt(InputSchemaVersion); err != nil || got != "question-v1" {
		t.Fatalf("question v1 prompt = (%q, %v)", got, err)
	}
	if got, err := questions.questionPrompt(InputSchemaVersionV2); err != nil || got != "question-v2" {
		t.Fatalf("question v2 prompt = (%q, %v)", got, err)
	}

	assessments := &PiRPCAssessmentEvaluator{systemPrompt: "assessment-v1", systemPromptV2: "assessment-v2"}
	if got, err := assessments.assessmentPrompt(AssessmentInputSchemaVersion); err != nil || got != "assessment-v1" {
		t.Fatalf("assessment v1 prompt = (%q, %v)", got, err)
	}
	if got, err := assessments.assessmentPrompt(AssessmentInputSchemaVersionV2); err != nil || got != "assessment-v2" {
		t.Fatalf("assessment v2 prompt = (%q, %v)", got, err)
	}

	legacyQuestions := &PiRPCEvaluator{systemPrompt: "question-v1"}
	if _, err := legacyQuestions.questionPrompt(InputSchemaVersionV2); err == nil {
		t.Fatal("legacy question adapter accepted evaluator-input@2")
	}
	legacyAssessments := &PiRPCAssessmentEvaluator{systemPrompt: "assessment-v1"}
	if _, err := legacyAssessments.assessmentPrompt(AssessmentInputSchemaVersionV2); err == nil {
		t.Fatal("legacy assessment adapter accepted evaluator-assessment-input@2")
	}
}

func TestVersionedPiRPCEvaluatorUsesV2PromptForV2Input(t *testing.T) {
	fake := installFakePi(t, "success")
	questions, err := NewVersionedPiRPCEvaluator(
		context.Background(),
		prompts.EvaluatorQuestionGenerationV1(),
		prompts.EvaluatorQuestionGenerationV2(),
	)
	if err != nil {
		t.Fatalf("NewVersionedPiRPCEvaluator(): %v", err)
	}
	input := syntheticRPCInput()
	input.SchemaVersion = InputSchemaVersionV2
	input.EvidenceBundle.EvidenceCount = 2
	input.EvidenceBundle.GoContext = &EvidenceGoContext{
		Items: []EvidenceContextItem{{Reference: "C001", Content: "synthetic context"}},
	}
	if _, err := questions.Evaluate(context.Background(), input, syntheticModelSelection()); err != nil {
		t.Fatalf("Evaluate(v2): %v", err)
	}
	request := readFakeWorkerRequest(t, fake.requestsPath, 1)
	if request.SystemPrompt != prompts.EvaluatorQuestionGenerationV2() {
		t.Fatal("v2 worker prompt does not equal released v2 prompt")
	}
}

func TestVersionedPiRPCConstructorsRejectMissingV2Prompt(t *testing.T) {
	installFakePi(t, "success")
	if _, err := NewVersionedPiRPCEvaluator(context.Background(), "question-v1", ""); err == nil {
		t.Fatal("question constructor accepted an empty v2 prompt")
	}
	if _, err := NewVersionedPiRPCAssessmentEvaluator(context.Background(), "assessment-v1", ""); err == nil {
		t.Fatal("assessment constructor accepted an empty v2 prompt")
	}
}
