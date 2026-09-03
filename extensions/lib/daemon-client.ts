import { createHash } from "node:crypto";
import { readFile, lstat } from "node:fs/promises";
import { request as httpRequest } from "node:http";
import { homedir } from "node:os";
import { join } from "node:path";

import {
  EvidenceClientError,
  type EvidenceClientErrorCode,
  type EvaluatorSelection,
  type LearnClient,
  type AssessmentResult,
  type AssessmentSubmission,
  type EvidencePreviewResponse,
  type EvidenceSelection,
  type GoContextBuild,
  type GoContextItem,
  type GoContextPreview,
  type LearningHistoryRecord,
  type LearningHistoryResponse,
  type PiSessionReviewResponse,
  type QuestionSet,
} from "./learn-command.ts";

const STATUS_TIMEOUT_MS = 2_000;
const PREVIEW_TIMEOUT_MS = 35_000;
const QUESTION_SET_TIMEOUT_MS = 130_000;
const ASSESSMENT_TIMEOUT_MS = 130_000;
const HISTORY_QUERY_TIMEOUT_MS = 35_000;
const MAX_RESPONSE_BYTES = 1024 * 1024;
const SERVER_ERROR_CODES = new Set<EvidenceClientErrorCode>([
  "unauthorized",
  "invalid_request",
  "invalid_repository",
  "invalid_revision",
  "forbidden",
  "source_unavailable",
  "invalid_source",
  "analysis_failed",
  "deadline_exceeded",
  "internal_error",
  "continuation_unavailable",
  "assessment_unavailable",
  "evaluator_failed",
  "evaluator_invalid_output",
  "evaluator_unavailable",
  "evaluator_timeout",
  "history_unavailable",
]);

interface RuntimeDescriptor {
  schema_version: 1;
  protocol_version: 1;
  instance_id: string;
  pid: number;
  base_url: string;
  started_at: string;
}

interface ClientOptions {
  runtimeDir?: string;
}

interface HTTPResult {
  statusCode: number;
  contentType: string;
  body: unknown;
}

export class DaemonEvidenceClient implements LearnClient {
  private readonly runtimeDir: string;

  constructor(options: ClientOptions = {}) {
    this.runtimeDir = options.runtimeDir ?? join(homedir(), "Library", "Application Support", "pi-learnloop", "runtime");
  }

  async preview(repository: string, selection: EvidenceSelection): Promise<EvidencePreviewResponse> {
    return this.previewWithDiscoveryRace(
      "/v1/go-context-evidence-previews",
      { repository, selection },
    );
  }

  async previewPiSession(
    repository: string,
    piSessionID: string,
    selection: EvidenceSelection,
  ): Promise<EvidencePreviewResponse> {
    if (!validPiSessionID(piSessionID)) {
      throw new EvidenceClientError("invalid_request", "Pi Session identity is invalid");
    }
    return this.previewWithDiscoveryRace(
      "/v1/pi-session-go-context-evidence-previews",
      { repository, pi_session_id: piSessionID, selection },
    );
  }

