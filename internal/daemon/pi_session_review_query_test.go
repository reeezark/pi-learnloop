package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestPiSessionReviewQueryReturnsOnlyCompletedCandidatesInRepositoryOrder(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	repository := newHistoryQueryRepository(t)
	nested := filepath.Join(repository, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("MkdirAll(nested): %v", err)
	}
	otherRepository := newHistoryQueryRepository(t)

	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	if _, err := store.CreateWithPiSession(ctx, historyQueryStart(t, repository), "interrupted-session"); err != nil {
		t.Fatalf("CreateWithPiSession(interrupted): %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(before recovery): %v", err)
	}
	store, err = history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("history.Open(after recovery): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.CreateWithPiSession(ctx, historyQueryStart(t, repository), "running-session"); err != nil {
		t.Fatalf("CreateWithPiSession(running): %v", err)
	}
	failedStart := historyQueryStart(t, repository)
	failedID, err := store.CreateWithPiSession(ctx, failedStart, "failed-session")
	if err != nil {
		t.Fatalf("CreateWithPiSession(failed): %v", err)
	}
	if err := store.Fail(ctx, failedID, history.Failure{FinishedAt: failedStart.StartedAt.Add(time.Minute), Code: history.FailureEvaluatorFailed}); err != nil {
		t.Fatalf("Fail(): %v", err)
	}
	completePiSessionForRoute(t, store, repository, "complete-a")
	completePiSessionForRoute(t, store, repository, "complete-b")
	completePiSessionForRoute(t, store, otherRepository, "other-only")

	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{history: store})
	candidates := []string{"running-session", "complete-b", "failed-session", "interrupted-session", "complete-a", "other-only", "no-record"}
	body, err := json.Marshal(map[string]any{"repository": nested, "pi_session_ids": candidates})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	response := servePiSessionReviewRequest(handler, body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(response): %v", err)
	}
	want := map[string]any{
		"protocol_version":        float64(1),
		"reviewed_pi_session_ids": []any{"complete-b", "complete-a"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}

	genericBody, err := json.Marshal(map[string]any{"repository": repository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(generic history request): %v", err)
	}
	genericResponse := serveLearningHistoryRequest(handler, genericBody)
	if genericResponse.Code != http.StatusOK {
		t.Fatalf("generic history status = %d, want 200; body = %s", genericResponse.Code, genericResponse.Body.String())
	}
	for _, candidate := range candidates {
		if bytes.Contains(genericResponse.Body.Bytes(), []byte(candidate)) {
			t.Fatalf("generic history response contains Pi Session identity %q: %s", candidate, genericResponse.Body.String())
		}
	}
}

func TestPiSessionReviewQueryRequiresExactBoundedRequest(t *testing.T) {
	store, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := newHistoryQueryRepository(t)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{history: store})
	valid := `{"repository":` + historyQueryJSON(repository) + `,"pi_session_ids":["session-1"]}`
	privateInvalid := "private/session-sentinel"
	tests := []struct {
		name string
		body string
	}{
		{name: "missing candidates", body: `{"repository":` + historyQueryJSON(repository) + `}`},
		{name: "empty candidates", body: `{"repository":` + historyQueryJSON(repository) + `,"pi_session_ids":[]}`},
		{name: "too many candidates", body: `{"repository":` + historyQueryJSON(repository) + `,"pi_session_ids":["` + strings.Join(routeSessionIDs(21), `","`) + `"]}`},
		{name: "duplicate candidate", body: `{"repository":` + historyQueryJSON(repository) + `,"pi_session_ids":["same","same"]}`},
		{name: "invalid candidate", body: strings.Replace(valid, "session-1", privateInvalid, 1)},
		{name: "oversized candidate", body: strings.Replace(valid, "session-1", strings.Repeat("a", 129), 1)},
		{name: "relative repository", body: strings.Replace(valid, historyQueryJSON(repository), `"relative"`, 1)},
		{name: "unknown field", body: strings.TrimSuffix(valid, "}") + `,"extra":true}`},
		{name: "duplicate field", body: strings.Replace(valid, `"pi_session_ids":["session-1"]`, `"pi_session_ids":["session-1"],"pi_session_ids":["session-1"]`, 1)},
		{name: "case-folded field", body: strings.Replace(valid, `"pi_session_ids"`, `"Pi_Session_IDs"`, 1)},
		{name: "trailing JSON", body: valid + `{}`},
		{name: "malformed JSON", body: `{"repository":`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := servePiSessionReviewRequest(handler, []byte(test.body))
			if response.Code != http.StatusBadRequest || historyQueryErrorCode(t, response) != "invalid_request" {
				t.Fatalf("response = (%d, %s), want invalid_request", response.Code, response.Body.String())
			}
			if bytes.Contains(response.Body.Bytes(), []byte(privateInvalid)) {
				t.Fatalf("safe error echoed invalid Session identity: %s", response.Body.String())
			}
		})
	}

	response := servePiSessionReviewRequest(handler, []byte(valid))
	if response.Code != http.StatusOK {
		t.Fatalf("valid request status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	var payload map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(response): %v", err)
	}
	if !reflect.DeepEqual(payload["reviewed_pi_session_ids"], []any{}) {
		t.Fatalf("reviewed IDs = %#v, want non-nil empty list", payload["reviewed_pi_session_ids"])
	}
	twentyBody, err := json.Marshal(map[string]any{"repository": repository, "pi_session_ids": routeSessionIDs(20)})
	if err != nil {
		t.Fatalf("Marshal(20 candidates): %v", err)
	}
	response = servePiSessionReviewRequest(handler, twentyBody)
	if response.Code != http.StatusOK {
		t.Fatalf("20-candidate status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
}

func TestPiSessionReviewQueryExplainsUnavailableStorageAfterRepositoryVerification(t *testing.T) {
	repository := newHistoryQueryRepository(t)
	body, err := json.Marshal(map[string]any{"repository": repository, "pi_session_ids": []string{"session-1"}})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	response := servePiSessionReviewRequest(handler, body)
	if response.Code != http.StatusServiceUnavailable || historyQueryErrorCode(t, response) != "history_unavailable" {
		t.Fatalf("response = (%d, %s), want history_unavailable", response.Code, response.Body.String())
	}

	notRepositoryBody, err := json.Marshal(map[string]any{"repository": t.TempDir(), "pi_session_ids": []string{"session-1"}})
	if err != nil {
		t.Fatalf("Marshal(non-repository request): %v", err)
	}
	response = servePiSessionReviewRequest(handler, notRepositoryBody)
	if response.Code != http.StatusUnprocessableEntity || historyQueryErrorCode(t, response) != "invalid_repository" {
		t.Fatalf("non-repository response = (%d, %s), want invalid_repository", response.Code, response.Body.String())
	}
}

func TestPiSessionReviewQueryReturnsStableDeadlineError(t *testing.T) {
	repository := newHistoryQueryRepository(t)
	body, err := json.Marshal(map[string]any{"repository": repository, "pi_session_ids": []string{"session-deadline"}})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/pi-session-review-queries", bytes.NewReader(body))
	deadlineCtx, cancel := context.WithDeadline(request.Context(), time.Now().Add(-time.Second))
	defer cancel()
	request = request.WithContext(deadlineCtx)
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop token")
	response := httptest.NewRecorder()
	newHandler("instance", "127.0.0.1:43210", "token", serverServices{}).ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout || historyQueryErrorCode(t, response) != "deadline_exceeded" {
		t.Fatalf("response = (%d, %s), want deadline_exceeded", response.Code, response.Body.String())
	}
}

func TestPiSessionReviewQueryRequiresAuthenticatedStrictHTTP(t *testing.T) {
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	tests := []struct {
		name        string
		method      string
		body        []byte
		authorize   bool
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{name: "method", method: http.MethodGet, authorize: true, contentType: "application/json", wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed"},
		{name: "authentication", method: http.MethodPost, body: []byte(`{}`), contentType: "application/json", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "media type", method: http.MethodPost, body: []byte(`{}`), authorize: true, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "body limit", method: http.MethodPost, body: []byte(strings.Repeat("x", maxRequestBytes+1)), authorize: true, contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "JSON", method: http.MethodPost, body: []byte(`{"repository":`), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://127.0.0.1:43210/v1/pi-session-review-queries", bytes.NewReader(test.body))
			request.Host = "127.0.0.1:43210"
			request.RemoteAddr = "127.0.0.1:54321"
			if test.contentType != "" {
				request.Header.Set("Content-Type", test.contentType)
			}
			if test.authorize {
				request.Header.Set("Authorization", "PiLearnLoop token")
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus || historyQueryErrorCode(t, response) != test.wantCode {
				t.Fatalf("response = (%d, %s), want (%d, %s)", response.Code, response.Body.String(), test.wantStatus, test.wantCode)
			}
		})
	}
}

func completePiSessionForRoute(t *testing.T, store *history.Store, repository, piSessionID string) {
	t.Helper()
	start := historyQueryStart(t, repository)
	recordID, err := store.CreateWithPiSession(context.Background(), start, piSessionID)
	if err != nil {
		t.Fatalf("CreateWithPiSession(%q): %v", piSessionID, err)
	}
	if err := store.Complete(context.Background(), recordID, history.Completion{
		FinishedAt: start.StartedAt.Add(time.Minute),
		Label:      history.LabelUnderstood,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictDemonstrated},
		},
	}); err != nil {
		t.Fatalf("Complete(%q): %v", piSessionID, err)
	}
}

func servePiSessionReviewRequest(handler http.Handler, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/pi-session-review-queries", bytes.NewReader(body))
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func routeSessionIDs(count int) []string {
	result := make([]string, count)
	for index := range result {
		result[index] = "session-" + string(rune('a'+index))
	}
	return result
}
