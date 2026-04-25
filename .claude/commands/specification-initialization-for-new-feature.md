---
name: specification-initialization-for-new-feature
description: Workflow command scaffold for specification-initialization-for-new-feature in spy.
allowed_tools: ["Bash", "Read", "Write", "Grep", "Glob"]
---

# /specification-initialization-for-new-feature

Use this workflow when working on **specification-initialization-for-new-feature** in `spy`.

## Goal

Creates all foundational specification and planning documents for a new feature, including spec, plan, research, data model, quickstart, contracts, and checklist files.

## Common Files

- `.specify/feature.json`
- `CLAUDE.md`
- `specs/*/spec.md`
- `specs/*/plan.md`
- `specs/*/research.md`
- `specs/*/data-model.md`

## Suggested Sequence

1. Understand the current state and failure mode before editing.
2. Make the smallest coherent change that satisfies the workflow goal.
3. Run the most relevant verification for touched files.
4. Summarize what changed and what still needs review.

## Typical Commit Signals

- Create or update .specify/feature.json to register the new feature.
- Add specs/feature/spec.md with clarified requirements.
- Add specs/feature/plan.md outlining implementation steps.
- Add specs/feature/research.md summarizing relevant research.
- Add specs/feature/data-model.md describing data structures.

## Notes

- Treat this as a scaffold, not a hard-coded script.
- Update the command if the workflow evolves materially.