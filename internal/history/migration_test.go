package history

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const versionOneAttemptColumns = `
	record_id, repository_id, started_at_unix_ms, finished_at_unix_ms, status, failure_code,
	base_revision, head_revision, evidence_manifest_sha256, question_schema_version, assessment_schema_version,
	question_prompt_id, question_prompt_version, question_prompt_sha256,
	assessment_prompt_id, assessment_prompt_version, assessment_prompt_sha256,
	pi_version, provider, model_id, thinking_level, follow_up_used, label`

func TestMigrationTwoPreservesEveryVersionOneValue(t *testing.T) {
	ctx := context.Background()
	database, conn := openMemoryConnection(t, ctx)
	defer database.Close()
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, migrationOne+"\nPRAGMA user_version = 1;"); err != nil {
		t.Fatalf("create schema v1: %v", err)
	}
	seedVersionOneHistory(t, ctx, conn, "/private/tmp/pi-learnloop-migration-repository")
	queries := []string{
		"SELECT id, canonical_root, created_at_unix_ms FROM repositories ORDER BY id",
		"SELECT " + versionOneAttemptColumns + " FROM learning_attempts ORDER BY record_id",
		"SELECT record_id, question_id, question_kind, verdict FROM question_outcomes ORDER BY record_id, question_id",
	}
	before := snapshotQueries(t, ctx, conn, queries)

	if err := applyMigration(ctx, conn, schemaMigration{version: 2, sql: migrationTwo}); err != nil {
		t.Fatalf("applyMigration(v2) error = %v", err)
	}
	after := snapshotQueries(t, ctx, conn, queries)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("v1 values changed during migration\nbefore: %#v\nafter:  %#v", before, after)
	}
	var version, total, nonNull int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read schema version: %v", err)
	}
	if err := conn.QueryRowContext(ctx, "SELECT COUNT(*), COUNT(pi_session_id) FROM learning_attempts").Scan(&total, &nonNull); err != nil {
		t.Fatalf("count migrated provenance: %v", err)
	}
	if version != 2 || total != 4 || nonNull != 0 {
		t.Fatalf("migrated state = version %d, rows %d, non-NULL IDs %d; want 2, 4, 0", version, total, nonNull)
	}
	if err := verifySchema(ctx, conn, 2); err != nil {
		t.Fatalf("verifySchema(v2) error = %v", err)
	}
}

func TestMigrationFailureRollsBackVersionAndSchema(t *testing.T) {
	ctx := context.Background()
	database, conn := openMemoryConnection(t, ctx)
	defer database.Close()
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, migrationOne+"\nPRAGMA user_version = 1;"); err != nil {
		t.Fatalf("create schema v1: %v", err)
	}
	seedVersionOneHistory(t, ctx, conn, "/private/tmp/pi-learnloop-rollback-repository")
	queries := []string{
		"SELECT id, canonical_root, created_at_unix_ms FROM repositories ORDER BY id",
		"SELECT " + versionOneAttemptColumns + " FROM learning_attempts ORDER BY record_id",
		"SELECT record_id, question_id, question_kind, verdict FROM question_outcomes ORDER BY record_id, question_id",
	}
	before := snapshotQueries(t, ctx, conn, queries)
	broken := schemaMigration{
		version: 2,
		sql:     migrationTwo + "\nINSERT INTO missing_migration_table(value) VALUES (1);",
	}
	if err := applyMigration(ctx, conn, broken); err == nil {
		t.Fatal("applyMigration(broken v2) error = nil, want rollback error")
	}
	if err := verifySchema(ctx, conn, 1); err != nil {
		t.Fatalf("schema after failed migration is not intact v1: %v", err)
	}
	after := snapshotQueries(t, ctx, conn, queries)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("v1 values changed after failed migration\nbefore: %#v\nafter:  %#v", before, after)
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA table_info(learning_attempts)")
	if err != nil {
		t.Fatalf("table_info after rollback: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, primaryKey int
		var name, columnType string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &columnType, &notNull, &defaultValue, &primaryKey); err != nil {
			t.Fatalf("scan table_info after rollback: %v", err)
		}
		if name == "pi_session_id" {
			t.Fatal("failed migration left pi_session_id behind")
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info after rollback: %v", err)
	}
}

