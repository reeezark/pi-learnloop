package evidence

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestWorkingTreeSnapshotEnumeratesTrackedAndNonignoredFiles(t *testing.T) {
	repo := newSnapshotRepository(t)
	writeSnapshotFile(t, repo, ".gitignore", "ignored.go\n")
	writeSnapshotFile(t, repo, "tracked.go", "package snapshot\n")
	writeSnapshotFile(t, repo, "pkg[one]/tracked.go", "package one\n")
	runSnapshotGit(t, repo, "add", ".gitignore", "tracked.go")
	runSnapshotGit(t, repo, "add", "pkg[one]/tracked.go")
	runSnapshotGit(t, repo, "commit", "-q", "-m", "base")
	writeSnapshotFile(t, repo, "visible.go", "package snapshot\n")
	writeSnapshotFile(t, repo, "ignored.go", "package snapshot\n")
	writeSnapshotFile(t, repo, "nested/nested.go", "package nested\n")

	adapter := &workingTreeSnapshot{root: repo}
	entries, err := adapter.ReadDir(context.Background(), "", 10)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	names := make([]string, len(entries))
	for index, entry := range entries {
		names[index] = entry.Name
	}
	sort.Strings(names)
	if want := []string{".gitignore", "tracked.go", "visible.go"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("ReadDir() names = %v, want %v", names, want)
	}

	if _, err := adapter.ReadDir(context.Background(), "", 2); !errors.Is(err, errSnapshotLimit) {
		t.Fatalf("ReadDir(limit) error = %v, want errSnapshotLimit", err)
	}
	entries, err = adapter.ReadDir(context.Background(), "pkg[one]", 10)
	if err != nil || len(entries) != 1 || entries[0].Name != "tracked.go" {
		t.Fatalf("ReadDir(glob-literal directory) = (%#v, %v), want tracked.go", entries, err)
	}
	revision := strings.TrimSpace(runSnapshotGit(t, repo, "rev-parse", "HEAD"))
	entries, err = (&commitSnapshot{root: repo, revision: revision}).ReadDir(context.Background(), "pkg[one]", 10)
	if err != nil || len(entries) != 1 || entries[0].Name != "tracked.go" {
		t.Fatalf("commit ReadDir(glob-literal directory) = (%#v, %v), want tracked.go", entries, err)
	}
}

func TestSnapshotReadsAreBoundedAndCancelable(t *testing.T) {
	repo := newSnapshotRepository(t)
	content := "package snapshot\n\nvar Value = 42\n"
	writeSnapshotFile(t, repo, "target.go", content)
	if err := os.Symlink("target.go", filepath.Join(repo, "link.go")); err != nil {
		t.Skipf("Symlink unavailable: %v", err)
	}
	runSnapshotGit(t, repo, "add", "target.go", "link.go")
	runSnapshotGit(t, repo, "commit", "-q", "-m", "base")
	revision := strings.TrimSpace(runSnapshotGit(t, repo, "rev-parse", "HEAD"))

	for name, adapter := range map[string]snapshot{
		"commit":       &commitSnapshot{root: repo, revision: revision},
		"working tree": &workingTreeSnapshot{root: repo},
	} {
		t.Run(name, func(t *testing.T) {
			got, err := adapter.ReadFile(context.Background(), "link.go", int64(len(content)))
			if err != nil || string(got) != content {
				t.Fatalf("ReadFile(link) = (%q, %v), want contained target", got, err)
			}
			if _, err := adapter.ReadFile(context.Background(), "target.go", int64(len(content)-1)); !errors.Is(err, errSnapshotLimit) {
				t.Fatalf("ReadFile(limit) error = %v, want errSnapshotLimit", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if _, err := adapter.ReadFile(ctx, "target.go", int64(len(content))); !errors.Is(err, context.Canceled) {
				t.Fatalf("ReadFile(canceled) error = %v, want context.Canceled", err)
			}
			if _, err := adapter.ReadDir(ctx, "", 10); !errors.Is(err, context.Canceled) {
				t.Fatalf("ReadDir(canceled) error = %v, want context.Canceled", err)
			}
		})
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := gitNULRecords(ctx, repo, 1, "ls-tree", "-z", revision+"^{tree}"); !errors.Is(err, context.Canceled) {
		t.Fatalf("gitNULRecords(canceled) error = %v, want context.Canceled", err)
	}
	if _, _, err := gitBytes(ctx, repo, 1, "rev-parse", "HEAD"); !errors.Is(err, context.Canceled) {
		t.Fatalf("gitBytes(canceled) error = %v, want context.Canceled", err)
	}
}

func newSnapshotRepository(t testing.TB) string {
	t.Helper()
	repo := t.TempDir()
	runSnapshotGit(t, repo, "init", "-q")
	runSnapshotGit(t, repo, "config", "user.name", "Pi LearnLoop Test")
	runSnapshotGit(t, repo, "config", "user.email", "test@example.invalid")
	return repo
}

func writeSnapshotFile(t testing.TB, repo, name, content string) {
	t.Helper()
	target := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(target), err)
	}
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", target, err)
	}
}

func runSnapshotGit(t testing.TB, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
