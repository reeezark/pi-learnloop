package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode/utf8"
)

const BundleFormatVersionV2 = 2

// BundleV2 is the pure, snapshot-retained evaluator bundle for enriched Go
// evidence. It intentionally contains no repository root or Session provenance.
type BundleV2 struct {
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
	GoContext        BundleGoContext
	Truncation       Truncation
}

type BundleGoContext struct {
	Status               ContextStatus
	Build                BundleGoBuildConfiguration
	AppliedLimits        BundleContextLimits
	AnalyzedPackageCount int
	AnalyzedFileCount    int
	AnalyzedSourceBytes  int
	DirectImportEdges    int
	ItemCount            int
	RelationCount        int
	ApproximateBytes     int
	Items                []BundleContextItem
	Relations            []ContextRelation
	Omissions            []ContextOmission
	Truncation           ContextTruncation
}

type BundleContextLimits struct {
	MaxChangedFiles        int
	MaxModuleRoots         int
	MaxPackages            int
	MaxFilesPerPackage     int
	MaxFiles               int
	MaxDirectoryEntries    int
	MaxSourceBytesPerFile  int
	MaxSourceBytes         int
	MaxDirectImportEdges   int
	AnalysisTimeoutMillis  int64
	MaxOutputFiles         int
	MaxOutputItems         int
	MaxRelations           int
	MaxExcerptBytes        int
	MaxOutputBytes         int
	MaxEvaluatorInputBytes int
}

type BundleGoBuildConfiguration struct {
	GOOS             string
	GOARCH           string
	CGOEnabled       bool
	BuildTags        []string
	ToolTags         []string
	ReleaseTags      []string
	ToolchainVersion string
	TestVariant      bool
	Modules          []ContextModule
	Workspaces       []ContextWorkspace
	Replacements     []ContextReplacement
}

type BundleContextItem struct {
	Reference       string
	Kind            ContextItemKind
	Path            string
	PackagePath     string
	DeclarationKind DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	Content         string
	ContentBytes    int
	ContentSHA256   string
	Truncated       bool
}

type bundleV2Manifest struct {
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
	GoContext        bundleV2ContextManifest
	Truncation       Truncation
}

type bundleV2ContextManifest struct {
	Status               ContextStatus
	Build                BundleGoBuildConfiguration
	AppliedLimits        BundleContextLimits
	AnalyzedPackageCount int
	AnalyzedFileCount    int
	AnalyzedSourceBytes  int
	DirectImportEdges    int
	ItemCount            int
	RelationCount        int
	ApproximateBytes     int
	Items                []bundleV2ContextManifestItem
	Relations            []ContextRelation
	Omissions            []ContextOmission
	Truncation           ContextTruncation
}

type bundleV2ContextManifestItem struct {
	Reference       string
	Kind            ContextItemKind
	Path            string
	PackagePath     string
	DeclarationKind DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	ContentBytes    int
	ContentSHA256   string
	Truncated       bool
}

