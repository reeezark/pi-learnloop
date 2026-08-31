package daemon

import (
	"bytes"
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/reeezark/pi-learnloop/internal/evidence"
)

const (
	maxRequestBytes = 16 * 1024
	evidenceTimeout = 30 * time.Second
	maxFiles        = 20
	maxDeclarations = 100
	maxExcerptBytes = 128 * 1024
)

type previewRequest struct {
	Repository string            `json:"repository"`
	Selection  *selectionRequest `json:"selection"`
}

type selectionRequest struct {
	Kind string  `json:"kind"`
	Base *string `json:"base"`
	Head *string `json:"head"`
}

type lineRangeResponse struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type omissionResponse struct {
	Reason evidence.OmissionReason `json:"reason"`
	Count  int                     `json:"count"`
}

type declarationResponse struct {
	Kind             evidence.DeclarationKind `json:"kind"`
	Name             string                   `json:"name"`
	Receiver         string                   `json:"receiver"`
	Identity         string                   `json:"identity"`
	StartLine        int                      `json:"start_line"`
	EndLine          int                      `json:"end_line"`
	ChangedLines     []lineRangeResponse      `json:"changed_lines"`
	Excerpt          string                   `json:"excerpt"`
	ExcerptTruncated bool                     `json:"excerpt_truncated"`
}

type fileResponse struct {
	Path         string                `json:"path"`
	Status       evidence.FileStatus   `json:"status"`
	ChangedLines []lineRangeResponse   `json:"changed_lines"`
	Declarations []declarationResponse `json:"declarations"`
	Omissions    []omissionResponse    `json:"omissions"`
}

type truncationResponse struct {
	Truncated           bool `json:"truncated"`
	OmittedFiles        int  `json:"omitted_files"`
	OmittedDeclarations int  `json:"omitted_declarations"`
	OmittedExcerptBytes int  `json:"omitted_excerpt_bytes"`
}

type evidenceResponse struct {
	RepositoryRoot string             `json:"repository_root"`
	BaseRevision   string             `json:"base_revision"`
	HeadRevision   string             `json:"head_revision"`
	Files          []fileResponse     `json:"files"`
	Truncation     truncationResponse `json:"truncation"`
}

func newHandler(instanceID, authority, token string) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		response.Header().Set("Cache-Control", "no-store")
		if !validPeer(request.RemoteAddr) || request.Host != authority || hasNonEmptyHeader(request.Header, "Origin") {
			writeError(response, http.StatusForbidden, "forbidden", "request is not allowed")
			return
		}

		switch request.URL.Path {
		case "/v1/status":
			handleStatus(response, request, instanceID)
		case "/v1/evidence-previews":
			handleEvidencePreview(response, request, token)
		default:
			writeError(response, http.StatusNotFound, "not_found", "route not found")
		}
	})
}

func hasNonEmptyHeader(header http.Header, name string) bool {
	for _, value := range header.Values(name) {
		if value != "" {
			return true
		}
	}
	return false
}

func validPeer(remoteAddress string) bool {
	host, _, err := net.SplitHostPort(remoteAddress)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.To4() != nil && ip.IsLoopback()
}

func handleStatus(response http.ResponseWriter, request *http.Request, instanceID string) {
	if request.Method != http.MethodGet {
		response.Header().Set("Allow", http.MethodGet)
		writeError(response, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"protocol_version": protocolVersion,
		"instance_id":      instanceID,
		"status":           "ready",
	})
}

func handleEvidencePreview(response http.ResponseWriter, request *http.Request, token string) {
	if request.Method != http.MethodPost {
		response.Header().Set("Allow", http.MethodPost)
		writeError(response, http.StatusMethodNotAllowed, "method_not_allowed", "method not allowed")
		return
	}
	if !authorized(request, token) {
		response.Header().Set("WWW-Authenticate", "PiLearnLoop")
		writeError(response, http.StatusUnauthorized, "unauthorized", "authentication required")
		return
	}
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/json" {
		writeError(response, http.StatusUnsupportedMediaType, "unsupported_media_type", "content type must be application/json")
		return
	}

	request.Body = http.MaxBytesReader(response, request.Body, maxRequestBytes)
	content, err := io.ReadAll(request.Body)
	if err != nil {
		var maxBytesError *http.MaxBytesError
		if errors.As(err, &maxBytesError) {
			writeError(response, http.StatusRequestEntityTooLarge, "request_too_large", "request body is too large")
			return
		}
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	var payload previewRequest
	if err := decodeStrictJSON(content, &payload); err != nil {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}

	selection, ok := validatePreviewRequest(payload)
	if !ok {
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
		return
	}
	previewCtx, cancel := context.WithTimeout(request.Context(), evidenceTimeout)
	defer cancel()
	result, err := evidence.Preview(previewCtx, evidence.Request{
		Repository: payload.Repository,
		Selection:  selection,
		Limits: evidence.Limits{
			MaxFiles:        maxFiles,
			MaxDeclarations: maxDeclarations,
			MaxExcerptBytes: maxExcerptBytes,
		},
	})
	if err != nil {
		if errors.Is(previewCtx.Err(), context.DeadlineExceeded) {
			writeError(response, http.StatusGatewayTimeout, "deadline_exceeded", "evidence analysis timed out")
			return
		}
		writePreviewError(response, err)
		return
	}
	writeJSON(response, http.StatusOK, map[string]any{
		"protocol_version": protocolVersion,
		"applied_limits": map[string]int{
			"max_files":         maxFiles,
			"max_declarations":  maxDeclarations,
			"max_excerpt_bytes": maxExcerptBytes,
		},
		"preview": mapEvidenceResult(result),
	})
}

