import assert from "node:assert/strict";
import test from "node:test";

import {
  createLearnCommand,
  EvidenceClientError,
  type LearnClient,
  type EvidencePreviewResponse,
  formatPreview,
  type LearnCommandContext,
} from "../../extensions/lib/learn-command.ts";

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
          truncation: {
            truncated: false,
            omitted_files: 0,
            omitted_declarations: 0,
            omitted_excerpt_bytes: 0,
          },
        },
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
  assert.match(notifications[0]?.message ?? "", /Truncation: none/);
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
      truncation: {
        truncated: false,
        omitted_files: 0,
        omitted_declarations: 0,
        omitted_excerpt_bytes: 0,
      },
    },
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
  assert.match(confirmations[0]?.message ?? "", /selected excerpts shown above/);
  assert.match(confirmations[0]?.message ?? "", /configured model/);
  assert.match(confirmations[0]?.message ?? "", /provider cost/);
  assert.match(confirmations[0]?.message ?? "", /transport may retry/);
  assert.match(notifications[0] ?? "", /Evidence preview/);
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
