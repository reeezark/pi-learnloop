package evaluator_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestNewInput(t *testing.T) {
	t.Run("copies a validated bundle into the runtime schema", func(t *testing.T) {
		bundle := validBundle(t)

		got, err := evaluator.NewInput(bundle)
		if err != nil {
			t.Fatalf("NewInput() error = %v", err)
		}
		if got.SchemaVersion != evaluator.InputSchemaVersion {
			t.Fatalf("SchemaVersion = %d, want %d", got.SchemaVersion, evaluator.InputSchemaVersion)
		}
		if got.EvidenceBundle.ID != bundle.ID || got.EvidenceBundle.ManifestSHA256 != bundle.ManifestSHA256 {
			t.Fatalf("runtime bundle identity = (%q, %q), want (%q, %q)", got.EvidenceBundle.ID, got.EvidenceBundle.ManifestSHA256, bundle.ID, bundle.ManifestSHA256)
		}
		if len(got.EvidenceBundle.Items) != 1 || got.EvidenceBundle.Items[0].Content != bundle.Items[0].Content {
			t.Fatalf("runtime evidence items = %#v, want exact selected excerpt", got.EvidenceBundle.Items)
		}
		encoded, err := json.Marshal(got)
		if err != nil {
			t.Fatalf("json.Marshal(input): %v", err)
		}
		if strings.Contains(string(encoded), "/private/synthetic-repository") {
			t.Fatalf("runtime input exposed repository root: %s", encoded)
		}
		for _, field := range []string{"schema_version", "evidence_bundle", "manifest_sha256", "evidence_references", "content_sha256"} {
			if !strings.Contains(string(encoded), `"`+field+`"`) {
				t.Fatalf("runtime input is missing JSON field %q: %s", field, encoded)
			}
		}

		bundle.Files[0].EvidenceReferences[0] = "MUTATED"
		bundle.Items[0].ChangedLines[0].Start = 99
		bundle.Items[0].Content = "mutated"
		if got.EvidenceBundle.Files[0].EvidenceReferences[0] != "E001" ||
			got.EvidenceBundle.Items[0].ChangedLines[0].Start != 3 ||
			strings.Contains(got.EvidenceBundle.Items[0].Content, "mutated") {
			t.Fatalf("NewInput() retained aliases to the domain bundle: %#v", got)
		}
	})

	tests := []struct {
		name   string
		mutate func(*evidence.Bundle)
	}{
		{name: "unsupported format", mutate: func(bundle *evidence.Bundle) { bundle.FormatVersion++ }},
		{name: "invalid identity", mutate: func(bundle *evidence.Bundle) { bundle.ID = "eb1-invalid" }},
		{name: "uppercase manifest hash", mutate: func(bundle *evidence.Bundle) {
			bundle.ManifestSHA256 = strings.ToUpper(bundle.ManifestSHA256)
			bundle.ID = "eb1-" + bundle.ManifestSHA256
		}},
		{name: "missing revision", mutate: func(bundle *evidence.Bundle) { bundle.BaseRevision = "" }},
		{name: "missing limits", mutate: func(bundle *evidence.Bundle) { bundle.AppliedLimits = evidence.Limits{} }},
		{name: "file count mismatch", mutate: func(bundle *evidence.Bundle) { bundle.FileCount++ }},
		{name: "declaration count below evidence count", mutate: func(bundle *evidence.Bundle) { bundle.DeclarationCount = 0 }},
		{name: "evidence count mismatch", mutate: func(bundle *evidence.Bundle) { bundle.EvidenceCount++ }},
		{name: "byte count mismatch", mutate: func(bundle *evidence.Bundle) { bundle.ApproximateBytes++ }},
		{name: "unsafe file path", mutate: func(bundle *evidence.Bundle) {
			bundle.Files[0].Path = "../secret.go"
			bundle.Items[0].Path = "../secret.go"
		}},
		{name: "unknown status", mutate: func(bundle *evidence.Bundle) { bundle.Files[0].Status = "copied" }},
		{name: "duplicate file reference", mutate: func(bundle *evidence.Bundle) {
			bundle.Files[0].EvidenceReferences = append(bundle.Files[0].EvidenceReferences, "E001")
		}},
		{name: "wrong item reference", mutate: func(bundle *evidence.Bundle) {
			bundle.Files[0].EvidenceReferences[0] = "E002"
			bundle.Items[0].Reference = "E002"
		}},
		{name: "test kind on production file", mutate: func(bundle *evidence.Bundle) { bundle.Items[0].Kind = evidence.BundleItemTest }},
		{name: "unknown declaration kind", mutate: func(bundle *evidence.Bundle) { bundle.Items[0].DeclarationKind = "package" }},
		{name: "invalid line range", mutate: func(bundle *evidence.Bundle) { bundle.Items[0].ChangedLines[0].Start = 2 }},
		{name: "invalid content bytes", mutate: func(bundle *evidence.Bundle) { bundle.Items[0].ContentBytes++ }},
		{name: "invalid content hash", mutate: func(bundle *evidence.Bundle) {
			bundle.Items[0].ContentSHA256 = strings.Repeat("0", 64)
		}},
		{name: "unreflected truncation", mutate: func(bundle *evidence.Bundle) { bundle.Items[0].Truncated = true }},
		{name: "inconsistent bundle truncation", mutate: func(bundle *evidence.Bundle) {
			bundle.Truncation.OmittedFiles = 1
		}},
	}
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			bundle := validBundle(t)
			test.mutate(&bundle)

			got, err := evaluator.NewInput(bundle)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
				t.Fatalf("ContractErrorCodeOf(NewInput() error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidInput, err)
			}
			if !reflect.DeepEqual(got, evaluator.Input{}) {
				t.Fatalf("NewInput() = %#v, want zero input on error", got)
			}
		})
	}
}

