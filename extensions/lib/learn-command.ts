import { createHash } from "node:crypto";

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

export interface GoContextModule {
  path: string;
  directory: string;
  go_version: string;
  toolchain: string;
}

export interface GoContextWorkspace {
  directory: string;
  go_version: string;
  toolchain: string;
}

export interface GoContextReplacement {
  module_path: string;
  directory: string;
  repository_local: boolean;
}

export interface GoContextBuild {
  goos: string;
  goarch: string;
  cgo_enabled: false;
  build_tags: string[];
  tool_tags: string[];
  release_tags: string[];
  toolchain_version: string;
  test_variant: boolean;
  modules: GoContextModule[];
  workspaces: GoContextWorkspace[];
  replacements: GoContextReplacement[];
}

export interface GoContextAppliedLimits {
  max_changed_files: 20;
  max_module_roots: 8;
  max_packages: 32;
  max_files_per_package: 64;
  max_files: 160;
  max_directory_entries: 256;
  max_source_bytes_per_file: 262144;
  max_source_bytes: 2097152;
  max_direct_import_edges: 256;
  analysis_timeout_millis: 30000;
  max_output_files: 20;
  max_output_items: 40;
  max_relations: 100;
  max_excerpt_bytes: 4096;
  max_output_bytes: 65536;
  max_evaluator_input_bytes: 262144;
}

export interface GoContextItem {
  reference: string;
  kind: "changed_import" | "context_declaration";
  path: string;
  package_path: string;
  declaration_kind: "" | EvidenceDeclaration["kind"];
  identity: string;
  start_line: number;
  end_line: number;
  content: string;
  content_bytes: number;
  content_sha256: string;
  truncated: boolean;
}

export interface GoContextRelation {
  from: string;
  to: string;
  kind: "imports" | "references" | "implements";
  strength: "syntactic" | "type_checked";
}

export interface GoContextPreview {
  status: "complete" | "partial" | "unavailable";
  build: GoContextBuild;
  applied_limits: GoContextAppliedLimits;
  analyzed_package_count: number;
  analyzed_file_count: number;
  analyzed_source_bytes: number;
  direct_import_edges: number;
  item_count: number;
  relation_count: number;
  approximate_bytes: number;
  items: GoContextItem[];
  relations: GoContextRelation[];
  omissions: Array<{
    reason:
      | "analysis_limit_exceeded"
      | "unsupported_module_layout"
      | "unsupported_go_version"
      | "outside_repository_dependency"
      | "cgo_unsupported"
      | "external_type_unavailable"
      | "context_parse_error"
      | "type_incomplete"
      | "output_truncated";
    count: number;
  }>;
  truncation: {
    truncated: boolean;
    omitted_files: number;
    omitted_items: number;
    omitted_relations: number;
    omitted_bytes: number;
  };
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
    go_context: GoContextPreview;
    truncation: {
      truncated: boolean;
      omitted_files: number;
      omitted_declarations: number;
      omitted_excerpt_bytes: number;
    };
  };
  continuation:
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
    editor(title: string, prefill?: string): Promise<string | undefined>;
    confirm(title: string, message: string): Promise<boolean>;
    notify(message: string, type?: "info" | "warning" | "error"): void;
  };
}

const COMMIT_RANGE = "Commit range";
const WORKING_TREE = "Working tree against a base revision";
const PI_SESSION = "Pi Session";
const MAX_PI_SESSION_CANDIDATES = 20;
const DEFAULT_HISTORY_LIMIT = 20;
const CONTINUE_TO_SHARING = "Continue to sharing confirmation";
const CANCEL_ANSWER_REVIEW = "Cancel";
const EDIT_ANSWER_ACTIONS = ["Edit Q1", "Edit Q2", "Edit Q3"];
const ANSWER_REVIEW_OPTIONS = [CONTINUE_TO_SHARING, ...EDIT_ANSWER_ACTIONS, CANCEL_ANSWER_REVIEW];
const INVALID_ANSWER_MESSAGE =
  "Answers must be non-empty after trimming, valid UTF-8, at most 4 KiB, and contain no control characters other than line feeds.";
