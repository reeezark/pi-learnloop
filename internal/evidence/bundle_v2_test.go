package evidence_test

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestBuildBundleV2IncludesImportOnlyAndContextEvidence(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/bundle-v2\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "old/old.go", "package token\n\ntype Token string\n")
	writeRepositoryFile(t, repo, "new/new.go", "package token\n\ntype Token string\n")
	writeRepositoryFile(t, repo, "changed.go", "package bundlev2\n\nimport \"example.com/bundle-v2/old\"\n\nfunc Build() token.Token { return \"ok\" }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "changed.go", "package bundlev2\n\nimport \"example.com/bundle-v2/new\"\n\nfunc Build() token.Token { return \"ok\" }\n")

	result := previewGoContext(t, repo, evidence.WorkingTree(base))
	bundle, err := evidence.BuildBundleV2(result)
	if err != nil {
		t.Fatalf("BuildBundleV2() error = %v", err)
	}
	if bundle.FormatVersion != evidence.BundleFormatVersionV2 || !strings.HasPrefix(bundle.ID, "eb2-") || len(bundle.ManifestSHA256) != 64 {
		t.Fatalf("bundle identity = (%d, %q, %q), want evidence-bundle@2", bundle.FormatVersion, bundle.ID, bundle.ManifestSHA256)
	}
	if bundle.GoContext.Status != result.GoContext.Status || bundle.GoContext.ItemCount != len(result.GoContext.Items) || bundle.GoContext.RelationCount != len(result.GoContext.Relations) {
		t.Fatalf("bundle Go context = %#v, want retained preview counts and status", bundle.GoContext)
	}
	if bundle.EvidenceCount != len(bundle.Items)+len(bundle.GoContext.Items) || bundle.EvidenceCount == 0 {
		t.Fatalf("EvidenceCount = %d, want all changed and context items", bundle.EvidenceCount)
	}
	if len(bundle.Items) != 0 {
		t.Fatalf("changed declaration items = %#v, want genuinely import-only evidence", bundle.Items)
	}
	foundImport := false
	for _, item := range bundle.GoContext.Items {
		if item.Kind == evidence.ContextItemChangedImport {
			foundImport = true
			if item.Reference == "" || item.Content == "" || item.ContentBytes != len(item.Content) || len(item.ContentSHA256) != 64 {
				t.Fatalf("changed import = %#v, want citation-ready content provenance", item)
			}
		}
	}
	if !foundImport {
		t.Fatalf("Go context items = %#v, want changed_import", bundle.GoContext.Items)
	}
	encoded, err := json.Marshal(bundle)
	if err != nil {
		t.Fatalf("json.Marshal(bundle): %v", err)
	}
	if strings.Contains(string(encoded), repo) {
		t.Fatalf("bundle contains repository root: %s", encoded)
	}
}

func TestBuildBundleV2IsDeterministicAndHashesAllContextMetadata(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/bundle-v2-hash\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "dep/dep.go", "package dep\n\ntype Token string\n")
	writeRepositoryFile(t, repo, "changed.go", "package bundlehash\n\nfunc Build() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "changed.go", "package bundlehash\n\nimport \"example.com/bundle-v2-hash/dep\"\n\nfunc Build() dep.Token { return \"ok\" }\n")
	result := previewGoContext(t, repo, evidence.WorkingTree(base))

	first, err := evidence.BuildBundleV2(result)
	if err != nil {
		t.Fatalf("BuildBundleV2(first) error = %v", err)
	}
	second, err := evidence.BuildBundleV2(result)
	if err != nil {
		t.Fatalf("BuildBundleV2(second) error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("BuildBundleV2() is nondeterministic:\nfirst=%#v\nsecond=%#v", first, second)
	}

	mutated := result
	mutated.GoContext = cloneGoContextForBundleTest(result.GoContext)
	mutated.GoContext.Status = evidence.ContextPartial
	mutated.GoContext.Omissions = []evidence.ContextOmission{{Reason: evidence.ContextOmissionTypeIncomplete, Count: 1}}
	changed, err := evidence.BuildBundleV2(mutated)
	if err != nil {
		t.Fatalf("BuildBundleV2(mutated) error = %v", err)
	}
	if changed.ManifestSHA256 == first.ManifestSHA256 {
		t.Fatal("manifest hash did not change with completeness metadata")
	}
}

func TestBuildBundleV2RejectsMissingOrTamperedContext(t *testing.T) {
	result := validBundleResult()
	if _, err := evidence.BuildBundleV2(result); evidence.BundleErrorCodeOf(err) != evidence.BundleErrorInvalidResult {
		t.Fatalf("missing context error = %v, want invalid_result", err)
	}

	repo := newRepository(t)
	writeRepositoryFile(t, repo, "go.mod", "module example.com/bundle-v2-invalid\n\ngo 1.21\n")
	writeRepositoryFile(t, repo, "changed.go", "package invalid\n\nfunc Build() int { return 1 }\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "changed.go", "package invalid\n\nfunc Build() int { return 2 }\n")
	result = previewGoContext(t, repo, evidence.WorkingTree(base))
	result.GoContext = cloneGoContextForBundleTest(result.GoContext)
	result.GoContext.AppliedLimits.MaxOutputItems++
	if _, err := evidence.BuildBundleV2(result); evidence.BundleErrorCodeOf(err) != evidence.BundleErrorInvalidResult {
		t.Fatalf("tampered limits error = %v, want invalid_result", err)
	}
}

func cloneGoContextForBundleTest(value *evidence.GoContext) *evidence.GoContext {
	encoded, _ := json.Marshal(value)
	var cloned evidence.GoContext
	_ = json.Unmarshal(encoded, &cloned)
	return &cloned
}
