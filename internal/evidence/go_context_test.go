package evidence_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestPreviewGoContextMatchesCommitAndWorkingTreeSnapshots(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/context\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "dep/dep.go", `package dep

type Runner interface {
	Run() string
}

type Token struct {
	Value string
}
`)
	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/context/dep"

type Worker struct {
	ID int
}

func (Worker) Run() string { return "base" }

func Build() dep.Token { return dep.Token{Value: "base"} }
`)
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/context/dep"

type Worker struct {
	ID int
}

func (Worker) Run() string { return "working" }

func Build() dep.Token { return dep.Token{Value: "working"} }
`)
	working := previewGoContext(t, repo, evidence.WorkingTree(base))
	assertCompleteGoContext(t, working.GoContext)
	assertContextItem(t, working.GoContext, "dep/dep.go", "Runner")
	assertContextItem(t, working.GoContext, "dep/dep.go", "Token")
	assertContextRelation(t, working.GoContext, evidence.ContextRelationImplements, evidence.ContextRelationTypeChecked, "Runner")
	assertContextRelation(t, working.GoContext, evidence.ContextRelationReferences, evidence.ContextRelationTypeChecked, "Token")
	assertContextRelation(t, working.GoContext, evidence.ContextRelationImports, evidence.ContextRelationSyntactic, "package:example.com/context/dep")

	commitAll(t, repo, "head")
	head := revision(t, repo, "HEAD")
	committed := previewGoContext(t, repo, evidence.CommitRange(base, head))
	if !reflect.DeepEqual(committed.GoContext, working.GoContext) {
		workingJSON, _ := json.MarshalIndent(working.GoContext, "", "  ")
		commitJSON, _ := json.MarshalIndent(committed.GoContext, "", "  ")
		t.Fatalf("working and commit Go contexts differ:\nworking=%s\ncommit=%s", workingJSON, commitJSON)
	}
}

func TestPreviewGoContextUsesHistoricalHeadOnly(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/history\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "dep/dep.go", `package dep

type Token struct { HeadMarker string }
`)
	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/history/dep"

func Build() dep.Token { return dep.Token{} }
`)
	commitAll(t, repo, "historical base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/history/dep"

func Build() dep.Token { return dep.Token{HeadMarker: "selected"} }
`)
	commitAll(t, repo, "historical head")
	head := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "dep/dep.go", `package dep

type Token struct { CurrentCheckoutSecret string }
`)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/history\n\ngo 1.99\n")
	result := previewGoContext(t, repo, evidence.CommitRange(base, head))
	item := assertContextItem(t, result.GoContext, "dep/dep.go", "Token")
	if !strings.Contains(item.Content, "HeadMarker") || strings.Contains(item.Content, "CurrentCheckoutSecret") {
		t.Fatalf("historical context content = %q, want selected head only", item.Content)
	}
	encoded, err := json.Marshal(result.GoContext)
	if err != nil {
		t.Fatalf("json.Marshal(GoContext): %v", err)
	}
	if strings.Contains(string(encoded), repo) {
		t.Fatalf("Go context contains absolute repository path: %s", encoded)
	}
}

func TestPreviewGoContextAddsImportEvidenceWithoutChangingV1(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/imports\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "sample.go", "package sample\n\nimport _ \"fmt\"\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "sample.go", "package sample\n\nimport named \"log\"\n")
	commitAll(t, repo, "head")
	head := revision(t, repo, "HEAD")

	plain, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, head),
		Limits:     generousLimits(),
	})
	if err != nil {
		t.Fatalf("plain Preview() error = %v", err)
	}
	enriched := previewGoContext(t, repo, evidence.CommitRange(base, head))
	contextValue := enriched.GoContext
	enriched.GoContext = nil
	if !reflect.DeepEqual(enriched, plain) {
		t.Fatalf("enriched changed evidence differs from v1:\nenriched=%#v\nplain=%#v", enriched, plain)
	}
	if contextValue == nil {
		t.Fatal("GoContext = nil, want enriched context")
	}
	var importItem *evidence.ContextItem
	for index := range contextValue.Items {
		if contextValue.Items[index].Kind == evidence.ContextItemChangedImport {
			importItem = &contextValue.Items[index]
			break
		}
	}
	if importItem == nil || importItem.Identity != "named log" || importItem.Content != "named \"log\"" {
		t.Fatalf("changed import item = %#v, want exact aliased import", importItem)
	}
	if contextValue.Status != evidence.ContextPartial || contextOmissionCount(contextValue, evidence.ContextOmissionExternalTypeUnavailable) == 0 {
		t.Fatalf("Go context = %#v, want explicit partial external import", contextValue)
	}
	if _, err := evidence.BuildBundle(previewGoContext(t, repo, evidence.CommitRange(base, head))); evidence.BundleErrorCodeOf(err) != evidence.BundleErrorInvalidResult {
		t.Fatalf("BuildBundle(enriched) error = %v, want v1 fail closed", err)
	}
}