const ANSWER_EDITOR_DISCLOSURE =
  "LearnLoop does not save answer drafts and keeps accepted answers only for this interaction. If you explicitly invoke Pi's external-editor shortcut, Pi writes the current draft to a temporary prompt.md, starts your configured editor, and attempts cleanup on a best-effort basis. The editor or your environment may retain swap, backup, recovery, history, or telemetry artifacts. Pi may materialize an oversized draft before LearnLoop can enforce the 4 KiB answer limit. Declining sends no answer. Continue?";

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
    const disclosedEvidenceBytes = approximateDisclosedEvidenceBytes(response);
    const modelDescription = `${displayInline(evaluatorSelection.provider)}/${displayInline(evaluatorSelection.id)} (thinking=${evaluatorSelection.thinking_level})`;
    const confirmed = await context.ui.confirm(
      "Generate learning questions?",
      `Model: ${modelDescription}. One evaluation will send the changed Go file/declaration metadata and excerpts, selected-snapshot Go context items and metadata, relationships, build configuration, limits, completeness, omissions, and truncation shown above. Estimated repository-derived evidence is ${disclosedEvidenceBytes} bytes; the complete evaluator input is capped at ${response.preview.go_context.applied_limits.max_evaluator_input_bytes} bytes. Pi LearnLoop does not know this provider's price, so the call may incur provider cost. Pi LearnLoop configures zero model retries. Continue?`,
    );
    if (!confirmed) {
      return;
    }

    let evaluationStage: "question" | "assessment" = "question";
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

      const editorConfirmed = await context.ui.confirm("Use the multiline answer editor?", ANSWER_EDITOR_DISCLOSURE);
      if (!editorConfirmed) {
        return;
      }
      const answers = await collectAnswers(questionSet, context);
      if (answers === undefined) {
        return;
      }
      const assessConfirmed = await context.ui.confirm(
        "Assess these answers?",
        `Model: ${modelDescription}. One evaluation will resend the same ${disclosedEvidenceBytes} bytes of displayed repository-derived evidence together with your three answers. Pi LearnLoop does not know this provider's price, so the call may incur provider cost. If one follow-up is needed, submitting its answer causes one additional evaluation. Pi LearnLoop configures zero model retries. Continue?`,
      );
      if (!assessConfirmed) {
        return;
      }

      evaluationStage = "assessment";
      let assessment = await client.assess(questionSet.assessment.id, {
        stage: "initial_answers",
        answers,
      });
      if (assessment.turn.disposition === "follow_up") {
        context.ui.notify(formatFollowUp(assessment.turn.follow_up), "info");
        const followUpAnswer = await collectAnswer("Answer F1", context);
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
      notifyEvaluationError(error, evaluationStage, context);
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
    const answer = await collectAnswer(`Answer ${question.id}`, context);
    if (answer === undefined) {
      return undefined;
    }
    answers.push({ question_id: question.id, text: answer });
  }

  while (true) {
    const action = await context.ui.select("Review answers", [...ANSWER_REVIEW_OPTIONS]);
    if (action === undefined || action === CANCEL_ANSWER_REVIEW) {
      return undefined;
    }
    if (action === CONTINUE_TO_SHARING) {
      return answers;
    }
    const answerIndex = EDIT_ANSWER_ACTIONS.indexOf(action);
    if (answerIndex === -1 || answers[answerIndex] === undefined) {
      context.ui.notify("Pi returned an unsupported answer review action. No answers were sent.", "error");
      return undefined;
    }

    const previousAnswer = answers[answerIndex].text;
    const replacement = await collectAnswer(`Answer Q${answerIndex + 1}`, context, previousAnswer);
    if (replacement !== undefined) {
      answers[answerIndex] = { ...answers[answerIndex], text: replacement };
    }
  }
}

async function collectAnswer(
  title: string,
  context: LearnCommandContext,
  acceptedAnswer?: string,
): Promise<string | undefined> {
  while (true) {
    const candidate = acceptedAnswer === undefined
      ? await context.ui.editor(title)
      : await context.ui.editor(title, acceptedAnswer);
    if (candidate === undefined) {
      return undefined;
    }
    const answer = candidate.trim();
    if (!validAnswer(candidate, answer)) {
      context.ui.notify(INVALID_ANSWER_MESSAGE, "warning");
      continue;
    }
    return answer;
  }
}

