// Package assessment owns the bounded, volatile answer-assessment lifecycle.
package assessment

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/history"
)

const (
	assessmentLifetime        = 30 * time.Minute
	maxLiveAssessments        = 8
	maxRetainedEvidenceBytes  = 1024 * 1024
	assessmentIdentifierBytes = 32
	historyWriteTimeout       = 5 * time.Second
)

var (
	ErrInvalidStart      = errors.New("assessment: invalid start context")
	ErrInvalidSubmission = errors.New("assessment: invalid submission")
	ErrUnavailable       = errors.New("assessment: unavailable")
	ErrClosed            = errors.New("assessment: service is closed")
)

type Descriptor struct {
	Available bool   `json:"available"`
	ID        string `json:"id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

type Submission struct {
	Stage      evaluator.AssessmentStage
	Answers    []evaluator.AssessmentAnswer
	FollowUpID string
	Answer     string
}

type Result struct {
	Turn    evaluator.AssessmentTurn
	Label   evaluator.AssessmentLabel
	History HistorySave
}

const HistoryStorageUnavailable = "storage_unavailable"

// HistorySave reports only whether the completed result reached durable
// storage. Follow-up turns do not expose the internal running record.
type HistorySave struct {
	Saved    bool   `json:"saved"`
	RecordID string `json:"record_id,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Provenance contains the server-owned values not already present in the
// validated evaluator input, question set, and model selection.
type Provenance struct {
	CanonicalRoot    string
	QuestionPrompt   history.PromptProvenance
	AssessmentPrompt history.PromptProvenance
}

type entryState uint8

const (
	stateAwaitingAnswers entryState = iota + 1
	stateEvaluatingInitial
	stateAwaitingFollowUp
	stateEvaluatingFollowUp
)

type entry struct {
	input         evaluator.Input
	questions     evaluator.QuestionSet
	selection     evaluator.ModelSelection
	initial       evaluator.AssessmentInput
	followUp      evaluator.FollowUpQuestion
	provenance    Provenance
	historyID     string
	state         entryState
	expiresAt     time.Time
	evidenceBytes int
}

// Service retains only validated evaluator values and enforces every lifecycle
// transition before an evaluator call begins.
type Service struct {
	mu            sync.Mutex
	entries       map[string]entry
	retainedBytes int
	evaluator     evaluator.AssessmentEvaluator
	history       *history.Store
	closed        bool
	now           func() time.Time
	newID         func() (string, error)
	lifetime      time.Duration
	maxEntries    int
	maxBytes      int
}

func New(assessmentEvaluator evaluator.AssessmentEvaluator, historyStore *history.Store) *Service {
	return &Service{
		entries:   make(map[string]entry),
		evaluator: assessmentEvaluator,
		history:   historyStore,
		now:       time.Now,
		newID: func() (string, error) {
			content := make([]byte, assessmentIdentifierBytes)
			if _, err := rand.Read(content); err != nil {
				return "", err
			}
			return "as1-" + base64.RawURLEncoding.EncodeToString(content), nil
		},
		lifetime:   assessmentLifetime,
		maxEntries: maxLiveAssessments,
		maxBytes:   maxRetainedEvidenceBytes,
	}
}

func (service *Service) Start(input evaluator.Input, questions evaluator.QuestionSet, selection evaluator.ModelSelection, provenance Provenance) (Descriptor, error) {
	if service == nil {
		return Descriptor{}, ErrClosed
	}
	service.mu.Lock()
	closed := service.closed
	service.mu.Unlock()
	if closed {
		return Descriptor{}, ErrClosed
	}
	if questions.SchemaVersion == evaluator.QuestionSetSchemaVersion &&
		questions.Disposition == evaluator.DispositionInsufficientEvidence &&
		questions.Questions != nil && len(questions.Questions) == 0 {
		return Descriptor{Available: false, Reason: "insufficient_evidence"}, nil
	}
	if service.evaluator == nil {
		return Descriptor{Available: false, Reason: "evaluator_unavailable"}, nil
	}
	if err := evaluator.ValidateModelSelection(selection); err != nil {
		return Descriptor{}, ErrInvalidStart
	}

	owned, err := ownStartContext(input, questions)
	if err != nil {
		return Descriptor{}, ErrInvalidStart
	}
	evidenceBytes := owned.EvaluatorInput.EvidenceBundle.ApproximateBytes
	if evidenceBytes <= 0 {
		return Descriptor{}, ErrInvalidStart
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	if service.closed {
		return Descriptor{}, ErrClosed
	}
	now := service.now().UTC()
	service.purgeExpiredLocked(now)
	if len(service.entries) >= service.maxEntries || service.retainedBytes+evidenceBytes > service.maxBytes {
		return Descriptor{Available: false, Reason: "capacity"}, nil
	}

	var id string
	for attempts := 0; attempts < 3; attempts++ {
		candidate, err := service.newID()
		if err != nil {
			return Descriptor{}, fmt.Errorf("assessment: generate ID: %w", err)
		}
		if !ValidID(candidate) {
			return Descriptor{}, errors.New("assessment: generated ID is invalid")
		}
		if _, exists := service.entries[candidate]; !exists {
			id = candidate
			break
		}
	}
	if id == "" {
		return Descriptor{}, errors.New("assessment: generate unique ID")
	}

	expiresAt := now.Add(service.lifetime)
	service.entries[id] = entry{
		input:         owned.EvaluatorInput,
		questions:     owned.QuestionSet,
		selection:     selection,
		provenance:    provenance,
		state:         stateAwaitingAnswers,
		expiresAt:     expiresAt,
		evidenceBytes: evidenceBytes,
	}
	service.retainedBytes += evidenceBytes
	return Descriptor{
		Available: true,
		ID:        id,
		ExpiresAt: expiresAt.Format(time.RFC3339Nano),
	}, nil
}

func (service *Service) Submit(ctx context.Context, id string, submission Submission) (Result, error) {
	if service == nil {
		return Result{}, ErrClosed
	}
	if ctx == nil {
		return Result{}, ErrInvalidSubmission
	}
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if !ValidID(id) {
		return Result{}, ErrUnavailable
	}

	service.mu.Lock()
	if service.closed {
		service.mu.Unlock()
		return Result{}, ErrClosed
	}
	service.purgeExpiredLocked(service.now().UTC())
	current, exists := service.entries[id]
	if !exists {
		service.mu.Unlock()
		return Result{}, ErrUnavailable
	}

	var assessmentInput evaluator.AssessmentInput
	var evaluatingState entryState
	switch submission.Stage {
	case evaluator.AssessmentStageInitialAnswers:
		if current.state != stateAwaitingAnswers || submission.FollowUpID != "" || submission.Answer != "" {
			service.mu.Unlock()
			return Result{}, ErrUnavailable
		}
		var err error
		assessmentInput, err = evaluator.NewInitialAssessmentInput(current.input, current.questions, submission.Answers)
		if err != nil {
			service.mu.Unlock()
			return Result{}, ErrInvalidSubmission
		}
		evaluatingState = stateEvaluatingInitial
	case evaluator.AssessmentStageFollowUpAnswer:
		if current.state != stateAwaitingFollowUp || len(submission.Answers) != 0 || submission.FollowUpID != "F1" {
			service.mu.Unlock()
			return Result{}, ErrUnavailable
		}
		var err error
		assessmentInput, err = evaluator.NewFollowUpAssessmentInput(current.initial, current.followUp, submission.Answer)
		if err != nil {
			service.mu.Unlock()
			return Result{}, ErrInvalidSubmission
		}
		evaluatingState = stateEvaluatingFollowUp
	default:
		service.mu.Unlock()
		return Result{}, ErrInvalidSubmission
	}
	current.state = evaluatingState
	if evaluatingState == stateEvaluatingInitial {
		current.input = evaluator.Input{}
		current.questions = evaluator.QuestionSet{}
	} else {
		current.initial = evaluator.AssessmentInput{}
		current.followUp = evaluator.FollowUpQuestion{}
	}
	service.entries[id] = current
	service.mu.Unlock()

	recordID := current.historyID
	if evaluatingState == stateEvaluatingInitial && service.history != nil {
		recordID = service.createHistory(ctx, current, assessmentInput)
		if recordID != "" {
			service.mu.Lock()
			retained, retainedExists := service.entries[id]
			if retainedExists && retained.state == evaluatingState && !service.closed {
				retained.historyID = recordID
				service.entries[id] = retained
			}
			service.mu.Unlock()
		}
	}

	turn, err := service.evaluator.EvaluateAssessment(ctx, assessmentInput, current.selection)
	if err == nil {
		turn, err = validateTurn(turn, assessmentInput)
	}
	if err != nil {
		service.failHistory(ctx, recordID, err)
		service.mu.Lock()
		service.removeIfStateLocked(id, evaluatingState)
		service.mu.Unlock()
		return Result{}, err
	}

	service.mu.Lock()
	defer service.mu.Unlock()
	current, exists = service.entries[id]
	if !exists || current.state != evaluatingState || service.closed {
		return Result{}, ErrUnavailable
	}
	if turn.Disposition == evaluator.AssessmentDispositionFollowUp {
		if recordID != "" && service.markHistoryFollowUp(ctx, recordID) != nil {
			recordID = ""
		}
		current.state = stateAwaitingFollowUp
		current.initial = assessmentInput
		current.followUp = *turn.FollowUp
		current.historyID = recordID
		service.entries[id] = current
		return Result{Turn: turn}, nil
	}

	label, err := evaluator.DeriveAssessmentLabel(turn)
	if err != nil {
		service.removeIfStateLocked(id, evaluatingState)
		return Result{}, err
	}
	historySave := service.completeHistory(ctx, recordID, assessmentInput.QuestionSet, turn, label)
	service.removeIfStateLocked(id, evaluatingState)
	return Result{Turn: turn, Label: label, History: historySave}, nil
}

func (service *Service) createHistory(ctx context.Context, current entry, input evaluator.AssessmentInput) string {
	recordID, err := service.history.Create(ctx, history.Start{
		CanonicalRoot:           current.provenance.CanonicalRoot,
		StartedAt:               service.historyNow(),
		BaseRevision:            input.EvaluatorInput.EvidenceBundle.BaseRevision,
		HeadRevision:            input.EvaluatorInput.EvidenceBundle.HeadRevision,
		EvidenceManifestSHA256:  input.EvaluatorInput.EvidenceBundle.ManifestSHA256,
		QuestionSchemaVersion:   input.QuestionSet.SchemaVersion,
		AssessmentSchemaVersion: evaluator.AssessmentTurnSchemaVersion,
		QuestionPrompt:          current.provenance.QuestionPrompt,
		AssessmentPrompt:        current.provenance.AssessmentPrompt,
		PiVersion:               current.selection.PiVersion,
		Provider:                current.selection.Provider,
		ModelID:                 current.selection.ModelID,
		ThinkingLevel:           current.selection.ThinkingLevel,
	})
	if err != nil {
		return ""
	}
	return recordID
}

func (service *Service) markHistoryFollowUp(parent context.Context, recordID string) error {
	ctx, cancel := historyWriteContext(parent)
	defer cancel()
	return service.history.MarkFollowUp(ctx, recordID)
}

func (service *Service) completeHistory(parent context.Context, recordID string, questions evaluator.QuestionSet, turn evaluator.AssessmentTurn, label evaluator.AssessmentLabel) HistorySave {
	if service.history == nil || recordID == "" {
		return HistorySave{Saved: false, Reason: HistoryStorageUnavailable}
	}
	outcomes := make([]history.Outcome, len(turn.Evaluations))
	for index := range turn.Evaluations {
		outcomes[index] = history.Outcome{
			QuestionID:   turn.Evaluations[index].QuestionID,
			QuestionKind: history.QuestionKind(questions.Questions[index].Kind),
			Verdict:      history.Verdict(turn.Evaluations[index].Verdict),
		}
	}
	ctx, cancel := historyWriteContext(parent)
	defer cancel()
	if err := service.history.Complete(ctx, recordID, history.Completion{
		FinishedAt: service.historyNow(),
		Label:      history.Label(label),
		Outcomes:   outcomes,
	}); err != nil {
		return HistorySave{Saved: false, Reason: HistoryStorageUnavailable}
	}
	return HistorySave{Saved: true, RecordID: recordID}
}

func (service *Service) failHistory(parent context.Context, recordID string, evaluationErr error) {
	if service.history == nil || recordID == "" {
		return
	}
	code := history.FailureEvaluatorFailed
	if errors.Is(evaluationErr, context.DeadlineExceeded) || errors.Is(parent.Err(), context.DeadlineExceeded) {
		code = history.FailureEvaluatorTimeout
	} else if evaluator.ContractErrorCodeOf(evaluationErr) == evaluator.ContractErrorInvalidOutput {
		code = history.FailureEvaluatorInvalidOutput
	}
	ctx, cancel := historyWriteContext(parent)
	defer cancel()
	_ = service.history.Fail(ctx, recordID, history.Failure{
		FinishedAt: service.historyNow(),
		Code:       code,
	})
}

func (service *Service) historyNow() time.Time {
	return service.now().UTC().Truncate(time.Millisecond)
}

func historyWriteContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(parent), historyWriteTimeout)
}