func TestOpenMigratesVersionOneAndPreservesRecovery(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dataDir, dataDirectoryMode); err != nil {
		t.Fatalf("Mkdir(data directory) error = %v", err)
	}
	databasePath := filepath.Join(dataDir, "history.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	database.SetMaxOpenConns(1)
	conn, err := database.Conn(ctx)
	if err != nil {
		database.Close()
		t.Fatalf("database.Conn() error = %v", err)
	}
	if _, err := conn.ExecContext(ctx, migrationOne+"\nPRAGMA user_version = 1;"); err != nil {
		conn.Close()
		database.Close()
		t.Fatalf("create schema v1: %v", err)
	}
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	seedVersionOneHistory(t, ctx, conn, repositoryRoot)
	if err := conn.Close(); err != nil {
		database.Close()
		t.Fatalf("close v1 connection: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close v1 database: %v", err)
	}
	if err := os.Chmod(databasePath, databaseFileMode); err != nil {
		t.Fatalf("Chmod(v1 database) error = %v", err)
	}

	store, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open(v1 database) error = %v", err)
	}
	defer store.Close()
	var version, total, nonNull int
	if err := store.conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		t.Fatalf("read migrated version: %v", err)
	}
	if err := store.conn.QueryRowContext(ctx, "SELECT COUNT(*), COUNT(pi_session_id) FROM learning_attempts").Scan(&total, &nonNull); err != nil {
		t.Fatalf("count migrated attempts: %v", err)
	}
	if version != 2 || total != 4 || nonNull != 0 {
		t.Fatalf("Open(v1) state = version %d, rows %d, non-NULL IDs %d; want 2, 4, 0", version, total, nonNull)
	}
	records, err := store.List(ctx, repositoryRoot, 10)
	if err != nil {
		t.Fatalf("List(migrated v1) error = %v", err)
	}
	statusCounts := make(map[Status]int)
	for _, record := range records {
		statusCounts[record.Status]++
	}
	if len(records) != 4 || statusCounts[StatusComplete] != 1 || statusCounts[StatusFailed] != 1 || statusCounts[StatusInterrupted] != 2 {
		t.Fatalf("migrated recovery statuses = %#v across %#v", statusCounts, records)
	}
}

func TestSchemaConstraintRejectsInvalidPiSessionIDs(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	recordID, err := store.CreateWithPiSession(ctx, internalValidStart(filepath.Join(t.TempDir(), "repository")), "valid-session")
	if err != nil {
		t.Fatalf("CreateWithPiSession() error = %v", err)
	}
	invalid := []string{"", "-session", "session_", "session/id", "sessiön", strings.Repeat("a", 129), "session\x00hidden"}
	for _, value := range invalid {
		if _, err := store.conn.ExecContext(ctx, "UPDATE learning_attempts SET pi_session_id = ? WHERE record_id = ?", value, recordID); err == nil {
			t.Errorf("schema accepted invalid Pi Session ID %q", value)
		}
	}
	var stored string
	if err := store.conn.QueryRowContext(ctx, "SELECT pi_session_id FROM learning_attempts WHERE record_id = ?", recordID).Scan(&stored); err != nil {
		t.Fatalf("query retained Pi Session ID: %v", err)
	}
	if stored != "valid-session" {
		t.Fatalf("stored Pi Session ID after rejected updates = %q, want original", stored)
	}
}

func openMemoryConnection(t *testing.T, ctx context.Context) (*sql.DB, *sql.Conn) {
	t.Helper()
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("sql.Open(:memory:) error = %v", err)
	}
	database.SetMaxOpenConns(1)
	conn, err := database.Conn(ctx)
	if err != nil {
		database.Close()
		t.Fatalf("database.Conn() error = %v", err)
	}
	return database, conn
}

