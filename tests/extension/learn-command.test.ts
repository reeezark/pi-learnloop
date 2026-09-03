import assert from "node:assert/strict";
import test from "node:test";

import {
  createLearnCommand,
  createLearnHistoryCommand,
  EvidenceClientError,
  type AssessmentResult,
  type LearnClient,
  type EvidencePreviewResponse,
  formatPreview,
  type LearnCommandContext,
  type LearningHistoryClient,
} from "../../extensions/lib/learn-command.ts";
import {
  completeGoContext,
  partialGoContextWithChangedImport,
  unavailableGoContext,
} from "./go-context-fixture.ts";

test("manual learning history query renders source-free repository records without a model client", async () => {
  const requests: Parameters<LearningHistoryClient["history"]>[] = [];
  const client: LearningHistoryClient = {
    async history(repository, limit) {
      requests.push([repository, limit]);
      return {
        protocol_version: 1,
        records: [{
          record_id: `lr1-${"h".repeat(43)}`,
          started_at: "2026-09-01T12:00:00Z",
          finished_at: "2026-09-01T12:01:00Z",
          status: "complete",
          failure_code: null,
          base_revision: "a".repeat(40),
          head_revision: "c".repeat(40),
          evidence_manifest_sha256: "b".repeat(64),
          question_schema_version: 1,
          assessment_schema_version: 1,
          question_prompt: { id: "question-prompt", version: "1.0.0", sha256: "d".repeat(64) },
          assessment_prompt: { id: "assessment-prompt", version: "1.0.0", sha256: "e".repeat(64) },
          pi_version: "0.84.3",
          provider: "provider",
          model_id: "model",
          thinking_level: "off",
          follow_up_used: false,
          label: "partial",
          outcomes: [
            { question_id: "Q1", question_kind: "code_specific", verdict: "demonstrated" },
            { question_id: "Q2", question_kind: "code_specific", verdict: "partial" },
            { question_id: "Q3", question_kind: "go_backend", verdict: "not_demonstrated" },
          ],
        }],
      };
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() { throw new Error("must not select"); },
      async input() { throw new Error("must not collect input"); },
      async editor() { throw new Error("must not edit"); },
      async confirm() { throw new Error("must not confirm a model call"); },
      notify(message, type) { notifications.push({ message, type }); },
    },
  };

  await createLearnHistoryCommand(client)("", context);

  assert.deepEqual(requests, [["/work/repository", 20]]);
  assert.equal(notifications.length, 1);
  assert.equal(notifications[0]?.type, "info");
  assert.match(notifications[0]?.message ?? "", /Learning history/);
  assert.match(notifications[0]?.message ?? "", /partial/);
  assert.match(notifications[0]?.message ?? "", /Q2 code_specific partial/);
  assert.match(notifications[0]?.message ?? "", /provider\/model/);
  assert.match(notifications[0]?.message ?? "", /question-prompt@1\.0\.0/);
});

test("empty learning history is a normal informational result", async () => {
  const client: LearningHistoryClient = {
    async history() { return { protocol_version: 1, records: [] }; },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() { throw new Error("must not select"); },
      async input() { throw new Error("must not collect input"); },
      async editor() { throw new Error("must not edit"); },
      async confirm() { throw new Error("must not confirm"); },
      notify(message, type) { notifications.push({ message, type }); },
    },
  };

  await createLearnHistoryCommand(client)("", context);

  assert.deepEqual(notifications, [{
    message: "No learning history is available for this repository.",
    type: "info",
  }]);
});

test("unavailable learning history is explained without attempting repair", async () => {
  let requests = 0;
  const client: LearningHistoryClient = {
    async history() {
      requests += 1;
      throw new EvidenceClientError("history_unavailable", "storage unavailable");
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() { throw new Error("must not select"); },
      async input() { throw new Error("must not collect input"); },
      async editor() { throw new Error("must not edit"); },
      async confirm() { throw new Error("must not confirm"); },
      notify(message, type) { notifications.push({ message, type }); },
    },
  };

  await createLearnHistoryCommand(client)("", context);

  assert.equal(requests, 1);
  assert.equal(notifications[0]?.type, "warning");
  assert.match(notifications[0]?.message ?? "", /left the database unchanged/i);
});

