import assert from "node:assert/strict";
import { once } from "node:events";
import { chmod, mkdtemp, writeFile } from "node:fs/promises";
import { createServer, type IncomingMessage, type ServerResponse } from "node:http";
import { tmpdir } from "node:os";
import { join } from "node:path";
import test from "node:test";

import { DaemonEvidenceClient } from "../../extensions/lib/daemon-client.ts";
import { EvidenceClientError } from "../../extensions/lib/learn-command.ts";

test("discovers the current daemon before sending an authenticated preview request", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "A".repeat(22);
  const token = "B".repeat(43);
  const requests: Array<{ method?: string; url?: string; authorization?: string; body: string }> = [];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      requests.push({ method: request.method, url: request.url, body: "" });
      await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    const body = await readBody(request);
    requests.push({
      method: request.method,
      url: request.url,
      authorization: request.headers.authorization,
      body,
    });
    writeJSON(response, 200, emptyPreview());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");

  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const previousHTTPProxy = process.env.HTTP_PROXY;
  const previousHTTPSProxy = process.env.HTTPS_PROXY;
  const previousProxyMode = process.env.NODE_USE_ENV_PROXY;
  process.env.HTTP_PROXY = "http://127.0.0.1:1";
  process.env.HTTPS_PROXY = "http://127.0.0.1:1";
  process.env.NODE_USE_ENV_PROXY = "1";
  t.after(() => {
    restoreEnvironment("HTTP_PROXY", previousHTTPProxy);
    restoreEnvironment("HTTPS_PROXY", previousHTTPSProxy);
    restoreEnvironment("NODE_USE_ENV_PROXY", previousProxyMode);
  });

  const result = await new DaemonEvidenceClient({ runtimeDir }).preview("/work/repository", {
    kind: "commit_range",
    base: "base-sha",
    head: "head-sha",
  });

  assert.deepEqual(result, emptyPreview());
  assert.deepEqual(requests, [
    { method: "GET", url: "/v1/status", body: "" },
    {
      method: "POST",
      url: "/v1/evidence-previews",
      authorization: `PiLearnLoop ${token}`,
      body: JSON.stringify({
        repository: "/work/repository",
        selection: { kind: "commit_range", base: "base-sha", head: "head-sha" },
      }),
    },
  ]);
});

test("rejects a runtime descriptor that is not an exact IPv4 loopback URL", async () => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  await writeProtectedFile(join(runtimeDir, "daemon.token"), "B".repeat(43));
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: "A".repeat(22),
      pid: process.pid,
      base_url: "http://localhost:49152",
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).preview("/work/repository", {
      kind: "working_tree",
      base: "HEAD",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "invalid_runtime_state",
  );
});

