// Package evaluator defines the versioned, provider-independent contract at
// the boundary between retained evidence and an isolated evaluator.
package evaluator

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

const (
	InputSchemaVersion       = 1
	QuestionSetSchemaVersion = 1
	MaxQuestionSetBytes      = 64 * 1024
	MaxQuestionTextBytes     = 1000
)

type ContractErrorCode string

const (
	ContractErrorUnknown       ContractErrorCode = "unknown"
	ContractErrorInvalidInput  ContractErrorCode = "invalid_input"
	ContractErrorInvalidOutput ContractErrorCode = "invalid_output"
)

type ContractError struct {
	Code ContractErrorCode
	Err  error
}

func (err *ContractError) Error() string {
	return err.Err.Error()
}

func (err *ContractError) Unwrap() error {
	return err.Err
}

func ContractErrorCodeOf(err error) ContractErrorCode {
	var contractError *ContractError
	if errors.As(err, &contractError) {
		return contractError.Code
	}
	return ContractErrorUnknown
}

// Input is the only runtime value that may be sent to the evaluator. It owns a
// copy of the selected bundle and intentionally has no repository-root field.
type Input struct {
	SchemaVersion  int            `json:"schema_version"`
	EvidenceBundle EvidenceBundle `json:"evidence_bundle"`
}

type EvidenceBundle struct {
	FormatVersion    int                `json:"format_version"`
	ID               string             `json:"id"`
	ManifestSHA256   string             `json:"manifest_sha256"`
	BaseRevision     string             `json:"base_revision"`
	HeadRevision     string             `json:"head_revision"`
	AppliedLimits    EvidenceLimits     `json:"applied_limits"`
	FileCount        int                `json:"file_count"`
	DeclarationCount int                `json:"declaration_count"`
	EvidenceCount    int                `json:"evidence_count"`
	ApproximateBytes int                `json:"approximate_bytes"`
	Files            []EvidenceFile     `json:"files"`
	Items            []EvidenceItem     `json:"items"`
	Truncation       EvidenceTruncation `json:"truncation"`
	GoContext        *EvidenceGoContext `json:"go_context,omitempty"`
}

type EvidenceLimits struct {
	MaxFiles        int `json:"max_files"`
	MaxDeclarations int `json:"max_declarations"`
	MaxExcerptBytes int `json:"max_excerpt_bytes"`
}

type EvidenceFile struct {
	Path               string              `json:"path"`
	Status             string              `json:"status"`
	ChangedLines       []EvidenceLineRange `json:"changed_lines"`
	EvidenceReferences []string            `json:"evidence_references"`
	Omissions          []EvidenceOmission  `json:"omissions"`
}

type EvidenceItem struct {
	Reference       string              `json:"reference"`
	Kind            string              `json:"kind"`
	Path            string              `json:"path"`
	DeclarationKind string              `json:"declaration_kind"`
	Identity        string              `json:"identity"`
	StartLine       int                 `json:"start_line"`
	EndLine         int                 `json:"end_line"`
	ChangedLines    []EvidenceLineRange `json:"changed_lines"`
	Content         string              `json:"content"`
	ContentBytes    int                 `json:"content_bytes"`
	ContentSHA256   string              `json:"content_sha256"`
	Truncated       bool                `json:"truncated"`
}

type EvidenceLineRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type EvidenceOmission struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type EvidenceTruncation struct {
	Truncated           bool `json:"truncated"`
	OmittedFiles        int  `json:"omitted_files"`
	OmittedDeclarations int  `json:"omitted_declarations"`
	OmittedExcerptBytes int  `json:"omitted_excerpt_bytes"`
}

