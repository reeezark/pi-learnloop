#!/bin/sh

set -eu

script_dir=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
validator="$script_dir/validate-agent-infra.sh"
tmp_root=$(mktemp -d "${TMPDIR:-/tmp}/pi-learnloop-agent-test.XXXXXX")

cleanup() {
  case "$tmp_root" in
    "${TMPDIR:-/tmp}"/pi-learnloop-agent-test.*) rm -rf "$tmp_root" ;;
  esac
}
trap cleanup EXIT HUP INT TERM

new_fixture() {
  fixture_name=$1
  fixture_root="$tmp_root/$fixture_name"
  mkdir -p "$fixture_root/plans" "$fixture_root/docs/checkpoints" "$fixture_root/docs/decisions"
  if [ -d "$script_dir/../agent" ]; then
    cp -R "$script_dir/../agent" "$fixture_root/agent"
  fi

  cat > "$fixture_root/plans/example-task.md" <<'EOF'
---
id: example-task
status: active
risk: medium
current_phase: 1
phase_status: in_progress
updated: 2026-08-31
---

# Example Task
EOF

  cat > "$fixture_root/docs/checkpoints/example-task-phase-1.md" <<'EOF'
---
id: example-task-phase-1
plan: example-task
phase: 1
status: current
updated: 2026-08-31
---

# Context
EOF

  cat > "$fixture_root/docs/decisions/ADR-0001-example.md" <<'EOF'
---
id: ADR-0001
status: accepted
date: 2026-08-31
supersedes: none
---

# ADR-0001: Example
EOF

  printf '%s\n' "$fixture_root"
}

expect_pass() {
  test_name=$1
  test_root=$2
  if "$validator" --root "$test_root" >/dev/null 2>&1; then
    echo "ok - $test_name"
  else
    echo "not ok - $test_name" >&2
    "$validator" --root "$test_root" >&2 || true
    exit 1
  fi
}

expect_fail() {
  test_name=$1
  test_root=$2
  if "$validator" --root "$test_root" >/dev/null 2>&1; then
    echo "not ok - $test_name unexpectedly passed" >&2
    exit 1
  fi
  echo "ok - $test_name"
}

valid_root=$(new_fixture valid)
expect_pass "valid lifecycle artifacts" "$valid_root"

duplicate_active_root=$(new_fixture duplicate-active)
cat > "$duplicate_active_root/plans/second-task.md" <<'EOF'
---
id: second-task
status: active
risk: low
current_phase: 1
phase_status: in_progress
updated: 2026-08-31
---

# Second Task
EOF
expect_fail "multiple active plans" "$duplicate_active_root"

invalid_risk_root=$(new_fixture invalid-risk)
sed -i.bak 's/risk: medium/risk: critical/' "$invalid_risk_root/plans/example-task.md"
rm -f "$invalid_risk_root/plans/example-task.md.bak"
expect_fail "invalid plan risk" "$invalid_risk_root"

missing_metadata_root=$(new_fixture missing-metadata)
sed -i.bak '/^risk: medium$/d' "$missing_metadata_root/plans/example-task.md"
rm -f "$missing_metadata_root/plans/example-task.md.bak"
expect_fail "missing plan metadata" "$missing_metadata_root"

invalid_lifecycle_root=$(new_fixture invalid-lifecycle)
sed -i.bak 's/status: active/status: approved/' "$invalid_lifecycle_root/plans/example-task.md"
rm -f "$invalid_lifecycle_root/plans/example-task.md.bak"
expect_fail "invalid plan lifecycle combination" "$invalid_lifecycle_root"

unknown_plan_root=$(new_fixture unknown-plan)
sed -i.bak 's/plan: example-task/plan: missing-task/' "$unknown_plan_root/docs/checkpoints/example-task-phase-1.md"
rm -f "$unknown_plan_root/docs/checkpoints/example-task-phase-1.md.bak"
expect_fail "checkpoint references unknown plan" "$unknown_plan_root"

future_checkpoint_root=$(new_fixture future-checkpoint)
sed -i.bak -e 's/id: example-task-phase-1/id: example-task-phase-2/' -e 's/phase: 1/phase: 2/' "$future_checkpoint_root/docs/checkpoints/example-task-phase-1.md"
rm -f "$future_checkpoint_root/docs/checkpoints/example-task-phase-1.md.bak"
mv "$future_checkpoint_root/docs/checkpoints/example-task-phase-1.md" "$future_checkpoint_root/docs/checkpoints/example-task-phase-2.md"
expect_fail "checkpoint exceeds plan phase" "$future_checkpoint_root"

