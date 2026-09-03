package evaluator

import (
	"context"
	"encoding/json"
	"errors"
)

// QuestionEvaluator is the only execution seam between validated retained
// evidence and a question generator. Implementations receive neither a
// repository path nor credentials, tools, or a development Session.
type QuestionEvaluator interface {
	Evaluate(context.Context, Input, ModelSelection) (QuestionSet, error)
}

// DeterministicEvaluator exercises the production contract and validator
// without starting Pi or contacting a model. It is the Phase 2 adapter only.
type DeterministicEvaluator struct{}

func (DeterministicEvaluator) Evaluate(ctx context.Context, input Input, selection ModelSelection) (QuestionSet, error) {
	if err := ctx.Err(); err != nil {
		return QuestionSet{}, err
	}
	if err := ValidateModelSelection(selection); err != nil {
		return QuestionSet{}, err
	}
	references, err := runtimeInputReferences(input)
	if err != nil {
		return QuestionSet{}, invalidInput(errors.New("deterministic evaluator requires validated evidence input"))
	}
	secondReference := references[0]
	if len(references) > 1 {
		secondReference = references[1]
	}
	fixture := QuestionSet{
		SchemaVersion: QuestionSetSchemaVersion,
		Disposition:   DispositionQuestions,
		Questions: []Question{
			{
				ID:                 "Q1",
				Kind:               QuestionKindCodeSpecific,
				Text:               "What behavior is implemented by the first selected declaration, including its error paths?",
				EvidenceReferences: []string{references[0]},
			},
			{
				ID:                 "Q2",
				Kind:               QuestionKindCodeSpecific,
				Text:               "Which edge cases should be tested for the selected code change?",
				EvidenceReferences: []string{secondReference},
			},
			{
				ID:                 "Q3",
				Kind:               QuestionKindGoBackend,
				Text:               "How would you design table-driven Go tests for the changed backend behavior?",
				EvidenceReferences: []string{},
			},
		},
	}
	content, err := json.Marshal(fixture)
	if err != nil {
		return QuestionSet{}, err
	}
	return ParseQuestionSet(content, references)
}
