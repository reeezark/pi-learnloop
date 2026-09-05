import assert from "node:assert/strict";
import { mkdir, mkdtemp, readFile, readdir, stat, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { spawnSync } from "node:child_process";
import test from "node:test";

import { evaluateWorkerRequest } from "../../internal/evaluator/pi_model_worker.mjs";

const sdkEntry = fileURLToPath(import.meta.resolve("@earendil-works/pi-coding-agent"));
const packageRoot = dirname(dirname(sdkEntry));
const paths = {
  sdk_entry: sdkEntry,
  settings_manager_entry: join(packageRoot, "dist/core/settings-manager.js"),
  http_dispatcher_entry: join(packageRoot, "dist/core/http-dispatcher.js"),
  attribution_entry: join(packageRoot, "dist/core/provider-attribution.js"),
};
const workerPath = fileURLToPath(new URL("../../internal/evaluator/pi_model_worker.mjs", import.meta.url));

function request(overrides = {}) {
  return {
    schema_version: 1,
    action: "evaluate",
    ...paths,
    system_prompt: "Return one synthetic result.",
    message: '{"synthetic":"evidence"}',
    model: { provider: "deepseek", id: "deepseek-v4-pro", thinking_level: "high" },
    ...overrides,
  };
}

function credentials(expectedProvider = "deepseek") {
  const state = { reads: 0, writes: 0 };
  return {
    state,
    async read(provider) {
      state.reads++;
      assert.equal(provider, expectedProvider);
      return { type: "api_key", key: "synthetic-credential" };
    },
    async list() { return []; },
    async modify() { state.writes++; throw new Error("credential write forbidden"); },
    async delete() { state.writes++; throw new Error("credential write forbidden"); },
  };
}

function successfulSSE(text = '{"synthetic":"result"}', model = "deepseek-v4-pro") {
  const chunks = [
    { id: "synthetic", object: "chat.completion.chunk", model, choices: [{ index: 0, delta: { role: "assistant", content: text }, finish_reason: null }] },
    { id: "synthetic", object: "chat.completion.chunk", model, choices: [{ index: 0, delta: {}, finish_reason: "stop" }], usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 } },
  ];
  return `${chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join("")}data: [DONE]\n\n`;
}

function chunkedSSE({ reasoning = [], text = ['{"synthetic":"result"}'] } = {}, model = "deepseek-v4-pro") {
  const chunks = [...reasoning.map(reasoningContent => ({
    id: "synthetic",
    object: "chat.completion.chunk",
    model,
    choices: [{ index: 0, delta: { role: "assistant", reasoning_content: reasoningContent }, finish_reason: null }],
  })), ...text.map(content => ({
    id: "synthetic",
    object: "chat.completion.chunk",
    model,
    choices: [{ index: 0, delta: { role: "assistant", content }, finish_reason: null }],
  })), {
    id: "synthetic",
    object: "chat.completion.chunk",
    model,
    choices: [{ index: 0, delta: {}, finish_reason: "stop" }],
    usage: { prompt_tokens: 1, completion_tokens: 1, total_tokens: 2 },
  }];
  return `${chunks.map(chunk => `data: ${JSON.stringify(chunk)}\n\n`).join("")}data: [DONE]\n\n`;
}

function response(body) {
  return new Response(body, { status: 200, headers: { "content-type": "text/event-stream" } });
}

