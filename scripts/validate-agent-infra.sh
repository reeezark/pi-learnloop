#!/bin/sh

set -u

usage() {
  echo "usage: scripts/validate-agent-infra.sh [--root PATH]" >&2
}

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
repo_root=$(CDPATH= cd -- "$script_dir/.." && pwd)

if [ "$#" -gt 0 ]; then
  if [ "$#" -ne 2 ] || [ "$1" != "--root" ]; then
    usage
    exit 2
  fi
  repo_root=$2
fi

if [ ! -d "$repo_root/plans" ] || [ ! -d "$repo_root/docs/checkpoints" ] || [ ! -d "$repo_root/docs/decisions" ]; then
  echo "error: root does not contain plans and docs lifecycle directories: $repo_root" >&2
  exit 2
fi

tmp_dir=$(mktemp -d "${TMPDIR:-/tmp}/pi-learnloop-agent-validate.XXXXXX") || exit 2
cleanup() {
  rm -f "$tmp_dir/plan-ids" "$tmp_dir/plan-phases" "$tmp_dir/checkpoint-current" "$tmp_dir/adr-ids" "$tmp_dir/adr-supersedes" "$tmp_dir/eval-case-ids" "$tmp_dir/eval-categories"
  rmdir "$tmp_dir" 2>/dev/null || true
}
trap cleanup EXIT HUP INT TERM

: > "$tmp_dir/plan-ids"
: > "$tmp_dir/plan-phases"
: > "$tmp_dir/checkpoint-current"
: > "$tmp_dir/adr-ids"
: > "$tmp_dir/adr-supersedes"
: > "$tmp_dir/eval-case-ids"
: > "$tmp_dir/eval-categories"

errors=0

error() {
  echo "error: $*" >&2
  errors=$((errors + 1))
}

frontmatter_value() {
  fm_file=$1
  fm_key=$2
  awk -v target="$fm_key" '
    NR == 1 {
      if ($0 != "---") {
        exit
      }
      next
    }
    $0 == "---" {
      exit
    }
    index($0, target ":") == 1 {
      value = substr($0, length(target) + 2)
      sub(/^[[:space:]]+/, "", value)
      sub(/[[:space:]]+$/, "", value)
      print value
      exit
    }
  ' "$fm_file"
}

is_date() {
  printf '%s\n' "$1" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}$'
}

is_semver() {
  printf '%s\n' "$1" | grep -Eq '^[0-9]+\.[0-9]+\.[0-9]+$'
}

sha256_file() {
  sha_file=$1
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "$sha_file" | awk '{ print $1 }'
  elif command -v sha256sum >/dev/null 2>&1; then
    sha256sum "$sha_file" | awk '{ print $1 }'
  elif command -v openssl >/dev/null 2>&1; then
    openssl dgst -sha256 "$sha_file" | awk '{ print $NF }'
  else
    return 2
  fi
}

json_backend=
if command -v python3 >/dev/null 2>&1; then
  json_backend=python
elif command -v osascript >/dev/null 2>&1; then
  json_backend=javascript
fi

json_lint() {
  json_file=$1
  case "$json_backend" in
    javascript)
      osascript -l JavaScript -e 'ObjC.import("Foundation"); function run(argv) { var data = $.NSData.dataWithContentsOfFile(argv[0]); var text = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js; JSON.parse(text); return "ok"; }' "$json_file" >/dev/null 2>&1
      ;;
    python)
      python3 -c 'import json, sys; json.load(open(sys.argv[1], encoding="utf-8"))' "$json_file" >/dev/null 2>&1
      ;;
    *)
      return 2
      ;;
  esac
}

json_value() {
  json_file=$1
  json_path=$2
  case "$json_backend" in
    javascript)
      osascript -l JavaScript -e 'ObjC.import("Foundation"); function run(argv) { var data = $.NSData.dataWithContentsOfFile(argv[0]); var text = $.NSString.alloc.initWithDataEncoding(data, $.NSUTF8StringEncoding).js; var value = JSON.parse(text); argv[1].split(".").forEach(function (key) { value = value[key]; }); if (Array.isArray(value)) { return String(value.length); } if (typeof value === "boolean") { return value ? "true" : "false"; } if (value === null || value === undefined) { throw new Error("missing JSON path"); } return String(value); }' "$json_file" "$json_path" 2>/dev/null
      ;;
    python)
      python3 -c 'import json, sys; from functools import reduce; value=reduce(lambda item, key: item[key], sys.argv[2].split("."), json.load(open(sys.argv[1], encoding="utf-8"))); print(len(value) if isinstance(value, list) else str(value).lower() if isinstance(value, bool) else value)' "$json_file" "$json_path" 2>/dev/null
      ;;
    *)
      return 2
      ;;
  esac
}