duplicate_checkpoint_root=$(new_fixture duplicate-checkpoint)
sed -i.bak 's/current_phase: 1/current_phase: 2/' "$duplicate_checkpoint_root/plans/example-task.md"
rm -f "$duplicate_checkpoint_root/plans/example-task.md.bak"
cp "$duplicate_checkpoint_root/docs/checkpoints/example-task-phase-1.md" "$duplicate_checkpoint_root/docs/checkpoints/example-task-phase-2.md"
sed -i.bak -e 's/id: example-task-phase-1/id: example-task-phase-2/' -e 's/phase: 1/phase: 2/' "$duplicate_checkpoint_root/docs/checkpoints/example-task-phase-2.md"
rm -f "$duplicate_checkpoint_root/docs/checkpoints/example-task-phase-2.md.bak"
expect_fail "multiple current checkpoints" "$duplicate_checkpoint_root"

invalid_adr_root=$(new_fixture invalid-adr)
sed -i.bak 's/status: accepted/status: final/' "$invalid_adr_root/docs/decisions/ADR-0001-example.md"
rm -f "$invalid_adr_root/docs/decisions/ADR-0001-example.md.bak"
expect_fail "invalid ADR status" "$invalid_adr_root"

unknown_supersedes_root=$(new_fixture unknown-supersedes)
sed -i.bak 's/supersedes: none/supersedes: ADR-9999/' "$unknown_supersedes_root/docs/decisions/ADR-0001-example.md"
rm -f "$unknown_supersedes_root/docs/decisions/ADR-0001-example.md.bak"
expect_fail "ADR supersedes unknown decision" "$unknown_supersedes_root"

permissive_policy_root=$(new_fixture permissive-policy)
sed -i.bak 's/"default": "deny"/"default": "allow"/' "$permissive_policy_root/agent/policies/evaluator-capabilities.json"
rm -f "$permissive_policy_root/agent/policies/evaluator-capabilities.json.bak"
expect_fail "capability policy must deny by default" "$permissive_policy_root"

invalid_json_root=$(new_fixture invalid-json)
sed -i.bak '$d' "$invalid_json_root/agent/policies/evaluator-capabilities.json"
rm -f "$invalid_json_root/agent/policies/evaluator-capabilities.json.bak"
expect_fail "invalid JSON is rejected" "$invalid_json_root"

missing_category_root=$(new_fixture missing-eval-category)
rm -f "$missing_category_root/agent/evals/cases/prompt-injection-in-evidence.json"
rm -f "$missing_category_root/agent/evals/cases/question-generation-injection.json"
expect_fail "all eval categories are required" "$missing_category_root"

invalid_case_version_root=$(new_fixture invalid-case-version)
sed -i.bak 's/"case_version": "1.0.0"/"case_version": "latest"/' "$invalid_case_version_root/agent/evals/cases/evidence-fidelity-unsupported-claim.json"
rm -f "$invalid_case_version_root/agent/evals/cases/evidence-fidelity-unsupported-claim.json.bak"
expect_fail "eval case version must be stable" "$invalid_case_version_root"

unsafe_run_record_root=$(new_fixture unsafe-run-record)
sed -i.bak 's/"raw_source_persisted": false/"raw_source_persisted": true/' "$unsafe_run_record_root/agent/fixtures/run-record/example-fixture-run.json"
rm -f "$unsafe_run_record_root/agent/fixtures/run-record/example-fixture-run.json.bak"
expect_fail "run record must not persist raw source" "$unsafe_run_record_root"

stale_policy_hash_root=$(new_fixture stale-policy-hash)
sed -i.bak 's/3d139c932c9296abe1f970f447e692556930476396243ccb021b34792138ab0b/eeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeeee/' "$stale_policy_hash_root/agent/fixtures/run-record/example-fixture-run.json"
rm -f "$stale_policy_hash_root/agent/fixtures/run-record/example-fixture-run.json.bak"
expect_fail "run record policy hash must match" "$stale_policy_hash_root"

missing_prompt_root=$(new_fixture missing-prompt)
rm -f "$missing_prompt_root/agent/prompts/evaluator-question-generation/v1.0.0.md"
expect_fail "released evaluator prompt is required" "$missing_prompt_root"

invalid_prompt_version_root=$(new_fixture invalid-prompt-version)
sed -i.bak 's/^version: 1.0.0$/version: latest/' "$invalid_prompt_version_root/agent/prompts/evaluator-question-generation/v1.0.0.md"
rm -f "$invalid_prompt_version_root/agent/prompts/evaluator-question-generation/v1.0.0.md.bak"
expect_fail "prompt version must match its immutable path" "$invalid_prompt_version_root"

permissive_prompt_root=$(new_fixture permissive-prompt)
sed -i.bak 's/^capability_policy: evaluator-capabilities@1.0.0$/capability_policy: evaluator-capabilities@2.0.0/' "$permissive_prompt_root/agent/prompts/evaluator-question-generation/v1.0.0.md"
rm -f "$permissive_prompt_root/agent/prompts/evaluator-question-generation/v1.0.0.md.bak"
expect_fail "prompt must use the approved capability policy" "$permissive_prompt_root"

echo "Agent infrastructure validator tests passed."
