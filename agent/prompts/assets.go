// Package prompts embeds immutable released evaluator prompts for production
// use without relying on a repository-relative runtime path.
package prompts

import _ "embed"

//go:embed evaluator-question-generation/v1.0.0.md
var evaluatorQuestionGenerationV1 string

//go:embed evaluator-answer-assessment/v1.0.0.md
var evaluatorAnswerAssessmentV1 string

// EvaluatorQuestionGenerationV1 returns the exact released prompt asset.
func EvaluatorQuestionGenerationV1() string {
	return evaluatorQuestionGenerationV1
}

// EvaluatorAnswerAssessmentV1 returns the exact released prompt asset.
func EvaluatorAnswerAssessmentV1() string {
	return evaluatorAnswerAssessmentV1
}
