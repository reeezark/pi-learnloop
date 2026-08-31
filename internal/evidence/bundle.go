package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"unicode/utf8"
)

const BundleFormatVersion = 1

type BundleErrorCode string

const (
	BundleErrorUnknown              BundleErrorCode = "unknown"
	BundleErrorInvalidResult        BundleErrorCode = "invalid_result"
	BundleErrorInsufficientEvidence BundleErrorCode = "insufficient_evidence"
)

type BundleError struct {
	Code BundleErrorCode
	Err  error
}

func (err *BundleError) Error() string {
	return err.Err.Error()
}

func (err *BundleError) Unwrap() error {
	return err.Err
}

func BundleErrorCodeOf(err error) BundleErrorCode {
	var bundleError *BundleError
	if errors.As(err, &bundleError) {
		return bundleError.Code
	}
	return BundleErrorUnknown
}

type BundleItemKind string

const (
	BundleItemCode BundleItemKind = "code"
	BundleItemTest BundleItemKind = "test"
)

type Bundle struct {
	FormatVersion    int
	ID               string
	ManifestSHA256   string
	BaseRevision     string
	HeadRevision     string
	AppliedLimits    Limits
	FileCount        int
	DeclarationCount int
	EvidenceCount    int
	ApproximateBytes int
	Files            []BundleFile
	Items            []BundleItem
	Truncation       Truncation
}

type BundleFile struct {
	Path               string
	Status             FileStatus
	ChangedLines       []LineRange
	EvidenceReferences []string
	Omissions          []Omission
}

type BundleItem struct {
	Reference       string
	Kind            BundleItemKind
	Path            string
	DeclarationKind DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	ChangedLines    []LineRange
	Content         string
	ContentBytes    int
	ContentSHA256   string
	Truncated       bool
}

type bundleManifest struct {
	FormatVersion    int
	BaseRevision     string
	HeadRevision     string
	AppliedLimits    Limits
	FileCount        int
	DeclarationCount int
	EvidenceCount    int
	ApproximateBytes int
	Files            []bundleManifestFile
	Items            []bundleManifestItem
	Truncation       Truncation
}

type bundleManifestFile struct {
	Path               string
	Status             FileStatus
	ChangedLines       []LineRange
	EvidenceReferences []string
	Omissions          []Omission
}

type bundleManifestItem struct {
	Reference       string
	Kind            BundleItemKind
	Path            string
	DeclarationKind DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	ChangedLines    []LineRange
	ContentBytes    int
	ContentSHA256   string
	Truncated       bool
}