  private async previewWithDiscoveryRace(path: string, payload: unknown): Promise<EvidencePreviewResponse> {
    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        return await this.previewOnce(path, payload);
      } catch (error) {
        const clientError = normalizeClientError(error);
        if (attempt === 0 && isDiscoveryRace(clientError)) {
          continue;
        }
        throw clientError;
      }
    }
    throw new EvidenceClientError("daemon_unavailable", "daemon discovery failed");
  }

  async questions(continuationID: string, selection: EvaluatorSelection): Promise<QuestionSet> {
    try {
      const { port, token } = await this.discover();
      const result = await requestJSON(
        port,
        "POST",
        "/v1/question-sets",
        JSON.stringify({
          continuation_id: continuationID,
          pi_version: selection.pi_version,
          model: {
            provider: selection.provider,
            id: selection.id,
            thinking_level: selection.thinking_level,
          },
        }),
        token,
        QUESTION_SET_TIMEOUT_MS,
      );
      if (result.statusCode === 401) {
        throw new EvidenceClientError("unauthorized", "authentication required");
      }
      if (result.statusCode !== 200) {
        throw parseServerError(result.body);
      }
      if (!isQuestionSetResponse(result.body)) {
        throw new EvidenceClientError("protocol_mismatch", "daemon question-set response is invalid");
      }
      return {
        ...result.body.question_set,
        ...(result.body.assessment === undefined ? {} : { assessment: result.body.assessment }),
      };
    } catch (error) {
      throw normalizeClientError(error);
    }
  }

  async reviewedPiSessionIDs(repository: string, piSessionIDs: string[]): Promise<PiSessionReviewResponse> {
    try {
      if (
        piSessionIDs.length < 1 ||
        piSessionIDs.length > 20 ||
        !piSessionIDs.every(validPiSessionID) ||
        new Set(piSessionIDs).size !== piSessionIDs.length
      ) {
        throw new EvidenceClientError("invalid_request", "Pi Session review candidates are invalid");
      }
      const { port, token } = await this.discover();
      const result = await requestJSON(
        port,
        "POST",
        "/v1/pi-session-review-queries",
        JSON.stringify({ repository, pi_session_ids: piSessionIDs }),
        token,
        HISTORY_QUERY_TIMEOUT_MS,
      );
      if (result.statusCode === 401) {
        throw new EvidenceClientError("unauthorized", "authentication required");
      }
      if (result.statusCode !== 200) {
        throw parseServerError(result.body);
      }
      if (!isPiSessionReviewResponse(result.body, piSessionIDs)) {
        throw new EvidenceClientError("protocol_mismatch", "daemon Pi Session review response is invalid");
      }
      return result.body;
    } catch (error) {
      throw normalizeClientError(error);
    }
  }

  async assess(assessmentID: string, submission: AssessmentSubmission): Promise<AssessmentResult> {
    try {
      const { port, token } = await this.discover();
      const result = await requestJSON(
        port,
        "POST",
        "/v1/assessment-turns",
        JSON.stringify({ assessment_id: assessmentID, ...submission }),
        token,
        ASSESSMENT_TIMEOUT_MS,
      );
      if (result.statusCode === 401) {
        throw new EvidenceClientError("unauthorized", "authentication required");
      }
      if (result.statusCode !== 200) {
        throw parseServerError(result.body);
      }
      if (!isAssessmentResultResponse(result.body)) {
        throw new EvidenceClientError("protocol_mismatch", "daemon assessment response is invalid");
      }
      return result.body.assessment_turn.disposition === "complete"
        ? { turn: result.body.assessment_turn, label: result.body.label!, history: result.body.history! }
        : { turn: result.body.assessment_turn };
    } catch (error) {
      throw normalizeClientError(error);
    }
  }

  async history(repository: string, limit: number): Promise<LearningHistoryResponse> {
    try {
      if (!Number.isSafeInteger(limit) || limit < 1 || limit > 50) {
        throw new EvidenceClientError("invalid_request", "history limit is invalid");
      }
      const { port, token } = await this.discover();
      const result = await requestJSON(
        port,
        "POST",
        "/v1/learning-history-queries",
        JSON.stringify({ repository, limit }),
        token,
        HISTORY_QUERY_TIMEOUT_MS,
      );
      if (result.statusCode === 401) {
        throw new EvidenceClientError("unauthorized", "authentication required");
      }
      if (result.statusCode !== 200) {
        throw parseServerError(result.body);
      }
      if (!isLearningHistoryResponse(result.body, limit)) {
        throw new EvidenceClientError("protocol_mismatch", "daemon history response is invalid");
      }
      return result.body;
    } catch (error) {
      throw normalizeClientError(error);
    }
  }

  private async previewOnce(
    path: string,
    payload: unknown,
  ): Promise<EvidencePreviewResponse> {
    const { port, token } = await this.discover();

    const result = await requestJSON(
      port,
      "POST",
      path,
      JSON.stringify(payload),
      token,
      PREVIEW_TIMEOUT_MS,
    );
    if (result.statusCode === 401) {
      throw new EvidenceClientError("unauthorized", "authentication required");
    }
    if (result.statusCode !== 200) {
      throw parseServerError(result.body);
    }
    if (!isPreviewResponse(result.body)) {
      throw new EvidenceClientError("protocol_mismatch", "daemon preview response is invalid");
    }
    return result.body;
  }

  private async discover(): Promise<{ port: number; token: string }> {
    await validateProtectedPath(this.runtimeDir, 0o700, "directory");
    const descriptorPath = join(this.runtimeDir, "daemon.json");
    await validateProtectedPath(descriptorPath, 0o600, "file");
    const descriptor = parseDescriptor(await readFile(descriptorPath, "utf8"));
    const port = descriptorPort(descriptor.base_url);

    const status = await requestJSON(port, "GET", "/v1/status", undefined, undefined, STATUS_TIMEOUT_MS);
    if (status.statusCode !== 200 || !isMatchingStatus(status.body, descriptor.instance_id)) {
      throw new EvidenceClientError("daemon_changed", "daemon status does not match the runtime descriptor");
    }

    const tokenPath = join(this.runtimeDir, "daemon.token");
    await validateProtectedPath(tokenPath, 0o600, "file");
    const token = await readFile(tokenPath, "utf8");
    if (!/^[A-Za-z0-9_-]{43}$/.test(token)) {
      throw new EvidenceClientError("invalid_runtime_state", "daemon token is invalid");
    }
    return { port, token };
  }
}

function validPiSessionID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    Buffer.byteLength(value, "ascii") <= 128 &&
    /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(value)
  );
}

function parseServerError(value: unknown): EvidenceClientError {
  if (
    isObject(value) &&
    value.protocol_version === 1 &&
    isObject(value.error) &&
    typeof value.error.code === "string" &&
    SERVER_ERROR_CODES.has(value.error.code as EvidenceClientErrorCode) &&
    typeof value.error.message === "string"
  ) {
    return new EvidenceClientError(value.error.code as EvidenceClientErrorCode, value.error.message);
  }
  return new EvidenceClientError("protocol_mismatch", "daemon error response is invalid");
}

function normalizeClientError(error: unknown): EvidenceClientError | Error {
  if (error instanceof EvidenceClientError) {
    return error;
  }
  if (isConnectionError(error)) {
    return new EvidenceClientError("daemon_unavailable", "daemon connection failed");
  }
  return error instanceof Error ? error : new Error("unknown daemon client error");
}

function isDiscoveryRace(error: Error): boolean {
  return (
    error instanceof EvidenceClientError &&
    ["daemon_unavailable", "daemon_changed", "unauthorized"].includes(error.code)
  );
}

