---
name: specification-maintenance-and-refinement
description: Workflow command scaffold for specification-maintenance-and-refinement in spy.
allowed_tools: ["Bash", "Read", "Write", "Grep", "Glob"]
---

# /specification-maintenance-and-refinement

Use this workflow when working on **specification-maintenance-and-refinement** in `spy`.

## Goal

Refines and maintains the specification and related documents in response to internal review, new findings, or evolving requirements, without a formal spec-panel trigger.

## Common Files

- `specs/*/spec.md`
- `specs/*/contracts/*.md`
- `specs/*/data-model.md`
- `specs/*/research.md`
- `specs/*/tasks.md`

## Suggested Sequence

1. Understand the current state and failure mode before editing.
2. Make the smallest coherent change that satisfies the workflow goal.
3. Run the most relevant verification for touched files.
4. Summarize what changed and what still needs review.

## Typical Commit Signals

- Identify inconsistencies, ambiguities, or areas for improvement in the spec or related docs.
- Update specs/feature/spec.md to clarify requirements or correct errors.
- Update specs/feature/contracts/*.md and data-model.md as needed.
- Update specs/feature/research.md and tasks.md to reflect new insights or actions.
- Document rationale for changes in commit messages.

## Notes

- Treat this as a scaffold, not a hard-coded script.
- Update the command if the workflow evolves materially.