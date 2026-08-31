package evidence_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

func TestBuildBundleProducesCitationReadyEvidence(t *testing.T) {
	result := validBundleResult()

	got, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle() error = %v", err)
	}

	if got.FormatVersion != evidence.BundleFormatVersion {
		t.Fatalf("FormatVersion = %d, want %d", got.FormatVersion, evidence.BundleFormatVersion)
	}
	if !strings.HasPrefix(got.ID, "eb1-") || len(got.ManifestSHA256) != 64 {
		t.Fatalf("bundle identity = (%q, %q), want eb1 prefix and SHA-256 manifest", got.ID, got.ManifestSHA256)
	}
	if got.BaseRevision != result.BaseRevision || got.HeadRevision != result.HeadRevision {
		t.Fatalf("revisions = (%q, %q), want (%q, %q)", got.BaseRevision, got.HeadRevision, result.BaseRevision, result.HeadRevision)
	}
	if got.AppliedLimits != result.AppliedLimits {
		t.Fatalf("AppliedLimits = %#v, want %#v", got.AppliedLimits, result.AppliedLimits)
	}
	if got.FileCount != 1 || got.DeclarationCount != 1 || got.EvidenceCount != 1 {
		t.Fatalf("counts = (%d files, %d declarations, %d evidence), want (1, 1, 1)", got.FileCount, got.DeclarationCount, got.EvidenceCount)
	}
	if len(got.Items) != 1 {
		t.Fatalf("Items = %#v, want one item", got.Items)
	}
	item := got.Items[0]
	if item.Reference != "E001" || item.Kind != evidence.BundleItemCode {
		t.Fatalf("item citation = (%q, %q), want (E001, code)", item.Reference, item.Kind)
	}
	if item.Path != "internal/answer.go" || item.Identity != "Answer" || item.StartLine != 3 || item.EndLine != 5 {
		t.Fatalf("item locator = %#v, want Answer at internal/answer.go:3-5", item)
	}
	if item.Content != result.Files[0].Declarations[0].Excerpt {
		t.Fatalf("Content = %q, want exact preview excerpt", item.Content)
	}
	wantContentHash := sha256.Sum256([]byte(item.Content))
	if item.ContentBytes != len(item.Content) || item.ContentSHA256 != hex.EncodeToString(wantContentHash[:]) {
		t.Fatalf("item content provenance = (%d, %q), want (%d, %q)", item.ContentBytes, item.ContentSHA256, len(item.Content), hex.EncodeToString(wantContentHash[:]))
	}
	if got.ApproximateBytes != len(item.Content) {
		t.Fatalf("ApproximateBytes = %d, want %d", got.ApproximateBytes, len(item.Content))
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(bundle): %v", err)
	}
	if strings.Contains(string(encoded), result.RepositoryRoot) {
		t.Fatalf("bundle contains absolute repository root: %s", encoded)
	}
}

func TestBuildBundleFailsClosedWithoutAppliedBudget(t *testing.T) {
	result := validBundleResult()
	result.AppliedLimits = evidence.Limits{}

	got, err := evidence.BuildBundle(result)
	if err == nil {
		t.Fatalf("BuildBundle() error = nil, want invalid result")
	}
	if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
		t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
	}
	if !reflect.DeepEqual(got, evidence.Bundle{}) {
		t.Fatalf("BuildBundle() bundle = %#v, want zero value on error", got)
	}
}

func TestBuildBundleRejectsFileCountOverAppliedBudget(t *testing.T) {
	result := validBundleResult()
	result.AppliedLimits.MaxFiles = 1
	result.Files = append(result.Files, evidence.File{
		Path:   "internal/extra.go",
		Status: evidence.FileAdded,
	})

	_, err := evidence.BuildBundle(result)
	if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
		t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
	}
}

func TestBuildBundleRejectsDeclarationCountOverAppliedBudget(t *testing.T) {
	result := validBundleResult()
	result.AppliedLimits.MaxDeclarations = 1
	result.Files[0].Declarations = append(result.Files[0].Declarations, evidence.Declaration{
		Kind:         evidence.DeclarationFunction,
		Name:         "Extra",
		Identity:     "Extra",
		StartLine:    7,
		EndLine:      7,
		ChangedLines: []evidence.LineRange{{Start: 7, End: 7}},
		Excerpt:      "func Extra() {}",
	})

	_, err := evidence.BuildBundle(result)
	if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
		t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
	}
}

func TestBuildBundleRejectsExcerptBytesOverAppliedBudget(t *testing.T) {
	result := validBundleResult()
	result.AppliedLimits.MaxExcerptBytes = len(result.Files[0].Declarations[0].Excerpt) - 1

	_, err := evidence.BuildBundle(result)
	if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
		t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
	}
}