async function validateProtectedPath(path: string, mode: number, kind: "directory" | "file"): Promise<void> {
  const info = await lstat(path);
  if (info.isSymbolicLink() || (kind === "directory" ? !info.isDirectory() : !info.isFile())) {
    throw new EvidenceClientError("invalid_runtime_state", `runtime ${kind} is not a real ${kind}`);
  }
  if ((info.mode & 0o777) !== mode) {
    throw new EvidenceClientError("invalid_runtime_state", `runtime ${kind} permissions are invalid`);
  }
  const effectiveUID = process.geteuid?.();
  if (effectiveUID === undefined || info.uid !== effectiveUID) {
    throw new EvidenceClientError("invalid_runtime_state", `runtime ${kind} ownership is invalid`);
  }
}

function parseDescriptor(content: string): RuntimeDescriptor {
  if (Buffer.byteLength(content, "utf8") > 4_096) {
    throw new EvidenceClientError("invalid_runtime_state", "runtime descriptor is too large");
  }
  let value: unknown;
  try {
    value = JSON.parse(content);
  } catch {
    throw new EvidenceClientError("invalid_runtime_state", "runtime descriptor is not valid JSON");
  }
  if (!isObject(value)) {
    throw new EvidenceClientError("invalid_runtime_state", "runtime descriptor is invalid");
  }
  if (
    value.schema_version !== 1 ||
    value.protocol_version !== 1 ||
    typeof value.instance_id !== "string" ||
    !/^[A-Za-z0-9_-]{22}$/.test(value.instance_id) ||
    !Number.isSafeInteger(value.pid) ||
    (value.pid as number) <= 0 ||
    typeof value.base_url !== "string" ||
    typeof value.started_at !== "string" ||
    !Number.isFinite(Date.parse(value.started_at))
  ) {
    throw new EvidenceClientError("invalid_runtime_state", "runtime descriptor is invalid");
  }
  descriptorPort(value.base_url);
  return value as unknown as RuntimeDescriptor;
}

function descriptorPort(baseURL: string): number {
  const match = /^http:\/\/127\.0\.0\.1:([1-9][0-9]{0,4})$/.exec(baseURL);
  const port = match ? Number(match[1]) : 0;
  if (port < 1 || port > 65_535) {
    throw new EvidenceClientError("invalid_runtime_state", "runtime descriptor base_url is invalid");
  }
  return port;
}

function requestJSON(
  port: number,
  method: "GET" | "POST",
  path: string,
  body: string | undefined,
  token: string | undefined,
  timeoutMS: number,
): Promise<HTTPResult> {
  return new Promise((resolve, reject) => {
    const headers: Record<string, string | number> = {
      Accept: "application/json",
      Host: `127.0.0.1:${port}`,
    };
    if (body !== undefined) {
      headers["Content-Type"] = "application/json";
      headers["Content-Length"] = Buffer.byteLength(body, "utf8");
    }
    if (token !== undefined) {
      headers.Authorization = `PiLearnLoop ${token}`;
    }

    const request = httpRequest(
      {
        hostname: "127.0.0.1",
        port,
        path,
        method,
        headers,
        agent: false,
      },
      (response) => {
        const chunks: Buffer[] = [];
        let size = 0;
        response.on("data", (chunk: Buffer) => {
          size += chunk.length;
          if (size > MAX_RESPONSE_BYTES) {
            response.destroy(new Error("daemon response is too large"));
            return;
          }
          chunks.push(chunk);
        });
        response.on("error", reject);
        response.on("end", () => {
          try {
            const contentType = String(response.headers["content-type"] ?? "").split(";", 1)[0]?.trim();
            if (contentType !== "application/json") {
              throw new Error("daemon response content type is invalid");
            }
            const content = Buffer.concat(chunks).toString("utf8");
            resolve({
              statusCode: response.statusCode ?? 0,
              contentType,
              body: JSON.parse(content),
            });
          } catch (error) {
            reject(error);
          }
        });
      },
    );
    request.setTimeout(timeoutMS, () => request.destroy(new Error("daemon request timed out")));
    request.on("error", reject);
    if (body !== undefined) {
      request.write(body);
    }
    request.end();
  });
}

function isMatchingStatus(value: unknown, instanceID: string): boolean {
  return (
    isObject(value) &&
    value.protocol_version === 1 &&
    value.instance_id === instanceID &&
    value.status === "ready"
  );
}

function isPreviewResponse(value: unknown): value is EvidencePreviewResponse {
  if (
    !isObject(value) ||
    !hasOnlyKeys(value, "protocol_version", "applied_limits", "preview", "continuation") ||
    value.protocol_version !== 1 ||
    !isObject(value.applied_limits) ||
    !hasOnlyKeys(value.applied_limits, "max_files", "max_declarations", "max_excerpt_bytes") ||
    value.applied_limits.max_files !== 20 ||
    value.applied_limits.max_declarations !== 100 ||
    value.applied_limits.max_excerpt_bytes !== 131_072 ||
    !isObject(value.preview) ||
    !hasOnlyKeys(value.preview, "repository_root", "base_revision", "head_revision", "files", "go_context", "truncation") ||
    !isAbsolutePreviewPath(value.preview.repository_root) ||
    !isBoundedPreviewText(value.preview.base_revision, 256) ||
    !isBoundedPreviewText(value.preview.head_revision, 256) ||
    !Array.isArray(value.preview.files) ||
    value.preview.files.length > 20 ||
    !value.preview.files.every(isEvidenceFile) ||
    !isGoContextPreview(value.preview.go_context) ||
    !isTruncation(value.preview.truncation) ||
    !isContinuation(value.continuation)
  ) {
    return false;
  }

  const declarations = value.preview.files.flatMap((file) => file.declarations);
  const excerptBytes = declarations.reduce(
    (total, declaration) => total + Buffer.byteLength(declaration.excerpt, "utf8"),
    0,
  );
  return declarations.length <= 100 && excerptBytes <= 131_072;
}