// NewInput validates the complete domain bundle before copying it into the
// independently versioned runtime schema.
func NewInput(bundle evidence.Bundle) (Input, error) {
	if err := validateBundle(bundle); err != nil {
		return Input{}, invalidInput(err)
	}

	runtimeBundle := EvidenceBundle{
		FormatVersion:    bundle.FormatVersion,
		ID:               bundle.ID,
		ManifestSHA256:   bundle.ManifestSHA256,
		BaseRevision:     bundle.BaseRevision,
		HeadRevision:     bundle.HeadRevision,
		AppliedLimits:    copyLimits(bundle.AppliedLimits),
		FileCount:        bundle.FileCount,
		DeclarationCount: bundle.DeclarationCount,
		EvidenceCount:    bundle.EvidenceCount,
		ApproximateBytes: bundle.ApproximateBytes,
		Files:            make([]EvidenceFile, len(bundle.Files)),
		Items:            make([]EvidenceItem, len(bundle.Items)),
		Truncation: EvidenceTruncation{
			Truncated:           bundle.Truncation.Truncated,
			OmittedFiles:        bundle.Truncation.OmittedFiles,
			OmittedDeclarations: bundle.Truncation.OmittedDeclarations,
			OmittedExcerptBytes: bundle.Truncation.OmittedExcerptBytes,
		},
	}
	for index, file := range bundle.Files {
		runtimeBundle.Files[index] = EvidenceFile{
			Path:               file.Path,
			Status:             string(file.Status),
			ChangedLines:       copyRanges(file.ChangedLines),
			EvidenceReferences: append([]string(nil), file.EvidenceReferences...),
			Omissions:          copyOmissions(file.Omissions),
		}
	}
	for index, item := range bundle.Items {
		runtimeBundle.Items[index] = EvidenceItem{
			Reference:       item.Reference,
			Kind:            string(item.Kind),
			Path:            item.Path,
			DeclarationKind: string(item.DeclarationKind),
			Identity:        item.Identity,
			StartLine:       item.StartLine,
			EndLine:         item.EndLine,
			ChangedLines:    copyRanges(item.ChangedLines),
			Content:         item.Content,
			ContentBytes:    item.ContentBytes,
			ContentSHA256:   item.ContentSHA256,
			Truncated:       item.Truncated,
		}
	}
	return Input{SchemaVersion: InputSchemaVersion, EvidenceBundle: runtimeBundle}, nil
}

type QuestionDisposition string

const (
	DispositionQuestions            QuestionDisposition = "questions"
	DispositionInsufficientEvidence QuestionDisposition = "insufficient_evidence"
)

type QuestionKind string

const (
	QuestionKindCodeSpecific QuestionKind = "code_specific"
	QuestionKindGoBackend    QuestionKind = "go_backend"
)

type QuestionSet struct {
	SchemaVersion int                 `json:"schema_version"`
	Disposition   QuestionDisposition `json:"disposition"`
	Questions     []Question          `json:"questions"`
}

type Question struct {
	ID                 string       `json:"id"`
	Kind               QuestionKind `json:"kind"`
	Text               string       `json:"text"`
	EvidenceReferences []string     `json:"evidence_references"`
}

