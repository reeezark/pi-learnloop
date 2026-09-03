package evaluator

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

const (
	AssessmentInputSchemaVersion   = 1
	AssessmentInputSchemaVersionV2 = 2
	AssessmentTurnSchemaVersion    = 1
	MaxAnswerTextBytes             = 4 * 1024
	MaxAssessmentTextBytes         = 1000
	MaxAssessmentTurnBytes         = 64 * 1024
)

type AssessmentStage string

const (
	AssessmentStageInitialAnswers AssessmentStage = "initial_answers"
	AssessmentStageFollowUpAnswer AssessmentStage = "follow_up_answer"
)

// AssessmentInput is the only runtime value that an answer evaluator may
// receive. It owns the validated evidence, questions, and user answers.
type AssessmentInput struct {
	SchemaVersion  int                `json:"schema_version"`
	Stage          AssessmentStage    `json:"stage"`
	EvaluatorInput Input              `json:"evaluator_input"`
	QuestionSet    QuestionSet        `json:"question_set"`
	Answers        []AssessmentAnswer `json:"answers"`
	FollowUp       *FollowUpExchange  `json:"follow_up"`
}

type AssessmentAnswer struct {
	QuestionID string `json:"question_id"`
	Text       string `json:"text"`
}

type FollowUpExchange struct {
	Question FollowUpQuestion `json:"question"`
	Answer   string           `json:"answer"`
}

type AssessmentDisposition string

const (
	AssessmentDispositionFollowUp AssessmentDisposition = "follow_up"
	AssessmentDispositionComplete AssessmentDisposition = "complete"
)

type AssessmentTurn struct {
	SchemaVersion int                   `json:"schema_version"`
	Disposition   AssessmentDisposition `json:"disposition"`
	FollowUp      *FollowUpQuestion     `json:"follow_up"`
	Evaluations   []QuestionEvaluation  `json:"evaluations"`
}

type FollowUpQuestion struct {
	ID                 string   `json:"id"`
	TargetQuestionID   string   `json:"target_question_id"`
	Text               string   `json:"text"`
	EvidenceReferences []string `json:"evidence_references"`
}

type AssessmentVerdict string

const (
	AssessmentVerdictDemonstrated    AssessmentVerdict = "demonstrated"
	AssessmentVerdictPartial         AssessmentVerdict = "partial"
	AssessmentVerdictNotDemonstrated AssessmentVerdict = "not_demonstrated"
)

type QuestionEvaluation struct {
	QuestionID         string            `json:"question_id"`
	Verdict            AssessmentVerdict `json:"verdict"`
	Feedback           string            `json:"feedback"`
	EvidenceReferences []string          `json:"evidence_references"`
}

type AssessmentLabel string

const (
	AssessmentLabelUnderstood   AssessmentLabel = "understood"
	AssessmentLabelPartial      AssessmentLabel = "partial"
	AssessmentLabelReviewNeeded AssessmentLabel = "review_needed"
)

func NewInitialAssessmentInput(input Input, questionSet QuestionSet, answers []AssessmentAnswer) (AssessmentInput, error) {
	ownedInput, references, err := cloneValidatedRuntimeInput(input)
	if err != nil {
		return AssessmentInput{}, invalidInput(errors.New("validated evaluator input is required"))
	}
	ownedQuestions, err := cloneValidatedQuestions(questionSet, references)
	if err != nil {
		return AssessmentInput{}, invalidInput(errors.New("validated question set is required"))
	}
	ownedAnswers, err := cloneValidatedAnswers(answers)
	if err != nil {
		return AssessmentInput{}, invalidInput(err)
	}
	schemaVersion := AssessmentInputSchemaVersion
	if ownedInput.SchemaVersion == InputSchemaVersionV2 {
		schemaVersion = AssessmentInputSchemaVersionV2
	}
	return AssessmentInput{
		SchemaVersion:  schemaVersion,
		Stage:          AssessmentStageInitialAnswers,
		EvaluatorInput: ownedInput,
		QuestionSet:    ownedQuestions,
		Answers:        ownedAnswers,
		FollowUp:       nil,
	}, nil
}

