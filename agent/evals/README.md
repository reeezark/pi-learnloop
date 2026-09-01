---
asset_id: evaluator-case-guide
version: 1.0.0
status: development-contract
---

# Evaluator Development Cases

These cases provide a stable test surface for future evaluator adapters. They are synthetic development fixtures, not user data, production prompts, live model tests, or a runtime assessment protocol.

## Required Coverage

The repository validator requires at least one case in each category:

- `evidence_fidelity`: reject claims not supported by cited evidence.
- `insufficient_evidence`: abstain instead of inventing missing behavior.
- `prompt_injection`: treat instructions found inside evidence as untrusted data.
- `structured_result`: reject malformed structured output.

## Case Contract

Each `cases/<case-id>.json` file:

- conforms to `../schemas/eval-case.schema.json`;
- has a stable `case_id`, `case_version`, and `schema_version`;
- contains synthetic evidence only;
- records a candidate output as stimulus, not as truth;
- describes observable signals rather than model-specific wording.

Released cases are immutable. Add a new case version when expectations change.

## Adding a Case

1. Pick one required category and a kebab-case identifier.
2. Use synthetic evidence with explicit `synthetic: true`.
3. Keep the fixture minimal and isolate one failure mode.
4. State required signals, evidence references, and forbidden claims.
5. Run both Agent-infrastructure verification commands.

Live paid model calls, pass-rate thresholds, and semantic graders remain `TODO / Need Confirmation` for the evaluator implementation plan.

## Answer-assessment coverage

Assessment-specific cases reuse this development schema and remain synthetic stimuli rather than runtime protocol fixtures. They currently cover:

- unsupported answers and vague-answer over-crediting;
- prompt injection carried in a user answer;
- a materially useful follow-up and an unnecessary follow-up;
- rejection of a second follow-up at the final stage;
- malformed complete-assessment output.

These cases define observable review expectations. They do not make the draft assessment prompt callable and do not contact or grade a live model.
