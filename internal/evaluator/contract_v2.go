package evaluator

import (
	"encoding/json"
	"errors"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

const InputSchemaVersionV2 = 2

type EvidenceGoContext struct {
	Status               string                       `json:"status"`
	Build                EvidenceGoBuildConfiguration `json:"build"`
	AppliedLimits        EvidenceContextLimits        `json:"applied_limits"`
	AnalyzedPackageCount int                          `json:"analyzed_package_count"`
	AnalyzedFileCount    int                          `json:"analyzed_file_count"`
	AnalyzedSourceBytes  int                          `json:"analyzed_source_bytes"`
	DirectImportEdges    int                          `json:"direct_import_edges"`
	ItemCount            int                          `json:"item_count"`
	RelationCount        int                          `json:"relation_count"`
	ApproximateBytes     int                          `json:"approximate_bytes"`
	Items                []EvidenceContextItem        `json:"items"`
	Relations            []EvidenceContextRelation    `json:"relations"`
	Omissions            []EvidenceContextOmission    `json:"omissions"`
	Truncation           EvidenceContextTruncation    `json:"truncation"`
}

type EvidenceContextLimits struct {
	MaxChangedFiles        int   `json:"max_changed_files"`
	MaxModuleRoots         int   `json:"max_module_roots"`
	MaxPackages            int   `json:"max_packages"`
	MaxFilesPerPackage     int   `json:"max_files_per_package"`
	MaxFiles               int   `json:"max_files"`
	MaxDirectoryEntries    int   `json:"max_directory_entries"`
	MaxSourceBytesPerFile  int   `json:"max_source_bytes_per_file"`
	MaxSourceBytes         int   `json:"max_source_bytes"`
	MaxDirectImportEdges   int   `json:"max_direct_import_edges"`
	AnalysisTimeoutMillis  int64 `json:"analysis_timeout_millis"`
	MaxOutputFiles         int   `json:"max_output_files"`
	MaxOutputItems         int   `json:"max_output_items"`
	MaxRelations           int   `json:"max_relations"`
	MaxExcerptBytes        int   `json:"max_excerpt_bytes"`
	MaxOutputBytes         int   `json:"max_output_bytes"`
	MaxEvaluatorInputBytes int   `json:"max_evaluator_input_bytes"`
}

type EvidenceGoBuildConfiguration struct {
	GOOS             string                       `json:"goos"`
	GOARCH           string                       `json:"goarch"`
	CGOEnabled       bool                         `json:"cgo_enabled"`
	BuildTags        []string                     `json:"build_tags"`
	ToolTags         []string                     `json:"tool_tags"`
	ReleaseTags      []string                     `json:"release_tags"`
	ToolchainVersion string                       `json:"toolchain_version"`
	TestVariant      bool                         `json:"test_variant"`
	Modules          []EvidenceContextModule      `json:"modules"`
	Workspaces       []EvidenceContextWorkspace   `json:"workspaces"`
	Replacements     []EvidenceContextReplacement `json:"replacements"`
}

type EvidenceContextModule struct {
	Path      string `json:"path"`
	Directory string `json:"directory"`
	GoVersion string `json:"go_version"`
	Toolchain string `json:"toolchain"`
}

type EvidenceContextWorkspace struct {
	Directory string `json:"directory"`
	GoVersion string `json:"go_version"`
	Toolchain string `json:"toolchain"`
}

type EvidenceContextReplacement struct {
	ModulePath      string `json:"module_path"`
	Directory       string `json:"directory"`
	RepositoryLocal bool   `json:"repository_local"`
}

type EvidenceContextItem struct {
	Reference       string `json:"reference"`
	Kind            string `json:"kind"`
	Path            string `json:"path"`
	PackagePath     string `json:"package_path"`
	DeclarationKind string `json:"declaration_kind"`
	Identity        string `json:"identity"`
	StartLine       int    `json:"start_line"`
	EndLine         int    `json:"end_line"`
	Content         string `json:"content"`
	ContentBytes    int    `json:"content_bytes"`
	ContentSHA256   string `json:"content_sha256"`
	Truncated       bool   `json:"truncated"`
}

type EvidenceContextRelation struct {
	From     string `json:"from"`
	To       string `json:"to"`
	Kind     string `json:"kind"`
	Strength string `json:"strength"`
}

type EvidenceContextOmission struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type EvidenceContextTruncation struct {
	Truncated        bool `json:"truncated"`
	OmittedFiles     int  `json:"omitted_files"`
	OmittedItems     int  `json:"omitted_items"`
	OmittedRelations int  `json:"omitted_relations"`
	OmittedBytes     int  `json:"omitted_bytes"`
}

// NewInputV2 validates evidence-bundle@2, creates an owned runtime copy, and
// enforces the complete serialized evaluator-input@2 budget.
func NewInputV2(bundle evidence.BundleV2) (Input, error) {
	if err := evidence.ValidateBundleV2(bundle); err != nil {
		return Input{}, invalidInput(err)
	}
	input := Input{
		SchemaVersion:  InputSchemaVersionV2,
		EvidenceBundle: runtimeBundleV2(bundle),
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return Input{}, invalidInput(errors.New("evaluator input cannot be encoded"))
	}
	if len(encoded) > bundle.GoContext.AppliedLimits.MaxEvaluatorInputBytes {
		return Input{}, invalidInput(errors.New("evaluator input exceeds its applied byte limit"))
	}
	return input, nil
}

func runtimeBundleV2(bundle evidence.BundleV2) EvidenceBundle {
	runtimeBundle := EvidenceBundle{
		FormatVersion: bundle.FormatVersion, ID: bundle.ID, ManifestSHA256: bundle.ManifestSHA256,
		BaseRevision: bundle.BaseRevision, HeadRevision: bundle.HeadRevision,
		AppliedLimits: copyLimits(bundle.AppliedLimits), FileCount: bundle.FileCount,
		DeclarationCount: bundle.DeclarationCount, EvidenceCount: bundle.EvidenceCount,
		ApproximateBytes: bundle.ApproximateBytes, Files: make([]EvidenceFile, len(bundle.Files)),
		Items: make([]EvidenceItem, len(bundle.Items)),
		Truncation: EvidenceTruncation{
			Truncated: bundle.Truncation.Truncated, OmittedFiles: bundle.Truncation.OmittedFiles,
			OmittedDeclarations: bundle.Truncation.OmittedDeclarations,
			OmittedExcerptBytes: bundle.Truncation.OmittedExcerptBytes,
		},
	}
	for index, file := range bundle.Files {
		runtimeBundle.Files[index] = EvidenceFile{
			Path: file.Path, Status: string(file.Status), ChangedLines: copyRanges(file.ChangedLines),
			EvidenceReferences: copyExplicitRuntimeSlice(file.EvidenceReferences), Omissions: copyOmissions(file.Omissions),
		}
	}
	for index, item := range bundle.Items {
		runtimeBundle.Items[index] = EvidenceItem{
			Reference: item.Reference, Kind: string(item.Kind), Path: item.Path,
			DeclarationKind: string(item.DeclarationKind), Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine, ChangedLines: copyRanges(item.ChangedLines),
			Content: item.Content, ContentBytes: item.ContentBytes, ContentSHA256: item.ContentSHA256,
			Truncated: item.Truncated,
		}
	}
	runtimeBundle.GoContext = runtimeGoContext(bundle.GoContext)
	return runtimeBundle
}

func runtimeGoContext(value evidence.BundleGoContext) *EvidenceGoContext {
	items := make([]EvidenceContextItem, len(value.Items))
	for index, item := range value.Items {
		items[index] = EvidenceContextItem{
			Reference: item.Reference, Kind: string(item.Kind), Path: item.Path, PackagePath: item.PackagePath,
			DeclarationKind: string(item.DeclarationKind), Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine, Content: item.Content,
			ContentBytes: item.ContentBytes, ContentSHA256: item.ContentSHA256, Truncated: item.Truncated,
		}
	}
	relations := make([]EvidenceContextRelation, len(value.Relations))
	for index, relation := range value.Relations {
		relations[index] = EvidenceContextRelation{From: relation.From, To: relation.To, Kind: string(relation.Kind), Strength: string(relation.Strength)}
	}
	omissions := make([]EvidenceContextOmission, len(value.Omissions))
	for index, omission := range value.Omissions {
		omissions[index] = EvidenceContextOmission{Reason: string(omission.Reason), Count: omission.Count}
	}
	return &EvidenceGoContext{
		Status: string(value.Status), Build: runtimeGoBuild(value.Build), AppliedLimits: runtimeContextLimits(value.AppliedLimits),
		AnalyzedPackageCount: value.AnalyzedPackageCount, AnalyzedFileCount: value.AnalyzedFileCount,
		AnalyzedSourceBytes: value.AnalyzedSourceBytes, DirectImportEdges: value.DirectImportEdges,
		ItemCount: value.ItemCount, RelationCount: value.RelationCount, ApproximateBytes: value.ApproximateBytes,
		Items: items, Relations: relations, Omissions: omissions,
		Truncation: EvidenceContextTruncation{
			Truncated: value.Truncation.Truncated, OmittedFiles: value.Truncation.OmittedFiles,
			OmittedItems: value.Truncation.OmittedItems, OmittedRelations: value.Truncation.OmittedRelations,
			OmittedBytes: value.Truncation.OmittedBytes,
		},
	}
}

func runtimeGoBuild(value evidence.BundleGoBuildConfiguration) EvidenceGoBuildConfiguration {
	modules := make([]EvidenceContextModule, len(value.Modules))
	for index, module := range value.Modules {
		modules[index] = EvidenceContextModule{Path: module.Path, Directory: module.Directory, GoVersion: module.GoVersion, Toolchain: module.Toolchain}
	}
	workspaces := make([]EvidenceContextWorkspace, len(value.Workspaces))
	for index, workspace := range value.Workspaces {
		workspaces[index] = EvidenceContextWorkspace{Directory: workspace.Directory, GoVersion: workspace.GoVersion, Toolchain: workspace.Toolchain}
	}
	replacements := make([]EvidenceContextReplacement, len(value.Replacements))
	for index, replacement := range value.Replacements {
		replacements[index] = EvidenceContextReplacement{ModulePath: replacement.ModulePath, Directory: replacement.Directory, RepositoryLocal: replacement.RepositoryLocal}
	}
	return EvidenceGoBuildConfiguration{
		GOOS: value.GOOS, GOARCH: value.GOARCH, CGOEnabled: value.CGOEnabled,
		BuildTags: copyExplicitRuntimeSlice(value.BuildTags), ToolTags: copyExplicitRuntimeSlice(value.ToolTags),
		ReleaseTags: copyExplicitRuntimeSlice(value.ReleaseTags), ToolchainVersion: value.ToolchainVersion,
		TestVariant: value.TestVariant, Modules: modules, Workspaces: workspaces, Replacements: replacements,
	}
}

func runtimeContextLimits(value evidence.BundleContextLimits) EvidenceContextLimits {
	return EvidenceContextLimits{
		MaxChangedFiles: value.MaxChangedFiles, MaxModuleRoots: value.MaxModuleRoots,
		MaxPackages: value.MaxPackages, MaxFilesPerPackage: value.MaxFilesPerPackage,
		MaxFiles: value.MaxFiles, MaxDirectoryEntries: value.MaxDirectoryEntries,
		MaxSourceBytesPerFile: value.MaxSourceBytesPerFile, MaxSourceBytes: value.MaxSourceBytes,
		MaxDirectImportEdges: value.MaxDirectImportEdges, AnalysisTimeoutMillis: value.AnalysisTimeoutMillis,
		MaxOutputFiles: value.MaxOutputFiles, MaxOutputItems: value.MaxOutputItems,
		MaxRelations: value.MaxRelations, MaxExcerptBytes: value.MaxExcerptBytes,
		MaxOutputBytes: value.MaxOutputBytes, MaxEvaluatorInputBytes: value.MaxEvaluatorInputBytes,
	}
}

func runtimeBundleV2ToDomain(bundle EvidenceBundle) evidence.BundleV2 {
	domain := evidence.BundleV2{
		FormatVersion: bundle.FormatVersion, ID: bundle.ID, ManifestSHA256: bundle.ManifestSHA256,
		BaseRevision: bundle.BaseRevision, HeadRevision: bundle.HeadRevision,
		AppliedLimits: evidence.Limits{
			MaxFiles: bundle.AppliedLimits.MaxFiles, MaxDeclarations: bundle.AppliedLimits.MaxDeclarations,
			MaxExcerptBytes: bundle.AppliedLimits.MaxExcerptBytes,
		},
		FileCount: bundle.FileCount, DeclarationCount: bundle.DeclarationCount,
		EvidenceCount: bundle.EvidenceCount, ApproximateBytes: bundle.ApproximateBytes,
		Files: make([]evidence.BundleFile, len(bundle.Files)), Items: make([]evidence.BundleItem, len(bundle.Items)),
		Truncation: evidence.Truncation{
			Truncated: bundle.Truncation.Truncated, OmittedFiles: bundle.Truncation.OmittedFiles,
			OmittedDeclarations: bundle.Truncation.OmittedDeclarations,
			OmittedExcerptBytes: bundle.Truncation.OmittedExcerptBytes,
		},
	}
	for index, file := range bundle.Files {
		domain.Files[index] = evidence.BundleFile{
			Path: file.Path, Status: evidence.FileStatus(file.Status), ChangedLines: domainRanges(file.ChangedLines),
			EvidenceReferences: copyExplicitRuntimeSlice(file.EvidenceReferences), Omissions: domainOmissions(file.Omissions),
		}
	}
	for index, item := range bundle.Items {
		domain.Items[index] = evidence.BundleItem{
			Reference: item.Reference, Kind: evidence.BundleItemKind(item.Kind), Path: item.Path,
			DeclarationKind: evidence.DeclarationKind(item.DeclarationKind), Identity: item.Identity,
			StartLine: item.StartLine, EndLine: item.EndLine, ChangedLines: domainRanges(item.ChangedLines),
			Content: item.Content, ContentBytes: item.ContentBytes, ContentSHA256: item.ContentSHA256,
			Truncated: item.Truncated,
		}
	}
	if bundle.GoContext != nil {
		domain.GoContext = domainGoContext(*bundle.GoContext)
	}
	return domain
}

func domainGoContext(value EvidenceGoContext) evidence.BundleGoContext {
	items := make([]evidence.BundleContextItem, len(value.Items))
	for index, item := range value.Items {
		items[index] = evidence.BundleContextItem{
			Reference: item.Reference, Kind: evidence.ContextItemKind(item.Kind), Path: item.Path,
			PackagePath: item.PackagePath, DeclarationKind: evidence.DeclarationKind(item.DeclarationKind),
			Identity: item.Identity, StartLine: item.StartLine, EndLine: item.EndLine,
			Content: item.Content, ContentBytes: item.ContentBytes, ContentSHA256: item.ContentSHA256,
			Truncated: item.Truncated,
		}
	}
	relations := make([]evidence.ContextRelation, len(value.Relations))
	for index, relation := range value.Relations {
		relations[index] = evidence.ContextRelation{From: relation.From, To: relation.To, Kind: evidence.ContextRelationKind(relation.Kind), Strength: evidence.ContextRelationStrength(relation.Strength)}
	}
	omissions := make([]evidence.ContextOmission, len(value.Omissions))
	for index, omission := range value.Omissions {
		omissions[index] = evidence.ContextOmission{Reason: evidence.ContextOmissionReason(omission.Reason), Count: omission.Count}
	}
	return evidence.BundleGoContext{
		Status: evidence.ContextStatus(value.Status), Build: domainGoBuild(value.Build), AppliedLimits: domainContextLimits(value.AppliedLimits),
		AnalyzedPackageCount: value.AnalyzedPackageCount, AnalyzedFileCount: value.AnalyzedFileCount,
		AnalyzedSourceBytes: value.AnalyzedSourceBytes, DirectImportEdges: value.DirectImportEdges,
		ItemCount: value.ItemCount, RelationCount: value.RelationCount, ApproximateBytes: value.ApproximateBytes,
		Items: items, Relations: relations, Omissions: omissions,
		Truncation: evidence.ContextTruncation{
			Truncated: value.Truncation.Truncated, OmittedFiles: value.Truncation.OmittedFiles,
			OmittedItems: value.Truncation.OmittedItems, OmittedRelations: value.Truncation.OmittedRelations,
			OmittedBytes: value.Truncation.OmittedBytes,
		},
	}
}

func domainGoBuild(value EvidenceGoBuildConfiguration) evidence.BundleGoBuildConfiguration {
	modules := make([]evidence.ContextModule, len(value.Modules))
	for index, module := range value.Modules {
		modules[index] = evidence.ContextModule{Path: module.Path, Directory: module.Directory, GoVersion: module.GoVersion, Toolchain: module.Toolchain}
	}
	workspaces := make([]evidence.ContextWorkspace, len(value.Workspaces))
	for index, workspace := range value.Workspaces {
		workspaces[index] = evidence.ContextWorkspace{Directory: workspace.Directory, GoVersion: workspace.GoVersion, Toolchain: workspace.Toolchain}
	}
	replacements := make([]evidence.ContextReplacement, len(value.Replacements))
	for index, replacement := range value.Replacements {
		replacements[index] = evidence.ContextReplacement{ModulePath: replacement.ModulePath, Directory: replacement.Directory, RepositoryLocal: replacement.RepositoryLocal}
	}
	return evidence.BundleGoBuildConfiguration{
		GOOS: value.GOOS, GOARCH: value.GOARCH, CGOEnabled: value.CGOEnabled,
		BuildTags: copyExplicitRuntimeSlice(value.BuildTags), ToolTags: copyExplicitRuntimeSlice(value.ToolTags),
		ReleaseTags: copyExplicitRuntimeSlice(value.ReleaseTags), ToolchainVersion: value.ToolchainVersion,
		TestVariant: value.TestVariant, Modules: modules, Workspaces: workspaces, Replacements: replacements,
	}
}

func copyExplicitRuntimeSlice[T any](value []T) []T {
	if value == nil {
		return nil
	}
	result := make([]T, len(value))
	copy(result, value)
	return result
}

func domainContextLimits(value EvidenceContextLimits) evidence.BundleContextLimits {
	return evidence.BundleContextLimits{
		MaxChangedFiles: value.MaxChangedFiles, MaxModuleRoots: value.MaxModuleRoots,
		MaxPackages: value.MaxPackages, MaxFilesPerPackage: value.MaxFilesPerPackage,
		MaxFiles: value.MaxFiles, MaxDirectoryEntries: value.MaxDirectoryEntries,
		MaxSourceBytesPerFile: value.MaxSourceBytesPerFile, MaxSourceBytes: value.MaxSourceBytes,
		MaxDirectImportEdges: value.MaxDirectImportEdges, AnalysisTimeoutMillis: value.AnalysisTimeoutMillis,
		MaxOutputFiles: value.MaxOutputFiles, MaxOutputItems: value.MaxOutputItems,
		MaxRelations: value.MaxRelations, MaxExcerptBytes: value.MaxExcerptBytes,
		MaxOutputBytes: value.MaxOutputBytes, MaxEvaluatorInputBytes: value.MaxEvaluatorInputBytes,
	}
}

func validatedInputReferences(input Input) ([]string, error) {
	if input.SchemaVersion == InputSchemaVersion {
		if input.EvidenceBundle.GoContext != nil {
			return nil, errors.New("evaluator-input@1 cannot contain Go context")
		}
		if err := validateBundle(runtimeBundleToDomain(input.EvidenceBundle)); err != nil {
			return nil, err
		}
	} else if input.SchemaVersion == InputSchemaVersionV2 {
		if input.EvidenceBundle.GoContext == nil {
			return nil, errors.New("evaluator-input@2 requires Go context")
		}
		if err := evidence.ValidateBundleV2(runtimeBundleV2ToDomain(input.EvidenceBundle)); err != nil {
			return nil, err
		}
	} else {
		return nil, errors.New("unsupported evaluator input schema version")
	}

	return runtimeInputReferences(input)
}

func runtimeInputReferences(input Input) ([]string, error) {
	if input.SchemaVersion == InputSchemaVersion {
		if input.EvidenceBundle.GoContext != nil || len(input.EvidenceBundle.Items) == 0 {
			return nil, errors.New("validated evaluator-input@1 is required")
		}
	} else if input.SchemaVersion == InputSchemaVersionV2 {
		if input.EvidenceBundle.GoContext == nil || len(input.EvidenceBundle.Items)+len(input.EvidenceBundle.GoContext.Items) == 0 {
			return nil, errors.New("validated evaluator-input@2 is required")
		}
	} else {
		return nil, errors.New("unsupported evaluator input schema version")
	}
	references := make([]string, 0, input.EvidenceBundle.EvidenceCount)
	for _, item := range input.EvidenceBundle.Items {
		references = append(references, item.Reference)
	}
	if input.EvidenceBundle.GoContext != nil {
		for _, item := range input.EvidenceBundle.GoContext.Items {
			references = append(references, item.Reference)
		}
	}
	if _, err := referenceSet(references); err != nil {
		return nil, err
	}
	return references, nil
}
