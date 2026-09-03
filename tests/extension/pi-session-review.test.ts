import assert from "node:assert/strict";
import test from "node:test";

import {
  createLearnCommand,
  EvidenceClientError,
  type AssessmentResult,
  type EvidencePreviewResponse,
  type EvidenceSelection,
  type LearnClient,
  type LearnCommandContext,
} from "../../extensions/lib/learn-command.ts";
import { completeGoContext } from "./go-context-fixture.ts";

interface SessionLearnClient extends LearnClient {
  previewPiSession(
    repository: string,
    piSessionID: string,
    selection: EvidenceSelection,
  ): Promise<EvidencePreviewResponse>;
  reviewedPiSessionIDs(
    repository: string,
    piSessionIDs: string[],
  ): Promise<{ protocol_version: 1; reviewed_pi_session_ids: string[] }>;
}

test("manual Session review projects the newest 20 IDs, filters completed IDs, and binds Git explicitly", async () => {
  const richSessions = Array.from({ length: 22 }, (_, index) => syntheticSession(`session-${String(index).padStart(2, "0")}`));
  richSessions[5] = syntheticSession("session-04");
  const listCalls: Array<[string, string]> = [];
  const reviewedCalls: Array<[string, string[]]> = [];
  const previewCalls: Array<[string, string, EvidenceSelection]> = [];
  let questionCalls = 0;
  const client: SessionLearnClient = {
    async preview() {
      throw new Error("generic preview must not be called for a Session review");
    },
    async previewPiSession(repository, piSessionID, selection) {
      previewCalls.push([repository, piSessionID, selection]);
      return previewWithContinuation();
    },
    async reviewedPiSessionIDs(repository, piSessionIDs) {
      reviewedCalls.push([repository, [...piSessionIDs]]);
      return { protocol_version: 1, reviewed_pi_session_ids: ["session-01", "session-03"] };
    },
    async questions() {
      questionCalls += 1;
      throw new Error("declining confirmation must not call the model path");
    },
  };
  const selections = ["Pi Session", "session-00", "Working tree against a base revision"];
  const selectionOptions: string[][] = [];
  const notifications: string[] = [];
  let confirmations = 0;
  let sessionDirectoryReads = 0;
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "provider", id: "model" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    sessionManager: new Proxy({
      getSessionDir() {
        sessionDirectoryReads += 1;
        return "/synthetic/sessions";
      },
    }, {
      get(target, property, receiver) {
        if (property !== "getSessionDir") {
          throw new Error(`unexpected Session manager access: ${String(property)}`);
        }
        return Reflect.get(target, property, receiver);
      },
    }),
    ui: {
      async select(_title, options) {
        selectionOptions.push([...options]);
        return selections.shift();
      },
      async input() {
        return "HEAD";
      },
      async editor() {
        throw new Error("must not edit");
      },
      async confirm() {
        confirmations += 1;
        return false;
      },
      notify(message) {
        notifications.push(message);
      },
    },
  };

  await createLearnCommand(client, "0.84.3", async (cwd, sessionDir) => {
    listCalls.push([cwd, sessionDir]);
    return richSessions;
  })("", context);

  const newestTwenty = [...new Set(richSessions.slice(0, 20).map((session) => session.id))];
  assert.deepEqual(listCalls, [["/work/repository", "/synthetic/sessions"]]);
  assert.equal(sessionDirectoryReads, 1);
  assert.deepEqual(reviewedCalls, [["/work/repository", newestTwenty]]);
  assert.deepEqual(selectionOptions[1], newestTwenty.filter((id) => id !== "session-01" && id !== "session-03"));
  assert.deepEqual(previewCalls, [[
    "/work/repository",
    "session-00",
    { kind: "working_tree", base: "HEAD" },
  ]]);
  assert.equal(confirmations, 1);
  assert.equal(questionCalls, 0);
  assert.match(notifications[0] ?? "", /Pi Session session-00/);
  assert.match(notifications[0] ?? "", /Git changeset base-sha\.\.WORKTREE/);
});

