package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"

	"github.com/reeezark/pi-learnloop/agent/prompts"
	"github.com/reeezark/pi-learnloop/internal/evaluator"
	"github.com/reeezark/pi-learnloop/internal/evidence"
	"github.com/reeezark/pi-learnloop/internal/history"
)

func evaluatorInputForContinuation(retained continuationValue) (evaluator.Input, prompts.Metadata, prompts.Metadata, error) {
	switch retained.contract {
	case evidenceContractV1:
		bundle, err := evidence.BuildBundle(retained.result)
		if err != nil {
			return evaluator.Input{}, prompts.Metadata{}, prompts.Metadata{}, err
		}
		input, err := evaluator.NewInput(bundle)
		return input, prompts.EvaluatorQuestionGenerationV1Metadata(), prompts.EvaluatorAnswerAssessmentV1Metadata(), err
	case evidenceContractV2:
		bundle, err := evidence.BuildBundleV2(retained.result)
		if err != nil {
			return evaluator.Input{}, prompts.Metadata{}, prompts.Metadata{}, err
		}
		input, err := evaluator.NewInputV2(bundle)
		return input, prompts.EvaluatorQuestionGenerationV2Metadata(), prompts.EvaluatorAnswerAssessmentV2Metadata(), err
	default:
		return evaluator.Input{}, prompts.Metadata{}, prompts.Metadata{}, errors.New("unsupported continuation evidence contract")
	}
}

