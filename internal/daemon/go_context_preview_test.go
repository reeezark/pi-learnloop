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

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/assessment"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestGoContextPreviewRoutesAreAdditiveStrictAndRetainV2(t *testing.T) {
	repository, base := newGoContextPreviewRepository(t)
	store := newContinuationStore()
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations: store, questionEvaluator: evaluator.DeterministicEvaluator{},
	})
	selection := map[string]any{"kind": "working_tree", "base": base}
	genericBody, _ := json.Marshal(map[string]any{"repository": repository, "selection": selection})
	sessionID := "session-v2-preview-isolation"
	sessionBody, _ := json.Marshal(map[string]any{"repository": repository, "pi_session_id": sessionID, "selection": selection})

	generic := servePreviewRoute(handler, "/v1/go-context-evidence-previews", genericBody, true, "application/json")
	session := servePreviewRoute(handler, "/v1/pi-session-go-context-evidence-previews", sessionBody, true, "application/json")
	if generic.Code != http.StatusOK || session.Code != http.StatusOK {
		t.Fatalf("v2 preview statuses = (%d, %d), want 200; bodies=(%s, %s)", generic.Code, session.Code, generic.Body.String(), session.Body.String())
	}
	if bytes.Contains(generic.Body.Bytes(), []byte(sessionID)) || bytes.Contains(session.Body.Bytes(), []byte(sessionID)) {
		t.Fatalf("Pi Session ID crossed a preview response: generic=%s session=%s", generic.Body.String(), session.Body.String())
	}
	var genericPayload, sessionPayload map[string]any
	if err := json.Unmarshal(generic.Body.Bytes(), &genericPayload); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(session.Body.Bytes(), &sessionPayload); err != nil {
		t.Fatal(err)
	}
	genericID := continuationIDFromPreview(t, genericPayload)
	sessionIDValue := continuationIDFromPreview(t, sessionPayload)
	delete(genericPayload, "continuation")
	delete(sessionPayload, "continuation")
	if !reflect.DeepEqual(genericPayload, sessionPayload) {
		t.Fatalf("Session-bound v2 preview differs from generic v2 preview")
	}
	preview, ok := genericPayload["preview"].(map[string]any)
	contextValue, contextOK := preview["go_context"].(map[string]any)
	if !ok || !contextOK || contextValue["status"] == "" || contextValue["items"] == nil || contextValue["relations"] == nil || contextValue["applied_limits"] == nil {
		t.Fatalf("v2 preview = %#v, want explicit Go context, limits, items, and relations", preview)
	}
	retained, ok := store.consume(genericID)
	if !ok || retained.contract != evidenceContractV2 || retained.piSessionID != "" || retained.result.GoContext == nil {
		t.Fatalf("generic retained value = (%#v, %t), want isolated v2 context", retained, ok)
	}
	retained, ok = store.consume(sessionIDValue)
	if !ok || retained.contract != evidenceContractV2 || retained.piSessionID != sessionID || retained.result.GoContext == nil {
		t.Fatalf("Session retained value = (%#v, %t), want separate v2 provenance", retained, ok)
	}

	legacy := servePreviewRoute(handler, "/v1/evidence-previews", genericBody, true, "application/json")
	if legacy.Code != http.StatusOK || bytes.Contains(legacy.Body.Bytes(), []byte("go_context")) {
		t.Fatalf("legacy route changed or exposed context: status=%d body=%s", legacy.Code, legacy.Body.String())
	}

	unknown := append(genericBody[:len(genericBody)-1], []byte(`,"mode":"go"}`)...)
	response := servePreviewRoute(handler, "/v1/go-context-evidence-previews", unknown, true, "application/json")
	if response.Code != http.StatusBadRequest || historyQueryErrorCode(t, response) != "invalid_request" {
		t.Fatalf("unknown field response = (%d, %s), want invalid_request", response.Code, response.Body.String())
	}
	invalidSession := bytes.Replace(sessionBody, []byte(sessionID), []byte("private/session/path"), 1)
	response = servePreviewRoute(handler, "/v1/pi-session-go-context-evidence-previews", invalidSession, true, "application/json")
	if response.Code != http.StatusBadRequest || bytes.Contains(response.Body.Bytes(), []byte("private/session/path")) {
		t.Fatalf("invalid Session response = (%d, %s), want safe invalid_request", response.Code, response.Body.String())
	}
}