test("re-reads discovery once after authentication changes and then stops", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "C".repeat(22);
  const token = "D".repeat(43);
  let statusRequests = 0;
  let previewRequests = 0;
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      statusRequests += 1;
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    previewRequests += 1;
    await readBody(request);
    writeJSON(response, 401, {
      protocol_version: 1,
      error: { code: "unauthorized", message: "authentication required" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");

  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).preview("/work/repository", {
      kind: "working_tree",
      base: "HEAD",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "unauthorized",
  );
  assert.equal(statusRequests, 2);
  assert.equal(previewRequests, 2);
});

test("preserves the daemon invalid-revision code for the command layer", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "E".repeat(22);
  const token = "F".repeat(43);
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    writeJSON(response, 422, {
      protocol_version: 1,
      error: { code: "invalid_revision", message: "revision cannot be resolved" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).preview("/work/repository", {
      kind: "commit_range",
      base: "missing",
      head: "HEAD",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "invalid_revision",
  );
});

test("rejects a malformed success payload before the command renders it", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "G".repeat(22);
  const token = "H".repeat(43);
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    writeJSON(response, 200, {
      protocol_version: 1,
      applied_limits: { max_files: 20, max_declarations: 100, max_excerpt_bytes: 131_072 },
      preview: {
        repository_root: "/work/repository",
        base_revision: "base-sha",
        head_revision: "head-sha",
        files: [{}],
        truncation: {
          truncated: false,
          omitted_files: 0,
          omitted_declarations: 0,
          omitted_excerpt_bytes: 0,
        },
      },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).preview("/work/repository", {
      kind: "working_tree",
      base: "HEAD",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
  );
});

test("sends one authenticated continuation request with only model metadata", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "J".repeat(22);
  const token = "K".repeat(43);
  const requests: Array<{ method?: string; url?: string; authorization?: string; body: string }> = [];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      requests.push({ method: request.method, url: request.url, body: "" });
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    const body = await readBody(request);
    requests.push({
      method: request.method,
      url: request.url,
      authorization: request.headers.authorization,
      body,
    });
    writeJSON(response, 200, validQuestionSetResponse());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const result = await new DaemonEvidenceClient({ runtimeDir }).questions(`pc1-${"L".repeat(43)}`, {
    pi_version: "0.84.3",
    provider: "anthropic",
    id: "claude-test",
    thinking_level: "off",
  });

  assert.equal(result.disposition, "questions");
  assert.equal(result.questions.length, 3);
  assert.deepEqual(requests, [
    { method: "GET", url: "/v1/status", body: "" },
    {
      method: "POST",
      url: "/v1/question-sets",
      authorization: `PiLearnLoop ${token}`,
      body: JSON.stringify({
        continuation_id: `pc1-${"L".repeat(43)}`,
        pi_version: "0.84.3",
        model: { provider: "anthropic", id: "claude-test", thinking_level: "off" },
      }),
    },
  ]);
});

test("never retries a consumed or unavailable continuation", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "M".repeat(22);
  const token = "N".repeat(43);
  let statusRequests = 0;
  let continuationRequests = 0;
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      statusRequests += 1;
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    continuationRequests += 1;
    await readBody(request);
    writeJSON(response, 409, {
      protocol_version: 1,
      error: { code: "continuation_unavailable", message: "continuation is unavailable" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).questions(`pc1-${"P".repeat(43)}`, {
      pi_version: "0.84.3",
      provider: "anthropic",
      id: "claude-test",
      thinking_level: "off",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "continuation_unavailable",
  );
  assert.equal(statusRequests, 1);
  assert.equal(continuationRequests, 1);
});

test("rejects an invalid question shape before rendering", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "Q".repeat(22);
  const token = "R".repeat(43);
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    const invalid = validQuestionSetResponse();
    invalid.question_set.questions[2] = {
      id: "Q3",
      kind: "code_specific",
      text: "Wrong kind",
      evidence_references: [],
    };
    writeJSON(response, 200, invalid);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).questions(`pc1-${"S".repeat(43)}`, {
      pi_version: "0.84.3",
      provider: "anthropic",
      id: "claude-test",
      thinking_level: "off",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
  );
});

test("carries an assessment descriptor and sends one strict initial-answer request", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "T".repeat(22);
  const token = "U".repeat(43);
  const requests: Array<{ url?: string; authorization?: string; body: string }> = [];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    const body = await readBody(request);
    requests.push({ url: request.url, authorization: request.headers.authorization, body });
    if (request.url === "/v1/question-sets") {
      writeJSON(response, 200, {
        ...validQuestionSetResponse(),
        assessment: {
          available: true,
          id: `as1-${"V".repeat(43)}`,
          expires_at: "2026-09-01T12:30:00Z",
        },
      });
      return;
    }
    writeJSON(response, 200, validCompleteAssessmentResponse());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const client = new DaemonEvidenceClient({ runtimeDir });
  const questions = await client.questions(`pc1-${"W".repeat(43)}`, {
    pi_version: "0.84.3",
    provider: "anthropic",
    id: "claude-test",
    thinking_level: "off",
  });
  assert.deepEqual(questions.assessment, {
    available: true,
    id: `as1-${"V".repeat(43)}`,
    expires_at: "2026-09-01T12:30:00Z",
  });

  const result = await client.assess(`as1-${"V".repeat(43)}`, {
    stage: "initial_answers",
    answers: [
      { question_id: "Q1", text: "first" },
      { question_id: "Q2", text: "second" },
      { question_id: "Q3", text: "third" },
    ],
  });

  assert.equal(result.turn.disposition, "complete");
  assert.equal("label" in result ? result.label : undefined, "partial");
  assert.deepEqual("history" in result ? result.history : undefined, {
    saved: true,
    record_id: `lr1-${"d".repeat(43)}`,
  });
  assert.deepEqual(requests[1], {
    url: "/v1/assessment-turns",
    authorization: `PiLearnLoop ${token}`,
    body: JSON.stringify({
      assessment_id: `as1-${"V".repeat(43)}`,
      stage: "initial_answers",
      answers: [
        { question_id: "Q1", text: "first" },
        { question_id: "Q2", text: "second" },
        { question_id: "Q3", text: "third" },
      ],
    }),
  });
});

test("never retries an unavailable assessment", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "X".repeat(22);
  const token = "Y".repeat(43);
  let statusRequests = 0;
  let assessmentRequests = 0;
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      statusRequests += 1;
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    assessmentRequests += 1;
    await readBody(request);
    writeJSON(response, 409, {
      protocol_version: 1,
      error: { code: "assessment_unavailable", message: "assessment is unavailable" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).assess(`as1-${"Z".repeat(43)}`, {
      stage: "follow_up_answer",
      follow_up_id: "F1",
      answer: "one answer",
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "assessment_unavailable",
  );
  assert.equal(statusRequests, 1);
  assert.equal(assessmentRequests, 1);
});

test("rejects malformed assessment feedback before rendering", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "a".repeat(22);
  const token = "b".repeat(43);
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    const invalid = validCompleteAssessmentResponse();
    invalid.assessment_turn.evaluations[0].evidence_references = ["E999", "E999"];
    writeJSON(response, 200, invalid);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).assess(`as1-${"c".repeat(43)}`, {
      stage: "initial_answers",
      answers: [
        { question_id: "Q1", text: "first" },
        { question_id: "Q2", text: "second" },
        { question_id: "Q3", text: "third" },
      ],
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
  );
});

test("rejects a malformed complete-assessment history descriptor", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "e".repeat(22);
  const token = "f".repeat(43);
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    const invalid = validCompleteAssessmentResponse();
    invalid.history = { saved: false, reason: "disk_full", record_id: `lr1-${"g".repeat(43)}` } as never;
    writeJSON(response, 200, invalid);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(join(runtimeDir, "daemon.json"), JSON.stringify({
    schema_version: 1,
    protocol_version: 1,
    instance_id: instanceID,
    pid: process.pid,
    base_url: `http://127.0.0.1:${address.port}`,
    started_at: new Date().toISOString(),
  }));

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).assess(`as1-${"h".repeat(43)}`, {
      stage: "initial_answers",
      answers: [
        { question_id: "Q1", text: "first" },
        { question_id: "Q2", text: "second" },
        { question_id: "Q3", text: "third" },
      ],
    }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
  );
});

test("sends one authenticated bounded history query and validates the exact response", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "H".repeat(22);
  const token = "I".repeat(43);
  const requests: Array<{ method?: string; url?: string; authorization?: string; body: string }> = [];
  const expected = validHistoryResponse();
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      requests.push({ method: request.method, url: request.url, body: "" });
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    const body = await readBody(request);
    requests.push({
      method: request.method,
      url: request.url,
      authorization: request.headers.authorization,
      body,
    });
    writeJSON(response, 200, expected);
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const result = await new DaemonEvidenceClient({ runtimeDir }).history("/work/repository", 20);

  assert.deepEqual(result, expected);
  assert.deepEqual(requests, [
    { method: "GET", url: "/v1/status", body: "" },
    {
      method: "POST",
      url: "/v1/learning-history-queries",
      authorization: `PiLearnLoop ${token}`,
      body: JSON.stringify({ repository: "/work/repository", limit: 20 }),
    },
  ]);
});

test("rejects a history response that adds source-bearing repository metadata", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "J".repeat(22);
  const token = "K".repeat(43);
  const valid = validHistoryResponse();
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    writeJSON(response, 200, {
      ...valid,
      records: [{ ...valid.records[0], canonical_root: "/private/source-bearing-repository" }],
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).history("/work/repository", 20),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
  );
});

test("preserves history-unavailable and never retries the query", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "L".repeat(22);
  const token = "M".repeat(43);
  let queryRequests = 0;
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    queryRequests += 1;
    await readBody(request);
    writeJSON(response, 503, {
      protocol_version: 1,
      error: { code: "history_unavailable", message: "local learning history is unavailable" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).history("/work/repository", 20),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "history_unavailable",
  );
  assert.equal(queryRequests, 1);
});

test("sends a Session-bound preview through the independent authenticated route", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "N".repeat(22);
  const token = "O".repeat(43);
  const requests: Array<{ url?: string; authorization?: string; body: string }> = [];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    requests.push({
      url: request.url,
      authorization: request.headers.authorization,
      body: await readBody(request),
    });
    writeJSON(response, 200, emptyPreview());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const result = await new DaemonEvidenceClient({ runtimeDir }).previewPiSession(
    "/work/repository",
    "session-123",
    { kind: "working_tree", base: "HEAD" },
  );

  assert.deepEqual(result, emptyPreview());
  assert.deepEqual(requests, [{
    url: "/v1/pi-session-evidence-previews",
    authorization: `PiLearnLoop ${token}`,
    body: JSON.stringify({
      repository: "/work/repository",
      pi_session_id: "session-123",
      selection: { kind: "working_tree", base: "HEAD" },
    }),
  }]);
});

