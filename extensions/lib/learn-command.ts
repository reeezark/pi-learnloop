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
}

export interface LearnClient extends EvidencePreviewClient {
  questions(continuationID: string, selection: EvaluatorSelection): Promise<QuestionSet>;
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
  | "evaluator_failed"
  | "evaluator_invalid_output"
  | "evaluator_unavailable"
  | "evaluator_timeout";

export interface LearnCommandContext {
  cwd: string;
  hasUI: boolean;
  model?: {
    provider: string;
    id: string;
  };
  thinkingLevel?: string;
  isProjectTrusted(): boolean;
  ui: {
    select(title: string, options: string[]): Promise<string | undefined>;
    input(title: string, placeholder?: string): Promise<string | undefined>;
    confirm(title: string, message: string): Promise<boolean>;
    notify(message: string, type?: "info" | "warning" | "error"): void;
  };
}

const COMMIT_RANGE = "Commit range";
const WORKING_TREE = "Working tree against a base revision";

export function createLearnCommand(client: LearnClient, piVersion = "0.84.3") {
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

    const selectionKind = await context.ui.select("Choose the Go changeset to preview", [WORKING_TREE, COMMIT_RANGE]);
    if (selectionKind === undefined) {
      return;
    }

    const selection = await collectSelection(selectionKind, context);
    if (selection === undefined) {
      return;
    }

    let response: EvidencePreviewResponse;
    try {
      response = await client.preview(context.cwd, selection);
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

    context.ui.notify(formatPreview(response), "info");
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
    } catch (error) {
      if (error instanceof EvidenceClientError && error.code === "continuation_unavailable") {
        context.ui.notify("This evidence preview expired or was already used. Run /learn again to review a new preview.", "warning");
        return;
      }
      if (error instanceof EvidenceClientError && error.code === "evaluator_unavailable") {
        context.ui.notify("The question evaluator is unavailable. Run /learn again after it is ready.", "error");
        return;
      }
      context.ui.notify("Pi LearnLoop could not generate learning questions. Run /learn again.", "error");
    }
  };
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

export function formatPreview(response: EvidencePreviewResponse): string {
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
    `Selection: ${preview.base_revision}..${preview.head_revision}`,
    `Files: ${preview.files.length} | Symbols: ${declarations.length} | Approx. excerpt: ${excerptBytes} bytes`,
    ...fileLines,
    truncation,
    "Preview only: no model was called and nothing was saved.",
  ].join("\n");
}