test("Session listing requires the no-argument, interactive, trusted, explicitly selected path", async () => {
  let listCalls = 0;
  const client = inertSessionClient();
  const list = async () => {
    listCalls += 1;
    return [];
  };
  const contexts: Array<{ args: string; context: LearnCommandContext }> = [
    { args: "unexpected", context: gatedContext({ hasUI: true, trusted: true }) },
    { args: "", context: gatedContext({ hasUI: false, trusted: true }) },
    { args: "", context: gatedContext({ hasUI: true, trusted: false }) },
    { args: "", context: gatedContext({ hasUI: true, trusted: true, selection: "Working tree against a base revision" }) },
  ];

  for (const entry of contexts) {
    await createLearnCommand(client, "0.84.3", list)(entry.args, entry.context);
  }

  assert.equal(listCalls, 0);
});

test("empty or invalid Session listings stop locally without a daemon or model call", async () => {
  for (const sessions of [[], [{ id: "private/session" }]]) {
    let daemonCalls = 0;
    let questionCalls = 0;
    const notifications: string[] = [];
    const client: SessionLearnClient = {
      async preview() { daemonCalls += 1; return previewWithContinuation(); },
      async previewPiSession() { daemonCalls += 1; return previewWithContinuation(); },
      async reviewedPiSessionIDs() {
        daemonCalls += 1;
        return { protocol_version: 1, reviewed_pi_session_ids: [] };
      },
      async questions() { questionCalls += 1; throw new Error("must not call questions"); },
    };

    await createLearnCommand(client, "0.84.3", async () => sessions)("", sessionContext(notifications));

    assert.equal(daemonCalls, 0);
    assert.equal(questionCalls, 0);
    assert.equal(notifications.length, 1);
  }
});

test("listing failure, unavailable history, and all-reviewed results never fall back to an unfiltered Session", async () => {
  const cases = [
    {
      list: async (): Promise<readonly unknown[]> => { throw new Error("synthetic list failure"); },
      reviewed: async (): Promise<{ protocol_version: 1; reviewed_pi_session_ids: string[] }> => {
        throw new Error("must not query");
      },
      message: /could not list Sessions/i,
    },
    {
      list: async (): Promise<readonly unknown[]> => [{ id: "session-a" }],
      reviewed: async (): Promise<{ protocol_version: 1; reviewed_pi_session_ids: string[] }> => {
        throw new EvidenceClientError("history_unavailable", "synthetic unavailable history");
      },
      message: /will not guess/i,
    },
    {
      list: async (): Promise<readonly unknown[]> => [{ id: "session-a" }],
      reviewed: async () => ({ protocol_version: 1 as const, reviewed_pi_session_ids: ["session-a"] }),
      message: /already have a completed review/i,
    },
  ];

  for (const scenario of cases) {
    let previewCalls = 0;
    let questionCalls = 0;
    const notifications: string[] = [];
    const client: SessionLearnClient = {
      async preview() { previewCalls += 1; return previewWithContinuation(); },
      async previewPiSession() { previewCalls += 1; return previewWithContinuation(); },
      reviewedPiSessionIDs: scenario.reviewed,
      async questions() { questionCalls += 1; throw new Error("must not call questions"); },
    };

    await createLearnCommand(client, "0.84.3", scenario.list)("", sessionContext(notifications));

    assert.equal(previewCalls, 0);
    assert.equal(questionCalls, 0);
    assert.match(notifications[0] ?? "", scenario.message);
  }
});