test("manual commit range shows the selected evidence preview", async () => {
  const requests: Parameters<LearnClient["preview"]>[] = [];
  const client: LearnClient = {
    async preview(repository, selection) {
      requests.push([repository, selection]);
      return {
        protocol_version: 1,
        applied_limits: {
          max_files: 20,
          max_declarations: 100,
          max_excerpt_bytes: 131_072,
        },
        preview: {
          repository_root: "/work/repository",
          base_revision: "base-sha",
          head_revision: "head-sha",
          files: [
            {
              path: "internal/example/example.go",
              status: "modified",
              changed_lines: [{ start: 3, end: 5 }],
              declarations: [
                {
                  kind: "function",
                  name: "Answer",
                  receiver: "",
                  identity: "Answer",
                  start_line: 3,
                  end_line: 5,
                  changed_lines: [{ start: 3, end: 5 }],
                  excerpt: "func Answer() int { return 42 }",
                  excerpt_truncated: false,
                },
              ],
              omissions: [],
            },
          ],
          go_context: completeGoContext(),
          truncation: {
            truncated: false,
            omitted_files: 0,
            omitted_declarations: 0,
            omitted_excerpt_bytes: 0,
          },
        },
        continuation: { available: false, reason: "insufficient_evidence" },
      };
    },
    async questions() {
      throw new Error("must not be called");
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const inputs = ["base-sha", "head-sha"];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Commit range";
      },
      async input() {
        return inputs.shift();
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return false;
      },
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  };

  await createLearnCommand(client)("", context);

  assert.deepEqual(requests, [
    ["/work/repository", { kind: "commit_range", base: "base-sha", head: "head-sha" }],
  ]);
  assert.equal(notifications.length, 1);
  assert.equal(notifications[0]?.type, "info");
  assert.match(notifications[0]?.message ?? "", /internal\/example\/example\.go/);
  assert.match(notifications[0]?.message ?? "", /Answer/);
  assert.match(notifications[0]?.message ?? "", /31 bytes/);
  assert.match(notifications[0]?.message ?? "", /evidence_kind=code/);
  assert.match(notifications[0]?.message ?? "", /sha256=[0-9a-f]{64}/);
  assert.match(notifications[0]?.message ?? "", /Changed-evidence Truncation: none/);
  assert.match(notifications[0]?.message ?? "", /Go context: complete/);
});

test("renders import-only context, build policy, relationships, omissions, and budget truncation", () => {
  const response = continuablePreview();
  response.preview.files = [{
    path: "main.go",
    status: "modified",
    changed_lines: [{ start: 3, end: 3 }],
    declarations: [],
    omissions: [{ reason: "outside_declaration", count: 1 }],
  }];
  response.preview.go_context = partialGoContextWithChangedImport();

  const message = formatPreview(response);

  assert.match(message, /Changed declarations: none/);
  assert.match(message, /Go context: partial/);
  assert.match(message, /C001 example\.com\/dep · changed_import/);
  assert.match(message, /"example\.com\/dep"/);
  assert.match(message, /imports\/syntactic/);
  assert.match(message, /Module: example\.com\/repo/);
  assert.match(message, /analysis_timeout_ms=30000/);
  assert.match(message, /evaluator_input_bytes=262144/);
  assert.match(message, /external_type_unavailable=1, output_truncated=1/);
  assert.match(message, /1 items, 0 relationships, 12 bytes omitted/);
});

test("renders repository control and format characters as visible escapes", () => {
  const response = continuablePreview();
  const declaration = response.preview.files[0]!.declarations[0]!;
  declaration.excerpt = `func Answer() int { return 42 }${String.fromCodePoint(0x1b, 0x202e)}`;

  const message = formatPreview(response);

  assert.match(message, /\\u\{1b\}\\u\{202e\}/);
  assert.doesNotMatch(message, /\u001b|\u202e/u);
});

test("unavailable context is shown and requires confirmation before continuation consumption", async () => {
  let previewCalls = 0;
  let questionCalls = 0;
  const events: string[] = [];
  const response = continuablePreview();
  response.preview.go_context = unavailableGoContext();
  const client: LearnClient = {
    async preview() {
      previewCalls += 1;
      return response;
    },
    async questions() {
      questionCalls += 1;
      events.push("questions");
      return { schema_version: 1, disposition: "insufficient_evidence", questions: [] };
    },
  };
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "anthropic", id: "claude-test" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    ui: {
      async select() { return "Working tree against a base revision"; },
      async input() { return "HEAD"; },
      async editor() { throw new Error("must not edit"); },
      async confirm(_title, message) {
        events.push("confirm");
        assert.match(message, /completeness, omissions, and truncation shown above/);
        return true;
      },
      notify(message) {
        events.push(message.includes("Go context: unavailable") ? "preview" : "notification");
      },
    },
  };

  await createLearnCommand(client)("", context);

  assert.equal(previewCalls, 1);
  assert.equal(questionCalls, 1);
  assert.deepEqual(events.slice(0, 3), ["preview", "confirm", "questions"]);
});

