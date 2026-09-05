package evaluator

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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
	t.Run("freezes matching Node Pi and SDK paths", func(t *testing.T) {
		fake := installFakePi(t, "success")
		evaluator, err := NewPiRPCEvaluator(context.Background(), prompts.EvaluatorQuestionGenerationV1())
		if err != nil {
			t.Fatalf("NewPiRPCEvaluator(): %v", err)
		}
		wantPi, _ := filepath.EvalSymlinks(fake.realPi)
		wantNode, _ := filepath.EvalSymlinks(fake.nodeExecutable)
		if evaluator.runtime.piExecutable != wantPi || evaluator.runtime.nodeExecutable != wantNode {
			t.Fatalf("runtime executables = (%q, %q), want frozen (%q, %q)", evaluator.runtime.piExecutable, evaluator.runtime.nodeExecutable, wantPi, wantNode)
		}
		wantRoot, _ := filepath.EvalSymlinks(fake.packageRoot)
		for _, path := range []string{evaluator.runtime.sdkEntry, evaluator.runtime.settingsEntry, evaluator.runtime.httpEntry, evaluator.runtime.attributionEntry} {
			if !strings.HasPrefix(path, wantRoot+string(os.PathSeparator)) {
				t.Fatalf("runtime path %q escapes package %q", path, fake.packageRoot)
			}
		}
		if evaluator.systemPrompt != prompts.EvaluatorQuestionGenerationV1() {
			t.Fatal("system prompt does not equal released embedded asset")
		}
	})

	for _, test := range []struct {
		name     string
		scenario string
	}{
		{name: "missing executable", scenario: "missing_pi"},
		{name: "missing Node", scenario: "missing_node"},
		{name: "unsupported Pi version", scenario: "wrong_version"},
		{name: "unsupported Node version", scenario: "wrong_node_version"},
		{name: "mismatched package version", scenario: "wrong_package_version"},
		{name: "missing SDK entry", scenario: "missing_sdk"},
		{name: "SDK preflight failure", scenario: "runtime_preflight_fail"},
	} {
		t.Run("rejects "+test.name+" opaquely", func(t *testing.T) {
			installFakePi(t, test.scenario)
			_, err := NewPiRPCEvaluator(context.Background(), "safe prompt")
			if err == nil || err.Error() != "Pi evaluator is unavailable" {
				t.Fatalf("NewPiRPCEvaluator() error = %v, want opaque unavailable error", err)
			}
		})
	}

	for name, prompt := range map[string]string{
		"empty":         "",
		"invalid UTF-8": string([]byte{0xff}),
		"oversized":     strings.Repeat("a", MaxSystemPromptBytes+1),
	} {
		t.Run("rejects "+name+" released prompt", func(t *testing.T) {
			installFakePi(t, "success")
			if _, err := NewPiRPCEvaluator(context.Background(), prompt); err == nil {
				t.Fatal("NewPiRPCEvaluator() error = nil, want invalid prompt rejection")
			}
		})
	}
}

func TestNewVersionedPiModelEvaluatorsShareOneFrozenPreflight(t *testing.T) {
	fake := installFakePi(t, "success")
	questions, assessments, err := NewVersionedPiModelEvaluators(
		context.Background(), "question-v1", "question-v2", "assessment-v1", "assessment-v2",
	)
	if err != nil || questions == nil || assessments == nil {
		t.Fatalf("NewVersionedPiModelEvaluators() = (%#v, %#v, %v)", questions, assessments, err)
	}
	if questions.runtime != assessments.runtime {
		t.Fatal("question and assessment adapters did not share frozen runtime paths")
	}
	content, err := os.ReadFile(fake.preflightsPath)
	if err != nil || string(content) != "preflight\n" {
		t.Fatalf("runtime preflights = %q, want exactly one (error = %v)", content, err)
	}
}

