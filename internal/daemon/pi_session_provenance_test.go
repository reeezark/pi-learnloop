package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/assessment"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/history"
)

type capturingQuestionEvaluator struct {
	input     evaluator.Input
	selection evaluator.ModelSelection
}

func (adapter *capturingQuestionEvaluator) Evaluate(ctx context.Context, input evaluator.Input, selection evaluator.ModelSelection) (evaluator.QuestionSet, error) {
	adapter.input = input
	adapter.selection = selection
	return (evaluator.DeterministicEvaluator{}).Evaluate(ctx, input, selection)
}

type capturingAssessmentEvaluator struct {
	input     evaluator.AssessmentInput
	selection evaluator.ModelSelection
}

func (adapter *capturingAssessmentEvaluator) EvaluateAssessment(ctx context.Context, input evaluator.AssessmentInput, selection evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
	adapter.input = input
	adapter.selection = selection
	return (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(ctx, input, selection)
}

func TestPiSessionProvenanceReachesHistoryWithoutCrossingModelOrGenericHTTPSeams(t *testing.T) {
	repository, base := newPiSessionPreviewRepository(t)
	store, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	piSessionID := "session-never-model-visible-123"
	continuations := newContinuationStore()
	questionEvaluator := &capturingQuestionEvaluator{}
	assessmentEvaluator := &capturingAssessmentEvaluator{}
	assessments := assessment.New(assessmentEvaluator, store)
	t.Cleanup(assessments.Close)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations:     continuations,
		questionEvaluator: questionEvaluator,
		assessments:       assessments,
		history:           store,
	})

	previewRequest, err := json.Marshal(map[string]any{
		"repository":    repository,
		"pi_session_id": piSessionID,
		"selection":     map[string]any{"kind": "working_tree", "base": base},
	})
	if err != nil {
		t.Fatalf("Marshal(preview request): %v", err)
	}
	previewResponse := servePreviewRoute(handler, "/v1/pi-session-evidence-previews", previewRequest, true, "application/json")
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200; body = %s", previewResponse.Code, previewResponse.Body.String())
	}
	if bytes.Contains(previewResponse.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("preview response contains Pi Session identity: %s", previewResponse.Body.String())
	}
	var previewPayload map[string]any
	if err := json.Unmarshal(previewResponse.Body.Bytes(), &previewPayload); err != nil {
		t.Fatalf("Unmarshal(preview response): %v", err)
	}
	continuationID := continuationIDFromPreview(t, previewPayload)

	questionResponse := serveQuestionRequest(handler, validDaemonQuestionRequest(continuationID))
	if questionResponse.Code != http.StatusOK {
		t.Fatalf("question status = %d, want 200; body = %s", questionResponse.Code, questionResponse.Body.String())
	}
	assertAbsentFromModelAndHTTPValues(t, piSessionID, questionResponse.Body.Bytes(), questionEvaluator.input, questionEvaluator.selection)
	if strings.Contains(prompts.EvaluatorQuestionGenerationV1(), piSessionID) || strings.Contains(prompts.EvaluatorAnswerAssessmentV1(), piSessionID) {
		t.Fatal("released system prompt contains Pi Session identity")
	}
	var questionResult struct {
		Assessment assessment.Descriptor `json:"assessment"`
	}
	if err := json.Unmarshal(questionResponse.Body.Bytes(), &questionResult); err != nil {
		t.Fatalf("Unmarshal(question response): %v", err)
	}
	if !questionResult.Assessment.Available {
		t.Fatalf("assessment descriptor = %#v, want available", questionResult.Assessment)
	}

	reviewRequest, err := json.Marshal(map[string]any{"repository": repository, "pi_session_ids": []string{piSessionID}})
	if err != nil {
		t.Fatalf("Marshal(review request): %v", err)
	}
	beforeAssessment := servePiSessionReviewRequest(handler, reviewRequest)
	if beforeAssessment.Code != http.StatusOK || !bytes.Contains(beforeAssessment.Body.Bytes(), []byte(`"reviewed_pi_session_ids":[]`)) {
		t.Fatalf("review query before assessment = (%d, %s), want empty", beforeAssessment.Code, beforeAssessment.Body.String())
	}

	assessmentResponse := serveAssessmentRequest(handler, validInitialAssessmentRequest(questionResult.Assessment.ID), true)
	if assessmentResponse.Code != http.StatusOK {
		t.Fatalf("assessment status = %d, want 200; body = %s", assessmentResponse.Code, assessmentResponse.Body.String())
	}
	assertAbsentFromModelAndHTTPValues(t, piSessionID, assessmentResponse.Body.Bytes(), assessmentEvaluator.input, assessmentEvaluator.selection)

	afterAssessment := servePiSessionReviewRequest(handler, reviewRequest)
	if afterAssessment.Code != http.StatusOK || !bytes.Contains(afterAssessment.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("review query after assessment = (%d, %s), want completed Session", afterAssessment.Code, afterAssessment.Body.String())
	}
	genericRequest, err := json.Marshal(map[string]any{"repository": repository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(generic request): %v", err)
	}
	genericResponse := serveLearningHistoryRequest(handler, genericRequest)
	if genericResponse.Code != http.StatusOK {
		t.Fatalf("generic history status = %d, want 200; body = %s", genericResponse.Code, genericResponse.Body.String())
	}
	if bytes.Contains(genericResponse.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("generic history response contains Pi Session identity: %s", genericResponse.Body.String())
	}
}

func assertAbsentFromModelAndHTTPValues(t *testing.T, piSessionID string, response []byte, values ...any) {
	t.Helper()
	if bytes.Contains(response, []byte(piSessionID)) {
		t.Fatalf("HTTP response contains Pi Session identity: %s", response)
	}
	serialized, err := json.Marshal(values)
	if err != nil {
		t.Fatalf("Marshal(model-visible values): %v", err)
	}
	if bytes.Contains(serialized, []byte(piSessionID)) {
		t.Fatalf("model-visible serialized values contain Pi Session identity: %s", serialized)
	}
}
