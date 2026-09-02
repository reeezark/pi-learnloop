package history

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type Store struct {
	mu     sync.Mutex
	db     *sql.DB
	conn   *sql.Conn
	closed bool
}

func Open(ctx context.Context, dataDir string) (*Store, error) {
	if ctx == nil {
		return nil, fmt.Errorf("%w: context is nil", ErrInvalid)
	}
	databasePath, err := prepareStorage(dataDir)
	if err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", databasePath)
	if err != nil {
		return nil, fmt.Errorf("open history database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	conn, err := db.Conn(ctx)
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("open history connection: %w", err)
	}
	store := &Store{db: db, conn: conn}
	if err := store.initialize(ctx); err != nil {
		conn.Close()
		db.Close()
		return nil, err
	}
	if err := validateProtectedPath(databasePath, false, databaseFileMode); err != nil {
		conn.Close()
		db.Close()
		return nil, err
	}
	return store, nil
}

func (store *Store) Close() error {
	if store == nil {
		return nil
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.closed {
		return nil
	}
	store.closed = true
	connErr := store.conn.Close()
	dbErr := store.db.Close()
	return errors.Join(connErr, dbErr)
}

func (store *Store) initialize(ctx context.Context) error {
	version, err := schemaVersion(ctx, store.conn)
	if err != nil {
		return fmt.Errorf("%w: read schema version: %v", ErrCorrupt, err)
	}
	if version > currentSchemaVersion {
		return fmt.Errorf("%w: database version %d is newer than supported version %d", ErrUnsupportedSchema, version, currentSchemaVersion)
	}
	if err := checkIntegrity(ctx, store.conn); err != nil {
		return err
	}
	if version == 0 {
		if err := verifyEmptySchema(ctx, store.conn); err != nil {
			return err
		}
	} else if err := verifySchema(ctx, store.conn, version); err != nil {
		return err
	}
	for version < currentSchemaVersion {
		migration, ok := migrationForVersion(version + 1)
		if !ok {
			return fmt.Errorf("%w: schema migration %d is unavailable", ErrCorrupt, version+1)
		}
		if err := applyMigration(ctx, store.conn, migration); err != nil {
			return err
		}
		version = migration.version
		if err := verifySchema(ctx, store.conn, version); err != nil {
			return err
		}
	}
	if err := validateStoredRepositories(ctx, store.conn); err != nil {
		return fmt.Errorf("%w: stored repository validation failed: %v", ErrCorrupt, err)
	}
	if err := validateStoredPiSessionIDs(ctx, store.conn); err != nil {
		return fmt.Errorf("%w: stored Pi Session validation failed: %v", ErrCorrupt, err)
	}
	if err := store.validateStoredRecords(ctx); err != nil {
		return fmt.Errorf("%w: stored record validation failed: %v", ErrCorrupt, err)
	}
	if err := configureConnection(ctx, store.conn); err != nil {
		return err
	}
	if err := checkIntegrity(ctx, store.conn); err != nil {
		return err
	}
	if err := store.recoverInterrupted(ctx); err != nil {
		return err
	}
	return nil
}

func migrationForVersion(version int) (schemaMigration, bool) {
	for _, migration := range schemaMigrations {
		if migration.version == version {
			return migration, true
		}
	}
	return schemaMigration{}, false
}

func validateStoredRepositories(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "SELECT canonical_root, created_at_unix_ms FROM repositories")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var canonicalRoot string
		var createdAt int64
		if err := rows.Scan(&canonicalRoot, &createdAt); err != nil {
			return err
		}
		if !validCanonicalRoot(canonicalRoot) || createdAt <= 0 {
			return fmt.Errorf("%w: stored repository is invalid", ErrInvalid)
		}
	}
	return rows.Err()
}

func validateStoredPiSessionIDs(ctx context.Context, conn *sql.Conn) error {
	rows, err := conn.QueryContext(ctx, "SELECT pi_session_id FROM learning_attempts WHERE pi_session_id IS NOT NULL")
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var piSessionID string
		if err := rows.Scan(&piSessionID); err != nil {
			return err
		}
		if !ValidPiSessionID(piSessionID) {
			return fmt.Errorf("%w: stored Pi Session identity is invalid", ErrInvalid)
		}
	}
	return rows.Err()
}

func (store *Store) recoverInterrupted(ctx context.Context) error {
	finishedAt := time.Now().UTC().Truncate(time.Millisecond).UnixMilli()
	if _, err := store.conn.ExecContext(ctx, `
		UPDATE learning_attempts
		SET finished_at_unix_ms = CASE
			WHEN started_at_unix_ms > ? THEN started_at_unix_ms
			ELSE ?
		END, status = 'interrupted'
		WHERE status = 'running'`, finishedAt, finishedAt); err != nil {
		return fmt.Errorf("recover interrupted history records: %w", err)
	}
	return nil
}

func schemaVersion(ctx context.Context, conn *sql.Conn) (int, error) {
	var version int
	if err := conn.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return 0, err
	}
	return version, nil
}