test("sends one bounded Session review query and validates its ID-only response", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "P".repeat(22);
  const token = "Q".repeat(43);
  const requests: Array<{ url?: string; authorization?: string; body: string }> = [];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    requests.push({
      url: request.url,
      authorization: request.headers.authorization,
      body: await readBody(request),
    });
    writeJSON(response, 200, {
      protocol_version: 1,
      reviewed_pi_session_ids: ["session-c", "session-a"],
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  const result = await new DaemonEvidenceClient({ runtimeDir }).reviewedPiSessionIDs(
    "/work/repository",
    ["session-c", "session-b", "session-a"],
  );

  assert.deepEqual(result, {
    protocol_version: 1,
    reviewed_pi_session_ids: ["session-c", "session-a"],
  });
  assert.deepEqual(requests, [{
    url: "/v1/pi-session-review-queries",
    authorization: `PiLearnLoop ${token}`,
    body: JSON.stringify({
      repository: "/work/repository",
      pi_session_ids: ["session-c", "session-b", "session-a"],
    }),
  }]);
});

test("rejects non-exact, invalid, duplicated, unrelated, or reordered Session review responses", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "R".repeat(22);
  const token = "S".repeat(43);
  const responses: unknown[] = [
    { protocol_version: 1, reviewed_pi_session_ids: ["session-a"], extra: true },
    { protocol_version: 1, reviewed_pi_session_ids: ["private/session"] },
    { protocol_version: 1, reviewed_pi_session_ids: ["session-a", "session-a"] },
    { protocol_version: 1, reviewed_pi_session_ids: ["session-z"] },
    { protocol_version: 1, reviewed_pi_session_ids: ["session-c", "session-a"] },
  ];
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    await readBody(request);
    writeJSON(response, 200, responses.shift());
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  for (let index = 0; index < 5; index += 1) {
    await assert.rejects(
      new DaemonEvidenceClient({ runtimeDir }).reviewedPiSessionIDs(
        "/work/repository",
        ["session-a", "session-b", "session-c"],
      ),
      (error: unknown) => error instanceof EvidenceClientError && error.code === "protocol_mismatch",
    );
  }
});