function isGoContextPreview(value: unknown): value is GoContextPreview {
  if (
    !isObject(value) ||
    !hasOnlyKeys(
      value,
      "status",
      "build",
      "applied_limits",
      "analyzed_package_count",
      "analyzed_file_count",
      "analyzed_source_bytes",
      "direct_import_edges",
      "item_count",
      "relation_count",
      "approximate_bytes",
      "items",
      "relations",
      "omissions",
      "truncation",
    ) ||
    !["complete", "partial", "unavailable"].includes(String(value.status)) ||
    !isGoContextBuild(value.build) ||
    !isGoContextLimits(value.applied_limits) ||
    !isNonNegativeInteger(value.analyzed_package_count) || value.analyzed_package_count > 32 ||
    !isNonNegativeInteger(value.analyzed_file_count) || value.analyzed_file_count > 160 ||
    !isNonNegativeInteger(value.analyzed_source_bytes) || value.analyzed_source_bytes > 2_097_152 ||
    !isNonNegativeInteger(value.direct_import_edges) || value.direct_import_edges > 256 ||
    !Array.isArray(value.items) || value.items.length > 40 ||
    !value.items.every((item, index) => isGoContextItem(item, index)) ||
    value.item_count !== value.items.length ||
    !Array.isArray(value.relations) || value.relations.length > 100 ||
    !value.relations.every((relation) => isGoContextRelation(relation, value.items as GoContextItem[])) ||
    value.relation_count !== value.relations.length ||
    !Array.isArray(value.omissions) || !isGoContextOmissions(value.status, value.omissions, value.truncation) ||
    !isGoContextTruncation(value.truncation) ||
    !isNonNegativeInteger(value.approximate_bytes) || value.approximate_bytes > 65_536
  ) {
    return false;
  }
  if (value.status === "unavailable" && (value.items.length !== 0 || value.relations.length !== 0)) {
    return false;
  }
  const paths = new Set(value.items.map((item) => item.path));
  return paths.size <= 20 && value.approximate_bytes === goContextApproximateBytes(value as unknown as GoContextPreview);
}

function isGoContextLimits(value: unknown): value is GoContextPreview["applied_limits"] {
  return (
    isObject(value) &&
    hasOnlyKeys(
      value,
      "max_changed_files",
      "max_module_roots",
      "max_packages",
      "max_files_per_package",
      "max_files",
      "max_directory_entries",
      "max_source_bytes_per_file",
      "max_source_bytes",
      "max_direct_import_edges",
      "analysis_timeout_millis",
      "max_output_files",
      "max_output_items",
      "max_relations",
      "max_excerpt_bytes",
      "max_output_bytes",
      "max_evaluator_input_bytes",
    ) &&
    value.max_changed_files === 20 &&
    value.max_module_roots === 8 &&
    value.max_packages === 32 &&
    value.max_files_per_package === 64 &&
    value.max_files === 160 &&
    value.max_directory_entries === 256 &&
    value.max_source_bytes_per_file === 262_144 &&
    value.max_source_bytes === 2_097_152 &&
    value.max_direct_import_edges === 256 &&
    value.analysis_timeout_millis === 30_000 &&
    value.max_output_files === 20 &&
    value.max_output_items === 40 &&
    value.max_relations === 100 &&
    value.max_excerpt_bytes === 4_096 &&
    value.max_output_bytes === 65_536 &&
    value.max_evaluator_input_bytes === 262_144
  );
}

function isGoContextBuild(value: unknown): value is GoContextBuild {
  if (
    !isObject(value) ||
    !hasOnlyKeys(
      value,
      "goos",
      "goarch",
      "cgo_enabled",
      "build_tags",
      "tool_tags",
      "release_tags",
      "toolchain_version",
      "test_variant",
      "modules",
      "workspaces",
      "replacements",
    ) ||
    !isContextText(value.goos) ||
    !isContextText(value.goarch) ||
    value.cgo_enabled !== false ||
    !Array.isArray(value.build_tags) || value.build_tags.length !== 0 ||
    !Array.isArray(value.tool_tags) || !value.tool_tags.every(isContextText) ||
    !Array.isArray(value.release_tags) || !value.release_tags.every(isContextText) ||
    !isContextText(value.toolchain_version) ||
    typeof value.test_variant !== "boolean" ||
    !Array.isArray(value.modules) || !value.modules.every(isGoContextModule) ||
    !Array.isArray(value.workspaces) || !value.workspaces.every(isGoContextWorkspace) ||
    value.modules.length + value.workspaces.length > 8 ||
    !Array.isArray(value.replacements) || !value.replacements.every(isGoContextReplacement)
  ) {
    return false;
  }
  return true;
}

