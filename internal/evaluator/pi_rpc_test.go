package evaluator

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/reeezark/pi-learnloop/agent/prompts"
)

func TestNewPiRPCEvaluator(t *testing.T) {
	t.Run("resolves a symlink and freezes a supported executable", func(t *testing.T) {
		fake := installFakePi(t, "success")
		evaluator, err := NewPiRPCEvaluator(context.Background(), prompts.EvaluatorQuestionGenerationV1())
		if err != nil {
			t.Fatalf("NewPiRPCEvaluator(): %v", err)
		}
		resolved, err := filepath.EvalSymlinks(fake.executable)
		if err != nil {
			t.Fatalf("EvalSymlinks(%q): %v", fake.executable, err)
		}
		if evaluator.executable != resolved {
			t.Fatalf("executable = %q, want frozen path %q", evaluator.executable, resolved)
		}
		if evaluator.systemPrompt != prompts.EvaluatorQuestionGenerationV1() {
			t.Fatal("system prompt does not equal the released embedded asset")
		}
	})

	t.Run("rejects a missing executable without exposing PATH", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, err := NewPiRPCEvaluator(context.Background(), "safe prompt")
		if err == nil || strings.Contains(err.Error(), os.Getenv("PATH")) {
			t.Fatalf("NewPiRPCEvaluator() error = %v, want opaque unavailable error", err)
		}
	})

	t.Run("rejects an unsupported version", func(t *testing.T) {
		installFakePi(t, "wrong_version")
		_, err := NewPiRPCEvaluator(context.Background(), "safe prompt")
		if err == nil || strings.Contains(err.Error(), "0.84.4") {
			t.Fatalf("NewPiRPCEvaluator() error = %v, want opaque unavailable error", err)
		}
	})

	t.Run("rejects an invalid released prompt", func(t *testing.T) {
		installFakePi(t, "success")
		if _, err := NewPiRPCEvaluator(context.Background(), ""); err == nil {
			t.Fatal("NewPiRPCEvaluator() error = nil, want invalid prompt rejection")
		}
	})
}

