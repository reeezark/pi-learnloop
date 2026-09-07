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

func TestSimplifiedChinesePromptAssetsAreVersionedAndHashed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		body     string
		metadata Metadata
		schema   string
	}{
		{
			name:     "question",
			body:     EvaluatorQuestionGenerationV2SimplifiedChinese(),
			metadata: EvaluatorQuestionGenerationV2SimplifiedChineseMetadata(),
			schema:   "input_schema: evaluator-input@2",
		},
		{
			name:     "assessment",
			body:     EvaluatorAnswerAssessmentV2SimplifiedChinese(),
			metadata: EvaluatorAnswerAssessmentV2SimplifiedChineseMetadata(),
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
			if !strings.Contains(test.body, "Simplified Chinese") {
				t.Fatal("embedded prompt does not require Simplified Chinese question prose")
			}
			if test.metadata.Version != "2.1.0" {
				t.Fatalf("metadata version = %q, want 2.1.0", test.metadata.Version)
			}
			digest := sha256.Sum256([]byte(test.body))
			if want := hex.EncodeToString(digest[:]); test.metadata.SHA256 != want {
				t.Fatalf("metadata hash = %q, want %q", test.metadata.SHA256, want)
			}
		})
	}
}

func TestHistoricalPromptAssetHashesRemainImmutable(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want string
	}{
		{name: "question v1.0.0", body: EvaluatorQuestionGenerationV1(), want: "00848334e9f64d2b7c1fdcc20a0854bceb230c6ba1d89c66f83057c44e79acc7"},
		{name: "question v2.0.0", body: EvaluatorQuestionGenerationV2(), want: "bc5935e26704e8a71ea15c0d6abe95f8cfa1b6ddf12d78d81523cd698c0417d8"},
		{name: "assessment v1.0.0", body: EvaluatorAnswerAssessmentV1(), want: "d057faac9a69ccf8c6a58af58cf4b13d8c9bfcd0e7ef0cf26c3348e013fefe80"},
		{name: "assessment v2.0.0", body: EvaluatorAnswerAssessmentV2(), want: "28864f16196514e72a530be18942b9ca015760e61cc435a123bd9c88c6247cff"},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			digest := sha256.Sum256([]byte(test.body))
			if got := hex.EncodeToString(digest[:]); got != test.want {
				t.Fatalf("asset SHA-256 = %q, want immutable %q", got, test.want)
			}
		})
	}
}
