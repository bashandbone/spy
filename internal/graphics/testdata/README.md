<!--
SPDX-FileCopyrightText: 2026 Adam Poulemanos

SPDX-License-Identifier: MIT OR Apache-2.0
-->

# Graphics encoder testdata

This directory holds binary fixtures for the per-protocol golden tests.

## Files

| File | Origin | Regen procedure |
|---|---|---|
| `kitty_input.png` | 16×16 deterministic gradient PNG | Re-run `TestRegenerateGoldens` (manual; see below) |
| `kitty_expected.bin` | Full Kitty graphics escape stream | Re-run `TestRegenerateGoldens` |
| `iterm2_expected.bin` | Full iTerm2 inline-image escape sequence | Re-run `TestRegenerateGoldens` |
| `sixel_expected.bin` | Full sixel byte stream | Re-run `TestRegenerateGoldens` |

## Regen procedure

Goldens are deterministic but pinned to the current dependency versions.
After bumping `mattn/go-sixel` (or any encoder that touches the wire
format), regenerate by re-running the test bootstrap with the
`graphics_regen` build tag set:

```sh
go test -tags graphics_regen ./internal/graphics/...
```

This rewrites every `*_expected.bin` from the current encoder output and
re-checks the input PNG bytes are still in sync with the
`buildKittyInputPNG` helper. Commit the regenerated files alongside the
dependency bump so reviewers can diff the wire format change.

## Why a deterministic input PNG?

The Kitty encoder packs PNG bytes directly into the protocol payload.
A non-deterministic input (e.g. `image.NewRGBA` with default zero values
re-rendered each test run) would produce a stable PNG only by accident
of the standard library's encoder; pinning the bytes to a checked-in
file keeps the golden honest if the stdlib ever changes the default
filter strategy or the PLTE chunk ordering.