func TestGoContextPreviewRoutesRequireStrictAuthenticatedHTTP(t *testing.T) {
	generic := `{"repository":"/tmp/repository","selection":{"kind":"working_tree","base":"HEAD"}}`
	session := `{"repository":"/tmp/repository","pi_session_id":"safe-session","selection":{"kind":"working_tree","base":"HEAD"}}`
	tests := []struct {
		name        string
		path        string
		method      string
		body        string
		authorized  bool
		contentType string
		host        string
		remote      string
		origin      string
		wantStatus  int
		wantCode    string
	}{
		{name: "generic method", path: "/v1/go-context-evidence-previews", method: http.MethodGet, body: generic, authorized: true, contentType: "application/json", wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed"},
		{name: "generic authentication", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: generic, contentType: "application/json", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "generic media type", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: generic, authorized: true, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "generic body limit", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: strings.Repeat("x", maxRequestBytes+1), authorized: true, contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "generic unknown field", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: strings.TrimSuffix(generic, "}") + `,"extra":true}`, authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "generic duplicate field", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: strings.Replace(generic, `"repository":`, `"repository":"/private/injected","repository":`, 1), authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "generic case-folded field", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: strings.Replace(generic, `"repository"`, `"Repository"`, 1), authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "generic duplicate nested field", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: strings.Replace(generic, `"kind":"working_tree"`, `"kind":"working_tree","kind":"working_tree"`, 1), authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "generic trailing JSON", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: generic + `{}`, authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "Session unknown field", path: "/v1/pi-session-go-context-evidence-previews", method: http.MethodPost, body: strings.TrimSuffix(session, "}") + `,"extra":true}`, authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "Session duplicate identity", path: "/v1/pi-session-go-context-evidence-previews", method: http.MethodPost, body: strings.Replace(session, `"pi_session_id":"safe-session"`, `"pi_session_id":"safe-session","pi_session_id":"private-injected"`, 1), authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "Session case-folded identity", path: "/v1/pi-session-go-context-evidence-previews", method: http.MethodPost, body: strings.Replace(session, `"pi_session_id"`, `"Pi_Session_ID"`, 1), authorized: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "forbidden origin", path: "/v1/go-context-evidence-previews", method: http.MethodPost, body: generic, authorized: true, contentType: "application/json", origin: "https://example.invalid", wantStatus: http.StatusForbidden, wantCode: "forbidden"},
	}
	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			host := test.host
			if host == "" {
				host = "127.0.0.1:43210"
			}
			remote := test.remote
			if remote == "" {
				remote = "127.0.0.1:54321"
			}
			response := servePreviewRequest(
				newHandler("instance", "127.0.0.1:43210", "token", serverServices{}),
				test.method, []byte(test.body), test.authorized, test.contentType,
				host, remote, test.origin, test.path,
			)
			if response.Code != test.wantStatus || historyQueryErrorCode(t, response) != test.wantCode {
				t.Fatalf("response = (%d, %s), want (%d, %s)", response.Code, response.Body.String(), test.wantStatus, test.wantCode)
			}
			if bytes.Contains(response.Body.Bytes(), []byte("private-injected")) {
				t.Fatalf("safe error echoed injected Session identity: %s", response.Body.String())
			}
		})
	}
}

