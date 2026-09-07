package evaluator

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/evidence"
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
	fake := installFakePi(t, "chinese_questions")
	questions, err := NewVersionedPiRPCEvaluator(
		context.Background(),
		prompts.EvaluatorQuestionGenerationV1(),
		prompts.EvaluatorQuestionGenerationV2SimplifiedChinese(),
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
	if request.SystemPrompt != prompts.EvaluatorQuestionGenerationV2SimplifiedChinese() {
		t.Fatal("v2 worker prompt does not equal released v2 prompt")
	}
}

func TestVersionedPiRPCEvaluatorRejectsEnglishOnlyV2Questions(t *testing.T) {
	fake := installFakePi(t, "english_questions")
	questions, err := NewVersionedPiRPCEvaluator(
		context.Background(),
		prompts.EvaluatorQuestionGenerationV1(),
		prompts.EvaluatorQuestionGenerationV2SimplifiedChinese(),
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

	_, err = questions.Evaluate(context.Background(), input, syntheticModelSelection())
	if ContractErrorCodeOf(err) != ContractErrorInvalidOutput {
		t.Fatalf("Evaluate(v2 English-only questions) error = %v, want invalid_output", err)
	}
	assertFakeProcessGone(t, fake.pidPath)
	assertOneFakeStart(t, fake.startsPath)
}

func TestVersionedPiRPCEvaluatorRequiresHanInEveryV2Question(t *testing.T) {
	for _, scenario := range []string{"q1_english", "q2_english", "q3_english"} {
		t.Run(scenario, func(t *testing.T) {
			fake := installFakePi(t, scenario)
			questions, err := NewVersionedPiRPCEvaluator(
				context.Background(),
				prompts.EvaluatorQuestionGenerationV1(),
				prompts.EvaluatorQuestionGenerationV2SimplifiedChinese(),
			)
			if err != nil {
				t.Fatalf("NewVersionedPiRPCEvaluator(): %v", err)
			}

			_, err = questions.Evaluate(context.Background(), syntheticRPCInputV2(), syntheticModelSelection())
			if ContractErrorCodeOf(err) != ContractErrorInvalidOutput {
				t.Fatalf("Evaluate(v2 %s) error = %v, want invalid_output", scenario, err)
			}
			assertFakeProcessGone(t, fake.pidPath)
			assertOneFakeStart(t, fake.startsPath)
		})
	}
}

func TestVersionedPiRPCAssessmentEvaluatorRejectsEnglishOnlyV2FollowUp(t *testing.T) {
	input := syntheticV2AssessmentInput(t)
	fake := installFakePi(t, "assessment_follow_up")
	assessments, err := NewVersionedPiRPCAssessmentEvaluator(
		context.Background(),
		prompts.EvaluatorAnswerAssessmentV1(),
		prompts.EvaluatorAnswerAssessmentV2SimplifiedChinese(),
	)
	if err != nil {
		t.Fatalf("NewVersionedPiRPCAssessmentEvaluator(): %v", err)
	}
	_, err = assessments.EvaluateAssessment(context.Background(), input, syntheticModelSelection())
	if ContractErrorCodeOf(err) != ContractErrorInvalidOutput {
		t.Fatalf("EvaluateAssessment(v2 English-only F1) error = %v, want invalid_output", err)
	}
	assertFakeProcessGone(t, fake.pidPath)
	assertOneFakeStart(t, fake.startsPath)
}

func TestVersionedPiRPCAssessmentEvaluatorAcceptsChineseV2FollowUp(t *testing.T) {
	input := syntheticV2AssessmentInput(t)
	fake := installFakePi(t, "assessment_chinese_follow_up")
	assessments, err := NewVersionedPiRPCAssessmentEvaluator(
		context.Background(),
		prompts.EvaluatorAnswerAssessmentV1(),
		prompts.EvaluatorAnswerAssessmentV2SimplifiedChinese(),
	)
	if err != nil {
		t.Fatalf("NewVersionedPiRPCAssessmentEvaluator(): %v", err)
	}

	turn, err := assessments.EvaluateAssessment(context.Background(), input, syntheticModelSelection())
	if err != nil || turn.Disposition != AssessmentDispositionFollowUp || turn.FollowUp == nil {
		t.Fatalf("EvaluateAssessment(v2 Chinese F1) = (%#v, %v), want follow_up", turn, err)
	}
	assertFakeProcessGone(t, fake.pidPath)
}

func TestVersionedPiRPCAssessmentEvaluatorDoesNotRequireChineseCompleteFeedback(t *testing.T) {
	input := syntheticV2AssessmentInput(t)
	fake := installFakePi(t, "assessment_complete")
	assessments, err := NewVersionedPiRPCAssessmentEvaluator(
		context.Background(),
		prompts.EvaluatorAnswerAssessmentV1(),
		prompts.EvaluatorAnswerAssessmentV2SimplifiedChinese(),
	)
	if err != nil {
		t.Fatalf("NewVersionedPiRPCAssessmentEvaluator(): %v", err)
	}

	turn, err := assessments.EvaluateAssessment(context.Background(), input, syntheticModelSelection())
	if err != nil || turn.Disposition != AssessmentDispositionComplete {
		t.Fatalf("EvaluateAssessment(v2 English feedback) = (%#v, %v), want complete", turn, err)
	}
	request := readFakeWorkerRequest(t, fake.requestsPath, 1)
	if request.SystemPrompt != prompts.EvaluatorAnswerAssessmentV2SimplifiedChinese() {
		t.Fatal("v2 worker prompt does not equal released Simplified-Chinese assessment prompt")
	}
	assertFakeProcessGone(t, fake.pidPath)
}

func syntheticRPCInputV2() Input {
	input := syntheticRPCInput()
	input.SchemaVersion = InputSchemaVersionV2
	input.EvidenceBundle.EvidenceCount = 2
	input.EvidenceBundle.GoContext = &EvidenceGoContext{
		Items: []EvidenceContextItem{{Reference: "C001", Content: "synthetic context"}},
	}
	return input
}

func syntheticV2AssessmentInput(t *testing.T) AssessmentInput {
	t.Helper()
	repository := t.TempDir()
	runGit := func(arguments ...string) string {
		t.Helper()
		command := exec.Command("git", arguments...)
		command.Dir = repository
		command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", arguments, err, output)
		}
		return string(output)
	}
	writeFile := func(path, content string) {
		t.Helper()
		fullPath := filepath.Join(repository, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGit("init", "-q")
	runGit("config", "user.email", "test@example.com")
	runGit("config", "user.name", "Test")
	writeFile("go.mod", "module example.com/pi-rpc-language\n\ngo 1.21\n")
	writeFile("changed.go", "package language\n\nfunc Value() int { return 1 }\n")
	runGit("add", ".")
	runGit("commit", "-qm", "base")
	base := strings.TrimSpace(runGit("rev-parse", "HEAD"))
	writeFile("changed.go", "package language\n\nfunc Value() int { return 2 }\n")
	result, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repository,
		Selection:  evidence.WorkingTree(base),
		Limits:     evidence.Limits{MaxFiles: 20, MaxDeclarations: 100, MaxExcerptBytes: 128 * 1024},
		Context:    evidence.ContextGo,
	})
	if err != nil {
		t.Fatalf("Preview(ContextGo): %v", err)
	}
	bundle, err := evidence.BuildBundleV2(result)
	if err != nil {
		t.Fatalf("BuildBundleV2(): %v", err)
	}
	evaluatorInput, err := NewInputV2(bundle)
	if err != nil {
		t.Fatalf("NewInputV2(): %v", err)
	}
	references, err := inputReferences(evaluatorInput)
	if err != nil {
		t.Fatalf("inputReferences(): %v", err)
	}
	questions, err := ParseQuestionSet([]byte(syntheticChineseQuestionSetJSON()), references)
	if err != nil {
		t.Fatalf("ParseQuestionSet(): %v", err)
	}
	assessmentInput, err := NewInitialAssessmentInput(evaluatorInput, questions, []AssessmentAnswer{
		{QuestionID: "Q1", Text: "first answer"},
		{QuestionID: "Q2", Text: "second answer"},
		{QuestionID: "Q3", Text: "third answer"},
	})
	if err != nil {
		t.Fatalf("NewInitialAssessmentInput(): %v", err)
	}
	return assessmentInput
}

func assertOneFakeStart(t *testing.T, path string) {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(starts): %v", err)
	}
	if got := len(strings.Fields(string(content))); got != 1 {
		t.Fatalf("worker starts = %d, want exactly one", got)
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
