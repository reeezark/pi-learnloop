package evaluator_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

func TestNewInitialAssessmentInput(t *testing.T) {
	input, questions, answers := validAssessmentValues(t)
	got, err := evaluator.NewInitialAssessmentInput(input, questions, answers)
	if err != nil {
		t.Fatalf("NewInitialAssessmentInput() error = %v", err)
	}
	if got.SchemaVersion != evaluator.AssessmentInputSchemaVersion || got.Stage != evaluator.AssessmentStageInitialAnswers || got.FollowUp != nil {
		t.Fatalf("NewInitialAssessmentInput() = %#v, want initial assessment schema", got)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(assessment input): %v", err)
	}
	if strings.Contains(string(encoded), "/private/synthetic-repository") {
		t.Fatalf("assessment input exposed repository root: %s", encoded)
	}

	input.EvidenceBundle.Items[0].Content = "mutated"
	questions.Questions[0].Text = "mutated"
	answers[0].Text = "mutated"
	if strings.Contains(got.EvaluatorInput.EvidenceBundle.Items[0].Content, "mutated") ||
		strings.Contains(got.QuestionSet.Questions[0].Text, "mutated") ||
		strings.Contains(got.Answers[0].Text, "mutated") {
		t.Fatalf("NewInitialAssessmentInput() retained caller aliases: %#v", got)
	}

	t.Run("accepts and preserves LF-only answers for v1 and v2", func(t *testing.T) {
		v1Input, v1Questions, v1Answers := validAssessmentValues(t)
		v2Input, v2Questions, v2Answers := validV2AssessmentValues(t)
		for _, test := range []struct {
			name      string
			input     evaluator.Input
			questions evaluator.QuestionSet
			answers   []evaluator.AssessmentAnswer
		}{
			{name: "v1", input: v1Input, questions: v1Questions, answers: v1Answers},
			{name: "v2", input: v2Input, questions: v2Questions, answers: v2Answers},
		} {
			t.Run(test.name, func(t *testing.T) {
				want := "first line\nsecond line"
				test.answers[0].Text = want
				result, err := evaluator.NewInitialAssessmentInput(test.input, test.questions, test.answers)
				if err != nil {
					t.Fatalf("NewInitialAssessmentInput() error = %v", err)
				}
				if result.Answers[0].Text != want {
					t.Fatalf("answer = %q, want exact LF-preserved %q", result.Answers[0].Text, want)
				}
			})
		}
	})

	tests := []struct {
		name   string
		mutate func(*evaluator.Input, *evaluator.QuestionSet, *[]evaluator.AssessmentAnswer)
	}{
		{name: "mutated evidence", mutate: func(input *evaluator.Input, _ *evaluator.QuestionSet, _ *[]evaluator.AssessmentAnswer) {
			input.EvidenceBundle.Items[0].Content = "changed without updating its hash"
		}},
		{name: "insufficient question set", mutate: func(_ *evaluator.Input, questions *evaluator.QuestionSet, _ *[]evaluator.AssessmentAnswer) {
			questions.Disposition = evaluator.DispositionInsufficientEvidence
			questions.Questions = []evaluator.Question{}
		}},
		{name: "wrong answer count", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			*answers = (*answers)[:2]
		}},
		{name: "wrong answer ID", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[1].QuestionID = "Q9"
		}},
		{name: "whitespace-only multiline answer", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = " \n \n "
		}},
		{name: "answer CR", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first line\rsecond line"
		}},
		{name: "answer CRLF", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first line\r\nsecond line"
		}},
		{name: "answer tab", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first\tsecond"
		}},
		{name: "answer NUL", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first\x00second"
		}},
		{name: "answer DEL", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first\x7fsecond"
		}},
		{name: "answer C1 control", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = "first\u0085second"
		}},
		{name: "answer invalid UTF-8", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = string([]byte{0xff})
		}},
		{name: "oversized answer", mutate: func(_ *evaluator.Input, _ *evaluator.QuestionSet, answers *[]evaluator.AssessmentAnswer) {
			(*answers)[0].Text = strings.Repeat("a", evaluator.MaxAnswerTextBytes+1)
		}},
	}
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			input, questions, answers := validAssessmentValues(t)
			test.mutate(&input, &questions, &answers)
			result, err := evaluator.NewInitialAssessmentInput(input, questions, answers)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
				t.Fatalf("ContractErrorCodeOf(error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidInput, err)
			}
			if !reflect.DeepEqual(result, evaluator.AssessmentInput{}) {
				t.Fatalf("result = %#v, want zero assessment input", result)
			}
		})
	}
}