test("cancelling Session or Git selection never starts a preview or model call", async () => {
  for (const selections of [["Pi Session", undefined], ["Pi Session", "session-a", undefined]]) {
    let previewCalls = 0;
    let questionCalls = 0;
    const client: SessionLearnClient = {
      async preview() { previewCalls += 1; return previewWithContinuation(); },
      async previewPiSession() { previewCalls += 1; return previewWithContinuation(); },
      async reviewedPiSessionIDs() {
        return { protocol_version: 1, reviewed_pi_session_ids: [] };
      },
      async questions() { questionCalls += 1; throw new Error("must not call questions"); },
    };
    const pending = [...selections];
    const context = sessionContext([]);
    context.ui.select = async () => pending.shift();

    await createLearnCommand(client, "0.84.3", async () => [{ id: "session-a" }])("", context);

    assert.equal(previewCalls, 0);
    assert.equal(questionCalls, 0);
  }
});

test("Session identity stays out of the model request after the visible association is confirmed", async () => {
  const events: string[] = [];
  const questionCalls: unknown[][] = [];
  const client: SessionLearnClient = {
    async preview() { throw new Error("must not use generic preview"); },
    async previewPiSession() { return previewWithContinuation(); },
    async reviewedPiSessionIDs() {
      return { protocol_version: 1, reviewed_pi_session_ids: [] };
    },
    async questions(...args) {
      events.push("questions");
      questionCalls.push(args);
      return { schema_version: 1, disposition: "insufficient_evidence", questions: [] };
    },
  };
  const selections = ["Pi Session", "session-model-isolation", "Working tree against a base revision"];
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "provider", id: "model" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    sessionManager: { getSessionDir: () => "/synthetic/sessions" },
    ui: {
      async select() { return selections.shift(); },
      async input() { return "HEAD"; },
      async editor() { throw new Error("must not edit"); },
      async confirm() { events.push("confirm"); return true; },
      notify(message) {
        if (message.includes("User-selected association")) {
          events.push("association");
        }
      },
    },
  };

  await createLearnCommand(client, "0.84.3", async () => [{ id: "session-model-isolation" }])("", context);

  assert.deepEqual(events.slice(0, 3), ["association", "confirm", "questions"]);
  assert.deepEqual(questionCalls, [[
    `pc1-${"c".repeat(43)}`,
    { pi_version: "0.84.3", provider: "provider", id: "model", thinking_level: "off" },
  ]]);
  assert.doesNotMatch(JSON.stringify(questionCalls), /session-model-isolation/);
});

test("Session-bound review uses the same multiline editor without adding Session provenance to assessment input", async () => {
  const piSessionID = "session-answer-isolation";
  const selections = ["Pi Session", piSessionID, "Working tree against a base revision"];
  const editorResults = ["session-private\nfirst", "second", "third"];
  const editorCalls: Array<{ title: string; prefill?: string }> = [];
  const modelRequests: unknown[] = [];
  const notifications: string[] = [];
  const client: SessionLearnClient = {
    async preview() { throw new Error("must not use generic preview"); },
    async previewPiSession() { return previewWithContinuation(); },
    async reviewedPiSessionIDs() {
      return { protocol_version: 1, reviewed_pi_session_ids: [] };
    },
    async questions(...args) {
      modelRequests.push({ questions: args });
      return sessionAssessableQuestions();
    },
    async assess(...args) {
      modelRequests.push({ assessment: args });
      return sessionCompleteAssessment();
    },
  };
  const context: LearnCommandContext = {
    cwd: "/work/repository",
    hasUI: true,
    model: { provider: "provider", id: "model" },
    thinkingLevel: "off",
    isProjectTrusted: () => true,
    sessionManager: { getSessionDir: () => "/synthetic/sessions" },
    ui: {
      async select(_title, options) {
        if (options.includes("Continue to sharing confirmation")) {
          return "Continue to sharing confirmation";
        }
        return selections.shift();
      },
      async input() { return "HEAD"; },
      async editor(title, prefill) {
        editorCalls.push({ title, ...(prefill === undefined ? {} : { prefill }) });
        return editorResults.shift();
      },
      async confirm() { return true; },
      notify(message) { notifications.push(message); },
    },
  };

  await createLearnCommand(client, "0.84.3", async () => [{ id: piSessionID }])("", context);

  assert.deepEqual(editorCalls, [
    { title: "Answer Q1" },
    { title: "Answer Q2" },
    { title: "Answer Q3" },
  ]);
  assert.deepEqual(modelRequests.at(-1), {
    assessment: [
      `as1-${"d".repeat(43)}`,
      {
        stage: "initial_answers",
        answers: [
          { question_id: "Q1", text: "session-private\nfirst" },
          { question_id: "Q2", text: "second" },
          { question_id: "Q3", text: "third" },
        ],
      },
    ],
  });
  assert.doesNotMatch(JSON.stringify(modelRequests), new RegExp(piSessionID));
  assert.doesNotMatch(JSON.stringify(notifications), /session-private/);
});

