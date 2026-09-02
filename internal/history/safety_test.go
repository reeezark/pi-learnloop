package history_test

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/history"
	_ "modernc.org/sqlite"
)

func TestOpenPreservesRejectedDatabase(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*testing.T, string)
		want    error
	}{
		{
			name: "future schema",
			prepare: func(t *testing.T, databasePath string) {
				database, err := sql.Open("sqlite", databasePath)
				if err != nil {
					t.Fatalf("sql.Open() error = %v", err)
				}
				if _, err := database.Exec("PRAGMA user_version = 3"); err != nil {
					database.Close()
					t.Fatalf("set future schema version: %v", err)
				}
				if err := database.Close(); err != nil {
					t.Fatalf("close future database: %v", err)
				}
			},
			want: history.ErrUnsupportedSchema,
		},
		{
			name: "corrupt header",
			prepare: func(t *testing.T, databasePath string) {
				if err := os.WriteFile(databasePath, []byte("not a SQLite database"), 0o600); err != nil {
					t.Fatalf("WriteFile(corrupt database) error = %v", err)
				}
			},
			want: history.ErrCorrupt,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dataDir := filepath.Join(t.TempDir(), "data")
			if err := os.Mkdir(dataDir, 0o700); err != nil {
				t.Fatalf("Mkdir(data directory) error = %v", err)
			}
			databasePath := filepath.Join(dataDir, "history.db")
			test.prepare(t, databasePath)
			if err := os.Chmod(databasePath, 0o600); err != nil {
				t.Fatalf("Chmod(database) error = %v", err)
			}
			before, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("ReadFile(before) error = %v", err)
			}

			store, err := history.Open(context.Background(), dataDir)
			if store != nil || !errors.Is(err, test.want) {
				t.Fatalf("Open(rejected database) = (%v, %v), want nil %v", store, err, test.want)
			}
			after, err := os.ReadFile(databasePath)
			if err != nil {
				t.Fatalf("ReadFile(after) error = %v", err)
			}
			if !bytes.Equal(after, before) {
				t.Fatal("Open(rejected database) rewrote the database")
			}
		})
	}
}

func TestOpenRejectsUnprotectedPaths(t *testing.T) {
	t.Run("data directory symlink", func(t *testing.T) {
		target := filepath.Join(t.TempDir(), "target")
		if err := os.Mkdir(target, 0o700); err != nil {
			t.Fatalf("Mkdir(target) error = %v", err)
		}
		dataDir := filepath.Join(t.TempDir(), "data")
		if err := os.Symlink(target, dataDir); err != nil {
			t.Fatalf("Symlink(data directory) error = %v", err)
		}
		if store, err := history.Open(context.Background(), dataDir); store != nil || !errors.Is(err, history.ErrUnsafePath) {
			t.Fatalf("Open(symlink directory) = (%v, %v), want nil ErrUnsafePath", store, err)
		}
	})

	t.Run("database symlink", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "data")
		if err := os.Mkdir(dataDir, 0o700); err != nil {
			t.Fatalf("Mkdir(data directory) error = %v", err)
		}
		target := filepath.Join(t.TempDir(), "target.db")
		if err := os.WriteFile(target, []byte("preserve me"), 0o600); err != nil {
			t.Fatalf("WriteFile(target) error = %v", err)
		}
		if err := os.Symlink(target, filepath.Join(dataDir, "history.db")); err != nil {
			t.Fatalf("Symlink(database) error = %v", err)
		}
		if store, err := history.Open(context.Background(), dataDir); store != nil || !errors.Is(err, history.ErrUnsafePath) {
			t.Fatalf("Open(symlink database) = (%v, %v), want nil ErrUnsafePath", store, err)
		}
		content, err := os.ReadFile(target)
		if err != nil || string(content) != "preserve me" {
			t.Fatalf("symlink target = (%q, %v), want preserved", content, err)
		}
	})

	t.Run("overbroad permissions", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "data")
		if err := os.Mkdir(dataDir, 0o755); err != nil {
			t.Fatalf("Mkdir(data directory) error = %v", err)
		}
		if store, err := history.Open(context.Background(), dataDir); store != nil || !errors.Is(err, history.ErrUnsafePath) {
			t.Fatalf("Open(overbroad directory) = (%v, %v), want nil ErrUnsafePath", store, err)
		}
	})

	t.Run("hard-linked database", func(t *testing.T) {
		dataDir := filepath.Join(t.TempDir(), "data")
		if err := os.Mkdir(dataDir, 0o700); err != nil {
			t.Fatalf("Mkdir(data directory) error = %v", err)
		}
		target := filepath.Join(t.TempDir(), "target.db")
		if err := os.WriteFile(target, nil, 0o600); err != nil {
			t.Fatalf("WriteFile(target) error = %v", err)
		}
		if err := os.Link(target, filepath.Join(dataDir, "history.db")); err != nil {
			t.Fatalf("Link(database) error = %v", err)
		}
		if store, err := history.Open(context.Background(), dataDir); store != nil || !errors.Is(err, history.ErrUnsafePath) {
			t.Fatalf("Open(hard-linked database) = (%v, %v), want nil ErrUnsafePath", store, err)
		}
	})
}