func TestV2ContinuationCarriesExactEvidenceThroughAssessmentAndSourceFreeHistory(t *testing.T) {
	repository, base := newGoContextPreviewRepository(t)
	historyStore, err := history.Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	t.Cleanup(func() { _ = historyStore.Close() })
	continuations := newContinuationStore()
	questionEvaluator := &capturingQuestionEvaluator{}
	assessmentEvaluator := &capturingAssessmentEvaluator{}
	assessments := assessment.New(assessmentEvaluator, historyStore)
	t.Cleanup(assessments.Close)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations: continuations, questionEvaluator: questionEvaluator,
		assessments: assessments, history: historyStore,
	})
	piSessionID := "session-v2-never-model-visible"
	body, _ := json.Marshal(map[string]any{
		"repository": repository, "pi_session_id": piSessionID,
		"selection": map[string]any{"kind": "working_tree", "base": base},
	})
	previewResponse := servePreviewRoute(handler, "/v1/pi-session-go-context-evidence-previews", body, true, "application/json")
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview status = %d; body=%s", previewResponse.Code, previewResponse.Body.String())
	}
	var previewPayload map[string]any
	_ = json.Unmarshal(previewResponse.Body.Bytes(), &previewPayload)
	continuationID := continuationIDFromPreview(t, previewPayload)

	// A repository reread would now fail. Successful question generation proves
	// the daemon used the exact retained enriched preview.
	if err := os.WriteFile(filepath.Join(repository, "changed.go"), []byte("package broken\nfunc Broken( {\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	questionResponse := serveQuestionRequest(handler, validDaemonQuestionRequest(continuationID))
	if questionResponse.Code != http.StatusOK {
		t.Fatalf("question status = %d; body=%s", questionResponse.Code, questionResponse.Body.String())
	}
	if questionEvaluator.input.SchemaVersion != evaluator.InputSchemaVersionV2 || questionEvaluator.input.EvidenceBundle.GoContext == nil {
		t.Fatalf("question evaluator input = %#v, want evaluator-input@2", questionEvaluator.input)
	}
	previewContext := previewPayload["preview"].(map[string]any)["go_context"]
	encodedContext, err := json.Marshal(questionEvaluator.input.EvidenceBundle.GoContext)
	if err != nil {
		t.Fatal(err)
	}
	var evaluatorContext any
	if err := json.Unmarshal(encodedContext, &evaluatorContext); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(previewContext, evaluatorContext) {
		t.Fatalf("model-visible Go context differs from preview:\npreview=%#v\nevaluator=%#v", previewContext, evaluatorContext)
	}
	assertV2SessionIsolation(t, piSessionID, questionResponse.Body.Bytes(), questionEvaluator.input)
	var questionResult struct {
		Assessment struct {
			Available bool   `json:"available"`
			ID        string `json:"id"`
		} `json:"assessment"`
	}
	if err := json.Unmarshal(questionResponse.Body.Bytes(), &questionResult); err != nil || !questionResult.Assessment.Available {
		t.Fatalf("question response = (%#v, %v), want assessment", questionResult, err)
	}
	assessmentResponse := serveAssessmentRequest(handler, validInitialAssessmentRequest(questionResult.Assessment.ID), true)
	if assessmentResponse.Code != http.StatusOK {
		t.Fatalf("assessment status = %d; body=%s", assessmentResponse.Code, assessmentResponse.Body.String())
	}
	if assessmentEvaluator.input.SchemaVersion != evaluator.AssessmentInputSchemaVersionV2 || assessmentEvaluator.input.EvaluatorInput.SchemaVersion != evaluator.InputSchemaVersionV2 {
		t.Fatalf("assessment input versions = (%d, %d), want v2", assessmentEvaluator.input.SchemaVersion, assessmentEvaluator.input.EvaluatorInput.SchemaVersion)
	}
	assertV2SessionIsolation(t, piSessionID, assessmentResponse.Body.Bytes(), assessmentEvaluator.input)
	records, err := historyStore.List(context.Background(), repository, 10)
	if err != nil || len(records) != 1 {
		t.Fatalf("history.List() = (%#v, %v), want one source-free record", records, err)
	}
	if records[0].Start.QuestionPrompt.Version != "2.0.0" || records[0].Start.AssessmentPrompt.Version != "2.0.0" {
		t.Fatalf("history prompt provenance = (%#v, %#v), want v2", records[0].Start.QuestionPrompt, records[0].Start.AssessmentPrompt)
	}
	encodedHistory, _ := json.Marshal(records)
	if bytes.Contains(encodedHistory, []byte(piSessionID)) || bytes.Contains(encodedHistory, []byte("func Build")) || bytes.Contains(encodedHistory, []byte("example.com/context-preview")) {
		t.Fatalf("generic history contains prohibited provenance or evidence: %s", encodedHistory)
	}
	reviewed, err := historyStore.ReviewedPiSessionIDs(context.Background(), repository, []string{piSessionID})
	if err != nil || !reflect.DeepEqual(reviewed, []string{piSessionID}) {
		t.Fatalf("ReviewedPiSessionIDs() = (%v, %v), want completion-only Session provenance", reviewed, err)
	}
	if strings.Contains(prompts.EvaluatorQuestionGenerationV2(), piSessionID) || strings.Contains(prompts.EvaluatorAnswerAssessmentV2(), piSessionID) {
		t.Fatal("v2 prompt contains Pi Session ID")
	}
}

func TestPiSessionGoContextPreviewCancellationUsesSafeError(t *testing.T) {
	repository, base := newGoContextPreviewRepository(t)
	piSessionID := "session-v2-cancel-isolation"
	body, err := json.Marshal(map[string]any{
		"repository": repository, "pi_session_id": piSessionID,
		"selection": map[string]any{"kind": "working_tree", "base": base},
	})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/pi-session-go-context-evidence-previews", bytes.NewReader(body))
	deadlineCtx, cancel := context.WithDeadline(request.Context(), time.Now().Add(-time.Second))
	defer cancel()
	request = request.WithContext(deadlineCtx)
	request.Host = "127.0.0.1:43210"
	request.RemoteAddr = "127.0.0.1:54321"
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop token")
	response := httptest.NewRecorder()
	newHandler("instance", "127.0.0.1:43210", "token", serverServices{}).ServeHTTP(response, request)
	if response.Code != http.StatusGatewayTimeout || historyQueryErrorCode(t, response) != "deadline_exceeded" || bytes.Contains(response.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("canceled response = (%d, %s), want safe deadline_exceeded", response.Code, response.Body.String())
	}
}

func TestExpiredPiSessionGoContextContinuationUsesSafeError(t *testing.T) {
	store := testContinuationStore()
	now := time.Date(2026, 9, 3, 12, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	result := continuationTestResult("changed evidence")
	result.GoContext = &evidence.GoContext{
		Items: []evidence.ContextItem{{ID: "C001", Content: "context evidence"}},
	}
	piSessionID := "session-v2-expiry-isolation"
	descriptor, err := store.retainGoContextWithPiSession(result, piSessionID)
	if err != nil || !descriptor.Available {
		t.Fatalf("retainGoContextWithPiSession() = (%#v, %v), want available", descriptor, err)
	}
	now = now.Add(continuationLifetime)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations: store, questionEvaluator: evaluator.DeterministicEvaluator{},
	})
	response := serveQuestionRequest(handler, validDaemonQuestionRequest(descriptor.ID))
	if response.Code != http.StatusConflict || historyQueryErrorCode(t, response) != "continuation_unavailable" || bytes.Contains(response.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("expired response = (%d, %s), want safe continuation_unavailable", response.Code, response.Body.String())
	}
}

