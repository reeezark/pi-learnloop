package daemon_test

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/daemon"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/history"
)

type runtimeDescriptor struct {
	SchemaVersion   int    `json:"schema_version"`
	ProtocolVersion int    `json:"protocol_version"`
	InstanceID      string `json:"instance_id"`
	PID             int    `json:"pid"`
	BaseURL         string `json:"base_url"`
	StartedAt       string `json:"started_at"`
}

func TestRunPublishesLoopbackStatus(t *testing.T) {
	_, descriptor := startDaemon(t)
	if descriptor.SchemaVersion != 1 || descriptor.ProtocolVersion != 1 {
		t.Fatalf("descriptor versions = (%d, %d), want (1, 1)", descriptor.SchemaVersion, descriptor.ProtocolVersion)
	}
	if descriptor.InstanceID == "" {
		t.Fatal("descriptor instance_id is empty")
	}
	parsedURL, err := url.Parse(descriptor.BaseURL)
	if err != nil {
		t.Fatalf("Parse(%q): %v", descriptor.BaseURL, err)
	}
	if parsedURL.Scheme != "http" || parsedURL.Hostname() != "127.0.0.1" || parsedURL.Port() == "" {
		t.Fatalf("base_url = %q, want an HTTP IPv4 loopback URL with an assigned port", descriptor.BaseURL)
	}
	if ip := net.ParseIP(parsedURL.Hostname()); ip == nil || ip.To4() == nil || !ip.IsLoopback() {
		t.Fatalf("base_url hostname = %q, want IPv4 loopback", parsedURL.Hostname())
	}

	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   2 * time.Second,
	}
	response, err := client.Get(descriptor.BaseURL + "/v1/status")
	if err != nil {
		t.Fatalf("GET /v1/status: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/status status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Header.Get("Cache-Control"); got != "no-store" {
		t.Fatalf("Cache-Control = %q, want no-store", got)
	}
	var status struct {
		ProtocolVersion int    `json:"protocol_version"`
		InstanceID      string `json:"instance_id"`
		Status          string `json:"status"`
	}
	if err := json.NewDecoder(response.Body).Decode(&status); err != nil {
		t.Fatalf("decode status: %v", err)
	}
	if status.ProtocolVersion != 1 || status.InstanceID != descriptor.InstanceID || status.Status != "ready" {
		t.Fatalf("status = %#v, want protocol 1, matching instance, and ready", status)
	}
}

func TestRunContinuesWhenHistoryStorageIsUnavailable(t *testing.T) {
	baseDirectory := t.TempDir()
	stateDir := filepath.Join(baseDirectory, "runtime")
	dataPath := filepath.Join(baseDirectory, "not-a-directory")
	if err := os.WriteFile(dataPath, []byte("occupied"), 0o600); err != nil {
		t.Fatalf("WriteFile(unavailable data path): %v", err)
	}
	running := startDaemonAtWithData(t, stateDir, dataPath)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	continuation := requestPreviewContinuation(t, running.descriptor.BaseURL, token, repository, `{"kind":"commit_range","base":`+quoted(base)+`,"head":`+quoted(head)+`}`)
	if !continuation.Available {
		t.Fatalf("continuation = %#v, want preview and question flow while history is unavailable", continuation)
	}
	response := postQuestionSet(t, running.descriptor.BaseURL, token, validQuestionRequest(continuation.ID))
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("question status = %d, want %d; body = %s", response.StatusCode, http.StatusOK, content)
	}

	historyBody, err := json.Marshal(map[string]any{"repository": repository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(history query): %v", err)
	}
	historyRequest, err := http.NewRequest(http.MethodPost, running.descriptor.BaseURL+"/v1/learning-history-queries", bytes.NewReader(historyBody))
	if err != nil {
		t.Fatalf("NewRequest(history query): %v", err)
	}
	historyRequest.Header.Set("Content-Type", "application/json")
	historyRequest.Header.Set("Authorization", "PiLearnLoop "+token)
	historyResponse, err := localHTTPClient().Do(historyRequest)
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	defer historyResponse.Body.Close()
	var historyError struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(historyResponse.Body).Decode(&historyError); err != nil {
		t.Fatalf("decode history error: %v", err)
	}
	if historyResponse.StatusCode != http.StatusServiceUnavailable || historyError.Error.Code != "history_unavailable" {
		t.Fatalf("history response = (%d, %#v), want history_unavailable", historyResponse.StatusCode, historyError)
	}
}