test("rejects unbounded or invalid Session query inputs before daemon discovery", async () => {
  const client = new DaemonEvidenceClient({ runtimeDir: "/missing/runtime" });
  const invalidCandidates = [
    [],
    Array.from({ length: 21 }, (_, index) => `session-${index}`),
    ["same", "same"],
    ["private/session"],
    ["a".repeat(129)],
  ];

  for (const candidates of invalidCandidates) {
    await assert.rejects(
      client.reviewedPiSessionIDs("/work/repository", candidates),
      (error: unknown) => error instanceof EvidenceClientError && error.code === "invalid_request",
    );
  }
  await assert.rejects(
    client.previewPiSession("/work/repository", "private/session", { kind: "working_tree", base: "HEAD" }),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "invalid_request",
  );
});

test("preserves Session history unavailability and never retries the review query", async (t) => {
  const runtimeDir = await mkdtemp(join(tmpdir(), "pi-learnloop-client-"));
  await chmod(runtimeDir, 0o700);
  const instanceID = "T".repeat(22);
  const token = "U".repeat(43);
  let statusRequests = 0;
  let queryRequests = 0;
  const server = createServer(async (request, response) => {
    if (request.url === "/v1/status") {
      statusRequests += 1;
      writeJSON(response, 200, { protocol_version: 1, instance_id: instanceID, status: "ready" });
      return;
    }
    queryRequests += 1;
    await readBody(request);
    writeJSON(response, 503, {
      protocol_version: 1,
      error: { code: "history_unavailable", message: "local learning history is unavailable" },
    });
  });
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  t.after(() => server.close());
  const address = server.address();
  assert.ok(address && typeof address !== "string");
  await writeProtectedFile(join(runtimeDir, "daemon.token"), token);
  await writeProtectedFile(
    join(runtimeDir, "daemon.json"),
    JSON.stringify({
      schema_version: 1,
      protocol_version: 1,
      instance_id: instanceID,
      pid: process.pid,
      base_url: `http://127.0.0.1:${address.port}`,
      started_at: new Date().toISOString(),
    }),
  );

  await assert.rejects(
    new DaemonEvidenceClient({ runtimeDir }).reviewedPiSessionIDs("/work/repository", ["session-a"]),
    (error: unknown) => error instanceof EvidenceClientError && error.code === "history_unavailable",
  );
  assert.equal(statusRequests, 1);
  assert.equal(queryRequests, 1);
});

