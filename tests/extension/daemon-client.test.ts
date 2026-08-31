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

function restoreEnvironment(name: string, value: string | undefined): void {
  if (value === undefined) {
    delete process.env[name];
  } else {
    process.env[name] = value;
  }
}
