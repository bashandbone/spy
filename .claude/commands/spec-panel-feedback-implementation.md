---
name: spec-panel-feedback-implementation
description: Workflow command scaffold for spec-panel-feedback-implementation in spy.
allowed_tools: ["Bash", "Read", "Write", "Grep", "Glob"]
---

# /spec-panel-feedback-implementation

Use this workflow when working on **spec-panel-feedback-implementation** in `spy`.

## Goal

Implements feedback from a spec-panel review (CRITICAL, HIGH, MEDIUM, LOW) by updating the specification, contracts, data model, research, quickstart, and tasks for a feature.

## Common Files

- `specs/*/contracts/*.md`
- `specs/*/data-model.md`
- `specs/*/spec.md`
- `specs/*/research.md`
- `specs/*/quickstart.md`
- `specs/*/tasks.md`

## Suggested Sequence

1. Understand the current state and failure mode before editing.
2. Make the smallest coherent change that satisfies the workflow goal.
3. Run the most relevant verification for touched files.
4. Summarize what changed and what still needs review.

## Typical Commit Signals

- Review spec-panel findings (CRITICAL/HIGH/MEDIUM/LOW).
- Update specs/feature/contracts/*.md to clarify or correct contracts.
- Update specs/feature/data-model.md to reflect new or changed data structures.
- Update specs/feature/spec.md to resolve contradictions, clarify requirements, or add edge cases.
- Update specs/feature/research.md with new findings or rationale.

## Notes

- Treat this as a scaffold, not a hard-coded script.
- Update the command if the workflow evolves materially.