function isGoContextModule(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "path", "directory", "go_version", "toolchain") &&
    isContextText(value.path) &&
    isOptionalPreviewPath(value.directory) &&
    isContextText(value.go_version) &&
    isOptionalContextText(value.toolchain)
  );
}

function isGoContextWorkspace(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "directory", "go_version", "toolchain") &&
    isOptionalPreviewPath(value.directory) &&
    isOptionalContextText(value.go_version) &&
    isOptionalContextText(value.toolchain)
  );
}

function isGoContextReplacement(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "module_path", "directory", "repository_local") &&
    isContextText(value.module_path) &&
    isOptionalPreviewPath(value.directory) &&
    typeof value.repository_local === "boolean" &&
    (!value.repository_local || value.directory !== "")
  );
}

function isGoContextItem(value: unknown, index: number): value is GoContextItem {
  if (
    !isObject(value) ||
    !hasOnlyKeys(
      value,
      "reference",
      "kind",
      "path",
      "package_path",
      "declaration_kind",
      "identity",
      "start_line",
      "end_line",
      "content",
      "content_bytes",
      "content_sha256",
      "truncated",
    ) ||
    value.reference !== `C${String(index + 1).padStart(3, "0")}` ||
    !["changed_import", "context_declaration"].includes(String(value.kind)) ||
    !isPreviewPath(value.path) ||
    !isContextText(value.package_path) ||
    !isContextText(value.identity) ||
    !isPositiveInteger(value.start_line) ||
    !isPositiveInteger(value.end_line) || value.end_line < value.start_line ||
    typeof value.content !== "string" || value.content === "" ||
    !isNonNegativeInteger(value.content_bytes) ||
    value.content_bytes !== Buffer.byteLength(value.content, "utf8") ||
    value.content_bytes > 4_096 ||
    typeof value.content_sha256 !== "string" ||
    value.content_sha256 !== createHash("sha256").update(value.content, "utf8").digest("hex") ||
    typeof value.truncated !== "boolean"
  ) {
    return false;
  }
  if (value.kind === "changed_import") {
    return value.declaration_kind === "";
  }
  return ["function", "method", "type", "interface", "variable", "constant"].includes(String(value.declaration_kind));
}

function isGoContextRelation(value: unknown, items: GoContextItem[]): boolean {
  if (
    !isObject(value) ||
    !hasOnlyKeys(value, "from", "to", "kind", "strength") ||
    !isContextText(value.from) ||
    !isContextText(value.to)
  ) {
    return false;
  }
  if (String(value.to).startsWith("C") && !items.some((item) => item.reference === value.to)) {
    return false;
  }
  return value.kind === "imports"
    ? value.strength === "syntactic"
    : ["references", "implements"].includes(String(value.kind)) && value.strength === "type_checked";
}

function isGoContextOmissions(status: unknown, values: unknown[], truncation: unknown): boolean {
  const allowed = [
    "analysis_limit_exceeded",
    "unsupported_module_layout",
    "unsupported_go_version",
    "outside_repository_dependency",
    "cgo_unsupported",
    "external_type_unavailable",
    "context_parse_error",
    "type_incomplete",
    "output_truncated",
  ];
  const reasons: string[] = [];
  for (const value of values) {
    if (
      !isObject(value) ||
      !hasOnlyKeys(value, "reason", "count") ||
      !allowed.includes(String(value.reason)) ||
      !isPositiveInteger(value.count)
    ) {
      return false;
    }
    reasons.push(String(value.reason));
  }
  if (new Set(reasons).size !== reasons.length || !isGoContextTruncation(truncation)) {
    return false;
  }
  const hasOutputTruncation = reasons.includes("output_truncated");
  if (truncation.truncated !== hasOutputTruncation) {
    return false;
  }
  return status === "complete" ? values.length === 0 : values.length > 0;
}

function isGoContextTruncation(value: unknown): value is GoContextPreview["truncation"] {
  if (
    !isObject(value) ||
    !hasOnlyKeys(value, "truncated", "omitted_files", "omitted_items", "omitted_relations", "omitted_bytes") ||
    typeof value.truncated !== "boolean" ||
    !isNonNegativeInteger(value.omitted_files) ||
    !isNonNegativeInteger(value.omitted_items) ||
    !isNonNegativeInteger(value.omitted_relations) ||
    !isNonNegativeInteger(value.omitted_bytes)
  ) {
    return false;
  }
  const hasCounts = value.omitted_files > 0 || value.omitted_items > 0 || value.omitted_relations > 0 || value.omitted_bytes > 0;
  return value.truncated === hasCounts;
}

function goContextApproximateBytes(value: GoContextPreview): number {
  let total = 0;
  for (const module of value.build.modules) {
    total += previewBytes(module.path, module.directory, module.go_version, module.toolchain);
  }
  for (const workspace of value.build.workspaces) {
    total += previewBytes(workspace.directory, workspace.go_version, workspace.toolchain);
  }
  for (const replacement of value.build.replacements) {
    total += previewBytes(replacement.module_path, replacement.directory);
  }
  for (const item of value.items) {
    total += previewBytes(item.path, item.package_path, item.identity, item.content);
  }
  for (const relation of value.relations) {
    total += previewBytes(relation.from, relation.to);
  }
  return total;
}

function previewBytes(...values: string[]): number {
  return values.reduce((total, value) => total + Buffer.byteLength(value, "utf8"), 0);
}

