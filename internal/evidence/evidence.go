// Package evidence builds a bounded preview of Go declarations affected by an
// explicit Git changeset. Git invocation, diff parsing, and Go syntax parsing
// are implementation details behind Preview.
package evidence

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"
)

const WorkingTreeRevision = "WORKTREE"

type Selection struct {
	Base string
	Head string
}

func CommitRange(base, head string) Selection {
	return Selection{Base: base, Head: head}
}

func WorkingTree(base string) Selection {
	return Selection{Base: base}
}

type Limits struct {
	MaxFiles        int
	MaxDeclarations int
	MaxExcerptBytes int
}

type Request struct {
	Repository string
	Selection  Selection
	Limits     Limits
	Context    ContextMode
}

type ContextMode string

const (
	ContextDisabled ContextMode = ""
	ContextGo       ContextMode = "go"
)

type ErrorCode string

const (
	ErrorUnknown           ErrorCode = "unknown"
	ErrorInvalidRequest    ErrorCode = "invalid_request"
	ErrorNotRepository     ErrorCode = "not_repository"
	ErrorInvalidRevision   ErrorCode = "invalid_revision"
	ErrorGit               ErrorCode = "git_failed"
	ErrorReadSource        ErrorCode = "read_source_failed"
	ErrorParseSource       ErrorCode = "parse_source_failed"
	ErrorOutsideRepository ErrorCode = "outside_repository"
)

type PreviewError struct {
	Code      ErrorCode
	Operation string
	Err       error
}

func (err *PreviewError) Error() string {
	if err.Operation == "" {
		return err.Err.Error()
	}
	return err.Operation + ": " + err.Err.Error()
}

func (err *PreviewError) Unwrap() error {
	return err.Err
}

func ErrorCodeOf(err error) ErrorCode {
	var previewError *PreviewError
	if errors.As(err, &previewError) {
		return previewError.Code
	}
	return ErrorUnknown
}

type Result struct {
	RepositoryRoot string
	BaseRevision   string
	HeadRevision   string
	AppliedLimits  Limits
	Files          []File
	Truncation     Truncation
	GoContext      *GoContext
}

type File struct {
	Path         string
	Status       FileStatus
	ChangedLines []LineRange
	Declarations []Declaration
	Omissions    []Omission
}

type FileStatus string

const (
	FileAdded    FileStatus = "added"
	FileModified FileStatus = "modified"
	FileDeleted  FileStatus = "deleted"
)

type LineRange struct {
	Start int
	End   int
}

type OmissionReason string

const (
	OmissionDeletedFile        OmissionReason = "deleted_file"
	OmissionDeletedOnlyHunk    OmissionReason = "deleted_only_hunk"
	OmissionOutsideDeclaration OmissionReason = "outside_declaration"
)

type Omission struct {
	Reason OmissionReason
	Count  int
}

type DeclarationKind string

const (
	DeclarationFunction  DeclarationKind = "function"
	DeclarationMethod    DeclarationKind = "method"
	DeclarationType      DeclarationKind = "type"
	DeclarationInterface DeclarationKind = "interface"
	DeclarationVariable  DeclarationKind = "variable"
	DeclarationConstant  DeclarationKind = "constant"
)

type Declaration struct {
	Kind             DeclarationKind
	Name             string
	Receiver         string
	Identity         string
	StartLine        int
	EndLine          int
	ChangedLines     []LineRange
	Excerpt          string
	ExcerptTruncated bool
}

type Truncation struct {
	Truncated           bool
	OmittedFiles        int
	OmittedDeclarations int
	OmittedExcerptBytes int
}

var hunkHeader = regexp.MustCompile(`^@@ -[0-9]+(?:,[0-9]+)? \+([0-9]+)(?:,([0-9]+))? @@`)

