import assert from "node:assert/strict";
import test from "node:test";

import {
  createLearnCommand,
  EvidenceClientError,
  type EvidencePreviewClient,
  type EvidencePreviewResponse,
  formatPreview,
  type LearnCommandContext,
} from "../../extensions/lib/learn-command.ts";

test("manual commit range shows the selected evidence preview", async () => {
  const requests: Parameters<EvidencePreviewClient["preview"]>[] = [];
  const client: EvidencePreviewClient = {
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
  const requests: Parameters<EvidencePreviewClient["preview"]>[] = [];
  const client: EvidencePreviewClient = {
    async preview(repository, selection) {
      requests.push([repository, selection]);
      throw new EvidenceClientError("daemon_unavailable", "connect ECONNREFUSED");
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
  const client: EvidencePreviewClient = {
    async preview() {
      requests += 1;
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
  const client: EvidencePreviewClient = {
    async preview() {
      throw new EvidenceClientError("unauthorized", "authentication required");
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
  const client: EvidencePreviewClient = {
    async preview() {
      throw new EvidenceClientError("invalid_revision", "revision cannot be resolved");
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
