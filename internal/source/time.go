// SPDX-FileCopyrightText: 2026 Adam Poulemanos
//
// SPDX-License-Identifier: MIT OR Apache-2.0

package source

import "time"

// asTime converts the cached unix-nanos modification stamp on
// [FileSource] back into a [time.Time]. Wrapped here so callers don't
// need to know about the storage representation.
func asTime(unixNanos int64) time.Time {
	if unixNanos == 0 {
		return time.Time{}
	}
	return time.Unix(0, unixNanos)
}
