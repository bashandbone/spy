// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

// Package search compiles forward / backward / regex / smart-case search
// queries into a Matcher and walks a source.LineProvider with Scan,
// emitting matches on a channel until cancellation or exhaustion. A
// synthetic wrap-around sentinel signals the UI to surface "search wrapped"
// without re-walking the buffer.
package search
