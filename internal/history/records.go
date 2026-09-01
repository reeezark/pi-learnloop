package history

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"time"
)

func (store *Store) Create(ctx context.Context, start Start) (string, error) {
	if ctx == nil {
		return "", fmt.Errorf("%w: context is nil", ErrInvalid)
	}
	if err := validateStart(start); err != nil {
		return "", err
	}
	recordID, err := newRecordID()
	if err != nil {
		return "", fmt.Errorf("generate history record ID: %w", err)
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return "", ErrClosed
	}
	err = immediate(ctx, store.conn, func() error {
		if _, err := store.conn.ExecContext(ctx, `
			INSERT INTO repositories(canonical_root, created_at_unix_ms)
			VALUES (?, ?)
			ON CONFLICT(canonical_root) DO NOTHING`, start.CanonicalRoot, start.StartedAt.UnixMilli()); err != nil {
			return err
		}
		var repositoryID int64
		if err := store.conn.QueryRowContext(ctx, "SELECT id FROM repositories WHERE canonical_root = ?", start.CanonicalRoot).Scan(&repositoryID); err != nil {
			return err
		}
		_, err := store.conn.ExecContext(ctx, `
			INSERT INTO learning_attempts(
				record_id, repository_id, started_at_unix_ms, status,
				base_revision, head_revision, evidence_manifest_sha256,
				question_schema_version, assessment_schema_version,
				question_prompt_id, question_prompt_version, question_prompt_sha256,
				assessment_prompt_id, assessment_prompt_version, assessment_prompt_sha256,
				pi_version, provider, model_id, thinking_level, follow_up_used
			) VALUES (?, ?, ?, 'running', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`,
			recordID, repositoryID, start.StartedAt.UnixMilli(),
			start.BaseRevision, start.HeadRevision, start.EvidenceManifestSHA256,
			start.QuestionSchemaVersion, start.AssessmentSchemaVersion,
			start.QuestionPrompt.ID, start.QuestionPrompt.Version, start.QuestionPrompt.SHA256,
			start.AssessmentPrompt.ID, start.AssessmentPrompt.Version, start.AssessmentPrompt.SHA256,
			start.PiVersion, start.Provider, start.ModelID, start.ThinkingLevel)
		return err
	})
	if err != nil {
		return "", fmt.Errorf("create history record: %w", err)
	}
	return recordID, nil
}

