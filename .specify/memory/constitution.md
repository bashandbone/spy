<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->
<!--
Sync Impact Report (2026-04-25)
================================
Version change: (template / unratified) → 1.0.0
Bump rationale: First ratified constitution. The previous file was an
unmodified template with placeholder principles; ratifying defined
principles is a 0→1 event, recorded as 1.0.0 per semantic versioning.

Modified principles:
- (none — initial ratification)

Added principles:
- I. Spec-Driven Development (NON-NEGOTIABLE)
- II. Test-First Discipline (NON-NEGOTIABLE)
- III. Unix Philosophy & Composability
- IV. Pure-Go by Default, cgo Opt-In
- V. Capability-Aware Graceful Degradation
- VI. REUSE-Compliant Licensing

Added sections:
- Quality Standards
- Development Workflow
- Governance

Removed sections:
- (none)

Templates requiring updates:
- ✅ .specify/memory/constitution.md (this file)
- ⚠ .specify/templates/plan-template.md — Constitution Check section
  is a placeholder ("[Gates determined based on constitution file]"); it
  remains generic but plans (e.g., specs/001-popup-reader/plan.md) now
  evaluate against the principles below. No template edit needed for
  v1.0.0; revisit if principles are renamed or reordered.
- ⚠ .specify/templates/spec-template.md — no constitution-driven sections
  added or removed; no edit needed.
- ⚠ .specify/templates/tasks-template.md — no principle-specific task
  categories required at v1.0.0; testing tasks remain optional per the
  template's existing guidance, which is consistent with Principle II's
  enforcement at code-review/PR gates rather than at task generation.
- ✅ specs/001-popup-reader/plan.md — already aligned via its
  Constitution Check stub; the stub's "no ratified constitution" note
  becomes stale on this commit. Updating the existing plan after
  ratification is left as a documentation TODO and does not affect
  implementation work.

Follow-up TODOs:
- TODO(plan-001): Refresh the Constitution Check section in
  specs/001-popup-reader/plan.md to cite this constitution by version
  next time that plan is amended.

================================
-->

# spy Constitution

`spy` is a focused popup viewer for text, code, PDFs, and images, designed
for review work in multiplexed terminal environments. This constitution
records the non-negotiable engineering principles, quality standards, and
governance rules that bind all contributions to the project.

## Core Principles

### I. Spec-Driven Development (NON-NEGOTIABLE)

Every feature, behavior change, or architectural shift MUST flow through the
spec-kit workflow before implementation begins:

1. `/speckit-specify` — capture user-facing requirements (no implementation detail).
2. `/speckit-clarify` — resolve ambiguities with the requester.
3. `/speckit-plan` — produce a Technical Context, Constitution Check,
   research notes, data model, contracts, and quickstart.
4. `/speckit-tasks` — break the plan into independently testable tasks.
5. `/speckit-implement` — execute tasks; gate-check against this constitution.

**Rule**: No production code lands without a corresponding `specs/NNN-*/`
directory containing at minimum `spec.md` and `plan.md`. Bug fixes ≤ 20
lines and dependency-version bumps are exempt; everything else is in scope.

**Rationale**: The project's own DEVELOPMENT.md and existing spec-kit
configuration already encode this expectation; making it constitutional
prevents the "small change, skip the spec" drift that erodes the workflow.

### II. Test-First Discipline (NON-NEGOTIABLE)

New features and behavior changes MUST be developed test-first:

- Write the failing test before the implementation; merge with the test in
  the same change set.
- All packages MUST have meaningful unit tests; coverage SHOULD remain at
  or above 80 %, measured per-package.
- `go test ./... -race` MUST pass before any commit pushed to a feature
  branch is opened as a PR.
- Integration tests for capability-driven paths (theme detection, graphics
  protocols, terminal resize) MUST exercise the real path with PTYs or
  documented stubs; mocks MAY be used only for I/O boundaries.
- Tests SHOULD be table-driven where the cardinality of cases warrants it.

**Rationale**: The project ships a TUI that runs against a heterogeneous
mix of terminals; regressions are easy to introduce and hard to detect by
inspection. Test-first discipline is the cheapest defense.

### III. Unix Philosophy & Composability

`spy` is a CLI tool first; the popup TUI is one of several output modes:

- **Errors to stderr; content to stdout.** Diagnostic messages, deprecation
  warnings, and debug output MUST NOT mix with user-visible content.
- **Stable exit codes.** The codes documented in `contracts/cli.md`
  (or its successors) are part of the public contract; semantic changes
  require a major version bump.
- **Stdin support is a first-class input.** Any feature that accepts a file
  path MUST also accept piped input where it makes semantic sense.
- **Do not write to disk for transient input.** Piped content lives in
  memory; the tool MUST NOT create temp files for stdin under normal
  operation.
- **Compose, do not capture.** When integrating with external tools, prefer
  invoking via documented APIs / pipelines over scraping or wrapping.

**Rationale**: The product was specified explicitly as a `bat`-like tool;
honouring Unix conventions keeps it a good citizen of any pipeline.

### IV. Pure-Go by Default, cgo Opt-In

The default `go build` target MUST produce a working binary using only
pure-Go dependencies:

- Any feature that requires cgo (e.g., MuPDF rasterization via `go-fitz`)
  MUST be guarded by a build tag and have a pure-Go fallback that
  degrades the feature gracefully (typically falling back to text mode
  or to a metadata block).