func TestNewFollowUpAssessmentInput(t *testing.T) {
	initial := validInitialAssessmentInput(t)
	question := validFollowUpQuestion()
	got, err := evaluator.NewFollowUpAssessmentInput(initial, question, "The empty-name branch returns ErrEmpty.")
	if err != nil {
		t.Fatalf("NewFollowUpAssessmentInput() error = %v", err)
	}
	if got.Stage != evaluator.AssessmentStageFollowUpAnswer || got.FollowUp == nil || got.FollowUp.Question.ID != "F1" {
		t.Fatalf("NewFollowUpAssessmentInput() = %#v, want follow-up stage", got)
	}
	question.EvidenceReferences[0] = "MUTATED"
	initial.Answers[0].Text = "mutated"
	if got.FollowUp.Question.EvidenceReferences[0] != "E001" || strings.Contains(got.Answers[0].Text, "mutated") {
		t.Fatalf("NewFollowUpAssessmentInput() retained caller aliases: %#v", got)
	}

	t.Run("accepts and preserves an LF-only follow-up answer", func(t *testing.T) {
		want := "The empty-name branch\nreturns ErrEmpty."
		result, err := evaluator.NewFollowUpAssessmentInput(validInitialAssessmentInput(t), validFollowUpQuestion(), want)
		if err != nil {
			t.Fatalf("NewFollowUpAssessmentInput() error = %v", err)
		}
		if result.FollowUp == nil || result.FollowUp.Answer != want {
			t.Fatalf("follow-up = %#v, want exact LF-preserved answer %q", result.FollowUp, want)
		}
	})

	tests := []struct {
		name         string
		mutateInput  func(*evaluator.AssessmentInput)
		mutateFollow func(*evaluator.FollowUpQuestion, *string)
	}{
		{name: "invalid initial input", mutateInput: func(input *evaluator.AssessmentInput) { input.SchemaVersion++ }},
		{name: "already follow-up stage", mutateInput: func(input *evaluator.AssessmentInput) {
			input.Stage = evaluator.AssessmentStageFollowUpAnswer
			input.FollowUp = &evaluator.FollowUpExchange{Question: validFollowUpQuestion(), Answer: "answer"}
		}},
		{name: "wrong follow-up ID", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) { question.ID = "F2" }},
		{name: "wrong target", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) { question.TargetQuestionID = "Q9" }},
		{name: "empty follow-up text", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) { question.Text = " " }},
		{name: "missing code references", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) { question.EvidenceReferences = []string{} }},
		{name: "unknown reference", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) { question.EvidenceReferences = []string{"E999"} }},
		{name: "duplicate reference", mutateFollow: func(question *evaluator.FollowUpQuestion, _ *string) {
			question.EvidenceReferences = []string{"E001", "E001"}
		}},
		{name: "empty follow-up answer", mutateFollow: func(_ *evaluator.FollowUpQuestion, answer *string) { *answer = " " }},
		{name: "follow-up answer CR", mutateFollow: func(_ *evaluator.FollowUpQuestion, answer *string) { *answer = "first\rsecond" }},
		{name: "follow-up answer tab", mutateFollow: func(_ *evaluator.FollowUpQuestion, answer *string) { *answer = "first\tsecond" }},
		{name: "oversized follow-up answer", mutateFollow: func(_ *evaluator.FollowUpQuestion, answer *string) {
			*answer = strings.Repeat("a", evaluator.MaxAnswerTextBytes+1)
		}},
	}
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			initial := validInitialAssessmentInput(t)
			question := validFollowUpQuestion()
			answer := "The empty-name branch returns ErrEmpty."
			if test.mutateInput != nil {
				test.mutateInput(&initial)
			}
			if test.mutateFollow != nil {
				test.mutateFollow(&question, &answer)
			}
			result, err := evaluator.NewFollowUpAssessmentInput(initial, question, answer)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
				t.Fatalf("ContractErrorCodeOf(error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidInput, err)
			}
			if !reflect.DeepEqual(result, evaluator.AssessmentInput{}) {
				t.Fatalf("result = %#v, want zero assessment input", result)
			}
		})
	}
}

