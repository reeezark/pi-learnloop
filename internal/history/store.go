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
	switch version {
	case 0:
		if err := verifyEmptySchema(ctx, store.conn); err != nil {
			return err
		}
		if err := applyMigrationOne(ctx, store.conn); err != nil {
			return err
		}
		if err := verifySchema(ctx, store.conn); err != nil {
			return err
		}
	case currentSchemaVersion:
		if err := verifySchema(ctx, store.conn); err != nil {
			return err
		}
	}
	if err := validateStoredRepositories(ctx, store.conn); err != nil {
		return fmt.Errorf("%w: stored repository validation failed: %v", ErrCorrupt, err)
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

func applyMigrationOne(ctx context.Context, conn *sql.Conn) error {
	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return fmt.Errorf("begin schema migration: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_, _ = conn.ExecContext(context.Background(), "ROLLBACK")
		}
	}()
	if _, err := conn.ExecContext(ctx, migrationOne); err != nil {
		return fmt.Errorf("apply schema migration 1: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "PRAGMA user_version = 1"); err != nil {
		return fmt.Errorf("record schema migration 1: %w", err)
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		return fmt.Errorf("commit schema migration 1: %w", err)
	}
	committed = true
	return nil
}

func verifySchema(ctx context.Context, conn *sql.Conn) error {
	version, err := schemaVersion(ctx, conn)
	if err != nil {
		return fmt.Errorf("%w: read migrated schema version: %v", ErrCorrupt, err)
	}
	if version != currentSchemaVersion {
		return fmt.Errorf("%w: got schema version %d, want %d", ErrCorrupt, version, currentSchemaVersion)
	}
	rows, err := conn.QueryContext(ctx, `
		SELECT type, name, sql
		FROM sqlite_schema
		WHERE name NOT LIKE 'sqlite_%' AND sql IS NOT NULL
		ORDER BY type, name`)
	if err != nil {
		return fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
	}
	defer rows.Close()
	actual := make(map[string]string)
	for rows.Next() {
		var objectType, name, statement string
		if err := rows.Scan(&objectType, &name, &statement); err != nil {
			return fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
		}
		actual[objectType+":"+name] = normalizeSQL(statement)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("%w: inspect schema: %v", ErrCorrupt, err)
	}
	expected, err := expectedSchema()
	if err != nil {
		return err
	}
	if len(actual) != len(expected) {
		return fmt.Errorf("%w: unexpected schema object count", ErrCorrupt)
	}
	for key, want := range expected {
		if got, ok := actual[key]; !ok || got != want {
			return fmt.Errorf("%w: schema object %q does not match migration 1", ErrCorrupt, key)
		}
	}
	return nil
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

func expectedSchema() (map[string]string, error) {
	expected := make(map[string]string)
	for _, raw := range strings.Split(migrationOne, ";") {
		statement := strings.TrimSpace(raw)
		if statement == "" {
			continue
		}
		fields := strings.Fields(statement)
		if len(fields) < 3 || fields[0] != "CREATE" || (fields[1] != "TABLE" && fields[1] != "INDEX") {
			return nil, fmt.Errorf("%w: migration 1 contains an unexpected statement", ErrCorrupt)
		}
		objectType := strings.ToLower(fields[1])
		name := fields[2]
		expected[objectType+":"+name] = normalizeSQL(statement)
	}
	return expected, nil
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