func NewFollowUpAssessmentInput(initial AssessmentInput, question FollowUpQuestion, answer string) (AssessmentInput, error) {
	if err := validateAssessmentInput(initial); err != nil || initial.Stage != AssessmentStageInitialAnswers {
		return AssessmentInput{}, invalidInput(errors.New("validated initial assessment input is required"))
	}
	references, err := assessmentReferences(initial.EvaluatorInput)
	if err != nil {
		return AssessmentInput{}, invalidInput(errors.New("validated evaluator input is required"))
	}
	if err := validateFollowUpQuestion(question, initial.QuestionSet, references); err != nil {
		return AssessmentInput{}, invalidInput(err)
	}
	if err := validateAnswerText(answer, MaxAnswerTextBytes, "follow-up answer"); err != nil {
		return AssessmentInput{}, invalidInput(err)
	}

	owned, err := cloneAssessmentInput(initial)
	if err != nil {
		return AssessmentInput{}, invalidInput(errors.New("assessment input cannot be copied"))
	}
	owned.Stage = AssessmentStageFollowUpAnswer
	owned.FollowUp = &FollowUpExchange{
		Question: cloneFollowUpQuestion(question),
		Answer:   answer,
	}
	return owned, nil
}

// ParseAssessmentTurn accepts exactly one strict JSON object and validates it
// against the stage, questions, and evidence references in input.
func ParseAssessmentTurn(content []byte, input AssessmentInput) (AssessmentTurn, error) {
	if err := validateAssessmentInput(input); err != nil {
		return AssessmentTurn{}, invalidInput(err)
	}
	if len(content) == 0 {
		return AssessmentTurn{}, invalidOutput("assessment output is empty")
	}
	if len(content) > MaxAssessmentTurnBytes {
		return AssessmentTurn{}, invalidOutput("assessment output exceeds %d bytes", MaxAssessmentTurnBytes)
	}
	if !utf8.Valid(content) {
		return AssessmentTurn{}, invalidOutput("assessment output is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(content); err != nil {
		return AssessmentTurn{}, invalidOutput("assessment output is not strict JSON")
	}
	if err := validateAssessmentTurnObjectKeys(content); err != nil {
		return AssessmentTurn{}, invalidOutput("assessment output does not use the exact schema fields")
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var result AssessmentTurn
	if err := decoder.Decode(&result); err != nil {
		return AssessmentTurn{}, invalidOutput("assessment output is not valid")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return AssessmentTurn{}, invalidOutput("assessment output contains trailing content")
	}
	if result.SchemaVersion != AssessmentTurnSchemaVersion {
		return AssessmentTurn{}, invalidOutput("unsupported assessment schema version")
	}

	references, err := assessmentReferences(input.EvaluatorInput)
	if err != nil {
		return AssessmentTurn{}, invalidInput(err)
	}
	switch result.Disposition {
	case AssessmentDispositionFollowUp:
		if input.Stage != AssessmentStageInitialAnswers {
			return AssessmentTurn{}, invalidOutput("follow-up is not allowed for this assessment stage")
		}
		if result.FollowUp == nil || result.Evaluations == nil || len(result.Evaluations) != 0 {
			return AssessmentTurn{}, invalidOutput("follow-up result requires one question and an explicit empty evaluations array")
		}
		if err := validateFollowUpQuestion(*result.FollowUp, input.QuestionSet, references); err != nil {
			return AssessmentTurn{}, invalidOutput("follow-up question is invalid")
		}
	case AssessmentDispositionComplete:
		if result.FollowUp != nil {
			return AssessmentTurn{}, invalidOutput("complete result cannot contain a follow-up")
		}
		if err := validateQuestionEvaluations(result.Evaluations, input.QuestionSet, references); err != nil {
			return AssessmentTurn{}, invalidOutput("question evaluations are invalid")
		}
	default:
		return AssessmentTurn{}, invalidOutput("assessment disposition is invalid")
	}
	return result, nil
}

func DeriveAssessmentLabel(turn AssessmentTurn) (AssessmentLabel, error) {
	if turn.SchemaVersion != AssessmentTurnSchemaVersion || turn.Disposition != AssessmentDispositionComplete || turn.FollowUp != nil || len(turn.Evaluations) != 3 {
		return "", invalidInput(errors.New("validated complete assessment turn is required"))
	}
	notDemonstrated := 0
	for index, evaluation := range turn.Evaluations {
		if evaluation.QuestionID != fmt.Sprintf("Q%d", index+1) {
			return "", invalidInput(errors.New("validated complete assessment turn is required"))
		}
		switch evaluation.Verdict {
		case AssessmentVerdictDemonstrated:
		case AssessmentVerdictPartial:
		case AssessmentVerdictNotDemonstrated:
			notDemonstrated++
		default:
			return "", invalidInput(errors.New("validated complete assessment turn is required"))
		}
	}
	if notDemonstrated >= 2 {
		return AssessmentLabelReviewNeeded, nil
	}
	for _, evaluation := range turn.Evaluations {
		if evaluation.Verdict != AssessmentVerdictDemonstrated {
			return AssessmentLabelPartial, nil
		}
	}
	return AssessmentLabelUnderstood, nil
}

func validateAssessmentInput(input AssessmentInput) error {
	expectedSchemaVersion := AssessmentInputSchemaVersion
	if input.EvaluatorInput.SchemaVersion == InputSchemaVersionV2 {
		expectedSchemaVersion = AssessmentInputSchemaVersionV2
	}
	if input.SchemaVersion != expectedSchemaVersion {
		return errors.New("unsupported assessment input schema version")
	}
	_, references, err := cloneValidatedRuntimeInput(input.EvaluatorInput)
	if err != nil {
		return errors.New("validated evaluator input is required")
	}
	if _, err := cloneValidatedQuestions(input.QuestionSet, references); err != nil {
		return errors.New("validated question set is required")
	}
	if _, err := cloneValidatedAnswers(input.Answers); err != nil {
		return err
	}
	switch input.Stage {
	case AssessmentStageInitialAnswers:
		if input.FollowUp != nil {
			return errors.New("initial assessment input cannot contain a follow-up")
		}
	case AssessmentStageFollowUpAnswer:
		if input.FollowUp == nil {
			return errors.New("follow-up assessment input requires one exchange")
		}
		if err := validateFollowUpQuestion(input.FollowUp.Question, input.QuestionSet, references); err != nil {
			return err
		}
		if err := validateAnswerText(input.FollowUp.Answer, MaxAnswerTextBytes, "follow-up answer"); err != nil {
			return err
		}
	default:
		return errors.New("assessment input stage is invalid")
	}
	return nil
}

func cloneValidatedRuntimeInput(input Input) (Input, []string, error) {
	content, err := json.Marshal(input)
	if err != nil {
		return Input{}, nil, err
	}
	var owned Input
	if err := json.Unmarshal(content, &owned); err != nil {
		return Input{}, nil, err
	}
	references, err := validatedInputReferences(owned)
	if err != nil {
		return Input{}, nil, err
	}
	return owned, references, nil
}

func runtimeBundleToDomain(bundle EvidenceBundle) evidence.Bundle {
	domain := evidence.Bundle{
		FormatVersion:  bundle.FormatVersion,
		ID:             bundle.ID,
		ManifestSHA256: bundle.ManifestSHA256,
		BaseRevision:   bundle.BaseRevision,
		HeadRevision:   bundle.HeadRevision,
		AppliedLimits: evidence.Limits{
			MaxFiles:        bundle.AppliedLimits.MaxFiles,
			MaxDeclarations: bundle.AppliedLimits.MaxDeclarations,
			MaxExcerptBytes: bundle.AppliedLimits.MaxExcerptBytes,
		},
		FileCount:        bundle.FileCount,
		DeclarationCount: bundle.DeclarationCount,
		EvidenceCount:    bundle.EvidenceCount,
		ApproximateBytes: bundle.ApproximateBytes,
		Files:            make([]evidence.BundleFile, len(bundle.Files)),
		Items:            make([]evidence.BundleItem, len(bundle.Items)),
		Truncation: evidence.Truncation{
			Truncated:           bundle.Truncation.Truncated,
			OmittedFiles:        bundle.Truncation.OmittedFiles,
			OmittedDeclarations: bundle.Truncation.OmittedDeclarations,
			OmittedExcerptBytes: bundle.Truncation.OmittedExcerptBytes,
		},
	}
	for index, file := range bundle.Files {
		domain.Files[index] = evidence.BundleFile{
			Path:               file.Path,
			Status:             evidence.FileStatus(file.Status),
			ChangedLines:       domainRanges(file.ChangedLines),
			EvidenceReferences: append([]string(nil), file.EvidenceReferences...),
			Omissions:          domainOmissions(file.Omissions),
		}
	}
	for index, item := range bundle.Items {
		domain.Items[index] = evidence.BundleItem{
			Reference:       item.Reference,
			Kind:            evidence.BundleItemKind(item.Kind),
			Path:            item.Path,
			DeclarationKind: evidence.DeclarationKind(item.DeclarationKind),
			Identity:        item.Identity,
			StartLine:       item.StartLine,
			EndLine:         item.EndLine,
			ChangedLines:    domainRanges(item.ChangedLines),
			Content:         item.Content,
			ContentBytes:    item.ContentBytes,
			ContentSHA256:   item.ContentSHA256,
			Truncated:       item.Truncated,
		}
	}
	return domain
}

func domainRanges(ranges []EvidenceLineRange) []evidence.LineRange {
	if len(ranges) == 0 {
		return nil
	}
	result := make([]evidence.LineRange, len(ranges))
	for index, lineRange := range ranges {
		result[index] = evidence.LineRange{Start: lineRange.Start, End: lineRange.End}
	}
	return result
}

func domainOmissions(omissions []EvidenceOmission) []evidence.Omission {
	if len(omissions) == 0 {
		return nil
	}
	result := make([]evidence.Omission, len(omissions))
	for index, omission := range omissions {
		result[index] = evidence.Omission{Reason: evidence.OmissionReason(omission.Reason), Count: omission.Count}
	}
	return result
}

func assessmentReferences(input Input) ([]string, error) {
	return validatedInputReferences(input)
}

func cloneValidatedQuestions(questionSet QuestionSet, references []string) (QuestionSet, error) {
	content, err := json.Marshal(questionSet)
	if err != nil {
		return QuestionSet{}, err
	}
	owned, err := ParseQuestionSet(content, references)
	if err != nil || owned.Disposition != DispositionQuestions {
		return QuestionSet{}, errors.New("question disposition must contain the fixed three questions")
	}
	return owned, nil
}

func cloneValidatedAnswers(answers []AssessmentAnswer) ([]AssessmentAnswer, error) {
	if len(answers) != 3 {
		return nil, errors.New("assessment requires exactly three answers")
	}
	owned := make([]AssessmentAnswer, len(answers))
	for index, answer := range answers {
		if answer.QuestionID != fmt.Sprintf("Q%d", index+1) {
			return nil, fmt.Errorf("answer %d has an invalid question ID", index+1)
		}
		if err := validateAnswerText(answer.Text, MaxAnswerTextBytes, fmt.Sprintf("answer %d", index+1)); err != nil {
			return nil, err
		}
		owned[index] = answer
	}
	return owned, nil
}

func validateFollowUpQuestion(question FollowUpQuestion, questionSet QuestionSet, references []string) error {
	if question.ID != "F1" {
		return errors.New("follow-up question ID is invalid")
	}
	targetIndex := questionIndex(question.TargetQuestionID)
	if targetIndex < 0 || targetIndex >= len(questionSet.Questions) {
		return errors.New("follow-up target question is invalid")
	}
	if err := validateAssessmentText(question.Text, MaxAssessmentTextBytes, "follow-up question"); err != nil {
		return err
	}
	if err := validateAssessmentReferences(question.EvidenceReferences, references, questionSet.Questions[targetIndex].Kind == QuestionKindCodeSpecific); err != nil {
		return err
	}
	return nil
}

func validateQuestionEvaluations(evaluations []QuestionEvaluation, questionSet QuestionSet, references []string) error {
	if len(evaluations) != 3 {
		return errors.New("complete assessment requires exactly three evaluations")
	}
	for index, evaluation := range evaluations {
		if evaluation.QuestionID != fmt.Sprintf("Q%d", index+1) {
			return errors.New("question evaluation ID is invalid")
		}
		switch evaluation.Verdict {
		case AssessmentVerdictDemonstrated, AssessmentVerdictPartial, AssessmentVerdictNotDemonstrated:
		default:
			return errors.New("question evaluation verdict is invalid")
		}
		if err := validateAssessmentText(evaluation.Feedback, MaxAssessmentTextBytes, "question feedback"); err != nil {
			return err
		}
		if err := validateAssessmentReferences(evaluation.EvidenceReferences, references, questionSet.Questions[index].Kind == QuestionKindCodeSpecific); err != nil {
			return err
		}
	}
	return nil
}

func validateAssessmentReferences(values, allowedValues []string, required bool) error {
	if values == nil {
		return errors.New("evidence references must be an explicit array")
	}
	allowed, err := referenceSet(allowedValues)
	if err != nil {
		return err
	}
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if _, duplicate := seen[value]; duplicate {
			return errors.New("evidence reference is duplicated")
		}
		seen[value] = struct{}{}
		if _, exists := allowed[value]; !exists {
			return errors.New("evidence reference is unknown")
		}
	}
	if required && len(values) == 0 {
		return errors.New("code-specific assessment requires evidence references")
	}
	return nil
}

func validateAssessmentText(value string, maximumBytes int, field string) error {
	return validateBoundedText(value, maximumBytes, field, false)
}

func validateAnswerText(value string, maximumBytes int, field string) error {
	return validateBoundedText(value, maximumBytes, field, true)
}

func validateBoundedText(value string, maximumBytes int, field string, allowLineFeed bool) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return fmt.Errorf("%s is empty or invalid UTF-8", field)
	}
	if len(value) > maximumBytes {
		return fmt.Errorf("%s exceeds %d bytes", field, maximumBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) && !(allowLineFeed && character == '\n') {
			return fmt.Errorf("%s contains a control character", field)
		}
	}
	return nil
}