func TestBuildBundleRejectsUnsafeEvidencePaths(t *testing.T) {
	paths := []string{
		"",
		"/private/source.go",
		"../source.go",
		"internal/../source.go",
		`internal\source.go`,
		string([]byte{0xff}),
	}
	for _, unsafePath := range paths {
		t.Run(strings.ReplaceAll(unsafePath, "/", "_"), func(t *testing.T) {
			result := validBundleResult()
			result.Files[0].Path = unsafePath

			_, err := evidence.BuildBundle(result)
			if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
				t.Fatalf("BundleErrorCodeOf(error) = %q, want %q for path %q", code, evidence.BundleErrorInvalidResult, unsafePath)
			}
		})
	}
}

func TestBuildBundleRejectsMalformedPreviewStructure(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*evidence.Result)
	}{
		{name: "missing base revision", mutate: func(result *evidence.Result) { result.BaseRevision = "" }},
		{name: "missing head revision", mutate: func(result *evidence.Result) { result.HeadRevision = "" }},
		{name: "duplicate file path", mutate: func(result *evidence.Result) { result.Files = append(result.Files, result.Files[0]) }},
		{name: "unknown file status", mutate: func(result *evidence.Result) { result.Files[0].Status = "copied" }},
		{name: "invalid file range", mutate: func(result *evidence.Result) { result.Files[0].ChangedLines[0].Start = 0 }},
		{name: "unordered file ranges", mutate: func(result *evidence.Result) {
			result.Files[0].ChangedLines = []evidence.LineRange{{Start: 5, End: 5}, {Start: 4, End: 4}}
		}},
		{name: "unknown omission", mutate: func(result *evidence.Result) {
			result.Files[0].Omissions = []evidence.Omission{{Reason: "unknown", Count: 1}}
		}},
		{name: "non-positive omission", mutate: func(result *evidence.Result) {
			result.Files[0].Omissions = []evidence.Omission{{Reason: evidence.OmissionOutsideDeclaration, Count: 0}}
		}},
		{name: "unknown declaration kind", mutate: func(result *evidence.Result) { result.Files[0].Declarations[0].Kind = "package" }},
		{name: "missing identity", mutate: func(result *evidence.Result) { result.Files[0].Declarations[0].Identity = "" }},
		{name: "invalid declaration span", mutate: func(result *evidence.Result) { result.Files[0].Declarations[0].EndLine = 0 }},
		{name: "changed line outside declaration", mutate: func(result *evidence.Result) {
			result.Files[0].Declarations[0].ChangedLines = []evidence.LineRange{{Start: 2, End: 2}}
		}},
		{name: "invalid excerpt UTF-8", mutate: func(result *evidence.Result) {
			result.Files[0].Declarations[0].Excerpt = string([]byte{0xff})
		}},
		{name: "negative truncation", mutate: func(result *evidence.Result) { result.Truncation.OmittedFiles = -1 }},
		{name: "inconsistent truncation", mutate: func(result *evidence.Result) { result.Truncation.OmittedFiles = 1 }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validBundleResult()
			test.mutate(&result)

			_, err := evidence.BuildBundle(result)
			if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
				t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
			}
		})
	}
}

func TestBuildBundleFailsClosedWithoutUsableEvidence(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*evidence.Result)
	}{
		{name: "no changed files", mutate: func(result *evidence.Result) { result.Files = nil }},
		{name: "excerpt fully truncated", mutate: func(result *evidence.Result) {
			result.Files[0].Declarations[0].Excerpt = ""
			result.Files[0].Declarations[0].ExcerptTruncated = true
			result.Truncation = evidence.Truncation{Truncated: true, OmittedExcerptBytes: 32}
		}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result := validBundleResult()
			test.mutate(&result)

			got, err := evidence.BuildBundle(result)
			if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInsufficientEvidence {
				t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInsufficientEvidence)
			}
			if !reflect.DeepEqual(got, evidence.Bundle{}) {
				t.Fatalf("BuildBundle() bundle = %#v, want zero value on error", got)
			}
		})
	}
}

func TestBuildBundleRejectsEmptyExcerptWithoutTruncationEvidence(t *testing.T) {
	result := validBundleResult()
	result.Files[0].Declarations[0].Excerpt = ""

	_, err := evidence.BuildBundle(result)
	if code := evidence.BundleErrorCodeOf(err); code != evidence.BundleErrorInvalidResult {
		t.Fatalf("BundleErrorCodeOf(error) = %q, want %q", code, evidence.BundleErrorInvalidResult)
	}
}

