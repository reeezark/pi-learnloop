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
  type QuestionSet,
} from "./learn-command.ts";

const STATUS_TIMEOUT_MS = 2_000;
const PREVIEW_TIMEOUT_MS = 35_000;
const QUESTION_SET_TIMEOUT_MS = 130_000;
const ASSESSMENT_TIMEOUT_MS = 130_000;
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
    for (let attempt = 0; attempt < 2; attempt += 1) {
      try {
        return await this.previewOnce(repository, selection);
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

  private async previewOnce(
    repository: string,
    selection: EvidenceSelection,
  ): Promise<EvidencePreviewResponse> {
    const { port, token } = await this.discover();

    const result = await requestJSON(
      port,
      "POST",
      "/v1/evidence-previews",
      JSON.stringify({ repository, selection }),
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
    value.protocol_version !== 1 ||
    !isObject(value.applied_limits) ||
    value.applied_limits.max_files !== 20 ||
    value.applied_limits.max_declarations !== 100 ||
    value.applied_limits.max_excerpt_bytes !== 131_072 ||
    !isObject(value.preview) ||
    typeof value.preview.repository_root !== "string" ||
    typeof value.preview.base_revision !== "string" ||
    typeof value.preview.head_revision !== "string" ||
    !Array.isArray(value.preview.files) ||
    value.preview.files.length > 20 ||
    !value.preview.files.every(isEvidenceFile) ||
    !isTruncation(value.preview.truncation) ||
    (value.continuation !== undefined && !isContinuation(value.continuation))
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

function isContinuation(value: unknown): boolean {
  if (!isObject(value) || typeof value.available !== "boolean") {
    return false;
  }
  if (value.available) {
    return (
      typeof value.id === "string" &&
      /^pc1-[A-Za-z0-9_-]{43}$/.test(value.id) &&
      typeof value.expires_at === "string" &&
      Number.isFinite(Date.parse(value.expires_at))
    );
  }
  return ["insufficient_evidence", "capacity", "evaluator_unavailable"].includes(String(value.reason));
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
      !Array.isArray(question.evidence_references) ||
      !question.evidence_references.every((reference) => typeof reference === "string") ||
      new Set(question.evidence_references).size !== question.evidence_references.length
    ) {
      return false;
    }
    return question.kind !== "code_specific" || question.evidence_references.length > 0;
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
    value.every((reference) => typeof reference === "string" && /^E[0-9]{3}$/.test(reference)) &&
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
    typeof value.path === "string" &&
    value.path !== "" &&
    !value.path.startsWith("/") &&
    !value.path.split("/").includes("..") &&
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
    ["function", "method", "type", "interface", "variable", "constant"].includes(String(value.kind)) &&
    typeof value.name === "string" &&
    typeof value.receiver === "string" &&
    typeof value.identity === "string" &&
    isPositiveInteger(value.start_line) &&
    isPositiveInteger(value.end_line) &&
    value.end_line >= value.start_line &&
    Array.isArray(value.changed_lines) &&
    value.changed_lines.every(isLineRange) &&
    typeof value.excerpt === "string" &&
    typeof value.excerpt_truncated === "boolean"
  );
}

function isLineRange(value: unknown): value is { start: number; end: number } {
  return (
    isObject(value) &&
    isPositiveInteger(value.start) &&
    isPositiveInteger(value.end) &&
    value.end >= value.start
  );
}

function isOmission(value: unknown): boolean {
  return (
    isObject(value) &&
    ["deleted_file", "deleted_only_hunk", "outside_declaration"].includes(String(value.reason)) &&
    isNonNegativeInteger(value.count)
  );
}

function isTruncation(value: unknown): boolean {
  return (
    isObject(value) &&
    typeof value.truncated === "boolean" &&
    isNonNegativeInteger(value.omitted_files) &&
    isNonNegativeInteger(value.omitted_declarations) &&
    isNonNegativeInteger(value.omitted_excerpt_bytes)
  );
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