test("actual Pi 0.84.3 ModelRuntime performs one bounded no-tools turn without settings writes or real network", async () => {
  const agentDir = await mkdtemp(join(tmpdir(), "pi-learnloop-worker-"));
  const settingsPath = join(agentDir, "settings.json");
  const sessionDir = join(agentDir, "sessions");
  const sessionPath = join(sessionDir, "sentinel.jsonl");
  const settings = JSON.stringify({
    transport: "sse",
    httpIdleTimeoutMs: 1234,
    websocketConnectTimeoutMs: 456,
    retry: { provider: { timeoutMs: 789, maxRetries: 7, maxRetryDelayMs: 55 } },
    thinkingBudgets: { high: 321 },
  });
  await writeFile(settingsPath, settings, { mode: 0o600 });
  await mkdir(sessionDir, { mode: 0o700 });
  await writeFile(sessionPath, "synthetic Session sentinel\n", { mode: 0o600 });
  const before = await stat(settingsPath);
  const sessionBefore = await stat(sessionPath);
  const originalAgentDir = process.env.PI_CODING_AGENT_DIR;
  process.env.PI_CODING_AGENT_DIR = agentDir;
  const store = credentials();
  const calls = [];
  let observed;
  const fakeFetch = async (url, options) => {
    calls.push({ url: String(url), options, body: JSON.parse(options.body) });
    return response(successfulSSE());
  };
  try {
    const result = await evaluateWorkerRequest(request(), {
      credentials: store,
      modelsPath: null,
      fetch: fakeFetch,
      observeRequest: value => { observed = value; },
    });
    assert.deepEqual(result, { schema_version: 1, status: "ok", text: '{"synthetic":"result"}' });
  } finally {
    if (originalAgentDir === undefined) delete process.env.PI_CODING_AGENT_DIR;
    else process.env.PI_CODING_AGENT_DIR = originalAgentDir;
  }

  assert.equal(calls.length, 1);
  assert.equal(new URL(calls[0].url).hostname, "api.deepseek.com");
  assert.equal(calls[0].body.model, "deepseek-v4-pro");
  assert.ok(calls[0].body.tools === undefined || calls[0].body.tools.length === 0);
  assert.equal(calls[0].body.tool_choice, "none");
  assert.equal(calls[0].body.reasoning_effort, "high");
  assert.ok(calls[0].options.signal instanceof AbortSignal);
  assert.equal(observed.model.provider, "deepseek");
  assert.equal(observed.model.id, "deepseek-v4-pro");
  assert.deepEqual(observed.runtime.getRegisteredProviderIds(), []);
  assert.deepEqual(observed.context.tools, []);
  assert.equal(observed.context.systemPrompt, "Return one synthetic result.");
  assert.equal(observed.context.messages[0].content, '{"synthetic":"evidence"}');
  assert.equal(observed.options.toolChoice, "none");
  assert.equal(observed.options.maxRetries, 0);
  assert.equal(observed.options.maxRetryDelayMs, 55);
  assert.equal(observed.options.transport, "sse");
  assert.equal(observed.options.timeoutMs, 789);
  assert.equal(observed.options.websocketConnectTimeoutMs, 456);
  assert.deepEqual(observed.options.thinkingBudgets, { high: 321 });
  assert.equal(observed.options.reasoning, "high");
  assert.equal(store.state.reads, 1);
  assert.equal(store.state.writes, 0);
  assert.equal(await readFile(settingsPath, "utf8"), settings);
  const after = await stat(settingsPath);
  assert.equal(after.size, before.size);
  assert.equal(after.mtimeMs, before.mtimeMs);
  assert.equal(await readFile(sessionPath, "utf8"), "synthetic Session sentinel\n");
  const sessionAfter = await stat(sessionPath);
  assert.equal(sessionAfter.size, sessionBefore.size);
  assert.equal(sessionAfter.mtimeMs, sessionBefore.mtimeMs);
  assert.deepEqual((await readdir(agentDir)).sort(), ["sessions", "settings.json"]);
});

test("actual Pi 0.84.3 stream accounting is invariant to provider chunking", async () => {
  const uniqueReasoning = "r".repeat(2_000);
  const bodies = [
    chunkedSSE({ reasoning: [uniqueReasoning] }),
    chunkedSSE({ reasoning: [...uniqueReasoning] }),
  ];
  for (const body of bodies) {
    const result = await evaluateWorkerRequest(request(), {
      settingsText: undefined,
      credentials: credentials(),
      modelsPath: null,
      fetch: async () => response(body),
    });
    assert.deepEqual(result, { schema_version: 1, status: "ok", text: '{"synthetic":"result"}' });
  }
});