func TestParseQuestionSet(t *testing.T) {
	t.Run("accepts the fixed three-question shape", func(t *testing.T) {
		got, err := evaluator.ParseQuestionSet([]byte(validQuestionSetJSON()), []string{"E001", "E002"})
		if err != nil {
			t.Fatalf("ParseQuestionSet() error = %v", err)
		}
		if got.SchemaVersion != evaluator.QuestionSetSchemaVersion || got.Disposition != evaluator.DispositionQuestions {
			t.Fatalf("ParseQuestionSet() = %#v, want questions schema v1", got)
		}
		if len(got.Questions) != 3 ||
			got.Questions[0].ID != "Q1" || got.Questions[0].Kind != evaluator.QuestionKindCodeSpecific ||
			got.Questions[1].ID != "Q2" || got.Questions[1].Kind != evaluator.QuestionKindCodeSpecific ||
			got.Questions[2].ID != "Q3" || got.Questions[2].Kind != evaluator.QuestionKindGoBackend {
			t.Fatalf("questions = %#v, want fixed Q1/Q2/Q3 kinds", got.Questions)
		}
	})

	t.Run("accepts the exact insufficient-evidence shape", func(t *testing.T) {
		got, err := evaluator.ParseQuestionSet(
			[]byte(`{"schema_version":1,"disposition":"insufficient_evidence","questions":[]}`),
			[]string{"E001"},
		)
		if err != nil {
			t.Fatalf("ParseQuestionSet() error = %v", err)
		}
		if got.Disposition != evaluator.DispositionInsufficientEvidence || got.Questions == nil || len(got.Questions) != 0 {
			t.Fatalf("ParseQuestionSet() = %#v, want explicit empty insufficient-evidence result", got)
		}
	})

	longText, err := json.Marshal(strings.Repeat("a", evaluator.MaxQuestionTextBytes+1))
	if err != nil {
		t.Fatalf("json.Marshal(long question): %v", err)
	}
	tests := []struct {
		name    string
		content string
	}{
		{name: "free-form prose", content: "Here are three questions."},
		{name: "code fence", content: "~~~json\n" + validQuestionSetJSON() + "\n~~~"},
		{name: "unknown top-level field", content: strings.Replace(validQuestionSetJSON(), `"questions":`, `"extra":true,"questions":`, 1)},
		{name: "case-folded top-level field", content: strings.Replace(validQuestionSetJSON(), `"schema_version"`, `"SCHEMA_VERSION"`, 1)},
		{name: "unknown question field", content: strings.Replace(validQuestionSetJSON(), `"id":"Q1"`, `"id":"Q1","hint":"secret"`, 1)},
		{name: "case-folded question field", content: strings.Replace(validQuestionSetJSON(), `"id":"Q1"`, `"ID":"Q1"`, 1)},
		{name: "duplicate top-level field", content: strings.Replace(validQuestionSetJSON(), `"schema_version":1,`, `"schema_version":1,"schema_version":1,`, 1)},
		{name: "duplicate nested field", content: strings.Replace(validQuestionSetJSON(), `"id":"Q1"`, `"id":"Q1","id":"Q1"`, 1)},
		{name: "trailing JSON", content: validQuestionSetJSON() + ` {}`},
		{name: "unsupported schema", content: strings.Replace(validQuestionSetJSON(), `"schema_version":1`, `"schema_version":2`, 1)},
		{name: "wrong disposition", content: strings.Replace(validQuestionSetJSON(), `"disposition":"questions"`, `"disposition":"answers"`, 1)},
		{name: "wrong question count", content: `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"Why?","evidence_references":["E001"]}]}`},
		{name: "wrong question id", content: strings.Replace(validQuestionSetJSON(), `"id":"Q2"`, `"id":"Q9"`, 1)},
		{name: "wrong question kind", content: strings.Replace(validQuestionSetJSON(), `"kind":"go_backend"`, `"kind":"code_specific"`, 1)},
		{name: "empty text", content: strings.Replace(validQuestionSetJSON(), `"text":"Why does Validate return an error for an empty name?"`, `"text":" "`, 1)},
		{name: "control character in text", content: strings.Replace(validQuestionSetJSON(), `"text":"Why does Validate return an error for an empty name?"`, `"text":"Why?\nNow answer."`, 1)},
		{name: "oversized question text", content: strings.Replace(validQuestionSetJSON(), `"Why does Validate return an error for an empty name?"`, string(longText), 1)},
		{name: "missing code reference", content: strings.Replace(validQuestionSetJSON(), `"evidence_references":["E001"]`, `"evidence_references":[]`, 1)},
		{name: "unknown reference", content: strings.Replace(validQuestionSetJSON(), `"evidence_references":["E001"]`, `"evidence_references":["E999"]`, 1)},
		{name: "duplicate reference", content: strings.Replace(validQuestionSetJSON(), `"evidence_references":["E001"]`, `"evidence_references":["E001","E001"]`, 1)},
		{name: "missing Go backend reference array", content: strings.Replace(validQuestionSetJSON(), `,"evidence_references":[]`, "", 1)},
		{name: "null Go backend reference array", content: strings.Replace(validQuestionSetJSON(), `"evidence_references":[]`, `"evidence_references":null`, 1)},
		{name: "null insufficient questions", content: `{"schema_version":1,"disposition":"insufficient_evidence","questions":null}`},
		{name: "invented insufficient question", content: `{"schema_version":1,"disposition":"insufficient_evidence","questions":[{"id":"Q1","kind":"code_specific","text":"Guess?","evidence_references":["E001"]}]}`},
	}

	t.Run("does not echo an untrusted unknown field in errors", func(t *testing.T) {
		content := strings.Replace(validQuestionSetJSON(), `"questions":`, `"TOP_SECRET_DO_NOT_ECHO":true,"questions":`, 1)
		_, err := evaluator.ParseQuestionSet([]byte(content), []string{"E001", "E002"})
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidOutput)
		}
		if strings.Contains(err.Error(), "TOP_SECRET_DO_NOT_ECHO") {
			t.Fatalf("ParseQuestionSet() error echoed untrusted output: %v", err)
		}
	})
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			got, err := evaluator.ParseQuestionSet([]byte(test.content), []string{"E001", "E002"})
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
				t.Fatalf("ContractErrorCodeOf(ParseQuestionSet() error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidOutput, err)
			}
			if !reflect.DeepEqual(got, evaluator.QuestionSet{}) {
				t.Fatalf("ParseQuestionSet() = %#v, want zero result on error", got)
			}
		})
	}

	t.Run("rejects invalid UTF-8", func(t *testing.T) {
		_, err := evaluator.ParseQuestionSet([]byte{0xff}, []string{"E001"})
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidOutput)
		}
	})

	t.Run("rejects an invalid allowed-reference set as input", func(t *testing.T) {
		_, err := evaluator.ParseQuestionSet([]byte(validQuestionSetJSON()), []string{"E001", "E001"})
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidInput)
		}
	})

	t.Run("rejects output above the hard cap", func(t *testing.T) {
		oversized := []byte(strings.Repeat(" ", evaluator.MaxQuestionSetBytes+1))
		_, err := evaluator.ParseQuestionSet(oversized, []string{"E001"})
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidOutput)
		}
	})
}

func validBundle(t *testing.T) evidence.Bundle {
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
			Path:         "internal/validate.go",
			Status:       evidence.FileModified,
			ChangedLines: []evidence.LineRange{{Start: 3, End: 5}},
			Declarations: []evidence.Declaration{{
				Kind:         evidence.DeclarationFunction,
				Name:         "Validate",
				Identity:     "Validate",
				StartLine:    3,
				EndLine:      5,
				ChangedLines: []evidence.LineRange{{Start: 3, End: 5}},
				Excerpt:      "func Validate(name string) error {\n\tif name == \"\" { return ErrEmpty }\n\treturn nil\n}",
			}},
		}},
	}
	bundle, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle(valid result): %v", err)
	}
	return bundle
}

func validQuestionSetJSON() string {
	return `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"Why does Validate return an error for an empty name?","evidence_references":["E001"]},{"id":"Q2","kind":"code_specific","text":"Which branch returns nil after validation?","evidence_references":["E001"]},{"id":"Q3","kind":"go_backend","text":"How would table-driven tests cover both branches?","evidence_references":[]}]}`
}