func (service *Service) Close() {
	if service == nil {
		return
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	service.closed = true
	clear(service.entries)
	service.retainedBytes = 0
}

func ValidID(value string) bool {
	if len(value) != len("as1-")+43 || value[:len("as1-")] != "as1-" {
		return false
	}
	decoded, err := base64.RawURLEncoding.DecodeString(value[len("as1-"):])
	return err == nil && len(decoded) == assessmentIdentifierBytes
}

func ownStartContext(input evaluator.Input, questions evaluator.QuestionSet) (evaluator.AssessmentInput, error) {
	return evaluator.NewInitialAssessmentInput(input, questions, []evaluator.AssessmentAnswer{
		{QuestionID: "Q1", Text: "pending assessment answer"},
		{QuestionID: "Q2", Text: "pending assessment answer"},
		{QuestionID: "Q3", Text: "pending assessment answer"},
	})
}

func validateTurn(turn evaluator.AssessmentTurn, input evaluator.AssessmentInput) (evaluator.AssessmentTurn, error) {
	content, err := json.Marshal(turn)
	if err != nil {
		return evaluator.AssessmentTurn{}, err
	}
	return evaluator.ParseAssessmentTurn(content, input)
}

func (service *Service) purgeExpiredLocked(now time.Time) {
	for id, current := range service.entries {
		if now.Before(current.expiresAt) {
			continue
		}
		service.retainedBytes -= current.evidenceBytes
		delete(service.entries, id)
	}
}

func (service *Service) removeIfStateLocked(id string, state entryState) {
	current, exists := service.entries[id]
	if !exists || current.state != state {
		return
	}
	service.retainedBytes -= current.evidenceBytes
	delete(service.entries, id)
}
