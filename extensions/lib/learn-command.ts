export type EvidenceSelection =
  | { kind: "commit_range"; base: string; head: string }
  | { kind: "working_tree"; base: string };

export interface LineRange {
  start: number;
  end: number;
}

export interface EvidenceDeclaration {
  kind: "function" | "method" | "type" | "interface" | "variable" | "constant";
  name: string;
  receiver: string;
  identity: string;
  start_line: number;
  end_line: number;
  changed_lines: LineRange[];
  excerpt: string;
  excerpt_truncated: boolean;
}

export interface EvidenceFile {
  path: string;
  status: "added" | "modified" | "deleted";
  changed_lines: LineRange[];
  declarations: EvidenceDeclaration[];
  omissions: Array<{
    reason: "deleted_file" | "deleted_only_hunk" | "outside_declaration";
    count: number;
  }>;
}

export interface EvidencePreviewResponse {
  protocol_version: 1;
  applied_limits: {
    max_files: number;
    max_declarations: number;
    max_excerpt_bytes: number;
  };
  preview: {
    repository_root: string;
    base_revision: string;
    head_revision: string;
    files: EvidenceFile[];
    truncation: {
      truncated: boolean;
      omitted_files: number;
      omitted_declarations: number;
      omitted_excerpt_bytes: number;
    };
  };
  continuation?:
    | {
        available: true;
        id: string;
        expires_at: string;
      }
    | {
        available: false;
        reason: "insufficient_evidence" | "capacity" | "evaluator_unavailable";
      };
}

export interface EvidencePreviewClient {
  preview(repository: string, selection: EvidenceSelection): Promise<EvidencePreviewResponse>;
}

export interface PiSessionReviewResponse {
  protocol_version: 1;
  reviewed_pi_session_ids: string[];
}

export type PiSessionLister = (cwd: string, sessionDir: string) => Promise<readonly unknown[]>;

export interface EvaluatorSelection {
  pi_version: string;
  provider: string;
  id: string;
  thinking_level: string;
}

export interface Question {
  id: "Q1" | "Q2" | "Q3";
  kind: "code_specific" | "go_backend";
  text: string;
  evidence_references: string[];
}

export interface QuestionSet {
  schema_version: 1;
  disposition: "questions" | "insufficient_evidence";
  questions: Question[];
  assessment?: AssessmentDescriptor;
}

export type AssessmentDescriptor =
  | { available: true; id: string; expires_at: string }
  | { available: false; reason: "insufficient_evidence" | "capacity" | "evaluator_unavailable" };

export interface AssessmentAnswer {
  question_id: "Q1" | "Q2" | "Q3";
  text: string;
}

export type AssessmentSubmission =
  | { stage: "initial_answers"; answers: AssessmentAnswer[] }
  | { stage: "follow_up_answer"; follow_up_id: "F1"; answer: string };

export interface FollowUpQuestion {
  id: "F1";
  target_question_id: "Q1" | "Q2" | "Q3";
  text: string;
  evidence_references: string[];
}

export interface QuestionEvaluation {
  question_id: "Q1" | "Q2" | "Q3";
  verdict: "demonstrated" | "partial" | "not_demonstrated";
  feedback: string;
  evidence_references: string[];
}

export type HistorySave =
  | { saved: true; record_id: string }
  | { saved: false; reason: "storage_unavailable" };

export interface LearningHistoryPrompt {
  id: string;
  version: string;
  sha256: string;
}

export interface LearningHistoryOutcome {
  question_id: "Q1" | "Q2" | "Q3";
  question_kind: "code_specific" | "go_backend";
  verdict: "demonstrated" | "partial" | "not_demonstrated";
}

export interface LearningHistoryRecord {
  record_id: string;
  started_at: string;
  finished_at: string | null;
  status: "running" | "complete" | "failed" | "interrupted";
  failure_code: "evaluator_failed" | "evaluator_invalid_output" | "evaluator_timeout" | null;
  base_revision: string;
  head_revision: string;
  evidence_manifest_sha256: string;
  question_schema_version: number;
  assessment_schema_version: number;
  question_prompt: LearningHistoryPrompt;
  assessment_prompt: LearningHistoryPrompt;
  pi_version: string;
  provider: string;
  model_id: string;
  thinking_level: string;
  follow_up_used: boolean;
  label: "understood" | "partial" | "review_needed" | null;
  outcomes: LearningHistoryOutcome[];
}

export interface LearningHistoryResponse {
  protocol_version: 1;
  records: LearningHistoryRecord[];
}