func (store *Store) MarkFollowUp(ctx context.Context, recordID string) error {
	if ctx == nil || !ValidRecordID(recordID) {
		return fmt.Errorf("%w: record identity is invalid", ErrInvalid)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return ErrClosed
	}
	result, err := store.conn.ExecContext(ctx, "UPDATE learning_attempts SET follow_up_used = 1 WHERE record_id = ? AND status = 'running'", recordID)
	if err != nil {
		return fmt.Errorf("mark history follow-up: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("mark history follow-up: %w", err)
	} else if affected == 1 {
		return nil
	}
	status, err := store.status(ctx, recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read history status: %w", err)
	}
	if status == StatusRunning {
		return nil
	}
	return ErrConflict
}

func (store *Store) Complete(ctx context.Context, recordID string, completion Completion) error {
	if ctx == nil || !ValidRecordID(recordID) {
		return fmt.Errorf("%w: record identity is invalid", ErrInvalid)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return ErrClosed
	}
	start, status, err := store.startAndStatus(ctx, recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read history record: %w", err)
	}
	if err := validateCompletion(start.StartedAt, completion); err != nil {
		return err
	}
	if status == StatusComplete {
		same, err := store.sameCompletion(ctx, recordID, completion)
		if err != nil {
			return fmt.Errorf("compare history completion: %w", err)
		}
		if same {
			return nil
		}
		return ErrConflict
	}
	if status != StatusRunning {
		return ErrConflict
	}
	err = immediate(ctx, store.conn, func() error {
		result, err := store.conn.ExecContext(ctx, `
			UPDATE learning_attempts
			SET finished_at_unix_ms = ?, status = 'complete', label = ?
			WHERE record_id = ? AND status = 'running'`, completion.FinishedAt.UnixMilli(), completion.Label, recordID)
		if err != nil {
			return err
		}
		affected, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if affected != 1 {
			return ErrConflict
		}
		for _, outcome := range completion.Outcomes {
			if _, err := store.conn.ExecContext(ctx, `
				INSERT INTO question_outcomes(record_id, question_id, question_kind, verdict)
				VALUES (?, ?, ?, ?)`, recordID, outcome.QuestionID, outcome.QuestionKind, outcome.Verdict); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("complete history record: %w", err)
	}
	return nil
}

func (store *Store) Fail(ctx context.Context, recordID string, failure Failure) error {
	if ctx == nil || !ValidRecordID(recordID) {
		return fmt.Errorf("%w: record identity is invalid", ErrInvalid)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return ErrClosed
	}
	start, status, err := store.startAndStatus(ctx, recordID)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("read history record: %w", err)
	}
	if err := validateFailure(start.StartedAt, failure); err != nil {
		return err
	}
	if status == StatusFailed {
		var finishedAt int64
		var code FailureCode
		if err := store.conn.QueryRowContext(ctx, "SELECT finished_at_unix_ms, failure_code FROM learning_attempts WHERE record_id = ?", recordID).Scan(&finishedAt, &code); err != nil {
			return fmt.Errorf("compare history failure: %w", err)
		}
		if finishedAt == failure.FinishedAt.UnixMilli() && code == failure.Code {
			return nil
		}
		return ErrConflict
	}
	if status != StatusRunning {
		return ErrConflict
	}
	result, err := store.conn.ExecContext(ctx, `
		UPDATE learning_attempts
		SET finished_at_unix_ms = ?, status = 'failed', failure_code = ?
		WHERE record_id = ? AND status = 'running'`, failure.FinishedAt.UnixMilli(), failure.Code, recordID)
	if err != nil {
		return fmt.Errorf("fail history record: %w", err)
	}
	if affected, err := result.RowsAffected(); err != nil {
		return fmt.Errorf("fail history record: %w", err)
	} else if affected != 1 {
		return ErrConflict
	}
	return nil
}

func (store *Store) List(ctx context.Context, canonicalRoot string, limit int) ([]Record, error) {
	if ctx == nil || canonicalRoot == "" || !validCanonicalRoot(canonicalRoot) || limit < 1 || limit > 50 {
		return nil, fmt.Errorf("%w: history query is invalid", ErrInvalid)
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil, ErrClosed
	}
	records, err := store.queryRecords(ctx, `
		SELECT
			a.record_id, r.canonical_root, a.started_at_unix_ms, a.finished_at_unix_ms,
			a.status, a.failure_code, a.base_revision, a.head_revision,
			a.evidence_manifest_sha256, a.question_schema_version, a.assessment_schema_version,
			a.question_prompt_id, a.question_prompt_version, a.question_prompt_sha256,
			a.assessment_prompt_id, a.assessment_prompt_version, a.assessment_prompt_sha256,
			a.pi_version, a.provider, a.model_id, a.thinking_level, a.follow_up_used, a.label
		FROM learning_attempts a
		JOIN repositories r ON r.id = a.repository_id
		WHERE r.canonical_root = ?
		ORDER BY a.started_at_unix_ms DESC, a.record_id DESC
		LIMIT ?`, canonicalRoot, limit)
	if err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	return records, nil
}

func (store *Store) validateStoredRecords(ctx context.Context) error {
	_, err := store.queryRecords(ctx, `
		SELECT
			a.record_id, r.canonical_root, a.started_at_unix_ms, a.finished_at_unix_ms,
			a.status, a.failure_code, a.base_revision, a.head_revision,
			a.evidence_manifest_sha256, a.question_schema_version, a.assessment_schema_version,
			a.question_prompt_id, a.question_prompt_version, a.question_prompt_sha256,
			a.assessment_prompt_id, a.assessment_prompt_version, a.assessment_prompt_sha256,
			a.pi_version, a.provider, a.model_id, a.thinking_level, a.follow_up_used, a.label
		FROM learning_attempts a
		JOIN repositories r ON r.id = a.repository_id
		ORDER BY a.started_at_unix_ms, a.record_id`)
	return err
}

func (store *Store) queryRecords(ctx context.Context, query string, arguments ...any) ([]Record, error) {
	rows, err := store.conn.QueryContext(ctx, query, arguments...)
	if err != nil {
		return nil, err
	}
	var records []Record
	for rows.Next() {
		var record Record
		var startedAt int64
		var finishedAt sql.NullInt64
		var failureCode sql.NullString
		var label sql.NullString
		var followUpUsed int
		if err := rows.Scan(
			&record.RecordID, &record.Start.CanonicalRoot, &startedAt, &finishedAt,
			&record.Status, &failureCode, &record.Start.BaseRevision, &record.Start.HeadRevision,
			&record.Start.EvidenceManifestSHA256, &record.Start.QuestionSchemaVersion, &record.Start.AssessmentSchemaVersion,
			&record.Start.QuestionPrompt.ID, &record.Start.QuestionPrompt.Version, &record.Start.QuestionPrompt.SHA256,
			&record.Start.AssessmentPrompt.ID, &record.Start.AssessmentPrompt.Version, &record.Start.AssessmentPrompt.SHA256,
			&record.Start.PiVersion, &record.Start.Provider, &record.Start.ModelID, &record.Start.ThinkingLevel, &followUpUsed, &label,
		); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan history: %w", err)
		}
		record.Start.StartedAt = time.UnixMilli(startedAt).UTC()
		if finishedAt.Valid {
			value := time.UnixMilli(finishedAt.Int64).UTC()
			record.FinishedAt = &value
		}
		if failureCode.Valid {
			record.FailureCode = FailureCode(failureCode.String)
		}
		if label.Valid {
			record.Label = Label(label.String)
		}
		if followUpUsed != 0 && followUpUsed != 1 {
			rows.Close()
			return nil, fmt.Errorf("%w: stored follow-up marker is invalid", ErrInvalid)
		}
		record.FollowUpUsed = followUpUsed == 1
		records = append(records, record)
	}
	if err := rows.Close(); err != nil {
		return nil, fmt.Errorf("close history query: %w", err)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("query history: %w", err)
	}
	for index := range records {
		outcomes, err := store.outcomes(ctx, records[index].RecordID)
		if err != nil {
			return nil, fmt.Errorf("query history outcomes: %w", err)
		}
		records[index].Outcomes = outcomes
		if err := validateRecord(records[index]); err != nil {
			return nil, err
		}
	}
	return records, nil
}

func (store *Store) status(ctx context.Context, recordID string) (Status, error) {
	var status Status
	err := store.conn.QueryRowContext(ctx, "SELECT status FROM learning_attempts WHERE record_id = ?", recordID).Scan(&status)
	return status, err
}

func (store *Store) startAndStatus(ctx context.Context, recordID string) (Start, Status, error) {
	var start Start
	var status Status
	var startedAt int64
	err := store.conn.QueryRowContext(ctx, `
		SELECT r.canonical_root, a.started_at_unix_ms, a.status
		FROM learning_attempts a JOIN repositories r ON r.id = a.repository_id
		WHERE a.record_id = ?`, recordID).Scan(&start.CanonicalRoot, &startedAt, &status)
	start.StartedAt = time.UnixMilli(startedAt).UTC()
	return start, status, err
}

func (store *Store) sameCompletion(ctx context.Context, recordID string, completion Completion) (bool, error) {
	var finishedAt int64
	var label Label
	if err := store.conn.QueryRowContext(ctx, "SELECT finished_at_unix_ms, label FROM learning_attempts WHERE record_id = ?", recordID).Scan(&finishedAt, &label); err != nil {
		return false, err
	}
	if finishedAt != completion.FinishedAt.UnixMilli() || label != completion.Label {
		return false, nil
	}
	outcomes, err := store.outcomes(ctx, recordID)
	if err != nil {
		return false, err
	}
	if len(outcomes) != len(completion.Outcomes) {
		return false, nil
	}
	for index := range outcomes {
		if outcomes[index] != completion.Outcomes[index] {
			return false, nil
		}
	}
	return true, nil
}

func (store *Store) outcomes(ctx context.Context, recordID string) ([]Outcome, error) {
	rows, err := store.conn.QueryContext(ctx, `
		SELECT question_id, question_kind, verdict
		FROM question_outcomes WHERE record_id = ? ORDER BY question_id`, recordID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var outcomes []Outcome
	for rows.Next() {
		var outcome Outcome
		if err := rows.Scan(&outcome.QuestionID, &outcome.QuestionKind, &outcome.Verdict); err != nil {
			return nil, err
		}
		outcomes = append(outcomes, outcome)
	}
	return outcomes, rows.Err()
}

func immediate(ctx context.Context, conn *sql.Conn, action func() error) (err error) {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if err := action(); err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return err
	}
	committed = true
	return nil
}

func newRecordID() (string, error) {
	random := make([]byte, recordIDRandomBytes)
	if _, err := rand.Read(random); err != nil {
		return "", err
	}
	return recordIDPrefix + base64.RawURLEncoding.EncodeToString(random), nil
}