test("daemon unavailability is reported with a recovery action", async () => {
  const requests: Parameters<LearnClient["preview"]>[] = [];
  const client: LearnClient = {
    async preview(repository, selection) {
      requests.push([repository, selection]);
      throw new EvidenceClientError("daemon_unavailable", "connect ECONNREFUSED");
    },
    async questions() {
      throw new Error("must not be called");
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return false;
      },
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  };

  await createLearnCommand(client)("", context);

  assert.deepEqual(requests, [["/work/repository", { kind: "working_tree", base: "HEAD" }]]);
  assert.deepEqual(notifications, [
    {
      message: "Pi LearnLoop daemon is unavailable. Start it with `pi-learnloop daemon`, then run /learn again.",
      type: "error",
    },
  ]);
});

test("unsupported or blank selections never reach the daemon", async () => {
  let requests = 0;
  const client: LearnClient = {
    async preview() {
      requests += 1;
      throw new Error("must not be called");
    },
    async questions() {
      throw new Error("must not be called");
    },
  };
  const notifications: string[] = [];
  const baseContext: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Unsupported selection";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return false;
      },
      notify(message) {
        notifications.push(message);
      },
    },
  };
  await createLearnCommand(client)("", baseContext);
  await createLearnCommand(client)("", {
    ...baseContext,
    ui: {
      ...baseContext.ui,
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "   ";
      },
    },
  });

  assert.equal(requests, 0);
  assert.deepEqual(notifications, [
    "That changeset type is not supported.",
    "A base Git revision is required.",
  ]);
});

test("persistent authentication failure explains how to recover", async () => {
  const client: LearnClient = {
    async preview() {
      throw new EvidenceClientError("unauthorized", "authentication required");
    },
    async questions() {
      throw new Error("must not be called");
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return false;
      },
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  };

  await createLearnCommand(client)("", context);

  assert.deepEqual(notifications, [
    {
      message: "Pi LearnLoop could not authenticate with the daemon. Restart `pi-learnloop daemon`, then run /learn again.",
      type: "error",
    },
  ]);
});

test("invalid revisions are reported as a correctable selection", async () => {
  const client: LearnClient = {
    async preview() {
      throw new EvidenceClientError("invalid_revision", "revision cannot be resolved");
    },
    async questions() {
      throw new Error("must not be called");
    },
  };
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Commit range";
      },
      async input(title) {
        return title.startsWith("Base") ? "missing-base" : "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return false;
      },
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  };

  await createLearnCommand(client)("", context);

  assert.deepEqual(notifications, [
    {
      message: "The selected Git revision could not be resolved. Check the base and head revisions, then run /learn again.",
      type: "error",
    },
  ]);
});

test("empty Go changes are explained without implying that evaluation ran", () => {
  const response: EvidencePreviewResponse = {
    protocol_version: 1,
    applied_limits: {
      max_files: 20,
      max_declarations: 100,
      max_excerpt_bytes: 131_072,
    },
    preview: {
      repository_root: "/work/repository",
      base_revision: "base-sha",
      head_revision: "WORKTREE",
      files: [],
      go_context: completeGoContext(),
      truncation: {
        truncated: false,
        omitted_files: 0,
        omitted_declarations: 0,
        omitted_excerpt_bytes: 0,
      },
    },
    continuation: { available: false, reason: "insufficient_evidence" },
  };

  const message = formatPreview(response);

  assert.match(message, /No changed Go code was found/);
  assert.match(message, /no model was called and nothing was saved/);
});

test("declining after the visible preview never sends a continuation request", async () => {
  let questionRequests = 0;
  const confirmations: Array<{ title: string; message: string }> = [];
  const notifications: string[] = [];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      questionRequests += 1;
      throw new Error("must not be called");
    },
  };
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "anthropic", id: "claude-test" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm(title, message) {
        confirmations.push({ title, message });
        return false;
      },
      notify(message) {
        notifications.push(message);
      },
    },
  };

  await createLearnCommand(client, "0.84.3")("", context);

  assert.equal(questionRequests, 0);
  assert.equal(confirmations.length, 1);
  assert.match(confirmations[0]?.message ?? "", /changed Go file\/declaration metadata and excerpts, selected-snapshot Go context items and metadata/);
  assert.match(confirmations[0]?.message ?? "", /Estimated repository-derived evidence is 31 bytes/);
  assert.match(confirmations[0]?.message ?? "", /complete evaluator input is capped at 262144 bytes/);
  assert.match(confirmations[0]?.message ?? "", /Model: anthropic\/claude-test \(thinking=off\)/);
  assert.match(confirmations[0]?.message ?? "", /provider cost/);
  assert.match(confirmations[0]?.message ?? "", /transport may retry/);
  assert.match(notifications[0] ?? "", /Enriched evidence preview/);
});

