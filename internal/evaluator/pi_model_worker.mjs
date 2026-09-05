import { open } from "node:fs/promises";
import { join } from "node:path";
import { pathToFileURL } from "node:url";

const SCHEMA_VERSION = 1;
const SUPPORTED_PI_VERSION = "0.84.3";
const MAX_REQUEST_BYTES = 3 * 1024 * 1024;
const MAX_SETTINGS_BYTES = 1024 * 1024;
const MAX_STREAM_BYTES = 2 * 1024 * 1024;
const MAX_STREAM_EVENTS = 32 * 1024;
const STREAM_EVENT_OVERHEAD_BYTES = 32;
const MAX_STREAM_ACCOUNTING_BYTES = MAX_STREAM_BYTES + MAX_STREAM_EVENTS * STREAM_EVENT_OVERHEAD_BYTES;
const MAX_TEXT_BYTES = 64 * 1024;
const MAX_INT32 = 2147483647;

const encoder = new TextEncoder();
const decoder = new TextDecoder("utf-8", { fatal: true });

function byteLength(value) {
  return encoder.encode(value).byteLength;
}

function hasExactKeys(value, expected) {
  if (value === null || typeof value !== "object" || Array.isArray(value)) return false;
  const actual = Object.keys(value).sort();
  const wanted = [...expected].sort();
  return actual.length === wanted.length && actual.every((key, index) => key === wanted[index]);
}

function parseStrictJSON(source) {
  let index = 0;
  const whitespace = /\s/u;

  function skipWhitespace() {
    while (index < source.length && whitespace.test(source[index])) index++;
  }

  function parseString() {
    const start = index++;
    while (index < source.length) {
      const current = source[index++];
      if (current === "\"") return JSON.parse(source.slice(start, index));
      if (current === "\\") {
        if (index >= source.length) throw new Error("invalid JSON");
        const escape = source[index++];
        if (escape === "u") {
          if (!/^[0-9a-fA-F]{4}$/u.test(source.slice(index, index + 4))) throw new Error("invalid JSON");
          index += 4;
        } else if (!"\"\\/bfnrt".includes(escape)) {
          throw new Error("invalid JSON");
        }
      } else if (current.charCodeAt(0) < 0x20) {
        throw new Error("invalid JSON");
      }
    }
    throw new Error("invalid JSON");
  }

  function parseValue() {
    skipWhitespace();
    const current = source[index];
    if (current === "\"") {
      parseString();
      return;
    }
    if (current === "{") {
      index++;
      skipWhitespace();
      const keys = new Set();
      if (source[index] === "}") {
        index++;
        return;
      }
      while (true) {
        skipWhitespace();
        if (source[index] !== "\"") throw new Error("invalid JSON");
        const key = parseString();
        if (keys.has(key)) throw new Error("duplicate JSON key");
        keys.add(key);
        skipWhitespace();
        if (source[index++] !== ":") throw new Error("invalid JSON");
        parseValue();
        skipWhitespace();
        const separator = source[index++];
        if (separator === "}") return;
        if (separator !== ",") throw new Error("invalid JSON");
      }
    }
    if (current === "[") {
      index++;
      skipWhitespace();
      if (source[index] === "]") {
        index++;
        return;
      }
      while (true) {
        parseValue();
        skipWhitespace();
        const separator = source[index++];
        if (separator === "]") return;
        if (separator !== ",") throw new Error("invalid JSON");
      }
    }
    const remainder = source.slice(index);
    const token = remainder.match(/^(?:true|false|null|-?(?:0|[1-9]\d*)(?:\.\d+)?(?:[eE][+-]?\d+)?)/u)?.[0];
    if (!token) throw new Error("invalid JSON");
    index += token.length;
  }

  parseValue();
  skipWhitespace();
  if (index !== source.length) throw new Error("invalid JSON");
  return JSON.parse(source);
}

function assertString(value) {
  if (typeof value !== "string" || value.length === 0) throw new Error("invalid request");
  return value;
}

function validatePaths(request) {
  return {
    sdk: assertString(request.sdk_entry),
    settings: assertString(request.settings_manager_entry),
    http: assertString(request.http_dispatcher_entry),
    attribution: assertString(request.attribution_entry),
  };
}

