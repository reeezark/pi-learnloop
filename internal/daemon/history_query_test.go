package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestLearningHistoryQueryReturnsOnlyCanonicalRepositoryRecords(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	repository := newHistoryQueryRepository(t)
	nested := filepath.Join(repository, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("mkdir(%q): %v", nested, err)
	}
	otherRepository := newHistoryQueryRepository(t)

	start := historyQueryStart(t, repository)
	recordID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create(repository): %v", err)
	}
	finishedAt := start.StartedAt.Add(time.Minute)
	if err := store.Complete(ctx, recordID, history.Completion{
		FinishedAt: finishedAt,
		Label:      history.LabelUnderstood,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictDemonstrated},
		},
	}); err != nil {
		t.Fatalf("Complete(repository): %v", err)
	}
	otherID, err := store.Create(ctx, historyQueryStart(t, otherRepository))
	if err != nil {
		t.Fatalf("Create(other repository): %v", err)
	}

	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{history: store})
	body, err := json.Marshal(map[string]any{"repository": nested, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	response := serveLearningHistoryRequest(handler, body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte(repository)) || bytes.Contains(response.Body.Bytes(), []byte(otherRepository)) || bytes.Contains(response.Body.Bytes(), []byte(otherID)) {
		t.Fatalf("response leaks a canonical root or another repository record: %s", response.Body.String())
	}

	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(response): %v", err)
	}
	want := map[string]any{
		"protocol_version": float64(1),
		"records": []any{map[string]any{
			"record_id":                 recordID,
			"started_at":                start.StartedAt.Format(time.RFC3339Nano),
			"finished_at":               finishedAt.Format(time.RFC3339Nano),
			"status":                    "complete",
			"failure_code":              nil,
			"base_revision":             start.BaseRevision,
			"head_revision":             start.HeadRevision,
			"evidence_manifest_sha256":  start.EvidenceManifestSHA256,
			"question_schema_version":   float64(start.QuestionSchemaVersion),
			"assessment_schema_version": float64(start.AssessmentSchemaVersion),
			"question_prompt":           map[string]any{"id": start.QuestionPrompt.ID, "version": start.QuestionPrompt.Version, "sha256": start.QuestionPrompt.SHA256},
			"assessment_prompt":         map[string]any{"id": start.AssessmentPrompt.ID, "version": start.AssessmentPrompt.Version, "sha256": start.AssessmentPrompt.SHA256},
			"pi_version":                start.PiVersion,
			"provider":                  start.Provider,
			"model_id":                  start.ModelID,
			"thinking_level":            start.ThinkingLevel,
			"follow_up_used":            false,
			"label":                     "understood",
			"outcomes": []any{
				map[string]any{"question_id": "Q1", "question_kind": "code_specific", "verdict": "demonstrated"},
				map[string]any{"question_id": "Q2", "question_kind": "code_specific", "verdict": "demonstrated"},
				map[string]any{"question_id": "Q3", "question_kind": "go_backend", "verdict": "demonstrated"},
			},
		}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestLearningHistoryQueryRejectsNonRepositoryWhenStorageIsUnavailable(t *testing.T) {
	notRepository := t.TempDir()
	body, err := json.Marshal(map[string]any{"repository": notRepository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	response := serveLearningHistoryRequest(handler, body)
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnprocessableEntity, response.Body.String())
	}
	if got := historyQueryErrorCode(t, response); got != "invalid_repository" {
		t.Fatalf("error code = %q, want invalid_repository", got)
	}
}

func TestLearningHistoryQueryRequiresAnExactBoundedRequestAndReturnsAnEmptyList(t *testing.T) {
	store, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	repository := newHistoryQueryRepository(t)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{history: store})

	tests := []struct {
		name string
		body string
	}{
		{name: "missing limit", body: `{"repository":` + historyQueryJSON(repository) + `}`},
		{name: "zero limit", body: `{"repository":` + historyQueryJSON(repository) + `,"limit":0}`},
		{name: "over maximum", body: `{"repository":` + historyQueryJSON(repository) + `,"limit":51}`},
		{name: "relative repository", body: `{"repository":"relative","limit":20}`},
		{name: "unknown field", body: `{"repository":` + historyQueryJSON(repository) + `,"limit":20,"extra":true}`},
		{name: "duplicate field", body: `{"repository":` + historyQueryJSON(repository) + `,"limit":20,"limit":20}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := serveLearningHistoryRequest(handler, []byte(test.body))
			if response.Code != http.StatusBadRequest || historyQueryErrorCode(t, response) != "invalid_request" {
				t.Fatalf("response = (%d, %s), want invalid_request", response.Code, response.Body.String())
			}
		})
	}

	body, err := json.Marshal(map[string]any{"repository": repository, "limit": 50})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	response := serveLearningHistoryRequest(handler, body)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &got); err != nil {
		t.Fatalf("Unmarshal(response): %v", err)
	}
	want := map[string]any{"protocol_version": float64(1), "records": []any{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("response = %#v, want %#v", got, want)
	}
}

func TestLearningHistoryQueryExplainsUnavailableStorage(t *testing.T) {
	repository := newHistoryQueryRepository(t)
	body, err := json.Marshal(map[string]any{"repository": repository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	response := serveLearningHistoryRequest(handler, body)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusServiceUnavailable, response.Body.String())
	}
	if got := historyQueryErrorCode(t, response); got != "history_unavailable" {
		t.Fatalf("error code = %q, want history_unavailable", got)
	}
}

func TestLearningHistoryQueryRequiresAuthenticatedStrictHTTP(t *testing.T) {
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
		{name: "body limit", method: http.MethodPost, body: []byte(strings.Repeat("x", maxHistoryQueryRequestBytes+1)), authorize: true, contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "JSON", method: http.MethodPost, body: []byte(`{"repository":`), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(test.method, "http://127.0.0.1:43210/v1/learning-history-queries", bytes.NewReader(test.body))
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

func newHistoryQueryRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	command := exec.Command("git", "init", "-q")
	command.Dir = repository
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
	canonical, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repository, err)
	}
	return canonical
}

func historyQueryStart(t *testing.T, canonicalRoot string) history.Start {
	t.Helper()
	return history.Start{
		CanonicalRoot:           canonicalRoot,
		StartedAt:               time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		BaseRevision:            strings.Repeat("a", 40),
		HeadRevision:            strings.Repeat("c", 40),
		EvidenceManifestSHA256:  strings.Repeat("b", 64),
		QuestionSchemaVersion:   1,
		AssessmentSchemaVersion: 1,
		QuestionPrompt:          history.PromptProvenance{ID: "question-prompt", Version: "1.0.0", SHA256: strings.Repeat("d", 64)},
		AssessmentPrompt:        history.PromptProvenance{ID: "assessment-prompt", Version: "1.0.0", SHA256: strings.Repeat("e", 64)},
		PiVersion:               "0.84.3",
		Provider:                "provider",
		ModelID:                 "model",
		ThinkingLevel:           "off",
	}
}

func serveLearningHistoryRequest(handler http.Handler, body []byte) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/learning-history-queries", bytes.NewReader(body))
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop token")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func historyQueryErrorCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var payload struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil {
		t.Fatalf("Unmarshal(error): %v", err)
	}
	return payload.Error.Code
}

func historyQueryJSON(value string) string {
	content, _ := json.Marshal(value)
	return string(content)
}