test("confirmation sends only the continuation and active model metadata, then renders three questions", async () => {
  const requests: Array<{ id: string; selection: Parameters<LearnClient["questions"]>[1] }> = [];
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions(id, selection) {
      requests.push({ id, selection });
      return {
        schema_version: 1,
        disposition: "questions",
        questions: [
          { id: "Q1", kind: "code_specific", text: "Explain the changed behavior?", evidence_references: ["E001"] },
          { id: "Q2", kind: "code_specific", text: "Which edge case matters?", evidence_references: ["E001"] },
          { id: "Q3", kind: "go_backend", text: "How would table-driven tests help?", evidence_references: [] },
        ],
      };
    },
  };
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "anthropic", id: "claude-test" },
    thinkingLevel: "high",
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        return true;
      },
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  };

  await createLearnCommand(client, "0.84.3")("", context);

  assert.deepEqual(requests, [{
    id: `pc1-${"A".repeat(43)}`,
    selection: { pi_version: "0.84.3", provider: "anthropic", id: "claude-test", thinking_level: "high" },
  }]);
  assert.equal(notifications.length, 2);
  assert.match(notifications[1]?.message ?? "", /1\. Explain the changed behavior\? \[Evidence: E001\]/);
  assert.match(notifications[1]?.message ?? "", /3\. How would table-driven tests help\?/);
});

test("missing supported model metadata disables continuation before confirmation", async () => {
  let confirmed = false;
  let questionRequests = 0;
  const notifications: string[] = [];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      questionRequests += 1;
      throw new Error("must not be called");
    },
  };
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    ui: {
      async select() {
        return "Working tree against a base revision";
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        confirmed = true;
        return true;
      },
      notify(message) {
        notifications.push(message);
      },
    },
  };

  await createLearnCommand(client, "0.84.3")("", context);

  assert.equal(confirmed, false);
  assert.equal(questionRequests, 0);
  assert.match(notifications.at(-1) ?? "", /active model selection is unsupported/);
});

test("collects reviewable multiline answers, discloses editor limits, and renders the Go-derived result", async () => {
  const submissions: unknown[] = [];
  const notifications: string[] = [];
  const inputs = ["HEAD", "  first\nanswer  ", "second\nanswer", "third answer"];
  const confirmations: Array<{ title: string; message: string }> = [];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const selectCalls: Array<{ title: string; options: string[] }> = [];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      return assessableQuestions();
    },
    async assess(id, submission) {
      submissions.push({ id, submission });
      return completeAssessment();
    },
  };
  const context = assessmentContext(inputs, confirmations, notifications, { editorCalls, selectCalls });

  await createLearnCommand(client, "0.84.3")("", context);

  assert.deepEqual(submissions, [{
    id: `as1-${"B".repeat(43)}`,
    submission: {
      stage: "initial_answers",
      answers: [
        { question_id: "Q1", text: "first\nanswer" },
        { question_id: "Q2", text: "second\nanswer" },
        { question_id: "Q3", text: "third answer" },
      ],
    },
  }]);
  assert.deepEqual(editorCalls, [
    { title: "Answer Q1" },
    { title: "Answer Q2" },
    { title: "Answer Q3" },
  ]);
  assert.deepEqual(selectCalls.at(-1), {
    title: "Review answers",
    options: ["Continue to sharing confirmation", "Edit Q1", "Edit Q2", "Edit Q3", "Cancel"],
  });
  assert.doesNotMatch(JSON.stringify(selectCalls), /first\nanswer|second\nanswer|third answer/);
  assert.equal(confirmations.length, 3);
  assert.equal(confirmations[1]?.title, "Use the multiline answer editor?");
  assert.match(confirmations[1]?.message ?? "", /does not save answer drafts/);
  assert.match(confirmations[1]?.message ?? "", /temporary prompt\.md/);
  assert.match(confirmations[1]?.message ?? "", /best-effort/);
  assert.match(confirmations[1]?.message ?? "", /swap, backup, recovery, history, or telemetry artifacts/);
  assert.match(confirmations[1]?.message ?? "", /oversized draft before LearnLoop can enforce the 4 KiB answer limit/);
  assert.match(confirmations[1]?.message ?? "", /Declining sends no answer/);
  assert.match(confirmations[2]?.message ?? "", /same 31 bytes of displayed repository-derived evidence together with your three answers/);
  assert.match(confirmations[2]?.message ?? "", /one additional evaluation/);
  assert.match(notifications.at(-1) ?? "", /Learning assessment: partial/);
  assert.match(notifications.at(-1) ?? "", /Q1 — demonstrated/);
});