func TestRunRecoversRunningHistoryWithoutEvaluatorCall(t *testing.T) {
	baseDirectory := t.TempDir()
	stateDir := filepath.Join(baseDirectory, "runtime")
	dataDir := filepath.Join(baseDirectory, "data")
	root := filepath.Join(baseDirectory, "repository")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	recordID, err := store.Create(context.Background(), daemonHistoryStart(root))
	if err != nil {
		t.Fatalf("history.Create(): %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("history.Close(): %v", err)
	}

	evaluatorMarker := filepath.Join(baseDirectory, "evaluator-called")
	t.Setenv("PI_LEARNLOOP_EVALUATOR_MARKER", evaluatorMarker)
	running := startDaemonAtWithData(t, stateDir, dataDir)
	if _, err := os.Lstat(evaluatorMarker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("evaluator marker error = %v, want no evaluator call during recovery", err)
	}

	database, err := sql.Open("sqlite", filepath.Join(dataDir, "history.db"))
	if err != nil {
		t.Fatalf("sql.Open(history): %v", err)
	}
	defer database.Close()
	var status history.Status
	if err := database.QueryRowContext(context.Background(), "SELECT status FROM learning_attempts WHERE record_id = ?", recordID).Scan(&status); err != nil {
		t.Fatalf("query recovered status: %v", err)
	}
	if status != history.StatusInterrupted {
		t.Fatalf("recovered status = %q, want interrupted", status)
	}
	running.stop()
}

func TestRunRejectsRelativeTestDataDirectory(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	err := daemon.Run(context.Background(), daemon.Config{StateDir: stateDir, DataDir: "relative-data"})
	if err == nil || !strings.Contains(err.Error(), "data directory must be absolute") {
		t.Fatalf("Run(relative DataDir) error = %v, want absolute-path rejection", err)
	}
}

func TestRunPublishesDiscoveryOnlyAfterEvaluatorPreflight(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	binDirectory := t.TempDir()
	marker := filepath.Join(t.TempDir(), "preflight-started")
	fakePi := filepath.Join(binDirectory, "pi")
	script := "#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then\n  : > \"$PI_LEARNLOOP_PREFLIGHT_MARKER\"\n  sleep 1\n  printf '0.84.3\\n'\n  exit 0\nfi\nexit 17\n"
	if err := os.WriteFile(fakePi, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(fake Pi): %v", err)
	}
	t.Setenv("PATH", binDirectory)
	t.Setenv("PI_LEARNLOOP_PREFLIGHT_MARKER", marker)

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx, daemon.Config{StateDir: stateDir, DataDir: filepath.Join(filepath.Dir(stateDir), "data")})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-errCh:
			if err != nil {
				t.Errorf("Run() shutdown error = %v", err)
			}
		case <-time.After(10 * time.Second):
			t.Error("Run() did not stop after cancellation")
		}
	})

	waitForFile(t, marker)
	assertNotExist(t, filepath.Join(stateDir, "daemon.json"))
	_ = waitForDescriptor(t, filepath.Join(stateDir, "daemon.json"))
}

func TestAuthorizedClientCanRequestEvidencePreview(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := waitForFile(t, filepath.Join(stateDir, "daemon.token"))
	repository, base, head := changedRepository(t)

	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+string(token))
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
		ProtocolVersion int `json:"protocol_version"`
		AppliedLimits   struct {
			MaxFiles        int `json:"max_files"`
			MaxDeclarations int `json:"max_declarations"`
			MaxExcerptBytes int `json:"max_excerpt_bytes"`
		} `json:"applied_limits"`
		Preview struct {
			RepositoryRoot string `json:"repository_root"`
			BaseRevision   string `json:"base_revision"`
			HeadRevision   string `json:"head_revision"`
			Files          []struct {
				Path         string `json:"path"`
				Declarations []struct {
					Name    string `json:"name"`
					Excerpt string `json:"excerpt"`
				} `json:"declarations"`
			} `json:"files"`
		} `json:"preview"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode evidence preview: %v", err)
	}
	if result.ProtocolVersion != 1 {
		t.Fatalf("protocol_version = %d, want 1", result.ProtocolVersion)
	}
	if result.AppliedLimits.MaxFiles != 20 || result.AppliedLimits.MaxDeclarations != 100 || result.AppliedLimits.MaxExcerptBytes != 128*1024 {
		t.Fatalf("applied_limits = %#v, want accepted ADR-0002 limits", result.AppliedLimits)
	}
	canonicalRepository, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repository, err)
	}
	if result.Preview.RepositoryRoot != canonicalRepository || result.Preview.BaseRevision != base || result.Preview.HeadRevision != head {
		t.Fatalf("preview revisions/root = (%q, %q, %q), want (%q, %q, %q)", result.Preview.RepositoryRoot, result.Preview.BaseRevision, result.Preview.HeadRevision, canonicalRepository, base, head)
	}
	if len(result.Preview.Files) != 1 || result.Preview.Files[0].Path != "sample.go" {
		t.Fatalf("preview files = %#v, want sample.go", result.Preview.Files)
	}
	if declarations := result.Preview.Files[0].Declarations; len(declarations) != 1 || declarations[0].Name != "Answer" || !strings.Contains(declarations[0].Excerpt, "return 2") {
		t.Fatalf("preview declarations = %#v, want changed Answer source", declarations)
	}
}

func TestEvidencePreviewRejectsDuplicateJSONFields(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := waitForFile(t, filepath.Join(stateDir, "daemon.token"))
	repository, base, head := changedRepository(t)
	body := []byte(`{"repository":` + quoted(repository) + `,"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+string(token))
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST duplicate JSON fields: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusBadRequest {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("duplicate JSON status = %d, want %d; body = %s", response.StatusCode, http.StatusBadRequest, content)
	}
	assertErrorCode(t, response.Body, "invalid_request")
}