func TestPiRPCEvaluatorEvaluate(t *testing.T) {
	input := syntheticRPCInput()
	selection := syntheticModelSelection()

	t.Run("uses one isolated worker and an exact LF-framed request", func(t *testing.T) {
		fake := installFakePi(t, "success")
		result, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		if err != nil {
			t.Fatalf("Evaluate(): %v", err)
		}
		if result.Disposition != DispositionQuestions || len(result.Questions) != 3 {
			t.Fatalf("result = %#v, want three validated questions", result)
		}
		assertFakeProcessGone(t, fake.pidPath)
		request := readFakeWorkerRequest(t, fake.requestsPath, 1)
		if request.Action != "evaluate" || request.SystemPrompt != prompts.EvaluatorQuestionGenerationV1() || request.Model == nil ||
			request.Model.Provider != selection.Provider || request.Model.ID != selection.ModelID || request.Model.ThinkingLevel != selection.ThinkingLevel {
			t.Fatalf("worker request = %#v, want exact prompt and model", request)
		}
		var sent Input
		if err := json.Unmarshal([]byte(request.Message), &sent); err != nil || sent.EvidenceBundle.Items[0].Content != "synthetic source" {
			t.Fatalf("worker message = %#v, want exact runtime input (error = %v)", request.Message, err)
		}
		arguments := readFakeArguments(t, fake.argumentsPath)
		if len(arguments) != 3 || arguments[0] != "--input-type=module" || arguments[1] != "--eval" {
			t.Fatalf("worker arguments = %#v, want fixed in-memory module invocation", arguments)
		}
		joined := strings.Join(arguments, "\x00")
		for _, forbidden := range []string{"synthetic source", "/private/repository", selection.Provider, selection.ModelID} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("argv contains runtime input %q", forbidden)
			}
		}
	})

	t.Run("does not enter the Pi CLI command registry", func(t *testing.T) {
		fake := installFakePi(t, "commands_present")
		result, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		if err != nil || result.Disposition != DispositionQuestions {
			t.Fatalf("Evaluate() = (%#v, %v), want valid questions", result, err)
		}
		if _, err := os.Stat(fake.commandsPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("Pi CLI command registry was entered: %v", err)
		}
	})

	t.Run("maps malformed assistant text to invalid output", func(t *testing.T) {
		installFakePi(t, "invalid_output")
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
		if ContractErrorCodeOf(err) != ContractErrorInvalidOutput {
			t.Fatalf("Evaluate() error = %v, want invalid_output", err)
		}
	})

	for _, scenario := range []string{"invalid_json", "duplicate_response", "extra_frame", "unknown_response", "auth_failure", "stdout_cap", "stderr_cap", "child_exit"} {
		t.Run("rejects worker "+scenario+" opaquely", func(t *testing.T) {
			fake := installFakePi(t, scenario)
			_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, selection)
			assertOpaqueRPCFailure(t, err)
			if strings.Contains(fmt.Sprint(err), "credential-secret-value") {
				t.Fatalf("Evaluate() exposed raw child output: %v", err)
			}
			assertFakeProcessGone(t, fake.pidPath)
		})
	}

	t.Run("rejects missing model metadata before spawning", func(t *testing.T) {
		fake := installFakePi(t, "success")
		invalid := selection
		invalid.ModelID = ""
		_, err := mustNewFakeEvaluator(t).Evaluate(context.Background(), input, invalid)
		if ContractErrorCodeOf(err) != ContractErrorInvalidInput {
			t.Fatalf("Evaluate() error = %v, want invalid_input", err)
		}
		if _, err := os.Stat(fake.pidPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("worker started for invalid input: %v", err)
		}
	})

	t.Run("honors a deadline and reaps the worker", func(t *testing.T) {
		fake := installFakePi(t, "hang")
		evaluator := mustNewFakeEvaluator(t)
		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()
		_, err := evaluator.Evaluate(ctx, input, selection)
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("Evaluate() error = %v, want deadline exceeded", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})

	t.Run("honors cancellation and reaps the worker", func(t *testing.T) {
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
		if err := <-result; !errors.Is(err, context.Canceled) {
			t.Fatalf("Evaluate() error = %v, want context canceled", err)
		}
		assertFakeProcessGone(t, fake.pidPath)
	})
}

func TestRunModelWorkerRejectsOversizedPrivateRequestBeforeSpawning(t *testing.T) {
	fake := installFakePi(t, "success")
	evaluator := mustNewFakeEvaluator(t)
	_, err := runModelWorker(context.Background(), evaluator.runtime, workerRequest{
		SchemaVersion: workerSchemaVersion,
		Action:        "evaluate",
		Message:       strings.Repeat("x", maxWorkerRequestBytes),
	}, time.Second)
	assertOpaqueRPCFailure(t, err)
	if _, err := os.Stat(fake.pidPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("oversized request started a worker: %v", err)
	}
}