async function importRuntime(paths) {
  const [sdk, settingsModule, httpModule, attributionModule] = await Promise.all([
    import(pathToFileURL(paths.sdk).href),
    import(pathToFileURL(paths.settings).href),
    import(pathToFileURL(paths.http).href),
    import(pathToFileURL(paths.attribution).href),
  ]);
  if (sdk.VERSION !== SUPPORTED_PI_VERSION || typeof sdk.ModelRuntime?.create !== "function" ||
      typeof sdk.getAgentDir !== "function" || typeof settingsModule.SettingsManager?.fromStorage !== "function" ||
      typeof httpModule.applyHttpProxySettings !== "function" ||
      typeof httpModule.configureHttpDispatcher !== "function" ||
      typeof attributionModule.mergeProviderAttributionHeaders !== "function") {
    throw new Error("unsupported runtime");
  }
  return { sdk, SettingsManager: settingsModule.SettingsManager, ...httpModule, ...attributionModule };
}

async function readBoundedFile(path, maximum) {
  let handle;
  try {
    handle = await open(path, "r");
    const stat = await handle.stat();
    if (!stat.isFile() || stat.size > maximum) throw new Error("file exceeds bound");
    const bytes = Buffer.alloc(stat.size);
    let offset = 0;
    while (offset < bytes.length) {
      const { bytesRead } = await handle.read(bytes, offset, bytes.length - offset, offset);
      if (bytesRead === 0) throw new Error("short read");
      offset += bytesRead;
    }
    return decoder.decode(bytes);
  } catch (error) {
    if (error?.code === "ENOENT") return undefined;
    throw error;
  } finally {
    await handle?.close();
  }
}

function readOnlySettingsStorage(content) {
  let globalReads = 0;
  return {
    withLock(scope, callback) {
      if (scope !== "global" || globalReads !== 0) throw new Error("unexpected settings access");
      globalReads++;
      if (callback(content) !== undefined) throw new Error("settings write forbidden");
    },
    assertComplete() {
      if (globalReads !== 1) throw new Error("settings snapshot not consumed exactly once");
    },
  };
}

function validateOptionalNonNegative(value, integer = false) {
  if (value === undefined) return;
  if (typeof value !== "number" || !Number.isFinite(value) || value < 0 || (integer && !Number.isInteger(value))) {
    throw new Error("unsupported settings");
  }
}

function effectiveSettings(settingsManager) {
  const global = settingsManager.getGlobalSettings();
  if (global.httpProxy !== undefined && typeof global.httpProxy !== "string") throw new Error("unsupported settings");
  const transport = settingsManager.getTransport();
  if (!["auto", "sse", "websocket", "websocket-cached"].includes(transport)) throw new Error("unsupported settings");
  const httpIdleTimeoutMs = settingsManager.getHttpIdleTimeoutMs();
  const providerRetry = settingsManager.getProviderRetrySettings();
  const websocketConnectTimeoutMs = settingsManager.getWebSocketConnectTimeoutMs();
  const thinkingBudgets = settingsManager.getThinkingBudgets();
  validateOptionalNonNegative(httpIdleTimeoutMs);
  validateOptionalNonNegative(providerRetry.timeoutMs);
  validateOptionalNonNegative(providerRetry.maxRetries, true);
  validateOptionalNonNegative(providerRetry.maxRetryDelayMs);
  validateOptionalNonNegative(websocketConnectTimeoutMs);
  if (thinkingBudgets !== undefined) {
    if (thinkingBudgets === null || typeof thinkingBudgets !== "object" || Array.isArray(thinkingBudgets)) {
      throw new Error("unsupported settings");
    }
    for (const key of Object.keys(thinkingBudgets)) {
      if (!["minimal", "low", "medium", "high"].includes(key)) throw new Error("unsupported settings");
      validateOptionalNonNegative(thinkingBudgets[key], true);
    }
  }
  return {
    httpProxy: global.httpProxy,
    transport,
    httpIdleTimeoutMs,
    timeoutMs: providerRetry.timeoutMs ?? (httpIdleTimeoutMs === 0 ? MAX_INT32 : httpIdleTimeoutMs),
    websocketConnectTimeoutMs,
    maxRetryDelayMs: providerRetry.maxRetryDelayMs,
    thinkingBudgets,
  };
}

function validateModelSelection(model) {
  if (!hasExactKeys(model, ["provider", "id", "thinking_level"]) ||
      typeof model.provider !== "string" || typeof model.id !== "string" ||
      !["off", "minimal", "low", "medium", "high", "xhigh", "max"].includes(model.thinking_level)) {
    throw new Error("invalid model selection");
  }
}