active_plans=0

for plan_file in "$repo_root"/plans/*.md; do
  [ -e "$plan_file" ] || continue
  [ "$(basename "$plan_file")" = "README.md" ] && continue

  plan_id=$(frontmatter_value "$plan_file" id)
  plan_status=$(frontmatter_value "$plan_file" status)
  plan_risk=$(frontmatter_value "$plan_file" risk)
  plan_phase=$(frontmatter_value "$plan_file" current_phase)
  plan_phase_status=$(frontmatter_value "$plan_file" phase_status)
  plan_updated=$(frontmatter_value "$plan_file" updated)

  [ -n "$plan_id" ] || error "$plan_file is missing frontmatter field 'id'"
  [ -n "$plan_status" ] || error "$plan_file is missing frontmatter field 'status'"
  [ -n "$plan_risk" ] || error "$plan_file is missing frontmatter field 'risk'"
  [ -n "$plan_phase" ] || error "$plan_file is missing frontmatter field 'current_phase'"
  [ -n "$plan_phase_status" ] || error "$plan_file is missing frontmatter field 'phase_status'"
  [ -n "$plan_updated" ] || error "$plan_file is missing frontmatter field 'updated'"

  if [ -n "$plan_id" ]; then
    if printf '%s\n' "$plan_id" | grep -Eq '^[a-z0-9]+(-[a-z0-9]+)*$'; then
      if [ "$(basename "$plan_file")" != "$plan_id.md" ]; then
        error "$plan_file name must match id '$plan_id'"
      fi
      if grep -Fqx "$plan_id" "$tmp_dir/plan-ids"; then
        error "duplicate plan id '$plan_id'"
      else
        printf '%s\n' "$plan_id" >> "$tmp_dir/plan-ids"
        printf '%s|%s\n' "$plan_id" "$plan_phase" >> "$tmp_dir/plan-phases"
      fi
    else
      error "$plan_file has invalid id '$plan_id'"
    fi
  fi

  case "$plan_status" in
    draft|approved|active|blocked|complete|superseded) ;;
    "") ;;
    *) error "$plan_file has invalid status '$plan_status'" ;;
  esac

  case "$plan_risk" in
    low|medium|high) ;;
    "") ;;
    *) error "$plan_file has invalid risk '$plan_risk'" ;;
  esac

  case "$plan_phase_status" in
    planned|awaiting_approval|authorized|in_progress|blocked|complete) ;;
    "") ;;
    *) error "$plan_file has invalid phase_status '$plan_phase_status'" ;;
  esac

  if [ -n "$plan_phase" ] && ! printf '%s\n' "$plan_phase" | grep -Eq '^[1-9][0-9]*$'; then
    error "$plan_file has invalid current_phase '$plan_phase'"
  fi

  if [ -n "$plan_updated" ] && ! is_date "$plan_updated"; then
    error "$plan_file has invalid updated date '$plan_updated'"
  fi

  [ "$plan_status" = "active" ] && active_plans=$((active_plans + 1))
  [ "$plan_status" = "draft" ] && [ "$plan_phase_status" != "planned" ] && error "$plan_file draft plan must have phase_status 'planned'"
  [ "$plan_status" = "approved" ] && [ "$plan_phase_status" != "authorized" ] && error "$plan_file approved plan must have phase_status 'authorized'"
  if [ "$plan_status" = "active" ]; then
    case "$plan_phase_status" in
      in_progress|awaiting_approval) ;;
      *) error "$plan_file active plan must have phase_status 'in_progress' or 'awaiting_approval'" ;;
    esac
  fi
  [ "$plan_status" = "blocked" ] && [ "$plan_phase_status" != "blocked" ] && error "$plan_file blocked plan must have phase_status 'blocked'"
  [ "$plan_status" = "complete" ] && [ "$plan_phase_status" != "complete" ] && error "$plan_file complete plan must have phase_status 'complete'"
done

if [ "$active_plans" -gt 1 ]; then
  error "at most one plan may have status 'active'"
fi

for checkpoint_file in "$repo_root"/docs/checkpoints/*.md; do
  [ -e "$checkpoint_file" ] || continue
  [ "$(basename "$checkpoint_file")" = "README.md" ] && continue

  checkpoint_id=$(frontmatter_value "$checkpoint_file" id)
  checkpoint_plan=$(frontmatter_value "$checkpoint_file" plan)
  checkpoint_phase=$(frontmatter_value "$checkpoint_file" phase)
  checkpoint_status=$(frontmatter_value "$checkpoint_file" status)
  checkpoint_updated=$(frontmatter_value "$checkpoint_file" updated)

  [ -n "$checkpoint_id" ] || error "$checkpoint_file is missing frontmatter field 'id'"
  [ -n "$checkpoint_plan" ] || error "$checkpoint_file is missing frontmatter field 'plan'"
  [ -n "$checkpoint_phase" ] || error "$checkpoint_file is missing frontmatter field 'phase'"
  [ -n "$checkpoint_status" ] || error "$checkpoint_file is missing frontmatter field 'status'"
  [ -n "$checkpoint_updated" ] || error "$checkpoint_file is missing frontmatter field 'updated'"

  [ -n "$checkpoint_id" ] && [ "$(basename "$checkpoint_file")" != "$checkpoint_id.md" ] && error "$checkpoint_file name must match id '$checkpoint_id'"
  if [ -n "$checkpoint_id" ] && [ -n "$checkpoint_plan" ] && [ -n "$checkpoint_phase" ]; then
    expected_checkpoint_id="$checkpoint_plan-phase-$checkpoint_phase"
    [ "$checkpoint_id" = "$expected_checkpoint_id" ] || error "$checkpoint_file id must be '$expected_checkpoint_id'"
  fi
  [ -n "$checkpoint_plan" ] && ! grep -Fqx "$checkpoint_plan" "$tmp_dir/plan-ids" && error "$checkpoint_file references unknown plan '$checkpoint_plan'"

  if [ -n "$checkpoint_phase" ] && ! printf '%s\n' "$checkpoint_phase" | grep -Eq '^[1-9][0-9]*$'; then
    error "$checkpoint_file has invalid phase '$checkpoint_phase'"
  fi

  if [ -n "$checkpoint_plan" ] && [ -n "$checkpoint_phase" ] && printf '%s\n' "$checkpoint_phase" | grep -Eq '^[1-9][0-9]*$'; then
    plan_current_phase=$(awk -F '|' -v plan="$checkpoint_plan" '$1 == plan { print $2; exit }' "$tmp_dir/plan-phases")
    if [ -n "$plan_current_phase" ] && [ "$checkpoint_phase" -gt "$plan_current_phase" ]; then
      error "$checkpoint_file phase '$checkpoint_phase' exceeds plan '$checkpoint_plan' current phase '$plan_current_phase'"
    fi
  fi

  case "$checkpoint_status" in
    current|superseded) ;;
    "") ;;
    *) error "$checkpoint_file has invalid status '$checkpoint_status'" ;;
  esac

  if [ -n "$checkpoint_updated" ] && ! is_date "$checkpoint_updated"; then
    error "$checkpoint_file has invalid updated date '$checkpoint_updated'"
  fi

  if [ "$checkpoint_status" = "current" ] && [ -n "$checkpoint_plan" ]; then
    if grep -Fqx "$checkpoint_plan" "$tmp_dir/checkpoint-current"; then
      error "plan '$checkpoint_plan' has more than one current checkpoint"
    else
      printf '%s\n' "$checkpoint_plan" >> "$tmp_dir/checkpoint-current"
    fi
  fi
done

for adr_file in "$repo_root"/docs/decisions/*.md; do
  [ -e "$adr_file" ] || continue
  [ "$(basename "$adr_file")" = "README.md" ] && continue

  adr_id=$(frontmatter_value "$adr_file" id)
  adr_status=$(frontmatter_value "$adr_file" status)
  adr_date=$(frontmatter_value "$adr_file" date)
  adr_supersedes=$(frontmatter_value "$adr_file" supersedes)

  [ -n "$adr_id" ] || error "$adr_file is missing frontmatter field 'id'"
  [ -n "$adr_status" ] || error "$adr_file is missing frontmatter field 'status'"
  [ -n "$adr_date" ] || error "$adr_file is missing frontmatter field 'date'"
  [ -n "$adr_supersedes" ] || error "$adr_file is missing frontmatter field 'supersedes'"

  if [ -n "$adr_id" ]; then
    if printf '%s\n' "$adr_id" | grep -Eq '^ADR-[0-9]{4}$'; then
      grep -Fqx "$adr_id" "$tmp_dir/adr-ids" && error "duplicate ADR id '$adr_id'"
      printf '%s\n' "$adr_id" >> "$tmp_dir/adr-ids"
      case "$(basename "$adr_file")" in
        "$adr_id"-*.md) ;;
        *) error "$adr_file name must start with '$adr_id-'" ;;
      esac
    else
      error "$adr_file has invalid id '$adr_id'"
    fi
  fi

  case "$adr_status" in
    proposed|accepted|superseded|deprecated) ;;
    "") ;;
    *) error "$adr_file has invalid status '$adr_status'" ;;
  esac

  if [ -n "$adr_date" ] && ! is_date "$adr_date"; then
    error "$adr_file has invalid date '$adr_date'"
  fi

  [ -n "$adr_supersedes" ] && [ "$adr_supersedes" != "none" ] && printf '%s|%s\n' "$adr_file" "$adr_supersedes" >> "$tmp_dir/adr-supersedes"
done

while IFS='|' read -r superseding_file superseded_id; do
  [ -n "$superseded_id" ] || continue
  grep -Fqx "$superseded_id" "$tmp_dir/adr-ids" || error "$superseding_file supersedes unknown ADR '$superseded_id'"
done < "$tmp_dir/adr-supersedes"

if [ -d "$repo_root/agent" ]; then
  if [ -z "$json_backend" ]; then
    error "agent JSON validation requires macOS osascript or Python 3"
  fi

  for required_asset in \
    "$repo_root/agent/README.md" \
    "$repo_root/agent/prompts/README.md" \
    "$repo_root/agent/prompts/evaluator-question-generation/v1.0.0.md" \
    "$repo_root/agent/evals/README.md" \
    "$repo_root/agent/policies/evaluator-capabilities.json" \
    "$repo_root/agent/schemas/eval-case.schema.json" \
    "$repo_root/agent/schemas/run-record.schema.json"; do
    [ -f "$required_asset" ] || error "missing required evaluator asset '$required_asset'"
  done

  for asset_spec in \
    "$repo_root/agent/README.md|evaluator-development-module" \
    "$repo_root/agent/prompts/README.md|prompt-versioning-guide" \
    "$repo_root/agent/evals/README.md|evaluator-case-guide"; do
    asset_file=${asset_spec%%|*}
    expected_asset_id=${asset_spec#*|}
    asset_id=$(frontmatter_value "$asset_file" asset_id)
    asset_version=$(frontmatter_value "$asset_file" version)
    asset_status=$(frontmatter_value "$asset_file" status)
    [ "$asset_id" = "$expected_asset_id" ] || error "$asset_file must use asset_id '$expected_asset_id'"
    is_semver "$asset_version" || error "$asset_file has invalid version '$asset_version'"
    [ "$asset_status" = "development-contract" ] || error "$asset_file must have status 'development-contract'"
  done

  prompt_count=0
  for prompt_file in "$repo_root"/agent/prompts/*/v*.md; do
    [ -e "$prompt_file" ] || continue
    prompt_count=$((prompt_count + 1))

    prompt_id=$(frontmatter_value "$prompt_file" id)
    prompt_version=$(frontmatter_value "$prompt_file" version)
    prompt_status=$(frontmatter_value "$prompt_file" status)
    prompt_input_schema=$(frontmatter_value "$prompt_file" input_schema)
    prompt_output_schema=$(frontmatter_value "$prompt_file" output_schema)
    prompt_policy=$(frontmatter_value "$prompt_file" capability_policy)
    prompt_updated=$(frontmatter_value "$prompt_file" updated)
    prompt_directory=$(basename "$(dirname "$prompt_file")")
    prompt_filename=$(basename "$prompt_file")

    [ -n "$prompt_id" ] || error "$prompt_file is missing frontmatter field 'id'"
    [ "$prompt_id" = "$prompt_directory" ] || error "$prompt_file id must match its parent directory"
    is_semver "$prompt_version" || error "$prompt_file has invalid version '$prompt_version'"
    [ "$prompt_filename" = "v$prompt_version.md" ] || error "$prompt_file name must match version '$prompt_version'"
    case "$prompt_status" in
      draft|released|deprecated) ;;
      *) error "$prompt_file has invalid status '$prompt_status'" ;;
    esac
    [ -n "$prompt_input_schema" ] && [ "$prompt_input_schema" != "TODO" ] || error "$prompt_file must identify input_schema"
    [ -n "$prompt_output_schema" ] && [ "$prompt_output_schema" != "TODO" ] || error "$prompt_file must identify output_schema"
    [ "$prompt_policy" = "evaluator-capabilities@1.0.0" ] || error "$prompt_file must use evaluator-capabilities@1.0.0"
    is_date "$prompt_updated" || error "$prompt_file has invalid updated date '$prompt_updated'"
    grep -qi 'untrusted' "$prompt_file" || error "$prompt_file must treat evidence as untrusted"
    grep -q 'evidence_references' "$prompt_file" || error "$prompt_file must require evidence references"
    grep -q 'insufficient_evidence' "$prompt_file" || error "$prompt_file must define the insufficient-evidence result"

    if [ "$prompt_id" = "evaluator-question-generation" ] && [ "$prompt_version" = "1.0.0" ]; then
      [ "$prompt_status" = "released" ] || error "$prompt_file must be released"
      [ "$prompt_input_schema" = "evaluator-input@1" ] || error "$prompt_file must use evaluator-input@1"
      [ "$prompt_output_schema" = "evaluator-question-set@1" ] || error "$prompt_file must use evaluator-question-set@1"
    fi
  done
  [ "$prompt_count" -gt 0 ] || error "agent/prompts must contain at least one versioned prompt"

  for json_file in \
    "$repo_root"/agent/policies/*.json \
    "$repo_root"/agent/schemas/*.json \
    "$repo_root"/agent/evals/cases/*.json \
    "$repo_root"/agent/fixtures/run-record/*.json; do
    [ -e "$json_file" ] || continue
    if ! json_lint "$json_file"; then
      error "$json_file is not valid JSON"
    fi
  done

  policy_file="$repo_root/agent/policies/evaluator-capabilities.json"
  capability_policy_version=
  capability_policy_hash=
  if [ -f "$policy_file" ] && json_lint "$policy_file"; then
    policy_id=$(json_value "$policy_file" id) || policy_id=
    policy_version=$(json_value "$policy_file" version) || policy_version=
    policy_status=$(json_value "$policy_file" status) || policy_status=
    policy_default=$(json_value "$policy_file" default) || policy_default=
    session_isolation=$(json_value "$policy_file" session.isolation) || session_isolation=
    session_reuse=$(json_value "$policy_file" session.development_session_reuse) || session_reuse=
    tools_default=$(json_value "$policy_file" tools.default) || tools_default=
    tools_allowed=$(json_value "$policy_file" tools.allowed) || tools_allowed=
    filesystem_access=$(json_value "$policy_file" filesystem.access) || filesystem_access=
    process_spawn=$(json_value "$policy_file" process.spawn) || process_spawn=
    process_shell=$(json_value "$policy_file" process.shell) || process_shell=
    network_tools=$(json_value "$policy_file" network.tool_access) || network_tools=
    network_transport=$(json_value "$policy_file" network.model_transport) || network_transport=
    evidence_selection=$(json_value "$policy_file" evidence.selection) || evidence_selection=
    evidence_preview=$(json_value "$policy_file" evidence.user_preview_required) || evidence_preview=
    budget_required=$(json_value "$policy_file" evidence.budget_required) || budget_required=
    budget_fail_closed=$(json_value "$policy_file" evidence.fail_closed_without_budget) || budget_fail_closed=
    unselected_content=$(json_value "$policy_file" evidence.unselected_content_allowed) || unselected_content=
    evidence_untrusted=$(json_value "$policy_file" evidence.content_is_untrusted) || evidence_untrusted=
    credentials_exposed=$(json_value "$policy_file" credentials.exposed_to_evaluator) || credentials_exposed=
    credentials_persisted=$(json_value "$policy_file" credentials.persisted) || credentials_persisted=
    structured_output=$(json_value "$policy_file" output.structured_output_required) || structured_output=
    evidence_references=$(json_value "$policy_file" output.evidence_references_required) || evidence_references=
    insufficient_evidence=$(json_value "$policy_file" output.insufficient_evidence_path_required) || insufficient_evidence=
    telemetry_default=$(json_value "$policy_file" telemetry.uploaded_by_default) || telemetry_default=

    [ "$policy_id" = "evaluator-capabilities" ] || error "$policy_file must use id 'evaluator-capabilities'"
    is_semver "$policy_version" || error "$policy_file has invalid version '$policy_version'"
    [ "$policy_status" = "development-contract" ] || error "$policy_file status must be 'development-contract'"
    [ "$policy_default" = "deny" ] || error "$policy_file default must be 'deny'"
    [ "$session_isolation" = "required" ] || error "$policy_file session.isolation must be 'required'"
    [ "$session_reuse" = "forbidden" ] || error "$policy_file development Session reuse must be forbidden"
    [ "$tools_default" = "deny" ] || error "$policy_file tools.default must be 'deny'"
    [ "$tools_allowed" = "0" ] || error "$policy_file tools.allowed must be empty"
    [ "$filesystem_access" = "none" ] || error "$policy_file filesystem.access must be 'none'"
    [ "$process_spawn" = "forbidden" ] || error "$policy_file process.spawn must be 'forbidden'"
    [ "$process_shell" = "forbidden" ] || error "$policy_file process.shell must be 'forbidden'"
    [ "$network_tools" = "none" ] || error "$policy_file network.tool_access must be 'none'"
    [ "$network_transport" = "pi-managed" ] || error "$policy_file model transport must be Pi-managed"
    [ "$evidence_selection" = "selected_bundle_only" ] || error "$policy_file evidence.selection must be 'selected_bundle_only'"
    [ "$evidence_preview" = "true" ] || error "$policy_file evidence.user_preview_required must be true"
    [ "$budget_required" = "true" ] || error "$policy_file evidence.budget_required must be true"
    [ "$budget_fail_closed" = "true" ] || error "$policy_file evidence.fail_closed_without_budget must be true"
    [ "$unselected_content" = "false" ] || error "$policy_file must deny unselected evidence"
    [ "$evidence_untrusted" = "true" ] || error "$policy_file must treat evidence as untrusted"
    [ "$credentials_exposed" = "false" ] || error "$policy_file must not expose credentials"
    [ "$credentials_persisted" = "false" ] || error "$policy_file credentials.persisted must be false"
    [ "$structured_output" = "true" ] || error "$policy_file must require structured output"
    [ "$evidence_references" = "true" ] || error "$policy_file must require evidence references"
    [ "$insufficient_evidence" = "true" ] || error "$policy_file must require an insufficient-evidence path"
    [ "$telemetry_default" = "false" ] || error "$policy_file telemetry.uploaded_by_default must be false"
    capability_policy_version=$policy_version
    capability_policy_hash=$(sha256_file "$policy_file") || capability_policy_hash=
    [ -n "$capability_policy_hash" ] || error "a SHA-256 command is required to validate evaluator assets"
  fi

  eval_schema="$repo_root/agent/schemas/eval-case.schema.json"
  run_schema="$repo_root/agent/schemas/run-record.schema.json"
  if [ -f "$eval_schema" ] && json_lint "$eval_schema"; then
    eval_schema_id=$(json_value "$eval_schema" asset_id) || eval_schema_id=
    eval_schema_version=$(json_value "$eval_schema" version) || eval_schema_version=
    [ "$eval_schema_id" = "eval-case-schema" ] || error "$eval_schema must use asset_id 'eval-case-schema'"
    is_semver "$eval_schema_version" || error "$eval_schema has invalid version '$eval_schema_version'"
  fi
  if [ -f "$run_schema" ] && json_lint "$run_schema"; then
    run_schema_id=$(json_value "$run_schema" asset_id) || run_schema_id=
    run_schema_version=$(json_value "$run_schema" version) || run_schema_version=
    [ "$run_schema_id" = "run-record-schema" ] || error "$run_schema must use asset_id 'run-record-schema'"
    is_semver "$run_schema_version" || error "$run_schema has invalid version '$run_schema_version'"
  fi

  eval_case_count=0
  for case_file in "$repo_root"/agent/evals/cases/*.json; do
    [ -e "$case_file" ] || continue
    eval_case_count=$((eval_case_count + 1))
    json_lint "$case_file" || continue

    case_id=$(json_value "$case_file" case_id) || case_id=
    case_version=$(json_value "$case_file" case_version) || case_version=
    case_schema_version=$(json_value "$case_file" schema_version) || case_schema_version=
    case_category=$(json_value "$case_file" category) || case_category=
    case_description=$(json_value "$case_file" description) || case_description=
    evidence_count=$(json_value "$case_file" stimulus.evidence) || evidence_count=
    expected_decision=$(json_value "$case_file" expectations.decision) || expected_decision=
    required_signal_count=$(json_value "$case_file" expectations.required_signals) || required_signal_count=

    [ -n "$case_id" ] || error "$case_file is missing case_id"
    [ "$(basename "$case_file")" = "$case_id.json" ] || error "$case_file name must match case_id '$case_id'"
    if grep -Fqx "$case_id" "$tmp_dir/eval-case-ids"; then
      error "duplicate eval case id '$case_id'"
    else
      printf '%s\n' "$case_id" >> "$tmp_dir/eval-case-ids"
    fi
    is_semver "$case_version" || error "$case_file has invalid case_version '$case_version'"
    [ "$case_schema_version" = "1.0.0" ] || error "$case_file has unsupported schema_version '$case_schema_version'"
    [ -n "$case_description" ] || error "$case_file must have a description"
    printf '%s\n' "$evidence_count" | grep -Eq '^[1-9][0-9]*$' || error "$case_file must contain evidence"
    case "$expected_decision" in
      accept|reject|abstain) ;;
      *) error "$case_file has invalid expectations.decision '$expected_decision'" ;;
    esac
    printf '%s\n' "$required_signal_count" | grep -Eq '^[1-9][0-9]*$' || error "$case_file must define required signals"
    case "$case_category" in
      evidence_fidelity|insufficient_evidence|prompt_injection|structured_result)
        printf '%s\n' "$case_category" >> "$tmp_dir/eval-categories"
        ;;
      *) error "$case_file has invalid category '$case_category'" ;;
    esac
    grep -q '"synthetic"[[:space:]]*:[[:space:]]*true' "$case_file" || error "$case_file must contain synthetic evidence"
    grep -q '"synthetic"[[:space:]]*:[[:space:]]*false' "$case_file" && error "$case_file contains non-synthetic evidence"
  done

  [ "$eval_case_count" -gt 0 ] || error "agent/evals/cases must contain at least one case"
  for required_category in evidence_fidelity insufficient_evidence prompt_injection structured_result; do
    grep -Fqx "$required_category" "$tmp_dir/eval-categories" || error "missing eval category '$required_category'"
  done

  run_record_count=0
  for run_file in "$repo_root"/agent/fixtures/run-record/*.json; do
    [ -e "$run_file" ] || continue
    run_record_count=$((run_record_count + 1))
    json_lint "$run_file" || continue

    record_id=$(json_value "$run_file" record_id) || record_id=
    record_schema_version=$(json_value "$run_file" schema_version) || record_schema_version=
    record_created_at=$(json_value "$run_file" created_at) || record_created_at=
    record_task_id=$(json_value "$run_file" task_id) || record_task_id=
    adapter_id=$(json_value "$run_file" adapter.id) || adapter_id=
    adapter_version=$(json_value "$run_file" adapter.version) || adapter_version=
    adapter_kind=$(json_value "$run_file" adapter.kind) || adapter_kind=
    prompt_id=$(json_value "$run_file" prompt.id) || prompt_id=
    prompt_version=$(json_value "$run_file" prompt.version) || prompt_version=
    prompt_hash=$(json_value "$run_file" prompt.sha256) || prompt_hash=
    policy_id=$(json_value "$run_file" policy.id) || policy_id=
    policy_version=$(json_value "$run_file" policy.version) || policy_version=
    policy_hash=$(json_value "$run_file" policy.sha256) || policy_hash=
    evidence_hash=$(json_value "$run_file" inputs.evidence_manifest_sha256) || evidence_hash=
    model_provider=$(json_value "$run_file" model.provider) || model_provider=
    model_name=$(json_value "$run_file" model.name) || model_name=
    model_version=$(json_value "$run_file" model.version) || model_version=
    execution_status=$(json_value "$run_file" execution.status) || execution_status=
    execution_attempt=$(json_value "$run_file" execution.attempt) || execution_attempt=
    capability_decision_count=$(json_value "$run_file" execution.capability_decisions) || capability_decision_count=
    result_schema_id=$(json_value "$run_file" result.schema_id) || result_schema_id=
    result_schema_version=$(json_value "$run_file" result.schema_version) || result_schema_version=
    result_hash=$(json_value "$run_file" result.sha256) || result_hash=
    raw_source=$(json_value "$run_file" privacy.raw_source_persisted) || raw_source=
    stored_credentials=$(json_value "$run_file" privacy.credentials_persisted) || stored_credentials=
    uploaded_telemetry=$(json_value "$run_file" privacy.telemetry_uploaded) || uploaded_telemetry=

    [ "$(basename "$run_file")" = "$record_id.json" ] || error "$run_file name must match record_id '$record_id'"
    [ "$record_schema_version" = "1.0.0" ] || error "$run_file has unsupported schema_version '$record_schema_version'"
    printf '%s\n' "$record_created_at" | grep -Eq '^[0-9]{4}-[0-9]{2}-[0-9]{2}T' || error "$run_file has invalid created_at '$record_created_at'"
    [ -n "$record_task_id" ] || error "$run_file must identify task_id"
    [ -n "$adapter_id" ] || error "$run_file must identify adapter.id"
    is_semver "$adapter_version" || error "$run_file has invalid adapter.version '$adapter_version'"
    case "$adapter_kind" in
      fixture|pi-rpc) ;;
      *) error "$run_file has invalid adapter.kind '$adapter_kind'" ;;
    esac
    [ -n "$prompt_id" ] || error "$run_file must identify prompt.id"
    is_semver "$prompt_version" || error "$run_file has invalid prompt.version '$prompt_version'"
    [ "$policy_id" = "evaluator-capabilities" ] || error "$run_file must reference evaluator-capabilities policy"
    is_semver "$policy_version" || error "$run_file has invalid policy.version '$policy_version'"
    [ "$policy_version" = "$capability_policy_version" ] || error "$run_file policy.version does not match the capability policy"
    [ "$policy_hash" = "$capability_policy_hash" ] || error "$run_file policy.sha256 does not match the capability policy"
    [ -n "$model_provider" ] || error "$run_file must identify model.provider"
    [ -n "$model_name" ] || error "$run_file must identify model.name"
    [ -n "$model_version" ] || error "$run_file must identify model.version"
    case "$execution_status" in
      succeeded|failed|rejected) ;;
      *) error "$run_file has invalid execution.status '$execution_status'" ;;
    esac
    printf '%s\n' "$execution_attempt" | grep -Eq '^[1-9][0-9]*$' || error "$run_file has invalid execution.attempt '$execution_attempt'"
    printf '%s\n' "$capability_decision_count" | grep -Eq '^[1-9][0-9]*$' || error "$run_file must record capability decisions"
    [ -n "$result_schema_id" ] || error "$run_file must identify result.schema_id"
    is_semver "$result_schema_version" || error "$run_file has invalid result.schema_version '$result_schema_version'"
    for hash_value in "$prompt_hash" "$policy_hash" "$evidence_hash" "$result_hash"; do
      printf '%s\n' "$hash_value" | grep -Eq '^[a-f0-9]{64}$' || error "$run_file contains an invalid SHA-256 value"
    done
    [ "$raw_source" = "false" ] || error "$run_file must not persist raw source"
    [ "$stored_credentials" = "false" ] || error "$run_file must not persist credentials"
    [ "$uploaded_telemetry" = "false" ] || error "$run_file must not upload telemetry"
    grep -Eq '"(source_code|raw_code|source_content|evidence_content)"[[:space:]]*:' "$run_file" && error "$run_file contains a raw source-code field"
  done
  [ "$run_record_count" -gt 0 ] || error "agent/fixtures/run-record must contain at least one fixture"
fi

if [ "$errors" -ne 0 ]; then
  echo "Agent infrastructure validation failed with $errors error(s)." >&2
  exit 1
fi

echo "Agent infrastructure validation passed."