function isContinuation(value: unknown): boolean {
  if (!isObject(value) || typeof value.available !== "boolean") {
    return false;
  }
  if (value.available) {
    return (
      hasOnlyKeys(value, "available", "id", "expires_at") &&
      typeof value.id === "string" &&
      /^pc1-[A-Za-z0-9_-]{43}$/.test(value.id) &&
      typeof value.expires_at === "string" &&
      Number.isFinite(Date.parse(value.expires_at))
    );
  }
  return (
    hasOnlyKeys(value, "available", "reason") &&
    ["insufficient_evidence", "capacity", "evaluator_unavailable"].includes(String(value.reason))
  );
}

function isQuestionSetResponse(value: unknown): value is {
  protocol_version: 1;
  question_set: QuestionSet;
  assessment?: QuestionSet["assessment"];
} {
  if (!isObject(value) || value.protocol_version !== 1 || !isObject(value.question_set)) {
    return false;
  }
  const questionSet = value.question_set;
  if (questionSet.schema_version !== 1 || !Array.isArray(questionSet.questions)) {
    return false;
  }
  if (questionSet.disposition === "insufficient_evidence") {
    return (
      questionSet.questions.length === 0 &&
      (value.assessment === undefined || isAssessmentDescriptor(value.assessment))
    );
  }
  if (questionSet.disposition !== "questions" || questionSet.questions.length !== 3) {
    return false;
  }
  const expectedKinds = ["code_specific", "code_specific", "go_backend"];
  const validQuestions = questionSet.questions.every((question, index) => {
    if (
      !isObject(question) ||
      question.id !== `Q${index + 1}` ||
      question.kind !== expectedKinds[index] ||
      typeof question.text !== "string" ||
      question.text.trim() === "" ||
      Buffer.byteLength(question.text, "utf8") > 1_000 ||
      /[\u0000-\u001f\u007f-\u009f]/u.test(question.text) ||
      !validEvidenceReferences(question.evidence_references, question.kind === "code_specific")
    ) {
      return false;
    }
    return true;
  });
  return validQuestions && (value.assessment === undefined || isAssessmentDescriptor(value.assessment));
}

function isAssessmentDescriptor(value: unknown): boolean {
  if (!isObject(value) || typeof value.available !== "boolean") {
    return false;
  }
  if (value.available) {
    return (
      hasOnlyKeys(value, "available", "id", "expires_at") &&
      typeof value.id === "string" &&
      /^as1-[A-Za-z0-9_-]{43}$/.test(value.id) &&
      typeof value.expires_at === "string" &&
      Number.isFinite(Date.parse(value.expires_at))
    );
  }
  return (
    hasOnlyKeys(value, "available", "reason") &&
    ["insufficient_evidence", "capacity", "evaluator_unavailable"].includes(String(value.reason))
  );
}

function isAssessmentResultResponse(value: unknown): value is {
  protocol_version: 1;
  assessment_turn: AssessmentResult["turn"];
  label?: "understood" | "partial" | "review_needed";
  history?: Extract<AssessmentResult, { label: string }>["history"];
} {
  if (!isObject(value) || value.protocol_version !== 1 || !isObject(value.assessment_turn)) {
    return false;
  }
  const turn = value.assessment_turn;
  if (
    !hasOnlyKeys(turn, "schema_version", "disposition", "follow_up", "evaluations") ||
    turn.schema_version !== 1 ||
    !Array.isArray(turn.evaluations)
  ) {
    return false;
  }
  if (turn.disposition === "follow_up") {
    return (
      hasOnlyKeys(value, "protocol_version", "assessment_turn") &&
      turn.evaluations.length === 0 &&
      isFollowUpQuestion(turn.follow_up)
    );
  }
  if (
    turn.disposition !== "complete" ||
    turn.follow_up !== null ||
    turn.evaluations.length !== 3 ||
    !["understood", "partial", "review_needed"].includes(String(value.label)) ||
    !isHistorySave(value.history) ||
    !hasOnlyKeys(value, "protocol_version", "assessment_turn", "label", "history")
  ) {
    return false;
  }
  return turn.evaluations.every(isQuestionEvaluation);
}

function isHistorySave(value: unknown): boolean {
  if (!isObject(value) || typeof value.saved !== "boolean") {
    return false;
  }
  if (value.saved) {
    return (
      hasOnlyKeys(value, "saved", "record_id") &&
      typeof value.record_id === "string" &&
      /^lr1-[A-Za-z0-9_-]{43}$/.test(value.record_id)
    );
  }
  return hasOnlyKeys(value, "saved", "reason") && value.reason === "storage_unavailable";
}

function isLearningHistoryResponse(value: unknown, limit: number): value is LearningHistoryResponse {
  if (
    !isObject(value) ||
    !hasOnlyKeys(value, "protocol_version", "records") ||
    value.protocol_version !== 1 ||
    !Array.isArray(value.records) ||
    value.records.length > limit ||
    !value.records.every(isLearningHistoryRecord)
  ) {
    return false;
  }
  const records = value.records;
  const recordIDs = records.map((record) => record.record_id);
  if (new Set(recordIDs).size !== recordIDs.length) {
    return false;
  }
  return records.every((record, index) =>
    index === 0 || Date.parse(records[index - 1]!.started_at) >= Date.parse(record.started_at)
  );
}

