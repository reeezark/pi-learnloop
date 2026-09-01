package daemon_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestDaemonServesDurableHistoryWithoutStartingAnEvaluator(t *testing.T) {
	repository := newRepository(t)
	canonicalRoot, err := filepath.EvalSymlinks(repository)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repository, err)
	}
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("history.Open(): %v", err)
	}
	start := daemonHistoryStart(canonicalRoot)
	recordID, err := store.Create(context.Background(), start)
	if err != nil {
		t.Fatalf("Create(): %v", err)
	}
	if err := store.Complete(context.Background(), recordID, history.Completion{
		FinishedAt: start.StartedAt.Add(time.Minute),
		Label:      history.LabelPartial,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictPartial},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictNotDemonstrated},
		},
	}); err != nil {
		t.Fatalf("Complete(): %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close(): %v", err)
	}

	evaluatorMarker := filepath.Join(t.TempDir(), "evaluator-called")
	t.Setenv("PI_LEARNLOOP_EVALUATOR_MARKER", evaluatorMarker)
	stateDir := filepath.Join(t.TempDir(), "runtime")
	running := startDaemonAtWithData(t, stateDir, dataDir)
	token := string(waitForFile(t, filepath.Join(stateDir, "daemon.token")))
	body, err := json.Marshal(map[string]any{"repository": repository, "limit": 20})
	if err != nil {
		t.Fatalf("Marshal(request): %v", err)
	}
	request, err := http.NewRequest(http.MethodPost, running.descriptor.BaseURL+"/v1/learning-history-queries", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("NewRequest(): %v", err)
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "PiLearnLoop "+token)
	response, err := localHTTPClient().Do(request)
	if err != nil {
		t.Fatalf("history query: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	var result struct {
		Records []struct {
			RecordID string `json:"record_id"`
			Label    string `json:"label"`
		} `json:"records"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("Decode(response): %v", err)
	}
	if len(result.Records) != 1 || result.Records[0].RecordID != recordID || result.Records[0].Label != "partial" {
		t.Fatalf("records = %#v, want persisted partial record %q", result.Records, recordID)
	}
	if _, err := os.Stat(evaluatorMarker); !os.IsNotExist(err) {
		t.Fatalf("history query started an evaluator: Stat() error = %v", err)
	}
}