// BuildBundleV2 constructs evidence-bundle@2 solely from the retained enriched
// preview. It performs no repository, filesystem, Git, process, or network work.
func BuildBundleV2(result Result) (BundleV2, error) {
	if result.GoContext == nil {
		return BundleV2{}, invalidBundleResult("evidence bundle v2 requires Go context")
	}
	if err := validateBundleResultStructure(result); err != nil {
		return BundleV2{}, err
	}
	if err := validateChangedBudgets(result); err != nil {
		return BundleV2{}, err
	}
	if err := validateGoContext(result.GoContext); err != nil {
		return BundleV2{}, err
	}

	bundle := BundleV2{
		FormatVersion: BundleFormatVersionV2,
		BaseRevision:  result.BaseRevision,
		HeadRevision:  result.HeadRevision,
		AppliedLimits: result.AppliedLimits,
		FileCount:     len(result.Files),
		Files:         make([]BundleFile, 0, len(result.Files)),
		Items:         []BundleItem{},
		Truncation:    result.Truncation,
	}
	for _, file := range result.Files {
		bundle.DeclarationCount += len(file.Declarations)
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

	bundle.GoContext = copyBundleGoContext(*result.GoContext)
	bundle.EvidenceCount = len(bundle.Items) + len(bundle.GoContext.Items)
	bundle.ApproximateBytes += bundle.GoContext.ApproximateBytes
	if bundle.EvidenceCount == 0 {
		return BundleV2{}, &BundleError{Code: BundleErrorInsufficientEvidence, Err: errors.New("evidence preview contains no usable excerpts")}
	}

	manifest, err := bundleV2ManifestBytes(bundle)
	if err != nil {
		return BundleV2{}, err
	}
	manifestHash := sha256.Sum256(manifest)
	bundle.ManifestSHA256 = hex.EncodeToString(manifestHash[:])
	bundle.ID = "eb2-" + bundle.ManifestSHA256
	return bundle, nil
}

// ValidateBundleV2 verifies a detached evidence-bundle@2 value, including its
// content hashes and manifest identity, without reading any external state.
func ValidateBundleV2(bundle BundleV2) error {
	if bundle.FormatVersion != BundleFormatVersionV2 || !validLowerHexSHA256(bundle.ManifestSHA256) || bundle.ID != "eb2-"+bundle.ManifestSHA256 {
		return invalidBundleResult("evidence bundle v2 identity is invalid")
	}
	if strings.TrimSpace(bundle.BaseRevision) == "" || !utf8.ValidString(bundle.BaseRevision) ||
		strings.TrimSpace(bundle.HeadRevision) == "" || !utf8.ValidString(bundle.HeadRevision) {
		return invalidBundleResult("evidence bundle v2 revisions are invalid")
	}
	if bundle.AppliedLimits.MaxFiles <= 0 || bundle.AppliedLimits.MaxDeclarations <= 0 || bundle.AppliedLimits.MaxExcerptBytes <= 0 ||
		bundle.FileCount != len(bundle.Files) || bundle.FileCount > bundle.AppliedLimits.MaxFiles ||
		bundle.DeclarationCount < len(bundle.Items) || bundle.DeclarationCount > bundle.AppliedLimits.MaxDeclarations {
		return invalidBundleResult("evidence bundle v2 changed-evidence counts are invalid")
	}
	if bundle.EvidenceCount != len(bundle.Items)+len(bundle.GoContext.Items) || bundle.EvidenceCount == 0 ||
		bundle.ApproximateBytes <= 0 || !validBundleTruncation(bundle.Truncation) {
		return invalidBundleResult("evidence bundle v2 totals are invalid")
	}

	files := make(map[string]BundleFile, len(bundle.Files))
	referenceOwners := make(map[string]string, bundle.EvidenceCount)
	for _, file := range bundle.Files {
		if !validBundlePath(file.Path) || !validFileStatus(file.Status) || !validLineRanges(file.ChangedLines, nil) {
			return invalidBundleResult("evidence bundle v2 file metadata is invalid")
		}
		if _, duplicate := files[file.Path]; duplicate {
			return invalidBundleResult("evidence bundle v2 file path is duplicated")
		}
		for _, omission := range file.Omissions {
			if omission.Count <= 0 || !validOmissionReason(omission.Reason) {
				return invalidBundleResult("evidence bundle v2 file omission is invalid")
			}
		}
		seen := make(map[string]struct{}, len(file.EvidenceReferences))
		for _, reference := range file.EvidenceReferences {
			if reference == "" {
				return invalidBundleResult("evidence bundle v2 file reference is empty")
			}
			if _, duplicate := seen[reference]; duplicate {
				return invalidBundleResult("evidence bundle v2 file reference is duplicated")
			}
			seen[reference] = struct{}{}
			if _, duplicate := referenceOwners[reference]; duplicate {
				return invalidBundleResult("evidence bundle v2 reference has multiple owners")
			}
			referenceOwners[reference] = file.Path
		}
		files[file.Path] = file
	}

	changedBytes := 0
	for index, item := range bundle.Items {
		expected := fmt.Sprintf("E%03d", index+1)
		if item.Reference != expected || referenceOwners[item.Reference] != item.Path || !validBundleItem(item, files) {
			return invalidBundleResult("evidence bundle v2 changed item is invalid")
		}
		changedBytes += item.ContentBytes
	}
	if len(referenceOwners) != len(bundle.Items) || changedBytes > bundle.AppliedLimits.MaxExcerptBytes {
		return invalidBundleResult("evidence bundle v2 changed references or bytes are inconsistent")
	}

	contextValue, err := resultGoContextFromBundle(bundle.GoContext)
	if err != nil {
		return err
	}
	if err := validateGoContext(contextValue); err != nil {
		return err
	}
	if bundle.GoContext.ItemCount != len(bundle.GoContext.Items) || bundle.GoContext.RelationCount != len(bundle.GoContext.Relations) ||
		bundle.GoContext.ApproximateBytes != contextApproximateBytes(*contextValue) ||
		bundle.ApproximateBytes != changedBytes+bundle.GoContext.ApproximateBytes {
		return invalidBundleResult("evidence bundle v2 context totals are inconsistent")
	}
	for _, item := range bundle.GoContext.Items {
		if _, duplicate := referenceOwners[item.Reference]; duplicate {
			return invalidBundleResult("evidence bundle v2 reference is duplicated")
		}
		referenceOwners[item.Reference] = item.Path
		if !validLowerHexSHA256(item.ContentSHA256) {
			return invalidBundleResult("evidence bundle v2 context content hash is invalid")
		}
		hash := sha256.Sum256([]byte(item.Content))
		if item.ContentSHA256 != hex.EncodeToString(hash[:]) {
			return invalidBundleResult("evidence bundle v2 context content hash is invalid")
		}
	}
	if len(referenceOwners) != bundle.EvidenceCount {
		return invalidBundleResult("evidence bundle v2 reference count is inconsistent")
	}
	manifest, err := bundleV2ManifestBytes(bundle)
	if err != nil {
		return invalidBundleResult("evidence bundle v2 manifest cannot be encoded")
	}
	hash := sha256.Sum256(manifest)
	if bundle.ManifestSHA256 != hex.EncodeToString(hash[:]) {
		return invalidBundleResult("evidence bundle v2 manifest hash is invalid")
	}
	return nil
}

func validBundleTruncation(value Truncation) bool {
	if value.OmittedFiles < 0 || value.OmittedDeclarations < 0 || value.OmittedExcerptBytes < 0 {
		return false
	}
	hasCounts := value.OmittedFiles > 0 || value.OmittedDeclarations > 0 || value.OmittedExcerptBytes > 0
	return value.Truncated == hasCounts
}

func validBundleItem(item BundleItem, files map[string]BundleFile) bool {
	if _, exists := files[item.Path]; !exists || !validDeclarationKind(item.DeclarationKind) ||
		strings.TrimSpace(item.Identity) == "" || !utf8.ValidString(item.Identity) ||
		item.StartLine <= 0 || item.EndLine < item.StartLine || len(item.ChangedLines) == 0 ||
		!validLineRanges(item.ChangedLines, &LineRange{Start: item.StartLine, End: item.EndLine}) ||
		item.Content == "" || !utf8.ValidString(item.Content) || item.ContentBytes != len(item.Content) ||
		!validLowerHexSHA256(item.ContentSHA256) {
		return false
	}
	wantKind := BundleItemCode
	if strings.HasSuffix(item.Path, "_test.go") {
		wantKind = BundleItemTest
	}
	if item.Kind != wantKind {
		return false
	}
	hash := sha256.Sum256([]byte(item.Content))
	return item.ContentSHA256 == hex.EncodeToString(hash[:])
}

func resultGoContextFromBundle(value BundleGoContext) (*GoContext, error) {
	items := make([]ContextItem, len(value.Items))
	for index, item := range value.Items {
		items[index] = ContextItem{
			ID: item.Reference, Kind: item.Kind, Path: item.Path, PackagePath: item.PackagePath,
			DeclarationKind: item.DeclarationKind, Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine,
			Content: item.Content, ContentBytes: item.ContentBytes, Truncated: item.Truncated,
		}
	}
	contextLimits := fixedContextLimits()
	if !reflect.DeepEqual(value.AppliedLimits, copyBundleContextLimits(contextLimits)) {
		return nil, invalidBundleResult("evidence bundle v2 context limits are invalid")
	}
	build := GoBuildConfiguration{
		GOOS: value.Build.GOOS, GOARCH: value.Build.GOARCH, CGOEnabled: value.Build.CGOEnabled,
		BuildTags: copyExplicitSlice(value.Build.BuildTags), ToolTags: copyExplicitSlice(value.Build.ToolTags),
		ReleaseTags: copyExplicitSlice(value.Build.ReleaseTags), ToolchainVersion: value.Build.ToolchainVersion,
		TestVariant: value.Build.TestVariant, Modules: copyExplicitSlice(value.Build.Modules),
		Workspaces: copyExplicitSlice(value.Build.Workspaces), Replacements: copyExplicitSlice(value.Build.Replacements),
	}
	return &GoContext{
		Status: value.Status, Build: build, AppliedLimits: contextLimits,
		AnalyzedPackageCount: value.AnalyzedPackageCount, AnalyzedFileCount: value.AnalyzedFileCount,
		AnalyzedSourceBytes: value.AnalyzedSourceBytes, DirectImportEdges: value.DirectImportEdges,
		ApproximateBytes: value.ApproximateBytes,
		Items:            items, Relations: copyExplicitSlice(value.Relations),
		Omissions: copyExplicitSlice(value.Omissions), Truncation: value.Truncation,
	}, nil
}

func validLowerHexSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validateChangedBudgets(result Result) error {
	if result.AppliedLimits.MaxFiles <= 0 || result.AppliedLimits.MaxDeclarations <= 0 || result.AppliedLimits.MaxExcerptBytes <= 0 {
		return invalidBundleResult("applied evidence limits must be positive")
	}
	if len(result.Files) > result.AppliedLimits.MaxFiles {
		return invalidBundleResult("file count exceeds applied evidence limit")
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
		return invalidBundleResult("declaration count exceeds applied evidence limit")
	}
	if excerptBytes > result.AppliedLimits.MaxExcerptBytes {
		return invalidBundleResult("excerpt bytes exceed applied evidence limit")
	}
	return nil
}

func validateGoContext(value *GoContext) error {
	if value == nil || !reflect.DeepEqual(value.AppliedLimits, fixedContextLimits()) {
		return invalidBundleResult("Go context limits are invalid")
	}
	limits := value.AppliedLimits
	if value.AnalyzedPackageCount < 0 || value.AnalyzedPackageCount > limits.MaxPackages ||
		value.AnalyzedFileCount < 0 || value.AnalyzedFileCount > limits.MaxFiles ||
		value.AnalyzedSourceBytes < 0 || value.AnalyzedSourceBytes > limits.MaxSourceBytes ||
		value.DirectImportEdges < 0 || value.DirectImportEdges > limits.MaxDirectImportEdges {
		return invalidBundleResult("Go context analysis counts are invalid")
	}
	if err := validateGoBuild(value.Build, limits); err != nil {
		return err
	}
	if value.Items == nil || value.Relations == nil || value.Omissions == nil {
		return invalidBundleResult("Go context collections must be explicit")
	}
	if len(value.Items) > limits.MaxOutputItems || len(value.Relations) > limits.MaxRelations {
		return invalidBundleResult("Go context output exceeds applied limits")
	}

	paths := make(map[string]struct{})
	references := make(map[string]struct{}, len(value.Items))
	for index, item := range value.Items {
		expected := fmt.Sprintf("C%03d", index+1)
		if item.ID != expected || !validBundlePath(item.Path) || !validContextText(item.PackagePath) ||
			!validContextText(item.Identity) || item.StartLine <= 0 || item.EndLine < item.StartLine ||
			item.Content == "" || !utf8.ValidString(item.Content) || item.ContentBytes != len(item.Content) ||
			item.ContentBytes > limits.MaxExcerptBytes {
			return invalidBundleResult("Go context item is invalid")
		}
		switch item.Kind {
		case ContextItemChangedImport:
			if item.DeclarationKind != "" {
				return invalidBundleResult("changed import has a declaration kind")
			}
		case ContextItemContextDeclaration:
			if !validDeclarationKind(item.DeclarationKind) {
				return invalidBundleResult("context declaration kind is invalid")
			}
		default:
			return invalidBundleResult("Go context item kind is invalid")
		}
		paths[item.Path] = struct{}{}
		references[item.ID] = struct{}{}
	}
	if len(paths) > limits.MaxOutputFiles {
		return invalidBundleResult("Go context file count exceeds applied limit")
	}
	for _, relation := range value.Relations {
		if !validContextText(relation.From) || !validContextText(relation.To) {
			return invalidBundleResult("Go context relation endpoint is invalid")
		}
		switch relation.Kind {
		case ContextRelationImports:
			if relation.Strength != ContextRelationSyntactic {
				return invalidBundleResult("Go context import strength is invalid")
			}
		case ContextRelationReferences, ContextRelationImplements:
			if relation.Strength != ContextRelationTypeChecked {
				return invalidBundleResult("Go context typed relation strength is invalid")
			}
		default:
			return invalidBundleResult("Go context relation kind is invalid")
		}
		if strings.HasPrefix(relation.To, "C") {
			if _, ok := references[relation.To]; !ok {
				return invalidBundleResult("Go context relation references an unknown item")
			}
		}
	}
	if !validContextTruncation(value.Truncation) || !validContextOmissions(value.Status, value.Omissions, value.Truncation) {
		return invalidBundleResult("Go context completeness metadata is invalid")
	}
	if value.Status == ContextUnavailable && (len(value.Items) != 0 || len(value.Relations) != 0) {
		return invalidBundleResult("unavailable Go context contains evidence")
	}
	if value.ApproximateBytes != contextApproximateBytes(*value) || value.ApproximateBytes > limits.MaxOutputBytes {
		return invalidBundleResult("Go context byte count exceeds applied limit")
	}
	return nil
}

func validateGoBuild(value GoBuildConfiguration, limits ContextLimits) error {
	if !validContextText(value.GOOS) || !validContextText(value.GOARCH) || value.CGOEnabled ||
		value.BuildTags == nil || len(value.BuildTags) != 0 || value.ToolTags == nil ||
		value.ReleaseTags == nil || !validContextText(value.ToolchainVersion) ||
		value.Modules == nil || value.Workspaces == nil || value.Replacements == nil ||
		len(value.Modules)+len(value.Workspaces) > limits.MaxModuleRoots {
		return invalidBundleResult("Go build configuration is invalid")
	}
	for _, tag := range append(append([]string(nil), value.ToolTags...), value.ReleaseTags...) {
		if !validContextText(tag) {
			return invalidBundleResult("Go build tag is invalid")
		}
	}
	for _, module := range value.Modules {
		if !validContextText(module.Path) || !validOptionalBundlePath(module.Directory) ||
			!validContextText(module.GoVersion) || !validOptionalContextText(module.Toolchain) {
			return invalidBundleResult("Go module configuration is invalid")
		}
	}
	for _, workspace := range value.Workspaces {
		if !validOptionalBundlePath(workspace.Directory) || !validOptionalContextText(workspace.GoVersion) ||
			!validOptionalContextText(workspace.Toolchain) {
			return invalidBundleResult("Go workspace configuration is invalid")
		}
	}
	for _, replacement := range value.Replacements {
		if !validContextText(replacement.ModulePath) || !validOptionalBundlePath(replacement.Directory) ||
			(replacement.RepositoryLocal && replacement.Directory == "") {
			return invalidBundleResult("Go replacement configuration is invalid")
		}
	}
	return nil
}

func validContextOmissions(status ContextStatus, omissions []ContextOmission, truncation ContextTruncation) bool {
	seen := make(map[ContextOmissionReason]struct{}, len(omissions))
	hasOutputTruncation := false
	for _, omission := range omissions {
		if omission.Count <= 0 || !validContextOmissionReason(omission.Reason) {
			return false
		}
		if _, duplicate := seen[omission.Reason]; duplicate {
			return false
		}
		seen[omission.Reason] = struct{}{}
		if omission.Reason == ContextOmissionOutputTruncated {
			hasOutputTruncation = true
		}
	}
	if truncation.Truncated != hasOutputTruncation {
		return false
	}
	switch status {
	case ContextComplete:
		return len(omissions) == 0
	case ContextPartial, ContextUnavailable:
		return len(omissions) > 0
	default:
		return false
	}
}

func validContextOmissionReason(reason ContextOmissionReason) bool {
	switch reason {
	case ContextOmissionAnalysisLimitExceeded, ContextOmissionUnsupportedModuleLayout,
		ContextOmissionUnsupportedGoVersion, ContextOmissionOutsideRepositoryDependency,
		ContextOmissionCGOUnsupported, ContextOmissionExternalTypeUnavailable,
		ContextOmissionParseError, ContextOmissionTypeIncomplete, ContextOmissionOutputTruncated:
		return true
	default:
		return false
	}
}

func validContextTruncation(value ContextTruncation) bool {
	if value.OmittedFiles < 0 || value.OmittedItems < 0 || value.OmittedRelations < 0 || value.OmittedBytes < 0 {
		return false
	}
	hasCounts := value.OmittedFiles > 0 || value.OmittedItems > 0 || value.OmittedRelations > 0 || value.OmittedBytes > 0
	return value.Truncated == hasCounts
}

func validOptionalContextText(value string) bool {
	return value == "" || validContextText(value)
}

func validOptionalBundlePath(value string) bool {
	return value == "" || validBundlePath(value)
}

func copyBundleGoContext(value GoContext) BundleGoContext {
	items := make([]BundleContextItem, len(value.Items))
	for index, item := range value.Items {
		hash := sha256.Sum256([]byte(item.Content))
		items[index] = BundleContextItem{
			Reference:       item.ID,
			Kind:            item.Kind,
			Path:            item.Path,
			PackagePath:     item.PackagePath,
			DeclarationKind: item.DeclarationKind,
			Identity:        item.Identity,
			StartLine:       item.StartLine,
			EndLine:         item.EndLine,
			Content:         item.Content,
			ContentBytes:    item.ContentBytes,
			ContentSHA256:   hex.EncodeToString(hash[:]),
			Truncated:       item.Truncated,
		}
	}
	return BundleGoContext{
		Status:               value.Status,
		Build:                copyBundleGoBuild(value.Build),
		AppliedLimits:        copyBundleContextLimits(value.AppliedLimits),
		AnalyzedPackageCount: value.AnalyzedPackageCount,
		AnalyzedFileCount:    value.AnalyzedFileCount,
		AnalyzedSourceBytes:  value.AnalyzedSourceBytes,
		DirectImportEdges:    value.DirectImportEdges,
		ItemCount:            len(items),
		RelationCount:        len(value.Relations),
		ApproximateBytes:     value.ApproximateBytes,
		Items:                items,
		Relations:            copyExplicitSlice(value.Relations),
		Omissions:            copyExplicitSlice(value.Omissions),
		Truncation:           value.Truncation,
	}
}

func copyBundleGoBuild(value GoBuildConfiguration) BundleGoBuildConfiguration {
	return BundleGoBuildConfiguration{
		GOOS:             value.GOOS,
		GOARCH:           value.GOARCH,
		CGOEnabled:       value.CGOEnabled,
		BuildTags:        copyExplicitSlice(value.BuildTags),
		ToolTags:         copyExplicitSlice(value.ToolTags),
		ReleaseTags:      copyExplicitSlice(value.ReleaseTags),
		ToolchainVersion: value.ToolchainVersion,
		TestVariant:      value.TestVariant,
		Modules:          copyExplicitSlice(value.Modules),
		Workspaces:       copyExplicitSlice(value.Workspaces),
		Replacements:     copyExplicitSlice(value.Replacements),
	}
}

func copyExplicitSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	result := make([]T, len(value))
	copy(result, value)
	return result
}