test("reopens one accepted answer with bounded prefill and submits the valid replacement", async () => {
  const submissions: unknown[] = [];
  const inputs = ["HEAD", "first answer", "second answer", "third answer", "  revised\nsecond  "];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const reviewActions = ["Edit Q2", "Continue to sharing confirmation"];
  const client: LearnClient = {
    async preview() { return continuablePreview(); },
    async questions() { return assessableQuestions(); },
    async assess(id, submission) {
      submissions.push({ id, submission });
      return completeAssessment();
    },
  };

  await createLearnCommand(client, "0.84.3")("", assessmentContext(inputs, [], [], {
    editorCalls,
    reviewActions,
  }));

  assert.deepEqual(editorCalls.at(-1), { title: "Answer Q2", prefill: "second answer" });
  assert.deepEqual(submissions, [{
    id: `as1-${"B".repeat(43)}`,
    submission: {
      stage: "initial_answers",
      answers: [
        { question_id: "Q1", text: "first answer" },
        { question_id: "Q2", text: "revised\nsecond" },
        { question_id: "Q3", text: "third answer" },
      ],
    },
  }]);
});

test("invalid draft candidates reopen generically without prefill, disclosure, retention, or submission", async () => {
  const invalidDrafts = [
    "private-cr\rvalue",
    "private-tab\tvalue",
    "\rprivate-leading",
    "private-trailing\t",
    `private-c1${String.fromCodePoint(0x85)}value`,
    " \n ",
    "x".repeat(4_097),
    "private-surrogate\ud800",
  ];
  const inputs = ["HEAD", ...invalidDrafts, "  approved\nanswer  ", "second", "third"];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const notifications: string[] = [];
  const submissions: unknown[] = [];
  const client: LearnClient = {
    async preview() { return continuablePreview(); },
    async questions() { return assessableQuestions(); },
    async assess(_id, submission) {
      submissions.push(submission);
      return completeAssessment();
    },
  };

  await createLearnCommand(client, "0.84.3")("", assessmentContext(inputs, [], notifications, { editorCalls }));

  assert.deepEqual(editorCalls.slice(0, invalidDrafts.length + 1), Array.from(
    { length: invalidDrafts.length + 1 },
    () => ({ title: "Answer Q1" }),
  ));
  const warnings = notifications.filter((message) => message.includes("non-empty after trimming"));
  assert.equal(warnings.length, invalidDrafts.length);
  assert.ok(warnings.every((message) => message === warnings[0]));
  assert.doesNotMatch(
    JSON.stringify(notifications),
    /private-cr|private-tab|private-leading|private-trailing|private-c1|private-surrogate/,
  );
  assert.deepEqual(submissions, [{
    stage: "initial_answers",
    answers: [
      { question_id: "Q1", text: "approved\nanswer" },
      { question_id: "Q2", text: "second" },
      { question_id: "Q3", text: "third" },
    ],
  }]);
});

test("invalid edit recovery keeps the prior accepted answer and cancellation returns to review", async () => {
  const inputs: Array<string | undefined> = [
    "HEAD",
    "original\nanswer",
    "second",
    "third",
    "private-invalid\rreplacement",
    undefined,
  ];
  const notifications: string[] = [];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const submissions: unknown[] = [];
  const client: LearnClient = {
    async preview() { return continuablePreview(); },
    async questions() { return assessableQuestions(); },
    async assess(_id, submission) {
      submissions.push(submission);
      return completeAssessment();
    },
  };

  await createLearnCommand(client, "0.84.3")("", assessmentContext(inputs, [], notifications, {
    editorCalls,
    reviewActions: ["Edit Q1", "Continue to sharing confirmation"],
  }));

  assert.deepEqual(editorCalls.slice(-2), [
    { title: "Answer Q1", prefill: "original\nanswer" },
    { title: "Answer Q1", prefill: "original\nanswer" },
  ]);
  assert.equal(notifications.filter((message) => message.includes("non-empty after trimming")).length, 1);
  assert.doesNotMatch(JSON.stringify(notifications), /private-invalid/);
  assert.deepEqual(submissions, [{
    stage: "initial_answers",
    answers: [
      { question_id: "Q1", text: "original\nanswer" },
      { question_id: "Q2", text: "second" },
      { question_id: "Q3", text: "third" },
    ],
  }]);
});