function isPiSessionReviewResponse(value: unknown, candidates: string[]): value is PiSessionReviewResponse {
  if (
    !isObject(value) ||
    !hasOnlyKeys(value, "protocol_version", "reviewed_pi_session_ids") ||
    value.protocol_version !== 1 ||
    !Array.isArray(value.reviewed_pi_session_ids) ||
    value.reviewed_pi_session_ids.length > candidates.length ||
    !value.reviewed_pi_session_ids.every(validPiSessionID) ||
    new Set(value.reviewed_pi_session_ids).size !== value.reviewed_pi_session_ids.length
  ) {
    return false;
  }
  let previousIndex = -1;
  for (const id of value.reviewed_pi_session_ids) {
    const index = candidates.indexOf(id);
    if (index <= previousIndex) {
      return false;
    }
    previousIndex = index;
  }
  return true;
}

function isLearningHistoryRecord(value: unknown): value is LearningHistoryRecord {
  if (
    !isObject(value) ||
    !hasOnlyKeys(
      value,
      "record_id",
      "started_at",
      "finished_at",
      "status",
      "failure_code",
      "base_revision",
      "head_revision",
      "evidence_manifest_sha256",
      "question_schema_version",
      "assessment_schema_version",
      "question_prompt",
      "assessment_prompt",
      "pi_version",
      "provider",
      "model_id",
      "thinking_level",
      "follow_up_used",
      "label",
      "outcomes",
    ) ||
    typeof value.record_id !== "string" ||
    !/^lr1-[A-Za-z0-9_-]{43}$/.test(value.record_id) ||
    !isHistoryTimestamp(value.started_at) ||
    (value.finished_at !== null && !isHistoryTimestamp(value.finished_at)) ||
    !["running", "complete", "failed", "interrupted"].includes(String(value.status)) ||
    (value.failure_code !== null && !["evaluator_failed", "evaluator_invalid_output", "evaluator_timeout"].includes(String(value.failure_code))) ||
    !isBoundedHistoryValue(value.base_revision, 256) ||
    !isBoundedHistoryValue(value.head_revision, 256) ||
    !isLowerSHA256(value.evidence_manifest_sha256) ||
    value.question_schema_version !== 1 ||
    value.assessment_schema_version !== 1 ||
    !isHistoryPrompt(value.question_prompt) ||
    !isHistoryPrompt(value.assessment_prompt) ||
    value.pi_version !== "0.84.3" ||
    !isBoundedHistoryArgument(value.provider, 128) ||
    !isBoundedHistoryArgument(value.model_id, 256) ||
    !["off", "minimal", "low", "medium", "high", "xhigh", "max"].includes(String(value.thinking_level)) ||
    typeof value.follow_up_used !== "boolean" ||
    (value.label !== null && !["understood", "partial", "review_needed"].includes(String(value.label))) ||
    !Array.isArray(value.outcomes)
  ) {
    return false;
  }
  switch (value.status) {
    case "running":
      return value.finished_at === null && value.failure_code === null && value.label === null && value.outcomes.length === 0;
    case "complete":
      return value.finished_at !== null && value.failure_code === null && value.label !== null && isCompleteHistoryOutcomes(value.outcomes);
    case "failed":
      return value.finished_at !== null && value.failure_code !== null && value.label === null && value.outcomes.length === 0;
    case "interrupted":
      return value.finished_at !== null && value.failure_code === null && value.label === null && value.outcomes.length === 0;
    default:
      return false;
  }
}

function isCompleteHistoryOutcomes(value: unknown[]): boolean {
  const kinds = ["code_specific", "code_specific", "go_backend"];
  return value.length === 3 && value.every((outcome, index) =>
    isObject(outcome) &&
    hasOnlyKeys(outcome, "question_id", "question_kind", "verdict") &&
    outcome.question_id === `Q${index + 1}` &&
    outcome.question_kind === kinds[index] &&
    ["demonstrated", "partial", "not_demonstrated"].includes(String(outcome.verdict))
  );
}

function isHistoryPrompt(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "id", "version", "sha256") &&
    isBoundedHistoryValue(value.id, 128) &&
    isBoundedHistoryValue(value.version, 64) &&
    isLowerSHA256(value.sha256)
  );
}

function isHistoryTimestamp(value: unknown): value is string {
  return (
    typeof value === "string" &&
    /^\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(?:\.\d{1,9})?Z$/.test(value) &&
    Number.isFinite(Date.parse(value))
  );
}

function isLowerSHA256(value: unknown): value is string {
  return typeof value === "string" && /^[0-9a-f]{64}$/.test(value);
}

function isBoundedHistoryArgument(value: unknown, maximumBytes: number): value is string {
  return isBoundedHistoryValue(value, maximumBytes) && !value.startsWith("-");
}

function isBoundedHistoryValue(value: unknown, maximumBytes: number): value is string {
  return (
    typeof value === "string" &&
    value.trim() === value &&
    value !== "" &&
    Buffer.byteLength(value, "utf8") <= maximumBytes &&
    !/[\u0000-\u001f\u007f-\u009f]/u.test(value)
  );
}

function isFollowUpQuestion(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "id", "target_question_id", "text", "evidence_references") &&
    value.id === "F1" &&
    ["Q1", "Q2", "Q3"].includes(String(value.target_question_id)) &&
    validAssessmentText(value.text) &&
    validEvidenceReferences(value.evidence_references, value.target_question_id !== "Q3")
  );
}