func copyBundleContextLimits(value ContextLimits) BundleContextLimits {
	return BundleContextLimits{
		MaxChangedFiles:        value.MaxChangedFiles,
		MaxModuleRoots:         value.MaxModuleRoots,
		MaxPackages:            value.MaxPackages,
		MaxFilesPerPackage:     value.MaxFilesPerPackage,
		MaxFiles:               value.MaxFiles,
		MaxDirectoryEntries:    value.MaxDirectoryEntries,
		MaxSourceBytesPerFile:  value.MaxSourceBytesPerFile,
		MaxSourceBytes:         value.MaxSourceBytes,
		MaxDirectImportEdges:   value.MaxDirectImportEdges,
		AnalysisTimeoutMillis:  value.AnalysisTimeout.Milliseconds(),
		MaxOutputFiles:         value.MaxOutputFiles,
		MaxOutputItems:         value.MaxOutputItems,
		MaxRelations:           value.MaxRelations,
		MaxExcerptBytes:        value.MaxExcerptBytes,
		MaxOutputBytes:         value.MaxOutputBytes,
		MaxEvaluatorInputBytes: value.MaxEvaluatorInputBytes,
	}
}

func contextApproximateBytes(value GoContext) int {
	total := repositoryDerivedBuildBytes(value.Build)
	for _, item := range value.Items {
		total += contextItemDerivedBytes(item)
	}
	for _, relation := range value.Relations {
		total += len(relation.From) + len(relation.To)
	}
	return total
}

