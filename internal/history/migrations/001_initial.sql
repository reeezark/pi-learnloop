CREATE TABLE repositories (
    id INTEGER PRIMARY KEY,
    canonical_root TEXT NOT NULL UNIQUE,
    created_at_unix_ms INTEGER NOT NULL CHECK (created_at_unix_ms > 0)
) STRICT;

CREATE TABLE learning_attempts (
    record_id TEXT PRIMARY KEY,
    repository_id INTEGER NOT NULL REFERENCES repositories(id),
    started_at_unix_ms INTEGER NOT NULL CHECK (started_at_unix_ms > 0),
    finished_at_unix_ms INTEGER,
    status TEXT NOT NULL CHECK (status IN ('running', 'complete', 'failed', 'interrupted')),
    failure_code TEXT,
    base_revision TEXT NOT NULL,
    head_revision TEXT NOT NULL,
    evidence_manifest_sha256 TEXT NOT NULL,
    question_schema_version INTEGER NOT NULL CHECK (question_schema_version > 0),
    assessment_schema_version INTEGER NOT NULL CHECK (assessment_schema_version > 0),
    question_prompt_id TEXT NOT NULL,
    question_prompt_version TEXT NOT NULL,
    question_prompt_sha256 TEXT NOT NULL,
    assessment_prompt_id TEXT NOT NULL,
    assessment_prompt_version TEXT NOT NULL,
    assessment_prompt_sha256 TEXT NOT NULL,
    pi_version TEXT NOT NULL,
    provider TEXT NOT NULL,
    model_id TEXT NOT NULL,
    thinking_level TEXT NOT NULL,
    follow_up_used INTEGER NOT NULL CHECK (follow_up_used IN (0, 1)),
    label TEXT CHECK (label IN ('understood', 'partial', 'review_needed')),
    CHECK (
        (status = 'running' AND finished_at_unix_ms IS NULL AND failure_code IS NULL AND label IS NULL) OR
        (status = 'complete' AND finished_at_unix_ms IS NOT NULL AND failure_code IS NULL AND label IS NOT NULL) OR
        (status = 'failed' AND finished_at_unix_ms IS NOT NULL AND failure_code IS NOT NULL AND label IS NULL) OR
        (status = 'interrupted' AND finished_at_unix_ms IS NOT NULL AND failure_code IS NULL AND label IS NULL)
    )
) STRICT;

CREATE TABLE question_outcomes (
    record_id TEXT NOT NULL REFERENCES learning_attempts(record_id) ON DELETE RESTRICT,
    question_id TEXT NOT NULL CHECK (question_id IN ('Q1', 'Q2', 'Q3')),
    question_kind TEXT NOT NULL CHECK (question_kind IN ('code_specific', 'go_backend')),
    verdict TEXT NOT NULL CHECK (verdict IN ('demonstrated', 'partial', 'not_demonstrated')),
    PRIMARY KEY (record_id, question_id)
) STRICT;

CREATE INDEX learning_attempts_repository_started_idx
    ON learning_attempts(repository_id, started_at_unix_ms DESC, record_id DESC);
