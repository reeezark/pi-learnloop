package evidence_test

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestPreviewMapsCommitRangeToChangedDeclarations(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	return 1
}
`)
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	return 2
}

func Added() string {
	return "added"
}
`)
	commitAll(t, repo, "head")
	head := revision(t, repo, "HEAD")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, head),
		Limits: evidence.Limits{
			MaxFiles:        10,
			MaxDeclarations: 20,
			MaxExcerptBytes: 16 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	wantLimits := evidence.Limits{
		MaxFiles:        10,
		MaxDeclarations: 20,
		MaxExcerptBytes: 16 * 1024,
	}
	if got.AppliedLimits != wantLimits {
		t.Fatalf("AppliedLimits = %#v, want %#v", got.AppliedLimits, wantLimits)
	}

	wantRoot, err := filepath.EvalSymlinks(repo)
	if err != nil {
		t.Fatalf("EvalSymlinks(%q): %v", repo, err)
	}
	if got.RepositoryRoot != wantRoot {
		t.Fatalf("RepositoryRoot = %q, want canonical root %q", got.RepositoryRoot, wantRoot)
	}
	if got.BaseRevision != base || got.HeadRevision != head {
		t.Fatalf("resolved revisions = (%q, %q), want (%q, %q)", got.BaseRevision, got.HeadRevision, base, head)
	}
	if len(got.Files) != 1 || got.Files[0].Path != "sample.go" {
		t.Fatalf("Files = %#v, want sample.go", got.Files)
	}

	declarations := got.Files[0].Declarations
	if names := declarationNames(declarations); !reflect.DeepEqual(names, []string{"Existing", "Added"}) {
		t.Fatalf("declaration names = %v, want [Existing Added]", names)
	}
	for _, declaration := range declarations {
		if declaration.Kind != evidence.DeclarationFunction {
			t.Errorf("%s kind = %q, want %q", declaration.Name, declaration.Kind, evidence.DeclarationFunction)
		}
		if declaration.StartLine <= 0 || declaration.EndLine < declaration.StartLine {
			t.Errorf("%s lines = %d-%d, want a valid span", declaration.Name, declaration.StartLine, declaration.EndLine)
		}
		if !strings.Contains(declaration.Excerpt, "func "+declaration.Name) {
			t.Errorf("%s excerpt = %q, want declaration source", declaration.Name, declaration.Excerpt)
		}
	}
	if got.Truncation.Truncated {
		t.Fatalf("Truncation = %#v, want an untruncated preview", got.Truncation)
	}
}

func TestPreviewIncludesModifiedAndUntrackedWorkingTreeFiles(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	return 1
}
`)
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	return 2
}
`)
	writeRepositoryFile(t, repo, "new.go", `package sample

func Untracked() bool {
	return true
}
`)
	writeRepositoryFile(t, repo, "notes.txt", "not Go source\n")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.WorkingTree(base),
		Limits: evidence.Limits{
			MaxFiles:        10,
			MaxDeclarations: 20,
			MaxExcerptBytes: 16 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if got.HeadRevision != evidence.WorkingTreeRevision {
		t.Fatalf("HeadRevision = %q, want %q", got.HeadRevision, evidence.WorkingTreeRevision)
	}
	if paths := filePaths(got.Files); !reflect.DeepEqual(paths, []string{"new.go", "sample.go"}) {
		t.Fatalf("file paths = %v, want [new.go sample.go]", paths)
	}
	if got.Files[0].Status != evidence.FileAdded || got.Files[1].Status != evidence.FileModified {
		t.Fatalf("file statuses = [%q %q], want [added modified]", got.Files[0].Status, got.Files[1].Status)
	}
	if names := declarationNames(got.Files[0].Declarations); !reflect.DeepEqual(names, []string{"Untracked"}) {
		t.Fatalf("new.go declarations = %v, want [Untracked]", names)
	}
	if names := declarationNames(got.Files[1].Declarations); !reflect.DeepEqual(names, []string{"Existing"}) {
		t.Fatalf("sample.go declarations = %v, want [Existing]", names)
	}
}