export interface LearningHistoryClient {
  history(repository: string, limit: number): Promise<LearningHistoryResponse>;
}

export type AssessmentResult =
  | {
      turn: {
        schema_version: 1;
        disposition: "follow_up";
        follow_up: FollowUpQuestion;
        evaluations: [];
      };
    }
  | {
      turn: {
        schema_version: 1;
        disposition: "complete";
        follow_up: null;
        evaluations: [QuestionEvaluation, QuestionEvaluation, QuestionEvaluation];
      };
      label: "understood" | "partial" | "review_needed";
      history: HistorySave;
    };

export interface LearnClient extends EvidencePreviewClient {
  previewPiSession?(
    repository: string,
    piSessionID: string,
    selection: EvidenceSelection,
  ): Promise<EvidencePreviewResponse>;
  reviewedPiSessionIDs?(
    repository: string,
    piSessionIDs: string[],
  ): Promise<PiSessionReviewResponse>;
  questions(continuationID: string, selection: EvaluatorSelection): Promise<QuestionSet>;
  assess?(
    assessmentID: string,
    submission: AssessmentSubmission,
  ): Promise<AssessmentResult>;
}

export class EvidenceClientError extends Error {
  readonly code: EvidenceClientErrorCode;

  constructor(code: EvidenceClientErrorCode, message: string) {
    super(message);
    this.name = "EvidenceClientError";
    this.code = code;
  }
}

export type EvidenceClientErrorCode =
  | "daemon_unavailable"
  | "daemon_changed"
  | "invalid_runtime_state"
  | "protocol_mismatch"
  | "unauthorized"
  | "invalid_request"
  | "invalid_repository"
  | "invalid_revision"
  | "forbidden"
  | "source_unavailable"
  | "invalid_source"
  | "analysis_failed"
  | "deadline_exceeded"
  | "internal_error"
  | "continuation_unavailable"
  | "assessment_unavailable"
  | "evaluator_failed"
  | "evaluator_invalid_output"
  | "evaluator_unavailable"
  | "evaluator_timeout"
  | "history_unavailable";

export interface LearnCommandContext {
  cwd: string;
  hasUI: boolean;
  model?: {
    provider: string;
    id: string;
  };
  thinkingLevel?: string;
  isProjectTrusted(): boolean;
  sessionManager?: {
    getSessionDir(): string;
  };
  ui: {
    select(title: string, options: string[]): Promise<string | undefined>;
    input(title: string, placeholder?: string): Promise<string | undefined>;
    confirm(title: string, message: string): Promise<boolean>;
    notify(message: string, type?: "info" | "warning" | "error"): void;
  };
}

const COMMIT_RANGE = "Commit range";
const WORKING_TREE = "Working tree against a base revision";
const PI_SESSION = "Pi Session";
const MAX_PI_SESSION_CANDIDATES = 20;
const DEFAULT_HISTORY_LIMIT = 20;

export function createLearnHistoryCommand(client: LearningHistoryClient) {
  return async function learnHistoryCommand(args: string, context: LearnCommandContext): Promise<void> {
    if (args.trim() !== "") {
      context.ui.notify("/learn-history does not accept arguments. Run it from the repository you want to inspect.", "warning");
      return;
    }
    if (!context.hasUI) {
      context.ui.notify("/learn-history requires Pi's interactive UI.", "error");
      return;
    }
    if (!context.isProjectTrusted()) {
      context.ui.notify("Trust this project in Pi before using /learn-history.", "error");
      return;
    }

    try {
      const response = await client.history(context.cwd, DEFAULT_HISTORY_LIMIT);
      context.ui.notify(formatLearningHistory(response), "info");
    } catch (error) {
      if (error instanceof EvidenceClientError && error.code === "daemon_unavailable") {
        context.ui.notify(
          "Pi LearnLoop daemon is unavailable. Start it with `pi-learnloop daemon`, then run /learn-history again.",
          "error",
        );
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "unauthorized") {
        context.ui.notify(
          "Pi LearnLoop could not authenticate with the daemon. Restart `pi-learnloop daemon`, then run /learn-history again.",
          "error",
        );
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "invalid_repository") {
        context.ui.notify("The current directory is not inside a supported Git repository.", "error");
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "history_unavailable") {
        context.ui.notify(
          "Local learning history is unavailable. Pi LearnLoop left the database unchanged; check the daemon and data-directory compatibility before trying again.",
          "warning",
        );
        return;
      }
      context.ui.notify("Pi LearnLoop could not load local learning history. Run /learn-history again.", "error");
    }
  };
}

