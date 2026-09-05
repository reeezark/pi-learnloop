package evaluator

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	piVersionPreflightTimeout = 2 * time.Second
	runtimePreflightTimeout   = 10 * time.Second
	evaluatorProcessTimeout   = 120 * time.Second
	maxWorkerRequestBytes     = 3 * 1024 * 1024
	maxRPCStdoutBytes         = 2 * 1024 * 1024
	maxRPCStderrBytes         = 64 * 1024
	maxAssistantTextBytes     = 64 * 1024
	maxVersionOutputBytes     = 4 * 1024
	maxPiPackageJSONBytes     = 64 * 1024
	maxPackageTraversal       = 6
	workerSchemaVersion       = 1
)

var errRPCFailure = errors.New("Pi evaluator runtime failed")

//go:embed pi_model_worker.mjs
var piModelWorkerSource string

type piModelRuntime struct {
	nodeExecutable   string
	piExecutable     string
	sdkEntry         string
	settingsEntry    string
	httpEntry        string
	attributionEntry string
}

// PiRPCEvaluator retains the established daemon-facing type while using one
// fresh, isolated Pi ModelRuntime child for each evaluation.
type PiRPCEvaluator struct {
	runtime        piModelRuntime
	systemPrompt   string
	systemPromptV2 string
}

// PiRPCAssessmentEvaluator keeps assessment generation behind its existing
// narrow interface and shares only private process-isolation mechanics.
type PiRPCAssessmentEvaluator struct {
	runtime        piModelRuntime
	systemPrompt   string
	systemPromptV2 string
}

