package evidence

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/build"
	"go/parser"
	"go/token"
	"go/types"
	"io"
	"path"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"golang.org/x/mod/modfile"
	modmodule "golang.org/x/mod/module"
)

const (
	maxContextChangedFiles       = 20
	maxContextModuleRoots        = 8
	maxContextPackages           = 32
	maxContextFilesPerPackage    = 64
	maxContextFiles              = 160
	maxContextDirectoryEntries   = 256
	maxContextSourceBytesPerFile = 256 * 1024
	maxContextSourceBytes        = 2 * 1024 * 1024
	maxContextImportEdges        = 256
	maxContextOutputFiles        = 20
	maxContextOutputItems        = 40
	maxContextRelations          = 100
	maxContextExcerptBytes       = 4 * 1024
	maxContextOutputBytes        = 64 * 1024
	maxEvaluatorInputBytes       = 256 * 1024
	contextAnalysisTimeout       = 30 * time.Second
)

type ContextStatus string

const (
	ContextComplete    ContextStatus = "complete"
	ContextPartial     ContextStatus = "partial"
	ContextUnavailable ContextStatus = "unavailable"
)

type ContextLimits struct {
	MaxChangedFiles        int
	MaxModuleRoots         int
	MaxPackages            int
	MaxFilesPerPackage     int
	MaxFiles               int
	MaxDirectoryEntries    int
	MaxSourceBytesPerFile  int
	MaxSourceBytes         int
	MaxDirectImportEdges   int
	AnalysisTimeout        time.Duration
	MaxOutputFiles         int
	MaxOutputItems         int
	MaxRelations           int
	MaxExcerptBytes        int
	MaxOutputBytes         int
	MaxEvaluatorInputBytes int
}

type GoContext struct {
	Status               ContextStatus
	Build                GoBuildConfiguration
	AppliedLimits        ContextLimits
	AnalyzedPackageCount int
	AnalyzedFileCount    int
	AnalyzedSourceBytes  int
	DirectImportEdges    int
	Items                []ContextItem
	Relations            []ContextRelation
	Omissions            []ContextOmission
	Truncation           ContextTruncation
}