func TestOpenRejectsUnexpectedSchemaWithoutRewriting(t *testing.T) {
	dataDir := filepath.Join(t.TempDir(), "data")
	if err := os.Mkdir(dataDir, 0o700); err != nil {
		t.Fatalf("Mkdir(data directory) error = %v", err)
	}
	databasePath := filepath.Join(dataDir, "history.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	_, err = database.Exec(`
		CREATE TABLE repositories (id INTEGER PRIMARY KEY);
		CREATE TABLE learning_attempts (record_id TEXT PRIMARY KEY);
		CREATE TABLE question_outcomes (record_id TEXT PRIMARY KEY);
		PRAGMA user_version = 1;`)
	if err != nil {
		database.Close()
		t.Fatalf("create unexpected schema: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close unexpected schema: %v", err)
	}
	if err := os.Chmod(databasePath, 0o600); err != nil {
		t.Fatalf("Chmod(database) error = %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	store, err := history.Open(context.Background(), dataDir)
	if store != nil || !errors.Is(err, history.ErrCorrupt) {
		t.Fatalf("Open(unexpected schema) = (%v, %v), want nil ErrCorrupt", store, err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("Open(unexpected schema) rewrote the database")
	}
	for _, suffix := range []string{"-wal", "-shm"} {
		if _, err := os.Lstat(databasePath + suffix); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Open(unexpected schema) created %q", databasePath+suffix)
		}
	}
}

func TestOpenRejectsInvalidStoredRecordWithoutRewriting(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	start := validStart(filepath.Join(t.TempDir(), "repository"))
	if _, err := store.Create(ctx, start); err != nil {
		store.Close()
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	databasePath := filepath.Join(dataDir, "history.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := database.Exec("UPDATE learning_attempts SET provider = ?", "unsafe\nprovider"); err != nil {
		database.Close()
		t.Fatalf("tamper history record: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close tampered database: %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	store, err = history.Open(ctx, dataDir)
	if store != nil || !errors.Is(err, history.ErrCorrupt) {
		t.Fatalf("Open(invalid stored record) = (%v, %v), want nil ErrCorrupt", store, err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("Open(invalid stored record) rewrote the database")
	}
}

func TestOpenRejectsInvalidStoredRepositoryWithoutRewriting(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	if _, err := store.Create(ctx, validStart(filepath.Join(t.TempDir(), "repository"))); err != nil {
		store.Close()
		t.Fatalf("Create() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}
	databasePath := filepath.Join(dataDir, "history.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	if _, err := database.Exec("UPDATE repositories SET canonical_root = 'relative/repository'"); err != nil {
		database.Close()
		t.Fatalf("tamper repository record: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close tampered database: %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	store, err = history.Open(ctx, dataDir)
	if store != nil || !errors.Is(err, history.ErrCorrupt) {
		t.Fatalf("Open(invalid stored repository) = (%v, %v), want nil ErrCorrupt", store, err)
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("Open(invalid stored repository) rewrote the database")
	}
}

func TestOpenRejectsInvalidStoredPiSessionWithoutRewritingOrEcho(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := history.Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	start := validStart(filepath.Join(t.TempDir(), "repository"))
	recordID, err := store.CreateWithPiSession(ctx, start, "valid-session")
	if err != nil {
		store.Close()
		t.Fatalf("CreateWithPiSession() error = %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	databasePath := filepath.Join(dataDir, "history.db")
	database, err := sql.Open("sqlite", databasePath)
	if err != nil {
		t.Fatalf("sql.Open() error = %v", err)
	}
	invalid := "session/secret"
	if _, err := database.Exec("PRAGMA ignore_check_constraints = ON"); err != nil {
		database.Close()
		t.Fatalf("disable check constraints: %v", err)
	}
	if _, err := database.Exec("UPDATE learning_attempts SET pi_session_id = ? WHERE record_id = ?", invalid, recordID); err != nil {
		database.Close()
		t.Fatalf("tamper Pi Session identity: %v", err)
	}
	if err := database.Close(); err != nil {
		t.Fatalf("close tampered database: %v", err)
	}
	before, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(before) error = %v", err)
	}

	store, err = history.Open(ctx, dataDir)
	if store != nil || !errors.Is(err, history.ErrCorrupt) {
		t.Fatalf("Open(invalid stored Pi Session) = (%v, %v), want nil ErrCorrupt", store, err)
	}
	if bytes.Contains([]byte(err.Error()), []byte(invalid)) {
		t.Fatal("Open(invalid stored Pi Session) echoed the rejected ID")
	}
	after, err := os.ReadFile(databasePath)
	if err != nil {
		t.Fatalf("ReadFile(after) error = %v", err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("Open(invalid stored Pi Session) rewrote the database")
	}
}