func TestParseAssessmentTurn(t *testing.T) {
	initial := validInitialAssessmentInput(t)

	t.Run("accepts one follow-up for initial answers", func(t *testing.T) {
		got, err := evaluator.ParseAssessmentTurn([]byte(validFollowUpTurnJSON()), initial)
		if err != nil {
			t.Fatalf("ParseAssessmentTurn() error = %v", err)
		}
		if got.Disposition != evaluator.AssessmentDispositionFollowUp || got.FollowUp == nil || got.FollowUp.ID != "F1" || got.Evaluations == nil || len(got.Evaluations) != 0 {
			t.Fatalf("ParseAssessmentTurn() = %#v, want one F1 follow-up", got)
		}
	})

	t.Run("accepts complete evaluations for either stage", func(t *testing.T) {
		inputs := []evaluator.AssessmentInput{initial}
		followUp, err := evaluator.NewFollowUpAssessmentInput(initial, validFollowUpQuestion(), "The branch returns ErrEmpty.")
		if err != nil {
			t.Fatalf("NewFollowUpAssessmentInput(): %v", err)
		}
		inputs = append(inputs, followUp)
		for _, input := range inputs {
			got, err := evaluator.ParseAssessmentTurn([]byte(validCompleteAssessmentJSON()), input)
			if err != nil {
				t.Fatalf("ParseAssessmentTurn(%s) error = %v", input.Stage, err)
			}
			if got.Disposition != evaluator.AssessmentDispositionComplete || got.FollowUp != nil || len(got.Evaluations) != 3 {
				t.Fatalf("ParseAssessmentTurn(%s) = %#v, want complete evaluations", input.Stage, got)
			}
		}
	})

	t.Run("rejects a second follow-up", func(t *testing.T) {
		followUp, err := evaluator.NewFollowUpAssessmentInput(initial, validFollowUpQuestion(), "The branch returns ErrEmpty.")
		if err != nil {
			t.Fatalf("NewFollowUpAssessmentInput(): %v", err)
		}
		_, err = evaluator.ParseAssessmentTurn([]byte(validFollowUpTurnJSON()), followUp)
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidOutput)
		}
	})

	longText, err := json.Marshal(strings.Repeat("a", evaluator.MaxAssessmentTextBytes+1))
	if err != nil {
		t.Fatalf("json.Marshal(long text): %v", err)
	}
	tests := []struct {
		name    string
		content string
	}{
		{name: "free-form prose", content: "The user understands the code."},
		{name: "code fence", content: "```json\n" + validCompleteAssessmentJSON() + "\n```"},
		{name: "unknown top-level field", content: strings.Replace(validCompleteAssessmentJSON(), `"evaluations":`, `"extra":true,"evaluations":`, 1)},
		{name: "case-folded top-level field", content: strings.Replace(validCompleteAssessmentJSON(), `"schema_version"`, `"SCHEMA_VERSION"`, 1)},
		{name: "duplicate top-level field", content: strings.Replace(validCompleteAssessmentJSON(), `"schema_version":1,`, `"schema_version":1,"schema_version":1,`, 1)},
		{name: "trailing JSON", content: validCompleteAssessmentJSON() + ` {}`},
		{name: "unsupported schema", content: strings.Replace(validCompleteAssessmentJSON(), `"schema_version":1`, `"schema_version":2`, 1)},
		{name: "wrong disposition", content: strings.Replace(validCompleteAssessmentJSON(), `"disposition":"complete"`, `"disposition":"accepted"`, 1)},
		{name: "follow-up without question", content: `{"schema_version":1,"disposition":"follow_up","follow_up":null,"evaluations":[]}`},
		{name: "follow-up with evaluations", content: strings.Replace(validFollowUpTurnJSON(), `"evaluations":[]`, `"evaluations":[{"question_id":"Q1","verdict":"partial","feedback":"Needs detail.","evidence_references":["E001"]}]`, 1)},
		{name: "wrong follow-up ID", content: strings.Replace(validFollowUpTurnJSON(), `"id":"F1"`, `"id":"F2"`, 1)},
		{name: "wrong follow-up target", content: strings.Replace(validFollowUpTurnJSON(), `"target_question_id":"Q1"`, `"target_question_id":"Q9"`, 1)},
		{name: "unknown follow-up reference", content: strings.Replace(validFollowUpTurnJSON(), `"evidence_references":["E001"]`, `"evidence_references":["E999"]`, 1)},
		{name: "control character in follow-up text", content: strings.Replace(validFollowUpTurnJSON(), `"text":"Which exact branch supports your first answer?"`, `"text":"first line\nsecond line"`, 1)},
		{name: "complete with follow-up", content: strings.Replace(validCompleteAssessmentJSON(), `"follow_up":null`, `"follow_up":{"id":"F1","target_question_id":"Q1","text":"Why?","evidence_references":["E001"]}`, 1)},
		{name: "wrong evaluation count", content: `{"schema_version":1,"disposition":"complete","follow_up":null,"evaluations":[]}`},
		{name: "wrong evaluation ID", content: strings.Replace(validCompleteAssessmentJSON(), `"question_id":"Q2"`, `"question_id":"Q9"`, 1)},
		{name: "wrong verdict", content: strings.Replace(validCompleteAssessmentJSON(), `"verdict":"partial"`, `"verdict":"almost"`, 1)},
		{name: "empty feedback", content: strings.Replace(validCompleteAssessmentJSON(), `"feedback":"The answer covers one branch but omits the success path."`, `"feedback":" "`, 1)},
		{name: "control character in feedback", content: strings.Replace(validCompleteAssessmentJSON(), `"feedback":"The answer covers one branch but omits the success path."`, `"feedback":"first line\nsecond line"`, 1)},
		{name: "oversized feedback", content: strings.Replace(validCompleteAssessmentJSON(), `"The answer covers one branch but omits the success path."`, string(longText), 1)},
		{name: "unknown evaluation field", content: strings.Replace(validCompleteAssessmentJSON(), `"question_id":"Q1"`, `"question_id":"Q1","score":100`, 1)},
		{name: "null references", content: strings.Replace(validCompleteAssessmentJSON(), `"evidence_references":["E001"]`, `"evidence_references":null`, 1)},
		{name: "missing code references", content: strings.Replace(validCompleteAssessmentJSON(), `"evidence_references":["E001"]`, `"evidence_references":[]`, 1)},
		{name: "unknown evaluation reference", content: strings.Replace(validCompleteAssessmentJSON(), `"evidence_references":["E001"]`, `"evidence_references":["E999"]`, 1)},
		{name: "duplicate evaluation reference", content: strings.Replace(validCompleteAssessmentJSON(), `"evidence_references":["E001"]`, `"evidence_references":["E001","E001"]`, 1)},
	}
	for _, test := range tests {
		t.Run("rejects "+test.name, func(t *testing.T) {
			got, err := evaluator.ParseAssessmentTurn([]byte(test.content), initial)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
				t.Fatalf("ContractErrorCodeOf(error) = %q, want %q (error = %v)", code, evaluator.ContractErrorInvalidOutput, err)
			}
			if !reflect.DeepEqual(got, evaluator.AssessmentTurn{}) {
				t.Fatalf("result = %#v, want zero assessment turn", got)
			}
		})
	}

	t.Run("rejects invalid UTF-8 and oversized output", func(t *testing.T) {
		for _, content := range [][]byte{{0xff}, []byte(strings.Repeat(" ", evaluator.MaxAssessmentTurnBytes+1))} {
			_, err := evaluator.ParseAssessmentTurn(content, initial)
			if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidOutput {
				t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidOutput)
			}
		}
	})

	t.Run("does not echo untrusted output", func(t *testing.T) {
		content := strings.Replace(validCompleteAssessmentJSON(), `"evaluations":`, `"TOP_SECRET":true,"evaluations":`, 1)
		_, err := evaluator.ParseAssessmentTurn([]byte(content), initial)
		if err == nil || strings.Contains(err.Error(), "TOP_SECRET") {
			t.Fatalf("ParseAssessmentTurn() error exposed untrusted output: %v", err)
		}
	})

	t.Run("rejects an invalid assessment input", func(t *testing.T) {
		invalid := initial
		invalid.EvaluatorInput.EvidenceBundle.Items[0].Content = "mutated"
		_, err := evaluator.ParseAssessmentTurn([]byte(validCompleteAssessmentJSON()), invalid)
		if code := evaluator.ContractErrorCodeOf(err); code != evaluator.ContractErrorInvalidInput {
			t.Fatalf("ContractErrorCodeOf(error) = %q, want %q", code, evaluator.ContractErrorInvalidInput)
		}
	})
}