async function writeProtectedFile(path: string, content: string): Promise<void> {
  await writeFile(path, content, { mode: 0o600 });
  await chmod(path, 0o600);
}

async function readBody(request: IncomingMessage): Promise<string> {
  const chunks: Buffer[] = [];
  for await (const chunk of request) {
    chunks.push(Buffer.from(chunk));
  }
  return Buffer.concat(chunks).toString("utf8");
}

function writeJSON(response: ServerResponse, status: number, value: unknown): void {
  response.writeHead(status, { "Content-Type": "application/json", "Cache-Control": "no-store" });
  response.end(JSON.stringify(value));
}

function emptyPreview() {
  return {
    protocol_version: 1 as const,
    applied_limits: {
      max_files: 20,
      max_declarations: 100,
      max_excerpt_bytes: 131_072,
    },
    preview: {
      repository_root: "/work/repository",
      base_revision: "base-sha",
      head_revision: "head-sha",
      files: [],
      truncation: {
        truncated: false,
        omitted_files: 0,
        omitted_declarations: 0,
        omitted_excerpt_bytes: 0,
      },
    },
  };
}

function validQuestionSetResponse() {
  return {
    protocol_version: 1,
    question_set: {
      schema_version: 1,
      disposition: "questions",
      questions: [
        { id: "Q1", kind: "code_specific", text: "Explain the changed behavior?", evidence_references: ["E001"] },
        { id: "Q2", kind: "code_specific", text: "Which edge case matters?", evidence_references: ["E001"] },
        { id: "Q3", kind: "go_backend", text: "How would table-driven tests help?", evidence_references: [] },
      ],
    },
  };
}

function validCompleteAssessmentResponse() {
  return {
    protocol_version: 1,
    assessment_turn: {
      schema_version: 1,
      disposition: "complete",
      follow_up: null,
      evaluations: [
        { question_id: "Q1", verdict: "demonstrated", feedback: "First is grounded.", evidence_references: ["E001"] },
        { question_id: "Q2", verdict: "partial", feedback: "Second omits one path.", evidence_references: ["E001"] },
        { question_id: "Q3", verdict: "not_demonstrated", feedback: "Third needs a test case.", evidence_references: [] },
      ],
    },
    label: "partial",
    history: { saved: true, record_id: `lr1-${"d".repeat(43)}` },
  };
}

function validHistoryResponse() {
  return {
    protocol_version: 1 as const,
    records: [
      {
        record_id: `lr1-${"j".repeat(43)}`,
        started_at: "2026-09-01T12:00:00Z",
        finished_at: "2026-09-01T12:01:00Z",
        status: "complete" as const,
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
        label: "understood" as const,
        outcomes: [
          { question_id: "Q1" as const, question_kind: "code_specific" as const, verdict: "demonstrated" as const },
          { question_id: "Q2" as const, question_kind: "code_specific" as const, verdict: "demonstrated" as const },
          { question_id: "Q3" as const, question_kind: "go_backend" as const, verdict: "demonstrated" as const },
        ],
      },
    ],
  };
}

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
