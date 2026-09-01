package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/assessment"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
)

type failingAssessmentEvaluator struct {
	err error
}

func (adapter failingAssessmentEvaluator) EvaluateAssessment(context.Context, evaluator.AssessmentInput, evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
	return evaluator.AssessmentTurn{}, adapter.err
}

type blockingAssessmentEvaluator struct {
	entered chan struct{}
	release chan struct{}
}

func (adapter blockingAssessmentEvaluator) EvaluateAssessment(ctx context.Context, input evaluator.AssessmentInput, selection evaluator.ModelSelection) (evaluator.AssessmentTurn, error) {
	close(adapter.entered)
	select {
	case <-adapter.release:
		return (evaluator.DeterministicAssessmentEvaluator{}).EvaluateAssessment(ctx, input, selection)
	case <-ctx.Done():
		return evaluator.AssessmentTurn{}, ctx.Err()
	}
}

func TestQuestionSetAssessmentDescriptor(t *testing.T) {
	t.Run("creates an assessment from the exact successful question context", func(t *testing.T) {
		handler, assessmentID := assessmentHandler(t, evaluator.DeterministicAssessmentEvaluator{})
		if !assessment.ValidID(assessmentID) {
			t.Fatalf("assessment ID = %q, want valid as1 ID", assessmentID)
		}
		response := serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
		if response.Code != http.StatusOK {
			t.Fatalf("POST /v1/assessment-turns status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
		}
		var result struct {
			ProtocolVersion int                       `json:"protocol_version"`
			Turn            evaluator.AssessmentTurn  `json:"assessment_turn"`
			Label           evaluator.AssessmentLabel `json:"label"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("Unmarshal(assessment response): %v", err)
		}
		if result.ProtocolVersion != 1 || result.Turn.Disposition != evaluator.AssessmentDispositionComplete || result.Label != evaluator.AssessmentLabelPartial {
			t.Fatalf("assessment response = %#v, want protocol 1 complete partial", result)
		}
	})

	t.Run("reports production assessment as unavailable without a fallback", func(t *testing.T) {
		continuations := newContinuationStore()
		continuation, err := continuations.retain(validAssessmentEvidence())
		if err != nil {
			t.Fatalf("retain(): %v", err)
		}
		assessments := assessment.New(nil)
		t.Cleanup(assessments.Close)
		handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
			continuations:     continuations,
			questionEvaluator: evaluator.DeterministicEvaluator{},
			assessments:       assessments,
		})
		response := serveQuestionRequest(handler, validDaemonQuestionRequest(continuation.ID))
		if response.Code != http.StatusOK {
			t.Fatalf("question status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
		}
		var result struct {
			Assessment assessment.Descriptor `json:"assessment"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
			t.Fatalf("Unmarshal(question response): %v", err)
		}
		if result.Assessment.Available || result.Assessment.Reason != "evaluator_unavailable" || result.Assessment.ID != "" {
			t.Fatalf("assessment descriptor = %#v, want evaluator_unavailable without ID", result.Assessment)
		}
	})
}

func TestAssessmentTurnFollowUpLifecycle(t *testing.T) {
	handler, assessmentID := assessmentHandler(t, evaluator.DeterministicAssessmentEvaluator{RequestFollowUp: true})
	first := serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
	if first.Code != http.StatusOK {
		t.Fatalf("initial status = %d, want %d; body = %s", first.Code, http.StatusOK, first.Body.String())
	}
	var firstResult struct {
		Turn  evaluator.AssessmentTurn   `json:"assessment_turn"`
		Label *evaluator.AssessmentLabel `json:"label"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &firstResult); err != nil {
		t.Fatalf("Unmarshal(initial assessment): %v", err)
	}
	if firstResult.Turn.Disposition != evaluator.AssessmentDispositionFollowUp || firstResult.Turn.FollowUp == nil || firstResult.Label != nil {
		t.Fatalf("initial assessment = %#v, want F1 without label", firstResult)
	}

	final := serveAssessmentRequest(handler, []byte(`{"assessment_id":"`+assessmentID+`","stage":"follow_up_answer","follow_up_id":"F1","answer":"The empty-name branch returns ErrEmpty."}`), true)
	if final.Code != http.StatusOK {
		t.Fatalf("follow-up status = %d, want %d; body = %s", final.Code, http.StatusOK, final.Body.String())
	}
	var finalResult struct {
		Turn  evaluator.AssessmentTurn  `json:"assessment_turn"`
		Label evaluator.AssessmentLabel `json:"label"`
	}
	if err := json.Unmarshal(final.Body.Bytes(), &finalResult); err != nil {
		t.Fatalf("Unmarshal(final assessment): %v", err)
	}
	if finalResult.Turn.Disposition != evaluator.AssessmentDispositionComplete || finalResult.Label != evaluator.AssessmentLabelPartial {
		t.Fatalf("final assessment = %#v, want complete partial", finalResult)
	}

	replay := serveAssessmentRequest(handler, []byte(`{"assessment_id":"`+assessmentID+`","stage":"follow_up_answer","follow_up_id":"F1","answer":"again"}`), true)
	if replay.Code != http.StatusConflict {
		t.Fatalf("replay status = %d, want %d; body = %s", replay.Code, http.StatusConflict, replay.Body.String())
	}
	assertRecorderErrorCode(t, replay, "assessment_unavailable")
}

func TestAssessmentTurnStrictFailuresDoNotConsumeState(t *testing.T) {
	handler, assessmentID := assessmentHandler(t, evaluator.DeterministicAssessmentEvaluator{})
	tests := []struct {
		name       string
		body       []byte
		authorized bool
		wantStatus int
		wantCode   string
	}{
		{name: "unauthorized", body: validInitialAssessmentRequest(assessmentID), wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "unknown field", body: []byte(strings.TrimSuffix(string(validInitialAssessmentRequest(assessmentID)), "}") + `,"source":"secret"}`), authorized: true, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "duplicate field", body: []byte(`{"assessment_id":"` + assessmentID + `","assessment_id":"` + assessmentID + `","stage":"initial_answers","answers":[]}`), authorized: true, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unknown answer field", body: []byte(strings.Replace(string(validInitialAssessmentRequest(assessmentID)), `"question_id":"Q1"`, `"question_id":"Q1","score":100`, 1)), authorized: true, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "empty answer", body: []byte(strings.Replace(string(validInitialAssessmentRequest(assessmentID)), `"text":"first answer"`, `"text":""`, 1)), authorized: true, wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "wrong lifecycle stage", body: []byte(`{"assessment_id":"` + assessmentID + `","stage":"follow_up_answer","follow_up_id":"F1","answer":"answer"}`), authorized: true, wantStatus: http.StatusConflict, wantCode: "assessment_unavailable"},
		{name: "malformed ID", body: validInitialAssessmentRequest("not-an-assessment"), authorized: true, wantStatus: http.StatusConflict, wantCode: "assessment_unavailable"},
		{name: "oversized body", body: []byte(strings.Repeat(" ", maxAssessmentRequestBytes+1)), authorized: true, wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveAssessmentRequest(handler, test.body, test.authorized)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, test.wantStatus, response.Body.String())
			}
			assertRecorderErrorCode(t, response, test.wantCode)
		})
	}

	valid := serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
	if valid.Code != http.StatusOK {
		t.Fatalf("valid request after rejected attempts status = %d, want %d; body = %s", valid.Code, http.StatusOK, valid.Body.String())
	}
}

func TestAssessmentTurnConcurrentSubmissionStartsOneEvaluation(t *testing.T) {
	blocking := blockingAssessmentEvaluator{entered: make(chan struct{}), release: make(chan struct{})}
	handler, assessmentID := assessmentHandler(t, blocking)
	start := make(chan struct{})
	responses := make(chan *httptest.ResponseRecorder, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			responses <- serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
		}()
	}
	close(start)
	<-blocking.entered
	close(blocking.release)
	wait.Wait()
	close(responses)
	successes := 0
	conflicts := 0
	for response := range responses {
		switch response.Code {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("concurrent status = %d, want 200 or 409; body = %s", response.Code, response.Body.String())
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent responses = (%d success, %d conflict), want (1, 1)", successes, conflicts)
	}
}

func TestAssessmentTurnEvaluatorFailureInvalidatesState(t *testing.T) {
	handler, assessmentID := assessmentHandler(t, failingAssessmentEvaluator{err: errors.New("synthetic failure")})
	first := serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
	if first.Code != http.StatusBadGateway {
		t.Fatalf("first status = %d, want %d; body = %s", first.Code, http.StatusBadGateway, first.Body.String())
	}
	assertRecorderErrorCode(t, first, "evaluator_failed")
	second := serveAssessmentRequest(handler, validInitialAssessmentRequest(assessmentID), true)
	if second.Code != http.StatusConflict {
		t.Fatalf("second status = %d, want %d; body = %s", second.Code, http.StatusConflict, second.Body.String())
	}
	assertRecorderErrorCode(t, second, "assessment_unavailable")
}

func assessmentHandler(t *testing.T, assessmentEvaluator evaluator.AssessmentEvaluator) (http.Handler, string) {
	t.Helper()
	continuations := newContinuationStore()
	continuation, err := continuations.retain(validAssessmentEvidence())
	if err != nil {
		t.Fatalf("retain(): %v", err)
	}
	assessments := assessment.New(assessmentEvaluator)
	t.Cleanup(assessments.Close)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations:     continuations,
		questionEvaluator: evaluator.DeterministicEvaluator{},
		assessments:       assessments,
	})
	questionResponse := serveQuestionRequest(handler, validDaemonQuestionRequest(continuation.ID))
	if questionResponse.Code != http.StatusOK {
		t.Fatalf("question status = %d, want %d; body = %s", questionResponse.Code, http.StatusOK, questionResponse.Body.String())
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
	return handler, questionResult.Assessment.ID
}

func serveAssessmentRequest(handler http.Handler, body []byte, authorized bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/assessment-turns", bytes.NewReader(body))
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	if authorized {
		request.Header.Set("Authorization", "PiLearnLoop token")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func validDaemonQuestionRequest(continuationID string) []byte {
	return []byte(`{"continuation_id":"` + continuationID + `","pi_version":"0.84.3","model":{"provider":"provider","id":"model","thinking_level":"off"}}`)
}

func validInitialAssessmentRequest(assessmentID string) []byte {
	return []byte(`{"assessment_id":"` + assessmentID + `","stage":"initial_answers","answers":[{"question_id":"Q1","text":"first answer"},{"question_id":"Q2","text":"second answer"},{"question_id":"Q3","text":"third answer"}]}`)
}

func validAssessmentEvidence() evidence.Result {
	return evidence.Result{
		RepositoryRoot: "/private/synthetic-repository",
		BaseRevision:   strings.Repeat("a", 40),
		HeadRevision:   evidence.WorkingTreeRevision,
		AppliedLimits: evidence.Limits{
			MaxFiles:        5,
			MaxDeclarations: 10,
			MaxExcerptBytes: 4096,
		},
		Files: []evidence.File{{
			Path:         "internal/validate.go",
			Status:       evidence.FileModified,
			ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
			Declarations: []evidence.Declaration{{
				Kind:         evidence.DeclarationFunction,
				Name:         "Validate",
				Identity:     "Validate",
				StartLine:    3,
				EndLine:      3,
				ChangedLines: []evidence.LineRange{{Start: 3, End: 3}},
				Excerpt:      "func Validate() error { return nil }",
			}},
		}},
	}
}

func assertRecorderErrorCode(t *testing.T, response *httptest.ResponseRecorder, want string) {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(error response): %v", err)
	}
	if payload.Error.Code != want {
		t.Fatalf("error code = %q, want %q; body = %s", payload.Error.Code, want, response.Body.String())
	}
}