func TestPreviewRejectsUnknownContextMode(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "sample.go", "package sample\n")
	commitAll(t, repo, "base")
	_, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange("HEAD", "HEAD"),
		Limits:     generousLimits(),
		Context:    evidence.ContextMode("future"),
	})
	if evidence.ErrorCodeOf(err) != evidence.ErrorInvalidRequest {
		t.Fatalf("Preview() error = %v, want invalid_request", err)
	}
}

func TestPreviewGoContextHonorsBuildConstraints(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/buildtags\n\ngo 1.21\n")
	selectedOS := runtime.GOOS
	excludedOS := "linux"
	if selectedOS == "linux" {
		excludedOS = "darwin"
	}
	writeRepositoryFile(t, repo, "dep/platform_"+selectedOS+".go", "package dep\n\ntype Platform struct { SelectedMarker string }\n")
	writeRepositoryFile(t, repo, "dep/platform_"+excludedOS+".go", "package dep\n\ntype Platform struct { ExcludedMarker string }\n")
	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/buildtags/dep"

func Build() dep.Platform { return dep.Platform{} }
`)
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "app/app.go", `package app

import "example.com/buildtags/dep"

func Build() dep.Platform { return dep.Platform{SelectedMarker: "selected"} }
`)
	result := previewGoContext(t, repo, evidence.WorkingTree(base))
	item := assertContextItem(t, result.GoContext, "dep/platform_"+selectedOS+".go", "Platform")
	if !strings.Contains(item.Content, "SelectedMarker") || strings.Contains(item.Content, "ExcludedMarker") {
		t.Fatalf("build-selected context = %q", item.Content)
	}
	if result.GoContext.Build.GOOS != runtime.GOOS || result.GoContext.Build.GOARCH != runtime.GOARCH || result.GoContext.Build.CGOEnabled {
		t.Fatalf("build configuration = %#v, want current GOOS/GOARCH and CGO disabled", result.GoContext.Build)
	}
}

func TestPreviewGoContextIncludesTestsOnlyForChangedTestVariant(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/tests\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "app.go", "package tests\n\nfunc Production() int { return 1 }\n")
	writeRepositoryFile(t, repo, "helper_test.go", "package tests\n\nvar testFixture = 42\n")
	writeRepositoryFile(t, repo, "app_test.go", "package tests\n\nfunc TestContext() int { return testFixture }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")

	writeRepositoryFile(t, repo, "app.go", "package tests\n\nfunc Production() int { return 2 }\n")
	production := previewGoContext(t, repo, evidence.WorkingTree(base))
	if production.GoContext.Build.TestVariant {
		t.Fatal("production-only change unexpectedly enabled test variant")
	}
	for _, item := range production.GoContext.Items {
		if strings.HasSuffix(item.Path, "_test.go") {
			t.Fatalf("production context included test item %#v", item)
		}
	}

	writeRepositoryFile(t, repo, "app.go", "package tests\n\nfunc Production() int { return 1 }\n")
	writeRepositoryFile(t, repo, "app_test.go", "package tests\n\nfunc TestContext() int { return testFixture + 1 }\n")
	testVariant := previewGoContext(t, repo, evidence.WorkingTree(base))
	if !testVariant.GoContext.Build.TestVariant {
		t.Fatal("changed test file did not enable test variant")
	}
	assertContextItem(t, testVariant.GoContext, "helper_test.go", "testFixture")
}

func TestPreviewGoContextHandlesAddedAndDeletedFilesWithoutReconstruction(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/file-status\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "deleted.go", "package filestatus\n\nfunc Removed() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	if err := os.Remove(filepath.Join(repo, "deleted.go")); err != nil {
		t.Fatalf("Remove(deleted.go): %v", err)
	}
	writeRepositoryFile(t, repo, "added.go", "package filestatus\n\nfunc Added() int { return 2 }\n")

	working := previewGoContext(t, repo, evidence.WorkingTree(base))
	assertCompleteGoContext(t, working.GoContext)
	if paths := filePaths(working.Files); !reflect.DeepEqual(paths, []string{"added.go", "deleted.go"}) {
		t.Fatalf("changed files = %v, want added and deleted", paths)
	}
	if working.Files[0].Status != evidence.FileAdded || working.Files[1].Status != evidence.FileDeleted || omissionCount(working.Files[1].Omissions, evidence.OmissionDeletedFile) != 1 {
		t.Fatalf("changed evidence = %#v, want explicit added/deleted states", working.Files)
	}
	commitAll(t, repo, "head")
	committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
	if !reflect.DeepEqual(committed.GoContext, working.GoContext) {
		t.Fatalf("added/deleted adapter mismatch:\nworking=%#v\ncommit=%#v", working.GoContext, committed.GoContext)
	}
}

func TestPreviewGoContextSupportsNestedModulesWorkspacesAndReplacements(t *testing.T) {
	t.Run("nested module", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/root\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "nested/go.mod", "module example.com/nested\n\ngo 1.21\n\ntoolchain go1.25.0\n")
		writeRepositoryFile(t, repo, "nested/dep/dep.go", "package dep\n\ntype Token struct{}\n")
		writeRepositoryFile(t, repo, "nested/app/app.go", "package app\n\nimport \"example.com/nested/dep\"\n\nfunc Build() dep.Token { return dep.Token{} }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "nested/app/app.go", "package app\n\nimport \"example.com/nested/dep\"\n\nfunc Build() dep.Token { return dep.Token{ } }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertCompleteGoContext(t, result.GoContext)
		assertContextItem(t, result.GoContext, "nested/dep/dep.go", "Token")
		if len(result.GoContext.Build.Modules) != 1 || result.GoContext.Build.Modules[0].Directory != "nested" || result.GoContext.Build.Modules[0].Toolchain != "go1.25.0" {
			t.Fatalf("modules = %#v, want nearest nested module only", result.GoContext.Build.Modules)
		}
		commitAll(t, repo, "head")
		committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
		if !reflect.DeepEqual(committed.GoContext, result.GoContext) {
			t.Fatalf("nested-module adapter mismatch:\nworking=%#v\ncommit=%#v", result.GoContext, committed.GoContext)
		}
	})

	t.Run("workspace", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.work", "go 1.21\n\ntoolchain go1.25.0\n\nuse (\n\t./app\n\t./dep\n)\n")
		writeRepositoryFile(t, repo, "app/go.mod", "module example.com/app\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "dep/go.mod", "module example.com/dep\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "dep/dep.go", "package dep\n\ntype Token struct{}\n")
		writeRepositoryFile(t, repo, "app/app.go", "package app\n\nimport \"example.com/dep\"\n\nfunc Build() dep.Token { return dep.Token{} }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "app/app.go", "package app\n\nimport \"example.com/dep\"\n\nfunc Build() dep.Token { return dep.Token{ } }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertCompleteGoContext(t, result.GoContext)
		assertContextItem(t, result.GoContext, "dep/dep.go", "Token")
		if len(result.GoContext.Build.Workspaces) != 1 || result.GoContext.Build.Workspaces[0].Directory != "" || result.GoContext.Build.Workspaces[0].Toolchain != "go1.25.0" {
			t.Fatalf("workspaces = %#v, want repository-root workspace", result.GoContext.Build.Workspaces)
		}
		commitAll(t, repo, "head")
		committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
		if !reflect.DeepEqual(committed.GoContext, result.GoContext) {
			t.Fatalf("workspace adapter mismatch:\nworking=%#v\ncommit=%#v", result.GoContext, committed.GoContext)
		}
	})

	t.Run("local replacement", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/app\n\ngo 1.21\n\nrequire example.com/api v0.0.0\nreplace example.com/api => ./localapi\n")
		writeRepositoryFile(t, repo, "localapi/go.mod", "module example.com/implementation\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "localapi/api.go", "package api\n\ntype Token struct{}\n")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nimport \"example.com/api\"\n\nfunc Build() api.Token { return api.Token{} }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nimport \"example.com/api\"\n\nfunc Build() api.Token { return api.Token{ } }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertCompleteGoContext(t, result.GoContext)
		assertContextItem(t, result.GoContext, "localapi/api.go", "Token")
		if len(result.GoContext.Build.Replacements) != 1 || result.GoContext.Build.Replacements[0].Directory != "localapi" {
			t.Fatalf("replacements = %#v, want contained replacement", result.GoContext.Build.Replacements)
		}
		commitAll(t, repo, "head")
		committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
		if !reflect.DeepEqual(committed.GoContext, result.GoContext) {
			t.Fatalf("replacement adapter mismatch:\nworking=%#v\ncommit=%#v", result.GoContext, committed.GoContext)
		}
	})
}

func TestPreviewGoContextRejectsWorkspaceThatOmitsAChangedModule(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.work", "go 1.21\n\nuse ./listed\n")
	writeRepositoryFile(t, repo, "listed/go.mod", "module example.com/listed\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "listed/listed.go", "package listed\n\nfunc Listed() int { return 1 }\n")
	writeRepositoryFile(t, repo, "unlisted/go.mod", "module example.com/unlisted\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "unlisted/unlisted.go", "package unlisted\n\nfunc Unlisted() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "listed/listed.go", "package listed\n\nfunc Listed() int { return 2 }\n")
	writeRepositoryFile(t, repo, "unlisted/unlisted.go", "package unlisted\n\nfunc Unlisted() int { return 2 }\n")
	result := previewGoContext(t, repo, evidence.WorkingTree(base))
	assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionUnsupportedModuleLayout)
}

func TestPreviewGoContextReportsBoundedIncompleteStates(t *testing.T) {
	tests := []struct {
		name       string
		module     string
		otherFiles map[string]string
		base       string
		head       string
		status     evidence.ContextStatus
		reason     evidence.ContextOmissionReason
	}{
		{
			name:   "external import",
			module: "module example.com/external\n\ngo 1.21\n",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\nimport \"fmt\"\n\nfunc Build() fmt.Stringer { return nil }\n",
			status: evidence.ContextPartial,
			reason: evidence.ContextOmissionExternalTypeUnavailable,
		},
		{
			name:   "cgo",
			module: "module example.com/cgo\n\ngo 1.21\n",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\n/* int answer; */\nimport \"C\"\n\nfunc Build() C.int { return C.answer }\n",
			status: evidence.ContextPartial,
			reason: evidence.ContextOmissionCGOUnsupported,
		},
		{
			name:       "malformed package peer",
			module:     "module example.com/parse\n\ngo 1.21\n",
			otherFiles: map[string]string{"broken.go": "package sample\n\nfunc Broken( {\n"},
			base:       "package sample\n\nfunc Build() int { return 1 }\n",
			head:       "package sample\n\nfunc Build() int { return 2 }\n",
			status:     evidence.ContextPartial,
			reason:     evidence.ContextOmissionParseError,
		},
		{
			name:       "import cycle",
			module:     "module example.com/cycle\n\ngo 1.21\n",
			otherFiles: map[string]string{"dep/dep.go": "package dep\n\nimport \"example.com/cycle\"\n\ntype Token struct { Root func() int }\n"},
			base:       "package cycle\n\nimport \"example.com/cycle/dep\"\n\nfunc Build() dep.Token { return dep.Token{} }\n",
			head:       "package cycle\n\nimport \"example.com/cycle/dep\"\n\nfunc Build() dep.Token { return dep.Token{Root: func() int { return 1 }} }\n",
			status:     evidence.ContextPartial,
			reason:     evidence.ContextOmissionTypeIncomplete,
		},
		{
			name:   "newer language version",
			module: "module example.com/future\n\ngo 1.99\n",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\nfunc Build() int { return 2 }\n",
			status: evidence.ContextUnavailable,
			reason: evidence.ContextOmissionUnsupportedGoVersion,
		},
		{
			name:   "missing module",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\nfunc Build() int { return 2 }\n",
			status: evidence.ContextUnavailable,
			reason: evidence.ContextOmissionUnsupportedModuleLayout,
		},
		{
			name:   "missing module language version",
			module: "module example.com/no-version\n",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\nfunc Build() int { return 2 }\n",
			status: evidence.ContextUnavailable,
			reason: evidence.ContextOmissionUnsupportedModuleLayout,
		},
		{
			name:   "version-specific replacement",
			module: "module example.com/versioned\n\ngo 1.21\n\nrequire example.com/api v1.2.3\nreplace example.com/api v1.2.3 => ./api\n",
			base:   "package sample\n\nfunc Build() int { return 1 }\n",
			head:   "package sample\n\nfunc Build() int { return 2 }\n",
			status: evidence.ContextUnavailable,
			reason: evidence.ContextOmissionUnsupportedModuleLayout,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			repo := newRepository(t)
			if test.module != "" {
				writeRepositoryFile(t, repo, "go.mod", test.module)
			}
			for name, content := range test.otherFiles {
				writeRepositoryFile(t, repo, name, content)
			}
			writeRepositoryFile(t, repo, "sample.go", test.base)
			commitAll(t, repo, "base")
			base := revision(t, repo, "HEAD")
			writeRepositoryFile(t, repo, "sample.go", test.head)
			result := previewGoContext(t, repo, evidence.WorkingTree(base))
			if result.GoContext.Status != test.status || contextOmissionCount(result.GoContext, test.reason) == 0 {
				t.Fatalf("Go context = %#v, want status %q and reason %q", result.GoContext, test.status, test.reason)
			}
			if (test.reason == evidence.ContextOmissionExternalTypeUnavailable || test.reason == evidence.ContextOmissionCGOUnsupported) && contextOmissionCount(result.GoContext, test.reason) != 1 {
				t.Fatalf("omissions = %#v, want one deduplicated %q import edge", result.GoContext.Omissions, test.reason)
			}
			if test.status == evidence.ContextUnavailable && (len(result.GoContext.Items) != 0 || len(result.GoContext.Relations) != 0) {
				t.Fatalf("unavailable context retained items or relations: %#v", result.GoContext)
			}
			if test.status == evidence.ContextPartial {
				for _, relation := range result.GoContext.Relations {
					if relation.Strength == evidence.ContextRelationTypeChecked {
						t.Fatalf("incomplete package emitted type-checked relation: %#v", relation)
					}
				}
			}
		})
	}
}

func TestPreviewGoContextDoesNotFollowOutsideConfigurationOrSymlinks(t *testing.T) {
	t.Run("outside workspace and replacement", func(t *testing.T) {
		parent := t.TempDir()
		repo := filepath.Join(parent, "repository")
		if err := os.MkdirAll(repo, 0o755); err != nil {
			t.Fatalf("MkdirAll(repository): %v", err)
		}
		runGit(t, repo, "init", "-q")
		runGit(t, repo, "config", "user.name", "Pi LearnLoop Test")
		runGit(t, repo, "config", "user.email", "test@example.invalid")
		outside := filepath.Join(parent, "outside")
		writeRepositoryFile(t, outside, "go.mod", "module example.com/outside\n\ngo 1.21\n")
		writeRepositoryFile(t, outside, "outside.go", "package outside\n\ntype HostSecret struct{}\n")
		writeRepositoryFile(t, repo, "go.mod", "module example.com/app\n\ngo 1.21\n\nrequire example.com/outside v0.0.0\nreplace example.com/outside => ../outside\n")
		writeRepositoryFile(t, repo, "go.work", "go 1.21\n\nuse (\n\t.\n\t../outside\n)\n")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nfunc Build() int { return 1 }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nimport _ \"example.com/outside\"\n\nfunc Build() int { return 2 }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		if result.GoContext.Status != evidence.ContextPartial || contextOmissionCount(result.GoContext, evidence.ContextOmissionOutsideRepositoryDependency) < 2 {
			t.Fatalf("Go context = %#v, want both outside workspace and replacement omissions", result.GoContext)
		}
		encoded, _ := json.Marshal(result.GoContext)
		if strings.Contains(string(encoded), "HostSecret") || strings.Contains(string(encoded), outside) {
			t.Fatalf("outside source or path entered context: %s", encoded)
		}
	})

	t.Run("outside symlink in both adapters", func(t *testing.T) {
		parent := t.TempDir()
		repo := filepath.Join(parent, "repository")
		if err := os.MkdirAll(filepath.Join(repo, "dep"), 0o755); err != nil {
			t.Fatalf("MkdirAll(repository): %v", err)
		}
		runGit(t, repo, "init", "-q")
		runGit(t, repo, "config", "user.name", "Pi LearnLoop Test")
		runGit(t, repo, "config", "user.email", "test@example.invalid")
		outsideFile := filepath.Join(parent, "outside.go")
		if err := os.WriteFile(outsideFile, []byte("package dep\n\ntype HostSecret struct{}\n"), 0o644); err != nil {
			t.Fatalf("WriteFile(outside): %v", err)
		}
		if err := os.Symlink("../../outside.go", filepath.Join(repo, "dep", "dep.go")); err != nil {
			t.Skipf("Symlink unavailable: %v", err)
		}
		writeRepositoryFile(t, repo, "go.mod", "module example.com/symlink\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nimport _ \"example.com/symlink/dep\"\n\nfunc Build() int { return 1 }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "app.go", "package app\n\nimport _ \"example.com/symlink/dep\"\n\nfunc Build() int { return 2 }\n")
		working := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, working.GoContext, evidence.ContextOmissionOutsideRepositoryDependency)
		commitAll(t, repo, "head")
		committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
		assertUnavailableReason(t, committed.GoContext, evidence.ContextOmissionOutsideRepositoryDependency)
	})

	t.Run("workspace replacement overrides module replacement", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/precedence\n\ngo 1.21\n\nrequire example.com/api v0.0.0\nreplace example.com/api => ./inside\n")
		writeRepositoryFile(t, repo, "go.work", "go 1.21\n\nuse .\n\nreplace example.com/api => ../outside\n")
		writeRepositoryFile(t, repo, "inside/go.mod", "module example.com/inside\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "inside/inside.go", "package inside\n\ntype InsideMarker struct{}\n")
		writeRepositoryFile(t, repo, "app.go", "package precedence\n\nfunc Build() int { return 1 }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "app.go", "package precedence\n\nimport _ \"example.com/api\"\n\nfunc Build() int { return 2 }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		if result.GoContext.Status != evidence.ContextPartial || contextOmissionCount(result.GoContext, evidence.ContextOmissionOutsideRepositoryDependency) == 0 {
			t.Fatalf("Go context = %#v, want partial outside workspace override", result.GoContext)
		}
		if len(result.GoContext.Build.Replacements) != 1 || result.GoContext.Build.Replacements[0].ModulePath != "example.com/api" || result.GoContext.Build.Replacements[0].Directory != "" || result.GoContext.Build.Replacements[0].RepositoryLocal {
			t.Fatalf("effective replacements = %#v, want source-free outside identity", result.GoContext.Build.Replacements)
		}
		encoded, _ := json.Marshal(result.GoContext)
		if strings.Contains(string(encoded), "InsideMarker") || result.GoContext.AnalyzedPackageCount != 1 {
			t.Fatalf("workspace override retained superseded module source: %s", encoded)
		}
	})
}

func TestPreviewGoContextDoesNotReadVendorOrExternalDependencySource(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/vendor-policy\n\ngo 1.21\n\nrequire example.com/external v1.0.0\n")
	writeRepositoryFile(t, repo, "vendor/example.com/external/external.go", "package external\n\ntype VendorSecret struct{}\n")
	writeRepositoryFile(t, repo, "sample.go", "package vendorpolicy\n\nfunc Build() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "sample.go", "package vendorpolicy\n\nimport _ \"example.com/external\"\n\nfunc Build() int { return 2 }\n")
	result := previewGoContext(t, repo, evidence.WorkingTree(base))
	if result.GoContext.Status != evidence.ContextPartial || contextOmissionCount(result.GoContext, evidence.ContextOmissionExternalTypeUnavailable) == 0 || result.GoContext.AnalyzedPackageCount != 1 {
		t.Fatalf("Go context = %#v, want source-free external import", result.GoContext)
	}
	encoded, _ := json.Marshal(result.GoContext)
	if strings.Contains(string(encoded), "VendorSecret") || strings.Contains(string(encoded), "vendor/example.com/external") {
		t.Fatalf("vendor source entered context: %s", encoded)
	}
}

func TestPreviewGoContextRejectsMalformedImportPathBeforeSourceDiscovery(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/import-path\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "secret/secret.go", "package secret\n\ntype SecretMarker struct{}\n")
	writeRepositoryFile(t, repo, "sample.go", "package importpath\n\nfunc Build() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "sample.go", "package importpath\n\nimport _ \"example.com/import-path/../secret\"\n\nfunc Build() int { return 2 }\n")
	result := previewGoContext(t, repo, evidence.WorkingTree(base))
	if result.GoContext.Status != evidence.ContextPartial || contextOmissionCount(result.GoContext, evidence.ContextOmissionParseError) == 0 || result.GoContext.AnalyzedPackageCount != 1 {
		t.Fatalf("Go context = %#v, want malformed import rejected before discovery", result.GoContext)
	}
	encoded, _ := json.Marshal(result.GoContext)
	if strings.Contains(string(encoded), "SecretMarker") {
		t.Fatalf("malformed import discovered unrelated source: %s", encoded)
	}
}

func TestPreviewGoContextTreatsMissingLocalPackageConsistently(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/missing-package\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "sample.go", "package missingpackage\n\nfunc Build() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "sample.go", "package missingpackage\n\nimport _ \"example.com/missing-package/absent\"\n\nfunc Build() int { return 2 }\n")
	working := previewGoContext(t, repo, evidence.WorkingTree(base))
	if working.GoContext.Status != evidence.ContextPartial || contextOmissionCount(working.GoContext, evidence.ContextOmissionTypeIncomplete) == 0 {
		t.Fatalf("working Go context = %#v, want partial missing local package", working.GoContext)
	}
	commitAll(t, repo, "head")
	committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
	if !reflect.DeepEqual(committed.GoContext, working.GoContext) {
		t.Fatalf("missing-package adapter mismatch:\nworking=%#v\ncommit=%#v", working.GoContext, committed.GoContext)
	}
}

func TestPreviewGoContextInputLimitsFailClosed(t *testing.T) {
	t.Run("changed files", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/files\n\ngo 1.21\n")
		for index := 0; index < 21; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("p%02d.go", index), fmt.Sprintf("package files\n\nfunc Value%02d() int { return 1 }\n", index))
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		for index := 0; index < 21; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("p%02d.go", index), fmt.Sprintf("package files\n\nfunc Value%02d() int { return 2 }\n", index))
		}
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("directory entries", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/entries\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "sample.go", "package entries\n\nfunc Build() int { return 1 }\n")
		for index := 0; index < 256; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("entry-%03d.txt", index), "bounded\n")
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", "package entries\n\nfunc Build() int { return 2 }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
		commitAll(t, repo, "head")
		committed := previewGoContext(t, repo, evidence.CommitRange(base, "HEAD"))
		assertUnavailableReason(t, committed.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("files per package", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/packagefiles\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "sample.go", "package packagefiles\n\nfunc Build() int { return 1 }\n")
		for index := 0; index < 64; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("peer-%02d.go", index), fmt.Sprintf("package packagefiles\n\nvar Peer%02d = %d\n", index, index))
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", "package packagefiles\n\nfunc Build() int { return 2 }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("source file bytes", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/large\n\ngo 1.21\n")
		baseSource := "package large\n\n/*" + strings.Repeat("x", 256*1024) + "*/\nfunc Build() int { return 1 }\n"
		writeRepositoryFile(t, repo, "sample.go", baseSource)
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", strings.Replace(baseSource, "return 1", "return 2", 1))
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("aggregate source bytes", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/aggregate\n\ngo 1.21\n")
		padding := strings.Repeat("x", 240*1024)
		for index := 0; index < 9; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("source-%02d.go", index), fmt.Sprintf("package aggregate\n\n/*%s*/\nvar Source%02d = %d\n", padding, index, index))
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "source-00.go", fmt.Sprintf("package aggregate\n\n/*%s*/\nvar Source00 = 99\n", padding))
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("direct import edges", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/edges\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "sample.go", "package edges\n\nfunc Build() int { return 1 }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		var source strings.Builder
		source.WriteString("package edges\n\nimport (\n")
		for index := 0; index < 257; index++ {
			fmt.Fprintf(&source, "\t_ \"example.invalid/dependency/%03d\"\n", index)
		}
		source.WriteString(")\n\nfunc Build() int { return 2 }\n")
		writeRepositoryFile(t, repo, "sample.go", source.String())
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("module roots", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/roots\n\ngo 1.21\n")
		var workspace strings.Builder
		workspace.WriteString("go 1.21\n\nuse (\n\t.\n")
		for index := 0; index < 7; index++ {
			directory := fmt.Sprintf("module-%02d", index)
			fmt.Fprintf(&workspace, "\t./%s\n", directory)
			writeRepositoryFile(t, repo, directory+"/go.mod", fmt.Sprintf("module example.com/root%d\n\ngo 1.21\n", index))
		}
		workspace.WriteString(")\n")
		writeRepositoryFile(t, repo, "go.work", workspace.String())
		writeRepositoryFile(t, repo, "sample.go", "package roots\n\nfunc Build() int { return 1 }\n")
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		writeRepositoryFile(t, repo, "sample.go", "package roots\n\nfunc Build() int { return 2 }\n")
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("package count", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/packages\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "sample.go", "package packages\n\nfunc Build() int { return 1 }\n")
		for index := 0; index < 32; index++ {
			writeRepositoryFile(t, repo, fmt.Sprintf("dep%02d/dep.go", index), fmt.Sprintf("package dep%02d\n\nvar Value = %d\n", index, index))
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		var source strings.Builder
		source.WriteString("package packages\n\nimport (\n")
		for index := 0; index < 32; index++ {
			fmt.Fprintf(&source, "\t_ \"example.com/packages/dep%02d\"\n", index)
		}
		source.WriteString(")\n\nfunc Build() int { return 2 }\n")
		writeRepositoryFile(t, repo, "sample.go", source.String())
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})

	t.Run("total analyzed files", func(t *testing.T) {
		repo := newRepository(t)
		writeRepositoryFile(t, repo, "go.mod", "module example.com/totalfiles\n\ngo 1.21\n")
		writeRepositoryFile(t, repo, "sample.go", "package totalfiles\n\nfunc Build() int { return 1 }\n")
		for packageIndex := 0; packageIndex < 31; packageIndex++ {
			for fileIndex := 0; fileIndex < 6; fileIndex++ {
				name := fmt.Sprintf("dep%02d/file%02d.go", packageIndex, fileIndex)
				content := fmt.Sprintf("package dep%02d\n\nvar Value%02d = %d\n", packageIndex, fileIndex, fileIndex)
				writeRepositoryFile(t, repo, name, content)
			}
		}
		commitAll(t, repo, "base")
		base := revision(t, repo, "HEAD")
		var source strings.Builder
		source.WriteString("package totalfiles\n\nimport (\n")
		for index := 0; index < 31; index++ {
			fmt.Fprintf(&source, "\t_ \"example.com/totalfiles/dep%02d\"\n", index)
		}
		source.WriteString(")\n\nfunc Build() int { return 2 }\n")
		writeRepositoryFile(t, repo, "sample.go", source.String())
		result := previewGoContext(t, repo, evidence.WorkingTree(base))
		assertUnavailableReason(t, result.GoContext, evidence.ContextOmissionAnalysisLimitExceeded)
	})
}

func TestPreviewGoContextOutputLimitsAreDeterministic(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/output\n\ngo 1.21\n")
	var declarations strings.Builder
	var references strings.Builder
	declarations.WriteString("package output\n\n")
	for index := 0; index < 41; index++ {
		fmt.Fprintf(&declarations, "var Context%02d = %d\n", index, index)
		fmt.Fprintf(&references, " + Context%02d", index)
	}
	writeRepositoryFile(t, repo, "context.go", declarations.String())
	writeRepositoryFile(t, repo, "changed.go", "package output\n\nfunc Build() int { return 0 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "changed.go", "package output\n\nfunc Build() int { return 0"+references.String()+" }\n")
	first := previewGoContext(t, repo, evidence.WorkingTree(base))
	second := previewGoContext(t, repo, evidence.WorkingTree(base))
	if !reflect.DeepEqual(first.GoContext, second.GoContext) {
		t.Fatalf("Go context is nondeterministic:\nfirst=%#v\nsecond=%#v", first.GoContext, second.GoContext)
	}
	if first.GoContext.Status != evidence.ContextPartial || len(first.GoContext.Items) != 40 || !first.GoContext.Truncation.Truncated || first.GoContext.Truncation.OmittedItems != 1 {
		t.Fatalf("Go context = %#v, want deterministic 40-item truncation", first.GoContext)
	}
	if first.GoContext.Truncation.OmittedRelations != 1 || contextOmissionCount(first.GoContext, evidence.ContextOmissionOutputTruncated) != 2 {
		t.Fatalf("truncation = %#v, omissions = %#v, want one omitted item and its relation", first.GoContext.Truncation, first.GoContext.Omissions)
	}
	identities := make([]string, len(first.GoContext.Items))
	for index, item := range first.GoContext.Items {
		identities[index] = item.Identity
		if item.ID != fmt.Sprintf("C%03d", index+1) {
			t.Fatalf("item ID = %q, want deterministic ordinal", item.ID)
		}
	}
	if !sort.StringsAreSorted(identities) {
		t.Fatalf("item identities are not sorted: %v", identities)
	}
}

func previewGoContext(t *testing.T, repo string, selection evidence.Selection) evidence.Result {
	t.Helper()
	result, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  selection,
		Limits:     generousLimits(),
		Context:    evidence.ContextGo,
	})
	if err != nil {
		t.Fatalf("Preview(Go context) error = %v", err)
	}
	if result.GoContext == nil {
		t.Fatal("Preview(Go context) returned nil GoContext")
	}
	return result
}

func assertCompleteGoContext(t *testing.T, contextValue *evidence.GoContext) {
	t.Helper()
	if contextValue == nil || contextValue.Status != evidence.ContextComplete || len(contextValue.Omissions) != 0 {
		t.Fatalf("Go context = %#v, want complete without omissions", contextValue)
	}
}

func assertUnavailableReason(t *testing.T, contextValue *evidence.GoContext, reason evidence.ContextOmissionReason) {
	t.Helper()
	if contextValue == nil || contextValue.Status != evidence.ContextUnavailable || contextOmissionCount(contextValue, reason) == 0 {
		t.Fatalf("Go context = %#v, want unavailable with %q", contextValue, reason)
	}
}

func assertContextItem(t *testing.T, contextValue *evidence.GoContext, path, identity string) evidence.ContextItem {
	t.Helper()
	for _, item := range contextValue.Items {
		if item.Path == path && item.Identity == identity {
			return item
		}
	}
	t.Fatalf("context items = %#v, want %s#%s", contextValue.Items, path, identity)
	return evidence.ContextItem{}
}

func assertContextRelation(t *testing.T, contextValue *evidence.GoContext, kind evidence.ContextRelationKind, strength evidence.ContextRelationStrength, targetIdentity string) {
	t.Helper()
	target := targetIdentity
	for _, item := range contextValue.Items {
		if item.Identity == targetIdentity {
			target = item.ID
			break
		}
	}
	for _, relation := range contextValue.Relations {
		if relation.Kind == kind && relation.Strength == strength && relation.To == target {
			return
		}
	}
	t.Fatalf("context relations = %#v, want %q/%q to %q", contextValue.Relations, kind, strength, target)
}

func contextOmissionCount(contextValue *evidence.GoContext, reason evidence.ContextOmissionReason) int {
	for _, omission := range contextValue.Omissions {
		if omission.Reason == reason {
			return omission.Count
		}
	}
	return 0
}
