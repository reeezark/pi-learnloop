package daemon_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

type previewContinuation struct {
	Available bool   `json:"available"`
	ID        string `json:"id"`
	ExpiresAt string `json:"expires_at"`
	Reason    string `json:"reason"`
}

func TestQuestionSetRequiresInstanceAuthentication(t *testing.T) {
	_, descriptor := startDaemon(t)
	request, err := http.NewRequest(
		http.MethodPost,
		descriptor.BaseURL+"/v1/question-sets",
		bytes.NewReader(validQuestionRequest("pc1-"+strings.Repeat("A", 43))),
	)
	if err != nil {
		t.Fatalf("NewRequest(question set): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST unauthenticated /v1/question-sets: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized || response.Header.Get("WWW-Authenticate") != "PiLearnLoop" {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("unauthenticated response = (%d, %q), want (401, PiLearnLoop); body = %s", response.StatusCode, response.Header.Get("WWW-Authenticate"), content)
	}
	assertErrorCode(t, response.Body, "unauthorized")
}

func TestQuestionSetConsumesExactWorkingTreePreviewOnce(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository := newRepository(t)
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Answer() int { return 1 }\n")
	commitAll(t, repository, "base")
	base := revision(t, repository, "HEAD")
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Answer() int { return 2 }\n")

	continuation := requestPreviewContinuation(t, descriptor.BaseURL, token, repository, `{"kind":"working_tree","base":`+quoted(base)+`}`)
	if !continuation.Available || !strings.HasPrefix(continuation.ID, "pc1-") {
		t.Fatalf("continuation = %#v, want available pc1 ID", continuation)
	}
	if _, err := time.Parse(time.RFC3339, continuation.ExpiresAt); err != nil {
		t.Fatalf("expires_at = %q, want RFC3339: %v", continuation.ExpiresAt, err)
	}

	// If continuation re-ran working-tree analysis, this malformed replacement
	// would fail. A successful result therefore proves the retained value is used.
	writeRepositoryFile(t, repository, "sample.go", "package sample\nfunc Broken( {\n")
	response := postQuestionSet(t, descriptor.BaseURL, token, validQuestionRequest(continuation.ID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /v1/question-sets status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, content)
	}
	var result struct {
		ProtocolVersion int                   `json:"protocol_version"`
		QuestionSet     evaluator.QuestionSet `json:"question_set"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode question set: %v", err)
	}
	if result.ProtocolVersion != 1 || result.QuestionSet.Disposition != evaluator.DispositionQuestions || len(result.QuestionSet.Questions) != 3 {
		t.Fatalf("question-set response = %#v, want protocol 1 and three questions", result)
	}
	if got := result.QuestionSet.Questions[0].EvidenceReferences; len(got) != 1 || got[0] != "E001" {
		t.Fatalf("Q1 references = %#v, want E001", got)
	}

	second := postQuestionSet(t, descriptor.BaseURL, token, validQuestionRequest(continuation.ID))
	defer second.Body.Close()
	if second.StatusCode != http.StatusConflict {
		content, _ := io.ReadAll(second.Body)
		t.Fatalf("second continuation status = %d, want %d; body = %s", second.StatusCode, http.StatusConflict, content)
	}
	assertErrorCode(t, second.Body, "continuation_unavailable")
}

func TestProductionDaemonCompletesAssessmentThroughPiRPC(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	continuation := requestPreviewContinuation(t, descriptor.BaseURL, token, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(head)+`}`)

	questionResponse := postQuestionSet(t, descriptor.BaseURL, token, validQuestionRequest(continuation.ID))
	defer questionResponse.Body.Close()
	if questionResponse.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(questionResponse.Body)
		t.Fatalf("POST /v1/question-sets status = %d, want 200; body = %s", questionResponse.StatusCode, content)
	}
	var questionResult struct {
		Assessment struct {
			Available bool   `json:"available"`
			ID        string `json:"id"`
		} `json:"assessment"`
	}
	if err := json.NewDecoder(questionResponse.Body).Decode(&questionResult); err != nil {
		t.Fatalf("decode question response: %v", err)
	}
	if !questionResult.Assessment.Available || !strings.HasPrefix(questionResult.Assessment.ID, "as1-") {
		t.Fatalf("assessment = %#v, want available production descriptor", questionResult.Assessment)
	}

	body := []byte(`{"assessment_id":` + quoted(questionResult.Assessment.ID) + `,"stage":"initial_answers","answers":[{"question_id":"Q1","text":"first answer"},{"question_id":"Q2","text":"second answer"},{"question_id":"Q3","text":"third answer"}]}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/assessment-turns", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(assessment): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	assessmentResponse, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST /v1/assessment-turns: %v", err)
	}
	defer assessmentResponse.Body.Close()
	if assessmentResponse.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(assessmentResponse.Body)
		t.Fatalf("POST /v1/assessment-turns status = %d, want 200; body = %s", assessmentResponse.StatusCode, content)
	}
	var assessmentResult struct {
		Turn  evaluator.AssessmentTurn  `json:"assessment_turn"`
		Label evaluator.AssessmentLabel `json:"label"`
	}
	if err := json.NewDecoder(assessmentResponse.Body).Decode(&assessmentResult); err != nil {
		t.Fatalf("decode assessment response: %v", err)
	}
	if assessmentResult.Turn.Disposition != evaluator.AssessmentDispositionComplete || assessmentResult.Label != evaluator.AssessmentLabelPartial {
		t.Fatalf("assessment result = %#v, want complete partial", assessmentResult)
	}
}