function validAnswer(candidate: string, answer: string): boolean {
  return (
    answer !== "" &&
    validUnicodeString(candidate) &&
    Buffer.byteLength(answer, "utf8") <= 4_096 &&
    !/[\u0000-\u0009\u000b-\u001f\u007f-\u009f]/u.test(candidate)
  );
}

function validUnicodeString(value: string): boolean {
  for (let index = 0; index < value.length; index += 1) {
    const codeUnit = value.charCodeAt(index);
    if (codeUnit >= 0xd800 && codeUnit <= 0xdbff) {
      const nextCodeUnit = value.charCodeAt(index + 1);
      if (!(nextCodeUnit >= 0xdc00 && nextCodeUnit <= 0xdfff)) {
        return false;
      }
      index += 1;
    } else if (codeUnit >= 0xdc00 && codeUnit <= 0xdfff) {
      return false;
    }
  }
  return true;
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

function notifyEvaluationError(
  error: unknown,
  stage: "question" | "assessment",
  context: LearnCommandContext,
): void {
  const subject = stage === "question" ? "Question generation" : "Answer assessment";
  const responseKind = stage === "question" ? "question-generation" : "assessment";
  if (error instanceof EvidenceClientError && error.code === "continuation_unavailable" && stage === "question") {
    context.ui.notify("Question generation did not start because this evidence preview expired or was already used. The request was not retried. Run /learn again to review a new preview.", "warning");
    return;
  }
  if (error instanceof EvidenceClientError && error.code === "assessment_unavailable" && stage === "assessment") {
    context.ui.notify("Answer assessment did not start because this assessment expired or was already submitted. The request was not retried. Run /learn again to start a new assessment.", "warning");
    return;
  }
  if (error instanceof EvidenceClientError) {
    switch (error.code) {
      case "evaluator_unavailable":
        context.ui.notify(`${subject} could not start because the Pi model runtime is unavailable. Initialization may have failed before any provider request. Verify the foreground daemon uses Pi 0.84.3 and Node 22.19.0 or newer, restart it, then run /learn again. The request was not retried.`, "error");
        return;
      case "evaluator_failed":
        context.ui.notify(`${subject} failed. The provider may or may not have received the request. The request was not retried; run /learn again to start a new review.`, "error");
        return;
      case "evaluator_timeout":
        context.ui.notify(`${subject} timed out. The provider may still have received the request. The request was not retried; run /learn again to start a new review.`, "error");
        return;
      case "evaluator_invalid_output":
        context.ui.notify(`${subject} returned output that Pi LearnLoop could not safely accept. The request was not retried; run /learn again to start a new review.`, "error");
        return;
      case "daemon_unavailable":
      case "daemon_changed":
        context.ui.notify(`The local daemon connection was lost during ${subject.toLowerCase()}. The outcome is unknown, and the request was not retried. Restart the foreground daemon, then run /learn again.`, "error");
        return;
      case "protocol_mismatch":
      case "continuation_unavailable":
      case "assessment_unavailable":
        context.ui.notify(`The daemon returned an incompatible ${responseKind} response. Update the daemon and extension together. The request was not retried; run /learn again.`, "error");
        return;
      case "invalid_runtime_state":
      case "unauthorized":
        context.ui.notify(`The local daemon runtime state changed during ${subject.toLowerCase()}. Restart the foreground daemon. The request was not retried; run /learn again.`, "error");
        return;
      case "invalid_request":
        context.ui.notify(
          `Pi LearnLoop daemon rejected the ${stage} request. Update the daemon and extension together, then run /learn again. The extension did not retry or alter an answer.`,
          "error",
        );
        return;
    }
  }
  context.ui.notify(`${subject} failed for an unknown local reason. The request was not retried; run /learn again.`, "error");
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
  let evidenceIndex = 0;
  const fileLines = preview.files.flatMap((file) => {
    const lines = [
      `- ${displayInline(file.path)} (${file.status}) · changed lines ${formatLineRanges(file.changed_lines)}`,
      `  File omissions: ${formatCounts(file.omissions)}`,
    ];
    if (file.declarations.length === 0) {
      lines.push("  Changed declarations: none");
      return lines;
    }
    for (const declaration of file.declarations) {
      evidenceIndex += 1;
      const reference = `E${String(evidenceIndex).padStart(3, "0")}`;
      const contentSHA256 = createHash("sha256").update(declaration.excerpt, "utf8").digest("hex");
      lines.push(
        `  ${reference} ${displayInline(declaration.identity)} · ${declaration.kind} · evidence_kind=${file.path.endsWith("_test.go") ? "test" : "code"} · lines ${declaration.start_line}-${declaration.end_line} · changed ${formatLineRanges(declaration.changed_lines)} · ${Buffer.byteLength(declaration.excerpt, "utf8")} bytes · sha256=${contentSHA256} · truncated=${declaration.excerpt_truncated}`,
        "    Excerpt (untrusted repository content; controls escaped for display):",
        indentEvidence(declaration.excerpt),
      );
    }
    return lines;
  });
  if (fileLines.length === 0) {
    fileLines.push("No changed Go code was found for this selection. Choose another revision or change Go code, then run /learn again.");
  }
  const truncation = preview.truncation.truncated
    ? `Truncated: ${preview.truncation.omitted_files} files, ${preview.truncation.omitted_declarations} symbols, ${preview.truncation.omitted_excerpt_bytes} excerpt bytes omitted`
    : "Truncation: none";

  const context = preview.go_context;
  const build = context.build;
  const contextItemLines = context.items.length === 0
    ? ["- none"]
    : context.items.flatMap((item) => [
      `- ${item.reference} ${displayInline(item.identity)} · ${item.kind}${item.declaration_kind === "" ? "" : `/${item.declaration_kind}`} · ${displayInline(item.path)}:${item.start_line}-${item.end_line} · package=${displayInline(item.package_path)} · ${item.content_bytes} bytes · sha256=${item.content_sha256} · truncated=${item.truncated}`,
      "  Content (untrusted repository content; controls escaped for display):",
      indentEvidence(item.content),
    ]);
  const relationLines = context.relations.length === 0
    ? ["- none"]
    : context.relations.map((relation) =>
      `- ${displayInline(relation.from)} --${relation.kind}/${relation.strength}--> ${displayInline(relation.to)}`
    );
  const moduleLines = build.modules.length === 0
    ? ["- Modules: none"]
    : build.modules.map((module) =>
      `- Module: ${displayInline(module.path)} · directory=${displayOptional(module.directory)} · go=${displayInline(module.go_version)} · toolchain=${displayOptional(module.toolchain)}`
    );
  const workspaceLines = build.workspaces.length === 0
    ? ["- Workspaces: none"]
    : build.workspaces.map((workspace) =>
      `- Workspace: directory=${displayOptional(workspace.directory)} · go=${displayOptional(workspace.go_version)} · toolchain=${displayOptional(workspace.toolchain)}`
    );
  const replacementLines = build.replacements.length === 0
    ? ["- Replacements: none"]
    : build.replacements.map((replacement) =>
      `- Replacement: ${displayInline(replacement.module_path)} · directory=${displayOptional(replacement.directory)} · repository_local=${replacement.repository_local}`
    );

  return [
    "Enriched evidence preview",
    ...(piSessionID === undefined
      ? [`Selection: ${preview.base_revision}..${preview.head_revision}`]
      : [`User-selected association: Pi Session ${piSessionID} ↔ Git changeset ${preview.base_revision}..${preview.head_revision}`]),
    `Changed evidence: ${preview.files.length} files · ${declarations.length} declarations · ${excerptBytes} excerpt bytes`,
    `Changed-evidence limits: files=${response.applied_limits.max_files} · declarations=${response.applied_limits.max_declarations} · excerpt_bytes=${response.applied_limits.max_excerpt_bytes}`,
    ...fileLines,
    `Changed-evidence ${truncation}`,
    "",
    `Go context: ${context.status}`,
    `Analysis totals: packages=${context.analyzed_package_count} · files=${context.analyzed_file_count} · source_bytes=${context.analyzed_source_bytes} · direct_import_edges=${context.direct_import_edges}`,
    `Selected context: items=${context.item_count} · relations=${context.relation_count} · repository-derived_bytes=${context.approximate_bytes}`,
    `Build: GOOS=${displayInline(build.goos)} · GOARCH=${displayInline(build.goarch)} · CGO_ENABLED=${build.cgo_enabled ? "1" : "0"} · test_variant=${build.test_variant} · toolchain=${displayInline(build.toolchain_version)}`,
    `Build tags: ${displayList(build.build_tags)} · Tool tags: ${displayList(build.tool_tags)} · Release tags: ${displayList(build.release_tags)}`,
    ...moduleLines,
    ...workspaceLines,
    ...replacementLines,
    "Context input limits:",
    `- changed_files=${context.applied_limits.max_changed_files} · module_roots=${context.applied_limits.max_module_roots} · packages=${context.applied_limits.max_packages} · files_per_package=${context.applied_limits.max_files_per_package} · files=${context.applied_limits.max_files} · directory_entries=${context.applied_limits.max_directory_entries}`,
    `- source_bytes_per_file=${context.applied_limits.max_source_bytes_per_file} · source_bytes=${context.applied_limits.max_source_bytes} · direct_import_edges=${context.applied_limits.max_direct_import_edges} · analysis_timeout_ms=${context.applied_limits.analysis_timeout_millis}`,
    "Context output limits:",
    `- files=${context.applied_limits.max_output_files} · items=${context.applied_limits.max_output_items} · relations=${context.applied_limits.max_relations} · excerpt_bytes=${context.applied_limits.max_excerpt_bytes} · repository-derived_bytes=${context.applied_limits.max_output_bytes} · evaluator_input_bytes=${context.applied_limits.max_evaluator_input_bytes}`,
    "Context evidence:",
    ...contextItemLines,
    "Context relationships:",
    ...relationLines,
    `Context omissions: ${formatCounts(context.omissions)}`,
    context.truncation.truncated
      ? `Context truncation: ${context.truncation.omitted_files} files, ${context.truncation.omitted_items} items, ${context.truncation.omitted_relations} relationships, ${context.truncation.omitted_bytes} bytes omitted`
      : "Context truncation: none",
    "Preview only: no model was called and nothing was saved.",
  ].join("\n");
}

function approximateDisclosedEvidenceBytes(response: EvidencePreviewResponse): number {
  const changedBytes = response.preview.files.reduce(
    (total, file) => total + file.declarations.reduce(
      (fileTotal, declaration) => fileTotal + Buffer.byteLength(declaration.excerpt, "utf8"),
      0,
    ),
    0,
  );
  return changedBytes + response.preview.go_context.approximate_bytes;
}

function formatLineRanges(ranges: LineRange[]): string {
  return ranges.length === 0 ? "none" : ranges.map((lineRange) =>
    lineRange.start === lineRange.end ? String(lineRange.start) : `${lineRange.start}-${lineRange.end}`
  ).join(", ");
}

function formatCounts(values: ReadonlyArray<{ reason: string; count: number }>): string {
  return values.length === 0 ? "none" : values.map((value) => `${value.reason}=${value.count}`).join(", ");
}

function displayList(values: string[]): string {
  return values.length === 0 ? "none" : values.map((value) => displayInline(value)).join(", ");
}

function displayOptional(value: string): string {
  return value === "" ? "none" : displayInline(value);
}

function indentEvidence(content: string): string {
  return displayInline(content, true).split("\n").map((line) => `      | ${line}`).join("\n");
}

function displayInline(value: string, preserveNewlines = false): string {
  let result = "";
  for (const character of value) {
    if (preserveNewlines && character === "\n") {
      result += character;
      continue;
    }
    if (/^[\p{Cc}\p{Cf}\p{Cs}]$/u.test(character)) {
      result += `\\u{${character.codePointAt(0)!.toString(16)}}`;
      continue;
    }
    result += character;
  }
  return result;
}