test("actual Pi 0.84.3 stream accounting rejects genuinely oversized unique content", async () => {
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: undefined,
    credentials: credentials(),
    modelsPath: null,
    fetch: async () => response(chunkedSSE({ reasoning: ["r".repeat(2 * 1024 * 1024 + 1)] })),
  }));
});

test("actual Pi 0.84.3 stream accounting rejects excessive event fragmentation", async () => {
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: undefined,
    credentials: credentials(),
    modelsPath: null,
    fetch: async () => response(chunkedSSE({ reasoning: Array.from({ length: 33_000 }, () => "r") })),
  }));
});

test("stream validation rejects content events before the stream starts", async () => {
  async function* invalidStream() {
    yield { type: "text_delta", contentIndex: 0, delta: "unsafe" };
  }
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: undefined,
    credentials: credentials(),
    modelsPath: null,
    stream: invalidStream(),
  }));
});

test("malformed global settings fail before model transport", async () => {
  let calls = 0;
  await assert.rejects(
    evaluateWorkerRequest(request(), {
      settingsText: "{not-json}",
      credentials: credentials(),
      modelsPath: null,
      fetch: async () => { calls++; return response(successfulSSE()); },
    }),
  );
  assert.equal(calls, 0);
});

test("unsupported transport settings fail before model transport", async () => {
  let calls = 0;
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: JSON.stringify({ transport: "unknown" }),
    credentials: credentials(),
    modelsPath: null,
    fetch: async () => { calls++; return response(successfulSSE()); },
  }));
  assert.equal(calls, 0);
});

test("an exact missing model fails before credentials or transport", async () => {
  const store = credentials();
  let calls = 0;
  await assert.rejects(
    evaluateWorkerRequest(request({ model: { provider: "deepseek", id: "missing-model", thinking_level: "off" } }), {
      settingsText: undefined,
      credentials: store,
      modelsPath: null,
      fetch: async () => { calls++; return response(successfulSSE()); },
    }),
  );
  assert.equal(store.state.reads, 0);
  assert.equal(calls, 0);
});

test("provider tool calls are rejected rather than executed or returned", async () => {
  const toolCall = {
    id: "synthetic",
    object: "chat.completion.chunk",
    model: "deepseek-v4-pro",
    choices: [{ index: 0, delta: { role: "assistant", tool_calls: [{ index: 0, id: "call-1", type: "function", function: { name: "unsafe", arguments: "{}" } }] }, finish_reason: null }],
  };
  const done = {
    id: "synthetic",
    object: "chat.completion.chunk",
    model: "deepseek-v4-pro",
    choices: [{ index: 0, delta: {}, finish_reason: "tool_calls" }],
  };
  const body = `data: ${JSON.stringify(toolCall)}\n\ndata: ${JSON.stringify(done)}\n\ndata: [DONE]\n\n`;
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: undefined,
    credentials: credentials(),
    modelsPath: null,
    fetch: async () => response(body),
  }));
});

for (const failure of [
  {
    name: "length-limited completion",
    body: () => {
      const chunk = { id: "synthetic", object: "chat.completion.chunk", model: "deepseek-v4-pro", choices: [{ index: 0, delta: { role: "assistant", content: "partial" }, finish_reason: "length" }] };
      return response(`data: ${JSON.stringify(chunk)}\n\ndata: [DONE]\n\n`);
    },
  },
  {
    name: "provider error completion",
    body: () => new Response('{"error":{"message":"synthetic secret"}}', { status: 500, headers: { "content-type": "application/json" } }),
  },
  {
    name: "stream byte overflow",
    body: () => response(successfulSSE("x".repeat(2 * 1024 * 1024))),
  },
  {
    name: "assistant-text byte overflow",
    body: () => response(successfulSSE("x".repeat(64 * 1024 + 1))),
  },
]) {
  test(`${failure.name} is rejected without output repair or retry`, async () => {
    let calls = 0;
    const store = credentials();
    await assert.rejects(evaluateWorkerRequest(request(), {
      settingsText: undefined,
      credentials: store,
      modelsPath: null,
      fetch: async () => { calls++; return failure.body(); },
    }));
    assert.equal(calls, 1);
    assert.equal(store.state.reads, 1);
    assert.equal(store.state.writes, 0);
  });
}

