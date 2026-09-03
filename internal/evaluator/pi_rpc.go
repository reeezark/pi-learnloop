package evaluator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	piVersionPreflightTimeout = 2 * time.Second
	evaluatorProcessTimeout   = 120 * time.Second
	maxRPCStdoutBytes         = 2 * 1024 * 1024
	maxRPCStderrBytes         = 64 * 1024
	maxVersionOutputBytes     = 4 * 1024
)

var errRPCFailure = errors.New("Pi evaluator RPC failed")

// PiRPCEvaluator runs one isolated, no-tools Pi RPC process per evaluation.
// Its executable path and released prompt are frozen at daemon startup.
type PiRPCEvaluator struct {
	executable     string
	systemPrompt   string
	systemPromptV2 string
}

// PiRPCAssessmentEvaluator runs one isolated, no-tools Pi RPC process per
// answer-assessment turn. It has a separate narrow interface from question
// generation while sharing only private process-isolation mechanics.
type PiRPCAssessmentEvaluator struct {
	executable     string
	systemPrompt   string
	systemPromptV2 string
}

// NewPiRPCEvaluator resolves and preflights the supported Pi executable once.
// All failures are intentionally opaque because paths and raw process output
// must not cross the evaluator boundary.
func NewPiRPCEvaluator(ctx context.Context, systemPrompt string) (*PiRPCEvaluator, error) {
	executable, err := resolvePiRPCExecutable(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &PiRPCEvaluator{executable: executable, systemPrompt: systemPrompt}, nil
}

// NewPiRPCAssessmentEvaluator resolves and preflights the supported Pi
// executable once and freezes the released assessment prompt.
func NewPiRPCAssessmentEvaluator(ctx context.Context, systemPrompt string) (*PiRPCAssessmentEvaluator, error) {
	executable, err := resolvePiRPCExecutable(ctx, systemPrompt)
	if err != nil {
		return nil, err
	}
	return &PiRPCAssessmentEvaluator{executable: executable, systemPrompt: systemPrompt}, nil
}

// NewVersionedPiRPCEvaluator freezes both released question prompts behind one
// evaluator seam. Runtime input version, not the client, selects the prompt.
func NewVersionedPiRPCEvaluator(ctx context.Context, systemPromptV1, systemPromptV2 string) (*PiRPCEvaluator, error) {
	executable, err := resolvePiRPCExecutable(ctx, systemPromptV1)
	if err != nil || validateAdditionalPrompt(systemPromptV2) != nil {
		return nil, errors.New("Pi evaluator is unavailable")
	}
	return &PiRPCEvaluator{executable: executable, systemPrompt: systemPromptV1, systemPromptV2: systemPromptV2}, nil
}

// NewVersionedPiRPCAssessmentEvaluator freezes both released assessment prompts
// behind the unchanged narrow assessment seam.
func NewVersionedPiRPCAssessmentEvaluator(ctx context.Context, systemPromptV1, systemPromptV2 string) (*PiRPCAssessmentEvaluator, error) {
	executable, err := resolvePiRPCExecutable(ctx, systemPromptV1)
	if err != nil || validateAdditionalPrompt(systemPromptV2) != nil {
		return nil, errors.New("Pi evaluator is unavailable")
	}
	return &PiRPCAssessmentEvaluator{executable: executable, systemPrompt: systemPromptV1, systemPromptV2: systemPromptV2}, nil
}

func validateAdditionalPrompt(systemPrompt string) error {
	_, err := BuildPiArguments(ModelSelection{
		PiVersion: SupportedPiVersion, Provider: "preflight-provider", ModelID: "preflight-model", ThinkingLevel: "off",
	}, systemPrompt)
	return err
}

func resolvePiRPCExecutable(ctx context.Context, systemPrompt string) (string, error) {
	if ctx == nil {
		return "", errors.New("Pi evaluator is unavailable")
	}
	if _, err := BuildPiArguments(ModelSelection{
		PiVersion:     SupportedPiVersion,
		Provider:      "preflight-provider",
		ModelID:       "preflight-model",
		ThinkingLevel: "off",
	}, systemPrompt); err != nil {
		return "", errors.New("Pi evaluator is unavailable")
	}

	executable, err := exec.LookPath("pi")
	if err != nil {
		return "", errors.New("Pi evaluator is unavailable")
	}
	if !filepath.IsAbs(executable) {
		executable, err = filepath.Abs(executable)
		if err != nil {
			return "", errors.New("Pi evaluator is unavailable")
		}
	}
	executable, err = filepath.EvalSymlinks(executable)
	if err != nil {
		return "", errors.New("Pi evaluator is unavailable")
	}
	info, err := os.Stat(executable)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return "", errors.New("Pi evaluator is unavailable")
	}
	if err := preflightPiVersion(ctx, executable); err != nil {
		return "", errors.New("Pi evaluator is unavailable")
	}
	return executable, nil
}