func BuildBundle(result Result) (Bundle, error) {
	if err := validateBundleResultStructure(result); err != nil {
		return Bundle{}, err
	}
	if result.AppliedLimits.MaxFiles <= 0 || result.AppliedLimits.MaxDeclarations <= 0 || result.AppliedLimits.MaxExcerptBytes <= 0 {
		return Bundle{}, invalidBundleResult("applied evidence limits must be positive")
	}
	if len(result.Files) > result.AppliedLimits.MaxFiles {
		return Bundle{}, invalidBundleResult("file count exceeds applied evidence limit")
	}
	declarationCount := 0
	excerptBytes := 0
	for _, file := range result.Files {
		declarationCount += len(file.Declarations)
		for _, declaration := range file.Declarations {
			excerptBytes += len(declaration.Excerpt)
		}
	}
	if declarationCount > result.AppliedLimits.MaxDeclarations {
		return Bundle{}, invalidBundleResult("declaration count exceeds applied evidence limit")
	}
	if excerptBytes > result.AppliedLimits.MaxExcerptBytes {
		return Bundle{}, invalidBundleResult("excerpt bytes exceed applied evidence limit")
	}
	bundle := Bundle{
		FormatVersion:    BundleFormatVersion,
		BaseRevision:     result.BaseRevision,
		HeadRevision:     result.HeadRevision,
		AppliedLimits:    result.AppliedLimits,
		FileCount:        len(result.Files),
		DeclarationCount: declarationCount,
		Files:            make([]BundleFile, 0, len(result.Files)),
		Items:            []BundleItem{},
		Truncation:       result.Truncation,
	}

	for _, file := range result.Files {
		bundleFile := BundleFile{
			Path:               file.Path,
			Status:             file.Status,
			ChangedLines:       append([]LineRange(nil), file.ChangedLines...),
			EvidenceReferences: []string{},
			Omissions:          append([]Omission(nil), file.Omissions...),
		}
		for _, declaration := range file.Declarations {
			if declaration.Excerpt == "" {
				continue
			}
			reference := fmt.Sprintf("E%03d", len(bundle.Items)+1)
			contentHash := sha256.Sum256([]byte(declaration.Excerpt))
			kind := BundleItemCode
			if strings.HasSuffix(file.Path, "_test.go") {
				kind = BundleItemTest
			}
			item := BundleItem{
				Reference:       reference,
				Kind:            kind,
				Path:            file.Path,
				DeclarationKind: declaration.Kind,
				Identity:        declaration.Identity,
				StartLine:       declaration.StartLine,
				EndLine:         declaration.EndLine,
				ChangedLines:    append([]LineRange(nil), declaration.ChangedLines...),
				Content:         declaration.Excerpt,
				ContentBytes:    len(declaration.Excerpt),
				ContentSHA256:   hex.EncodeToString(contentHash[:]),
				Truncated:       declaration.ExcerptTruncated,
			}
			bundle.Items = append(bundle.Items, item)
			bundleFile.EvidenceReferences = append(bundleFile.EvidenceReferences, reference)
			bundle.ApproximateBytes += item.ContentBytes
		}
		bundle.Files = append(bundle.Files, bundleFile)
	}
	bundle.EvidenceCount = len(bundle.Items)
	if bundle.EvidenceCount == 0 {
		return Bundle{}, &BundleError{Code: BundleErrorInsufficientEvidence, Err: errors.New("evidence preview contains no usable excerpts")}
	}

	manifestFiles := make([]bundleManifestFile, len(bundle.Files))
	for index, file := range bundle.Files {
		manifestFiles[index] = bundleManifestFile{
			Path:               file.Path,
			Status:             file.Status,
			ChangedLines:       append([]LineRange(nil), file.ChangedLines...),
			EvidenceReferences: append([]string(nil), file.EvidenceReferences...),
			Omissions:          append([]Omission(nil), file.Omissions...),
		}
	}
	manifestItems := make([]bundleManifestItem, len(bundle.Items))
	for index, item := range bundle.Items {
		manifestItems[index] = bundleManifestItem{
			Reference:       item.Reference,
			Kind:            item.Kind,
			Path:            item.Path,
			DeclarationKind: item.DeclarationKind,
			Identity:        item.Identity,
			StartLine:       item.StartLine,
			EndLine:         item.EndLine,
			ChangedLines:    append([]LineRange(nil), item.ChangedLines...),
			ContentBytes:    item.ContentBytes,
			ContentSHA256:   item.ContentSHA256,
			Truncated:       item.Truncated,
		}
	}
	manifest, err := json.Marshal(bundleManifest{
		FormatVersion:    bundle.FormatVersion,
		BaseRevision:     bundle.BaseRevision,
		HeadRevision:     bundle.HeadRevision,
		AppliedLimits:    bundle.AppliedLimits,
		FileCount:        bundle.FileCount,
		DeclarationCount: bundle.DeclarationCount,
		EvidenceCount:    bundle.EvidenceCount,
		ApproximateBytes: bundle.ApproximateBytes,
		Files:            manifestFiles,
		Items:            manifestItems,
		Truncation:       bundle.Truncation,
	})
	if err != nil {
		return Bundle{}, err
	}
	manifestHash := sha256.Sum256(manifest)
	bundle.ManifestSHA256 = hex.EncodeToString(manifestHash[:])
	bundle.ID = "eb1-" + bundle.ManifestSHA256
	return bundle, nil
}

