package evaluator

import (
	"context"
	"encoding/json"
	"errors"
)

// AssessmentEvaluator is the execution seam between validated answers and an
// isolated evaluator. It receives no repository path, credentials, tools, or
// development Session.
type AssessmentEvaluator interface {
	EvaluateAssessment(context.Context, AssessmentInput, ModelSelection) (AssessmentTurn, error)
}

// DeterministicAssessmentEvaluator exercises the assessment contracts without
// starting Pi or contacting a model. It is a test fixture, never a production
// fallback.
type DeterministicAssessmentEvaluator struct {
	RequestFollowUp bool
}

func (adapter DeterministicAssessmentEvaluator) EvaluateAssessment(ctx context.Context, input AssessmentInput, selection ModelSelection) (AssessmentTurn, error) {
	if ctx == nil {
		return AssessmentTurn{}, invalidInput(errors.New("assessment context is required"))
	}
	if err := ctx.Err(); err != nil {
		return AssessmentTurn{}, err
	}
	if err := ValidateModelSelection(selection); err != nil {
		return AssessmentTurn{}, err
	}
	if err := validateAssessmentInput(input); err != nil {
		return AssessmentTurn{}, invalidInput(err)
	}

	var fixture AssessmentTurn
	if input.Stage == AssessmentStageInitialAnswers && adapter.RequestFollowUp {
		fixture = AssessmentTurn{
			SchemaVersion: AssessmentTurnSchemaVersion,
			Disposition:   AssessmentDispositionFollowUp,
			FollowUp: &FollowUpQuestion{
				ID:                 "F1",
				TargetQuestionID:   "Q1",
				Text:               "Which exact branch in the selected code supports your first answer?",
				EvidenceReferences: append([]string(nil), input.QuestionSet.Questions[0].EvidenceReferences...),
			},
			Evaluations: []QuestionEvaluation{},
		}
	} else {
		fixture = AssessmentTurn{
			SchemaVersion: AssessmentTurnSchemaVersion,
			Disposition:   AssessmentDispositionComplete,
			FollowUp:      nil,
			Evaluations: []QuestionEvaluation{
				{
					QuestionID:         "Q1",
					Verdict:            AssessmentVerdictDemonstrated,
					Feedback:           "The answer identifies the selected code behavior and its relevant branch.",
					EvidenceReferences: append([]string(nil), input.QuestionSet.Questions[0].EvidenceReferences...),
				},
				{
					QuestionID:         "Q2",
					Verdict:            AssessmentVerdictPartial,
					Feedback:           "The answer identifies an edge case but does not explain every selected path.",
					EvidenceReferences: append([]string(nil), input.QuestionSet.Questions[1].EvidenceReferences...),
				},
				{
					QuestionID:         "Q3",
					Verdict:            AssessmentVerdictNotDemonstrated,
					Feedback:           "The answer needs a clearer explanation of the underlying Go testing principle.",
					EvidenceReferences: []string{},
				},
			},
		}
	}
	content, err := json.Marshal(fixture)
	if err != nil {
		return AssessmentTurn{}, err
	}
	return ParseAssessmentTurn(content, input)
}