func preflightPiVersion(ctx context.Context, executable string) error {
	preflightCtx, cancel := context.WithTimeout(ctx, piVersionPreflightTimeout)
	defer cancel()
	stdout := newBoundedCapture(maxVersionOutputBytes)
	stderr := newBoundedCapture(maxVersionOutputBytes)
	command := exec.CommandContext(preflightCtx, executable, "--version")
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Run(); err != nil || preflightCtx.Err() != nil || stdout.overflow || stderr.overflow {
		return errRPCFailure
	}
	if strings.TrimSpace(stdout.String()) != SupportedPiVersion {
		return errRPCFailure
	}
	return nil
}

// Evaluate sends exactly one validated runtime input to a new Pi RPC process.
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
	assistantText, err := evaluatePiRPC(ctx, evaluator.executable, systemPrompt, message, selection)
	if err != nil {
		return QuestionSet{}, err
	}
	if len(assistantText) > MaxQuestionSetBytes {
		return QuestionSet{}, invalidOutput("question-set output exceeds %d bytes", MaxQuestionSetBytes)
	}
	return ParseQuestionSet([]byte(assistantText), references)
}

// EvaluateAssessment sends exactly one validated assessment turn to a new Pi
// RPC process. The process is never retained across human input.
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
	assistantText, err := evaluatePiRPC(ctx, evaluator.executable, systemPrompt, message, selection)
	if err != nil {
		return AssessmentTurn{}, err
	}
	if len(assistantText) > MaxAssessmentTurnBytes {
		return AssessmentTurn{}, invalidOutput("assessment output exceeds %d bytes", MaxAssessmentTurnBytes)
	}
	return ParseAssessmentTurn([]byte(assistantText), input)
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

func evaluatePiRPC(ctx context.Context, executable, systemPrompt string, message []byte, selection ModelSelection) (string, error) {
	arguments, err := BuildPiArguments(selection, systemPrompt)
	if err != nil {
		return "", err
	}
	evaluationCtx, cancel := context.WithTimeout(ctx, evaluatorProcessTimeout)
	defer cancel()
	process, err := startRPCProcess(evaluationCtx, executable, arguments)
	if err != nil {
		return "", errRPCFailure
	}
	defer process.close()

	if err := process.send(evaluationCtx, map[string]any{
		"id":      "pll-setup-retry",
		"type":    "set_auto_retry",
		"enabled": false,
	}); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.awaitSimpleResponse(evaluationCtx, "pll-setup-retry", "set_auto_retry"); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.send(evaluationCtx, map[string]any{
		"id":      "pll-setup-compaction",
		"type":    "set_auto_compaction",
		"enabled": false,
	}); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.awaitSimpleResponse(evaluationCtx, "pll-setup-compaction", "set_auto_compaction"); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.send(evaluationCtx, map[string]any{
		"id":   "pll-get-commands",
		"type": "get_commands",
	}); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.awaitEmptyCommands(evaluationCtx, "pll-get-commands"); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.send(evaluationCtx, map[string]any{
		"id":      "pll-prompt",
		"type":    "prompt",
		"message": string(message),
	}); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.awaitPromptSettled(evaluationCtx, "pll-prompt"); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	if err := process.send(evaluationCtx, map[string]any{
		"id":   "pll-last-text",
		"type": "get_last_assistant_text",
	}); err != nil {
		return "", evaluationError(evaluationCtx)
	}
	assistantText, err := process.awaitLastAssistantText(evaluationCtx, "pll-last-text")
	if err != nil {
		return "", evaluationError(evaluationCtx)
	}
	return assistantText, nil
}