test("declining the editor disclosure or sharing confirmation sends no answer", async () => {
  for (const scenario of [
    { inputs: ["HEAD"], confirmationResults: [true, false], expectedConfirmations: 2 },
    { inputs: ["HEAD", "first", "second", "third"], confirmationResults: [true, true, false], expectedConfirmations: 3 },
  ]) {
    let assessmentRequests = 0;
    const confirmations: Array<{ title: string; message: string }> = [];
    const editorCalls: Array<{ title: string; prefill?: string }> = [];
    const client: LearnClient = {
      async preview() { return continuablePreview(); },
      async questions() { return assessableQuestions(); },
      async assess() {
        assessmentRequests += 1;
        return completeAssessment();
      },
    };

    await createLearnCommand(client, "0.84.3")("", assessmentContext(
      [...scenario.inputs],
      confirmations,
      [],
      { confirmationResults: [...scenario.confirmationResults], editorCalls },
    ));

    assert.equal(assessmentRequests, 0);
    assert.equal(confirmations.length, scenario.expectedConfirmations);
    if (scenario.expectedConfirmations === 2) {
      assert.deepEqual(editorCalls, []);
    }
  }
});

test("cancelling Q1, Q2, or Q3 stops before review and assessment sharing", async () => {
  for (let cancelIndex = 0; cancelIndex < 3; cancelIndex += 1) {
    let assessmentRequests = 0;
    const confirmations: Array<{ title: string; message: string }> = [];
    const editorCalls: Array<{ title: string; prefill?: string }> = [];
    const inputs: Array<string | undefined> = ["HEAD", ...Array.from({ length: cancelIndex }, (_, index) => `answer-${index}`), undefined];
    const client: LearnClient = {
      async preview() { return continuablePreview(); },
      async questions() { return assessableQuestions(); },
      async assess() {
        assessmentRequests += 1;
        return completeAssessment();
      },
    };

    await createLearnCommand(client, "0.84.3")("", assessmentContext(inputs, confirmations, [], { editorCalls }));

    assert.equal(assessmentRequests, 0);
    assert.equal(confirmations.length, 2);
    assert.equal(editorCalls.length, cancelIndex + 1);
  }
});

test("cancelling or dismissing answer review discards all local answers", async () => {
  for (const reviewAction of ["Cancel", undefined]) {
    let assessmentRequests = 0;
    const confirmations: Array<{ title: string; message: string }> = [];
    const client: LearnClient = {
      async preview() { return continuablePreview(); },
      async questions() { return assessableQuestions(); },
      async assess() {
        assessmentRequests += 1;
        return completeAssessment();
      },
    };

    await createLearnCommand(client, "0.84.3")("", assessmentContext(
      ["HEAD", "first", "second", "third"],
      confirmations,
      [],
      { reviewActions: [reviewAction] },
    ));

    assert.equal(assessmentRequests, 0);
    assert.equal(confirmations.length, 2);
  }
});

test("an old daemon invalid_request fails closed without retry or answer disclosure", async () => {
  const privateAnswer = "private multiline\nanswer";
  const notifications: string[] = [];
  let assessmentRequests = 0;
  const client: LearnClient = {
    async preview() { return continuablePreview(); },
    async questions() { return assessableQuestions(); },
    async assess() {
      assessmentRequests += 1;
      throw new EvidenceClientError("invalid_request", `old daemon rejected ${privateAnswer}`);
    },
  };

  await createLearnCommand(client, "0.84.3")("", assessmentContext(
    ["HEAD", privateAnswer, "second", "third"],
    [],
    notifications,
  ));

  assert.equal(assessmentRequests, 1);
  assert.match(notifications.at(-1) ?? "", /Update the daemon and extension together/);
  assert.match(notifications.at(-1) ?? "", /did not retry or alter an answer/);
  assert.doesNotMatch(JSON.stringify(notifications), /private multiline|old daemon rejected/);
});