func TestRunRejectsSecondInstanceForSameStateDirectory(t *testing.T) {
	stateDir, _ := startDaemon(t)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := daemon.Run(ctx, daemon.Config{StateDir: stateDir, DataDir: filepath.Join(filepath.Dir(stateDir), "data")})
	if !errors.Is(err, daemon.ErrAlreadyRunning) {
		t.Fatalf("second Run() error = %v, want ErrAlreadyRunning", err)
	}
}

func TestRunRejectsSymlinkedRuntimeCredential(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("Mkdir(%q): %v", stateDir, err)
	}
	target := filepath.Join(t.TempDir(), "outside-token")
	if err := os.WriteFile(target, []byte("do not replace"), 0o600); err != nil {
		t.Fatalf("WriteFile(%q): %v", target, err)
	}
	tokenPath := filepath.Join(stateDir, "daemon.token")
	if err := os.Symlink(target, tokenPath); err != nil {
		t.Fatalf("Symlink(%q): %v", tokenPath, err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := daemon.Run(ctx, daemon.Config{StateDir: stateDir, DataDir: filepath.Join(filepath.Dir(stateDir), "data")}); err == nil {
		t.Fatal("Run() error = nil, want symlinked credential rejection")
	}
	info, err := os.Lstat(tokenPath)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", tokenPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("runtime token mode = %v, want original symlink to remain", info.Mode())
	}
}

func TestRunRejectsInsecureStateDirectoryPermissions(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("Mkdir(%q): %v", stateDir, err)
	}
	if err := os.Chmod(stateDir, 0o755); err != nil {
		t.Fatalf("Chmod(%q): %v", stateDir, err)
	}
	if err := daemon.Run(context.Background(), daemon.Config{StateDir: stateDir, DataDir: filepath.Join(filepath.Dir(stateDir), "data")}); err == nil || !strings.Contains(err.Error(), "want 0700") {
		t.Fatalf("Run() error = %v, want insecure directory permission rejection", err)
	}
	assertMode(t, stateDir, 0o755)
}