export function createLearnCommand(client: LearnClient, piVersion = "0.84.3", listPiSessions?: PiSessionLister) {
  return async function learnCommand(args: string, context: LearnCommandContext): Promise<void> {
    if (args.trim() !== "") {
      context.ui.notify("/learn does not accept arguments. Run it without arguments and choose a changeset.", "warning");
      return;
    }
    if (!context.hasUI) {
      context.ui.notify("/learn requires Pi's interactive UI.", "error");
      return;
    }
    if (!context.isProjectTrusted()) {
      context.ui.notify("Trust this project in Pi before using /learn.", "error");
      return;
    }

    let selectionKind = await context.ui.select("Choose what to review", [WORKING_TREE, COMMIT_RANGE, PI_SESSION]);
    if (selectionKind === undefined) {
      return;
    }

    let piSessionID: string | undefined;
    if (selectionKind === PI_SESSION) {
      if (
        listPiSessions === undefined ||
        context.sessionManager === undefined ||
        client.reviewedPiSessionIDs === undefined ||
        client.previewPiSession === undefined
      ) {
        context.ui.notify("Pi Session review is unavailable in this extension. Update Pi LearnLoop and run /learn again.", "warning");
        return;
      }

      let candidateIDs: string[];
      try {
        candidateIDs = projectPiSessionIDs(
          await listPiSessions(context.cwd, context.sessionManager.getSessionDir()),
        );
      } catch (error) {
        context.ui.notify(
          error instanceof PiSessionListError
            ? "Pi returned invalid Session identities. Pi LearnLoop did not select, send, or save any Session data."
            : "Pi could not list Sessions for this project. Pi LearnLoop did not select, send, or save any Session data.",
          "error",
        );
        return;
      }
      if (candidateIDs.length === 0) {
        context.ui.notify("No Pi Sessions are available for the current project.", "info");
        return;
      }

      let reviewed: PiSessionReviewResponse;
      try {
        reviewed = await client.reviewedPiSessionIDs(context.cwd, candidateIDs);
      } catch (error) {
        notifyPiSessionReviewQueryError(error, context);
        return;
      }
      const reviewedIDs = new Set(reviewed.reviewed_pi_session_ids);
      const availableIDs = candidateIDs.filter((id) => !reviewedIDs.has(id));
      if (availableIDs.length === 0) {
        context.ui.notify("All of the newest Pi Sessions for this project already have a completed review.", "info");
        return;
      }

      const selectedPiSessionID = await context.ui.select("Choose a Pi Session to review", availableIDs);
      if (selectedPiSessionID === undefined) {
        return;
      }
      if (!availableIDs.includes(selectedPiSessionID)) {
        context.ui.notify("That Pi Session is not available for review.", "error");
        return;
      }
      piSessionID = selectedPiSessionID;
      selectionKind = await context.ui.select(
        "Choose the Git changeset to associate with this Pi Session",
        [WORKING_TREE, COMMIT_RANGE],
      );
      if (selectionKind === undefined) {
        return;
      }
    }

    const selection = await collectSelection(selectionKind, context);
    if (selection === undefined) {
      return;
    }

    let response: EvidencePreviewResponse;
    try {
      response = piSessionID === undefined
        ? await client.preview(context.cwd, selection)
        : await client.previewPiSession!(context.cwd, piSessionID, selection);
    } catch (error) {
      if (error instanceof EvidenceClientError && error.code === "daemon_unavailable") {
        context.ui.notify(
          "Pi LearnLoop daemon is unavailable. Start it with `pi-learnloop daemon`, then run /learn again.",
          "error",
        );
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "unauthorized") {
        context.ui.notify(
          "Pi LearnLoop could not authenticate with the daemon. Restart `pi-learnloop daemon`, then run /learn again.",
          "error",
        );
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "invalid_revision") {
        context.ui.notify(
          "The selected Git revision could not be resolved. Check the base and head revisions, then run /learn again.",
          "error",
        );
        return;
      }
      context.ui.notify("Pi LearnLoop could not load the evidence preview. Run /learn again.", "error");
      return;
    }

    context.ui.notify(formatPreview(response, piSessionID), "info");
    if (response.continuation === undefined) {
      return;
    }
    if (!response.continuation.available) {
      if (response.continuation.reason !== "insufficient_evidence") {
        context.ui.notify(continuationUnavailableMessage(response.continuation.reason), "warning");
      }
      return;
    }

    const evaluatorSelection = activeEvaluatorSelection(context, piVersion);
    if (evaluatorSelection === undefined) {
      context.ui.notify(
        "Pi LearnLoop cannot continue because this Pi version or active model selection is unsupported.",
        "warning",
      );
      return;
    }
    const confirmed = await context.ui.confirm(
      "Generate learning questions?",
      "One evaluation will send only the selected excerpts shown above to your configured model. This may incur provider cost, and Pi/provider transport may retry transient network failures according to your Pi configuration. Continue?",
    );
    if (!confirmed) {
      return;
    }

    try {
      const questionSet = await client.questions(response.continuation.id, evaluatorSelection);
      context.ui.notify(formatQuestionSet(questionSet), questionSet.disposition === "questions" ? "info" : "warning");
      if (questionSet.disposition !== "questions" || questionSet.assessment === undefined) {
        return;
      }
      if (!questionSet.assessment.available) {
        if (questionSet.assessment.reason !== "insufficient_evidence") {
          context.ui.notify(assessmentUnavailableMessage(questionSet.assessment.reason), "warning");
        }
        return;
      }
      if (client.assess === undefined) {
        context.ui.notify("Answer assessment is unavailable in this client. Update Pi LearnLoop and run /learn again.", "warning");
        return;
      }

      const answers = await collectAnswers(questionSet, context);
      if (answers === undefined) {
        return;
      }
      const assessConfirmed = await context.ui.confirm(
        "Assess these answers?",
        "One evaluation will send the same selected excerpts and your three answers to the configured model. This may incur provider cost. If one follow-up is needed, submitting its answer causes one additional evaluation, and Pi/provider transport may retry according to your Pi configuration. Continue?",
      );
      if (!assessConfirmed) {
        return;
      }

      let assessment = await client.assess(questionSet.assessment.id, {
        stage: "initial_answers",
        answers,
      });
      if (assessment.turn.disposition === "follow_up") {
        context.ui.notify(formatFollowUp(assessment.turn.follow_up), "info");
        const followUpAnswer = await collectAnswer("Answer F1", assessment.turn.follow_up.text, context);
        if (followUpAnswer === undefined) {
          return;
        }
        assessment = await client.assess(questionSet.assessment.id, {
          stage: "follow_up_answer",
          follow_up_id: "F1",
          answer: followUpAnswer,
        });
        if (assessment.turn.disposition === "follow_up") {
          context.ui.notify("Pi LearnLoop rejected an unexpected second follow-up. Run /learn again.", "error");
          return;
        }
      }
      if (assessment.turn.disposition !== "complete" || !("label" in assessment)) {
        context.ui.notify("Pi LearnLoop received an invalid assessment result. Run /learn again.", "error");
        return;
      }
      context.ui.notify(formatAssessmentResult(assessment), "info");
      if (!assessment.history.saved) {
        context.ui.notify("The assessment completed, but local learning history could not be saved.", "warning");
      }
    } catch (error) {
      if (error instanceof EvidenceClientError && error.code === "continuation_unavailable") {
        context.ui.notify("This evidence preview expired or was already used. Run /learn again to review a new preview.", "warning");
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "evaluator_unavailable") {
        context.ui.notify("The question evaluator is unavailable. Run /learn again after it is ready.", "error");
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "assessment_unavailable") {
        context.ui.notify("This assessment expired or was already submitted. Run /learn again to start a new assessment.", "warning");
        return;
      }
      context.ui.notify("Pi LearnLoop could not generate learning questions. Run /learn again.", "error");
    }
  };
}