func validateBundleResultStructure(result Result) error {
	if strings.TrimSpace(result.BaseRevision) == "" || !utf8.ValidString(result.BaseRevision) {
		return invalidBundleResult("base revision is invalid")
	}
	if strings.TrimSpace(result.HeadRevision) == "" || !utf8.ValidString(result.HeadRevision) {
		return invalidBundleResult("head revision is invalid")
	}
	if result.Truncation.OmittedFiles < 0 || result.Truncation.OmittedDeclarations < 0 || result.Truncation.OmittedExcerptBytes < 0 {
		return invalidBundleResult("truncation counts must not be negative")
	}
	hasTruncation := result.Truncation.OmittedFiles > 0 || result.Truncation.OmittedDeclarations > 0 || result.Truncation.OmittedExcerptBytes > 0
	if result.Truncation.Truncated != hasTruncation {
		return invalidBundleResult("truncation flag does not match omission counts")
	}

	seenPaths := make(map[string]struct{}, len(result.Files))
	for _, file := range result.Files {
		if !validBundlePath(file.Path) {
			return invalidBundleResult("evidence path %q is not a safe repository-relative slash path", file.Path)
		}
		if _, duplicate := seenPaths[file.Path]; duplicate {
			return invalidBundleResult("evidence path %q is duplicated", file.Path)
		}
		seenPaths[file.Path] = struct{}{}
		if !validFileStatus(file.Status) {
			return invalidBundleResult("file %q has invalid status %q", file.Path, file.Status)
		}
		if !validLineRanges(file.ChangedLines, nil) {
			return invalidBundleResult("file %q has invalid changed lines", file.Path)
		}
		for _, omission := range file.Omissions {
			if omission.Count <= 0 || !validOmissionReason(omission.Reason) {
				return invalidBundleResult("file %q has an invalid omission", file.Path)
			}
		}
		if file.Status == FileDeleted && len(file.Declarations) > 0 {
			return invalidBundleResult("deleted file %q contains declarations", file.Path)
		}
		for _, declaration := range file.Declarations {
			if !validDeclarationKind(declaration.Kind) || strings.TrimSpace(declaration.Identity) == "" || !utf8.ValidString(declaration.Identity) {
				return invalidBundleResult("file %q has invalid declaration identity", file.Path)
			}
			if declaration.StartLine <= 0 || declaration.EndLine < declaration.StartLine {
				return invalidBundleResult("declaration %q has an invalid span", declaration.Identity)
			}
			span := LineRange{Start: declaration.StartLine, End: declaration.EndLine}
			if len(declaration.ChangedLines) == 0 || !validLineRanges(declaration.ChangedLines, &span) {
				return invalidBundleResult("declaration %q has invalid changed lines", declaration.Identity)
			}
			if !utf8.ValidString(declaration.Excerpt) {
				return invalidBundleResult("declaration %q excerpt is not valid UTF-8", declaration.Identity)
			}
			if declaration.Excerpt == "" && !declaration.ExcerptTruncated {
				return invalidBundleResult("declaration %q has an empty excerpt without truncation", declaration.Identity)
			}
			if declaration.ExcerptTruncated && result.Truncation.OmittedExcerptBytes == 0 {
				return invalidBundleResult("declaration %q truncation is not reflected in the result", declaration.Identity)
			}
		}
	}
	return nil
}

func validFileStatus(status FileStatus) bool {
	switch status {
	case FileAdded, FileModified, FileDeleted:
		return true
	default:
		return false
	}
}

func validOmissionReason(reason OmissionReason) bool {
	switch reason {
	case OmissionDeletedFile, OmissionDeletedOnlyHunk, OmissionOutsideDeclaration:
		return true
	default:
		return false
	}
}

func validDeclarationKind(kind DeclarationKind) bool {
	switch kind {
	case DeclarationFunction, DeclarationMethod, DeclarationType, DeclarationInterface, DeclarationVariable, DeclarationConstant:
		return true
	default:
		return false
	}
}

func validLineRanges(ranges []LineRange, bounds *LineRange) bool {
	for index, lineRange := range ranges {
		if lineRange.Start <= 0 || lineRange.End < lineRange.Start {
			return false
		}
		if index > 0 && lineRange.Start <= ranges[index-1].End {
			return false
		}
		if bounds != nil && (lineRange.Start < bounds.Start || lineRange.End > bounds.End) {
			return false
		}
	}
	return true
}

func invalidBundleResult(message string, arguments ...any) error {
	return &BundleError{Code: BundleErrorInvalidResult, Err: fmt.Errorf(message, arguments...)}
}

func validBundlePath(value string) bool {
	return value != "" &&
		utf8.ValidString(value) &&
		!path.IsAbs(value) &&
		!strings.Contains(value, `\`) &&
		path.Clean(value) == value &&
		value != "." &&
		value != ".." &&
		!strings.HasPrefix(value, "../")
}
