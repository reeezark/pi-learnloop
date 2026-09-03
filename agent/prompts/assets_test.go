package prompts

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestEnrichedPromptAssetsAreVersionedAndHashed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		metadata Metadata
		schema   string
	}{
		{
			name:     "question",
			body:     EvaluatorQuestionGenerationV2(),
			metadata: EvaluatorQuestionGenerationV2Metadata(),
			schema:   "input_schema: evaluator-input@2",
		},
		{
			name:     "assessment",
			body:     EvaluatorAnswerAssessmentV2(),
			metadata: EvaluatorAnswerAssessmentV2Metadata(),
			schema:   "input_schema: evaluator-assessment-input@2",
		},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if test.body == "" || !strings.Contains(test.body, "status: released") || !strings.Contains(test.body, test.schema) {
				t.Fatalf("embedded prompt is empty or missing released v2 metadata")
			}
			if test.metadata.Version != "2.0.0" {
				t.Fatalf("metadata version = %q, want 2.0.0", test.metadata.Version)
			}
			digest := sha256.Sum256([]byte(test.body))
			if want := hex.EncodeToString(digest[:]); test.metadata.SHA256 != want {
				t.Fatalf("metadata hash = %q, want %q", test.metadata.SHA256, want)
			}
		})
	}
}
