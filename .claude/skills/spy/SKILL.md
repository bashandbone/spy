```markdown
# spy Development Patterns

> Auto-generated skill from repository analysis

## Overview

This skill teaches the core development patterns, coding conventions, and specification workflows used in the `spy` TypeScript codebase. The repository emphasizes clear specification-driven development, modular TypeScript code, and a structured approach to feature planning and refinement. Whether you're contributing code, writing specifications, or maintaining documentation, this guide will help you align with the project's standards.

## Coding Conventions

### File Naming

- Use **camelCase** for file names.
  - Example: `dataModel.ts`, `userService.ts`

### Import Style

- Use **relative imports** for all modules.
  - Example:
    ```typescript
    import { fetchData } from './fetchData';
    ```

### Export Style

- Use **named exports** only.
  - Example:
    ```typescript
    // Good
    export function parseUser(input: string) { ... }
    export const DEFAULT_TIMEOUT = 5000;

    // Avoid
    export default function parseUser(input: string) { ... }
    ```

### Commit Patterns

- Commit messages are freeform, typically concise (~49 characters).
- No strict prefixing, but clarity is valued.

## Workflows

### Spec Panel Feedback Implementation

**Trigger:** When a spec-panel review produces findings that must be addressed in the documentation/specification.  
**Command:** `/apply-spec-panel-feedback`

1. Review all findings from the spec-panel (CRITICAL/HIGH/MEDIUM/LOW).
2. Update `specs/feature/contracts/*.md` to clarify or correct contracts.
3. Update `specs/feature/data-model.md` to reflect new or changed data structures.
4. Update `specs/feature/spec.md` to resolve contradictions, clarify requirements, or add edge cases.
5. Update `specs/feature/research.md` with new findings or rationale.
6. Update `specs/feature/quickstart.md` if user-facing steps change.
7. Update `specs/feature/tasks.md` to add, update, or close implementation tasks.

**Example:**
```bash
# After receiving panel feedback
/apply-spec-panel-feedback
# Edit relevant .md files as per findings
```

---

### Specification Initialization for New Feature

**Trigger:** When a new feature is being planned and its specification is to be captured.  
**Command:** `/init-spec-feature`

1. Create or update `.specify/feature.json` to register the new feature.
2. Add `specs/feature/spec.md` with clarified requirements.
3. Add `specs/feature/plan.md` outlining implementation steps.
4. Add `specs/feature/research.md` summarizing relevant research.
5. Add `specs/feature/data-model.md` describing data structures.
6. Add `specs/feature/quickstart.md` for user onboarding.
7. Add `specs/feature/contracts/*.md` for CLI, config, internal APIs, keys, etc.
8. Add `specs/feature/checklists/requirements.md` for requirement tracking.
9. Update `CLAUDE.md` or other project-level markers to reference the new feature.

**Example:**
```bash
/init-spec-feature
# Fill in the new spec, plan, and related docs for your feature
```

---

### Specification Maintenance and Refinement

**Trigger:** When inconsistencies, ambiguities, or improvements are identified in the specification or related docs outside formal panel reviews.  
**Command:** `/refine-spec`

1. Identify inconsistencies, ambiguities, or areas for improvement in the spec or related docs.
2. Update `specs/feature/spec.md` to clarify requirements or correct errors.
3. Update `specs/feature/contracts/*.md` and `data-model.md` as needed.
4. Update `specs/feature/research.md` and `tasks.md` to reflect new insights or actions.
5. Document rationale for changes in commit messages.

**Example:**
```bash
/refine-spec
# Edit relevant .md files to improve clarity or correctness
```

## Testing Patterns

- Test files follow the `*.test.*` pattern (e.g., `userService.test.ts`).
- Testing framework is **unknown**; check existing test files for conventions.
- Place tests alongside implementation or in a dedicated test directory.

**Example:**
```typescript
// userService.test.ts
import { getUser } from './userService';

test('getUser returns correct user', () => {
  expect(getUser('alice')).toEqual({ name: 'Alice' });
});
```

## Commands

| Command                   | Purpose                                                        |
|---------------------------|----------------------------------------------------------------|
| /apply-spec-panel-feedback| Apply and document feedback from a spec-panel review           |
| /init-spec-feature        | Initialize all spec and planning docs for a new feature        |
| /refine-spec              | Refine and maintain specs based on internal review or findings |
```