func inputReferences(input Input) ([]string, error) {
	return runtimeInputReferences(input)
}

func evaluationError(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
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

func (capture *boundedCapture) String() string {
	return capture.buffer.String()
}

type rpcFrame struct {
	content []byte
	err     error
}

type rpcProcess struct {
	command        *exec.Cmd
	stdin          io.WriteCloser
	frames         <-chan rpcFrame
	stderrOverflow <-chan struct{}
	wait           <-chan error
	cancelReaders  context.CancelFunc
	waited         bool
}

func startRPCProcess(ctx context.Context, executable string, arguments []string) (*rpcProcess, error) {
	command := exec.Command(executable, arguments...)
	command.Dir = os.TempDir()
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	stderr, err := command.StderrPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, err
	}

	readerCtx, cancelReaders := context.WithCancel(ctx)
	frames := make(chan rpcFrame, 8)
	stderrOverflow := make(chan struct{}, 1)
	wait := make(chan error, 1)
	go readRPCFrames(readerCtx, stdout, frames)
	go monitorRPCStderr(stderr, stderrOverflow)
	go func() {
		wait <- command.Wait()
	}()
	return &rpcProcess{
		command:        command,
		stdin:          stdin,
		frames:         frames,
		stderrOverflow: stderrOverflow,
		wait:           wait,
		cancelReaders:  cancelReaders,
	}, nil
}

func (process *rpcProcess) close() {
	process.cancelReaders()
	_ = process.stdin.Close()
	if !process.waited {
		_ = process.command.Process.Kill()
		<-process.wait
		process.waited = true
	}
}