func NewPiRPCEvaluator(ctx context.Context, systemPrompt string) (*PiRPCEvaluator, error) {
	runtime, err := resolvePiModelRuntime(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &PiRPCEvaluator{runtime: runtime, systemPrompt: systemPrompt}, nil
}

func NewPiRPCAssessmentEvaluator(ctx context.Context, systemPrompt string) (*PiRPCAssessmentEvaluator, error) {
	runtime, err := resolvePiModelRuntime(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &PiRPCAssessmentEvaluator{runtime: runtime, systemPrompt: systemPrompt}, nil
}

func NewVersionedPiRPCEvaluator(ctx context.Context, systemPromptV1, systemPromptV2 string) (*PiRPCEvaluator, error) {
	runtime, err := resolvePiModelRuntime(ctx, systemPromptV1)
	if err != nil || validateSystemPrompt(systemPromptV2) != nil {
		return nil, errors.New("Pi evaluator is unavailable")
	}
	return &PiRPCEvaluator{runtime: runtime, systemPrompt: systemPromptV1, systemPromptV2: systemPromptV2}, nil
}

func NewVersionedPiRPCAssessmentEvaluator(ctx context.Context, systemPromptV1, systemPromptV2 string) (*PiRPCAssessmentEvaluator, error) {
	runtime, err := resolvePiModelRuntime(ctx, systemPromptV1)
	if err != nil || validateSystemPrompt(systemPromptV2) != nil {
		return nil, errors.New("Pi evaluator is unavailable")
	}
	return &PiRPCAssessmentEvaluator{runtime: runtime, systemPrompt: systemPromptV1, systemPromptV2: systemPromptV2}, nil
}

// NewVersionedPiModelEvaluators performs the shared runtime preflight once and
// returns the two established evaluator interfaces over the same frozen paths.
// Each Evaluate call still starts its own fresh worker process.
func NewVersionedPiModelEvaluators(ctx context.Context, questionV1, questionV2, assessmentV1, assessmentV2 string) (*PiRPCEvaluator, *PiRPCAssessmentEvaluator, error) {
	if validateSystemPrompt(questionV2) != nil || validateSystemPrompt(assessmentV1) != nil || validateSystemPrompt(assessmentV2) != nil {
		return nil, nil, errors.New("Pi evaluator is unavailable")
	}
	runtime, err := resolvePiModelRuntime(ctx, questionV1)
	if err != nil {
		return nil, nil, err
	}
	return &PiRPCEvaluator{runtime: runtime, systemPrompt: questionV1, systemPromptV2: questionV2},
		&PiRPCAssessmentEvaluator{runtime: runtime, systemPrompt: assessmentV1, systemPromptV2: assessmentV2}, nil
}

func resolvePiModelRuntime(ctx context.Context, systemPrompt string) (piModelRuntime, error) {
	unavailable := func() (piModelRuntime, error) {
		return piModelRuntime{}, errors.New("Pi evaluator is unavailable")
	}
	if ctx == nil || validateSystemPrompt(systemPrompt) != nil {
		return unavailable()
	}
	nodeExecutable, err := resolveExecutable("node")
	if err != nil || preflightNodeVersion(ctx, nodeExecutable) != nil {
		return unavailable()
	}
	piExecutable, err := resolveExecutable("pi")
	if err != nil || preflightPiVersion(ctx, piExecutable) != nil {
		return unavailable()
	}
	runtime, err := resolvePiPackage(piExecutable)
	if err != nil {
		return unavailable()
	}
	runtime.nodeExecutable = nodeExecutable
	runtime.piExecutable = piExecutable
	if err := preflightModelWorker(ctx, runtime); err != nil {
		return unavailable()
	}
	return runtime, nil
}

func resolveExecutable(name string) (string, error) {
	executable, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	if !filepath.IsAbs(executable) {
		executable, err = filepath.Abs(executable)
		if err != nil {
			return "", err
		}
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("executable is unavailable")
	}
	return executable, nil
}

func preflightNodeVersion(ctx context.Context, executable string) error {
	stdout, err := boundedCommand(ctx, piVersionPreflightTimeout, maxVersionOutputBytes, maxVersionOutputBytes, executable, "--version")
	if err != nil {
		return err
	}
	version := strings.TrimSpace(string(stdout))
	if !strings.HasPrefix(version, "v") {
		return errRPCFailure
	}
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	if len(parts) != 3 {
		return errRPCFailure
	}
	major, majorErr := strconv.Atoi(parts[0])
	minor, minorErr := strconv.Atoi(parts[1])
	_, patchErr := strconv.Atoi(parts[2])
	if majorErr != nil || minorErr != nil || patchErr != nil || major < 22 || (major == 22 && minor < 19) {
		return errRPCFailure
	}
	return nil
}

func preflightPiVersion(ctx context.Context, executable string) error {
	stdout, err := boundedCommand(ctx, piVersionPreflightTimeout, maxVersionOutputBytes, maxVersionOutputBytes, executable, "--version")
	if err != nil || strings.TrimSpace(string(stdout)) != SupportedPiVersion {
		return errRPCFailure
	}
	return nil
}

type piPackageManifest struct {
	Name    string          `json:"name"`
	Version string          `json:"version"`
	Main    string          `json:"main"`
	Bin     json.RawMessage `json:"bin"`
}

func resolvePiPackage(piExecutable string) (piModelRuntime, error) {
	directory := filepath.Dir(piExecutable)
	for depth := 0; depth <= maxPackageTraversal; depth++ {
		manifestPath := filepath.Join(directory, "package.json")
		content, err := readBoundedRegularFile(manifestPath, maxPiPackageJSONBytes)
		if err == nil {
			return runtimeFromManifest(directory, piExecutable, content)
		}
		parent := filepath.Dir(directory)
		if parent == directory {
			break
		}
		directory = parent
	}
	return piModelRuntime{}, errors.New("owning Pi package is unavailable")
}

func runtimeFromManifest(root, piExecutable string, content []byte) (piModelRuntime, error) {
	if rejectDuplicateJSONKeys(content) != nil {
		return piModelRuntime{}, errRPCFailure
	}
	var manifest piPackageManifest
	if json.Unmarshal(content, &manifest) != nil || manifest.Name != "@earendil-works/pi-coding-agent" ||
		manifest.Version != SupportedPiVersion || manifest.Main == "" {
		return piModelRuntime{}, errRPCFailure
	}
	var binPath string
	if json.Unmarshal(manifest.Bin, &binPath) != nil {
		var bins map[string]string
		if json.Unmarshal(manifest.Bin, &bins) != nil {
			return piModelRuntime{}, errRPCFailure
		}
		binPath = bins["pi"]
	}
	resolvedBin, err := resolvePackageFile(root, binPath, true)
	if err != nil || resolvedBin != piExecutable {
		return piModelRuntime{}, errRPCFailure
	}
	sdkEntry, err := resolvePackageFile(root, manifest.Main, false)
	if err != nil {
		return piModelRuntime{}, errRPCFailure
	}
	settingsEntry, err := resolvePackageFile(root, "dist/core/settings-manager.js", false)
	if err != nil {
		return piModelRuntime{}, errRPCFailure
	}
	httpEntry, err := resolvePackageFile(root, "dist/core/http-dispatcher.js", false)
	if err != nil {
		return piModelRuntime{}, errRPCFailure
	}
	attributionEntry, err := resolvePackageFile(root, "dist/core/provider-attribution.js", false)
	if err != nil {
		return piModelRuntime{}, errRPCFailure
	}
	return piModelRuntime{sdkEntry: sdkEntry, settingsEntry: settingsEntry, httpEntry: httpEntry, attributionEntry: attributionEntry}, nil
}

func resolvePackageFile(root, relative string, executable bool) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", errRPCFailure
	}
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	candidate, err := filepath.EvalSymlinks(filepath.Join(root, filepath.Clean(relative)))
	if err != nil {
		return "", err
	}
	relativeToRoot, err := filepath.Rel(root, candidate)
	if err != nil || relativeToRoot == ".." || strings.HasPrefix(relativeToRoot, ".."+string(os.PathSeparator)) {
		return "", errRPCFailure
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() || (executable && info.Mode().Perm()&0o111 == 0) {
		return "", errRPCFailure
	}
	return candidate, nil
}

func readBoundedRegularFile(path string, maximum int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Size() > maximum {
		return nil, errRPCFailure
	}
	content, err := os.ReadFile(path)
	if err != nil || int64(len(content)) > maximum {
		return nil, errRPCFailure
	}
	return content, nil
}

func preflightModelWorker(ctx context.Context, runtime piModelRuntime) error {
	response, err := runModelWorker(ctx, runtime, workerRequest{
		SchemaVersion:    workerSchemaVersion,
		Action:           "preflight",
		SDKEntry:         runtime.sdkEntry,
		SettingsEntry:    runtime.settingsEntry,
		HTTPEntry:        runtime.httpEntry,
		AttributionEntry: runtime.attributionEntry,
	}, runtimePreflightTimeout)
	if err != nil || response.Status != "ready" {
		return errRPCFailure
	}
	return nil
}

type workerModel struct {
	Provider      string `json:"provider"`
	ID            string `json:"id"`
	ThinkingLevel string `json:"thinking_level"`
}

type workerRequest struct {
	SchemaVersion    int          `json:"schema_version"`
	Action           string       `json:"action"`
	SDKEntry         string       `json:"sdk_entry"`
	SettingsEntry    string       `json:"settings_manager_entry"`
	HTTPEntry        string       `json:"http_dispatcher_entry"`
	AttributionEntry string       `json:"attribution_entry"`
	SystemPrompt     string       `json:"system_prompt,omitempty"`
	Message          string       `json:"message,omitempty"`
	Model            *workerModel `json:"model,omitempty"`
}

type workerResponse struct {
	SchemaVersion int    `json:"schema_version"`
	Status        string `json:"status"`
	Text          string `json:"text,omitempty"`
	Code          string `json:"code,omitempty"`
}

func runModelWorker(ctx context.Context, runtime piModelRuntime, request workerRequest, timeout time.Duration) (workerResponse, error) {
	if ctx == nil {
		return workerResponse{}, errRPCFailure
	}
	content, err := json.Marshal(request)
	if err != nil || len(content)+1 > maxWorkerRequestBytes {
		return workerResponse{}, errRPCFailure
	}
	content = append(content, '\n')
	workerSource := piModelWorkerSource + "\nawait workerMain();\n"
	processCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	command := exec.CommandContext(processCtx, runtime.nodeExecutable, "--input-type=module", "--eval", workerSource)
	command.Dir = os.TempDir()
	command.Stdin = bytes.NewReader(content)
	stdout := newBoundedCapture(maxRPCStdoutBytes)
	stderr := newBoundedCapture(maxRPCStderrBytes)
	command.Stdout = stdout
	command.Stderr = stderr
	err = command.Run()
	if processCtx.Err() != nil {
		return workerResponse{}, processCtx.Err()
	}
	if err != nil || stdout.overflow || stderr.overflow {
		return workerResponse{}, errRPCFailure
	}
	frame := stdout.Bytes()
	if len(frame) < 2 || frame[len(frame)-1] != '\n' || bytes.Contains(frame[:len(frame)-1], []byte{'\n'}) || bytes.Contains(frame, []byte{'\r'}) {
		return workerResponse{}, errRPCFailure
	}
	frame = frame[:len(frame)-1]
	if rejectDuplicateJSONKeys(frame) != nil {
		return workerResponse{}, errRPCFailure
	}
	var raw map[string]json.RawMessage
	if json.Unmarshal(frame, &raw) != nil {
		return workerResponse{}, errRPCFailure
	}
	var response workerResponse
	if json.Unmarshal(frame, &response) != nil || response.SchemaVersion != workerSchemaVersion {
		return workerResponse{}, errRPCFailure
	}
	switch response.Status {
	case "ready":
		if !hasExactKeys(raw, "schema_version", "status") {
			return workerResponse{}, errRPCFailure
		}
	case "ok":
		if !hasExactKeys(raw, "schema_version", "status", "text") || response.Text == "" || len(response.Text) > maxAssistantTextBytes {
			return workerResponse{}, errRPCFailure
		}
	case "error":
		if !hasExactKeys(raw, "schema_version", "status", "code") || response.Code != "runtime_failed" {
			return workerResponse{}, errRPCFailure
		}
		return workerResponse{}, errRPCFailure
	default:
		return workerResponse{}, errRPCFailure
	}
	return response, nil
}

func boundedCommand(ctx context.Context, timeout time.Duration, stdoutLimit, stderrLimit int, executable string, arguments ...string) ([]byte, error) {
	commandCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	stdout := newBoundedCapture(stdoutLimit)
	stderr := newBoundedCapture(stderrLimit)
	command := exec.CommandContext(commandCtx, executable, arguments...)
	command.Dir = os.TempDir()
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil || commandCtx.Err() != nil || stdout.overflow || stderr.overflow {
		return nil, errRPCFailure
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func (evaluator *PiRPCEvaluator) Evaluate(ctx context.Context, input Input, selection ModelSelection) (QuestionSet, error) {
	if evaluator == nil || ctx == nil {
		return QuestionSet{}, invalidInput(errors.New("evaluator and context are required"))
	}
	if err := ValidateModelSelection(selection); err != nil {
		return QuestionSet{}, err
	}
	references, err := inputReferences(input)
	if err != nil {
		return QuestionSet{}, invalidInput(err)
	}
	message, err := json.Marshal(input)
	if err != nil {
		return QuestionSet{}, invalidInput(errors.New("evaluator input cannot be encoded"))
	}
	systemPrompt, err := evaluator.questionPrompt(input.SchemaVersion)
	if err != nil {
		return QuestionSet{}, invalidInput(err)
	}
	assistantText, err := evaluatePiModel(ctx, evaluator.runtime, systemPrompt, message, selection)
	if err != nil {
		return QuestionSet{}, err
	}
	if len(assistantText) > MaxQuestionSetBytes {
		return QuestionSet{}, invalidOutput("question-set output exceeds %d bytes", MaxQuestionSetBytes)
	}
	return ParseQuestionSet([]byte(assistantText), references)
}

func (evaluator *PiRPCAssessmentEvaluator) EvaluateAssessment(ctx context.Context, input AssessmentInput, selection ModelSelection) (AssessmentTurn, error) {
	if evaluator == nil || ctx == nil {
		return AssessmentTurn{}, invalidInput(errors.New("assessment evaluator and context are required"))
	}
	if err := ValidateModelSelection(selection); err != nil {
		return AssessmentTurn{}, err
	}
	if err := validateAssessmentInput(input); err != nil {
		return AssessmentTurn{}, invalidInput(err)
	}
	message, err := json.Marshal(input)
	if err != nil {
		return AssessmentTurn{}, invalidInput(errors.New("assessment input cannot be encoded"))
	}
	systemPrompt, err := evaluator.assessmentPrompt(input.SchemaVersion)
	if err != nil {
		return AssessmentTurn{}, invalidInput(err)
	}
	assistantText, err := evaluatePiModel(ctx, evaluator.runtime, systemPrompt, message, selection)
	if err != nil {
		return AssessmentTurn{}, err
	}
	if len(assistantText) > MaxAssessmentTurnBytes {
		return AssessmentTurn{}, invalidOutput("assessment output exceeds %d bytes", MaxAssessmentTurnBytes)
	}
	return ParseAssessmentTurn([]byte(assistantText), input)
}

func evaluatePiModel(ctx context.Context, runtime piModelRuntime, systemPrompt string, message []byte, selection ModelSelection) (string, error) {
	if validateSystemPrompt(systemPrompt) != nil {
		return "", invalidInput(errors.New("system prompt is invalid"))
	}
	response, err := runModelWorker(ctx, runtime, workerRequest{
		SchemaVersion:    workerSchemaVersion,
		Action:           "evaluate",
		SDKEntry:         runtime.sdkEntry,
		SettingsEntry:    runtime.settingsEntry,
		HTTPEntry:        runtime.httpEntry,
		AttributionEntry: runtime.attributionEntry,
		SystemPrompt:     systemPrompt,
		Message:          string(message),
		Model: &workerModel{
			Provider: selection.Provider, ID: selection.ModelID, ThinkingLevel: selection.ThinkingLevel,
		},
	}, evaluatorProcessTimeout)
	if err != nil {
		return "", evaluationError(ctx, err)
	}
	if response.Status != "ok" {
		return "", errRPCFailure
	}
	return response.Text, nil
}

func (evaluator *PiRPCEvaluator) questionPrompt(schemaVersion int) (string, error) {
	switch schemaVersion {
	case InputSchemaVersion:
		if evaluator.systemPrompt != "" {
			return evaluator.systemPrompt, nil
		}
	case InputSchemaVersionV2:
		if evaluator.systemPromptV2 != "" {
			return evaluator.systemPromptV2, nil
		}
	}
	return "", errors.New("question evaluator prompt is unavailable")
}

func (evaluator *PiRPCAssessmentEvaluator) assessmentPrompt(schemaVersion int) (string, error) {
	switch schemaVersion {
	case AssessmentInputSchemaVersion:
		if evaluator.systemPrompt != "" {
			return evaluator.systemPrompt, nil
		}
	case AssessmentInputSchemaVersionV2:
		if evaluator.systemPromptV2 != "" {
			return evaluator.systemPromptV2, nil
		}
	}
	return "", errors.New("assessment evaluator prompt is unavailable")
}

func inputReferences(input Input) ([]string, error) {
	return runtimeInputReferences(input)
}

func evaluationError(ctx context.Context, workerError error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if errors.Is(workerError, context.DeadlineExceeded) || errors.Is(workerError, context.Canceled) {
		return workerError
	}
	return errRPCFailure
}

type boundedCapture struct {
	buffer   bytes.Buffer
	maximum  int
	overflow bool
}

func newBoundedCapture(maximum int) *boundedCapture {
	return &boundedCapture{maximum: maximum}
}

func (capture *boundedCapture) Write(content []byte) (int, error) {
	remaining := capture.maximum - capture.buffer.Len()
	if remaining > 0 {
		kept := len(content)
		if kept > remaining {
			kept = remaining
		}
		_, _ = capture.buffer.Write(content[:kept])
	}
	if len(content) > remaining {
		capture.overflow = true
	}
	return len(content), nil
}

func (capture *boundedCapture) Bytes() []byte {
	return capture.buffer.Bytes()
}

func (capture *boundedCapture) String() string {
	return capture.buffer.String()
}
