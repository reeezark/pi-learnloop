package history_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/internal/history"
	_ "modernc.org/sqlite"
)

func TestValidPiSessionID(t *testing.T) {
	maximum := "a" + strings.Repeat("-", 126) + "Z"
	tests := []struct {
		value string
		want  bool
	}{
		{value: "a", want: true},
		{value: "01991d28-052a-72fe-bc0c-f028ce528348", want: true},
		{value: "Session_1.alpha-beta", want: true},
		{value: maximum, want: true},
		{value: "", want: false},
		{value: strings.Repeat("a", 129), want: false},
		{value: "-session", want: false},
		{value: "session_", want: false},
		{value: ".session", want: false},
		{value: "session.", want: false},
		{value: "session/id", want: false},
		{value: "session id", want: false},
		{value: "session\nsecret", want: false},
		{value: "sessiön", want: false},
	}
	for _, test := range tests {
		if got := history.ValidPiSessionID(test.value); got != test.want {
			t.Errorf("ValidPiSessionID(%q) = %v, want %v", test.value, got, test.want)
		}
	}
}

func TestCreateStoresOnlyOptionalPiSessionID(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	start := validStart(repositoryRoot)
	gitOnlyID, err := store.Create(ctx, start)
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	piSessionID := "a" + strings.Repeat("-", 126) + "Z"
	sessionBoundID, err := store.CreateWithPiSession(ctx, start, piSessionID)
	if err != nil {
		t.Fatalf("CreateWithPiSession() error = %v", err)
	}
	records, err := store.List(ctx, repositoryRoot, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	encoded, err := json.Marshal(records)
	if err != nil {
		t.Fatalf("json.Marshal(records) error = %v", err)
	}
	if strings.Contains(string(encoded), piSessionID) {
		t.Fatal("generic history records exposed Pi Session identity")
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	database, err := sql.Open("sqlite", filepath.Join(dataDir, "history.db"))
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	defer database.Close()
	var gitOnly sql.NullString
	if err := database.QueryRow("SELECT pi_session_id FROM learning_attempts WHERE record_id = ?", gitOnlyID).Scan(&gitOnly); err != nil {
		t.Fatalf("query Git-only provenance: %v", err)
	}
	if gitOnly.Valid {
		t.Fatalf("Git-only pi_session_id = %q, want SQL NULL", gitOnly.String)
	}
	var stored string
	if err := database.QueryRow("SELECT pi_session_id FROM learning_attempts WHERE record_id = ?", sessionBoundID).Scan(&stored); err != nil {
		t.Fatalf("query Session provenance: %v", err)
	}
	if stored != piSessionID {
		t.Fatalf("stored pi_session_id = %q, want selected ID", stored)
	}
}

func TestCreateWithPiSessionRejectsInvalidIDWithoutEcho(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()
	start := validStart(filepath.Join(t.TempDir(), "repository"))
	invalid := "session/secret"
	if recordID, err := store.CreateWithPiSession(ctx, start, invalid); recordID != "" || !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("CreateWithPiSession(invalid) = (%q, %v), want empty ErrInvalid", recordID, err)
	} else if strings.Contains(err.Error(), invalid) {
		t.Fatal("invalid Pi Session error echoed the rejected ID")
	}
	if recordID, err := store.CreateWithPiSession(nil, start, "valid-session"); recordID != "" || !errors.Is(err, history.ErrInvalid) {
		t.Fatalf("CreateWithPiSession(nil context) = (%q, %v), want empty ErrInvalid", recordID, err)
	}
	records, err := store.List(ctx, start.CanonicalRoot, 10)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(records) != 0 {
		t.Fatalf("invalid Session starts persisted records: %#v", records)
	}
}

func TestReviewedPiSessionIDsUsesCompletionAndRepositoryOnly(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}

	interruptedStart := validStart(repositoryRoot)
	if _, err := store.CreateWithPiSession(ctx, interruptedStart, "interrupted-session"); err != nil {
		t.Fatalf("CreateWithPiSession(interrupted) error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	store, err = history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open(existing store) error = %v", err)
	}
	defer store.Close()

	completePiSession(t, store, validStart(repositoryRoot), "complete-a")
	completePiSession(t, store, validStart(repositoryRoot), "complete-b")

	failedStart := validStart(repositoryRoot)
	failedID, err := store.CreateWithPiSession(ctx, failedStart, "failed-session")
	if err != nil {
		t.Fatalf("CreateWithPiSession(failed) error = %v", err)
	}
	if err := store.Fail(ctx, failedID, history.Failure{
		FinishedAt: failedStart.StartedAt.Add(time.Minute),
		Code:       history.FailureEvaluatorTimeout,
	}); err != nil {
		t.Fatalf("Fail() error = %v", err)
	}
	if _, err := store.CreateWithPiSession(ctx, validStart(repositoryRoot), "running-session"); err != nil {
		t.Fatalf("CreateWithPiSession(running) error = %v", err)
	}

	gitOnlyStart := validStart(repositoryRoot)
	gitOnlyID, err := store.Create(ctx, gitOnlyStart)
	if err != nil {
		t.Fatalf("Create(Git-only) error = %v", err)
	}
	if err := store.Complete(ctx, gitOnlyID, validCompletion(gitOnlyStart)); err != nil {
		t.Fatalf("Complete(Git-only) error = %v", err)
	}

	otherRoot := filepath.Join(t.TempDir(), "other-repository")
	completePiSession(t, store, validStart(otherRoot), "other-only")

	candidates := []string{
		"failed-session", "complete-b", "running-session", "complete-a",
		"interrupted-session", "other-only", "no-record",
	}
	got, err := store.ReviewedPiSessionIDs(ctx, repositoryRoot, candidates)
	if err != nil {
		t.Fatalf("ReviewedPiSessionIDs() error = %v", err)
	}
	want := []string{"complete-b", "complete-a"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ReviewedPiSessionIDs() = %#v, want candidate-order completed subset %#v", got, want)
	}

	empty, err := store.ReviewedPiSessionIDs(ctx, repositoryRoot, []string{"no-record"})
	if err != nil {
		t.Fatalf("ReviewedPiSessionIDs(no match) error = %v", err)
	}
	if empty == nil || len(empty) != 0 {
		t.Fatalf("ReviewedPiSessionIDs(no match) = %#v, want non-nil empty result", empty)
	}
}

