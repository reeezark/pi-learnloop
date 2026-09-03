// Package prompts embeds immutable released evaluator prompts for production
// use without relying on a repository-relative runtime path.
package prompts

import (
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
)

// Metadata identifies one immutable released prompt without exposing its body.
type Metadata struct {
	ID      string
	Version string
	SHA256  string
}

//go:embed evaluator-question-generation/v1.0.0.md
var evaluatorQuestionGenerationV1 string

//go:embed evaluator-answer-assessment/v1.0.0.md
var evaluatorAnswerAssessmentV1 string

//go:embed evaluator-question-generation/v2.0.0.md
var evaluatorQuestionGenerationV2 string

//go:embed evaluator-answer-assessment/v2.0.0.md
var evaluatorAnswerAssessmentV2 string

// EvaluatorQuestionGenerationV1 returns the exact released prompt asset.
func EvaluatorQuestionGenerationV1() string {
	return evaluatorQuestionGenerationV1
}

// EvaluatorQuestionGenerationV1Metadata returns the immutable prompt identity
// and the SHA-256 of the exact embedded bytes used in production.
func EvaluatorQuestionGenerationV1Metadata() Metadata {
	return metadata("evaluator-question-generation", "1.0.0", evaluatorQuestionGenerationV1)
}

// EvaluatorAnswerAssessmentV1 returns the exact released prompt asset.
func EvaluatorAnswerAssessmentV1() string {
	return evaluatorAnswerAssessmentV1
}

// EvaluatorAnswerAssessmentV1Metadata returns the immutable prompt identity
// and the SHA-256 of the exact embedded bytes used in production.
func EvaluatorAnswerAssessmentV1Metadata() Metadata {
	return metadata("evaluator-answer-assessment", "1.0.0", evaluatorAnswerAssessmentV1)
}

// EvaluatorQuestionGenerationV2 returns the exact released enriched-evidence prompt asset.
func EvaluatorQuestionGenerationV2() string {
	return evaluatorQuestionGenerationV2
}

// EvaluatorQuestionGenerationV2Metadata returns the immutable enriched prompt identity.
func EvaluatorQuestionGenerationV2Metadata() Metadata {
	return metadata("evaluator-question-generation", "2.0.0", evaluatorQuestionGenerationV2)
}

// EvaluatorAnswerAssessmentV2 returns the exact released enriched assessment prompt asset.
func EvaluatorAnswerAssessmentV2() string {
	return evaluatorAnswerAssessmentV2
}

// EvaluatorAnswerAssessmentV2Metadata returns the immutable enriched assessment prompt identity.
func EvaluatorAnswerAssessmentV2Metadata() Metadata {
	return metadata("evaluator-answer-assessment", "2.0.0", evaluatorAnswerAssessmentV2)
}

func metadata(id, version, content string) Metadata {
	digest := sha256.Sum256([]byte(content))
	return Metadata{ID: id, Version: version, SHA256: hex.EncodeToString(digest[:])}
}
