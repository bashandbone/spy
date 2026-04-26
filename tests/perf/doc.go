// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

//go:build !race

// Package perf hosts the success-criteria benchmarks for spy.
//
// The PR-gate tier (no build tag) enforces the SC-001..SC-008 budgets
// against modest inputs that fit in commodity-CI wall-clock and memory
// budgets. The nightly tier (`-tags perf`) covers the heavyweight
// scaling cases (1 GiB file load, RSS-bound large-buffer paths) and is
// driven from `.github/workflows/nightly-perf.yml`.
//
// All tests in this package are intentionally `_test.go` files: they
// don't ship as production code, only as gates.
package perf