func TestBuildBundleManifestIsDeterministicAndCoversProvenance(t *testing.T) {
	result := validBundleResult()
	result.Files[0].Omissions = []evidence.Omission{{Reason: evidence.OmissionOutsideDeclaration, Count: 1}}
	result.Files = append(result.Files, evidence.File{
		Path:         "internal/answer_test.go",
		Status:       evidence.FileAdded,
		ChangedLines: []evidence.LineRange{{Start: 1, End: 3}},
		Declarations: []evidence.Declaration{
			{
				Kind:         evidence.DeclarationFunction,
				Name:         "TestAnswer",
				Identity:     "TestAnswer",
				StartLine:    1,
				EndLine:      3,
				ChangedLines: []evidence.LineRange{{Start: 1, End: 3}},
				Excerpt:      "func TestAnswer(t *testing.T) {}",
			},
		},
	})

	first, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle() first error = %v", err)
	}
	second, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle() second error = %v", err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated BuildBundle() results differ:\nfirst: %#v\nsecond: %#v", first, second)
	}
	if got := []string{first.Items[0].Reference, first.Items[1].Reference}; !reflect.DeepEqual(got, []string{"E001", "E002"}) {
		t.Fatalf("references = %v, want [E001 E002]", got)
	}
	if first.Items[0].Kind != evidence.BundleItemCode || first.Items[1].Kind != evidence.BundleItemTest {
		t.Fatalf("item kinds = [%q %q], want [code test]", first.Items[0].Kind, first.Items[1].Kind)
	}
	if !reflect.DeepEqual(first.Files[0].Omissions, result.Files[0].Omissions) || !reflect.DeepEqual(first.Files[1].EvidenceReferences, []string{"E002"}) {
		t.Fatalf("file provenance = %#v, want omissions and E002 test reference", first.Files)
	}

	differentRoot := result
	differentRoot.RepositoryRoot = "/another/private/root"
	rootBundle, err := evidence.BuildBundle(differentRoot)
	if err != nil {
		t.Fatalf("BuildBundle() different root error = %v", err)
	}
	if rootBundle.ManifestSHA256 != first.ManifestSHA256 {
		t.Fatalf("manifest changed with excluded repository root: %q != %q", rootBundle.ManifestSHA256, first.ManifestSHA256)
	}

	differentContent := validBundleResult()
	differentContent.Files[0].Declarations[0].Excerpt = strings.Replace(differentContent.Files[0].Declarations[0].Excerpt, "42", "43", 1)
	contentBundle, err := evidence.BuildBundle(differentContent)
	if err != nil {
		t.Fatalf("BuildBundle() different content error = %v", err)
	}
	baseBundle, err := evidence.BuildBundle(validBundleResult())
	if err != nil {
		t.Fatalf("BuildBundle() base error = %v", err)
	}
	if contentBundle.ManifestSHA256 == baseBundle.ManifestSHA256 {
		t.Fatalf("manifest hash did not change with excerpt content")
	}
	differentMetadata := validBundleResult()
	differentMetadata.Files[0].Declarations[0].Identity = "RenamedAnswer"
	metadataBundle, err := evidence.BuildBundle(differentMetadata)
	if err != nil {
		t.Fatalf("BuildBundle() different metadata error = %v", err)
	}
	if metadataBundle.ManifestSHA256 == baseBundle.ManifestSHA256 {
		t.Fatalf("manifest hash did not change with covered metadata")
	}
	truncated := validBundleResult()
	truncated.Files[0].Declarations[0].ExcerptTruncated = true
	truncated.Truncation = evidence.Truncation{Truncated: true, OmittedExcerptBytes: 1}
	truncatedBundle, err := evidence.BuildBundle(truncated)
	if err != nil {
		t.Fatalf("BuildBundle() truncation metadata error = %v", err)
	}
	if truncatedBundle.ManifestSHA256 == baseBundle.ManifestSHA256 {
		t.Fatalf("manifest hash did not change with truncation metadata")
	}
}

func TestBuildBundleReportsDeclarationsWhoseContentWasFullyTruncated(t *testing.T) {
	result := validBundleResult()
	result.Files[0].Declarations = append(result.Files[0].Declarations, evidence.Declaration{
		Kind:             evidence.DeclarationFunction,
		Name:             "Hidden",
		Identity:         "Hidden",
		StartLine:        7,
		EndLine:          8,
		ChangedLines:     []evidence.LineRange{{Start: 7, End: 8}},
		Excerpt:          "",
		ExcerptTruncated: true,
	})
	result.Truncation = evidence.Truncation{Truncated: true, OmittedExcerptBytes: 24}

	got, err := evidence.BuildBundle(result)
	if err != nil {
		t.Fatalf("BuildBundle() error = %v", err)
	}
	if got.DeclarationCount != 2 || got.EvidenceCount != 1 {
		t.Fatalf("counts = (%d declarations, %d evidence), want (2, 1)", got.DeclarationCount, got.EvidenceCount)
	}
	if !reflect.DeepEqual(got.Files[0].EvidenceReferences, []string{"E001"}) {
		t.Fatalf("EvidenceReferences = %v, want [E001]", got.Files[0].EvidenceReferences)
	}
	if got.Truncation != result.Truncation {
		t.Fatalf("Truncation = %#v, want %#v", got.Truncation, result.Truncation)
	}
}