func TestQuestionSetConcurrentConsumeStartsOneEvaluation(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	continuation := requestPreviewContinuation(t, descriptor.BaseURL, token, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(head)+`}`)

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/question-sets", bytes.NewReader(validQuestionRequest(continuation.ID)))
			if err != nil {
				statuses <- 0
				return
			}
			request.Header.Set("Content-Type", "application/json")
			request.Header.Set("Authorization", "PiLearnLoop "+token)
			response, err := localHTTPClient().Do(request)
			if err != nil {
				statuses <- 0
				return
			}
			_, _ = io.Copy(io.Discard, response.Body)
			response.Body.Close()
			statuses <- response.StatusCode
		}()
	}
	close(start)
	wait.Wait()
	close(statuses)
	successes := 0
	conflicts := 0
	for status := range statuses {
		switch status {
		case http.StatusOK:
			successes++
		case http.StatusConflict:
			conflicts++
		default:
			t.Fatalf("concurrent continuation status = %d, want 200 or 409", status)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent statuses = (%d success, %d conflict), want (1, 1)", successes, conflicts)
	}
}

func TestQuestionSetStrictRequestFailuresDoNotConsumeGrant(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	continuation := requestPreviewContinuation(t, descriptor.BaseURL, token, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(head)+`}`)

	tests := []struct {
		name       string
		body       []byte
		wantStatus int
		wantCode   string
	}{
		{name: "unknown field", body: []byte(strings.TrimSuffix(string(validQuestionRequest(continuation.ID)), "}") + `,"extra":true}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "duplicate field", body: []byte(`{"continuation_id":` + quoted(continuation.ID) + `,"continuation_id":` + quoted(continuation.ID) + `,"pi_version":"0.84.3","model":{"provider":"provider","id":"model","thinking_level":"off"}}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "case folded field", body: []byte(`{"continuation_id":` + quoted(continuation.ID) + `,"Pi_Version":"0.84.3","model":{"provider":"provider","id":"model","thinking_level":"off"}}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unsupported Pi", body: []byte(`{"continuation_id":` + quoted(continuation.ID) + `,"pi_version":"0.85.0","model":{"provider":"provider","id":"model","thinking_level":"off"}}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unsafe provider", body: []byte(`{"continuation_id":` + quoted(continuation.ID) + `,"pi_version":"0.84.3","model":{"provider":"-unsafe","id":"model","thinking_level":"off"}}`), wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "malformed continuation", body: validQuestionRequest("not-a-continuation"), wantStatus: http.StatusConflict, wantCode: "continuation_unavailable"},
		{name: "wrong instance continuation", body: validQuestionRequest("pc1-" + strings.Repeat("Z", 43)), wantStatus: http.StatusConflict, wantCode: "continuation_unavailable"},
		{name: "oversized body", body: []byte(strings.Repeat(" ", 4*1024+1)), wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := postQuestionSet(t, descriptor.BaseURL, token, test.body)
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				content, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, test.wantStatus, content)
			}
			assertErrorCode(t, response.Body, test.wantCode)
		})
	}

	response := postQuestionSet(t, descriptor.BaseURL, token, validQuestionRequest(continuation.ID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("valid request after invalid attempts status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, content)
	}
}

func TestPreviewWithoutUsableExcerptReportsUnavailableContinuation(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository := newRepository(t)
	writeRepositoryFile(t, repository, "sample.go", "package sample\n")
	commitAll(t, repository, "base")
	base := revision(t, repository, "HEAD")

	continuation := requestPreviewContinuation(t, descriptor.BaseURL, token, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(base)+`}`)
	if continuation.Available || continuation.Reason != "insufficient_evidence" || continuation.ID != "" || continuation.ExpiresAt != "" {
		t.Fatalf("continuation = %#v, want unavailable insufficient_evidence without an ID", continuation)
	}
}

func TestContinuationDoesNotSurviveDaemonRestart(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	first := startDaemonAt(t, stateDir)
	firstToken := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	continuation := requestPreviewContinuation(t, first.descriptor.BaseURL, firstToken, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(head)+`}`)
	first.stop()

	second := startDaemonAt(t, stateDir)
	secondToken := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	response := postQuestionSet(t, second.descriptor.BaseURL, secondToken, validQuestionRequest(continuation.ID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusConflict {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("old continuation after restart status = %d, want %d; body = %s", response.StatusCode, http.StatusConflict, content)
	}
	assertErrorCode(t, response.Body, "continuation_unavailable")
}

func requestPreviewContinuation(t *testing.T, baseURL, token, repository, selection string) previewContinuation {
	t.Helper()
	body := []byte(`{"repository":` + quoted(repository) + `,"selection":` + selection + `}`)
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(preview): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST /v1/evidence-previews: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("POST /v1/evidence-previews status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, content)
	}
	var result struct {
		Continuation previewContinuation `json:"continuation"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode continuation preview: %v", err)
	}
	return result.Continuation
}

func postQuestionSet(t *testing.T, baseURL, token string, body []byte) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, baseURL+"/v1/question-sets", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(question set): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST /v1/question-sets: %v", err)
	}
	return response
}

func validQuestionRequest(continuationID string) []byte {
	return []byte(`{"continuation_id":` + quoted(continuationID) + `,"pi_version":"0.84.3","model":{"provider":"provider","id":"model","thinking_level":"off"}}`)
}