function assistantText(message, requestedModel) {
  if (message === null || typeof message !== "object" || message.role !== "assistant" ||
      message.provider !== requestedModel.provider || message.model !== requestedModel.id ||
      message.stopReason !== "stop" || !Array.isArray(message.content)) {
    throw new Error("invalid completion");
  }
  const text = [];
  for (const item of message.content) {
    if (item === null || typeof item !== "object" || !["text", "thinking"].includes(item.type)) {
      throw new Error("tool or unknown content");
    }
    if (item.type === "text") {
      if (typeof item.text !== "string") throw new Error("invalid text");
      text.push(item.text);
    }
  }
  const result = text.join("").trim();
  if (result.length === 0 || byteLength(result) > MAX_TEXT_BYTES) throw new Error("invalid text");
  return result;
}

function accountEvent(event, current) {
  const eventCount = current.eventCount + 1;
  if (eventCount > MAX_STREAM_EVENTS) throw new Error("stream exceeds event bound");
  let contentBytes = current.contentBytes;
  if (["text_delta", "thinking_delta"].includes(event?.type)) {
    if (typeof event.delta !== "string") throw new Error("invalid content delta");
    contentBytes += byteLength(event.delta);
    if (contentBytes > MAX_STREAM_BYTES) throw new Error("stream exceeds content bound");
  }
  const accountedBytes = contentBytes + eventCount * STREAM_EVENT_OVERHEAD_BYTES;
  if (accountedBytes > MAX_STREAM_ACCOUNTING_BYTES) throw new Error("stream exceeds accounting bound");
  return {
    eventCount,
    contentBytes,
    accountedBytes,
  };
}

function contentBlock(blocks, event, expectedType) {
  if (!Number.isSafeInteger(event?.contentIndex) || event.contentIndex < 0) {
    throw new Error("invalid content index");
  }
  const block = blocks.get(event.contentIndex);
  if (!block || block.type !== expectedType || block.ended) throw new Error("invalid content order");
  return block;
}

function startContentBlock(blocks, event, type) {
  if (!Number.isSafeInteger(event?.contentIndex) || event.contentIndex !== blocks.size || blocks.has(event.contentIndex)) {
    throw new Error("invalid content start");
  }
  blocks.set(event.contentIndex, { type, chunks: [], content: undefined, ended: false });
}

function appendContentBlock(blocks, event, type) {
  contentBlock(blocks, event, type).chunks.push(event.delta);
}

function endContentBlock(blocks, event, type) {
  if (typeof event?.content !== "string") throw new Error("invalid content end");
  const block = contentBlock(blocks, event, type);
  const content = block.chunks.join("");
  if (event.content !== content) throw new Error("inconsistent content end");
  block.chunks = undefined;
  block.content = content;
  block.ended = true;
}

function validateFinalContent(message, blocks) {
  if (message === null || typeof message !== "object" || !Array.isArray(message.content) ||
      message.content.length !== blocks.size) {
    throw new Error("inconsistent final content");
  }
  for (let index = 0; index < message.content.length; index++) {
    const block = blocks.get(index);
    const item = message.content[index];
    if (!block?.ended || item === null || typeof item !== "object" || item.type !== block.type) {
      throw new Error("inconsistent final content");
    }
    const content = block.type === "text" ? item.text : item.thinking;
    if (typeof content !== "string" || content !== block.content) throw new Error("inconsistent final content");
  }
}