function isQuestionEvaluation(value: unknown, index: number): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "question_id", "verdict", "feedback", "evidence_references") &&
    value.question_id === `Q${index + 1}` &&
    ["demonstrated", "partial", "not_demonstrated"].includes(String(value.verdict)) &&
    validAssessmentText(value.feedback) &&
    validEvidenceReferences(value.evidence_references, index < 2)
  );
}

function validAssessmentText(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value.trim() !== "" &&
    Buffer.byteLength(value, "utf8") <= 1_000 &&
    !/[\u0000-\u001f\u007f-\u009f]/u.test(value)
  );
}

function validEvidenceReferences(value: unknown, required: boolean): value is string[] {
  return (
    Array.isArray(value) &&
    (!required || value.length > 0) &&
    value.every((reference) => typeof reference === "string" && /^[EC][0-9]{3}$/.test(reference)) &&
    new Set(value).size === value.length
  );
}

function hasOnlyKeys(value: Record<string, unknown>, ...keys: string[]): boolean {
  const actual = Object.keys(value);
  return actual.length === keys.length && keys.every((key) => Object.hasOwn(value, key));
}

function isEvidenceFile(value: unknown): value is EvidencePreviewResponse["preview"]["files"][number] {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "path", "status", "changed_lines", "declarations", "omissions") &&
    isPreviewPath(value.path) &&
    ["added", "modified", "deleted"].includes(String(value.status)) &&
    Array.isArray(value.changed_lines) &&
    value.changed_lines.every(isLineRange) &&
    Array.isArray(value.declarations) &&
    value.declarations.every(isDeclaration) &&
    Array.isArray(value.omissions) &&
    value.omissions.every(isOmission)
  );
}

function isDeclaration(value: unknown): value is EvidencePreviewResponse["preview"]["files"][number]["declarations"][number] {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "kind", "name", "receiver", "identity", "start_line", "end_line", "changed_lines", "excerpt", "excerpt_truncated") &&
    ["function", "method", "type", "interface", "variable", "constant"].includes(String(value.kind)) &&
    isContextText(value.name) &&
    isOptionalContextText(value.receiver) &&
    isContextText(value.identity) &&
    isPositiveInteger(value.start_line) &&
    isPositiveInteger(value.end_line) &&
    value.end_line >= value.start_line &&
    Array.isArray(value.changed_lines) &&
    value.changed_lines.every(isLineRange) &&
    typeof value.excerpt === "string" && value.excerpt !== "" &&
    typeof value.excerpt_truncated === "boolean"
  );
}

function isLineRange(value: unknown): value is { start: number; end: number } {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "start", "end") &&
    isPositiveInteger(value.start) &&
    isPositiveInteger(value.end) &&
    value.end >= value.start
  );
}

function isOmission(value: unknown): boolean {
  return (
    isObject(value) &&
    hasOnlyKeys(value, "reason", "count") &&
    ["deleted_file", "deleted_only_hunk", "outside_declaration"].includes(String(value.reason)) &&
    isPositiveInteger(value.count)
  );
}

function isTruncation(value: unknown): boolean {
  if (!(
    isObject(value) &&
    hasOnlyKeys(value, "truncated", "omitted_files", "omitted_declarations", "omitted_excerpt_bytes") &&
    typeof value.truncated === "boolean" &&
    isNonNegativeInteger(value.omitted_files) &&
    isNonNegativeInteger(value.omitted_declarations) &&
    isNonNegativeInteger(value.omitted_excerpt_bytes)
  )) {
    return false;
  }
  const hasCounts = value.omitted_files > 0 || value.omitted_declarations > 0 || value.omitted_excerpt_bytes > 0;
  return value.truncated === hasCounts;
}

function isAbsolutePreviewPath(value: unknown): value is string {
  return typeof value === "string" && value !== "" && Buffer.byteLength(value, "utf8") <= MAX_RESPONSE_BYTES && value.startsWith("/");
}

function isPreviewPath(value: unknown): value is string {
  return (
    typeof value === "string" &&
    value !== "" &&
    Buffer.byteLength(value, "utf8") <= MAX_RESPONSE_BYTES &&
    !value.startsWith("/") &&
    !value.includes("\\") &&
    value.split("/").every((part) => part !== "" && part !== "." && part !== "..")
  );
}

function isOptionalPreviewPath(value: unknown): value is string {
  return value === "" || isPreviewPath(value);
}

function isContextText(value: unknown): value is string {
  return isBoundedPreviewText(value, MAX_RESPONSE_BYTES);
}

function isOptionalContextText(value: unknown): value is string {
  return value === "" || isContextText(value);
}

function isBoundedPreviewText(value: unknown, maximumBytes: number): value is string {
  if (typeof value !== "string" || value === "" || Buffer.byteLength(value, "utf8") > maximumBytes) {
    return false;
  }
  return !/\p{Cc}/u.test(value);
}

function isPositiveInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) > 0;
}

function isNonNegativeInteger(value: unknown): value is number {
  return Number.isSafeInteger(value) && (value as number) >= 0;
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function isConnectionError(error: unknown): boolean {
  return (
    error instanceof Error &&
    "code" in error &&
    ["ECONNREFUSED", "ECONNRESET", "EHOSTUNREACH", "ENETUNREACH", "ETIMEDOUT", "ENOENT"].includes(
      String(error.code),
    )
  );
}