func TestDeriveAssessmentLabel(t *testing.T) {
	verdicts := []evaluator.AssessmentVerdict{
		evaluator.AssessmentVerdictDemonstrated,
		evaluator.AssessmentVerdictPartial,
		evaluator.AssessmentVerdictNotDemonstrated,
	}
	for _, first := range verdicts {
		for _, second := range verdicts {
			for _, third := range verdicts {
				turn := evaluator.AssessmentTurn{
					SchemaVersion: evaluator.AssessmentTurnSchemaVersion,
					Disposition:   evaluator.AssessmentDispositionComplete,
					Evaluations: []evaluator.QuestionEvaluation{
						{QuestionID: "Q1", Verdict: first},
						{QuestionID: "Q2", Verdict: second},
						{QuestionID: "Q3", Verdict: third},
					},
				}
				want := evaluator.AssessmentLabelPartial
				notDemonstrated := 0
				for _, verdict := range []evaluator.AssessmentVerdict{first, second, third} {
					if verdict == evaluator.AssessmentVerdictNotDemonstrated {
						notDemonstrated++
					}
				}
				if first == evaluator.AssessmentVerdictDemonstrated && second == first && third == first {
					want = evaluator.AssessmentLabelUnderstood
				} else if notDemonstrated >= 2 {
					want = evaluator.AssessmentLabelReviewNeeded
				}
				got, err := evaluator.DeriveAssessmentLabel(turn)
				if err != nil || got != want {
					t.Fatalf("DeriveAssessmentLabel(%s,%s,%s) = (%q, %v), want %q", first, second, third, got, err, want)
				}
			}
		}
	}

	tests := []evaluator.AssessmentTurn{
		{},
		{SchemaVersion: evaluator.AssessmentTurnSchemaVersion, Disposition: evaluator.AssessmentDispositionFollowUp},
		{
			SchemaVersion: evaluator.AssessmentTurnSchemaVersion,
			Disposition:   evaluator.AssessmentDispositionComplete,
			Evaluations: []evaluator.QuestionEvaluation{
				{QuestionID: "Q9", Verdict: evaluator.AssessmentVerdictDemonstrated},
				{QuestionID: "Q2", Verdict: evaluator.AssessmentVerdictDemonstrated},
				{QuestionID: "Q3", Verdict: evaluator.AssessmentVerdictDemonstrated},
			},
		},
		{
			SchemaVersion: evaluator.AssessmentTurnSchemaVersion,
			Disposition:   evaluator.AssessmentDispositionComplete,
			Evaluations: []evaluator.QuestionEvaluation{
				{QuestionID: "Q1", Verdict: "unknown"},
				{QuestionID: "Q2", Verdict: evaluator.AssessmentVerdictDemonstrated},
				{QuestionID: "Q3", Verdict: evaluator.AssessmentVerdictDemonstrated},
			},
		},
	}
	for index, turn := range tests {
		if _, err := evaluator.DeriveAssessmentLabel(turn); evaluator.ContractErrorCodeOf(err) != evaluator.ContractErrorInvalidInput {
			t.Fatalf("invalid turn %d error = %v, want invalid_input", index, err)
		}
	}
}