test("settings and Session sentinels survive malformed settings, provider failure, timeout, and cancellation", async () => {
  const originalAgentDir = process.env.PI_CODING_AGENT_DIR;
  try {
    for (const scenario of ["malformed settings", "provider failure", "timeout", "cancellation"]) {
      const agentDir = await mkdtemp(join(tmpdir(), "pi-learnloop-worker-failure-"));
      const settingsPath = join(agentDir, "settings.json");
      const sessionDir = join(agentDir, "sessions");
      const sessionPath = join(sessionDir, "sentinel.jsonl");
      const settings = scenario === "malformed settings" ? "{not-json}" : "{}";
      const session = `synthetic ${scenario} Session sentinel\n`;
      await writeFile(settingsPath, settings, { mode: 0o600 });
      await mkdir(sessionDir, { mode: 0o700 });
      await writeFile(sessionPath, session, { mode: 0o600 });
      const settingsBefore = await stat(settingsPath);
      const sessionBefore = await stat(sessionPath);
      process.env.PI_CODING_AGENT_DIR = agentDir;

      const controller = new AbortController();
      let signal = controller.signal;
      let fakeFetch;
      if (scenario === "provider failure") {
        fakeFetch = async () => new Response('{"error":{"message":"synthetic"}}', {
          status: 500,
          headers: { "content-type": "application/json" },
        });
      } else if (scenario === "timeout") {
        signal = AbortSignal.timeout(5);
        fakeFetch = async (_url, options) => new Promise((resolve, reject) => {
          const abort = () => reject(options.signal.reason ?? new Error("aborted"));
          if (options.signal.aborted) abort();
          else options.signal.addEventListener("abort", abort, { once: true });
        });
      } else {
        fakeFetch = async () => response(successfulSSE());
      }
      if (scenario === "cancellation") controller.abort();

      await assert.rejects(evaluateWorkerRequest(request(), {
        credentials: credentials(),
        modelsPath: null,
        signal,
        fetch: fakeFetch,
      }));
      assert.equal(await readFile(settingsPath, "utf8"), settings, scenario);
      assert.equal(await readFile(sessionPath, "utf8"), session, scenario);
      const settingsAfter = await stat(settingsPath);
      const sessionAfter = await stat(sessionPath);
      assert.equal(settingsAfter.size, settingsBefore.size, scenario);
      assert.equal(settingsAfter.mtimeMs, settingsBefore.mtimeMs, scenario);
      assert.equal(sessionAfter.size, sessionBefore.size, scenario);
      assert.equal(sessionAfter.mtimeMs, sessionBefore.mtimeMs, scenario);
      assert.deepEqual((await readdir(agentDir)).sort(), ["sessions", "settings.json"], scenario);
    }
  } finally {
    if (originalAgentDir === undefined) delete process.env.PI_CODING_AGENT_DIR;
    else process.env.PI_CODING_AGENT_DIR = originalAgentDir;
  }
});

test("an aborted call stops before transport and does not retry", async () => {
  const controller = new AbortController();
  controller.abort();
  let calls = 0;
  await assert.rejects(evaluateWorkerRequest(request(), {
    settingsText: undefined,
    credentials: credentials(),
    modelsPath: null,
    signal: controller.signal,
    fetch: async () => { calls++; return response(successfulSSE()); },
  }));
  assert.equal(calls, 0);
});