func TestEvidencePreviewEnforcesLocalAuthenticationBoundary(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`)

	tests := []struct {
		name       string
		token      string
		origin     string
		host       string
		wantStatus int
		wantCode   string
	}{
		{name: "missing token", wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "incorrect token", token: strings.Repeat("x", 43), wantStatus: http.StatusUnauthorized, wantCode: "unauthorized"},
		{name: "browser origin", token: token, origin: "https://example.invalid", wantStatus: http.StatusForbidden, wantCode: "forbidden"},
		{name: "unadvertised host", token: token, host: "localhost:8080", wantStatus: http.StatusForbidden, wantCode: "forbidden"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
			if err != nil {
				t.Fatalf("NewRequest(): %v", err)
			}
			request.Header.Set("Content-Type", "application/json")
			if test.token != "" {
				request.Header.Set("Authorization", "PiLearnLoop "+test.token)
			}
			if test.origin != "" {
				request.Header.Set("Origin", test.origin)
			}
			if test.host != "" {
				request.Host = test.host
			}
			response, err := localHTTPClient().Do(request)
			if err != nil {
				t.Fatalf("POST guarded request: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != test.wantStatus {
				content, _ := io.ReadAll(response.Body)
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, test.wantStatus, content)
			}
			if got := response.Header.Get("Access-Control-Allow-Origin"); got != "" {
				t.Fatalf("Access-Control-Allow-Origin = %q, want none", got)
			}
			if test.wantStatus == http.StatusUnauthorized && response.Header.Get("WWW-Authenticate") != "PiLearnLoop" {
				t.Fatalf("WWW-Authenticate = %q, want PiLearnLoop", response.Header.Get("WWW-Authenticate"))
			}
			assertErrorCode(t, response.Body, test.wantCode)
		})
	}
}

func TestEvidencePreviewRejectsAnyNonEmptyOriginValue(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	request.Header["Origin"] = []string{"", "https://example.invalid"}
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST multiple Origin values: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("multiple Origin values status = %d, want %d; body = %s", response.StatusCode, http.StatusForbidden, content)
	}
}

func TestEvidencePreviewReturnsStableProtocolErrors(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, base, head := changedRepository(t)
	validBody := `{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`
	invalidRepository := t.TempDir()

	brokenRepository := newRepository(t)
	writeRepositoryFile(t, brokenRepository, "broken.go", "package broken\n")
	commitAll(t, brokenRepository, "base")
	brokenBase := revision(t, brokenRepository, "HEAD")
	writeRepositoryFile(t, brokenRepository, "broken.go", "package broken\nfunc Broken( {\n")
	commitAll(t, brokenRepository, "broken")
	brokenHead := revision(t, brokenRepository, "HEAD")
	brokenBody := `{"repository":` + quoted(brokenRepository) + `,"selection":{"kind":"commit_range","base":` + quoted(brokenBase) + `,"head":` + quoted(brokenHead) + `}}`

	tests := []struct {
		name        string
		body        string
		contentType string
		wantStatus  int
		wantCode    string
	}{
		{name: "unknown field", body: strings.TrimSuffix(validBody, "}") + `,"limits":{}}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "trailing JSON", body: validBody + `{}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "relative repository", body: `{"repository":"relative","selection":{"kind":"commit_range","base":"HEAD","head":"HEAD"}}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "working tree with head", body: `{"repository":` + quoted(repository) + `,"selection":{"kind":"working_tree","base":` + quoted(base) + `,"head":` + quoted(head) + `}}`, contentType: "application/json", wantStatus: http.StatusBadRequest, wantCode: "invalid_request"},
		{name: "oversized body", body: strings.Repeat(" ", 16*1024+1), contentType: "application/json", wantStatus: http.StatusRequestEntityTooLarge, wantCode: "request_too_large"},
		{name: "unsupported media type", body: validBody, contentType: "text/plain", wantStatus: http.StatusUnsupportedMediaType, wantCode: "unsupported_media_type"},
		{name: "not a repository", body: `{"repository":` + quoted(invalidRepository) + `,"selection":{"kind":"commit_range","base":"HEAD","head":"HEAD"}}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_repository"},
		{name: "invalid revision", body: `{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":"does-not-exist","head":"HEAD"}}`, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_revision"},
		{name: "invalid Go source", body: brokenBody, contentType: "application/json", wantStatus: http.StatusUnprocessableEntity, wantCode: "invalid_source"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", strings.NewReader(test.body))
			if err != nil {
				t.Fatalf("NewRequest(): %v", err)
			}
			request.Header.Set("Content-Type", test.contentType)
			request.Header.Set("Authorization", "PiLearnLoop "+token)
			response, err := localHTTPClient().Do(request)
			if err != nil {
				t.Fatalf("POST protocol error case: %v", err)
			}
			defer response.Body.Close()
			content, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("ReadAll(error response): %v", err)
			}
			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.StatusCode, test.wantStatus, content)
			}
			if response.Header.Get("Cache-Control") != "no-store" {
				t.Fatalf("Cache-Control = %q, want no-store", response.Header.Get("Cache-Control"))
			}
			if bytes.Contains(content, []byte(token)) || bytes.Contains(content, []byte(repository)) || bytes.Contains(content, []byte(brokenRepository)) {
				t.Fatalf("error response leaks a token or repository path: %s", content)
			}
			assertErrorCode(t, bytes.NewReader(content), test.wantCode)
		})
	}
}