test("warns once when assessment succeeds but local history is unavailable", async () => {
  const notifications: Array<{ message: string; type?: "info" | "warning" | "error" }> = [];
  const inputs = ["HEAD", "first answer", "second answer", "third answer"];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      return assessableQuestions();
    },
    async assess() {
      return {
        ...completeAssessment(),
        history: { saved: false, reason: "storage_unavailable" },
      };
    },
  };
  const baseContext = assessmentContext(inputs, [], []);
  await createLearnCommand(client, "0.84.3")("", {
    ...baseContext,
    ui: {
      ...baseContext.ui,
      notify(message, type) {
        notifications.push({ message, type });
      },
    },
  });

  assert.ok(notifications.some(({ message, type }) => type === "info" && /Learning assessment: partial/.test(message)));
  assert.deepEqual(notifications.at(-1), {
    message: "The assessment completed, but local learning history could not be saved.",
    type: "warning",
  });
});

test("submits one multiline F1 through the same editor and never asks for a second follow-up", async () => {
  const submissions: unknown[] = [];
  const notifications: string[] = [];
  const inputs = ["HEAD", "first", "second", "third", "  follow-up\nanswer  "];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const selectCalls: Array<{ title: string; options: string[] }> = [];
  let calls = 0;
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      return assessableQuestions();
    },
    async assess(id, submission) {
      calls += 1;
      submissions.push({ id, submission });
      if (calls === 1) {
        return {
          turn: {
            schema_version: 1,
            disposition: "follow_up",
            follow_up: {
              id: "F1",
              target_question_id: "Q1",
              text: "Which selected branch supports that answer?",
              evidence_references: ["E001"],
            },
            evaluations: [],
          },
        };
      }
      return completeAssessment();
    },
  };
  const context = assessmentContext(inputs, [], notifications, { editorCalls, selectCalls });

  await createLearnCommand(client, "0.84.3")("", context);

  assert.equal(submissions.length, 2);
  assert.deepEqual(submissions[1], {
    id: `as1-${"B".repeat(43)}`,
    submission: { stage: "follow_up_answer", follow_up_id: "F1", answer: "follow-up\nanswer" },
  });
  assert.deepEqual(editorCalls.at(-1), { title: "Answer F1" });
  assert.equal(selectCalls.filter(({ title }) => title === "Review answers").length, 1);
  assert.ok(notifications.some((message) => /Follow-up for Q1/.test(message)));
  assert.match(notifications.at(-1) ?? "", /Learning assessment: partial/);
});

test("invalid F1 reopens generically and cancellation sends no follow-up request", async () => {
  const submissions: unknown[] = [];
  const notifications: string[] = [];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const inputs: Array<string | undefined> = ["HEAD", "first", "second", "third", "private\rF1", undefined];
  const client: LearnClient = {
    async preview() { return continuablePreview(); },
    async questions() { return assessableQuestions(); },
    async assess(_id, submission) {
      submissions.push(submission);
      return {
        turn: {
          schema_version: 1,
          disposition: "follow_up",
          follow_up: {
            id: "F1",
            target_question_id: "Q1",
            text: "Which selected branch supports that answer?",
            evidence_references: ["E001"],
          },
          evaluations: [],
        },
      };
    },
  };

  await createLearnCommand(client, "0.84.3")("", assessmentContext(inputs, [], notifications, { editorCalls }));

  assert.equal(submissions.length, 1);
  assert.deepEqual(editorCalls.slice(-2), [{ title: "Answer F1" }, { title: "Answer F1" }]);
  assert.equal(notifications.filter((message) => message.includes("non-empty after trimming")).length, 1);
  assert.doesNotMatch(JSON.stringify(notifications), /private/);
});

test("cancelling an answer stops locally before assessment confirmation or submission", async () => {
  let assessmentRequests = 0;
  const confirmations: Array<{ title: string; message: string }> = [];
  const notifications: string[] = [];
  const inputs: Array<string | undefined> = ["HEAD", "first", undefined];
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      return assessableQuestions();
    },
    async assess() {
      assessmentRequests += 1;
      return completeAssessment();
    },
  };
  const context = assessmentContext(inputs, confirmations, notifications);

  await createLearnCommand(client, "0.84.3")("", context);

  assert.equal(assessmentRequests, 0);
  assert.equal(confirmations.length, 2);
  assert.match(notifications.at(-1) ?? "", /Learning questions/);
});