// ParseQuestionSet accepts exactly one strict JSON object and validates every
// evidence reference against the supplied bundle reference set.
func ParseQuestionSet(content []byte, allowedReferences []string) (QuestionSet, error) {
	allowed, err := referenceSet(allowedReferences)
	if err != nil {
		return QuestionSet{}, invalidInput(err)
	}
	if len(content) == 0 {
		return QuestionSet{}, invalidOutput("question-set output is empty")
	}
	if len(content) > MaxQuestionSetBytes {
		return QuestionSet{}, invalidOutput("question-set output exceeds %d bytes", MaxQuestionSetBytes)
	}
	if !utf8.Valid(content) {
		return QuestionSet{}, invalidOutput("question-set output is not valid UTF-8")
	}
	if err := rejectDuplicateJSONKeys(content); err != nil {
		return QuestionSet{}, invalidOutput("question-set output is not strict JSON")
	}
	if err := validateQuestionSetObjectKeys(content); err != nil {
		return QuestionSet{}, invalidOutput("question-set output does not use the exact schema fields")
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var result QuestionSet
	if err := decoder.Decode(&result); err != nil {
		return QuestionSet{}, invalidOutput("question-set output is not valid")
	}
	if err := requireJSONEOF(decoder); err != nil {
		return QuestionSet{}, invalidOutput("question-set output contains trailing content")
	}
	if result.SchemaVersion != QuestionSetSchemaVersion {
		return QuestionSet{}, invalidOutput("unsupported question-set schema version")
	}

	switch result.Disposition {
	case DispositionQuestions:
		if len(result.Questions) != 3 {
			return QuestionSet{}, invalidOutput("questions disposition requires exactly three questions")
		}
		expectedKinds := []QuestionKind{QuestionKindCodeSpecific, QuestionKindCodeSpecific, QuestionKindGoBackend}
		for index := range result.Questions {
			question := result.Questions[index]
			expectedID := fmt.Sprintf("Q%d", index+1)
			if question.ID != expectedID || question.Kind != expectedKinds[index] {
				return QuestionSet{}, invalidOutput("question %d has an invalid id or kind", index+1)
			}
			if err := validateQuestionText(question.Text); err != nil {
				return QuestionSet{}, invalidOutput("question %d: %v", index+1, err)
			}
			if question.EvidenceReferences == nil {
				return QuestionSet{}, invalidOutput("question %d requires an explicit evidence-references array", index+1)
			}
			seen := make(map[string]struct{}, len(question.EvidenceReferences))
			for _, reference := range question.EvidenceReferences {
				if _, duplicate := seen[reference]; duplicate {
					return QuestionSet{}, invalidOutput("question %d repeats an evidence reference", index+1)
				}
				seen[reference] = struct{}{}
				if _, exists := allowed[reference]; !exists {
					return QuestionSet{}, invalidOutput("question %d uses an unknown evidence reference", index+1)
				}
			}
			if question.Kind == QuestionKindCodeSpecific && len(question.EvidenceReferences) == 0 {
				return QuestionSet{}, invalidOutput("question %d requires evidence references", index+1)
			}
		}
	case DispositionInsufficientEvidence:
		if result.Questions == nil || len(result.Questions) != 0 {
			return QuestionSet{}, invalidOutput("insufficient-evidence result requires an explicit empty questions array")
		}
	default:
		return QuestionSet{}, invalidOutput("question-set disposition is invalid")
	}
	return result, nil
}

func validateBundle(bundle evidence.Bundle) error {
	if bundle.FormatVersion != evidence.BundleFormatVersion {
		return errors.New("unsupported evidence bundle format")
	}
	if !validLowerSHA256(bundle.ManifestSHA256) || bundle.ID != "eb1-"+bundle.ManifestSHA256 {
		return errors.New("evidence bundle identity is invalid")
	}
	if strings.TrimSpace(bundle.BaseRevision) == "" || !utf8.ValidString(bundle.BaseRevision) ||
		strings.TrimSpace(bundle.HeadRevision) == "" || !utf8.ValidString(bundle.HeadRevision) {
		return errors.New("evidence bundle revisions are invalid")
	}
	limits := bundle.AppliedLimits
	if limits.MaxFiles <= 0 || limits.MaxDeclarations <= 0 || limits.MaxExcerptBytes <= 0 {
		return errors.New("evidence bundle limits must be positive")
	}
	if bundle.FileCount != len(bundle.Files) || bundle.FileCount > limits.MaxFiles {
		return errors.New("evidence bundle file count is invalid")
	}
	if bundle.EvidenceCount != len(bundle.Items) || bundle.EvidenceCount == 0 ||
		bundle.DeclarationCount < bundle.EvidenceCount || bundle.DeclarationCount > limits.MaxDeclarations {
		return errors.New("evidence bundle declaration or evidence count is invalid")
	}
	if bundle.ApproximateBytes <= 0 || bundle.ApproximateBytes > limits.MaxExcerptBytes {
		return errors.New("evidence bundle byte count is invalid")
	}
	if !validTruncation(bundle.Truncation) {
		return errors.New("evidence bundle truncation is invalid")
	}

	files := make(map[string]evidence.BundleFile, len(bundle.Files))
	referenceOwners := make(map[string]string, len(bundle.Items))
	for _, file := range bundle.Files {
		if !validPath(file.Path) {
			return fmt.Errorf("evidence file path is invalid")
		}
		if _, duplicate := files[file.Path]; duplicate {
			return errors.New("evidence file path is duplicated")
		}
		if !validStatus(file.Status) || !validRanges(file.ChangedLines, 0, 0) {
			return errors.New("evidence file metadata is invalid")
		}
		for _, omission := range file.Omissions {
			if omission.Count <= 0 || !validOmission(omission.Reason) {
				return errors.New("evidence file omission is invalid")
			}
		}
		seen := make(map[string]struct{}, len(file.EvidenceReferences))
		for _, reference := range file.EvidenceReferences {
			if reference == "" {
				return errors.New("evidence file reference is empty")
			}
			if _, duplicate := seen[reference]; duplicate {
				return errors.New("evidence file reference is duplicated")
			}
			seen[reference] = struct{}{}
			if _, duplicate := referenceOwners[reference]; duplicate {
				return errors.New("evidence reference has multiple file owners")
			}
			referenceOwners[reference] = file.Path
		}
		files[file.Path] = file
	}

	totalBytes := 0
	for index, item := range bundle.Items {
		expectedReference := fmt.Sprintf("E%03d", index+1)
		if item.Reference != expectedReference || referenceOwners[item.Reference] != item.Path {
			return errors.New("evidence item reference is invalid")
		}
		if _, exists := files[item.Path]; !exists || !validItemKind(item.Kind, item.Path) {
			return errors.New("evidence item path or kind is invalid")
		}
		if !validDeclaration(item.DeclarationKind) || strings.TrimSpace(item.Identity) == "" || !utf8.ValidString(item.Identity) {
			return errors.New("evidence item declaration is invalid")
		}
		if item.StartLine <= 0 || item.EndLine < item.StartLine ||
			!validRanges(item.ChangedLines, item.StartLine, item.EndLine) || len(item.ChangedLines) == 0 {
			return errors.New("evidence item line range is invalid")
		}
		if item.Content == "" || !utf8.ValidString(item.Content) || item.ContentBytes != len(item.Content) ||
			!validLowerSHA256(item.ContentSHA256) {
			return errors.New("evidence item content metadata is invalid")
		}
		contentHash := sha256.Sum256([]byte(item.Content))
		if item.ContentSHA256 != hex.EncodeToString(contentHash[:]) {
			return errors.New("evidence item content hash is invalid")
		}
		if item.Truncated && bundle.Truncation.OmittedExcerptBytes == 0 {
			return errors.New("evidence item truncation is not reflected in the bundle")
		}
		totalBytes += item.ContentBytes
	}
	if len(referenceOwners) != len(bundle.Items) || totalBytes != bundle.ApproximateBytes {
		return errors.New("evidence bundle references or byte count are inconsistent")
	}
	hash, err := manifestSHA256(bundle)
	if err != nil || hash != bundle.ManifestSHA256 {
		return errors.New("evidence bundle manifest hash is invalid")
	}
	return nil
}

type manifestFile struct {
	Path               string
	Status             evidence.FileStatus
	ChangedLines       []evidence.LineRange
	EvidenceReferences []string
	Omissions          []evidence.Omission
}

type manifestItem struct {
	Reference       string
	Kind            evidence.BundleItemKind
	Path            string
	DeclarationKind evidence.DeclarationKind
	Identity        string
	StartLine       int
	EndLine         int
	ChangedLines    []evidence.LineRange
	ContentBytes    int
	ContentSHA256   string
	Truncated       bool
}

func manifestSHA256(bundle evidence.Bundle) (string, error) {
	files := make([]manifestFile, len(bundle.Files))
	for index, file := range bundle.Files {
		files[index] = manifestFile{
			Path:               file.Path,
			Status:             file.Status,
			ChangedLines:       file.ChangedLines,
			EvidenceReferences: file.EvidenceReferences,
			Omissions:          file.Omissions,
		}
	}
	items := make([]manifestItem, len(bundle.Items))
	for index, item := range bundle.Items {
		items[index] = manifestItem{
			Reference:       item.Reference,
			Kind:            item.Kind,
			Path:            item.Path,
			DeclarationKind: item.DeclarationKind,
			Identity:        item.Identity,
			StartLine:       item.StartLine,
			EndLine:         item.EndLine,
			ChangedLines:    item.ChangedLines,
			ContentBytes:    item.ContentBytes,
			ContentSHA256:   item.ContentSHA256,
			Truncated:       item.Truncated,
		}
	}
	manifest := struct {
		FormatVersion    int
		BaseRevision     string
		HeadRevision     string
		AppliedLimits    evidence.Limits
		FileCount        int
		DeclarationCount int
		EvidenceCount    int
		ApproximateBytes int
		Files            []manifestFile
		Items            []manifestItem
		Truncation       evidence.Truncation
	}{
		FormatVersion:    bundle.FormatVersion,
		BaseRevision:     bundle.BaseRevision,
		HeadRevision:     bundle.HeadRevision,
		AppliedLimits:    bundle.AppliedLimits,
		FileCount:        bundle.FileCount,
		DeclarationCount: bundle.DeclarationCount,
		EvidenceCount:    bundle.EvidenceCount,
		ApproximateBytes: bundle.ApproximateBytes,
		Files:            files,
		Items:            items,
		Truncation:       bundle.Truncation,
	}
	encoded, err := json.Marshal(manifest)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(encoded)
	return hex.EncodeToString(sum[:]), nil
}

func rejectDuplicateJSONKeys(content []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(content))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delim, composite := token.(json.Delim)
		if !composite {
			return nil
		}
		switch delim {
		case '{':
			seen := make(map[string]struct{})
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return errors.New("object key is not a string")
				}
				if _, duplicate := seen[key]; duplicate {
					return fmt.Errorf("duplicate object key %q", key)
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return errors.New("object is not closed")
			}
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return errors.New("array is not closed")
			}
		default:
			return errors.New("unexpected JSON delimiter")
		}
		return nil
	}
	if err := walk(); err != nil {
		return err
	}
	return requireJSONEOF(decoder)
}