func validAssessmentValues(t *testing.T) (evaluator.Input, evaluator.QuestionSet, []evaluator.AssessmentAnswer) {
	t.Helper()
	input, err := evaluator.NewInput(validBundle(t))
	if err != nil {
		t.Fatalf("NewInput(valid bundle): %v", err)
	}
	questions, err := evaluator.ParseQuestionSet([]byte(validQuestionSetJSON()), []string{"E001"})
	if err != nil {
		t.Fatalf("ParseQuestionSet(valid questions): %v", err)
	}
	answers := []evaluator.AssessmentAnswer{
		{QuestionID: "Q1", Text: "Validate returns ErrEmpty when name is empty."},
		{QuestionID: "Q2", Text: "The success branch returns nil after validation."},
		{QuestionID: "Q3", Text: "A table-driven test can cover empty and non-empty names."},
	}
	return input, questions, answers
}

func validV2AssessmentValues(t *testing.T) (evaluator.Input, evaluator.QuestionSet, []evaluator.AssessmentAnswer) {
	t.Helper()
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
		t.Fatalf("v2 references = %v, want a context reference", references)
	}
	questionJSON := `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"What does the selected context prove?","evidence_references":["` + contextReference + `"]},{"id":"Q2","kind":"code_specific","text":"How does the changed code use that context?","evidence_references":["` + contextReference + `"]},{"id":"Q3","kind":"go_backend","text":"Why report incomplete type evidence?","evidence_references":[]}]}`
	questions, err := evaluator.ParseQuestionSet([]byte(questionJSON), references)
	if err != nil {
		t.Fatalf("ParseQuestionSet(v2 references): %v", err)
	}
	answers := []evaluator.AssessmentAnswer{
		{QuestionID: "Q1", Text: "The context item is a selected-snapshot declaration."},
		{QuestionID: "Q2", Text: "The recorded relation connects the changed code to it."},
		{QuestionID: "Q3", Text: "Explicit omissions prevent unsupported conclusions."},
	}
	return input, questions, answers
}