func bundleV2ManifestBytes(bundle BundleV2) ([]byte, error) {
	files := make([]bundleManifestFile, len(bundle.Files))
	for index, file := range bundle.Files {
		files[index] = bundleManifestFile{
			Path:               file.Path,
			Status:             file.Status,
			ChangedLines:       append([]LineRange(nil), file.ChangedLines...),
			EvidenceReferences: append([]string(nil), file.EvidenceReferences...),
			Omissions:          append([]Omission(nil), file.Omissions...),
		}
	}
	items := make([]bundleManifestItem, len(bundle.Items))
	for index, item := range bundle.Items {
		items[index] = bundleManifestItem{
			Reference: item.Reference, Kind: item.Kind, Path: item.Path,
			DeclarationKind: item.DeclarationKind, Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine,
			ChangedLines: append([]LineRange(nil), item.ChangedLines...),
			ContentBytes: item.ContentBytes, ContentSHA256: item.ContentSHA256, Truncated: item.Truncated,
		}
	}
	contextItems := make([]bundleV2ContextManifestItem, len(bundle.GoContext.Items))
	for index, item := range bundle.GoContext.Items {
		contextItems[index] = bundleV2ContextManifestItem{
			Reference: item.Reference, Kind: item.Kind, Path: item.Path, PackagePath: item.PackagePath,
			DeclarationKind: item.DeclarationKind, Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine, ContentBytes: item.ContentBytes,
			ContentSHA256: item.ContentSHA256, Truncated: item.Truncated,
		}
	}
	manifest := bundleV2Manifest{
		FormatVersion: bundle.FormatVersion, BaseRevision: bundle.BaseRevision, HeadRevision: bundle.HeadRevision,
		AppliedLimits: bundle.AppliedLimits, FileCount: bundle.FileCount, DeclarationCount: bundle.DeclarationCount,
		EvidenceCount: bundle.EvidenceCount, ApproximateBytes: bundle.ApproximateBytes,
		Files: files, Items: items, Truncation: bundle.Truncation,
		GoContext: bundleV2ContextManifest{
			Status: bundle.GoContext.Status, Build: bundle.GoContext.Build, AppliedLimits: bundle.GoContext.AppliedLimits,
			AnalyzedPackageCount: bundle.GoContext.AnalyzedPackageCount, AnalyzedFileCount: bundle.GoContext.AnalyzedFileCount,
			AnalyzedSourceBytes: bundle.GoContext.AnalyzedSourceBytes, DirectImportEdges: bundle.GoContext.DirectImportEdges,
			ItemCount: bundle.GoContext.ItemCount, RelationCount: bundle.GoContext.RelationCount,
			ApproximateBytes: bundle.GoContext.ApproximateBytes, Items: contextItems,
			Relations: append([]ContextRelation(nil), bundle.GoContext.Relations...),
			Omissions: append([]ContextOmission(nil), bundle.GoContext.Omissions...), Truncation: bundle.GoContext.Truncation,
		},
	}
	return json.Marshal(manifest)
}