function syntheticSession(id: string) {
  return {
    id,
    get path(): never { throw new Error("Session path must not be read"); },
    get cwd(): never { throw new Error("Session cwd must not be read"); },
    get name(): never { throw new Error("Session name must not be read"); },
    get parentSessionPath(): never { throw new Error("Session parent must not be read"); },
    get created(): never { throw new Error("Session creation time must not be read"); },
    get modified(): never { throw new Error("Session modification time must not be read"); },
    get messageCount(): never { throw new Error("Session message count must not be read"); },
    get firstMessage(): never { throw new Error("Session message content must not be read"); },
    get allMessagesText(): never { throw new Error("Session transcript content must not be read"); },
  };
}

function inertSessionClient(): SessionLearnClient {
  return {
    async preview() { throw new Error("must not preview"); },
    async previewPiSession() { throw new Error("must not preview"); },
    async reviewedPiSessionIDs() { throw new Error("must not query history"); },
    async questions() { throw new Error("must not call questions"); },
  };
}

function gatedContext(options: { hasUI: boolean; trusted: boolean; selection?: string }): LearnCommandContext {
  return {
    cwd: "/work/repository",
    hasUI: options.hasUI,
    isProjectTrusted: () => options.trusted,
    sessionManager: { getSessionDir: () => "/synthetic/sessions" },
    ui: {
      async select() {
        if (options.selection === undefined) {
          throw new Error("must not select");
        }
        return options.selection;
      },
      async input() { return undefined; },
      async editor() { throw new Error("must not edit"); },
      async confirm() { throw new Error("must not confirm"); },
      notify() {},
    },
  };
}

function sessionContext(notifications: string[]): LearnCommandContext {
  return {
    cwd: "/work/repository",
    hasUI: true,
    isProjectTrusted: () => true,
    sessionManager: { getSessionDir: () => "/synthetic/sessions" },
    ui: {
      async select() { return "Pi Session"; },
      async input() { throw new Error("must not request Git input"); },
      async editor() { throw new Error("must not edit"); },
      async confirm() { throw new Error("must not confirm"); },
      notify(message) { notifications.push(message); },
    },
  };
}

function previewWithContinuation(): EvidencePreviewResponse {
  return {
    protocol_version: 1,
    applied_limits: { max_files: 20, max_declarations: 100, max_excerpt_bytes: 131_072 },
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
    continuation: {
      available: true,
      id: `pc1-${"c".repeat(43)}`,
      expires_at: "2026-09-02T12:05:00Z",
    },
  };
}

function sessionAssessableQuestions() {
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
      id: `as1-${"d".repeat(43)}`,
      expires_at: "2026-09-03T12:30:00Z",
    },
  };
}

function sessionCompleteAssessment(): Extract<AssessmentResult, { label: string }> {
  return {
    turn: {
      schema_version: 1,
      disposition: "complete",
      follow_up: null,
      evaluations: [
        { question_id: "Q1", verdict: "demonstrated", feedback: "First is grounded.", evidence_references: ["E001"] },
        { question_id: "Q2", verdict: "demonstrated", feedback: "Second is grounded.", evidence_references: ["E001"] },
        { question_id: "Q3", verdict: "demonstrated", feedback: "Third is grounded.", evidence_references: [] },
      ],
    },
    label: "understood",
    history: { saved: true, record_id: `lr1-${"e".repeat(43)}` },
  };
}