func validInitialAssessmentInput(t *testing.T) evaluator.AssessmentInput {
	t.Helper()
	input, questions, answers := validAssessmentValues(t)
	result, err := evaluator.NewInitialAssessmentInput(input, questions, answers)
	if err != nil {
		t.Fatalf("NewInitialAssessmentInput(valid values): %v", err)
	}
	return result
}

func validFollowUpQuestion() evaluator.FollowUpQuestion {
	return evaluator.FollowUpQuestion{
		ID:                 "F1",
		TargetQuestionID:   "Q1",
		Text:               "Which exact branch supports your first answer?",
		EvidenceReferences: []string{"E001"},
	}
}

func validFollowUpTurnJSON() string {
	return `{"schema_version":1,"disposition":"follow_up","follow_up":{"id":"F1","target_question_id":"Q1","text":"Which exact branch supports your first answer?","evidence_references":["E001"]},"evaluations":[]}`
}

func validCompleteAssessmentJSON() string {
	return `{"schema_version":1,"disposition":"complete","follow_up":null,"evaluations":[{"question_id":"Q1","verdict":"demonstrated","feedback":"The answer identifies the empty-name error branch.","evidence_references":["E001"]},{"question_id":"Q2","verdict":"partial","feedback":"The answer covers one branch but omits the success path.","evidence_references":["E001"]},{"question_id":"Q3","verdict":"not_demonstrated","feedback":"The answer needs a clearer Go testing explanation.","evidence_references":[]}]}`
}