func TestPiModelWorkerHelperProcess(t *testing.T) {
	if os.Getenv("PI_LEARNLOOP_FAKE_WORKER") != "1" {
		return
	}
	os.Exit(runFakeWorker())
}

type fakePi struct {
	packageRoot    string
	realPi         string
	nodeExecutable string
	argumentsPath  string
	requestsPath   string
	commandsPath   string
	pidPath        string
	startsPath     string
	preflightsPath string
}

func installFakePi(t *testing.T, scenario string) fakePi {
	t.Helper()
	packageRoot := t.TempDir()
	binDirectory := t.TempDir()
	recordDirectory := t.TempDir()
	for _, relative := range []string{
		"dist/bundle/cli.js", "dist/index.js", "dist/core/settings-manager.js",
		"dist/core/http-dispatcher.js", "dist/core/provider-attribution.js",
	} {
		if scenario == "missing_sdk" && relative == "dist/index.js" {
			continue
		}
		path := filepath.Join(packageRoot, relative)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		mode := os.FileMode(0o600)
		content := "export {};\n"
		if relative == "dist/bundle/cli.js" {
			mode = 0o700
			version := SupportedPiVersion
			if scenario == "wrong_version" {
				version = "0.84.4"
			}
			content = fmt.Sprintf("#!/bin/sh\nprintf '%%s\\n' %q\n", version)
		}
		if err := os.WriteFile(path, []byte(content), mode); err != nil {
			t.Fatal(err)
		}
	}
	packageVersion := SupportedPiVersion
	if scenario == "wrong_package_version" {
		packageVersion = "0.84.4"
	}
	manifest := fmt.Sprintf(`{"name":"@earendil-works/pi-coding-agent","version":%q,"type":"module","main":"./dist/index.js","bin":{"pi":"./dist/bundle/cli.js"}}`, packageVersion)
	if err := os.WriteFile(filepath.Join(packageRoot, "package.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	realPi := filepath.Join(packageRoot, "dist/bundle/cli.js")
	if scenario != "missing_pi" {
		if err := os.Symlink(realPi, filepath.Join(binDirectory, "pi")); err != nil {
			t.Fatal(err)
		}
	}
	nodeVersion := "v22.19.0"
	if scenario == "wrong_node_version" {
		nodeVersion = "v22.18.0"
	}
	nodeExecutable := filepath.Join(binDirectory, "node")
	nodeScript := fmt.Sprintf("#!/bin/sh\nif [ \"$1\" = \"--version\" ]; then printf '%%s\\n' %q; exit 0; fi\nexec %q -test.run '^TestPiModelWorkerHelperProcess$' -- \"$@\"\n", nodeVersion, os.Args[0])
	if scenario != "missing_node" {
		if err := os.WriteFile(nodeExecutable, []byte(nodeScript), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	fake := fakePi{
		packageRoot: packageRoot, realPi: realPi, nodeExecutable: nodeExecutable,
		argumentsPath:  filepath.Join(recordDirectory, "arguments.json"),
		requestsPath:   filepath.Join(recordDirectory, "requests.jsonl"),
		commandsPath:   filepath.Join(recordDirectory, "commands.jsonl"),
		pidPath:        filepath.Join(recordDirectory, "pid"),
		startsPath:     filepath.Join(recordDirectory, "starts"),
		preflightsPath: filepath.Join(recordDirectory, "preflights"),
	}
	t.Setenv("PATH", binDirectory)
	t.Setenv("PI_LEARNLOOP_FAKE_WORKER", "1")
	t.Setenv("PI_LEARNLOOP_FAKE_SCENARIO", scenario)
	t.Setenv("PI_LEARNLOOP_FAKE_ARGUMENTS", fake.argumentsPath)
	t.Setenv("PI_LEARNLOOP_FAKE_REQUESTS", fake.requestsPath)
	t.Setenv("PI_LEARNLOOP_FAKE_PID", fake.pidPath)
	t.Setenv("PI_LEARNLOOP_FAKE_STARTS", fake.startsPath)
	t.Setenv("PI_LEARNLOOP_FAKE_PREFLIGHTS", fake.preflightsPath)
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

func runFakeWorker() int {
	arguments := fakeProcessArguments()
	content, err := os.ReadFile("/dev/stdin")
	if err != nil || len(content) == 0 || content[len(content)-1] != '\n' {
		return 90
	}
	var request workerRequest
	if json.Unmarshal(content[:len(content)-1], &request) != nil {
		return 91
	}
	scenario := os.Getenv("PI_LEARNLOOP_FAKE_SCENARIO")
	if request.Action == "preflight" {
		file, err := os.OpenFile(os.Getenv("PI_LEARNLOOP_FAKE_PREFLIGHTS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			return 97
		}
		_, err = fmt.Fprintln(file, "preflight")
		_ = file.Close()
		if err != nil {
			return 98
		}
		if scenario == "runtime_preflight_fail" {
			writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "error", "code": "runtime_failed"})
			return 1
		}
		writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "ready"})
		return 0
	}
	if err := os.WriteFile(os.Getenv("PI_LEARNLOOP_FAKE_PID"), []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		return 92
	}
	if err := appendFakeStart(); err != nil {
		return 93
	}
	encodedArguments, _ := json.Marshal(arguments)
	if err := os.WriteFile(os.Getenv("PI_LEARNLOOP_FAKE_ARGUMENTS"), encodedArguments, 0o600); err != nil {
		return 94
	}
	file, err := os.OpenFile(os.Getenv("PI_LEARNLOOP_FAKE_REQUESTS"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return 95
	}
	_, writeErr := file.Write(content)
	_ = file.Close()
	if writeErr != nil {
		return 96
	}
	switch scenario {
	case "hang":
		time.Sleep(time.Hour)
	case "stderr_cap":
		_, _ = os.Stderr.Write([]byte(strings.Repeat("s", maxRPCStderrBytes+1)))
	case "stdout_cap":
		_, _ = os.Stdout.Write([]byte(strings.Repeat("x", maxRPCStdoutBytes+1)))
	case "child_exit":
		return 17
	case "invalid_json":
		fmt.Fprintln(os.Stdout, "{not-json}")
		return 0
	case "duplicate_response":
		fmt.Fprintf(os.Stdout, `{"schema_version":1,"status":"ok","status":"ok","text":%q}`+"\n", syntheticQuestionSetJSON())
		return 0
	case "extra_frame":
		writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "ok", "text": syntheticQuestionSetJSON()})
		writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "ok", "text": syntheticQuestionSetJSON()})
		return 0
	case "unknown_response":
		writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "ok", "text": syntheticQuestionSetJSON(), "extra": true})
		return 0
	case "auth_failure":
		fmt.Fprintln(os.Stderr, "credential-secret-value")
		writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "error", "code": "runtime_failed"})
		return 1
	}
	text := fakeAssistantText(scenario, request.Message)
	writeFakeJSON(map[string]any{"schema_version": workerSchemaVersion, "status": "ok", "text": text})
	return 0
}