test("proxy configuration stays process-local and Pi attribution headers are projected without a Session ID", async () => {
  const originalHTTP = process.env.HTTP_PROXY;
  const originalHTTPS = process.env.HTTPS_PROXY;
  delete process.env.HTTP_PROXY;
  delete process.env.HTTPS_PROXY;
  const calls = [];
  let observed;
  try {
    const model = "aion-labs/aion-2.0";
    await evaluateWorkerRequest(request({
      model: { provider: "openrouter", id: model, thinking_level: "off" },
    }), {
      settingsText: JSON.stringify({ httpProxy: "http://synthetic-proxy.invalid", enableInstallTelemetry: true }),
      credentials: credentials("openrouter"),
      modelsPath: null,
      fetch: async (url, options) => { calls.push({ url, options }); return response(successfulSSE("ok", model)); },
      observeRequest: value => { observed = value; },
    });
    assert.equal(process.env.HTTP_PROXY, "http://synthetic-proxy.invalid");
    assert.equal(process.env.HTTPS_PROXY, "http://synthetic-proxy.invalid");
    assert.equal(observed.options.reasoning, undefined);
    const projected = await observed.options.transformHeaders({ "X-Synthetic": "present" });
    assert.equal(projected["X-Synthetic"], "present");
    assert.equal(projected["X-OpenRouter-Title"], "pi");
    assert.equal(projected["x-opencode-session"], undefined);
    assert.equal(calls.length, 1);
    const sentHeaders = new Headers(calls[0].options.headers);
    assert.equal(sentHeaders.get("x-openrouter-title"), "pi");
    assert.equal(sentHeaders.get("x-opencode-session"), null);
  } finally {
    if (originalHTTP === undefined) delete process.env.HTTP_PROXY;
    else process.env.HTTP_PROXY = originalHTTP;
    if (originalHTTPS === undefined) delete process.env.HTTPS_PROXY;
    else process.env.HTTPS_PROXY = originalHTTPS;
  }
});

test("preflight imports the pinned actual SDK without settings credentials or model access", async () => {
  assert.deepEqual(await evaluateWorkerRequest({ schema_version: 1, action: "preflight", ...paths }), {
    schema_version: 1,
    status: "ready",
  });
});

test("the actual worker accepts exactly one LF-framed preflight through private pipes", async () => {
  const source = `${await readFile(workerPath, "utf8")}\nawait workerMain();\n`;
  const input = `${JSON.stringify({ schema_version: 1, action: "preflight", ...paths })}\n`;
  const child = spawnSync(process.execPath, ["--input-type=module", "--eval", source], {
    cwd: tmpdir(), input, encoding: "utf8", timeout: 10_000, maxBuffer: 3 * 1024 * 1024,
  });
  assert.equal(child.status, 0);
  assert.equal(child.signal, null);
  assert.equal(child.stderr, "");
  assert.deepEqual(JSON.parse(child.stdout), { schema_version: 1, status: "ready" });
  assert.equal(child.stdout.endsWith("\n"), true);
  assert.equal(child.stdout.slice(0, -1).includes("\n"), false);
});

for (const malformed of [
  '{"schema_version":1,"schema_version":1,"action":"preflight"}\n',
  `${JSON.stringify({ schema_version: 1, action: "preflight", ...paths })}\n{}\n`,
  `${JSON.stringify({ schema_version: 1, action: "preflight", ...paths })}\r\n`,
]) {
  test("the worker rejects malformed or duplicate private framing with only a safe error", async () => {
    const source = `${await readFile(workerPath, "utf8")}\nawait workerMain();\n`;
    const child = spawnSync(process.execPath, ["--input-type=module", "--eval", source], {
      cwd: tmpdir(), input: malformed, encoding: "utf8", timeout: 10_000, maxBuffer: 3 * 1024 * 1024,
    });
    assert.equal(child.status, 1);
    assert.equal(child.stderr, "");
    assert.deepEqual(JSON.parse(child.stdout), { schema_version: 1, status: "error", code: "runtime_failed" });
  });
}