- Cross-compilation to common targets (`linux/amd64`, `linux/arm64`,
  `darwin/amd64`, `darwin/arm64`) MUST work from a single host without
  cgo toolchains.
- Adding a new dependency that pulls in cgo, GPL/AGPL licensing, or > 5 MB
  of transitive code requires explicit justification in the originating
  plan's Complexity Tracking section.

**Rationale**: Distributors and users rely on `go install` and static
binaries; making cgo opt-in preserves that contract while still allowing
high-fidelity features where they make sense.

### V. Capability-Aware Graceful Degradation

`spy` runs across a wide range of terminals. The tool MUST:

- Detect terminal capabilities (TTY presence, color depth, graphics
  protocols, dimensions, background luminance) before applying behaviour
  that depends on them.
- Provide a working — even if reduced — experience in the absence of
  capabilities. A 60×20 dumb terminal MUST still display content; an
  emulator without graphics protocols MUST still display PDFs as text
  and images as metadata blocks.
- Never crash, hang, or corrupt terminal state because of an unsupported
  capability. Restoring terminal state on exit (including signals and
  panics) is mandatory.
- Honour `NO_COLOR`, `XDG_CONFIG_HOME`, and other documented user-facing
  environment variables.

**Rationale**: The product's value disappears in any environment where it
crashes; degradation MUST be a designed-in property, not an afterthought.

### VI. REUSE-Compliant Licensing

The repository is and MUST remain REUSE-compliant under the existing
dual licence (MIT OR Apache-2.0):

- Every source, configuration, documentation, and asset file MUST carry
  a valid SPDX header (or be covered by `REUSE.toml`).
- New dependencies MUST be license-compatible with both MIT and Apache-2.0;
  GPL/AGPL/LGPL dependencies are forbidden in production code paths.
- The `reuse lint` check MUST pass on `main` at all times. CI breakage
  from licensing must be repaired before any unrelated work is merged.

**Rationale**: The project has explicitly invested in REUSE compliance
(REUSE.toml, SPDX headers, dual licence) — preserving that investment is
a constitutional matter, not a nice-to-have.

## Quality Standards

- **Formatting**: `gofmt` and `goimports` MUST pass with no diff. CI hooks
  enforce this; manual overrides are not permitted.
- **Static analysis**: `go vet ./...` MUST pass. `staticcheck` and `gosec`
  SHOULD pass; remaining findings MUST be triaged with a tracking
  comment or issue.
- **Performance budgets** (current targets, revisit per major release):
  - First viewport frame ≤ 100 ms for ≤ 100-line files.
  - Resident memory ≤ 500 MB at 1 GB inputs.
  - Search across 1 MiB completes in ≤ 500 ms.
- **Error handling**: Errors MUST be wrapped with `%w` so callers can
  unwrap them. User-facing error messages MUST include the program name
  prefix and a concrete cause.
- **Documentation**: Public-facing CLI flags, env vars, exit codes, key
  bindings, and config keys MUST be documented in the relevant
  `specs/NNN-*/contracts/` file before they become reachable from a
  release build.

## Development Workflow

- **Branching**: Feature work happens on `NNN-feature-name` branches
  generated by `/speckit-git-feature`. Direct commits to `main` are
  forbidden except by the constitution-amendment process below.
- **Spec hooks**: The configured `.specify/extensions.yml` hooks govern
  before/after-stage automation; disabling a mandatory hook for a feature
  requires an explicit note in that feature's plan.
- **Code review**: Every PR MUST be reviewed against this constitution.
  PRs introducing complexity that violates a principle MUST link to the
  Complexity Tracking entry justifying the violation.
- **Definition of Done**: A feature is "done" when its `plan.md`'s
  `quickstart.md` validates end-to-end on a real terminal, all tests pass
  with `-race`, REUSE lint is clean, and the relevant `tasks.md` is
  fully checked off (or remaining tasks are explicitly deferred with
  rationale).
- **Releases**: Versioning follows semver. Breaking changes to the public
  CLI surface (flags, env vars, exit codes) require a major bump.

## Governance

This constitution governs all contributions to the `spy` repository and
supersedes any conflicting practice in individual PRs, agent
instructions, or developer preferences.

**Amendment procedure**:

1. Open a PR modifying `.specify/memory/constitution.md` (typically via
   `/speckit-constitution`).
2. Include a Sync Impact Report at the top of the file documenting
   version change, principles affected, dependent templates updated, and
   deferred TODOs.
3. Apply the semver bump rules from the `/speckit-constitution` skill:
   - **MAJOR** — backward-incompatible governance change or principle
     removal/redefinition;
   - **MINOR** — new principle or section added, or material expansion of
     guidance;
   - **PATCH** — clarifications, wording, typo fixes.
4. Update `LAST_AMENDED_DATE` to the merge date.
5. After merge, update dependent templates flagged in the Sync Impact
   Report. Pending updates MUST be tracked as follow-up issues.

**Compliance review**:

- Every plan's "Constitution Check" section MUST cite the constitution
  version it was evaluated against.
- Quarterly, the maintainer SHOULD walk the most recent plans and PRs
  against the current principles and open issues for any drift.
- Failing a Constitution Check at planning time blocks Phase 0 research
  unless an explicit Complexity Tracking entry justifies the deviation.

**Version**: 1.0.0 | **Ratified**: 2026-04-25 | **Last Amended**: 2026-04-25