func fakeAssistantText(scenario, message string) string {
	var envelope map[string]json.RawMessage
	if json.Unmarshal([]byte(message), &envelope) != nil {
		return syntheticQuestionSetJSON()
	}
	var stage AssessmentStage
	if raw, exists := envelope["stage"]; !exists || json.Unmarshal(raw, &stage) != nil {
		if scenario == "invalid_output" {
			return "not-json"
		}
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
	return Input{SchemaVersion: InputSchemaVersion, EvidenceBundle: EvidenceBundle{
		Items: []EvidenceItem{{Reference: "E001", Content: "synthetic source"}},
	}}
}

func syntheticModelSelection() ModelSelection {
	return ModelSelection{PiVersion: SupportedPiVersion, Provider: "synthetic-provider", ModelID: "synthetic-model", ThinkingLevel: "off"}
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

func readFakeWorkerRequest(t *testing.T, path string, count int) workerRequest {
	t.Helper()
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(requests): %v", err)
	}
	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != count {
		t.Fatalf("worker request count = %d, want %d", len(lines), count)
	}
	var request workerRequest
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &request); err != nil {
		t.Fatalf("Unmarshal(worker request): %v", err)
	}
	return request
}

func assertOpaqueRPCFailure(t *testing.T, err error) {
	t.Helper()
	if !errors.Is(err, errRPCFailure) || err.Error() != errRPCFailure.Error() {
		t.Fatalf("error = %v, want opaque runtime failure", err)
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
	t.Fatalf("fake worker process %d is still alive", pid)
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
	t.Fatalf("fake worker PID %q was not published", pidPath)
	return nil
}