func configureConnection(ctx context.Context, conn *sql.Conn) error {
	var journalMode string
	if err := conn.QueryRowContext(ctx, "PRAGMA journal_mode = WAL").Scan(&journalMode); err != nil {
		return fmt.Errorf("configure WAL mode: %w", err)
	}
	if !strings.EqualFold(journalMode, "wal") {
		return fmt.Errorf("verify WAL mode: got %q", journalMode)
	}
	settings := []struct {
		set  string
		read string
		want int
	}{
		{set: "PRAGMA synchronous = FULL", read: "PRAGMA synchronous", want: 2},
		{set: "PRAGMA foreign_keys = ON", read: "PRAGMA foreign_keys", want: 1},
		{set: "PRAGMA trusted_schema = OFF", read: "PRAGMA trusted_schema", want: 0},
		{set: "PRAGMA busy_timeout = 5000", read: "PRAGMA busy_timeout", want: 5000},
	}
	for _, setting := range settings {
		if _, err := conn.ExecContext(ctx, setting.set); err != nil {
			return fmt.Errorf("configure SQLite setting: %w", err)
		}
		var got int
		if err := conn.QueryRowContext(ctx, setting.read).Scan(&got); err != nil {
			return fmt.Errorf("read SQLite setting: %w", err)
		}
		if got != setting.want {
			return fmt.Errorf("verify SQLite setting %q: got %d, want %d", setting.read, got, setting.want)
		}
	}
	return nil
}

func applyMigration(ctx context.Context, conn *sql.Conn, migration schemaMigration) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := conn.ExecContext(ctx, migration.sql); err != nil {
		return fmt.Errorf("apply schema migration %d: %w", migration.version, err)
	}
	if _, err := conn.ExecContext(ctx, fmt.Sprintf("PRAGMA user_version = %d", migration.version)); err != nil {
		return fmt.Errorf("record schema migration %d: %w", migration.version, err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit schema migration %d: %w", migration.version, err)
	}
	committed = true
	return nil
}

func verifySchema(ctx context.Context, conn *sql.Conn, wantVersion int) error {
	version, err := schemaVersion(ctx, conn)
	if err != nil {
		return fmt.Errorf("%w: read migrated schema version: %v", ErrCorrupt, err)
	}
	if version != wantVersion {
		return fmt.Errorf("%w: got schema version %d, want %d", ErrCorrupt, version, wantVersion)
	}
	actual, err := inspectSchema(ctx, conn)
	if err != nil {
		return err
	}
	expected, err := expectedSchema(ctx, wantVersion)
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("%w: unexpected schema object count", ErrCorrupt)
	}
	for key, want := range expected {
		if got, ok := actual[key]; !ok || got != want {
			return fmt.Errorf("%w: schema object %q does not match migration %d", ErrCorrupt, key, wantVersion)
		}
	}
	return nil
}

func inspectSchema(ctx context.Context, conn *sql.Conn) (map[string]string, error) {
	rows, err := conn.QueryContext(ctx, `
		SELECT type, name, sql
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL
		ORDER BY type, name`)
	if err != nil {
		return nil, fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
	}
	defer rows.Close()
	actual := make(map[string]string)
	for rows.Next() {
		var objectType, name, statement string
		if err := rows.Scan(&objectType, &name, &statement); err != nil {
			return nil, fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
		}
		actual[objectType+":"+name] = normalizeSQL(statement)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
	}
	return actual, nil
}

func verifyEmptySchema(ctx context.Context, conn *sql.Conn) error {
	var count int
	if err := conn.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL`).Scan(&count); err != nil {
		return fmt.Errorf("%w: inspect empty schema: %v", ErrCorrupt, err)
	}
	if count != 0 {
		return fmt.Errorf("%w: unversioned database is not empty", ErrCorrupt)
	}
	return nil
}

func expectedSchema(ctx context.Context, version int) (map[string]string, error) {
	database, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		return nil, fmt.Errorf("%w: open expected schema database: %v", ErrCorrupt, err)
	}
	database.SetMaxOpenConns(1)
	defer database.Close()
	conn, err := database.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("%w: open expected schema connection: %v", ErrCorrupt, err)
	}
	defer conn.Close()
	for _, migration := range schemaMigrations {
		if migration.version > version {
			break
		}
		if _, err := conn.ExecContext(ctx, migration.sql); err != nil {
			return nil, fmt.Errorf("%w: construct expected schema %d: %v", ErrCorrupt, version, err)
		}
	}
	return inspectSchema(ctx, conn)
}

func normalizeSQL(value string) string {
	return strings.Join(strings.Fields(value), " ")
}

func checkIntegrity(ctx context.Context, conn *sql.Conn) error {
	var result string
	if err := conn.QueryRowContext(ctx, "PRAGMA quick_check(1)").Scan(&result); err != nil {
		return fmt.Errorf("%w: integrity check failed: %v", ErrCorrupt, err)
	}
	if result != "ok" {
		return fmt.Errorf("%w: integrity check returned %q", ErrCorrupt, result)
	}
	rows, err := conn.QueryContext(ctx, "PRAGMA foreign_key_check")
	if err != nil {
		return fmt.Errorf("%w: foreign key check failed: %v", ErrCorrupt, err)
	}
	defer rows.Close()
	if rows.Next() {
		return fmt.Errorf("%w: foreign key check found a violation", ErrCorrupt)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: foreign key check failed: %v", ErrCorrupt, err)
	}
	return nil
}