func validateQuestionSetObjectKeys(content []byte) error {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil {
		return err
	}
	if !hasExactKeys(object, "schema_version", "disposition", "questions") {
		return errors.New("top-level fields are invalid")
	}

	var questions []json.RawMessage
	if err := json.Unmarshal(object["questions"], &questions); err != nil {
		return err
	}
	for _, rawQuestion := range questions {
		var question map[string]json.RawMessage
		if err := json.Unmarshal(rawQuestion, &question); err != nil {
			return err
		}
		if !hasExactKeys(question, "id", "kind", "text", "evidence_references") {
			return errors.New("question fields are invalid")
		}
	}
	return nil
}

func hasExactKeys(values map[string]json.RawMessage, expected ...string) bool {
	if len(values) != len(expected) {
		return false
	}
	for _, key := range expected {
		if _, exists := values[key]; !exists {
			return false
		}
	}
	return true
}

func requireJSONEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("additional JSON value")
	}
	return err
}

func validateQuestionText(value string) error {
	if strings.TrimSpace(value) == "" || !utf8.ValidString(value) {
		return errors.New("text must be non-empty valid UTF-8")
	}
	if len(value) > MaxQuestionTextBytes {
		return fmt.Errorf("text exceeds %d bytes", MaxQuestionTextBytes)
	}
	for _, character := range value {
		if unicode.IsControl(character) {
			return errors.New("text contains a control character")
		}
	}
	return nil
}