class PiSessionListError extends Error {}

function projectPiSessionIDs(sessions: readonly unknown[]): string[] {
  const ids: string[] = [];
  const seen = new Set<string>();
  for (let index = 0; index < Math.min(sessions.length, MAX_PI_SESSION_CANDIDATES); index += 1) {
    const session = sessions[index];
    if (!isObject(session) || !validPiSessionID(session.id)) {
      throw new PiSessionListError("invalid Pi Session identity");
    }
    if (!seen.has(session.id)) {
      seen.add(session.id);
      ids.push(session.id);
    }
  }
  return ids;
}

function validPiSessionID(value: unknown): value is string {
  return (
    typeof value === "string" &&
    Buffer.byteLength(value, "ascii") <= 128 &&
    /^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$/.test(value)
  );
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function notifyPiSessionReviewQueryError(error: unknown, context: LearnCommandContext): void {
  if (error instanceof EvidenceClientError && error.code === "history_unavailable") {
    context.ui.notify(
      "Pi Session review status is unavailable. Pi LearnLoop will not guess which Sessions are unreviewed; check the daemon and data directory, then run /learn again.",
      "warning",
    );
    return;
  }
  if (error instanceof EvidenceClientError && error.code === "daemon_unavailable") {
    context.ui.notify(
      "Pi LearnLoop daemon is unavailable. Start it with `pi-learnloop daemon`, then run /learn again.",
      "error",
    );
    return;
  }
  if (error instanceof EvidenceClientError && error.code === "unauthorized") {
    context.ui.notify(
      "Pi LearnLoop could not authenticate with the daemon. Restart `pi-learnloop daemon`, then run /learn again.",
      "error",
    );
    return;
  }
  if (error instanceof EvidenceClientError && error.code === "invalid_repository") {
    context.ui.notify("The current directory is not inside a supported Git repository.", "error");
    return;
  }
  context.ui.notify("Pi LearnLoop could not check completed Pi Session reviews. Run /learn again.", "error");
}

async function collectAnswers(questionSet: QuestionSet, context: LearnCommandContext): Promise<AssessmentAnswer[] | undefined> {
  const answers: AssessmentAnswer[] = [];
  for (const question of questionSet.questions) {
    const answer = await collectAnswer(`Answer ${question.id}`, question.text, context);
    if (answer === undefined) {
      return undefined;
    }
    answers.push({ question_id: question.id, text: answer });
  }
  return answers;
}

async function collectAnswer(title: string, question: string, context: LearnCommandContext): Promise<string | undefined> {
  const answer = (await context.ui.input(title, question))?.trim();
  if (answer === undefined || answer === "") {
    return undefined;
  }
  if (Buffer.byteLength(answer, "utf8") > 4_096 || /[\u0000-\u001f\u007f-\u009f]/u.test(answer)) {
    context.ui.notify("Answers must be at most 4 KiB and contain no control characters.", "warning");
    return undefined;
  }
  return answer;
}

function activeEvaluatorSelection(context: LearnCommandContext, piVersion: string): EvaluatorSelection | undefined {
  const provider = context.model?.provider;
  const id = context.model?.id;
  const thinkingLevel = context.thinkingLevel;
  if (
    piVersion !== "0.84.3" ||
    provider === undefined ||
    !validArgumentValue(provider, 128) ||
    id === undefined ||
    !validArgumentValue(id, 256) ||
    thinkingLevel === undefined ||
    !["off", "minimal", "low", "medium", "high", "xhigh", "max"].includes(thinkingLevel)
  ) {
    return undefined;
  }
  return { pi_version: piVersion, provider, id, thinking_level: thinkingLevel };
}

function validArgumentValue(value: string, maximumBytes: number): boolean {
  return (
    value.trim() !== "" &&
    !value.startsWith("-") &&
    Buffer.byteLength(value, "utf8") <= maximumBytes &&
    !/[\u0000-\u001f\u007f-\u009f]/u.test(value)
  );
}

function continuationUnavailableMessage(reason: "capacity" | "evaluator_unavailable"): string {
  return reason === "capacity"
    ? "The evidence preview is available, but the daemon has too many pending previews. Try /learn again shortly."
    : "The evidence preview is available, but the question evaluator is unavailable.";
}

function assessmentUnavailableMessage(reason: "capacity" | "evaluator_unavailable"): string {
  return reason === "capacity"
    ? "The questions are ready, but the daemon has too many pending assessments. Run /learn again shortly."
    : "The questions are ready, but answer assessment is not available yet.";
}

export function formatQuestionSet(questionSet: QuestionSet): string {
  if (questionSet.disposition === "insufficient_evidence") {
    return "The selected evidence is not sufficient to generate grounded learning questions.";
  }
  return [
    "Learning questions",
    ...questionSet.questions.map((question, index) => {
      const references = question.evidence_references.length > 0
        ? ` [Evidence: ${question.evidence_references.join(", ")}]`
        : "";
      return `${index + 1}. ${question.text}${references}`;
    }),
  ].join("\n");
}

function formatFollowUp(question: FollowUpQuestion): string {
  const references = question.evidence_references.length > 0
    ? ` [Evidence: ${question.evidence_references.join(", ")}]`
    : "";
  return `Follow-up for ${question.target_question_id}: ${question.text}${references}`;
}

export function formatAssessmentResult(result: Extract<AssessmentResult, { label: string }>): string {
  return [
    `Learning assessment: ${result.label}`,
    ...result.turn.evaluations.map((evaluation) => {
      const references = evaluation.evidence_references.length > 0
        ? ` [Evidence: ${evaluation.evidence_references.join(", ")}]`
        : "";
      return `${evaluation.question_id} — ${evaluation.verdict}: ${evaluation.feedback}${references}`;
    }),
  ].join("\n");
}

export function formatLearningHistory(response: LearningHistoryResponse): string {
  if (response.records.length === 0) {
    return "No learning history is available for this repository.";
  }
  return [
    `Learning history (${response.records.length} most recent)`,
    ...response.records.flatMap((record, index) => {
      const terminal = record.label ?? record.failure_code;
      const outcomes = record.outcomes.length === 0
        ? "No completed question outcomes"
        : record.outcomes.map((outcome) => `${outcome.question_id} ${outcome.question_kind} ${outcome.verdict}`).join(" · ");
      return [
        `${index + 1}. ${record.started_at} — ${record.status}${terminal === null ? "" : ` · ${terminal}`}`,
        `   Finished: ${record.finished_at ?? "not finished"} · Follow-up: ${record.follow_up_used ? "yes" : "no"}`,
        `   Revisions: ${shortRevision(record.base_revision)}..${shortRevision(record.head_revision)} · Evidence: ${shortHash(record.evidence_manifest_sha256)}`,
        `   ${outcomes}`,
        `   Evaluator: ${record.provider}/${record.model_id} · thinking=${record.thinking_level} · Pi ${record.pi_version} · Schemas Q${record.question_schema_version}/A${record.assessment_schema_version}`,
        `   Prompts: ${record.question_prompt.id}@${record.question_prompt.version}#${shortHash(record.question_prompt.sha256)} · ${record.assessment_prompt.id}@${record.assessment_prompt.version}#${shortHash(record.assessment_prompt.sha256)}`,
        `   Record: ${record.record_id}`,
      ];
    }),
  ].join("\n");
}

function shortRevision(value: string): string {
  return value.length > 12 ? value.slice(0, 12) : value;
}

function shortHash(value: string): string {
  return value.slice(0, 12);
}

async function collectSelection(
  selectionKind: string,
  context: LearnCommandContext,
): Promise<EvidenceSelection | undefined> {
  if (selectionKind !== COMMIT_RANGE && selectionKind !== WORKING_TREE) {
    context.ui.notify("That changeset type is not supported.", "error");
    return undefined;
  }

  const base = (await context.ui.input("Base Git revision", "For example: HEAD or main"))?.trim();
  if (!base) {
    context.ui.notify("A base Git revision is required.", "warning");
    return undefined;
  }
  if (selectionKind === WORKING_TREE) {
    return { kind: "working_tree", base };
  }

  const head = (await context.ui.input("Head Git revision", "For example: HEAD"))?.trim();
  if (!head) {
    context.ui.notify("A head Git revision is required for a commit range.", "warning");
    return undefined;
  }
  return { kind: "commit_range", base, head };
}

export function formatPreview(response: EvidencePreviewResponse, piSessionID?: string): string {
  const { preview } = response;
  const declarations = preview.files.flatMap((file) => file.declarations);
  const excerptBytes = declarations.reduce(
    (total, declaration) => total + Buffer.byteLength(declaration.excerpt, "utf8"),
    0,
  );
  const fileLines = preview.files.map((file) => {
    const symbols = file.declarations.map((declaration) => declaration.identity).join(", ");
    return `- ${file.path} (${file.status}): ${symbols || "no mapped declarations"}`;
  });
  if (fileLines.length === 0) {
    fileLines.push("No changed Go code was found for this selection. Choose another revision or change Go code, then run /learn again.");
  }
  const truncation = preview.truncation.truncated
    ? `Truncated: ${preview.truncation.omitted_files} files, ${preview.truncation.omitted_declarations} symbols, ${preview.truncation.omitted_excerpt_bytes} excerpt bytes omitted`
    : "Truncation: none";

  return [
    "Evidence preview",
    ...(piSessionID === undefined
      ? [`Selection: ${preview.base_revision}..${preview.head_revision}`]
      : [`User-selected association: Pi Session ${piSessionID} ↔ Git changeset ${preview.base_revision}..${preview.head_revision}`]),
    `Files: ${preview.files.length} | Symbols: ${declarations.length} | Approx. excerpt: ${excerptBytes} bytes`,
    ...fileLines,
    truncation,
    "Preview only: no model was called and nothing was saved.",
  ].join("\n");
}