func TestReviewedPiSessionIDsValidatesBoundsAndIdentity(t *testing.T) {
	ctx := context.Background()
	store, err := history.Open(ctx, filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	repositoryRoot := filepath.Join(t.TempDir(), "repository")
	twenty := make([]string, 20)
	for index := range twenty {
		twenty[index] = fmt.Sprintf("session-%02d", index)
	}
	if got, err := store.ReviewedPiSessionIDs(ctx, repositoryRoot, twenty); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("ReviewedPiSessionIDs(20) = (%#v, %v), want non-nil empty result", got, err)
	}
	invalidQueries := []struct {
		name       string
		ctx        context.Context
		root       string
		candidates []string
	}{
		{name: "nil context", ctx: nil, root: repositoryRoot, candidates: []string{"session-1"}},
		{name: "relative repository", ctx: ctx, root: "relative/repository", candidates: []string{"session-1"}},
		{name: "empty", ctx: ctx, root: repositoryRoot, candidates: nil},
		{name: "more than 20", ctx: ctx, root: repositoryRoot, candidates: append(twenty, "session-20")},
		{name: "duplicate", ctx: ctx, root: repositoryRoot, candidates: []string{"session-1", "session-1"}},
		{name: "invalid ID", ctx: ctx, root: repositoryRoot, candidates: []string{"session/secret"}},
	}
	for _, test := range invalidQueries {
		t.Run(test.name, func(t *testing.T) {
			if got, err := store.ReviewedPiSessionIDs(test.ctx, test.root, test.candidates); got != nil || !errors.Is(err, history.ErrInvalid) {
				t.Fatalf("ReviewedPiSessionIDs(invalid) = (%#v, %v), want nil ErrInvalid", got, err)
			}
		})
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	if got, err := store.ReviewedPiSessionIDs(ctx, repositoryRoot, []string{"session-1"}); got != nil || !errors.Is(err, history.ErrClosed) {
		t.Fatalf("ReviewedPiSessionIDs(closed) = (%#v, %v), want nil ErrClosed", got, err)
	}
}

func completePiSession(t *testing.T, store *history.Store, start history.Start, piSessionID string) {
	t.Helper()
	recordID, err := store.CreateWithPiSession(context.Background(), start, piSessionID)
	if err != nil {
		t.Fatalf("CreateWithPiSession(%q) error = %v", piSessionID, err)
	}
	if err := store.Complete(context.Background(), recordID, validCompletion(start)); err != nil {
		t.Fatalf("Complete(%q) error = %v", piSessionID, err)
	}
}

func validCompletion(start history.Start) history.Completion {
	return history.Completion{
		FinishedAt: start.StartedAt.Add(time.Minute),
		Label:      history.LabelUnderstood,
		Outcomes: []history.Outcome{
			{QuestionID: "Q1", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q2", QuestionKind: history.QuestionKindCodeSpecific, Verdict: history.VerdictDemonstrated},
			{QuestionID: "Q3", QuestionKind: history.QuestionKindGoBackend, Verdict: history.VerdictDemonstrated},
		},
	}
}
