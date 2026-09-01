package history

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
)

func TestOpenConfiguresAndVerifiesSQLite(t *testing.T) {
	store, err := Open(context.Background(), filepath.Join(t.TempDir(), "data"))
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer store.Close()

	var journalMode string
	if err := store.conn.QueryRowContext(context.Background(), "PRAGMA journal_mode").Scan(&journalMode); err != nil {
		t.Fatalf("PRAGMA journal_mode error = %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode = %q, want wal", journalMode)
	}
	settings := []struct {
		name string
		want int
	}{
		{name: "synchronous", want: 2},
		{name: "foreign_keys", want: 1},
		{name: "trusted_schema", want: 0},
		{name: "busy_timeout", want: 5000},
		{name: "user_version", want: 1},
	}
	for _, setting := range settings {
		t.Run(setting.name, func(t *testing.T) {
			var got int
			if err := store.conn.QueryRowContext(context.Background(), "PRAGMA "+setting.name).Scan(&got); err != nil {
				t.Fatalf("PRAGMA %s error = %v", setting.name, err)
			}
			if got != setting.want {
				t.Fatalf("PRAGMA %s = %d, want %d", setting.name, got, setting.want)
			}
		})
	}
}

func TestImmediateRollbackDoesNotSurviveReopen(t *testing.T) {
	ctx := context.Background()
	dataDir := filepath.Join(t.TempDir(), "data")
	store, err := Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	wantErr := errors.New("stop transaction")
	err = immediate(ctx, store.conn, func() error {
		if _, err := store.conn.ExecContext(ctx, `
			INSERT INTO repositories(canonical_root, created_at_unix_ms)
			VALUES ('/private/temporary-repository', 1)`); err != nil {
			return err
		}
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("immediate() error = %v, want sentinel", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close() error = %v", err)
	}

	store, err = Open(ctx, dataDir)
	if err != nil {
		t.Fatalf("Open(existing store) error = %v", err)
	}
	defer store.Close()
	var count int
	if err := store.conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM repositories").Scan(&count); err != nil {
		t.Fatalf("count repositories error = %v", err)
	}
	if count != 0 {
		t.Fatalf("repository count = %d, want rolled-back zero", count)
	}
}
