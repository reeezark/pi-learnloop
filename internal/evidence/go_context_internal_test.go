package evidence

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestLocalOnlyGitEnvironmentIsOptInAndOverridesAmbientValues(t *testing.T) {
	if environment := gitCommandEnvironment(context.Background()); environment != nil {
		t.Fatalf("ordinary Git environment = %#v, want inherited nil environment", environment)
	}
	t.Setenv("GIT_NO_LAZY_FETCH", "0")
	t.Setenv("GIT_TERMINAL_PROMPT", "1")
	t.Setenv("GIT_OPTIONAL_LOCKS", "1")
	environment := gitCommandEnvironment(withLocalOnlyGit(context.Background()))
	for name, want := range map[string]string{
		"GIT_NO_LAZY_FETCH":   "1",
		"GIT_TERMINAL_PROMPT": "0",
		"GIT_OPTIONAL_LOCKS":  "0",
	} {
		got := ""
		for _, entry := range environment {
			if strings.HasPrefix(entry, name+"=") {
				got = strings.TrimPrefix(entry, name+"=")
			}
		}
		if got != want {
			t.Fatalf("%s = %q, want %q in local-only Git environment", name, got, want)
		}
	}
	if len(environment) <= len(os.Environ()) {
		t.Fatal("local-only Git environment did not append fixed policy values")
	}
}

func TestFixedContextLimitsMatchADR0007(t *testing.T) {
	want := ContextLimits{
		MaxChangedFiles:        20,
		MaxModuleRoots:         8,
		MaxPackages:            32,
		MaxFilesPerPackage:     64,
		MaxFiles:               160,
		MaxDirectoryEntries:    256,
		MaxSourceBytesPerFile:  256 * 1024,
		MaxSourceBytes:         2 * 1024 * 1024,
		MaxDirectImportEdges:   256,
		AnalysisTimeout:        30 * time.Second,
		MaxOutputFiles:         20,
		MaxOutputItems:         40,
		MaxRelations:           100,
		MaxExcerptBytes:        4 * 1024,
		MaxOutputBytes:         64 * 1024,
		MaxEvaluatorInputBytes: 256 * 1024,
	}
	if got := fixedContextLimits(); !reflect.DeepEqual(got, want) {
		t.Fatalf("fixedContextLimits() = %#v, want %#v", got, want)
	}
}

func TestContextOutputLimitsAreAppliedAfterDeterministicOrdering(t *testing.T) {
	t.Run("items", func(t *testing.T) {
		analyzer := newOutputLimitAnalyzer()
		for index := 40; index >= 0; index-- {
			key := fmt.Sprintf("item-%02d", index)
			analyzer.itemCandidates[key] = contextItemCandidate{key: key, item: ContextItem{
				Kind:        ContextItemContextDeclaration,
				Path:        "same.go",
				PackagePath: "example.com/context",
				Identity:    key,
				Content:     "var " + key + " = 1",
			}}
		}
		analyzer.finish()
		if len(analyzer.output.Items) != maxContextOutputItems || analyzer.output.Truncation.OmittedItems != 1 {
			t.Fatalf("output = %#v, want 40 items and one omission", analyzer.output)
		}
		for index, item := range analyzer.output.Items {
			if item.Identity != fmt.Sprintf("item-%02d", index) || item.ID != fmt.Sprintf("C%03d", index+1) {
				t.Fatalf("item[%d] = %#v, want sorted identity and stable ID", index, item)
			}
		}
	})

	t.Run("files", func(t *testing.T) {
		analyzer := newOutputLimitAnalyzer()
		for index := 0; index < maxContextOutputFiles+1; index++ {
			key := fmt.Sprintf("item-%02d", index)
			analyzer.itemCandidates[key] = contextItemCandidate{key: key, item: ContextItem{
				Kind:        ContextItemContextDeclaration,
				Path:        fmt.Sprintf("package-%02d/context.go", index),
				PackagePath: fmt.Sprintf("example.com/package-%02d", index),
				Identity:    "Context",
				Content:     "type Context struct{}",
			}}
		}
		analyzer.finish()
		if len(analyzer.output.Items) != maxContextOutputFiles || analyzer.output.Truncation.OmittedFiles != 1 || analyzer.output.Truncation.OmittedItems != 1 {
			t.Fatalf("output = %#v, want 20 files and one omitted file/item", analyzer.output)
		}
	})

	t.Run("relations", func(t *testing.T) {
		analyzer := newOutputLimitAnalyzer()
		for index := 0; index < maxContextRelations+1; index++ {
			key := fmt.Sprintf("relation-%03d", index)
			analyzer.relationCandidates[key] = contextRelationCandidate{
				from:     fmt.Sprintf("changed.go#Build%03d", index),
				target:   "package:example.com/context",
				kind:     ContextRelationImports,
				strength: ContextRelationSyntactic,
			}
		}
		analyzer.finish()
		if len(analyzer.output.Relations) != maxContextRelations || analyzer.output.Truncation.OmittedRelations != 1 {
			t.Fatalf("output = %#v, want 100 relations and one omission", analyzer.output)
		}
	})

	t.Run("excerpt and aggregate bytes", func(t *testing.T) {
		analyzer := newOutputLimitAnalyzer()
		for index := 0; index < maxContextOutputFiles; index++ {
			key := fmt.Sprintf("item-%02d", index)
			analyzer.itemCandidates[key] = contextItemCandidate{key: key, item: ContextItem{
				Kind:        ContextItemContextDeclaration,
				Path:        fmt.Sprintf("package-%02d/context.go", index),
				PackagePath: fmt.Sprintf("example.com/package-%02d", index),
				Identity:    "Context",
				Content:     strings.Repeat("x", maxContextExcerptBytes+1),
			}}
		}
		analyzer.finish()
		derivedBytes := repositoryDerivedBuildBytes(analyzer.output.Build)
		for _, item := range analyzer.output.Items {
			if len(item.Content) > maxContextExcerptBytes {
				t.Fatalf("item content bytes = %d, want at most %d", len(item.Content), maxContextExcerptBytes)
			}
			derivedBytes += contextItemDerivedBytes(item)
		}
		for _, relation := range analyzer.output.Relations {
			derivedBytes += len(relation.From) + len(relation.To)
		}
		if derivedBytes > maxContextOutputBytes || !analyzer.output.Truncation.Truncated || contextOmissionCountInternal(analyzer.output, ContextOmissionOutputTruncated) == 0 {
			t.Fatalf("output bytes/status = (%d, %#v), want bounded explicit truncation", derivedBytes, analyzer.output)
		}
		encoded, err := json.Marshal(analyzer.output)
		if err != nil {
			t.Fatalf("json.Marshal(GoContext): %v", err)
		}
		if len(encoded) > maxEvaluatorInputBytes {
			t.Fatalf("serialized Go context = %d bytes, want at most evaluator-input budget %d", len(encoded), maxEvaluatorInputBytes)
		}
	})

	t.Run("build metadata", func(t *testing.T) {
		analyzer := newOutputLimitAnalyzer()
		analyzer.output.Build.Modules = []ContextModule{{Path: strings.Repeat("m", maxContextOutputBytes+1)}}
		analyzer.finish()
		if analyzer.output.Status != ContextUnavailable || len(analyzer.output.Build.Modules) != 0 || contextOmissionCountInternal(analyzer.output, ContextOmissionAnalysisLimitExceeded) != 1 {
			t.Fatalf("output = %#v, want unavailable oversized build metadata", analyzer.output)
		}
	})
}

