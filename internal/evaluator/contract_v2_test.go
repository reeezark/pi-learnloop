package evaluator_test

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestNewInputV2OwnsEnrichedEvidenceAndKeepsV1JSONExact(t *testing.T) {
	bundle := validV2Bundle(t)
	input, err := evaluator.NewInputV2(bundle)
	if err != nil {
		t.Fatalf("NewInputV2(): %v", err)
	}
	if input.SchemaVersion != evaluator.InputSchemaVersionV2 || input.EvidenceBundle.FormatVersion != evidence.BundleFormatVersionV2 || input.EvidenceBundle.GoContext == nil {
		t.Fatalf("NewInputV2() = %#v, want evaluator-input@2 with evidence-bundle@2", input)
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("json.Marshal(v2 input): %v", err)
	}
	if len(encoded) > input.EvidenceBundle.GoContext.AppliedLimits.MaxEvaluatorInputBytes {
		t.Fatalf("serialized evaluator-input@2 = %d bytes, exceeds %d", len(encoded), input.EvidenceBundle.GoContext.AppliedLimits.MaxEvaluatorInputBytes)
	}
	if strings.Contains(string(encoded), v2RepositoryMarker) || strings.Contains(string(encoded), "pi-session") {
		t.Fatalf("v2 evaluator input leaks prohibited provenance: %s", encoded)
	}
	if len(input.EvidenceBundle.Items) != 0 || strings.Contains(string(encoded), `"evidence_references":null`) {
		t.Fatalf("import-only evaluator input lost its explicit empty changed-evidence arrays: %s", encoded)
	}
	wantContextContent := input.EvidenceBundle.GoContext.Items[0].Content
	bundle.GoContext.Items[0].Content = "mutated context"
	bundle.GoContext.Relations = append(bundle.GoContext.Relations, evidence.ContextRelation{From: "mutated", To: "mutated"})
	bundle.GoContext.Build.Modules[0].Path = "mutated.example/module"
	if input.EvidenceBundle.GoContext.Items[0].Content != wantContextContent ||
		input.EvidenceBundle.GoContext.Build.Modules[0].Path == "mutated.example/module" ||
		len(input.EvidenceBundle.GoContext.Relations) != bundle.GoContext.RelationCount {
		t.Fatalf("NewInputV2 retained aliases to the domain bundle: %#v", input.EvidenceBundle.GoContext)
	}

	v1, err := evaluator.NewInput(validBundle(t))
	if err != nil {
		t.Fatalf("NewInput(v1): %v", err)
	}
	v1JSON, err := json.Marshal(v1)
	if err != nil {
		t.Fatalf("json.Marshal(v1 input): %v", err)
	}
	if strings.Contains(string(v1JSON), "go_context") || v1.SchemaVersion != evaluator.InputSchemaVersion {
		t.Fatalf("v1 evaluator JSON changed: %s", v1JSON)
	}
}

func TestV2QuestionAndAssessmentContractsAcceptContextReferences(t *testing.T) {
	input := validV2Input(t)
	references := v2References(input)
	contextReference := ""
	for _, reference := range references {
		if strings.HasPrefix(reference, "C") {
			contextReference = reference
			break
		}
	}
	if contextReference == "" {
		t.Fatalf("references = %v, want a context reference", references)
	}
	questionJSON := `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"What does the selected context prove?","evidence_references":["` + contextReference + `"]},{"id":"Q2","kind":"code_specific","text":"How does the changed code use that context?","evidence_references":["` + contextReference + `"]},{"id":"Q3","kind":"go_backend","text":"Why should incomplete type evidence be reported explicitly?","evidence_references":[]}]}`
	questions, err := evaluator.ParseQuestionSet([]byte(questionJSON), references)
	if err != nil {
		t.Fatalf("ParseQuestionSet(v2 references): %v", err)
	}
	assessmentInput, err := evaluator.NewInitialAssessmentInput(input, questions, []evaluator.AssessmentAnswer{
		{QuestionID: "Q1", Text: "The context item is a selected-snapshot declaration."},
		{QuestionID: "Q2", Text: "The recorded relation connects the changed code to it."},
		{QuestionID: "Q3", Text: "Explicit omissions prevent unsupported conclusions."},
	})
	if err != nil {
		t.Fatalf("NewInitialAssessmentInput(v2): %v", err)
	}
	if assessmentInput.SchemaVersion != evaluator.AssessmentInputSchemaVersionV2 {
		t.Fatalf("assessment schema = %d, want %d", assessmentInput.SchemaVersion, evaluator.AssessmentInputSchemaVersionV2)
	}
	turnJSON := `{"schema_version":1,"disposition":"complete","follow_up":null,"evaluations":[{"question_id":"Q1","verdict":"demonstrated","feedback":"The answer identifies the selected context fact.","evidence_references":["` + contextReference + `"]},{"question_id":"Q2","verdict":"partial","feedback":"The answer does not identify the exact relationship strength.","evidence_references":["` + contextReference + `"]},{"question_id":"Q3","verdict":"demonstrated","feedback":"The answer explains why omissions constrain conclusions.","evidence_references":[]}]}`
	if _, err := evaluator.ParseAssessmentTurn([]byte(turnJSON), assessmentInput); err != nil {
		t.Fatalf("ParseAssessmentTurn(v2 references): %v", err)
	}
}