func TestBuildBundleNormalizesEmptyCollectionsForManifestHash(t *testing.T) {
	nilCollections := validBundleResult()
	emptyCollections := validBundleResult()
	emptyCollections.Files[0].Omissions = []evidence.Omission{}

	withNil, err := evidence.BuildBundle(nilCollections)
	if err != nil {
		t.Fatalf("BuildBundle() nil collections error = %v", err)
	}
	withEmpty, err := evidence.BuildBundle(emptyCollections)
	if err != nil {
		t.Fatalf("BuildBundle() empty collections error = %v", err)
	}
	if withNil.ManifestSHA256 != withEmpty.ManifestSHA256 || withNil.ID != withEmpty.ID {
		t.Fatalf("equivalent empty collections changed identity: (%q, %q) != (%q, %q)", withNil.ManifestSHA256, withNil.ID, withEmpty.ManifestSHA256, withEmpty.ID)
	}
}

func TestBundleErrorCodeOfReturnsUnknownForUnclassifiedErrors(t *testing.T) {
	if got := evidence.BundleErrorCodeOf(errors.New("unclassified")); got != evidence.BundleErrorUnknown {
		t.Fatalf("BundleErrorCodeOf(unclassified) = %q, want %q", got, evidence.BundleErrorUnknown)
	}
}

func TestPreviewResultBuildsBundleFromExactlyBoundedEvidence(t *testing.T) {
	repo := newRepository(t)
	writeRepositoryFile(t, repo, "answer.go", "package sample\n")
	commitAll(t, repo, "base")
	base := revision(t, repo, "HEAD")
	writeRepositoryFile(t, repo, "answer.go", `package sample

func Answer() int {
	return 42
}
`)
	commitAll(t, repo, "add Answer")
	limits := evidence.Limits{MaxFiles: 1, MaxDeclarations: 1, MaxExcerptBytes: 20}
	preview, err := evidence.Preview(context.Background(), evidence.Request{
		Repository: repo,
		Selection:  evidence.CommitRange(base, "HEAD"),
		Limits:     limits,
	})
	if err != nil {
		t.Fatalf("Preview() error = %v", err)
	}

	bundle, err := evidence.BuildBundle(preview)
	if err != nil {
		t.Fatalf("BuildBundle() error = %v", err)
	}
	if bundle.AppliedLimits != limits {
		t.Fatalf("AppliedLimits = %#v, want %#v", bundle.AppliedLimits, limits)
	}
	if len(bundle.Items) != 1 || bundle.Items[0].Content != preview.Files[0].Declarations[0].Excerpt {
		t.Fatalf("bundle items = %#v, want exact bounded preview excerpt", bundle.Items)
	}
	if !bundle.Items[0].Truncated || bundle.ApproximateBytes != len(preview.Files[0].Declarations[0].Excerpt) {
		t.Fatalf("bounded content = (truncated:%v, bytes:%d), want truncated preview with %d bytes", bundle.Items[0].Truncated, bundle.ApproximateBytes, len(preview.Files[0].Declarations[0].Excerpt))
	}
}

func validBundleResult() evidence.Result {
	return evidence.Result{
		RepositoryRoot: "/Users/example/private-repository",
		BaseRevision:   strings.Repeat("a", 40),
		HeadRevision:   strings.Repeat("b", 40),
		AppliedLimits: evidence.Limits{
			MaxFiles:        2,
			MaxDeclarations: 4,
			MaxExcerptBytes: 1024,
		},
		Files: []evidence.File{
			{
				Path:         "internal/answer.go",
				Status:       evidence.FileModified,
				ChangedLines: []evidence.LineRange{{Start: 3, End: 4}},
				Declarations: []evidence.Declaration{
					{
						Kind:         evidence.DeclarationFunction,
						Name:         "Answer",
						Identity:     "Answer",
						StartLine:    3,
						EndLine:      5,
						ChangedLines: []evidence.LineRange{{Start: 3, End: 4}},
						Excerpt:      "func Answer() int {\n\treturn 42\n}",
					},
				},
			},
		},
	}
}