export async function evaluateWorkerRequest(request, testDependencies = {}) {
  const baseKeys = ["schema_version", "action", "sdk_entry", "settings_manager_entry", "http_dispatcher_entry", "attribution_entry"];
  if (request?.schema_version !== SCHEMA_VERSION || !["preflight", "evaluate"].includes(request?.action)) {
    throw new Error("invalid request");
  }
  const expected = request.action === "preflight" ? baseKeys : [...baseKeys, "system_prompt", "message", "model"];
  if (!hasExactKeys(request, expected)) throw new Error("invalid request");
  const paths = validatePaths(request);
  const runtimeModules = await importRuntime(paths);
  if (request.action === "preflight") return { schema_version: SCHEMA_VERSION, status: "ready" };

  assertString(request.system_prompt);
  assertString(request.message);
  validateModelSelection(request.model);

  const settingsText = Object.hasOwn(testDependencies, "settingsText")
    ? testDependencies.settingsText
    : await readBoundedFile(join(runtimeModules.sdk.getAgentDir(), "settings.json"), MAX_SETTINGS_BYTES);
  const storage = readOnlySettingsStorage(settingsText);
  const settingsManager = runtimeModules.SettingsManager.fromStorage(storage, { projectTrusted: false });
  storage.assertComplete();
  if (settingsManager.drainErrors().length !== 0) throw new Error("settings unavailable");
  const settings = effectiveSettings(settingsManager);
  runtimeModules.applyHttpProxySettings(settings.httpProxy);
  runtimeModules.configureHttpDispatcher(settings.httpIdleTimeoutMs);

  const signal = testDependencies.signal ?? AbortSignal.timeout(120_000);
  const runtimeOptions = {
    allowModelNetwork: false,
    refreshOnCreate: false,
    signal,
  };
  for (const key of ["credentials", "modelsPath", "modelsStore"]) {
    if (Object.hasOwn(testDependencies, key)) runtimeOptions[key] = testDependencies[key];
  }
  const runtime = await runtimeModules.sdk.ModelRuntime.create(runtimeOptions);
  const model = runtime.getModel(request.model.provider, request.model.id);
  if (!model || model.provider !== request.model.provider || model.id !== request.model.id) {
    throw new Error("model unavailable");
  }

  const options = {
    signal,
    toolChoice: "none",
    maxRetries: 0,
    maxRetryDelayMs: settings.maxRetryDelayMs,
    transport: settings.transport,
    timeoutMs: settings.timeoutMs,
    websocketConnectTimeoutMs: settings.websocketConnectTimeoutMs,
    thinkingBudgets: settings.thinkingBudgets,
    transformHeaders: async requestHeaders =>
      runtimeModules.mergeProviderAttributionHeaders(model, settingsManager, undefined, requestHeaders) ?? {},
  };
  if (request.model.thinking_level !== "off") options.reasoning = request.model.thinking_level;
  if (testDependencies.fetch !== undefined) options.fetch = testDependencies.fetch;

  const context = {
    systemPrompt: request.system_prompt,
    messages: [{ role: "user", content: request.message, timestamp: Date.now() }],
    tools: [],
  };
  testDependencies.observeRequest?.({ runtime, model, context, options });
  const stream = testDependencies.stream ?? runtime.streamSimple(model, context, options);
  let streamAccounting = { eventCount: 0, contentBytes: 0, accountedBytes: 0 };
  const contentBlocks = new Map();
  let finalMessage;
  let started = false;
  let done = false;
  for await (const event of stream) {
    streamAccounting = accountEvent(event, streamAccounting);
    switch (event?.type) {
      case "start":
        if (started || done) throw new Error("duplicate stream start");
        started = true;
        break;
      case "text_start":
        if (!started || done) throw new Error("event outside stream");
        startContentBlock(contentBlocks, event, "text");
        break;
      case "text_delta":
        if (!started || done) throw new Error("event outside stream");
        appendContentBlock(contentBlocks, event, "text");
        break;
      case "text_end":
        if (!started || done) throw new Error("event outside stream");
        endContentBlock(contentBlocks, event, "text");
        break;
      case "thinking_start":
        if (!started || done) throw new Error("event outside stream");
        startContentBlock(contentBlocks, event, "thinking");
        break;
      case "thinking_delta":
        if (!started || done) throw new Error("event outside stream");
        appendContentBlock(contentBlocks, event, "thinking");
        break;
      case "thinking_end":
        if (!started || done) throw new Error("event outside stream");
        endContentBlock(contentBlocks, event, "thinking");
        break;
      case "toolcall_start":
      case "toolcall_delta":
      case "toolcall_end":
      case "error":
        throw new Error("unsafe completion");
      case "done":
        if (!started || done || event.reason !== "stop") throw new Error("partial completion");
        validateFinalContent(event.message, contentBlocks);
        done = true;
        finalMessage = event.message;
        break;
      default:
        throw new Error("unknown completion event");
    }
  }
  if (!done) throw new Error("missing completion");
  return { schema_version: SCHEMA_VERSION, status: "ok", text: assistantText(finalMessage, request.model) };
}

async function readRequest() {
  const chunks = [];
  let total = 0;
  for await (const chunk of process.stdin) {
    total += chunk.length;
    if (total > MAX_REQUEST_BYTES) throw new Error("request exceeds bound");
    chunks.push(chunk);
  }
  const source = Buffer.concat(chunks).toString("utf8");
  if (!source.endsWith("\n") || source.endsWith("\r\n") || source.indexOf("\n") !== source.length - 1) {
    throw new Error("invalid frame");
  }
  return parseStrictJSON(source.slice(0, -1));
}

function writeResponse(response) {
  process.stdout.write(`${JSON.stringify(response)}\n`);
}

export async function workerMain() {
  try {
    writeResponse(await evaluateWorkerRequest(await readRequest()));
  } catch {
    writeResponse({ schema_version: SCHEMA_VERSION, status: "error", code: "runtime_failed" });
    process.exitCode = 1;
  }
}