type GoBuildConfiguration struct {
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

type ContextModule struct {
	Path      string
	Directory string
	GoVersion string
	Toolchain string
}

type ContextWorkspace struct {
	Directory string
	GoVersion string
	Toolchain string
}

type ContextReplacement struct {
	ModulePath      string
	Directory       string
	RepositoryLocal bool
}

type ContextItemKind string

const (
	ContextItemChangedImport      ContextItemKind = "changed_import"
	ContextItemContextDeclaration ContextItemKind = "context_declaration"
)

type ContextItem struct {
	ID              string
	Kind            ContextItemKind
	Path            string
	PackagePath     string
	DeclarationKind DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	Content         string
	ContentBytes    int
	Truncated       bool
}

type ContextRelationKind string

const (
	ContextRelationImports    ContextRelationKind = "imports"
	ContextRelationReferences ContextRelationKind = "references"
	ContextRelationImplements ContextRelationKind = "implements"
)

type ContextRelationStrength string

const (
	ContextRelationSyntactic   ContextRelationStrength = "syntactic"
	ContextRelationTypeChecked ContextRelationStrength = "type_checked"
)

type ContextRelation struct {
	From     string
	To       string
	Kind     ContextRelationKind
	Strength ContextRelationStrength
}

type ContextOmissionReason string

const (
	ContextOmissionAnalysisLimitExceeded       ContextOmissionReason = "analysis_limit_exceeded"
	ContextOmissionUnsupportedModuleLayout     ContextOmissionReason = "unsupported_module_layout"
	ContextOmissionUnsupportedGoVersion        ContextOmissionReason = "unsupported_go_version"
	ContextOmissionOutsideRepositoryDependency ContextOmissionReason = "outside_repository_dependency"
	ContextOmissionCGOUnsupported              ContextOmissionReason = "cgo_unsupported"
	ContextOmissionExternalTypeUnavailable     ContextOmissionReason = "external_type_unavailable"
	ContextOmissionParseError                  ContextOmissionReason = "context_parse_error"
	ContextOmissionTypeIncomplete              ContextOmissionReason = "type_incomplete"
	ContextOmissionOutputTruncated             ContextOmissionReason = "output_truncated"
)

type ContextOmission struct {
	Reason ContextOmissionReason
	Count  int
}

type ContextTruncation struct {
	Truncated        bool
	OmittedFiles     int
	OmittedItems     int
	OmittedRelations int
	OmittedBytes     int
}

func fixedContextLimits() ContextLimits {
	return ContextLimits{
		MaxChangedFiles:        maxContextChangedFiles,
		MaxModuleRoots:         maxContextModuleRoots,
		MaxPackages:            maxContextPackages,
		MaxFilesPerPackage:     maxContextFilesPerPackage,
		MaxFiles:               maxContextFiles,
		MaxDirectoryEntries:    maxContextDirectoryEntries,
		MaxSourceBytesPerFile:  maxContextSourceBytesPerFile,
		MaxSourceBytes:         maxContextSourceBytes,
		MaxDirectImportEdges:   maxContextImportEdges,
		AnalysisTimeout:        contextAnalysisTimeout,
		MaxOutputFiles:         maxContextOutputFiles,
		MaxOutputItems:         maxContextOutputItems,
		MaxRelations:           maxContextRelations,
		MaxExcerptBytes:        maxContextExcerptBytes,
		MaxOutputBytes:         maxContextOutputBytes,
		MaxEvaluatorInputBytes: maxEvaluatorInputBytes,
	}
}

type contextFailure struct {
	Reason ContextOmissionReason
}

func (failure *contextFailure) Error() string {
	return string(failure.Reason)
}

type contextAnalyzer struct {
	snapshot snapshot
	result   Result
	limits   ContextLimits
	output   *GoContext

	sources             map[string][]byte
	directories         map[string]*parsedContextDirectory
	moduleByDirectory   map[string]*contextModuleRoot
	moduleMappings      map[string]contextModuleRoot
	configurationRoots  map[string]struct{}
	workspaceModules    map[string]map[string]struct{}
	replacements        map[string]string
	blockedReplacements map[string]struct{}
	packages            map[string]*contextPackage
	changedPackages     map[string]*contextPackage
	fakePackages        map[string]*types.Package
	omissionCounts      map[ContextOmissionReason]int
	omissionKeys        map[ContextOmissionReason]map[string]struct{}
	itemCandidates      map[string]contextItemCandidate
	relationCandidates  map[string]contextRelationCandidate
	objectDeclarations  map[types.Object]*contextDeclaration
	analyzedFiles       int
	analyzedSourceBytes int
	directImportEdges   int
	includeTests        bool
}

type contextModuleRoot struct {
	Path      string
	Directory string
	GoVersion string
	Toolchain string
}

type parsedContextDirectory struct {
	fileSet    *token.FileSet
	groups     map[string][]*contextSourceFile
	incomplete bool
}

type contextSourceFile struct {
	path      string
	source    []byte
	parsed    *ast.File
	tokenFile *token.File
}

type contextPackageState uint8

const (
	contextPackageUnchecked contextPackageState = iota
	contextPackageChecking
	contextPackageChecked
)

type contextPackage struct {
	key              string
	importPath       string
	name             string
	directory        string
	module           *contextModuleRoot
	fileSet          *token.FileSet
	files            []*contextSourceFile
	typesFiles       []*ast.File
	typesInfo        *types.Info
	typesPkg         *types.Package
	state            contextPackageState
	complete         bool
	incompleteInputs bool
	declarations     []*contextDeclaration
}

type contextDeclaration struct {
	object       types.Object
	path         string
	packagePath  string
	kind         DeclarationKind
	identity     string
	matchStart   int
	matchEnd     int
	startLine    int
	endLine      int
	content      string
	inspectNode  ast.Node
	packageValue *contextPackage
}

type contextItemCandidate struct {
	key  string
	item ContextItem
}

type contextRelationCandidate struct {
	from      string
	targetKey string
	target    string
	kind      ContextRelationKind
	strength  ContextRelationStrength
}

func analyzeGoContext(parent context.Context, root, head string, result Result) *GoContext {
	limits := fixedContextLimits()
	output := &GoContext{
		Status:        ContextComplete,
		AppliedLimits: limits,
		Build: GoBuildConfiguration{
			GOOS:             runtime.GOOS,
			GOARCH:           runtime.GOARCH,
			CGOEnabled:       false,
			BuildTags:        []string{},
			ToolTags:         append([]string(nil), build.Default.ToolTags...),
			ReleaseTags:      append([]string(nil), build.Default.ReleaseTags...),
			ToolchainVersion: runtime.Version(),
			Modules:          []ContextModule{},
			Workspaces:       []ContextWorkspace{},
			Replacements:     []ContextReplacement{},
		},
		Items:     []ContextItem{},
		Relations: []ContextRelation{},
		Omissions: []ContextOmission{},
	}
	analysisContext, cancel := context.WithTimeout(parent, limits.AnalysisTimeout)
	defer cancel()
	analyzer := &contextAnalyzer{
		snapshot:            selectedSnapshot(root, head),
		result:              result,
		limits:              limits,
		output:              output,
		sources:             make(map[string][]byte),
		directories:         make(map[string]*parsedContextDirectory),
		moduleByDirectory:   make(map[string]*contextModuleRoot),
		moduleMappings:      make(map[string]contextModuleRoot),
		configurationRoots:  make(map[string]struct{}),
		workspaceModules:    make(map[string]map[string]struct{}),
		replacements:        make(map[string]string),
		blockedReplacements: make(map[string]struct{}),
		packages:            make(map[string]*contextPackage),
		changedPackages:     make(map[string]*contextPackage),
		fakePackages:        make(map[string]*types.Package),
		omissionCounts:      make(map[ContextOmissionReason]int),
		omissionKeys:        make(map[ContextOmissionReason]map[string]struct{}),
		itemCandidates:      make(map[string]contextItemCandidate),
		relationCandidates:  make(map[string]contextRelationCandidate),
		objectDeclarations:  make(map[types.Object]*contextDeclaration),
	}
	var analysisErr error
	if len(result.Files) > limits.MaxChangedFiles || result.Truncation.OmittedFiles > 0 {
		analysisErr = &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
	} else {
		analysisErr = analyzer.run(analysisContext)
	}
	if analysisErr != nil {
		reason := ContextOmissionUnsupportedModuleLayout
		var failure *contextFailure
		if errors.As(analysisErr, &failure) {
			reason = failure.Reason
		} else if errors.Is(analysisErr, context.DeadlineExceeded) || errors.Is(analysisErr, context.Canceled) {
			reason = ContextOmissionAnalysisLimitExceeded
		}
		output.Status = ContextUnavailable
		output.Items = []ContextItem{}
		output.Relations = []ContextRelation{}
		output.Truncation = ContextTruncation{}
		analyzer.omissionCounts = map[ContextOmissionReason]int{reason: 1}
	}
	analyzer.finish()
	return output
}

func (analyzer *contextAnalyzer) run(ctx context.Context) error {
	for _, file := range analyzer.result.Files {
		if file.Status != FileDeleted && strings.HasSuffix(file.Path, "_test.go") {
			analyzer.includeTests = true
			break
		}
	}
	analyzer.output.Build.TestVariant = analyzer.includeTests

	for _, file := range analyzer.result.Files {
		if file.Status == FileDeleted {
			continue
		}
		if err := analyzer.addChangedFile(ctx, file); err != nil {
			return err
		}
	}
	if len(analyzer.changedPackages) == 0 {
		return nil
	}
	if err := analyzer.discoverDirectPackages(ctx); err != nil {
		return err
	}
	keys := sortedContextPackageKeys(analyzer.packages)
	for _, key := range keys {
		analyzer.checkPackage(analyzer.packages[key])
	}
	analyzer.collectDeclarations()
	analyzer.collectRelations()
	return nil
}

func (analyzer *contextAnalyzer) addChangedFile(ctx context.Context, file File) error {
	source, err := analyzer.readSource(ctx, file.Path)
	if err != nil {
		return analyzer.snapshotFailure(err)
	}
	parsed, err := parser.ParseFile(token.NewFileSet(), file.Path, source, parser.ImportsOnly)
	if err != nil || parsed.Name == nil {
		return &contextFailure{Reason: ContextOmissionParseError}
	}
	module, err := analyzer.moduleForFile(ctx, file.Path)
	if err != nil {
		return err
	}
	directory := snapshotPathDirectory(file.Path)
	importPath, err := moduleImportPath(*module, directory)
	if err != nil {
		return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	directoryValue, err := analyzer.loadDirectory(ctx, directory)
	if err != nil {
		return err
	}
	files := directoryValue.groups[parsed.Name.Name]
	if len(files) == 0 {
		return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	key := importPath
	if strings.HasSuffix(parsed.Name.Name, "_test") {
		key += "_test"
	}
	packageValue := analyzer.packages[key]
	if packageValue == nil {
		if len(analyzer.packages) == analyzer.limits.MaxPackages {
			return &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
		}
		packageValue = newContextPackage(key, importPath, parsed.Name.Name, directory, module, directoryValue.fileSet, files, directoryValue.incomplete)
		analyzer.packages[key] = packageValue
	}
	analyzer.changedPackages[key] = packageValue
	analyzer.collectChangedImports(file, source, importPath)
	return nil
}

func newContextPackage(key, importPath, name, directory string, module *contextModuleRoot, fileSet *token.FileSet, files []*contextSourceFile, incomplete bool) *contextPackage {
	typesFiles := make([]*ast.File, len(files))
	for index, file := range files {
		typesFiles[index] = file.parsed
	}
	return &contextPackage{
		key:              key,
		importPath:       importPath,
		name:             name,
		directory:        directory,
		module:           module,
		fileSet:          fileSet,
		files:            files,
		typesFiles:       typesFiles,
		incompleteInputs: incomplete,
	}
}

func sortedContextPackageKeys(packages map[string]*contextPackage) []string {
	keys := make([]string, 0, len(packages))
	for key := range packages {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (analyzer *contextAnalyzer) readSource(ctx context.Context, relative string) ([]byte, error) {
	if source, ok := analyzer.sources[relative]; ok {
		return source, nil
	}
	if analyzer.analyzedFiles == analyzer.limits.MaxFiles {
		return nil, errSnapshotLimit
	}
	source, err := analyzer.snapshot.ReadFile(ctx, relative, int64(analyzer.limits.MaxSourceBytesPerFile))
	if err != nil {
		return nil, err
	}
	if analyzer.analyzedSourceBytes+len(source) > analyzer.limits.MaxSourceBytes {
		return nil, errSnapshotLimit
	}
	analyzer.analyzedFiles++
	analyzer.analyzedSourceBytes += len(source)
	analyzer.sources[relative] = source
	return source, nil
}

func (analyzer *contextAnalyzer) loadDirectory(ctx context.Context, directory string) (*parsedContextDirectory, error) {
	if value, ok := analyzer.directories[directory]; ok {
		return value, nil
	}
	entries, err := analyzer.snapshot.ReadDir(ctx, directory, analyzer.limits.MaxDirectoryEntries)
	if err != nil {
		return nil, analyzer.snapshotFailure(err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name, ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name, "_test.go") && !analyzer.includeTests {
			continue
		}
		names = append(names, entry.Name)
	}
	sort.Strings(names)
	if len(names) > analyzer.limits.MaxFilesPerPackage {
		return nil, &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
	}
	sources := make(map[string][]byte, len(names))
	for _, name := range names {
		relative := joinSnapshotPath(directory, name)
		source, err := analyzer.readSource(ctx, relative)
		if errors.Is(err, errSnapshotNotFound) {
			continue
		}
		if err != nil {
			return nil, analyzer.snapshotFailure(err)
		}
		sources[relative] = source
	}
	buildContext := selectedBuildContext(sources)
	fileSet := token.NewFileSet()
	result := &parsedContextDirectory{fileSet: fileSet, groups: make(map[string][]*contextSourceFile)}
	for _, name := range names {
		relative := joinSnapshotPath(directory, name)
		source, ok := sources[relative]
		if !ok {
			continue
		}
		matched, err := buildContext.MatchFile(directory, name)
		if err != nil {
			analyzer.addOmission(ContextOmissionParseError, 1)
			result.incomplete = true
			continue
		}
		if !matched {
			continue
		}
		parsed, err := parser.ParseFile(fileSet, relative, source, parser.AllErrors|parser.ParseComments)
		if err != nil || parsed == nil || parsed.Name == nil {
			analyzer.addOmission(ContextOmissionParseError, 1)
			result.incomplete = true
			continue
		}
		tokenFile := fileSet.File(parsed.Pos())
		if tokenFile == nil {
			analyzer.addOmission(ContextOmissionParseError, 1)
			result.incomplete = true
			continue
		}
		result.groups[parsed.Name.Name] = append(result.groups[parsed.Name.Name], &contextSourceFile{
			path:      relative,
			source:    source,
			parsed:    parsed,
			tokenFile: tokenFile,
		})
	}
	analyzer.directories[directory] = result
	return result, nil
}

func selectedBuildContext(sources map[string][]byte) build.Context {
	value := build.Default
	value.GOOS = runtime.GOOS
	value.GOARCH = runtime.GOARCH
	value.CgoEnabled = false
	value.BuildTags = nil
	value.ToolTags = append([]string(nil), build.Default.ToolTags...)
	value.ReleaseTags = append([]string(nil), build.Default.ReleaseTags...)
	value.JoinPath = path.Join
	value.IsAbsPath = path.IsAbs
	value.OpenFile = func(name string) (io.ReadCloser, error) {
		source, ok := sources[path.Clean(name)]
		if !ok {
			return nil, errSnapshotNotFound
		}
		return io.NopCloser(bytes.NewReader(source)), nil
	}
	return value
}

func (analyzer *contextAnalyzer) snapshotFailure(err error) error {
	switch {
	case errors.Is(err, errSnapshotLimit):
		return &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
	case errors.Is(err, errSnapshotOutside):
		return &contextFailure{Reason: ContextOmissionOutsideRepositoryDependency}
	case errors.Is(err, context.DeadlineExceeded), errors.Is(err, context.Canceled):
		return err
	default:
		return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
}

func snapshotPathDirectory(relative string) string {
	directory := path.Dir(relative)
	if directory == "." {
		return ""
	}
	return directory
}

func joinSnapshotPath(directory, name string) string {
	if directory == "" {
		return name
	}
	return path.Join(directory, name)
}

func (analyzer *contextAnalyzer) moduleForFile(ctx context.Context, relative string) (*contextModuleRoot, error) {
	directory := snapshotPathDirectory(relative)
	for {
		if module, ok := analyzer.moduleByDirectory[directory]; ok {
			return module, nil
		}
		modulePath := joinSnapshotPath(directory, "go.mod")
		content, err := analyzer.snapshot.ReadFile(ctx, modulePath, int64(analyzer.limits.MaxSourceBytesPerFile))
		switch {
		case err == nil:
			value, err := analyzer.addModule(ctx, directory, content, "")
			if err != nil {
				return nil, err
			}
			for current := snapshotPathDirectory(relative); ; current = snapshotPathDirectory(current) {
				analyzer.moduleByDirectory[current] = value
				if current == directory || current == "" {
					break
				}
			}
			if err := analyzer.loadWorkspace(ctx, value); err != nil {
				return nil, err
			}
			return value, nil
		case errors.Is(err, errSnapshotNotFound):
			if directory == "" {
				return nil, &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
			}
			directory = snapshotPathDirectory(directory)
		default:
			return nil, analyzer.snapshotFailure(err)
		}
	}
}

func (analyzer *contextAnalyzer) addModule(ctx context.Context, directory string, content []byte, importOverride string) (*contextModuleRoot, error) {
	if existing, ok := analyzer.moduleByDirectory[directory]; ok {
		if importOverride != "" {
			mapping := *existing
			mapping.Path = importOverride
			analyzer.moduleMappings[importOverride] = mapping
		}
		return existing, nil
	}
	if err := analyzer.addConfigurationRoot(joinSnapshotPath(directory, "go.mod")); err != nil {
		return nil, err
	}
	parsed, err := modfile.Parse(joinSnapshotPath(directory, "go.mod"), content, nil)
	if err != nil || parsed.Module == nil || !validContextModulePath(parsed.Module.Mod.Path) {
		return nil, &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	if parsed.Go == nil {
		return nil, &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	goVersion := parsed.Go.Version
	if !supportedGoVersion(goVersion, runtime.Version()) {
		return nil, &contextFailure{Reason: ContextOmissionUnsupportedGoVersion}
	}
	toolchain := ""
	if parsed.Toolchain != nil {
		toolchain = parsed.Toolchain.Name
		if !validContextText(toolchain) {
			return nil, &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
		}
	}
	module := &contextModuleRoot{
		Path:      parsed.Module.Mod.Path,
		Directory: directory,
		GoVersion: goVersion,
		Toolchain: toolchain,
	}
	if existing, exists := analyzer.moduleMappings[module.Path]; exists && existing.Directory != module.Directory {
		return nil, &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	analyzer.moduleByDirectory[directory] = module
	analyzer.moduleMappings[module.Path] = *module
	if importOverride != "" {
		mapping := *module
		mapping.Path = importOverride
		analyzer.moduleMappings[importOverride] = mapping
	}
	analyzer.output.Build.Modules = append(analyzer.output.Build.Modules, ContextModule{
		Path:      module.Path,
		Directory: module.Directory,
		GoVersion: module.GoVersion,
		Toolchain: module.Toolchain,
	})
	if err := analyzer.processReplacements(ctx, directory, parsed.Replace, false); err != nil {
		return nil, err
	}
	return module, nil
}

func (analyzer *contextAnalyzer) loadWorkspace(ctx context.Context, module *contextModuleRoot) error {
	directory := module.Directory
	for {
		workspacePath := joinSnapshotPath(directory, "go.work")
		content, err := analyzer.snapshot.ReadFile(ctx, workspacePath, int64(analyzer.limits.MaxSourceBytesPerFile))
		switch {
		case err == nil:
			if modules, processed := analyzer.workspaceModules[workspacePath]; processed {
				if _, included := modules[module.Directory]; !included {
					return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
				}
				return nil
			}
			if err := analyzer.addConfigurationRoot(workspacePath); err != nil {
				return err
			}
			workspace, err := modfile.ParseWork(workspacePath, content, nil)
			if err != nil {
				return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
			}
			workspaceModules := make(map[string]struct{})
			analyzer.workspaceModules[workspacePath] = workspaceModules
			if workspace.Go == nil {
				return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
			}
			goVersion := workspace.Go.Version
			if !supportedGoVersion(goVersion, runtime.Version()) {
				return &contextFailure{Reason: ContextOmissionUnsupportedGoVersion}
			}
			toolchain := ""
			if workspace.Toolchain != nil {
				toolchain = workspace.Toolchain.Name
				if !validContextText(toolchain) {
					return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
				}
			}
			analyzer.output.Build.Workspaces = append(analyzer.output.Build.Workspaces, ContextWorkspace{
				Directory: directory,
				GoVersion: goVersion,
				Toolchain: toolchain,
			})
			for _, use := range workspace.Use {
				usedDirectory, ok := repositoryRelative(directory, use.Path)
				if !ok {
					analyzer.addOmission(ContextOmissionOutsideRepositoryDependency, 1)
					continue
				}
				workspaceModules[usedDirectory] = struct{}{}
				modulePath := joinSnapshotPath(usedDirectory, "go.mod")
				moduleContent, err := analyzer.snapshot.ReadFile(ctx, modulePath, int64(analyzer.limits.MaxSourceBytesPerFile))
				if err != nil {
					if errors.Is(err, errSnapshotOutside) {
						analyzer.addOmission(ContextOmissionOutsideRepositoryDependency, 1)
						continue
					}
					return analyzer.snapshotFailure(err)
				}
				if _, err := analyzer.addModule(ctx, usedDirectory, moduleContent, ""); err != nil {
					return err
				}
			}
			if _, included := workspaceModules[module.Directory]; !included {
				return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
			}
			return analyzer.processReplacements(ctx, directory, workspace.Replace, true)
		case errors.Is(err, errSnapshotNotFound):
			if directory == "" {
				return nil
			}
			directory = snapshotPathDirectory(directory)
		default:
			return analyzer.snapshotFailure(err)
		}
	}
}

func (analyzer *contextAnalyzer) processReplacements(ctx context.Context, baseDirectory string, replacements []*modfile.Replace, workspaceOverride bool) error {
	for _, replacement := range replacements {
		if replacement == nil || !validContextModulePath(replacement.Old.Path) {
			return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
		}
		if replacement.Old.Version != "" {
			return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
		}
		if replacement.New.Version != "" || !isLocalModulePath(replacement.New.Path) {
			if err := analyzer.setBlockedReplacement(replacement.Old.Path, workspaceOverride); err != nil {
				return err
			}
			continue
		}
		directory, ok := repositoryRelative(baseDirectory, replacement.New.Path)
		if !ok {
			if err := analyzer.setBlockedReplacement(replacement.Old.Path, workspaceOverride); err != nil {
				return err
			}
			analyzer.addOmission(ContextOmissionOutsideRepositoryDependency, 1)
			continue
		}
		if existing, exists := analyzer.replacements[replacement.Old.Path]; exists && existing != directory && !workspaceOverride {
			return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
		}
		content, err := analyzer.snapshot.ReadFile(ctx, joinSnapshotPath(directory, "go.mod"), int64(analyzer.limits.MaxSourceBytesPerFile))
		if err != nil {
			return analyzer.snapshotFailure(err)
		}
		module, err := analyzer.addModule(ctx, directory, content, replacement.Old.Path)
		if err != nil {
			return err
		}
		delete(analyzer.blockedReplacements, replacement.Old.Path)
		analyzer.replacements[replacement.Old.Path] = directory
		analyzer.replaceBuildReplacement(ContextReplacement{
			ModulePath:      replacement.Old.Path,
			Directory:       module.Directory,
			RepositoryLocal: true,
		})
	}
	return nil
}

func (analyzer *contextAnalyzer) setBlockedReplacement(modulePath string, workspaceOverride bool) error {
	if existing, exists := analyzer.replacements[modulePath]; exists && existing != "" && !workspaceOverride {
		return &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
	}
	analyzer.replacements[modulePath] = ""
	analyzer.blockedReplacements[modulePath] = struct{}{}
	analyzer.replaceBuildReplacement(ContextReplacement{ModulePath: modulePath})
	return nil
}

func (analyzer *contextAnalyzer) replaceBuildReplacement(replacement ContextReplacement) {
	retained := analyzer.output.Build.Replacements[:0]
	for _, existing := range analyzer.output.Build.Replacements {
		if existing.ModulePath != replacement.ModulePath {
			retained = append(retained, existing)
		}
	}
	analyzer.output.Build.Replacements = retained
	analyzer.output.Build.Replacements = append(analyzer.output.Build.Replacements, replacement)
}

func (analyzer *contextAnalyzer) addConfigurationRoot(relative string) error {
	if _, exists := analyzer.configurationRoots[relative]; exists {
		return nil
	}
	if len(analyzer.configurationRoots) == analyzer.limits.MaxModuleRoots {
		return &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
	}
	analyzer.configurationRoots[relative] = struct{}{}
	return nil
}

func repositoryRelative(baseDirectory, configured string) (string, bool) {
	if configured == "" || path.IsAbs(configured) || hasWindowsVolume(configured) || !utf8.ValidString(configured) || containsControl(configured) {
		return "", false
	}
	joined := path.Clean(path.Join(baseDirectory, configured))
	if joined == ".." || strings.HasPrefix(joined, "../") {
		return "", false
	}
	if joined == "." {
		return "", true
	}
	return joined, true
}

func isLocalModulePath(value string) bool {
	return strings.HasPrefix(value, "./") || strings.HasPrefix(value, "../") || path.IsAbs(value) || hasWindowsVolume(value)
}

func hasWindowsVolume(value string) bool {
	return len(value) >= 2 && ((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) && value[1] == ':'
}

func validContextText(value string) bool {
	return value != "" && utf8.ValidString(value) && !containsControl(value)
}

func validContextModulePath(value string) bool {
	return validContextText(value) && modmodule.CheckPath(value) == nil
}

func validContextImportPath(value string) bool {
	return validContextText(value) && modmodule.CheckImportPath(value) == nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if unicode.IsControl(character) {
			return true
		}
	}
	return false
}

func supportedGoVersion(moduleVersion, toolchainVersion string) bool {
	moduleMajor, moduleMinor, ok := goMajorMinor(moduleVersion)
	if !ok {
		return false
	}
	toolchainMajor, toolchainMinor, ok := goMajorMinor(strings.TrimPrefix(toolchainVersion, "go"))
	if !ok {
		return false
	}
	return moduleMajor < toolchainMajor || moduleMajor == toolchainMajor && moduleMinor <= toolchainMinor
}

func goMajorMinor(value string) (int, int, bool) {
	parts := strings.SplitN(value, ".", 3)
	if len(parts) < 2 {
		return 0, 0, false
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	minorText := parts[1]
	for index, character := range minorText {
		if character < '0' || character > '9' {
			minorText = minorText[:index]
			break
		}
	}
	minor, err := strconv.Atoi(minorText)
	if err != nil {
		return 0, 0, false
	}
	return major, minor, true
}

func moduleImportPath(module contextModuleRoot, directory string) (string, error) {
	if directory == module.Directory {
		return module.Path, nil
	}
	prefix := module.Directory
	if prefix != "" {
		prefix += "/"
	}
	if !strings.HasPrefix(directory, prefix) {
		return "", errSnapshotOutside
	}
	relative := strings.TrimPrefix(directory, prefix)
	if relative == "" {
		return module.Path, nil
	}
	return strings.TrimSuffix(module.Path, "/") + "/" + relative, nil
}

func (analyzer *contextAnalyzer) collectChangedImports(file File, source []byte, packagePath string) {
	fileSet := token.NewFileSet()
	reparsed, err := parser.ParseFile(fileSet, file.Path, source, parser.ImportsOnly)
	if err != nil || reparsed == nil {
		analyzer.addOmission(ContextOmissionParseError, 1)
		return
	}
	tokenFile := fileSet.File(reparsed.Pos())
	if tokenFile == nil {
		analyzer.addOmission(ContextOmissionParseError, 1)
		return
	}
	for _, specification := range reparsed.Imports {
		startLine := fileSet.Position(specification.Pos()).Line
		endLine := fileSet.Position(specification.End()).Line
		if len(intersections(file.ChangedLines, LineRange{Start: startLine, End: endLine})) == 0 {
			continue
		}
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil || !validContextImportPath(importPath) {
			analyzer.addOmission(ContextOmissionParseError, 1)
			continue
		}
		alias := ""
		if specification.Name != nil {
			alias = specification.Name.Name
		}
		identity := importPath
		if alias != "" {
			identity = alias + " " + importPath
		}
		startOffset := tokenFile.Offset(specification.Pos())
		endOffset := tokenFile.Offset(specification.End())
		if startOffset < 0 || endOffset < startOffset || endOffset > len(source) {
			analyzer.addOmission(ContextOmissionParseError, 1)
			continue
		}
		key := fmt.Sprintf("import:%s:%06d:%s", file.Path, startLine, identity)
		analyzer.itemCandidates[key] = contextItemCandidate{
			key: key,
			item: ContextItem{
				Kind:        ContextItemChangedImport,
				Path:        file.Path,
				PackagePath: packagePath,
				Identity:    identity,
				StartLine:   startLine,
				EndLine:     endLine,
				Content:     string(source[startOffset:endOffset]),
			},
		}
		from := fmt.Sprintf("%s#import@%d", file.Path, startLine)
		target := "package:" + importPath
		analyzer.addRelation(contextRelationCandidate{
			from:     from,
			target:   target,
			kind:     ContextRelationImports,
			strength: ContextRelationSyntactic,
		})
	}
}

func (analyzer *contextAnalyzer) discoverDirectPackages(ctx context.Context) error {
	edges := make(map[string]struct{})
	for _, key := range sortedContextPackageKeys(analyzer.changedPackages) {
		packageValue := analyzer.changedPackages[key]
		for _, file := range packageValue.files {
			for _, specification := range file.parsed.Imports {
				importPath, err := strconv.Unquote(specification.Path.Value)
				if err != nil || !validContextImportPath(importPath) {
					analyzer.addOmission(ContextOmissionParseError, 1)
					continue
				}
				edgeKey := packageValue.key + "\x00" + importPath
				if _, duplicate := edges[edgeKey]; duplicate {
					continue
				}
				edges[edgeKey] = struct{}{}
				analyzer.directImportEdges++
				if analyzer.directImportEdges > analyzer.limits.MaxDirectImportEdges {
					return &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
				}
				if importPath == "C" {
					analyzer.addUniqueOmission(ContextOmissionCGOUnsupported, edgeKey)
					continue
				}
				module, directory, local, err := analyzer.resolveImport(ctx, importPath)
				if err != nil {
					return err
				}
				if !local {
					analyzer.addUniqueOmission(ContextOmissionExternalTypeUnavailable, edgeKey)
					continue
				}
				directoryValue, err := analyzer.loadDirectory(ctx, directory)
				if err != nil {
					return err
				}
				name, files := productionContextPackage(directoryValue.groups)
				if len(files) == 0 {
					analyzer.addOmission(ContextOmissionTypeIncomplete, 1)
					continue
				}
				if _, exists := analyzer.packages[importPath]; exists {
					continue
				}
				if len(analyzer.packages) == analyzer.limits.MaxPackages {
					return &contextFailure{Reason: ContextOmissionAnalysisLimitExceeded}
				}
				moduleCopy := module
				analyzer.packages[importPath] = newContextPackage(importPath, importPath, name, directory, &moduleCopy, directoryValue.fileSet, files, directoryValue.incomplete)
			}
		}
	}
	return nil
}

func productionContextPackage(groups map[string][]*contextSourceFile) (string, []*contextSourceFile) {
	names := make([]string, 0, len(groups))
	for name := range groups {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	if len(names) != 1 {
		return "", nil
	}
	return names[0], groups[names[0]]
}

func (analyzer *contextAnalyzer) resolveImport(ctx context.Context, importPath string) (contextModuleRoot, string, bool, error) {
	for prefix := range analyzer.blockedReplacements {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return contextModuleRoot{}, "", false, nil
		}
	}
	prefixes := make([]string, 0, len(analyzer.moduleMappings))
	for prefix := range analyzer.moduleMappings {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			prefixes = append(prefixes, prefix)
		}
	}
	sort.Slice(prefixes, func(i, j int) bool {
		if len(prefixes[i]) == len(prefixes[j]) {
			return prefixes[i] < prefixes[j]
		}
		return len(prefixes[i]) > len(prefixes[j])
	})
	if len(prefixes) == 0 {
		return contextModuleRoot{}, "", false, nil
	}
	mapping := analyzer.moduleMappings[prefixes[0]]
	suffix := strings.TrimPrefix(importPath, prefixes[0])
	suffix = strings.TrimPrefix(suffix, "/")
	directory := mapping.Directory
	if suffix != "" {
		directory = joinSnapshotPath(directory, suffix)
	}
	nearest, err := analyzer.nearestModuleDirectory(ctx, directory)
	if err != nil {
		var failure *contextFailure
		if errors.As(err, &failure) && failure.Reason == ContextOmissionUnsupportedModuleLayout {
			return contextModuleRoot{}, "", false, nil
		}
		return contextModuleRoot{}, "", false, err
	}
	if nearest != mapping.Directory {
		return contextModuleRoot{}, "", false, nil
	}
	return mapping, directory, true, nil
}

func (analyzer *contextAnalyzer) nearestModuleDirectory(ctx context.Context, directory string) (string, error) {
	for {
		_, err := analyzer.snapshot.ReadFile(ctx, joinSnapshotPath(directory, "go.mod"), int64(analyzer.limits.MaxSourceBytesPerFile))
		switch {
		case err == nil:
			return directory, nil
		case errors.Is(err, errSnapshotNotFound):
			if directory == "" {
				return "", &contextFailure{Reason: ContextOmissionUnsupportedModuleLayout}
			}
			directory = snapshotPathDirectory(directory)
		default:
			return "", analyzer.snapshotFailure(err)
		}
	}
}

type contextImporter struct {
	analyzer *contextAnalyzer
	current  *contextPackage
}

func (importer *contextImporter) Import(importPath string) (*types.Package, error) {
	if packageValue, ok := importer.analyzer.packages[importPath]; ok {
		if packageValue.state == contextPackageChecking {
			importer.current.incompleteInputs = true
			return packageValue.typesPkg, nil
		}
		importer.analyzer.checkPackage(packageValue)
		if !packageValue.complete {
			importer.current.incompleteInputs = true
		}
		return packageValue.typesPkg, nil
	}
	importer.current.incompleteInputs = true
	if !importer.analyzer.isKnownLocalImport(importPath) {
		reason := ContextOmissionExternalTypeUnavailable
		if importPath == "C" {
			reason = ContextOmissionCGOUnsupported
		}
		importer.analyzer.addUniqueOmission(reason, importer.current.key+"\x00"+importPath)
	}
	if packageValue, ok := importer.analyzer.fakePackages[importPath]; ok {
		return packageValue, nil
	}
	name := path.Base(importPath)
	if !token.IsIdentifier(name) {
		name = "dependency"
	}
	packageValue := types.NewPackage(importPath, name)
	packageValue.MarkComplete()
	importer.analyzer.fakePackages[importPath] = packageValue
	return packageValue, nil
}

func (analyzer *contextAnalyzer) isKnownLocalImport(importPath string) bool {
	for prefix := range analyzer.blockedReplacements {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return false
		}
	}
	for prefix := range analyzer.moduleMappings {
		if importPath == prefix || strings.HasPrefix(importPath, prefix+"/") {
			return true
		}
	}
	return false
}

func (analyzer *contextAnalyzer) checkPackage(packageValue *contextPackage) {
	if packageValue.state == contextPackageChecked {
		return
	}
	if packageValue.state == contextPackageChecking {
		packageValue.incompleteInputs = true
		analyzer.addOmission(ContextOmissionTypeIncomplete, 1)
		return
	}
	packageValue.state = contextPackageChecking
	packageValue.typesPkg = types.NewPackage(packageValue.key, packageValue.name)
	packageValue.typesInfo = &types.Info{
		Types:      make(map[ast.Expr]types.TypeAndValue),
		Defs:       make(map[*ast.Ident]types.Object),
		Uses:       make(map[*ast.Ident]types.Object),
		Implicits:  make(map[ast.Node]types.Object),
		Selections: make(map[*ast.SelectorExpr]*types.Selection),
	}
	configuration := &types.Config{
		Importer: &contextImporter{analyzer: analyzer, current: packageValue},
		Error:    func(error) {},
	}
	if packageValue.module.GoVersion != "" {
		configuration.GoVersion = "go" + packageValue.module.GoVersion
	}
	checker := types.NewChecker(configuration, packageValue.fileSet, packageValue.typesPkg, packageValue.typesInfo)
	err := checker.Files(packageValue.typesFiles)
	packageValue.state = contextPackageChecked
	packageValue.complete = err == nil && !packageValue.incompleteInputs
	if !packageValue.complete {
		analyzer.addOmission(ContextOmissionTypeIncomplete, 1)
	}
}

func (analyzer *contextAnalyzer) collectDeclarations() {
	for _, key := range sortedContextPackageKeys(analyzer.packages) {
		packageValue := analyzer.packages[key]
		for _, file := range packageValue.files {
			for _, declaration := range file.parsed.Decls {
				switch typed := declaration.(type) {
				case *ast.FuncDecl:
					kind := DeclarationFunction
					identity := typed.Name.Name
					if typed.Recv != nil && len(typed.Recv.List) > 0 {
						kind = DeclarationMethod
						receiver := formatNode(packageValue.fileSet, typed.Recv.List[0].Type)
						if strings.HasPrefix(receiver, "*") {
							identity = "(" + receiver + ")." + typed.Name.Name
						} else {
							identity = receiver + "." + typed.Name.Name
						}
					}
					analyzer.addDeclaration(packageValue, file, packageValue.typesInfo.Defs[typed.Name], typed, typed, kind, identity)
				case *ast.GenDecl:
					for _, specification := range typed.Specs {
						switch value := specification.(type) {
						case *ast.TypeSpec:
							kind := DeclarationType
							if _, ok := value.Type.(*ast.InterfaceType); ok {
								kind = DeclarationInterface
							}
							analyzer.addDeclaration(packageValue, file, packageValue.typesInfo.Defs[value.Name], value, typed, kind, value.Name.Name)
						case *ast.ValueSpec:
							kind := DeclarationVariable
							if typed.Tok == token.CONST {
								kind = DeclarationConstant
							}
							for _, name := range value.Names {
								analyzer.addDeclaration(packageValue, file, packageValue.typesInfo.Defs[name], value, typed, kind, name.Name)
							}
						}
					}
				}
			}
		}
	}
}

func (analyzer *contextAnalyzer) addDeclaration(
	packageValue *contextPackage,
	file *contextSourceFile,
	object types.Object,
	matchNode ast.Node,
	excerptNode ast.Node,
	kind DeclarationKind,
	identity string,
) {
	if object == nil || matchNode == nil || excerptNode == nil || identity == "" {
		return
	}
	content, ok := contextNodeExcerpt(file, excerptNode)
	if !ok {
		analyzer.addOmission(ContextOmissionParseError, 1)
		return
	}
	declaration := &contextDeclaration{
		object:       object,
		path:         file.path,
		packagePath:  packageValue.importPath,
		kind:         kind,
		identity:     identity,
		matchStart:   packageValue.fileSet.Position(matchNode.Pos()).Line,
		matchEnd:     packageValue.fileSet.Position(matchNode.End()).Line,
		startLine:    packageValue.fileSet.Position(excerptNode.Pos()).Line,
		endLine:      packageValue.fileSet.Position(excerptNode.End()).Line,
		content:      content,
		inspectNode:  excerptNode,
		packageValue: packageValue,
	}
	packageValue.declarations = append(packageValue.declarations, declaration)
	analyzer.objectDeclarations[object] = declaration
}

func contextNodeExcerpt(file *contextSourceFile, node ast.Node) (string, bool) {
	start := file.tokenFile.Offset(node.Pos())
	end := file.tokenFile.Offset(node.End())
	if start < 0 || end < start || end > len(file.source) {
		return "", false
	}
	return string(file.source[start:end]), true
}

func (analyzer *contextAnalyzer) collectRelations() {
	selected := make(map[string]struct{})
	changedDeclarations := make([]*contextDeclaration, 0)
	for _, file := range analyzer.result.Files {
		for _, declaration := range file.Declarations {
			key := selectedDeclarationKey(file.Path, declaration.Identity, declaration.StartLine)
			selected[key] = struct{}{}
			if value := analyzer.findChangedDeclaration(file.Path, declaration); value != nil {
				changedDeclarations = append(changedDeclarations, value)
			}
		}
	}
	sort.Slice(changedDeclarations, func(i, j int) bool {
		return contextDeclarationKey(changedDeclarations[i]) < contextDeclarationKey(changedDeclarations[j])
	})
	for _, changed := range changedDeclarations {
		from := changed.path + "#" + changed.identity
		analyzer.collectSyntacticImportRelations(changed, from)
		if changed.packageValue.complete {
			ast.Inspect(changed.inspectNode, func(node ast.Node) bool {
				identifier, ok := node.(*ast.Ident)
				if !ok {
					return true
				}
				object := changed.packageValue.typesInfo.Uses[identifier]
				target := analyzer.objectDeclarations[object]
				if target == nil || !target.packageValue.complete {
					return true
				}
				if _, isSelected := selected[selectedDeclarationKey(target.path, target.identity, target.matchStart)]; isSelected {
					return true
				}
				targetKey := analyzer.addDeclarationCandidate(target)
				analyzer.addRelation(contextRelationCandidate{
					from:      from,
					targetKey: targetKey,
					kind:      ContextRelationReferences,
					strength:  ContextRelationTypeChecked,
				})
				return true
			})
		}
		analyzer.collectImplementationRelations(changed, from, selected)
	}
}

func (analyzer *contextAnalyzer) findChangedDeclaration(filePath string, changed Declaration) *contextDeclaration {
	for _, packageValue := range analyzer.changedPackages {
		for _, declaration := range packageValue.declarations {
			if declaration.path == filePath && declaration.identity == changed.Identity && declaration.matchStart == changed.StartLine {
				return declaration
			}
		}
	}
	return nil
}

func (analyzer *contextAnalyzer) collectSyntacticImportRelations(changed *contextDeclaration, from string) {
	var sourceFile *contextSourceFile
	for _, file := range changed.packageValue.files {
		if file.path == changed.path {
			sourceFile = file
			break
		}
	}
	if sourceFile == nil {
		return
	}
	aliases := make(map[string]string)
	for _, specification := range sourceFile.parsed.Imports {
		importPath, err := strconv.Unquote(specification.Path.Value)
		if err != nil {
			continue
		}
		alias := path.Base(importPath)
		if specification.Name != nil {
			alias = specification.Name.Name
		}
		if alias == "_" || alias == "." || !token.IsIdentifier(alias) {
			continue
		}
		aliases[alias] = importPath
	}
	ast.Inspect(changed.inspectNode, func(node ast.Node) bool {
		selector, ok := node.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		identifier, ok := selector.X.(*ast.Ident)
		if !ok {
			return true
		}
		importPath, ok := aliases[identifier.Name]
		if !ok {
			return true
		}
		analyzer.addRelation(contextRelationCandidate{
			from:     from,
			target:   "package:" + importPath,
			kind:     ContextRelationImports,
			strength: ContextRelationSyntactic,
		})
		return true
	})
}

func (analyzer *contextAnalyzer) collectImplementationRelations(changed *contextDeclaration, from string, selected map[string]struct{}) {
	if !changed.packageValue.complete {
		return
	}
	named, ok := declarationNamedType(changed.object)
	if !ok {
		return
	}
	for _, packageValue := range analyzer.packages {
		if !packageValue.complete {
			continue
		}
		for _, target := range packageValue.declarations {
			if target.kind != DeclarationInterface {
				continue
			}
			interfaceName, ok := target.object.(*types.TypeName)
			if !ok {
				continue
			}
			interfaceType, ok := interfaceName.Type().Underlying().(*types.Interface)
			if !ok {
				continue
			}
			if !types.Implements(named, interfaceType) && !types.Implements(types.NewPointer(named), interfaceType) {
				continue
			}
			if _, isSelected := selected[selectedDeclarationKey(target.path, target.identity, target.matchStart)]; isSelected {
				continue
			}
			targetKey := analyzer.addDeclarationCandidate(target)
			analyzer.addRelation(contextRelationCandidate{
				from:      from,
				targetKey: targetKey,
				kind:      ContextRelationImplements,
				strength:  ContextRelationTypeChecked,
			})
		}
	}
}

func declarationNamedType(object types.Object) (*types.Named, bool) {
	var candidate types.Type
	switch typed := object.(type) {
	case *types.TypeName:
		candidate = typed.Type()
	case *types.Func:
		signature, ok := typed.Type().(*types.Signature)
		if !ok || signature.Recv() == nil {
			return nil, false
		}
		candidate = signature.Recv().Type()
	default:
		return nil, false
	}
	if pointer, ok := candidate.(*types.Pointer); ok {
		candidate = pointer.Elem()
	}
	named, ok := candidate.(*types.Named)
	return named, ok
}

func (analyzer *contextAnalyzer) addDeclarationCandidate(declaration *contextDeclaration) string {
	key := "declaration:" + contextDeclarationKey(declaration)
	if _, exists := analyzer.itemCandidates[key]; !exists {
		analyzer.itemCandidates[key] = contextItemCandidate{
			key: key,
			item: ContextItem{
				Kind:            ContextItemContextDeclaration,
				Path:            declaration.path,
				PackagePath:     declaration.packagePath,
				DeclarationKind: declaration.kind,
				Identity:        declaration.identity,
				StartLine:       declaration.startLine,
				EndLine:         declaration.endLine,
				Content:         declaration.content,
			},
		}
	}
	return key
}

func (analyzer *contextAnalyzer) addRelation(candidate contextRelationCandidate) {
	key := strings.Join([]string{candidate.from, string(candidate.kind), candidate.targetKey, candidate.target, string(candidate.strength)}, "\x00")
	analyzer.relationCandidates[key] = candidate
}

func selectedDeclarationKey(pathValue, identity string, startLine int) string {
	return fmt.Sprintf("%s\x00%s\x00%09d", pathValue, identity, startLine)
}

func contextDeclarationKey(declaration *contextDeclaration) string {
	return selectedDeclarationKey(declaration.path, declaration.identity, declaration.matchStart)
}

func (analyzer *contextAnalyzer) addOmission(reason ContextOmissionReason, count int) {
	if count <= 0 {
		return
	}
	analyzer.omissionCounts[reason] += count
}

func (analyzer *contextAnalyzer) addUniqueOmission(reason ContextOmissionReason, key string) {
	if analyzer.omissionKeys == nil {
		analyzer.omissionKeys = make(map[ContextOmissionReason]map[string]struct{})
	}
	keys := analyzer.omissionKeys[reason]
	if keys == nil {
		keys = make(map[string]struct{})
		analyzer.omissionKeys[reason] = keys
	}
	if _, exists := keys[key]; exists {
		return
	}
	keys[key] = struct{}{}
	analyzer.addOmission(reason, 1)
}

func (analyzer *contextAnalyzer) finish() {
	analyzer.output.AnalyzedPackageCount = len(analyzer.packages)
	analyzer.output.AnalyzedFileCount = analyzer.analyzedFiles
	analyzer.output.AnalyzedSourceBytes = analyzer.analyzedSourceBytes
	analyzer.output.DirectImportEdges = analyzer.directImportEdges
	sort.Slice(analyzer.output.Build.Modules, func(i, j int) bool {
		if analyzer.output.Build.Modules[i].Directory == analyzer.output.Build.Modules[j].Directory {
			return analyzer.output.Build.Modules[i].Path < analyzer.output.Build.Modules[j].Path
		}
		return analyzer.output.Build.Modules[i].Directory < analyzer.output.Build.Modules[j].Directory
	})
	sort.Slice(analyzer.output.Build.Workspaces, func(i, j int) bool {
		return analyzer.output.Build.Workspaces[i].Directory < analyzer.output.Build.Workspaces[j].Directory
	})
	sort.Slice(analyzer.output.Build.Replacements, func(i, j int) bool {
		if analyzer.output.Build.Replacements[i].ModulePath == analyzer.output.Build.Replacements[j].ModulePath {
			return analyzer.output.Build.Replacements[i].Directory < analyzer.output.Build.Replacements[j].Directory
		}
		return analyzer.output.Build.Replacements[i].ModulePath < analyzer.output.Build.Replacements[j].ModulePath
	})
	if analyzer.output.Status != ContextUnavailable {
		if repositoryDerivedBuildBytes(analyzer.output.Build) > analyzer.limits.MaxOutputBytes {
			analyzer.output.Status = ContextUnavailable
			analyzer.output.Build.Modules = []ContextModule{}
			analyzer.output.Build.Workspaces = []ContextWorkspace{}
			analyzer.output.Build.Replacements = []ContextReplacement{}
			analyzer.omissionCounts = map[ContextOmissionReason]int{ContextOmissionAnalysisLimitExceeded: 1}
		} else {
			analyzer.applyOutputLimits()
		}
	}
	if analyzer.output.Status != ContextUnavailable && len(analyzer.omissionCounts) > 0 {
		analyzer.output.Status = ContextPartial
	}
	analyzer.output.Omissions = orderedContextOmissions(analyzer.omissionCounts)
}

func (analyzer *contextAnalyzer) applyOutputLimits() {
	remainingBytes := analyzer.limits.MaxOutputBytes - repositoryDerivedBuildBytes(analyzer.output.Build)
	keys := make([]string, 0, len(analyzer.itemCandidates))
	for key := range analyzer.itemCandidates {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	selectedFiles := make(map[string]struct{})
	omittedFiles := make(map[string]struct{})
	selectedIDs := make(map[string]string)
	for _, key := range keys {
		candidate := analyzer.itemCandidates[key]
		item := candidate.item
		if _, exists := selectedFiles[item.Path]; !exists && len(selectedFiles) == analyzer.limits.MaxOutputFiles {
			omittedFiles[item.Path] = struct{}{}
			analyzer.output.Truncation.OmittedItems++
			analyzer.output.Truncation.OmittedBytes += contextItemDerivedBytes(item)
			continue
		}
		if len(analyzer.output.Items) == analyzer.limits.MaxOutputItems {
			if _, exists := selectedFiles[item.Path]; !exists {
				omittedFiles[item.Path] = struct{}{}
			}
			analyzer.output.Truncation.OmittedItems++
			analyzer.output.Truncation.OmittedBytes += contextItemDerivedBytes(item)
			continue
		}
		originalContentBytes := len(item.Content)
		if originalContentBytes > analyzer.limits.MaxExcerptBytes {
			item.Content = validUTF8Prefix(item.Content, analyzer.limits.MaxExcerptBytes)
			item.Truncated = true
			analyzer.output.Truncation.OmittedBytes += originalContentBytes - len(item.Content)
		}
		fixedBytes := len(item.Path) + len(item.PackagePath) + len(item.Identity)
		if fixedBytes > remainingBytes {
			if _, exists := selectedFiles[item.Path]; !exists {
				omittedFiles[item.Path] = struct{}{}
			}
			analyzer.output.Truncation.OmittedItems++
			analyzer.output.Truncation.OmittedBytes += fixedBytes + len(item.Content)
			continue
		}
		contentBudget := remainingBytes - fixedBytes
		if len(item.Content) > contentBudget {
			original := len(item.Content)
			item.Content = validUTF8Prefix(item.Content, contentBudget)
			item.Truncated = true
			analyzer.output.Truncation.OmittedBytes += original - len(item.Content)
		}
		if item.Content == "" && originalContentBytes > 0 {
			analyzer.output.Truncation.OmittedItems++
			analyzer.output.Truncation.OmittedBytes += fixedBytes
			continue
		}
		item.ID = fmt.Sprintf("C%03d", len(analyzer.output.Items)+1)
		item.ContentBytes = len(item.Content)
		analyzer.output.Items = append(analyzer.output.Items, item)
		selectedFiles[item.Path] = struct{}{}
		selectedIDs[candidate.key] = item.ID
		remainingBytes -= fixedBytes + len(item.Content)
	}
	analyzer.output.Truncation.OmittedFiles = len(omittedFiles)

	relationKeys := make([]string, 0, len(analyzer.relationCandidates))
	for key := range analyzer.relationCandidates {
		relationKeys = append(relationKeys, key)
	}
	sort.Strings(relationKeys)
	for _, key := range relationKeys {
		candidate := analyzer.relationCandidates[key]
		target := candidate.target
		if candidate.targetKey != "" {
			var selected bool
			target, selected = selectedIDs[candidate.targetKey]
			if !selected {
				analyzer.output.Truncation.OmittedRelations++
				analyzer.output.Truncation.OmittedBytes += len(candidate.from) + len(candidate.targetKey)
				continue
			}
		}
		if len(analyzer.output.Relations) == analyzer.limits.MaxRelations {
			analyzer.output.Truncation.OmittedRelations++
			analyzer.output.Truncation.OmittedBytes += len(candidate.from) + len(target)
			continue
		}
		relationBytes := len(candidate.from) + len(target)
		if relationBytes > remainingBytes {
			analyzer.output.Truncation.OmittedRelations++
			analyzer.output.Truncation.OmittedBytes += relationBytes
			continue
		}
		analyzer.output.Relations = append(analyzer.output.Relations, ContextRelation{
			From:     candidate.from,
			To:       target,
			Kind:     candidate.kind,
			Strength: candidate.strength,
		})
		remainingBytes -= relationBytes
	}
	truncation := &analyzer.output.Truncation
	truncation.Truncated = truncation.OmittedFiles > 0 || truncation.OmittedItems > 0 || truncation.OmittedRelations > 0 || truncation.OmittedBytes > 0
	if truncation.Truncated {
		count := truncation.OmittedItems + truncation.OmittedRelations
		if count == 0 {
			count = 1
		}
		analyzer.addOmission(ContextOmissionOutputTruncated, count)
	}
}

func contextItemDerivedBytes(item ContextItem) int {
	return len(item.Path) + len(item.PackagePath) + len(item.Identity) + len(item.Content)
}

func repositoryDerivedBuildBytes(configuration GoBuildConfiguration) int {
	total := 0
	for _, module := range configuration.Modules {
		total += len(module.Path) + len(module.Directory) + len(module.GoVersion) + len(module.Toolchain)
	}
	for _, workspace := range configuration.Workspaces {
		total += len(workspace.Directory) + len(workspace.GoVersion) + len(workspace.Toolchain)
	}
	for _, replacement := range configuration.Replacements {
		total += len(replacement.ModulePath) + len(replacement.Directory)
	}
	return total
}

func orderedContextOmissions(counts map[ContextOmissionReason]int) []ContextOmission {
	order := []ContextOmissionReason{
		ContextOmissionAnalysisLimitExceeded,
		ContextOmissionUnsupportedModuleLayout,
		ContextOmissionUnsupportedGoVersion,
		ContextOmissionOutsideRepositoryDependency,
		ContextOmissionCGOUnsupported,
		ContextOmissionExternalTypeUnavailable,
		ContextOmissionParseError,
		ContextOmissionTypeIncomplete,
		ContextOmissionOutputTruncated,
	}
	result := make([]ContextOmission, 0, len(counts))
	for _, reason := range order {
		if count := counts[reason]; count > 0 {
			result = append(result, ContextOmission{Reason: reason, Count: count})
		}
	}
	return result
}