func referenceSet(references []string) (map[string]struct{}, error) {
	if len(references) == 0 {
		return nil, errors.New("allowed evidence references are required")
	}
	result := make(map[string]struct{}, len(references))
	for _, reference := range references {
		if strings.TrimSpace(reference) == "" || !utf8.ValidString(reference) {
			return nil, errors.New("allowed evidence reference is invalid")
		}
		if _, duplicate := result[reference]; duplicate {
			return nil, errors.New("allowed evidence reference is duplicated")
		}
		result[reference] = struct{}{}
	}
	return result, nil
}

func validPath(value string) bool {
	return value != "" && utf8.ValidString(value) && !path.IsAbs(value) &&
		!strings.Contains(value, `\`) && path.Clean(value) == value &&
		value != "." && value != ".." && !strings.HasPrefix(value, "../")
}

func validLowerSHA256(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

func validRanges(ranges []evidence.LineRange, minimum, maximum int) bool {
	for index, lineRange := range ranges {
		if lineRange.Start <= 0 || lineRange.End < lineRange.Start {
			return false
		}
		if index > 0 && lineRange.Start <= ranges[index-1].End {
			return false
		}
		if minimum > 0 && (lineRange.Start < minimum || lineRange.End > maximum) {
			return false
		}
	}
	return true
}

func validTruncation(value evidence.Truncation) bool {
	if value.OmittedFiles < 0 || value.OmittedDeclarations < 0 || value.OmittedExcerptBytes < 0 {
		return false
	}
	hasOmissions := value.OmittedFiles > 0 || value.OmittedDeclarations > 0 || value.OmittedExcerptBytes > 0
	return value.Truncated == hasOmissions
}

func validStatus(value evidence.FileStatus) bool {
	return value == evidence.FileAdded || value == evidence.FileModified || value == evidence.FileDeleted
}

func validOmission(value evidence.OmissionReason) bool {
	return value == evidence.OmissionDeletedFile || value == evidence.OmissionDeletedOnlyHunk || value == evidence.OmissionOutsideDeclaration
}

func validDeclaration(value evidence.DeclarationKind) bool {
	switch value {
	case evidence.DeclarationFunction, evidence.DeclarationMethod, evidence.DeclarationType,
		evidence.DeclarationInterface, evidence.DeclarationVariable, evidence.DeclarationConstant:
		return true
	default:
		return false
	}
}

func validItemKind(value evidence.BundleItemKind, itemPath string) bool {
	switch value {
	case evidence.BundleItemTest:
		return strings.HasSuffix(itemPath, "_test.go")
	case evidence.BundleItemCode:
		return !strings.HasSuffix(itemPath, "_test.go")
	default:
		return false
	}
}

func copyLimits(value evidence.Limits) EvidenceLimits {
	return EvidenceLimits{
		MaxFiles:        value.MaxFiles,
		MaxDeclarations: value.MaxDeclarations,
		MaxExcerptBytes: value.MaxExcerptBytes,
	}
}

func copyRanges(values []evidence.LineRange) []EvidenceLineRange {
	result := make([]EvidenceLineRange, len(values))
	for index, value := range values {
		result[index] = EvidenceLineRange{Start: value.Start, End: value.End}
	}
	return result
}

func copyOmissions(values []evidence.Omission) []EvidenceOmission {
	result := make([]EvidenceOmission, len(values))
	for index, value := range values {
		result[index] = EvidenceOmission{Reason: string(value.Reason), Count: value.Count}
	}
	return result
}

func invalidInput(err error) error {
	return &ContractError{Code: ContractErrorInvalidInput, Err: err}
}

func invalidOutput(message string, arguments ...any) error {
	return &ContractError{Code: ContractErrorInvalidOutput, Err: fmt.Errorf(message, arguments...)}
}