func Preview(ctx context.Context, request Request) (Result, error) {
	if err := validateRequest(request); err != nil {
		return Result{}, err
	}
	if request.Context == ContextGo {
		ctx = withLocalOnlyGit(ctx)
	}

	root, err := ResolveRepositoryRoot(ctx, request.Repository)
	if err != nil {
		return Result{}, err
	}
	base, err := resolveCommit(ctx, root, request.Selection.Base)
	if err != nil {
		return Result{}, fmt.Errorf("resolve base revision: %w", err)
	}

	head := WorkingTreeRevision
	if request.Selection.Head != "" {
		head, err = resolveCommit(ctx, root, request.Selection.Head)
		if err != nil {
			return Result{}, fmt.Errorf("resolve head revision: %w", err)
		}
	}

	files, err := changedFiles(ctx, root, base, head)
	if err != nil {
		return Result{}, err
	}

	result := Result{
		RepositoryRoot: root,
		BaseRevision:   base,
		HeadRevision:   head,
		AppliedLimits:  request.Limits,
	}
	for _, changed := range files {
		if len(result.Files) == request.Limits.MaxFiles {
			result.Truncation.OmittedFiles += len(files) - len(result.Files)
			break
		}

		var source []byte
		if changed.Status != FileDeleted {
			source, err = sourceAt(ctx, root, head, changed.Path)
			if err != nil {
				return Result{}, err
			}
		}
		ranges, deletedOnlyHunks, err := changedLineRanges(ctx, root, base, head, changed.Path)
		if err != nil {
			return Result{}, err
		}
		if head == WorkingTreeRevision && changed.Status == FileAdded && len(ranges) == 0 && len(source) > 0 {
			ranges = []LineRange{{Start: 1, End: lineCount(source)}}
		}
		file := File{Path: changed.Path, Status: changed.Status, ChangedLines: ranges}
		if changed.Status == FileDeleted {
			file.Omissions = append(file.Omissions, Omission{Reason: OmissionDeletedFile, Count: 1})
		} else {
			if deletedOnlyHunks > 0 {
				file.Omissions = append(file.Omissions, Omission{Reason: OmissionDeletedOnlyHunk, Count: deletedOnlyHunks})
			}
			file.Declarations, err = mapDeclarations(changed.Path, source, ranges)
			if err != nil {
				return Result{}, err
			}
			if unmapped := countUnmappedRanges(ranges, file.Declarations); unmapped > 0 {
				file.Omissions = append(file.Omissions, Omission{Reason: OmissionOutsideDeclaration, Count: unmapped})
			}
		}
		result.Files = append(result.Files, file)
	}

	applyLimits(&result, request.Limits)
	if request.Context == ContextGo {
		result.GoContext = analyzeGoContext(ctx, root, head, result)
	}
	return result, nil
}

func validateRequest(request Request) error {
	if strings.TrimSpace(request.Repository) == "" {
		return previewError(ErrorInvalidRequest, "validate request", errors.New("repository is required"))
	}
	if strings.TrimSpace(request.Selection.Base) == "" {
		return previewError(ErrorInvalidRequest, "validate request", errors.New("base revision is required"))
	}
	if request.Limits.MaxFiles <= 0 || request.Limits.MaxDeclarations <= 0 || request.Limits.MaxExcerptBytes <= 0 {
		return previewError(ErrorInvalidRequest, "validate request", errors.New("all evidence limits must be positive"))
	}
	if request.Context != ContextDisabled && request.Context != ContextGo {
		return previewError(ErrorInvalidRequest, "validate request", errors.New("context mode is invalid"))
	}
	return nil
}

// ResolveRepositoryRoot verifies repository through Git and returns its
// canonical top-level working-tree path.
func ResolveRepositoryRoot(ctx context.Context, repository string) (string, error) {
	if strings.TrimSpace(repository) == "" {
		return "", previewError(ErrorInvalidRequest, "resolve repository path", errors.New("repository is required"))
	}
	absolute, err := filepath.Abs(repository)
	if err != nil {
		return "", previewError(ErrorInvalidRequest, "resolve repository path", err)
	}
	output, err := gitOutput(ctx, absolute, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", previewError(ErrorNotRepository, "find repository root", err)
	}
	return filepath.Clean(strings.TrimSpace(string(output))), nil
}