func TestNewInputV2RejectsTamperedContext(t *testing.T) {
	bundle := validV2Bundle(t)
	bundle.GoContext.Items[0].Content += "tampered"
	if _, err := evaluator.NewInputV2(bundle); evaluator.ContractErrorCodeOf(err) != evaluator.ContractErrorInvalidInput {
		t.Fatalf("NewInputV2(tampered) error = %v, want invalid_input", err)
	}
}

const v2RepositoryMarker = "pi-learnloop-v2-evaluator-repository"

func validV2Input(t *testing.T) evaluator.Input {
	t.Helper()
	bundle := validV2Bundle(t)
	input, err := evaluator.NewInputV2(bundle)
	if err != nil {
		t.Fatalf("NewInputV2(): %v", err)
	}
	return input
}

func validV2Bundle(t *testing.T) evidence.BundleV2 {
	t.Helper()
	repo := filepath.Join(t.TempDir(), v2RepositoryMarker)
	if err := os.MkdirAll(filepath.Join(repo, "old"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(repo, "new"), 0o755); err != nil {
		t.Fatal(err)
	}
	runGitV2(t, repo, "init", "-q")
	runGitV2(t, repo, "config", "user.email", "test@example.com")
	runGitV2(t, repo, "config", "user.name", "Test")
	writeV2File(t, repo, "go.mod", "module example.com/evaluator-v2\n\ngo 1.21\n")
	writeV2File(t, repo, "old/old.go", "package token\n\ntype Token string\n")
	writeV2File(t, repo, "new/new.go", "package token\n\ntype Token string\n")
	writeV2File(t, repo, "changed.go", "package evaluatorv2\n\nimport \"example.com/evaluator-v2/old\"\n\nfunc Build() token.Token { return \"ok\" }\n")
	runGitV2(t, repo, "add", ".")
	runGitV2(t, repo, "commit", "-qm", "base")
	base := strings.TrimSpace(runGitV2(t, repo, "rev-parse", "HEAD"))
	writeV2File(t, repo, "changed.go", "package evaluatorv2\n\nimport \"example.com/evaluator-v2/new\"\n\nfunc Build() token.Token { return \"ok\" }\n")
	result, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo, Selection: evidence.WorkingTree(base),
		Limits:  evidence.Limits{MaxFiles: 20, MaxDeclarations: 100, MaxExcerptBytes: 128 * 1024},
		Context: evidence.ContextGo,
	})
	if err != nil {
		t.Fatalf("Preview(ContextGo): %v", err)
	}
	bundle, err := evidence.BuildBundleV2(result)
	if err != nil {
		t.Fatalf("BuildBundleV2(): %v", err)
	}
	return bundle
}

func v2References(input evaluator.Input) []string {
	references := make([]string, 0, input.EvidenceBundle.EvidenceCount)
	for _, item := range input.EvidenceBundle.Items {
		references = append(references, item.Reference)
	}
	for _, item := range input.EvidenceBundle.GoContext.Items {
		references = append(references, item.Reference)
	}
	return references
}

func writeV2File(t *testing.T, root, path, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func runGitV2(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = directory
	command.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_TERMINAL_PROMPT=0")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}