test("renders questions but asks for no answers when assessment is unavailable", async () => {
  const inputs: Array<string | undefined> = ["HEAD"];
  const confirmations: Array<{ title: string; message: string }> = [];
  const notifications: string[] = [];
  let assessmentRequests = 0;
  const client: LearnClient = {
    async preview() {
      return continuablePreview();
    },
    async questions() {
      const questions = assessableQuestions();
      return { ...questions, assessment: { available: false, reason: "evaluator_unavailable" } };
    },
    async assess() {
      assessmentRequests += 1;
      return completeAssessment();
    },
  };
  const context = assessmentContext(inputs, confirmations, notifications);

  await createLearnCommand(client, "0.84.3")("", context);

  assert.equal(assessmentRequests, 0);
  assert.equal(inputs.length, 0);
  assert.equal(confirmations.length, 1);
  assert.match(notifications.at(-1) ?? "", /answer assessment is not available yet/);
});

function assessmentContext(
  inputs: Array<string | undefined>,
  confirmations: Array<{ title: string; message: string }>,
  notifications: string[],
  options: {
    reviewActions?: Array<string | undefined>;
    editorCalls?: Array<{ title: string; prefill?: string }>;
    selectCalls?: Array<{ title: string; options: string[] }>;
    confirmationResults?: boolean[];
  } = {},
): LearnCommandContext {
  return {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "anthropic", id: "claude-test" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    ui: {
      async select(title, choices) {
        options.selectCalls?.push({ title, options: [...choices] });
        if (choices.includes("Continue to sharing confirmation")) {
          if (options.reviewActions !== undefined && options.reviewActions.length > 0) {
            return options.reviewActions.shift();
          }
          return "Continue to sharing confirmation";
        }
        return "Working tree against a base revision";
      },
      async input() {
        return inputs.shift();
      },
      async editor(title, prefill) {
        options.editorCalls?.push({ title, ...(prefill === undefined ? {} : { prefill }) });
        return inputs.shift();
      },
      async confirm(title, message) {
        confirmations.push({ title, message });
        if (options.confirmationResults !== undefined && options.confirmationResults.length > 0) {
          return options.confirmationResults.shift() ?? false;
        }
        return true;
      },
      notify(message) {
        notifications.push(message);
      },
    },
  };
}

function assessableQuestions() {
  return {
    schema_version: 1 as const,
    disposition: "questions" as const,
    questions: [
      { id: "Q1" as const, kind: "code_specific" as const, text: "Explain the behavior.", evidence_references: ["E001"] },
      { id: "Q2" as const, kind: "code_specific" as const, text: "Which edge matters?", evidence_references: ["E001"] },
      { id: "Q3" as const, kind: "go_backend" as const, text: "How should it be tested?", evidence_references: [] },
    ],
    assessment: {
      available: true as const,
      id: `as1-${"B".repeat(43)}`,
      expires_at: "2026-09-01T12:30:00Z",
    },
  };
}

function completeAssessment(): Extract<AssessmentResult, { label: string }> {
  return {
    turn: {
      schema_version: 1 as const,
      disposition: "complete" as const,
      follow_up: null,
      evaluations: [
        { question_id: "Q1" as const, verdict: "demonstrated" as const, feedback: "First is grounded.", evidence_references: ["E001"] },
        { question_id: "Q2" as const, verdict: "partial" as const, feedback: "Second omits one path.", evidence_references: ["E001"] },
        { question_id: "Q3" as const, verdict: "not_demonstrated" as const, feedback: "Third needs a test case.", evidence_references: [] },
      ],
    },
    label: "partial" as const,
    history: { saved: true as const, record_id: `lr1-${"C".repeat(43)}` },
  };
}

function continuablePreview(): EvidencePreviewResponse {
  return {
    protocol_version: 1,
    applied_limits: {
      max_files: 20,
      max_declarations: 100,
      max_excerpt_bytes: 131_072,
    },
    preview: {
      repository_root: "/work/repository",
      base_revision: "base-sha",
      head_revision: "WORKTREE",
      files: [
        {
          path: "internal/example/example.go",
          status: "modified",
          changed_lines: [{ start: 3, end: 3 }],
          declarations: [{
            kind: "function",
            name: "Answer",
            receiver: "",
            identity: "Answer",
            start_line: 3,
            end_line: 3,
            changed_lines: [{ start: 3, end: 3 }],
            excerpt: "func Answer() int { return 42 }",
            excerpt_truncated: false,
          }],
          omissions: [],
        },
      ],
      go_context: completeGoContext(),
      truncation: {
        truncated: false,
        omitted_files: 0,
        omitted_declarations: 0,
        omitted_excerpt_bytes: 0,
      },
    },
    continuation: {
      available: true,
      id: `pc1-${"A".repeat(43)}`,
      expires_at: "2026-09-01T12:05:00Z",
    },
  };
}