func newOutputLimitAnalyzer() *contextAnalyzer {
	return &contextAnalyzer{
		limits:             fixedContextLimits(),
		output:             &GoContext{Status: ContextComplete},
		itemCandidates:     make(map[string]contextItemCandidate),
		relationCandidates: make(map[string]contextRelationCandidate),
		omissionCounts:     make(map[ContextOmissionReason]int),
		omissionKeys:       make(map[ContextOmissionReason]map[string]struct{}),
		packages:           make(map[string]*contextPackage),
	}
}

func contextOmissionCountInternal(contextValue *GoContext, reason ContextOmissionReason) int {
	for _, omission := range contextValue.Omissions {
		if omission.Reason == reason {
			return omission.Count
		}
	}
	return 0
}

func BenchmarkPreviewGoContextAtOutputBudget(b *testing.B) {
	repo := newSnapshotRepository(b)
	writeSnapshotFile(b, repo, "go.mod", "module example.com/benchmark\n\ngo 1.21\n")
	var declarations strings.Builder
	var references strings.Builder
	declarations.WriteString("package benchmark\n\n")
	for index := 0; index < maxContextOutputItems; index++ {
		fmt.Fprintf(&declarations, "var Context%02d = %d\n", index, index)
		fmt.Fprintf(&references, " + Context%02d", index)
	}
	writeSnapshotFile(b, repo, "context.go", declarations.String())
	writeSnapshotFile(b, repo, "changed.go", "package benchmark\n\nfunc Build() int { return 0 }\n")
	runSnapshotGit(b, repo, "add", "--all")
	runSnapshotGit(b, repo, "commit", "-q", "-m", "base")
	base := strings.TrimSpace(runSnapshotGit(b, repo, "rev-parse", "HEAD"))
	writeSnapshotFile(b, repo, "changed.go", "package benchmark\n\nfunc Build() int { return 0"+references.String()+" }\n")

	b.ReportAllocs()
	b.ResetTimer()
	for index := 0; index < b.N; index++ {
		result, err := Preview(context.Background(), Request{
			Repository: repo,
			Selection:  WorkingTree(base),
			Limits: Limits{
				MaxFiles:        100,
				MaxDeclarations: 100,
				MaxExcerptBytes: 1024 * 1024,
			},
			Context: ContextGo,
		})
		if err != nil || result.GoContext == nil || result.GoContext.Status != ContextComplete {
			b.Fatalf("Preview() = (%#v, %v), want complete Go context", result.GoContext, err)
		}
	}
}
