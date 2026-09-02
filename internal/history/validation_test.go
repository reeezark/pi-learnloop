package history_test

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
)

func TestCreateRejectsInvalidSourceFreeProvenance(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	tests := []struct {
		name   string
		mutate func(*history.Start)
	}{
		{name: "relative repository", mutate: func(value *history.Start) { value.CanonicalRoot = "relative/repository" }},
		{name: "unclean repository", mutate: func(value *history.Start) { value.CanonicalRoot += "/../repository" }},
		{name: "sub-millisecond start", mutate: func(value *history.Start) { value.StartedAt = value.StartedAt.Add(time.Nanosecond) }},
		{name: "empty base revision", mutate: func(value *history.Start) { value.BaseRevision = "" }},
		{name: "uppercase manifest hash", mutate: func(value *history.Start) {
			value.EvidenceManifestSHA256 = strings.ToUpper(value.EvidenceManifestSHA256)
		}},
		{name: "zero schema version", mutate: func(value *history.Start) { value.QuestionSchemaVersion = 0 }},
		{name: "unsafe prompt id", mutate: func(value *history.Start) { value.QuestionPrompt.ID = "prompt\nbody" }},
		{name: "invalid prompt hash", mutate: func(value *history.Start) { value.AssessmentPrompt.SHA256 = "not-a-hash" }},
		{name: "unsupported Pi", mutate: func(value *history.Start) { value.PiVersion = "latest" }},
		{name: "argument-like provider", mutate: func(value *history.Start) { value.Provider = "--unsafe" }},
		{name: "control in model", mutate: func(value *history.Start) { value.ModelID = "model\x00secret" }},
		{name: "unknown thinking", mutate: func(value *history.Start) { value.ThinkingLevel = "ultra" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			start := validStart(repositoryRoot)
			test.mutate(&start)
			if recordID, err := store.Create(ctx, start); recordID != "" || !errors.Is(err, history.ErrInvalid) {
				t.Fatalf("Create(invalid start) = (%q, %v), want empty ErrInvalid", recordID, err)
			}
		})
	}
	records, err := store.List(ctx, repositoryRoot, 50)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("invalid Create calls persisted records: %#v", records)
	}
}

func TestTerminalValidationLeavesAttemptRunning(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	start := validStart(filepath.Join(t.TempDir(), "repository"))
	recordID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	invalid := history.Completion{
		FinishedAt: start.StartedAt.Add(-time.Second),
		Label:      "excellent",
		Outcomes:   []history.Outcome{{QuestionID: "Q1", QuestionKind: history.QuestionKindGoBackend, Verdict: "correct"}},
	}
	if err := store.Complete(ctx, recordID, invalid); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("Complete(invalid) error = %v, want ErrInvalid", err)
	}
	if err := store.Fail(ctx, recordID, history.Failure{FinishedAt: start.StartedAt.Add(time.Second), Code: "provider_error_text"}); !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("Fail(invalid) error = %v, want ErrInvalid", err)
	}
	records, err := store.List(ctx, start.CanonicalRoot, 1)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 1 || records[0].Status != history.StatusRunning || records[0].FinishedAt != nil {
		t.Fatalf("record after invalid terminal calls = %#v, want running", records)
	}
}

func TestSchemaAndPublicValuesContainOnlyApprovedHistoryFields(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(context.Background(), dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	database, err := sql.Open("sqlite", filepath.Join(dataDir, "history.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer database.Close()
	wantColumns := map[string][]string{
		"repositories": {"id", "canonical_root", "created_at_unix_ms"},
		"learning_attempts": {
			"record_id", "repository_id", "started_at_unix_ms", "finished_at_unix_ms", "status", "failure_code",
			"base_revision", "head_revision", "evidence_manifest_sha256", "question_schema_version", "assessment_schema_version",
			"question_prompt_id", "question_prompt_version", "question_prompt_sha256",
			"assessment_prompt_id", "assessment_prompt_version", "assessment_prompt_sha256",
			"pi_version", "provider", "model_id", "thinking_level", "follow_up_used", "label", "pi_session_id",
		},
		"question_outcomes": {"record_id", "question_id", "question_kind", "verdict"},
	}
	for table, want := range wantColumns {
		rows, err := database.Query("PRAGMA table_info(" + table + ")")
		if err != nil {
			t.Fatalf("table_info(%s) error = %v", table, err)
		}
		var got []string
		for rows.Next() {
			var cid, notNull, primaryKey int
			var name, columnType string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
				rows.Close()
				t.Fatalf("scan table_info(%s): %v", table, err)
			}
			got = append(got, name)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close table_info(%s): %v", table, err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Fatalf("columns(%s) = %#v, want %#v", table, got, want)
		}
	}

	rows, err := database.Query("SELECT sql FROM sqlite_schema WHERE sql IS NOT NULL")
	if err != nil {
		t.Fatalf("query schema SQL: %v", err)
	}
	var schema strings.Builder
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			rows.Close()
			t.Fatalf("scan schema SQL: %v", err)
		}
		schema.WriteString(strings.ToLower(statement))
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("close schema query: %v", err)
	}
	for _, forbidden := range []string{
		"source_excerpt", "file_path", "question_text", "user_answer", "follow_up_text", "feedback",
		"prompt_body", "rpc_frame", "model_output", "credential", "instance_token", "session_transcript",
		"session_path", "session_cwd", "session_name", "session_timestamp", "session_message",
		"parent_session", "session_leaf", "session_prompt", "session_answer", "session_tool", "session_summary",
	} {
		if strings.Contains(schema.String(), forbidden) {
			t.Fatalf("schema contains forbidden field %q", forbidden)
		}
	}

	assertStructFields(t, history.Start{}, []string{
		"CanonicalRoot", "StartedAt", "BaseRevision", "HeadRevision", "EvidenceManifestSHA256",
		"QuestionSchemaVersion", "AssessmentSchemaVersion", "QuestionPrompt", "AssessmentPrompt",
		"PiVersion", "Provider", "ModelID", "ThinkingLevel",
	})
	assertStructFields(t, history.Completion{}, []string{"FinishedAt", "Label", "Outcomes"})
	assertStructFields(t, history.Outcome{}, []string{"QuestionID", "QuestionKind", "Verdict"})
	assertStructFields(t, history.Failure{}, []string{"FinishedAt", "Code"})
	assertStructFields(t, history.Record{}, []string{
		"RecordID", "Start", "FinishedAt", "Status", "FailureCode", "FollowUpUsed", "Label", "Outcomes",
	})

	if _, err := os.Stat(filepath.Join(dataDir, "history.db")); err != nil {
		t.Fatalf("history database missing: %v", err)
	}
}

func assertStructFields(t *testing.T, value any, want []string) {
	t.Helper()
	typeOf := reflect.TypeOf(value)
	got := make([]string, typeOf.NumField())
	for index := range got {
		got[index] = typeOf.Field(index).Name
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("fields(%s) = %#v, want %#v", typeOf.Name(), got, want)
	}
}