func assertV2SessionIsolation(t *testing.T, sessionID string, response []byte, values ...any) {
	t.Helper()
	encoded, err := json.Marshal(values)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(response, []byte(sessionID)) || bytes.Contains(encoded, []byte(sessionID)) {
		t.Fatalf("Pi Session ID crossed model/response boundary: response=%s values=%s", response, encoded)
	}
}

func newGoContextPreviewRepository(t *testing.T) (string, string) {
	t.Helper()
	repository := newHistoryQueryRepository(t)
	runPiSessionGit(t, repository, "config", "user.name", "Pi LearnLoop Test")
	runPiSessionGit(t, repository, "config", "user.email", "test@example.invalid")
	if err := os.MkdirAll(filepath.Join(repository, "dep"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "go.mod"), []byte("module example.com/context-preview\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "dep", "dep.go"), []byte("package dep\n\ntype Token string\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repository, "changed.go"), []byte("package contextpreview\n\nfunc Build() int { return 1 }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runPiSessionGit(t, repository, "add", ".")
	runPiSessionGit(t, repository, "commit", "-q", "-m", "base")
	base := strings.TrimSpace(runPiSessionGit(t, repository, "rev-parse", "HEAD"))
	if err := os.WriteFile(filepath.Join(repository, "changed.go"), []byte("package contextpreview\n\nimport \"example.com/context-preview/dep\"\n\nfunc Build() dep.Token { return \"ok\" }\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return repository, base
}