func TestRoutesReturnVersionedNotFoundAndMethodErrors(t *testing.T) {
	_, descriptor := startDaemon(t)
	tests := []struct {
		method     string
		path       string
		wantStatus int
		wantCode   string
		wantAllow  string
	}{
		{method: http.MethodGet, path: "/v1/unknown", wantStatus: http.StatusNotFound, wantCode: "not_found"},
		{method: http.MethodPost, path: "/v1/status", wantStatus: http.StatusMethodNotAllowed, wantCode: "method_not_allowed", wantAllow: http.MethodGet},
	}
	for _, test := range tests {
		request, err := http.NewRequest(test.method, descriptor.BaseURL+test.path, nil)
		if err != nil {
			t.Fatalf("NewRequest(): %v", err)
		}
		response, err := localHTTPClient().Do(request)
		if err != nil {
			t.Fatalf("%s %s: %v", test.method, test.path, err)
		}
		if response.StatusCode != test.wantStatus {
			content, _ := io.ReadAll(response.Body)
			response.Body.Close()
			t.Fatalf("%s %s status = %d, want %d; body = %s", test.method, test.path, response.StatusCode, test.wantStatus, content)
		}
		if response.Header.Get("Allow") != test.wantAllow {
			response.Body.Close()
			t.Fatalf("%s %s Allow = %q, want %q", test.method, test.path, response.Header.Get("Allow"), test.wantAllow)
		}
		assertErrorCode(t, response.Body, test.wantCode)
		response.Body.Close()
	}
}

func TestRuntimeCredentialsAreProtectedRotatedAndCleanedUp(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	first := startDaemonAt(t, stateDir)
	firstToken := waitForFile(t, filepath.Join(stateDir, "daemon.token"))
	assertMode(t, stateDir, 0o700)
	assertMode(t, filepath.Join(stateDir, "daemon.json"), 0o600)
	assertMode(t, filepath.Join(stateDir, "daemon.token"), 0o600)
	assertMode(t, filepath.Join(stateDir, "daemon.lock"), 0o600)
	decodedToken, err := base64.RawURLEncoding.DecodeString(string(firstToken))
	if err != nil || len(decodedToken) != 32 || len(firstToken) != 43 {
		t.Fatalf("instance token = %q, want 32 random bytes encoded as 43 unpadded base64url characters", firstToken)
	}
	decodedInstanceID, err := base64.RawURLEncoding.DecodeString(first.descriptor.InstanceID)
	if err != nil || len(decodedInstanceID) != 16 {
		t.Fatalf("instance_id = %q, want 16 random bytes encoded as unpadded base64url", first.descriptor.InstanceID)
	}
	if _, err := time.Parse(time.RFC3339, first.descriptor.StartedAt); err != nil {
		t.Fatalf("started_at = %q, want RFC3339: %v", first.descriptor.StartedAt, err)
	}

	first.stop()
	assertNotExist(t, filepath.Join(stateDir, "daemon.json"))
	assertNotExist(t, filepath.Join(stateDir, "daemon.token"))
	assertMode(t, filepath.Join(stateDir, "daemon.lock"), 0o600)

	second := startDaemonAt(t, stateDir)
	secondToken := waitForFile(t, filepath.Join(stateDir, "daemon.token"))
	if bytes.Equal(firstToken, secondToken) {
		t.Fatal("instance token did not rotate across daemon restart")
	}
	if first.descriptor.InstanceID == second.descriptor.InstanceID {
		t.Fatal("instance_id did not rotate across daemon restart")
	}
	second.stop()
}

func TestRunReplacesProtectedStaleDiscoveryState(t *testing.T) {
	stateDir := filepath.Join(t.TempDir(), "runtime")
	if err := os.Mkdir(stateDir, 0o700); err != nil {
		t.Fatalf("Mkdir(%q): %v", stateDir, err)
	}
	staleDescriptor := runtimeDescriptor{
		SchemaVersion:   1,
		ProtocolVersion: 1,
		InstanceID:      "stale-instance",
		PID:             1,
		BaseURL:         "http://127.0.0.1:1",
		StartedAt:       "2026-08-31T00:00:00Z",
	}
	content, err := json.Marshal(staleDescriptor)
	if err != nil {
		t.Fatalf("Marshal(stale descriptor): %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.json"), content, 0o600); err != nil {
		t.Fatalf("WriteFile(stale descriptor): %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.token"), []byte(strings.Repeat("s", 43)), 0o600); err != nil {
		t.Fatalf("WriteFile(stale token): %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "daemon.lock"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile(stale lock): %v", err)
	}

	running := startDaemonAt(t, stateDir)
	newDescriptor := waitForNewDescriptor(t, filepath.Join(stateDir, "daemon.json"), staleDescriptor.InstanceID)
	newToken := waitForChangedFile(t, filepath.Join(stateDir, "daemon.token"), []byte(strings.Repeat("s", 43)))
	if newDescriptor.InstanceID == staleDescriptor.InstanceID || bytes.Equal(newToken, []byte(strings.Repeat("s", 43))) {
		t.Fatal("stale discovery state was not replaced")
	}
	running.descriptor = newDescriptor
}

