package history

import "time"

type Status string

const (
	StatusRunning     Status = "running"
	StatusComplete    Status = "complete"
	StatusFailed      Status = "failed"
	StatusInterrupted Status = "interrupted"
)

type Label string

const (
	LabelUnderstood   Label = "understood"
	LabelPartial      Label = "partial"
	LabelReviewNeeded Label = "review_needed"
)

type FailureCode string

const (
	FailureEvaluatorFailed        FailureCode = "evaluator_failed"
	FailureEvaluatorInvalidOutput FailureCode = "evaluator_invalid_output"
	FailureEvaluatorTimeout       FailureCode = "evaluator_timeout"
)

type QuestionKind string

const (
	QuestionKindCodeSpecific QuestionKind = "code_specific"
	QuestionKindGoBackend    QuestionKind = "go_backend"
)

type Verdict string

const (
	VerdictDemonstrated    Verdict = "demonstrated"
	VerdictPartial         Verdict = "partial"
	VerdictNotDemonstrated Verdict = "not_demonstrated"
)

type PromptProvenance struct {
	ID      string
	Version string
	SHA256  string
}

type Start struct {
	CanonicalRoot           string
	StartedAt               time.Time
	BaseRevision            string
	HeadRevision            string
	EvidenceManifestSHA256  string
	QuestionSchemaVersion   int
	AssessmentSchemaVersion int
	QuestionPrompt          PromptProvenance
	AssessmentPrompt        PromptProvenance
	PiVersion               string
	Provider                string
	ModelID                 string
	ThinkingLevel           string
}

type Outcome struct {
	QuestionID   string
	QuestionKind QuestionKind
	Verdict      Verdict
}

type Completion struct {
	FinishedAt time.Time
	Label      Label
	Outcomes   []Outcome
}

type Failure struct {
	FinishedAt time.Time
	Code       FailureCode
}

type Record struct {
	RecordID     string
	Start        Start
	FinishedAt   *time.Time
	Status       Status
	FailureCode  FailureCode
	FollowUpUsed bool
	Label        Label
	Outcomes     []Outcome
}