func (process *rpcProcess) send(ctx context.Context, command any) error {
	content, err := json.Marshal(command)
	if err != nil {
		return errRPCFailure
	}
	content = append(content, '\n')
	done := make(chan error, 1)
	go func() {
		_, err := process.stdin.Write(content)
		done <- err
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (process *rpcProcess) next(ctx context.Context) ([]byte, error) {
	select {
	case frame, ok := <-process.frames:
		if !ok || frame.err != nil {
			return nil, errRPCFailure
		}
		return frame.content, nil
	default:
	}
	select {
	case frame, ok := <-process.frames:
		if !ok || frame.err != nil {
			return nil, errRPCFailure
		}
		return frame.content, nil
	case <-process.stderrOverflow:
		return nil, errRPCFailure
	case <-process.wait:
		process.waited = true
		return nil, errRPCFailure
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (process *rpcProcess) awaitSimpleResponse(ctx context.Context, id, command string) error {
	content, err := process.next(ctx)
	if err != nil {
		return err
	}
	object, err := strictRPCObject(content)
	if err != nil {
		return err
	}
	return validateSimpleResponse(object, id, command)
}

func (process *rpcProcess) awaitEmptyCommands(ctx context.Context, id string) error {
	content, err := process.next(ctx)
	if err != nil {
		return err
	}
	object, err := strictRPCObject(content)
	if err != nil || !hasExactKeys(object, "id", "type", "command", "success", "data") {
		return errRPCFailure
	}
	if !matchesResponse(object, id, "get_commands") {
		return errRPCFailure
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(object["data"], &data); err != nil || !hasExactKeys(data, "commands") {
		return errRPCFailure
	}
	var commands []json.RawMessage
	if err := json.Unmarshal(data["commands"], &commands); err != nil || commands == nil || len(commands) != 0 {
		return errRPCFailure
	}
	return nil
}

func (process *rpcProcess) awaitPromptSettled(ctx context.Context, id string) error {
	accepted := false
	started := false
	settled := false
	for !accepted || !settled {
		content, err := process.next(ctx)
		if err != nil {
			return err
		}
		object, err := strictRPCObject(content)
		if err != nil {
			return err
		}
		kind, err := rpcObjectType(object)
		if err != nil {
			return err
		}
		if kind == "response" {
			if accepted || validateSimpleResponse(object, id, "prompt") != nil {
				return errRPCFailure
			}
			accepted = true
			continue
		}
		event, err := validateRPCEvent(kind, object)
		if err != nil {
			return err
		}
		switch event {
		case rpcEventStarted:
			if started || settled {
				return errRPCFailure
			}
			started = true
		case rpcEventSettled:
			if !started || settled {
				return errRPCFailure
			}
			settled = true
		}
	}
	return nil
}

func (process *rpcProcess) awaitLastAssistantText(ctx context.Context, id string) (string, error) {
	content, err := process.next(ctx)
	if err != nil {
		return "", err
	}
	object, err := strictRPCObject(content)
	if err != nil || !hasExactKeys(object, "id", "type", "command", "success", "data") {
		return "", errRPCFailure
	}
	if !matchesResponse(object, id, "get_last_assistant_text") {
		return "", errRPCFailure
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(object["data"], &data); err != nil || !hasExactKeys(data, "text") {
		return "", errRPCFailure
	}
	var text *string
	if err := json.Unmarshal(data["text"], &text); err != nil || text == nil {
		return "", errRPCFailure
	}
	return *text, nil
}

func strictRPCObject(content []byte) (map[string]json.RawMessage, error) {
	if len(content) == 0 || !json.Valid(content) || rejectDuplicateJSONKeys(content) != nil {
		return nil, errRPCFailure
	}
	var object map[string]json.RawMessage
	if err := json.Unmarshal(content, &object); err != nil || object == nil {
		return nil, errRPCFailure
	}
	return object, nil
}

func validateSimpleResponse(object map[string]json.RawMessage, id, command string) error {
	if !hasExactKeys(object, "id", "type", "command", "success") || !matchesResponse(object, id, command) {
		return errRPCFailure
	}
	return nil
}

func matchesResponse(object map[string]json.RawMessage, id, command string) bool {
	var responseType, responseID, responseCommand string
	var success bool
	return json.Unmarshal(object["type"], &responseType) == nil && responseType == "response" &&
		json.Unmarshal(object["id"], &responseID) == nil && responseID == id &&
		json.Unmarshal(object["command"], &responseCommand) == nil && responseCommand == command &&
		json.Unmarshal(object["success"], &success) == nil && success
}

func rpcObjectType(object map[string]json.RawMessage) (string, error) {
	raw, exists := object["type"]
	if !exists {
		return "", errRPCFailure
	}
	var kind string
	if err := json.Unmarshal(raw, &kind); err != nil || kind == "" {
		return "", errRPCFailure
	}
	return kind, nil
}

type rpcEvent int

const (
	rpcEventNormal rpcEvent = iota
	rpcEventStarted
	rpcEventSettled
)

func validateRPCEvent(kind string, object map[string]json.RawMessage) (rpcEvent, error) {
	switch kind {
	case "agent_start":
		if !hasExactKeys(object, "type") {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventStarted, nil
	case "agent_settled":
		if !hasExactKeys(object, "type") {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventSettled, nil
	case "turn_start":
		if !hasExactKeys(object, "type") {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventNormal, nil
	case "message_start", "message_end":
		if _, exists := object["message"]; !exists || containsToolValue(object["message"]) {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventNormal, nil
	case "message_update":
		var update map[string]json.RawMessage
		if raw, exists := object["assistantMessageEvent"]; !exists || json.Unmarshal(raw, &update) != nil {
			return rpcEventNormal, errRPCFailure
		}
		var updateType string
		if json.Unmarshal(update["type"], &updateType) != nil {
			return rpcEventNormal, errRPCFailure
		}
		switch updateType {
		case "start", "text_start", "text_delta", "text_end", "thinking_start", "thinking_delta", "thinking_end":
		case "done":
			var reason string
			if json.Unmarshal(update["reason"], &reason) != nil || (reason != "stop" && reason != "length") {
				return rpcEventNormal, errRPCFailure
			}
			if message, exists := update["message"]; !exists || containsToolValue(message) {
				return rpcEventNormal, errRPCFailure
			}
		default:
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventNormal, nil
	case "turn_end":
		var toolResults []json.RawMessage
		if raw, exists := object["toolResults"]; !exists || json.Unmarshal(raw, &toolResults) != nil || len(toolResults) != 0 {
			return rpcEventNormal, errRPCFailure
		}
		if raw, exists := object["message"]; !exists || containsToolValue(raw) {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventNormal, nil
	case "agent_end":
		var willRetry bool
		if raw, exists := object["willRetry"]; !exists || json.Unmarshal(raw, &willRetry) != nil || willRetry {
			return rpcEventNormal, errRPCFailure
		}
		if raw, exists := object["messages"]; !exists || containsToolValue(raw) {
			return rpcEventNormal, errRPCFailure
		}
		return rpcEventNormal, nil
	case "tool_execution_start", "tool_execution_update", "tool_execution_end",
		"bash_execution_update", "extension_ui_request", "extension_error",
		"auto_retry_start", "auto_retry_end", "compaction_start", "compaction_end",
		"summarization_retry_scheduled", "summarization_retry_attempt_start", "summarization_retry_finished",
		"queue_update":
		return rpcEventNormal, errRPCFailure
	default:
		return rpcEventNormal, errRPCFailure
	}
}

func containsToolValue(content json.RawMessage) bool {
	var value any
	if json.Unmarshal(content, &value) != nil {
		return true
	}
	var walk func(any) bool
	walk = func(current any) bool {
		switch typed := current.(type) {
		case map[string]any:
			if kind, ok := typed["type"].(string); ok && (kind == "toolCall" || kind == "toolResult") {
				return true
			}
			for _, nested := range typed {
				if walk(nested) {
					return true
				}
			}
		case []any:
			for _, nested := range typed {
				if walk(nested) {
					return true
				}
			}
		}
		return false
	}
	return walk(value)
}

func readRPCFrames(ctx context.Context, stdout io.Reader, frames chan<- rpcFrame) {
	defer close(frames)
	reader := bufio.NewReaderSize(io.LimitReader(stdout, maxRPCStdoutBytes+1), 32*1024)
	total := 0
	for {
		record, err := reader.ReadBytes('\n')
		total += len(record)
		if total > maxRPCStdoutBytes {
			sendRPCFrame(ctx, frames, rpcFrame{err: errRPCFailure})
			return
		}
		if len(record) > 0 {
			if record[len(record)-1] != '\n' {
				sendRPCFrame(ctx, frames, rpcFrame{err: errRPCFailure})
				return
			}
			record = record[:len(record)-1]
			if len(record) > 0 && record[len(record)-1] == '\r' {
				record = record[:len(record)-1]
			}
			if len(record) == 0 {
				sendRPCFrame(ctx, frames, rpcFrame{err: errRPCFailure})
				return
			}
			if !sendRPCFrame(ctx, frames, rpcFrame{content: append([]byte(nil), record...)}) {
				return
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) || len(record) == 0 {
				sendRPCFrame(ctx, frames, rpcFrame{err: errRPCFailure})
			}
			return
		}
	}
}

func sendRPCFrame(ctx context.Context, frames chan<- rpcFrame, frame rpcFrame) bool {
	select {
	case frames <- frame:
		return true
	case <-ctx.Done():
		return false
	}
}

func monitorRPCStderr(stderr io.Reader, overflow chan<- struct{}) {
	buffer := make([]byte, 32*1024)
	total := 0
	reported := false
	for {
		count, err := stderr.Read(buffer)
		total += count
		if total > maxRPCStderrBytes && !reported {
			reported = true
			select {
			case overflow <- struct{}{}:
			default:
			}
		}
		if err != nil {
			return
		}
	}
}