func TestPreviewIdentifiesGoDeclarationKindsAndMethodReceiver(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "declarations.go", "package sample\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "declarations.go", `package sample

const Limit = 10

var Current = Limit

type Payload struct {
	Value string
}

type Runner interface {
	Run() error
}

type Worker struct{}

func (worker *Worker) Run() error {
	return nil
}
`)
	commitAll(t, repo, "add declarations")
	head := revision(t, repo, "HEAD")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, head),
		Limits: evidence.Limits{
			MaxFiles:        10,
			MaxDeclarations: 20,
			MaxExcerptBytes: 16 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	declarations := got.Files[0].Declarations
	want := []struct {
		kind     evidence.DeclarationKind
		name     string
		receiver string
		identity string
	}{
		{kind: evidence.DeclarationConstant, name: "Limit", identity: "Limit"},
		{kind: evidence.DeclarationVariable, name: "Current", identity: "Current"},
		{kind: evidence.DeclarationType, name: "Payload", identity: "Payload"},
		{kind: evidence.DeclarationInterface, name: "Runner", identity: "Runner"},
		{kind: evidence.DeclarationType, name: "Worker", identity: "Worker"},
		{kind: evidence.DeclarationMethod, name: "Run", receiver: "*Worker", identity: "(*Worker).Run"},
	}
	if len(declarations) != len(want) {
		t.Fatalf("declarations = %#v, want %d declarations", declarations, len(want))
	}
	for index, expected := range want {
		declaration := declarations[index]
		if declaration.Kind != expected.kind || declaration.Name != expected.name || declaration.Receiver != expected.receiver || declaration.Identity != expected.identity {
			t.Errorf("declaration[%d] = {kind:%q name:%q receiver:%q identity:%q}, want %#v", index, declaration.Kind, declaration.Name, declaration.Receiver, declaration.Identity, expected)
		}
	}
}

func TestPreviewReportsRenameAsDeletedAndAdded(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "old.go", `package sample

func Renamed() {}
`)
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	if err := os.Rename(filepath.Join(repo, "old.go"), filepath.Join(repo, "new.go")); err != nil {
		t.Fatalf("Rename(): %v", err)
	}
	commitAll(t, repo, "rename")
	head := revision(t, repo, "HEAD")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, head),
		Limits: evidence.Limits{
			MaxFiles:        10,
			MaxDeclarations: 20,
			MaxExcerptBytes: 16 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if paths := filePaths(got.Files); !reflect.DeepEqual(paths, []string{"new.go", "old.go"}) {
		t.Fatalf("file paths = %v, want [new.go old.go]", paths)
	}
	if got.Files[0].Status != evidence.FileAdded || got.Files[1].Status != evidence.FileDeleted {
		t.Fatalf("file statuses = [%q %q], want [added deleted]", got.Files[0].Status, got.Files[1].Status)
	}
	if names := declarationNames(got.Files[0].Declarations); !reflect.DeepEqual(names, []string{"Renamed"}) {
		t.Fatalf("new.go declarations = %v, want [Renamed]", names)
	}
	if len(got.Files[1].Declarations) != 0 {
		t.Fatalf("deleted file declarations = %#v, want none", got.Files[1].Declarations)
	}
	if count := omissionCount(got.Files[1].Omissions, evidence.OmissionDeletedFile); count != 1 {
		t.Fatalf("deleted file omission count = %d, want 1", count)
	}
}

func TestPreviewReturnsEmptyResultForNonGoChanges(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "README.md", "before\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "README.md", "after\n")
	commitAll(t, repo, "head")
	head := revision(t, repo, "HEAD")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, head),
		Limits: evidence.Limits{
			MaxFiles:        10,
			MaxDeclarations: 20,
			MaxExcerptBytes: 16 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}
	if len(got.Files) != 0 {
		t.Fatalf("Files = %#v, want an empty Go preview", got.Files)
	}
}

func TestPreviewReturnsStableErrorCodes(t *testing.T) {
	t.Run("invalid request", func(t *testing.T) {
		_, err := evidence.Preview(context.Background(), evidence.Request{})
		if code := evidence.ErrorCodeOf(err); code != evidence.ErrorInvalidRequest {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, code, evidence.ErrorInvalidRequest)
		}
	})

	t.Run("not a repository", func(t *testing.T) {
		_, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: t.TempDir(),
			Selection:  evidence.CommitRange("HEAD", "HEAD"),
			Limits:     generousLimits(),
		})
		if code := evidence.ErrorCodeOf(err); code != evidence.ErrorNotRepository {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, code, evidence.ErrorNotRepository)
		}
	})

	t.Run("invalid revision", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "sample.go", "package sample\n")
		commitAll(t, repo, "base")

		_, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: repo,
			Selection:  evidence.CommitRange("does-not-exist", "HEAD"),
			Limits:     generousLimits(),
		})
		if code := evidence.ErrorCodeOf(err); code != evidence.ErrorInvalidRevision {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, code, evidence.ErrorInvalidRevision)
		}
	})

	t.Run("malformed Go source", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "broken.go", "package sample\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "broken.go", "package sample\nfunc Broken( {\n")
		commitAll(t, repo, "broken")

		_, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: repo,
			Selection:  evidence.CommitRange(base, "HEAD"),
			Limits:     generousLimits(),
		})
		if code := evidence.ErrorCodeOf(err); code != evidence.ErrorParseSource {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, code, evidence.ErrorParseSource)
		}
	})

	t.Run("working tree path escapes repository", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "sample.go", "package sample\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")

		outside := filepath.Join(t.TempDir(), "outside.go")
		if err := os.WriteFile(outside, []byte("package outside\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(%q): %v", outside, err)
		}
		if err := os.Symlink(outside, filepath.Join(repo, "escape.go")); err != nil {
			t.Skipf("Symlink() unavailable: %v", err)
		}

		_, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: repo,
			Selection:  evidence.WorkingTree(base),
			Limits:     generousLimits(),
		})
		if code := evidence.ErrorCodeOf(err); code != evidence.ErrorOutsideRepository {
			t.Fatalf("ErrorCodeOf(%v) = %q, want %q", err, code, evidence.ErrorOutsideRepository)
		}
	})
}

func generousLimits() evidence.Limits {
	return evidence.Limits{
		MaxFiles:        100,
		MaxDeclarations: 100,
		MaxExcerptBytes: 1024 * 1024,
	}
}

func TestPreviewAppliesDeterministicEvidenceLimits(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "README.md", "base\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "a.go", "package sample\n\nfunc 学习() {}\n")
	writeRepositoryFile(t, repo, "b.go", "package sample\n\nfunc Beta() {}\n")
	writeRepositoryFile(t, repo, "c.go", "package sample\n\nfunc Gamma() {}\n")
	commitAll(t, repo, "add Go files")

	got, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, "HEAD"),
		Limits: evidence.Limits{
			MaxFiles:        2,
			MaxDeclarations: 1,
			MaxExcerptBytes: 6,
		},
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	if paths := filePaths(got.Files); !reflect.DeepEqual(paths, []string{"a.go", "b.go"}) {
		t.Fatalf("file paths = %v, want [a.go b.go]", paths)
	}
	if got.Truncation.OmittedFiles != 1 || got.Truncation.OmittedDeclarations != 1 || got.Truncation.OmittedExcerptBytes == 0 || !got.Truncation.Truncated {
		t.Fatalf("Truncation = %#v, want one omitted file, one omitted declaration, and omitted excerpt bytes", got.Truncation)
	}
	if len(got.Files[0].Declarations) != 1 || len(got.Files[1].Declarations) != 0 {
		t.Fatalf("declaration counts = [%d %d], want [1 0]", len(got.Files[0].Declarations), len(got.Files[1].Declarations))
	}
	excerpt := got.Files[0].Declarations[0].Excerpt
	if len(excerpt) > 6 || !got.Files[0].Declarations[0].ExcerptTruncated {
		t.Fatalf("excerpt = %q (%d bytes), want a truncated excerpt of at most 6 bytes", excerpt, len(excerpt))
	}
	if !utf8.ValidString(excerpt) {
		t.Fatalf("excerpt = %q, want valid UTF-8 after truncation", excerpt)
	}
}

func TestPreviewExplainsChangesThatCannotMapToDeclarations(t *testing.T) {
	t.Run("change outside declaration", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "sample.go", `package sample

import "fmt"

func Existing() { fmt.Println("same") }
`)
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", `package sample

import "log"

func Existing() { fmt.Println("same") }
`)
		commitAll(t, repo, "change import")

		got, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: repo,
			Selection:  evidence.CommitRange(base, "HEAD"),
			Limits:     generousLimits(),
		})
		if err != nil {
			t.Fatalf("Preview() error = %v", err)
		}
		if len(got.Files[0].Declarations) != 0 {
			t.Fatalf("Declarations = %#v, want none for an import-only change", got.Files[0].Declarations)
		}
		if count := omissionCount(got.Files[0].Omissions, evidence.OmissionOutsideDeclaration); count != 1 {
			t.Fatalf("outside-declaration omission count = %d, want 1", count)
		}
	})

	t.Run("deletion-only hunk", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	// remove this line
	return 1
}
`)
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", `package sample