func questionIndex(id string) int {
	switch id {
	case "Q1":
		return 0
	case "Q2":
		return 1
	case "Q3":
		return 2
	default:
		return -1
	}
}

func cloneAssessmentInput(input AssessmentInput) (AssessmentInput, error) {
	content, err := json.Marshal(input)
	if err != nil {
		return AssessmentInput{}, err
	}
	var owned AssessmentInput
	if err := json.Unmarshal(content, &owned); err != nil {
		return AssessmentInput{}, err
	}
	return owned, nil
}

func cloneFollowUpQuestion(question FollowUpQuestion) FollowUpQuestion {
	question.EvidenceReferences = append([]string(nil), question.EvidenceReferences...)
	return question
}

func validateAssessmentTurnObjectKeys(content []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil || !hasExactKeys(object, "schema_version", "disposition", "follow_up", "evaluations") {
		return errors.New("assessment fields are invalid")
	}
	if !bytes.Equal(bytes.TrimSpace(object["follow_up"]), []byte("null")) {
		var followUp map[string]json.RawMessage
		if err := json.Unmarshal(object["follow_up"], &followUp); err != nil || !hasExactKeys(followUp, "id", "target_question_id", "text", "evidence_references") {
			return errors.New("follow-up fields are invalid")
		}
	}
	var evaluations []json.RawMessage
	if err := json.Unmarshal(object["evaluations"], &evaluations); err != nil {
		return errors.New("evaluations are invalid")
	}
	for _, raw := range evaluations {
		var evaluation map[string]json.RawMessage
		if err := json.Unmarshal(raw, &evaluation); err != nil || !hasExactKeys(evaluation, "question_id", "verdict", "feedback", "evidence_references") {
			return errors.New("evaluation fields are invalid")
		}
	}
	return nil
}