func decodeStrictJSON(content []byte, destination any) error {
	if !utf8.Valid(content) {
		return errors.New("JSON is not valid UTF-8")
	}
	structureDecoder := json.NewDecoder(bytes.NewReader(content))
	if err := readUniqueJSONValue(structureDecoder); err != nil {
		return err
	}
	if _, err := structureDecoder.Token(); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains a trailing value")
		}
		return err
	}

	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("JSON contains a trailing value")
		}
		return err
	}
	return nil
}

func readUniqueJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		seen := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("JSON object key is not a string")
			}
			if _, duplicate := seen[key]; duplicate {
				return fmt.Errorf("duplicate JSON object key %q", key)
			}
			seen[key] = struct{}{}
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim('}') {
			return errors.New("JSON object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := readUniqueJSONValue(decoder); err != nil {
				return err
			}
		}
		closing, err := decoder.Token()
		if err != nil {
			return err
		}
		if closing != json.Delim(']') {
			return errors.New("JSON array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}

func authorized(request *http.Request, token string) bool {
	values := request.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "PiLearnLoop ") {
		return false
	}
	provided := strings.TrimPrefix(values[0], "PiLearnLoop ")
	return len(provided) == len(token) && subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func validatePreviewRequest(request previewRequest) (evidence.Selection, bool) {
	if request.Repository == "" || len(request.Repository) > 4096 || !filepath.IsAbs(request.Repository) || request.Selection == nil || request.Selection.Base == nil {
		return evidence.Selection{}, false
	}
	base := *request.Selection.Base
	if base == "" || len(base) > 256 {
		return evidence.Selection{}, false
	}
	switch request.Selection.Kind {
	case "commit_range":
		if request.Selection.Head == nil || *request.Selection.Head == "" || len(*request.Selection.Head) > 256 {
			return evidence.Selection{}, false
		}
		return evidence.CommitRange(base, *request.Selection.Head), true
	case "working_tree":
		if request.Selection.Head != nil {
			return evidence.Selection{}, false
		}
		return evidence.WorkingTree(base), true
	default:
		return evidence.Selection{}, false
	}
}

func writePreviewError(response http.ResponseWriter, err error) {
	if errors.Is(err, context.DeadlineExceeded) {
		writeError(response, http.StatusGatewayTimeout, "deadline_exceeded", "evidence analysis timed out")
		return
	}
	switch evidence.ErrorCodeOf(err) {
	case evidence.ErrorInvalidRequest:
		writeError(response, http.StatusBadRequest, "invalid_request", "request body is invalid")
	case evidence.ErrorNotRepository:
		writeError(response, http.StatusUnprocessableEntity, "invalid_repository", "repository is not supported")
	case evidence.ErrorInvalidRevision:
		writeError(response, http.StatusUnprocessableEntity, "invalid_revision", "revision cannot be resolved")
	case evidence.ErrorOutsideRepository:
		writeError(response, http.StatusForbidden, "forbidden", "repository evidence is not allowed")
	case evidence.ErrorReadSource:
		writeError(response, http.StatusConflict, "source_unavailable", "source changed or became unavailable")
	case evidence.ErrorParseSource:
		writeError(response, http.StatusUnprocessableEntity, "invalid_source", "changed Go source cannot be parsed")
	case evidence.ErrorGit:
		writeError(response, http.StatusInternalServerError, "analysis_failed", "evidence analysis failed")
	default:
		writeError(response, http.StatusInternalServerError, "internal_error", "internal error")
	}
}

func mapEvidenceResult(result evidence.Result) evidenceResponse {
	files := make([]fileResponse, len(result.Files))
	for fileIndex, file := range result.Files {
		declarations := make([]declarationResponse, len(file.Declarations))
		for declarationIndex, declaration := range file.Declarations {
			declarations[declarationIndex] = declarationResponse{
				Kind:             declaration.Kind,
				Name:             declaration.Name,
				Receiver:         declaration.Receiver,
				Identity:         declaration.Identity,
				StartLine:        declaration.StartLine,
				EndLine:          declaration.EndLine,
				ChangedLines:     mapLineRanges(declaration.ChangedLines),
				Excerpt:          declaration.Excerpt,
				ExcerptTruncated: declaration.ExcerptTruncated,
			}
		}
		omissions := make([]omissionResponse, len(file.Omissions))
		for omissionIndex, omission := range file.Omissions {
			omissions[omissionIndex] = omissionResponse{Reason: omission.Reason, Count: omission.Count}
		}
		files[fileIndex] = fileResponse{
			Path:         filepath.ToSlash(file.Path),
			Status:       file.Status,
			ChangedLines: mapLineRanges(file.ChangedLines),
			Declarations: declarations,
			Omissions:    omissions,
		}
	}
	return evidenceResponse{
		RepositoryRoot: result.RepositoryRoot,
		BaseRevision:   result.BaseRevision,
		HeadRevision:   result.HeadRevision,
		Files:          files,
		Truncation: truncationResponse{
			Truncated:           result.Truncation.Truncated,
			OmittedFiles:        result.Truncation.OmittedFiles,
			OmittedDeclarations: result.Truncation.OmittedDeclarations,
			OmittedExcerptBytes: result.Truncation.OmittedExcerptBytes,
		},
	}
}

func mapLineRanges(ranges []evidence.LineRange) []lineRangeResponse {
	mapped := make([]lineRangeResponse, len(ranges))
	for index, lineRange := range ranges {
		mapped[index] = lineRangeResponse{Start: lineRange.Start, End: lineRange.End}
	}
	return mapped
}

func writeError(response http.ResponseWriter, status int, code, message string) {
	writeJSON(response, status, map[string]any{
		"protocol_version": protocolVersion,
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

func writeJSON(response http.ResponseWriter, status int, value any) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(value)
}