func TestEvidencePreviewCancellationStopsGitAnalysis(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, _, _ := changedRepository(t)

	fakeBin := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "git.pid")
	fakeGit := filepath.Join(fakeBin, "git")
	script := "#!/bin/sh\necho $$ > \"$PI_LEARNLOOP_TEST_PID_FILE\"\nexec sleep 60\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(%q): %v", fakeGit, err)
	}
	t.Setenv("PI_LEARNLOOP_TEST_PID_FILE", pidFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":"HEAD","head":"HEAD"}}`)
	requestCtx, cancel := context.WithCancel(context.Background())
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequestWithContext(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	type requestResult struct {
		response *http.Response
		err      error
	}
	resultCh := make(chan requestResult, 1)
	go func() {
		response, err := localHTTPClient().Do(request)
		resultCh <- requestResult{response: response, err: err}
	}()
	pidContent := waitForFile(t, pidFile)
	cancel()
	select {
	case result := <-resultCh:
		if result.response != nil {
			result.response.Body.Close()
		}
		if !errors.Is(result.err, context.Canceled) {
			t.Fatalf("cancelled POST error = %v, want context canceled", result.err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled POST did not return")
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(pidContent)))
	if err != nil {
		t.Fatalf("parse fake Git PID %q: %v", pidContent, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		err := syscall.Kill(pid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake Git process %d is still alive after request cancellation", pid)
}

func TestEvidencePreviewDeadlineReturnsStableError(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository, _, _ := changedRepository(t)

	fakeBin := t.TempDir()
	pidFile := filepath.Join(t.TempDir(), "git.pid")
	fakeGit := filepath.Join(fakeBin, "git")
	script := "#!/bin/sh\necho $$ > \"$PI_LEARNLOOP_TEST_PID_FILE\"\nexec sleep 60\n"
	if err := os.WriteFile(fakeGit, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(%q): %v", fakeGit, err)
	}
	t.Setenv("PI_LEARNLOOP_TEST_PID_FILE", pidFile)
	t.Setenv("PATH", fakeBin+string(os.PathListSeparator)+os.Getenv("PATH"))

	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"commit_range","base":"HEAD","head":"HEAD"}}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	client := &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   40 * time.Second,
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatalf("POST deadline case: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusGatewayTimeout {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("deadline status = %d, want %d; body = %s", response.StatusCode, http.StatusGatewayTimeout, content)
	}
	assertErrorCode(t, response.Body, "deadline_exceeded")

	pidContent := waitForFile(t, pidFile)
	pid, err := strconv.Atoi(strings.TrimSpace(string(pidContent)))
	if err != nil {
		t.Fatalf("parse fake Git PID %q: %v", pidContent, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake Git process %d is still alive after evidence deadline", pid)
}

func TestAuthorizedClientCanRequestWorkingTreePreview(t *testing.T) {
	stateDir, descriptor := startDaemon(t)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	repository := newRepository(t)
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Pending() int { return 1 }\n")
	commitAll(t, repository, "base")
	base := revision(t, repository, "HEAD")
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Pending() int { return 2 }\n")

	body := []byte(`{"repository":` + quoted(repository) + `,"selection":{"kind":"working_tree","base":` + quoted(base) + `}}`)
	request, err := http.NewRequest(http.MethodPost, descriptor.BaseURL+"/v1/evidence-previews", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("POST working-tree preview: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		content, _ := io.ReadAll(response.Body)
		t.Fatalf("working-tree status = %d, want 200; body = %s", response.StatusCode, content)
	}
	var result struct {
		Preview struct {
			HeadRevision string `json:"head_revision"`
			Files        []struct {
				Path         string `json:"path"`
				Declarations []struct {
					Name string `json:"name"`
				} `json:"declarations"`
			} `json:"files"`
		} `json:"preview"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode working-tree preview: %v", err)
	}
	if result.Preview.HeadRevision != "WORKTREE" || len(result.Preview.Files) != 1 || result.Preview.Files[0].Path != "sample.go" || len(result.Preview.Files[0].Declarations) != 1 || result.Preview.Files[0].Declarations[0].Name != "Pending" {
		t.Fatalf("working-tree preview = %#v, want WORKTREE and changed Pending declaration", result.Preview)
	}
}

type runningDaemon struct {
	t          *testing.T
	cancel     context.CancelFunc
	errCh      chan error
	descriptor runtimeDescriptor
	stopOnce   sync.Once
}

func startDaemonAt(t *testing.T, stateDir string) *runningDaemon {
	t.Helper()
	return startDaemonAtWithData(t, stateDir, filepath.Join(filepath.Dir(stateDir), "data"))
}

func startDaemonAtWithData(t *testing.T, stateDir, dataDir string) *runningDaemon {
	t.Helper()
	installDaemonFakePi(t)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- daemon.Run(ctx, daemon.Config{StateDir: stateDir, DataDir: dataDir})
	}()
	running := &runningDaemon{
		t:          t,
		cancel:     cancel,
		errCh:      errCh,
		descriptor: waitForDescriptor(t, filepath.Join(stateDir, "daemon.json")),
	}
	t.Cleanup(running.stop)
	return running
}

func TestDaemonPiHelperProcess(t *testing.T) {
	if os.Getenv("PI_LEARNLOOP_DAEMON_FAKE_PI") != "1" {
		return
	}
	arguments := argumentsAfterDoubleDash(os.Args)
	if len(arguments) == 1 && arguments[0] == "--version" {
		fmt.Fprintln(os.Stdout, "0.84.3")
		os.Exit(0)
	}
	if marker := os.Getenv("PI_LEARNLOOP_EVALUATOR_MARKER"); marker != "" {
		if err := os.WriteFile(marker, []byte("called"), 0o600); err != nil {
			os.Exit(93)
		}
	}

	lastAssistantText := `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"What behavior changed?","evidence_references":["E001"]},{"id":"Q2","kind":"code_specific","text":"Which boundary matters?","evidence_references":["E001"]},{"id":"Q3","kind":"go_backend","text":"How would a Go test cover this?","evidence_references":[]}]}`
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil {
			os.Exit(0)
		}
		var command map[string]any
		if json.Unmarshal(line, &command) != nil {
			os.Exit(91)
		}
		id, _ := command["id"].(string)
		kind, _ := command["type"].(string)
		switch kind {
		case "set_auto_retry", "set_auto_compaction", "prompt":
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": id, "type": "response", "command": kind, "success": true})
			if kind == "prompt" {
				message, _ := command["message"].(string)
				var envelope map[string]json.RawMessage
				if json.Unmarshal([]byte(message), &envelope) == nil {
					if _, isAssessment := envelope["stage"]; isAssessment {
						lastAssistantText = `{"schema_version":1,"disposition":"complete","follow_up":null,"evaluations":[{"question_id":"Q1","verdict":"demonstrated","feedback":"The answer identifies the selected behavior.","evidence_references":["E001"]},{"question_id":"Q2","verdict":"partial","feedback":"The answer omits one selected edge path.","evidence_references":["E001"]},{"question_id":"Q3","verdict":"not_demonstrated","feedback":"The answer needs a clearer testing explanation.","evidence_references":[]}]}`
					}
				}
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "agent_start"})
				_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"type": "agent_settled"})
			}
		case "get_commands":
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": id, "type": "response", "command": kind, "success": true, "data": map[string]any{"commands": []any{}}})
		case "get_last_assistant_text":
			_ = json.NewEncoder(os.Stdout).Encode(map[string]any{"id": id, "type": "response", "command": kind, "success": true, "data": map[string]any{"text": lastAssistantText}})
		default:
			os.Exit(92)
		}
	}
}

func installDaemonFakePi(t *testing.T) {
	t.Helper()
	binDirectory := t.TempDir()
	fakePi := filepath.Join(binDirectory, "pi")
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '0.84.3\\n'; exit 0; fi\nexec %q -test.run '^TestDaemonPiHelperProcess$' -- \"$@\"\n", os.Args[0])
	if err := os.WriteFile(fakePi, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(fake Pi): %v", err)
	}
	t.Setenv("PI_LEARNLOOP_DAEMON_FAKE_PI", "1")
	t.Setenv("PATH", binDirectory+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func argumentsAfterDoubleDash(arguments []string) []string {
	for index, argument := range arguments {
		if argument == "--" {
			return arguments[index+1:]
		}
	}
	return nil
}

func (running *runningDaemon) stop() {
	running.stopOnce.Do(func() {
		running.cancel()
		select {
		case err := <-running.errCh:
			if err != nil {
				running.t.Errorf("Run() shutdown error = %v", err)
			}
		case <-time.After(10 * time.Second):
			running.t.Error("Run() did not stop after cancellation")
		}
	})
}

func assertMode(t *testing.T, path string, want os.FileMode) {
	t.Helper()
	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat(%q): %v", path, err)
	}
	if got := info.Mode().Perm(); got != want {
		t.Fatalf("%s mode = %04o, want %04o", path, got, want)
	}
}

func assertNotExist(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Lstat(%q) error = %v, want not exist", path, err)
	}
}

func startDaemon(t *testing.T) (string, runtimeDescriptor) {
	t.Helper()
	stateDir := filepath.Join(t.TempDir(), "runtime")
	running := startDaemonAt(t, stateDir)
	return stateDir, running.descriptor
}

func localHTTPClient() *http.Client {
	return &http.Client{
		Transport: &http.Transport{Proxy: nil},
		Timeout:   5 * time.Second,
	}
}

func waitForDescriptor(t *testing.T, path string) runtimeDescriptor {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			var descriptor runtimeDescriptor
			if err := json.Unmarshal(content, &descriptor); err == nil {
				return descriptor
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime descriptor %q was not published", path)
	return runtimeDescriptor{}
}

func waitForNewDescriptor(t *testing.T, path, previousInstanceID string) runtimeDescriptor {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			var descriptor runtimeDescriptor
			if err := json.Unmarshal(content, &descriptor); err == nil && descriptor.InstanceID != "" && descriptor.InstanceID != previousInstanceID {
				return descriptor
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime descriptor %q did not replace instance %q", path, previousInstanceID)
	return runtimeDescriptor{}
}

func waitForFile(t *testing.T, path string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil {
			return content
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime file %q was not published", path)
	return nil
}

func waitForChangedFile(t *testing.T, path string, previous []byte) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(path)
		if err == nil && !bytes.Equal(content, previous) {
			return content
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("runtime file %q did not change", path)
	return nil
}

func quoted(value string) string {
	content, _ := json.Marshal(value)
	return string(content)
}

func daemonHistoryStart(root string) history.Start {
	questionPrompt := prompts.EvaluatorQuestionGenerationV1Metadata()
	assessmentPrompt := prompts.EvaluatorAnswerAssessmentV1Metadata()
	return history.Start{
		CanonicalRoot:           root,
		StartedAt:               time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC),
		BaseRevision:            strings.Repeat("a", 40),
		HeadRevision:            strings.Repeat("c", 40),
		EvidenceManifestSHA256:  strings.Repeat("b", 64),
		QuestionSchemaVersion:   evaluator.QuestionSetSchemaVersion,
		AssessmentSchemaVersion: evaluator.AssessmentTurnSchemaVersion,
		QuestionPrompt: history.PromptProvenance{
			ID: questionPrompt.ID, Version: questionPrompt.Version, SHA256: questionPrompt.SHA256,
		},
		AssessmentPrompt: history.PromptProvenance{
			ID: assessmentPrompt.ID, Version: assessmentPrompt.Version, SHA256: assessmentPrompt.SHA256,
		},
		PiVersion:     evaluator.SupportedPiVersion,
		Provider:      "provider",
		ModelID:       "model",
		ThinkingLevel: "off",
	}
}

func changedRepository(t *testing.T) (string, string, string) {
	t.Helper()
	repository := newRepository(t)
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Answer() int { return 1 }\n")
	commitAll(t, repository, "base")
	base := revision(t, repository, "HEAD")
	writeRepositoryFile(t, repository, "sample.go", "package sample\n\nfunc Answer() int { return 2 }\n")
	commitAll(t, repository, "head")
	return repository, base, revision(t, repository, "HEAD")
}

func assertErrorCode(t *testing.T, body io.Reader, want string) {
	t.Helper()
	var response struct {
		ProtocolVersion int `json:"protocol_version"`
		Error           struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.NewDecoder(body).Decode(&response); err != nil {
		t.Fatalf("decode error response: %v", err)
	}
	if response.ProtocolVersion != 1 || response.Error.Code != want {
		t.Fatalf("error response = %#v, want protocol 1 and code %q", response, want)
	}
}

func newRepository(t *testing.T) string {
	t.Helper()
	repository := t.TempDir()
	runGit(t, repository, "init", "-q")
	runGit(t, repository, "config", "user.name", "Pi LearnLoop Test")
	runGit(t, repository, "config", "user.email", "test@example.invalid")
	return repository
}

func writeRepositoryFile(t *testing.T, repository, name, content string) {
	t.Helper()
	path := filepath.Join(repository, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func commitAll(t *testing.T, repository, message string) {
	t.Helper()
	runGit(t, repository, "add", "--all")
	runGit(t, repository, "commit", "-q", "-m", message)
}

func revision(t *testing.T, repository, name string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, repository, "rev-parse", "--verify", name))
}

func runGit(t *testing.T, repository string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", arguments...)
	command.Dir = repository
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", arguments, err, output)
	}
	return string(output)
}