func Existing() int {
	return 1
}
`)
		commitAll(t, repo, "delete line")

		got, err := evidence.Preview(context.Background(), evidence.Request{
			Repository: repo,
			Selection:  evidence.CommitRange(base, "HEAD"),
			Limits:     generousLimits(),
		})
		if err != nil {
			t.Fatalf("Preview() error = %v", err)
		}
		if count := omissionCount(got.Files[0].Omissions, evidence.OmissionDeletedOnlyHunk); count != 1 {
			t.Fatalf("deleted-only omission count = %d, want 1", count)
		}
	})
}

func omissionCount(omissions []evidence.Omission, reason evidence.OmissionReason) int {
	for _, omission := range omissions {
		if omission.Reason == reason {
			return omission.Count
		}
	}
	return 0
}

func declarationNames(declarations []evidence.Declaration) []string {
	names := make([]string, len(declarations))
	for i, declaration := range declarations {
		names[i] = declaration.Name
	}
	return names
}

func filePaths(files []evidence.File) []string {
	paths := make([]string, len(files))
	for i, file := range files {
		paths[i] = file.Path
	}
	return paths
}

func newRepository(t *testing.T) string {
	t.Helper()
	repo := t.TempDir()
	runGit(t, repo, "init", "-q")
	runGit(t, repo, "config", "user.name", "Pi LearnLoop Test")
	runGit(t, repo, "config", "user.email", "test@example.invalid")
	return repo
}

func writeRepositoryFile(t *testing.T, repo, name, content string) {
	t.Helper()
	path := filepath.Join(repo, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("MkdirAll(%q): %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("WriteFile(%q): %v", path, err)
	}
}

func commitAll(t *testing.T, repo, message string) {
	t.Helper()
	runGit(t, repo, "add", "--all")
	runGit(t, repo, "commit", "-q", "-m", message)
}

func revision(t *testing.T, repo, name string) string {
	t.Helper()
	return strings.TrimSpace(runGit(t, repo, "rev-parse", name))
}

func runGit(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
	return string(output)
}
