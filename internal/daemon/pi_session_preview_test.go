package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/evaluator"
)

func TestPiSessionEvidencePreviewMatchesGenericPreviewAndRetainsSeparateProvenance(t *testing.T) {
	repository, base := newPiSessionPreviewRepository(t)
	store := newContinuationStore()
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{
		continuations:     store,
		questionEvaluator: evaluator.DeterministicEvaluator{},
	})
	selection := map[string]any{"kind": "working_tree", "base": base}
	genericBody, err := json.Marshal(map[string]any{"repository": repository, "selection": selection})
	if err != nil {
		t.Fatalf("Marshal(generic request): %v", err)
	}
	piSessionID := "session-preview-isolation-123"
	piSessionBody, err := json.Marshal(map[string]any{
		"repository": repository, "pi_session_id": piSessionID, "selection": selection,
	})
	if err != nil {
		t.Fatalf("Marshal(Pi Session request): %v", err)
	}

	genericResponse := servePreviewRoute(handler, "/v1/evidence-previews", genericBody, true, "application/json")
	piSessionResponse := servePreviewRoute(handler, "/v1/pi-session-evidence-previews", piSessionBody, true, "application/json")
	if genericResponse.Code != http.StatusOK || piSessionResponse.Code != http.StatusOK {
		t.Fatalf("preview statuses = (%d, %d), want (200, 200); bodies = (%s, %s)", genericResponse.Code, piSessionResponse.Code, genericResponse.Body.String(), piSessionResponse.Body.String())
	}
	if piSessionResponse.Header().Get("Cache-Control") != "no-store" || bytes.Contains(piSessionResponse.Body.Bytes(), []byte(piSessionID)) {
		t.Fatalf("Pi Session preview leaked identity or omitted no-store: headers=%v body=%s", piSessionResponse.Header(), piSessionResponse.Body.String())
	}

	var genericPayload map[string]any
	var piSessionPayload map[string]any
	if err := json.Unmarshal(genericResponse.Body.Bytes(), &genericPayload); err != nil {
		t.Fatalf("Unmarshal(generic preview): %v", err)
	}
	if err := json.Unmarshal(piSessionResponse.Body.Bytes(), &piSessionPayload); err != nil {
		t.Fatalf("Unmarshal(Pi Session preview): %v", err)
	}
	genericContinuationID := continuationIDFromPreview(t, genericPayload)
	piSessionContinuationID := continuationIDFromPreview(t, piSessionPayload)
	delete(genericPayload, "continuation")
	delete(piSessionPayload, "continuation")
	if !reflect.DeepEqual(piSessionPayload, genericPayload) {
		t.Fatalf("Pi Session preview = %#v, want exact generic evidence response %#v", piSessionPayload, genericPayload)
	}

	genericRetained, ok := store.consume(genericContinuationID)
	if !ok || genericRetained.piSessionID != "" {
		t.Fatalf("generic retained value = (%#v, %t), want empty provenance", genericRetained, ok)
	}
	piSessionRetained, ok := store.consume(piSessionContinuationID)
	if !ok || piSessionRetained.piSessionID != piSessionID || piSessionRetained.result.RepositoryRoot != repository {
		t.Fatalf("Pi Session retained value = (%#v, %t), want separate provenance for %q", piSessionRetained, ok, repository)
	}
}

func TestPiSessionEvidencePreviewAcceptsExplicitCommitRange(t *testing.T) {
	repository, base := newPiSessionPreviewRepository(t)
	runPiSessionGit(t, repository, "add", "sample.go")
	runPiSessionGit(t, repository, "commit", "-q", "-m", "head")
	head := strings.TrimSpace(runPiSessionGit(t, repository, "rev-parse", "HEAD"))
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	body, err := json.Marshal(map[string]any{
		"repository":    repository,
		"pi_session_id": "session-commit-range",
		"selection": map[string]any{
			"kind": "commit_range", "base": base, "head": head,
		},
	})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	response := servePreviewRoute(handler, "/v1/pi-session-evidence-previews", body, true, "application/json")
	if response.Code != http.StatusOK {
		t.Fatalf("commit-range preview status = %d, want 200; body = %s", response.Code, response.Body.String())
	}
	if bytes.Contains(response.Body.Bytes(), []byte("session-commit-range")) {
		t.Fatalf("commit-range preview echoes Pi Session identity: %s", response.Body.String())
	}
}