func seedVersionOneHistory(t *testing.T, ctx context.Context, conn *sql.Conn, repositoryRoot string) {
	t.Helper()
	const startedAt = int64(1788244800000)
	result, err := conn.ExecContext(ctx, "INSERT INTO repositories(canonical_root, created_at_unix_ms) VALUES (?, ?)", repositoryRoot, startedAt)
	if err != nil {
		t.Fatalf("insert v1 repository: %v", err)
	}
	repositoryID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("read v1 repository ID: %v", err)
	}
	records := []struct {
		id       string
		status   Status
		finished any
		failure  any
		label    any
	}{
		{id: deterministicRecordID(1), status: StatusRunning},
		{id: deterministicRecordID(2), status: StatusComplete, finished: startedAt + 1000, label: LabelUnderstood},
		{id: deterministicRecordID(3), status: StatusFailed, finished: startedAt + 2000, failure: FailureEvaluatorTimeout},
		{id: deterministicRecordID(4), status: StatusInterrupted, finished: startedAt + 3000},
	}
	for index, record := range records {
		_, err := conn.ExecContext(ctx, `
			INSERT INTO learning_attempts(`+versionOneAttemptColumns+`)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			record.id, repositoryID, startedAt+int64(index), record.finished, record.status, record.failure,
			"base-revision", "head-revision", strings.Repeat("a", 64), 1, 1,
			"question-prompt", "1.0.0", strings.Repeat("b", 64),
			"assessment-prompt", "1.0.0", strings.Repeat("c", 64),
			"0.84.3", "anthropic", "claude-sonnet-4", "medium", index%2, record.label)
		if err != nil {
			t.Fatalf("insert v1 %s record: %v", record.status, err)
		}
	}
	for index, outcome := range []struct {
		id      string
		kind    QuestionKind
		verdict Verdict
	}{
		{id: "Q1", kind: QuestionKindCodeSpecific, verdict: VerdictDemonstrated},
		{id: "Q2", kind: QuestionKindCodeSpecific, verdict: VerdictPartial},
		{id: "Q3", kind: QuestionKindGoBackend, verdict: VerdictNotDemonstrated},
	} {
		if _, err := conn.ExecContext(ctx, `
			INSERT INTO question_outcomes(record_id, question_id, question_kind, verdict)
			VALUES (?, ?, ?, ?)`, records[1].id, outcome.id, outcome.kind, outcome.verdict); err != nil {
			t.Fatalf("insert v1 outcome %d: %v", index, err)
		}
	}
}

func snapshotQueries(t *testing.T, ctx context.Context, conn *sql.Conn, queries []string) []string {
	t.Helper()
	var snapshot []string
	for queryIndex, query := range queries {
		rows, err := conn.QueryContext(ctx, query)
		if err != nil {
			t.Fatalf("snapshot query %d: %v", queryIndex, err)
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			t.Fatalf("snapshot columns %d: %v", queryIndex, err)
		}
		for rows.Next() {
			values := make([]sql.RawBytes, len(columns))
			destinations := make([]any, len(columns))
			for index := range values {
				destinations[index] = &values[index]
			}
			if err := rows.Scan(destinations...); err != nil {
				rows.Close()
				t.Fatalf("snapshot scan %d: %v", queryIndex, err)
			}
			encoded := make([]string, len(values))
			for index, value := range values {
				if value == nil {
					encoded[index] = "NULL"
				} else {
					encoded[index] = "VALUE:" + hex.EncodeToString(value)
				}
			}
			snapshot = append(snapshot, strings.Join(encoded, "|"))
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("snapshot close %d: %v", queryIndex, err)
		}
		if err := rows.Err(); err != nil {
			t.Fatalf("snapshot rows %d: %v", queryIndex, err)
		}
	}
	return snapshot
}

func deterministicRecordID(value byte) string {
	return recordIDPrefix + base64.RawURLEncoding.EncodeToString(bytes.Repeat([]byte{value}, recordIDRandomBytes))
}

func internalValidStart(repositoryRoot string) Start {
	return Start{
		CanonicalRoot:           repositoryRoot,
		StartedAt:               timeFromUnixMilli(1788244800000),
		BaseRevision:            "base-revision",
		HeadRevision:            "head-revision",
		EvidenceManifestSHA256:  strings.Repeat("a", 64),
		QuestionSchemaVersion:   1,
		AssessmentSchemaVersion: 1,
		QuestionPrompt:          PromptProvenance{ID: "question-prompt", Version: "1.0.0", SHA256: strings.Repeat("b", 64)},
		AssessmentPrompt:        PromptProvenance{ID: "assessment-prompt", Version: "1.0.0", SHA256: strings.Repeat("c", 64)},
		PiVersion:               "0.84.3",
		Provider:                "anthropic",
		ModelID:                 "claude-sonnet-4",
		ThinkingLevel:           "medium",
	}
}

func timeFromUnixMilli(value int64) time.Time {
	return time.UnixMilli(value).UTC()
}