func handleGoContextEvidencePreview(response http.ResponseWriter, request *http.Request, token string, services serverServices) {
	if !prepareGoContextPreviewRequest(response, request, token) {
		return
	}
	content, ok := readGoContextPreviewBody(response, request)
	if !ok {
		return
	}
	var payload previewRequest
	if err := decodeStrictJSON(content, &payload); err != nil || !hasExactPreviewFields(content) {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	selection, ok := validatePreviewRequest(payload)
	if !ok {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	serveGoContextEvidencePreview(response, request, payload, selection, "", services)
}

func handlePiSessionGoContextEvidencePreview(response http.ResponseWriter, request *http.Request, token string, services serverServices) {
	if !prepareGoContextPreviewRequest(response, request, token) {
		return
	}
	content, ok := readGoContextPreviewBody(response, request)
	if !ok {
		return
	}
	var payload piSessionPreviewRequest
	if err := decodeStrictJSON(content, &payload); err != nil || !hasExactPiSessionPreviewFields(content) || !history.ValidPiSessionID(payload.PiSessionID) {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	preview := previewRequest{Repository: payload.Repository, Selection: payload.Selection}
	selection, ok := validatePreviewRequest(preview)
	if !ok {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	serveGoContextEvidencePreview(response, request, preview, selection, payload.PiSessionID, services)
}

func prepareGoContextPreviewRequest(response http.ResponseWriter, request *http.Request, token string) bool {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		writeError(response, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return false
	}
	if !authorized(request, token) {
		response.Header().Set("WWW-Authenticate", "PiLearnLoop")
		writeError(response, http.StatusUnauthorized, "unauthorized", "authentication required")
		return false
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media_type", "content type must be application/json")
		return false
	}
	return true
}

func readGoContextPreviewBody(response http.ResponseWriter, request *http.Request) ([]byte, bool) {
	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBytes)
	content, err := io.ReadAll(request.Body)
	if err == nil {
		return content, true
	}
	var maxBytesError *http.MaxBytesError
	if errors.As(err, &maxBytesError) {
		writeError(response, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
	} else {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
	}
	return nil, false
}

func serveGoContextEvidencePreview(response http.ResponseWriter, request *http.Request, payload previewRequest, selection evidence.Selection, piSessionID string, services serverServices) {
	previewCtx, cancel := context.WithTimeout(request.Context(), evidenceTimeout)
	defer cancel()
	result, err := evidence.Preview(previewCtx, evidence.Request{
		Repository: payload.Repository,
		Selection:  selection,
		Limits: evidence.Limits{
			MaxFiles: maxFiles, MaxDeclarations: maxDeclarations, MaxExcerptBytes: maxExcerptBytes,
		},
		Context: evidence.ContextGo,
	})
	if err != nil {
		if errors.Is(previewCtx.Err(), context.DeadlineExceeded) {
			writeError(response, http.StatusGatewayTimeout, "deadline_exceeded", "evidence analysis timed out")
			return
		}
		writePreviewError(response, err)
		return
	}
	if result.GoContext == nil {
		writeError(response, http.StatusInternalServerError, "internal_error", "internal error")
		return
	}

	continuation := continuationDescriptor{Available: false, Reason: "evaluator_unavailable"}
	if services.questionEvaluator != nil {
		if piSessionID == "" {
			continuation, err = services.continuations.retainGoContext(result)
		} else {
			continuation, err = services.continuations.retainGoContextWithPiSession(result, piSessionID)
		}
		if err != nil {
			writeError(response, http.StatusInternalServerError, "internal_error", "internal error")
			return
		}
	}
	mapped := mapEvidenceResult(result)
	writeJSON(response, http.StatusOK, map[string]any{
		"protocol_version": protocolVersion,
		"applied_limits": map[string]int{
			"max_files": maxFiles, "max_declarations": maxDeclarations, "max_excerpt_bytes": maxExcerptBytes,
		},
		"preview": map[string]any{
			"repository_root": mapped.RepositoryRoot,
			"base_revision":   mapped.BaseRevision,
			"head_revision":   mapped.HeadRevision,
			"files":           mapped.Files,
			"go_context":      mapGoContext(*result.GoContext),
			"truncation":      mapped.Truncation,
		},
		"continuation": continuation,
	})
}

func hasExactPreviewFields(content []byte) bool {
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil || !hasExactKeys(object, "repository", "selection") {
		return false
	}
	var selection map[string]json.RawMessage
	if err := json.Unmarshal(object["selection"], &selection); err != nil {
		return false
	}
	var kind string
	if err := json.Unmarshal(selection["kind"], &kind); err != nil {
		return false
	}
	if kind == "working_tree" {
		return hasExactKeys(selection, "kind", "base")
	}
	return kind == "commit_range" && hasExactKeys(selection, "kind", "base", "head")
}

func mapGoContext(value evidence.GoContext) map[string]any {
	items := make([]map[string]any, len(value.Items))
	for index, item := range value.Items {
		hash := sha256.Sum256([]byte(item.Content))
		items[index] = map[string]any{
			"reference": item.ID, "kind": item.Kind, "path": item.Path, "package_path": item.PackagePath,
			"declaration_kind": item.DeclarationKind, "identity": item.Identity,
			"start_line": item.StartLine, "end_line": item.EndLine,
			"content": item.Content, "content_bytes": item.ContentBytes,
			"content_sha256": hex.EncodeToString(hash[:]), "truncated": item.Truncated,
		}
	}
	relations := make([]map[string]any, len(value.Relations))
	for index, relation := range value.Relations {
		relations[index] = map[string]any{
			"from": relation.From, "to": relation.To, "kind": relation.Kind, "strength": relation.Strength,
		}
	}
	omissions := make([]map[string]any, len(value.Omissions))
	for index, omission := range value.Omissions {
		omissions[index] = map[string]any{"reason": omission.Reason, "count": omission.Count}
	}
	return map[string]any{
		"status": value.Status,
		"build":  mapGoBuild(value.Build),
		"applied_limits": map[string]any{
			"max_changed_files":         value.AppliedLimits.MaxChangedFiles,
			"max_module_roots":          value.AppliedLimits.MaxModuleRoots,
			"max_packages":              value.AppliedLimits.MaxPackages,
			"max_files_per_package":     value.AppliedLimits.MaxFilesPerPackage,
			"max_files":                 value.AppliedLimits.MaxFiles,
			"max_directory_entries":     value.AppliedLimits.MaxDirectoryEntries,
			"max_source_bytes_per_file": value.AppliedLimits.MaxSourceBytesPerFile,
			"max_source_bytes":          value.AppliedLimits.MaxSourceBytes,
			"max_direct_import_edges":   value.AppliedLimits.MaxDirectImportEdges,
			"analysis_timeout_millis":   value.AppliedLimits.AnalysisTimeout.Milliseconds(),
			"max_output_files":          value.AppliedLimits.MaxOutputFiles,
			"max_output_items":          value.AppliedLimits.MaxOutputItems,
			"max_relations":             value.AppliedLimits.MaxRelations,
			"max_excerpt_bytes":         value.AppliedLimits.MaxExcerptBytes,
			"max_output_bytes":          value.AppliedLimits.MaxOutputBytes,
			"max_evaluator_input_bytes": value.AppliedLimits.MaxEvaluatorInputBytes,
		},
		"analyzed_package_count": value.AnalyzedPackageCount,
		"analyzed_file_count":    value.AnalyzedFileCount,
		"analyzed_source_bytes":  value.AnalyzedSourceBytes,
		"direct_import_edges":    value.DirectImportEdges,
		"item_count":             len(items), "relation_count": len(relations), "approximate_bytes": value.ApproximateBytes,
		"items": items, "relations": relations, "omissions": omissions,
		"truncation": map[string]any{
			"truncated": value.Truncation.Truncated, "omitted_files": value.Truncation.OmittedFiles,
			"omitted_items": value.Truncation.OmittedItems, "omitted_relations": value.Truncation.OmittedRelations,
			"omitted_bytes": value.Truncation.OmittedBytes,
		},
	}
}

func mapGoBuild(value evidence.GoBuildConfiguration) map[string]any {
	modules := make([]map[string]any, len(value.Modules))
	for index, module := range value.Modules {
		modules[index] = map[string]any{"path": module.Path, "directory": module.Directory, "go_version": module.GoVersion, "toolchain": module.Toolchain}
	}
	workspaces := make([]map[string]any, len(value.Workspaces))
	for index, workspace := range value.Workspaces {
		workspaces[index] = map[string]any{"directory": workspace.Directory, "go_version": workspace.GoVersion, "toolchain": workspace.Toolchain}
	}
	replacements := make([]map[string]any, len(value.Replacements))
	for index, replacement := range value.Replacements {
		replacements[index] = map[string]any{"module_path": replacement.ModulePath, "directory": replacement.Directory, "repository_local": replacement.RepositoryLocal}
	}
	return map[string]any{
		"goos": value.GOOS, "goarch": value.GOARCH, "cgo_enabled": value.CGOEnabled,
		"build_tags": value.BuildTags, "tool_tags": value.ToolTags, "release_tags": value.ReleaseTags,
		"toolchain_version": value.ToolchainVersion, "test_variant": value.TestVariant,
		"modules": modules, "workspaces": workspaces, "replacements": replacements,
	}
}
