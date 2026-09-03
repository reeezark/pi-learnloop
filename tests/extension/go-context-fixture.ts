import { createHash } from "node:crypto";

import type { GoContextPreview } from "../../extensions/lib/learn-command.ts";

export function completeGoContext(): GoContextPreview {
  return {
    status: "complete",
    build: {
      goos: "darwin",
      goarch: "arm64",
      cgo_enabled: false,
      build_tags: [],
      tool_tags: [],
      release_tags: [],
      toolchain_version: "go1.21.13",
      test_variant: false,
      modules: [],
      workspaces: [],
      replacements: [],
    },
    applied_limits: {
      max_changed_files: 20,
      max_module_roots: 8,
      max_packages: 32,
      max_files_per_package: 64,
      max_files: 160,
      max_directory_entries: 256,
      max_source_bytes_per_file: 262_144,
      max_source_bytes: 2_097_152,
      max_direct_import_edges: 256,
      analysis_timeout_millis: 30_000,
      max_output_files: 20,
      max_output_items: 40,
      max_relations: 100,
      max_excerpt_bytes: 4_096,
      max_output_bytes: 65_536,
      max_evaluator_input_bytes: 262_144,
    },
    analyzed_package_count: 0,
    analyzed_file_count: 0,
    analyzed_source_bytes: 0,
    direct_import_edges: 0,
    item_count: 0,
    relation_count: 0,
    approximate_bytes: 0,
    items: [],
    relations: [],
    omissions: [],
    truncation: {
      truncated: false,
      omitted_files: 0,
      omitted_items: 0,
      omitted_relations: 0,
      omitted_bytes: 0,
    },
  };
}

export function partialGoContextWithChangedImport(): GoContextPreview {
  const context = completeGoContext();
  const content = '"example.com/dep"';
  const module = { path: "example.com/repo", directory: "", go_version: "1.21", toolchain: "" };
  const item = {
    reference: "C001",
    kind: "changed_import" as const,
    path: "main.go",
    package_path: "example.com/repo",
    declaration_kind: "" as const,
    identity: "example.com/dep",
    start_line: 3,
    end_line: 3,
    content,
    content_bytes: Buffer.byteLength(content, "utf8"),
    content_sha256: createHash("sha256").update(content, "utf8").digest("hex"),
    truncated: false,
  };
  const relation = {
    from: "main.go:import:example.com/dep",
    to: "C001",
    kind: "imports" as const,
    strength: "syntactic" as const,
  };
  context.status = "partial";
  context.build.modules = [module];
  context.analyzed_package_count = 1;
  context.analyzed_file_count = 1;
  context.analyzed_source_bytes = 96;
  context.direct_import_edges = 1;
  context.item_count = 1;
  context.relation_count = 1;
  context.items = [item];
  context.relations = [relation];
  context.omissions = [
    { reason: "external_type_unavailable", count: 1 },
    { reason: "output_truncated", count: 1 },
  ];
  context.truncation = {
    truncated: true,
    omitted_files: 0,
    omitted_items: 1,
    omitted_relations: 0,
    omitted_bytes: 12,
  };
  context.approximate_bytes = byteLength(
    module.path,
    module.directory,
    module.go_version,
    module.toolchain,
    item.path,
    item.package_path,
    item.identity,
    item.content,
    relation.from,
    relation.to,
  );
  return context;
}

export function unavailableGoContext(): GoContextPreview {
  const context = completeGoContext();
  context.status = "unavailable";
  context.omissions = [{ reason: "unsupported_module_layout", count: 1 }];
  return context;
}

function byteLength(...values: string[]): number {
  return values.reduce((total, value) => total + Buffer.byteLength(value, "utf8"), 0);
}