func TestPiSessionEvidencePreviewReturnsStableDeadlineError(t *testing.T) {
	repository, base := newPiSessionPreviewRepository(t)
	body, err := json.Marshal(map[string]any{
		"repository": repository, "pi_session_id": "session-deadline", "selection": map[string]any{"kind": "working_tree", "base": base},
	})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:43210/v1/pi-session-evidence-previews", bytes.NewReader(body))
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

func TestPiSessionEvidencePreviewRequiresStrictAuthenticatedHTTP(t *testing.T) {
	repository, base := newPiSessionPreviewRepository(t)
	handler := newHandler("instance", "127.0.0.1:43210", "token", serverServices{})
	valid := `{"repository":` + historyQueryJSON(repository) + `,"pi_session_id":"safe-session","selection":{"kind":"working_tree","base":` + historyQueryJSON(base) + `}}`
	privateInvalid := "private/session-sentinel"
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
		{name: "authentication", method: http.MethodPost, body: []byte(valid), contentType: "application/json", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "media type", method: http.MethodPost, body: []byte(valid), authorize: true, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "body limit", method: http.MethodPost, body: []byte(strings.Repeat("x", maxRequestBytes+1)), authorize: true, contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "malformed JSON", method: http.MethodPost, body: []byte(`{"repository":`), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "unknown field", method: http.MethodPost, body: []byte(strings.TrimSuffix(valid, "}") + `,"extra":true}`), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "duplicate field", method: http.MethodPost, body: []byte(strings.Replace(valid, `"pi_session_id":"safe-session"`, `"pi_session_id":"safe-session","pi_session_id":"safe-session"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "case-folded field", method: http.MethodPost, body: []byte(strings.Replace(valid, `"pi_session_id"`, `"Pi_Session_ID"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "duplicate nested field", method: http.MethodPost, body: []byte(strings.Replace(valid, `"kind":"working_tree"`, `"kind":"working_tree","kind":"working_tree"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "case-folded selection field", method: http.MethodPost, body: []byte(strings.Replace(valid, `"base"`, `"Base"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "working tree head", method: http.MethodPost, body: []byte(strings.Replace(valid, `"base":`+historyQueryJSON(base), `"base":`+historyQueryJSON(base)+`,"head":null`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "trailing JSON", method: http.MethodPost, body: []byte(valid + `{}`), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "missing Session identity", method: http.MethodPost, body: []byte(strings.Replace(valid, `,"pi_session_id":"safe-session"`, "", 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "invalid Session identity", method: http.MethodPost, body: []byte(strings.Replace(valid, "safe-session", privateInvalid, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "oversized Session identity", method: http.MethodPost, body: []byte(strings.Replace(valid, "safe-session", strings.Repeat("a", 129), 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "relative repository", method: http.MethodPost, body: []byte(strings.Replace(valid, historyQueryJSON(repository), `"relative"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "invalid selection", method: http.MethodPost, body: []byte(strings.Replace(valid, `"working_tree"`, `"automatic"`, 1)), authorize: true, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := servePreviewRequest(handler, test.method, []byte(test.body), test.authorize, test.contentType, "127.0.0.1:43210", "127.0.0.1:54321", "")
			if response.Code != test.wantStatus || historyQueryErrorCode(t, response) != test.wantCode {
				t.Fatalf("response = (%d, %s), want (%d, %s)", response.Code, response.Body.String(), test.wantStatus, test.wantCode)
			}
			if bytes.Contains(response.Body.Bytes(), []byte(privateInvalid)) {
				t.Fatalf("safe error echoed invalid Session identity: %s", response.Body.String())
			}
		})
	}

	for _, test := range []struct {
		name   string
		host   string
		remote string
		origin string
	}{
		{name: "host", host: "127.0.0.1:43211", remote: "127.0.0.1:54321"},
		{name: "origin", host: "127.0.0.1:43210", remote: "127.0.0.1:54321", origin: "https://example.invalid"},
		{name: "IPv6 peer", host: "127.0.0.1:43210", remote: "[::1]:54321"},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := servePreviewRequest(handler, http.MethodPost, []byte(valid), true, "application/json", test.host, test.remote, test.origin)
			if response.Code != http.StatusForbidden || historyQueryErrorCode(t, response) != "forbidden" {
				t.Fatalf("response = (%d, %s), want forbidden", response.Code, response.Body.String())
			}
		})
	}
}

func servePreviewRoute(handler http.Handler, path string, body []byte, authorize bool, contentType string) *httptest.ResponseRecorder {
	return servePreviewRequest(handler, http.MethodPost, body, authorize, contentType, "127.0.0.1:43210", "127.0.0.1:54321", "", path)
}

func servePreviewRequest(handler http.Handler, method string, body []byte, authorize bool, contentType, host, remote, origin string, paths ...string) *httptest.ResponseRecorder {
	path := "/v1/pi-session-evidence-previews"
	if len(paths) == 1 {
		path = paths[0]
	}
	request := httptest.NewRequest(method, "http://127.0.0.1:43210"+path, bytes.NewReader(body))
	request.Host = host
	request.RemoteAddr = remote
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if authorize {
		request.Header.Set("Authorization", "PiLearnLoop token")
	}
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func continuationIDFromPreview(t *testing.T, payload map[string]any) string {
	t.Helper()
	continuation, ok := payload["continuation"].(map[string]any)
	if !ok || continuation["available"] != true {
		t.Fatalf("continuation = %#v, want available", payload["continuation"])
	}
	id, ok := continuation["id"].(string)
	if !ok || !validContinuationID(id) {
		t.Fatalf("continuation ID = %#v, want valid", continuation["id"])
	}
	return id
}

func newPiSessionPreviewRepository(t *testing.T) (string, string) {
	t.Helper()
	repository := newHistoryQueryRepository(t)
	runPiSessionGit(t, repository, "config", "user.name", "Pi LearnLoop Test")
	runPiSessionGit(t, repository, "config", "user.email", "test@example.invalid")
	path := repository + "/sample.go"
	if err := os.WriteFile(path, []byte("package sample\n\nfunc Answer() int { return 1 }\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(base): %v", err)
	}
	runPiSessionGit(t, repository, "add", "sample.go")
	runPiSessionGit(t, repository, "commit", "-q", "-m", "base")
	base := strings.TrimSpace(runPiSessionGit(t, repository, "rev-parse", "HEAD"))
	if err := os.WriteFile(path, []byte("package sample\n\nfunc Answer() int { return 2 }\n"), 0o600); err != nil {
		t.Fatalf("WriteFile(change): %v", err)
	}
	return repository, base
}

func runPiSessionGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v: %s", arguments, err, output)
	}
	return string(output)
}