func resolveCommit(ctx context.Context, root, revision string) (string, error) {
	output, err := gitOutput(ctx, root, "rev-parse", "--verify", "--end-of-options", revision+"^{commit}")
	if err != nil {
		return "", previewError(ErrorInvalidRevision, "resolve revision "+strconv.Quote(revision), err)
	}
	return strings.TrimSpace(string(output)), nil
}

type changedFile struct {
	Path   string
	Status FileStatus
}

func changedFiles(ctx context.Context, root, base, head string) ([]changedFile, error) {
	var files []changedFile
	for _, item := range []struct {
		filter string
		status FileStatus
	}{
		{filter: "A", status: FileAdded},
		{filter: "MRTUXB", status: FileModified},
		{filter: "D", status: FileDeleted},
	} {
		arguments := diffArguments(base, head, "--name-only", "-z", "--diff-filter="+item.filter)
		output, err := gitOutput(ctx, root, arguments...)
		if err != nil {
			return nil, previewError(ErrorGit, "list changed files", err)
		}
		for _, path := range splitNUL(output) {
			if !strings.HasSuffix(path, ".go") {
				continue
			}
			files = append(files, changedFile{Path: path, Status: item.status})
		}
	}
	if head == WorkingTreeRevision {
		output, err := gitOutput(ctx, root, "ls-files", "--others", "--exclude-standard", "-z", "--", "*.go")
		if err != nil {
			return nil, previewError(ErrorGit, "list untracked Go files", err)
		}
		for _, path := range splitNUL(output) {
			files = append(files, changedFile{Path: path, Status: FileAdded})
		}
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, nil
}

func changedLineRanges(ctx context.Context, root, base, head, path string) ([]LineRange, int, error) {
	arguments := diffArguments(base, head, "--unified=0", "--no-color")
	arguments = append(arguments, "--", path)
	output, err := gitOutput(ctx, root, arguments...)
	if err != nil {
		return nil, 0, previewError(ErrorGit, "read diff for "+strconv.Quote(path), err)
	}

	var ranges []LineRange
	deletedOnlyHunks := 0
	scanner := bufio.NewScanner(bytes.NewReader(output))
	for scanner.Scan() {
		matches := hunkHeader.FindStringSubmatch(scanner.Text())
		if matches == nil {
			continue
		}
		start, _ := strconv.Atoi(matches[1])
		count := 1
		if matches[2] != "" {
			count, _ = strconv.Atoi(matches[2])
		}
		if count == 0 {
			deletedOnlyHunks++
			continue
		}
		ranges = append(ranges, LineRange{Start: start, End: start + count - 1})
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, previewError(ErrorGit, "scan diff for "+strconv.Quote(path), err)
	}
	return ranges, deletedOnlyHunks, nil
}

func diffArguments(base, head string, options ...string) []string {
	arguments := []string{"diff", "--no-ext-diff", "--no-renames"}
	arguments = append(arguments, options...)
	arguments = append(arguments, base)
	if head != WorkingTreeRevision {
		arguments = append(arguments, head)
	}
	if contains(options, "--name-only") {
		arguments = append(arguments, "--", "*.go")
	}
	return arguments
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sourceAt(ctx context.Context, root, head, path string) ([]byte, error) {
	if head == WorkingTreeRevision {
		return readWorkingTreeFile(root, path)
	}
	output, err := gitOutput(ctx, root, "show", head+":"+path)
	if err != nil {
		return nil, previewError(ErrorReadSource, "read "+strconv.Quote(path)+" at "+head, err)
	}
	return output, nil
}

func readWorkingTreeFile(root, path string) ([]byte, error) {
	if filepath.IsAbs(path) {
		return nil, previewError(ErrorOutsideRepository, "resolve working tree source", fmt.Errorf("path %q must be relative", path))
	}
	candidate := filepath.Join(root, filepath.FromSlash(path))
	resolved, err := filepath.EvalSymlinks(candidate)
	if err != nil {
		return nil, previewError(ErrorReadSource, "resolve working tree path "+strconv.Quote(path), err)
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil || relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return nil, previewError(ErrorOutsideRepository, "resolve working tree source", fmt.Errorf("path %q resolves outside repository", path))
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, previewError(ErrorReadSource, "stat working tree path "+strconv.Quote(path), err)
	}
	if !info.Mode().IsRegular() {
		return nil, previewError(ErrorReadSource, "read working tree source", fmt.Errorf("path %q is not a regular file", path))
	}
	content, err := os.ReadFile(resolved)
	if err != nil {
		return nil, previewError(ErrorReadSource, "read working tree path "+strconv.Quote(path), err)
	}
	return content, nil
}

func lineCount(content []byte) int {
	if len(content) == 0 {
		return 0
	}
	count := bytes.Count(content, []byte{'\n'})
	if content[len(content)-1] != '\n' {
		count++
	}
	return count
}

func mapDeclarations(path string, source []byte, ranges []LineRange) ([]Declaration, error) {
	fileSet := token.NewFileSet()
	parsed, err := parser.ParseFile(fileSet, path, source, parser.ParseComments)
	if err != nil {
		return nil, previewError(ErrorParseSource, "parse "+strconv.Quote(path), err)
	}
	tokenFile := fileSet.File(parsed.Pos())

	var declarations []Declaration
	for _, item := range parsed.Decls {
		switch declaration := item.(type) {
		case *ast.FuncDecl:
			kind := DeclarationFunction
			name := declaration.Name.Name
			receiver := ""
			identity := name
			if declaration.Recv != nil && len(declaration.Recv.List) > 0 {
				kind = DeclarationMethod
				receiver = formatNode(fileSet, declaration.Recv.List[0].Type)
				if strings.HasPrefix(receiver, "*") {
					identity = "(" + receiver + ")." + name
				} else {
					identity = receiver + "." + name
				}
			}
			declarations = appendMappedDeclaration(
				declarations,
				fileSet,
				tokenFile,
				source,
				ranges,
				declaration,
				declaration,
				kind,
				name,
				receiver,
				identity,
			)
		case *ast.GenDecl:
			for _, specification := range declaration.Specs {
				switch typed := specification.(type) {
				case *ast.TypeSpec:
					kind := DeclarationType
					if _, ok := typed.Type.(*ast.InterfaceType); ok {
						kind = DeclarationInterface
					}
					declarations = appendMappedDeclaration(declarations, fileSet, tokenFile, source, ranges, typed, declaration, kind, typed.Name.Name, "", typed.Name.Name)
				case *ast.ValueSpec:
					kind := DeclarationVariable
					if declaration.Tok == token.CONST {
						kind = DeclarationConstant
					}
					for _, name := range typed.Names {
						declarations = appendMappedDeclaration(declarations, fileSet, tokenFile, source, ranges, typed, declaration, kind, name.Name, "", name.Name)
					}
				}
			}
		}
	}
	return declarations, nil
}

func appendMappedDeclaration(
	declarations []Declaration,
	fileSet *token.FileSet,
	tokenFile *token.File,
	source []byte,
	ranges []LineRange,
	span ast.Node,
	excerpt ast.Node,
	kind DeclarationKind,
	name string,
	receiver string,
	identity string,
) []Declaration {
	start := fileSet.Position(span.Pos()).Line
	end := fileSet.Position(span.End()).Line
	changed := intersections(ranges, LineRange{Start: start, End: end})
	if len(changed) == 0 {
		return declarations
	}
	startOffset := tokenFile.Offset(excerpt.Pos())
	endOffset := tokenFile.Offset(excerpt.End())
	return append(declarations, Declaration{
		Kind:         kind,
		Name:         name,
		Receiver:     receiver,
		Identity:     identity,
		StartLine:    start,
		EndLine:      end,
		ChangedLines: changed,
		Excerpt:      string(source[startOffset:endOffset]),
	})
}

func formatNode(fileSet *token.FileSet, node ast.Node) string {
	var buffer bytes.Buffer
	if err := format.Node(&buffer, fileSet, node); err != nil {
		return ""
	}
	return buffer.String()
}

func intersections(ranges []LineRange, declaration LineRange) []LineRange {
	var result []LineRange
	for _, changed := range ranges {
		start := max(changed.Start, declaration.Start)
		end := min(changed.End, declaration.End)
		if start <= end {
			result = append(result, LineRange{Start: start, End: end})
		}
	}
	return result
}

func countUnmappedRanges(ranges []LineRange, declarations []Declaration) int {
	unmapped := 0
	for _, changed := range ranges {
		mapped := false
		for _, declaration := range declarations {
			for _, declarationChange := range declaration.ChangedLines {
				if changed.Start <= declarationChange.End && declarationChange.Start <= changed.End {
					mapped = true
					break
				}
			}
			if mapped {
				break
			}
		}
		if !mapped {
			unmapped++
		}
	}
	return unmapped
}

func applyLimits(result *Result, limits Limits) {
	remainingDeclarations := limits.MaxDeclarations
	remainingExcerptBytes := limits.MaxExcerptBytes
	for fileIndex := range result.Files {
		file := &result.Files[fileIndex]
		if len(file.Declarations) > remainingDeclarations {
			result.Truncation.OmittedDeclarations += len(file.Declarations) - remainingDeclarations
			file.Declarations = file.Declarations[:remainingDeclarations]
		}
		remainingDeclarations -= len(file.Declarations)
		for declarationIndex := range file.Declarations {
			declaration := &file.Declarations[declarationIndex]
			if len(declaration.Excerpt) <= remainingExcerptBytes {
				remainingExcerptBytes -= len(declaration.Excerpt)
				continue
			}
			originalLength := len(declaration.Excerpt)
			declaration.Excerpt = validUTF8Prefix(declaration.Excerpt, remainingExcerptBytes)
			result.Truncation.OmittedExcerptBytes += originalLength - len(declaration.Excerpt)
			declaration.ExcerptTruncated = true
			remainingExcerptBytes -= len(declaration.Excerpt)
		}
	}
	result.Truncation.Truncated = result.Truncation.OmittedFiles > 0 ||
		result.Truncation.OmittedDeclarations > 0 ||
		result.Truncation.OmittedExcerptBytes > 0
}

func validUTF8Prefix(value string, maximumBytes int) string {
	if len(value) <= maximumBytes {
		return value
	}
	if maximumBytes <= 0 {
		return ""
	}
	prefix := value[:maximumBytes]
	for len(prefix) > 0 && !utf8.ValidString(prefix) {
		prefix = prefix[:len(prefix)-1]
	}
	return prefix
}

func splitNUL(output []byte) []string {
	parts := bytes.Split(output, []byte{0})
	values := make([]string, 0, len(parts))
	for _, part := range parts {
		if len(part) > 0 {
			values = append(values, string(part))
		}
	}
	return values
}

func gitOutput(ctx context.Context, root string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, "git", append([]string{"-C", root}, args...)...)
	command.Env = gitCommandEnvironment(ctx)
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if message == "" {
			message = err.Error()
		}
		return nil, errors.New(message)
	}
	return output, nil
}

type localOnlyGitContextKey struct{}

func withLocalOnlyGit(ctx context.Context) context.Context {
	return context.WithValue(ctx, localOnlyGitContextKey{}, struct{}{})
}

func gitCommandEnvironment(ctx context.Context) []string {
	if _, localOnly := ctx.Value(localOnlyGitContextKey{}).(struct{}); !localOnly {
		return nil
	}
	environment := append([]string(nil), os.Environ()...)
	return append(environment,
		"GIT_NO_LAZY_FETCH=1",
		"GIT_TERMINAL_PROMPT=0",
		"GIT_OPTIONAL_LOCKS=0",
	)
}

func previewError(code ErrorCode, operation string, err error) error {
	return &PreviewError{Code: code, Operation: operation, Err: err}
}