func TestPiRPCEvaluatorEvaluate(t *testing.T) {
	input := syntheticRPCInput()
	selection := syntheticModelSelection()

	t.Run("uses the fixed isolated invocation and one LF-framed prompt", func(t *testing.T) {
		fake := installFakePi(t, "success")
		evaluator := mustNewFakeEvaluator(t)
		result, err := evaluator.Evaluate(context.Background(), input, selection)
		if err != nil {
			t.Fatalf("Evaluate(): %v", err)
		}
		if result.Disposition != DispositionQuestions || len(result.Questions) != 3 {
			t.Fatalf("result = %#v, want three validated questions", result)
		}
		assertFakeProcessGone(t, fake.pidPath)

		arguments := readFakeArguments(t, fake.argumentsPath)
		wantArguments, err := BuildPiArguments(selection, prompts.EvaluatorQuestionGenerationV1())
		if err != nil {
			t.Fatalf("BuildPiArguments(): %v", err)
		}
		if strings.Join(arguments, "\x00") != strings.Join(wantArguments, "\x00") {
			t.Fatalf("arguments = %#v, want %#v", arguments, wantArguments)
		}
		for _, argument := range arguments {
			if strings.Contains(argument, "synthetic source") || strings.Contains(argument, "/private/repository") {
				t.Fatalf("argv contains runtime evidence or a repository path: %#v", arguments)
			}
		}

		commands := readFakeCommands(t, fake.commandsPath)
		if len(commands) != 5 {
			t.Fatalf("command count = %d, want exactly 5 setup/prompt/result commands", len(commands))
		}
		wantTypes := []string{"set_auto_retry", "set_auto_compaction", "get_commands", "prompt", "get_last_assistant_text"}
		for index, wantType := range wantTypes {
			if commands[index]["type"] != wantType {
				t.Fatalf("command %d type = %#v, want %q", index, commands[index]["type"], wantType)
			}
		}
		if commands[0]["enabled"] != false || commands[1]["enabled"] != false {
			t.Fatalf("setup commands = %#v, want retry and compaction disabled", commands[:2])
		}
		promptMessage, ok := commands[3]["message"].(string)
		if !ok {
			t.Fatalf("prompt message = %#v, want string", commands[3]["message"])
		}
		var sent Input
		if err := json.Unmarshal([]byte(promptMessage), &sent); err != nil {
			t.Fatalf("prompt message is not the runtime input JSON: %v", err)
		}
		if sent.SchemaVersion != InputSchemaVersion || len(sent.EvidenceBundle.Items) != 1 || sent.EvidenceBundle.Items[0].Content != "synthetic source" {
			t.Fatalf("sent input = %#v, want exact synthetic evidence input", sent)
		}
	})

	t.Run("keeps Unicode separators inside one JSONL record", func(t *testing.T) {
		installFakePi(t, "unicode_separator")
		result, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		if err != nil || result.Disposition != DispositionQuestions {
			t.Fatalf("Evaluate() = (%#v, %v), want valid questions", result, err)
		}
	})

	t.Run("rejects a mismatched response id", func(t *testing.T) {
		fake := installFakePi(t, "wrong_id")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("rejects invalid RPC JSON", func(t *testing.T) {
		fake := installFakePi(t, "invalid_json")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("rejects discovered commands before prompting", func(t *testing.T) {
		fake := installFakePi(t, "commands_present")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		commands := readFakeCommands(t, fake.commandsPath)
		for _, command := range commands {
			if command["type"] == "prompt" {
				t.Fatal("prompt was sent after discovered commands violated isolation")
			}
		}
	})

	t.Run("rejects a tool execution event", func(t *testing.T) {
		fake := installFakePi(t, "tool_event")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("rejects an unknown assistant update shape", func(t *testing.T) {
		fake := installFakePi(t, "unknown_update")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("maps malformed assistant text to invalid output", func(t *testing.T) {
		installFakePi(t, "invalid_output")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		if ContractErrorCodeOf(err) != ContractErrorInvalidOutput {
			t.Fatalf("Evaluate() error = %v, want invalid_output", err)
		}
	})

	t.Run("does not expose an authentication failure", func(t *testing.T) {
		installFakePi(t, "auth_failure")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		if strings.Contains(err.Error(), "credential-secret-value") {
			t.Fatalf("Evaluate() exposed raw child output: %v", err)
		}
	})

	t.Run("rejects missing model metadata before spawning RPC", func(t *testing.T) {
		fake := installFakePi(t, "success")
		evaluator := mustNewFakeEvaluator(t)
		invalid := selection
		invalid.ModelID = ""
		_, err := evaluator.Evaluate(context.Background(), input, invalid)
		if ContractErrorCodeOf(err) != ContractErrorInvalidInput {
			t.Fatalf("Evaluate() error = %v, want invalid_input", err)
		}
		if _, err := os.Stat(fake.pidPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("RPC process PID file error = %v, want process not started", err)
		}
	})

	t.Run("honors deadline and reaps the process", func(t *testing.T) {
		fake := installFakePi(t, "hang")
		evaluator := mustNewFakeEvaluator(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := evaluator.Evaluate(ctx, input, selection)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Evaluate() error = %v, want context deadline exceeded", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("honors cancellation and reaps the process", func(t *testing.T) {
		fake := installFakePi(t, "hang")
		evaluator := mustNewFakeEvaluator(t)
		ctx, cancel := context.WithCancel(context.Background())
		result := make(chan error, 1)
		go func() {
			_, err := evaluator.Evaluate(ctx, input, selection)
			result <- err
		}()
		waitForFakePID(t, fake.pidPath)
		cancel()
		err := <-result
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Evaluate() error = %v, want context canceled", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("enforces the stdout cap", func(t *testing.T) {
		fake := installFakePi(t, "stdout_cap")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("enforces the stderr cap", func(t *testing.T) {
		fake := installFakePi(t, "stderr_cap")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("reports an early child exit and reaps it", func(t *testing.T) {
		fake := installFakePi(t, "child_exit")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		assertOpaqueRPCFailure(t, err)
		assertFakeProcessGone(t, fake.pidPath)
	})
}

func TestReadRPCFrames(t *testing.T) {
	t.Run("stops reading immediately after the stdout cap", func(t *testing.T) {
		reader := &countingByteReader{remaining: 2 * maxRPCStdoutBytes}
		frames := make(chan rpcFrame, 1)
		readRPCFrames(context.Background(), reader, frames)
		frame := <-frames
		if frame.err == nil {
			t.Fatal("readRPCFrames() error = nil, want stdout cap failure")
		}
		if reader.read > maxRPCStdoutBytes+1 {
			t.Fatalf("readRPCFrames() consumed %d bytes, want at most %d", reader.read, maxRPCStdoutBytes+1)
		}
	})
}

func TestPiRPCHelperProcess(t *testing.T) {
	if os.Getenv("PI_LEARNLOOP_FAKE_RPC") != "1" {
		return
	}
	os.Exit(runFakePiProcess(fakeProcessArguments()))
}

type fakePi struct {
	executable    string
	argumentsPath string
	commandsPath  string
	pidPath       string
	startsPath    string
}

func installFakePi(t *testing.T, scenario string) fakePi {
	t.Helper()
	realDirectory := t.TempDir()
	binDirectory := t.TempDir()
	realExecutable := filepath.Join(realDirectory, "pi-real")
	version := SupportedPiVersion
	if scenario == "wrong_version" {
		version = "0.84.4"
	}
	script := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '%%s\\n' %q; exit 0; fi\nexec %q -test.run '^TestPiRPCHelperProcess$' -- \"$@\"\n", version, os.Args[0])
	if err := os.WriteFile(realExecutable, []byte(script), 0o700); err != nil {
		t.Fatalf("WriteFile(fake Pi): %v", err)
	}
	executable := filepath.Join(binDirectory, "pi")
	if err := os.Symlink(realExecutable, executable); err != nil {
		t.Fatalf("Symlink(fake Pi): %v", err)
	}
	recordDirectory := t.TempDir()
	fake := fakePi{
		executable:    executable,
		argumentsPath: filepath.Join(recordDirectory, "arguments.json"),
		commandsPath:  filepath.Join(recordDirectory, "commands.jsonl"),
		pidPath:       filepath.Join(recordDirectory, "pid"),
		startsPath:    filepath.Join(recordDirectory, "starts"),
	}
	t.Setenv("PATH", binDirectory)
	t.Setenv("PI_LEARNLOOP_FAKE_RPC", "1")
	t.Setenv("PI_LEARNLOOP_FAKE_SCENARIO", scenario)
	t.Setenv("PI_LEARNLOOP_FAKE_ARGUMENTS", fake.argumentsPath)
	t.Setenv("PI_LEARNLOOP_FAKE_COMMANDS", fake.commandsPath)
	t.Setenv("PI_LEARNLOOP_FAKE_PID", fake.pidPath)
	t.Setenv("PI_LEARNLOOP_FAKE_STARTS", fake.startsPath)
	return fake
}

func mustNewFakeEvaluator(t *testing.T) *PiRPCEvaluator {
	t.Helper()
	evaluator, err := NewPiRPCEvaluator(context.Background(), prompts.EvaluatorQuestionGenerationV1())
	if err != nil {
		t.Fatalf("NewPiRPCEvaluator(): %v", err)
	}
	return evaluator
}

func runFakePiProcess(arguments []string) int {
	scenario := os.Getenv("PI_LEARNLOOP_FAKE_SCENARIO")
	if len(arguments) == 1 && arguments[0] == "--version" {
		if scenario == "wrong_version" {
			fmt.Fprintln(os.Stdout, "0.84.4")
		} else {
			fmt.Fprintln(os.Stdout, SupportedPiVersion)
		}
		return 0
	}
	if err := appendFakeStart(); err != nil {
		return 90
	}
	if err := os.WriteFile(os.Getenv("PI_LEARNLOOP_FAKE_PID"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return 91
	}
	encodedArguments, _ := json.Marshal(arguments)
	if err := os.WriteFile(os.Getenv("PI_LEARNLOOP_FAKE_ARGUMENTS"), encodedArguments, 0o600); err != nil {
		return 92
	}

	switch scenario {
	case "hang":
		time.Sleep(time.Hour)
	case "stderr_cap":
		_, _ = os.Stderr.Write([]byte(strings.Repeat("s", maxRPCStderrBytes+1)))
		time.Sleep(time.Hour)
	case "child_exit":
		return 17
	}

	lastAssistantText := syntheticQuestionSetJSON()
	reader := bufio.NewReader(os.Stdin)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return 0
		}
		if !strings.HasSuffix(line, "\n") || strings.HasSuffix(line, "\r\n") {
			return 93
		}
		line = strings.TrimSuffix(line, "\n")
		if err := appendFakeCommand(line); err != nil {
			return 94
		}
		if scenario == "stdout_cap" {
			_, _ = os.Stdout.Write([]byte(strings.Repeat("x", maxRPCStdoutBytes+1) + "\n"))
			time.Sleep(time.Hour)
		}
		if scenario == "invalid_json" {
			fmt.Fprintln(os.Stdout, "{not-json}")
			time.Sleep(time.Hour)
		}

		var command map[string]any
		if json.Unmarshal([]byte(line), &command) != nil {
			return 95
		}
		kind, _ := command["type"].(string)
		id, _ := command["id"].(string)
		switch kind {
		case "set_auto_retry", "set_auto_compaction":
			if scenario == "wrong_id" && kind == "set_auto_retry" {
				id = "wrong-id"
			}
			writeFakeJSON(map[string]any{"id": id, "type": "response", "command": kind, "success": true})
		case "get_commands":
			commands := []any{}
			if scenario == "commands_present" {
				commands = []any{map[string]any{"name": "unsafe", "source": "extension"}}
			}
			writeFakeJSON(map[string]any{"id": id, "type": "response", "command": kind, "success": true, "data": map[string]any{"commands": commands}})
		case "prompt":
			if scenario == "auth_failure" {
				writeFakeJSON(map[string]any{"id": id, "type": "response", "command": kind, "success": false, "error": "credential-secret-value"})
				time.Sleep(time.Hour)
			}
			writeFakeJSON(map[string]any{"id": id, "type": "response", "command": kind, "success": true})
			message, _ := command["message"].(string)
			lastAssistantText = fakeAssistantText(scenario, message)
			writeFakeJSON(map[string]any{"type": "agent_start"})
			writeFakeJSON(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "start"}})
			switch scenario {
			case "tool_event":
				writeFakeJSON(map[string]any{"type": "tool_execution_start", "toolCallId": "call-1", "toolName": "bash", "args": map[string]any{}})
			case "unknown_update":
				writeFakeJSON(map[string]any{"type": "message_update", "assistantMessageEvent": map[string]any{"type": "credential_event"}})
			case "unicode_separator":
				fmt.Fprintf(os.Stdout, "{\"type\":\"message_update\",\"assistantMessageEvent\":{\"type\":\"text_delta\",\"contentIndex\":0,\"delta\":\"left%sright\"}}\n", "\u2028")
			}
			writeFakeJSON(map[string]any{
				"type": "message_update",
				"assistantMessageEvent": map[string]any{
					"type":   "done",
					"reason": "stop",
					"message": map[string]any{
						"role":    "assistant",
						"content": []any{map[string]any{"type": "text", "text": lastAssistantText}},
					},
				},
			})
			writeFakeJSON(map[string]any{"type": "agent_settled"})
		case "get_last_assistant_text":
			text := lastAssistantText
			if scenario == "invalid_output" {
				text = "not-json"
			}
			writeFakeJSON(map[string]any{"id": id, "type": "response", "command": kind, "success": true, "data": map[string]any{"text": text}})
		default:
			return 96
		}
	}
}

func fakeAssistantText(scenario, message string) string {
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(message), &envelope) != nil {
		return syntheticQuestionSetJSON()
	}
	var stage AssessmentStage
	if raw, exists := envelope["stage"]; !exists || json.Unmarshal(raw, &stage) != nil {
		return syntheticQuestionSetJSON()
	}
	switch scenario {
	case "assessment_follow_up":
		if stage == AssessmentStageInitialAnswers {
			return syntheticAssessmentFollowUpJSON()
		}
		return syntheticAssessmentCompleteJSON()
	case "assessment_invalid_output":
		return "not-json"
	case "assessment_invalid_schema":
		return strings.TrimSuffix(syntheticAssessmentCompleteJSON(), "}") + `,"score":100}`
	case "assessment_unknown_reference":
		return strings.Replace(syntheticAssessmentCompleteJSON(), `"E001"`, `"E999"`, 1)
	case "assessment_oversized_output":
		return strings.Repeat("x", MaxAssessmentTurnBytes+1)
	default:
		return syntheticAssessmentCompleteJSON()
	}
}

func fakeProcessArguments() []string {
	for index, argument := range os.Args {
		if argument == "--" {
			return os.Args[index+1:]
		}
	}
	return nil
}

func appendFakeCommand(line string) error {
	file, err := os.OpenFile(os.Getenv("PI_LEARNLOOP_FAKE_COMMANDS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, line)
	return err
}

func appendFakeStart() error {
	file, err := os.OpenFile(os.Getenv("PI_LEARNLOOP_FAKE_STARTS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = fmt.Fprintln(file, os.Getpid())
	return err
}

func writeFakeJSON(value any) {
	_ = json.NewEncoder(os.Stdout).Encode(value)
}

func syntheticRPCInput() Input {
	return Input{
		SchemaVersion: InputSchemaVersion,
		EvidenceBundle: EvidenceBundle{
			Items: []EvidenceItem{{Reference: "E001", Content: "synthetic source"}},
		},
	}
}

func syntheticModelSelection() ModelSelection {
	return ModelSelection{
		PiVersion:     SupportedPiVersion,
		Provider:      "synthetic-provider",
		ModelID:       "synthetic-model",
		ThinkingLevel: "off",
	}
}

func syntheticQuestionSetJSON() string {
	return `{"schema_version":1,"disposition":"questions","questions":[{"id":"Q1","kind":"code_specific","text":"What behavior changed?","evidence_references":["E001"]},{"id":"Q2","kind":"code_specific","text":"Which boundary matters?","evidence_references":["E001"]},{"id":"Q3","kind":"go_backend","text":"How would a Go test cover this?","evidence_references":[]}]}`
}

func syntheticAssessmentFollowUpJSON() string {
	return `{"schema_version":1,"disposition":"follow_up","follow_up":{"id":"F1","target_question_id":"Q1","text":"Which exact branch supports your first answer?","evidence_references":["E001"]},"evaluations":[]}`
}

func syntheticAssessmentCompleteJSON() string {
	return `{"schema_version":1,"disposition":"complete","follow_up":null,"evaluations":[{"question_id":"Q1","verdict":"demonstrated","feedback":"The answer identifies the selected behavior.","evidence_references":["E001"]},{"question_id":"Q2","verdict":"partial","feedback":"The answer omits one selected edge path.","evidence_references":["E001"]},{"question_id":"Q3","verdict":"not_demonstrated","feedback":"The answer needs a clearer testing explanation.","evidence_references":[]}]}`
}

func readFakeArguments(t *testing.T, path string) []string {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(arguments): %v", err)
	}
	var arguments []string
	if err := json.Unmarshal(content, &arguments); err != nil {
		t.Fatalf("Unmarshal(arguments): %v", err)
	}
	return arguments
}

func readFakeCommands(t *testing.T, path string) []map[string]any {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(commands): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	commands := make([]map[string]any, len(lines))
	for index, line := range lines {
		if err := json.Unmarshal([]byte(line), &commands[index]); err != nil {
			t.Fatalf("Unmarshal(command %d): %v", index, err)
		}
	}
	return commands
}

func assertOpaqueRPCFailure(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errRPCFailure) || err.Error() != errRPCFailure.Error() {
		t.Fatalf("error = %v, want opaque RPC failure", err)
	}
}

func assertFakeProcessGone(t *testing.T, pidPath string) {
	t.Helper()
	content := waitForFakePID(t, pidPath)
	pid, err := strconv.Atoi(string(content))
	if err != nil {
		t.Fatalf("Atoi(PID): %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pid, 0); errors.Is(err, syscall.ESRCH) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake Pi process %d is still alive", pid)
}

func waitForFakePID(t *testing.T, pidPath string) []byte {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		content, err := os.ReadFile(pidPath)
		if err == nil {
			return content
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("fake Pi PID %q was not published", pidPath)
	return nil
}

type countingByteReader struct {
	remaining int
	read      int
}

func (reader *countingByteReader) Read(content []byte) (int, error) {
	if reader.remaining == 0 {
		return 0, io.EOF
	}
	count := len(content)
	if count > reader.remaining {
		count = reader.remaining
	}
	for index := 0; index < count; index++ {
		content[index] = 'x'
	}
	reader.remaining -= count
	reader.read += count
	return count, nil
}
