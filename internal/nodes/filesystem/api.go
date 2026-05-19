// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// This file exposes a minimal public surface of the filesystem package
// for other node packages that need the same path-resolution and write
// conventions (e.g. core/csv_out.go). Keep the surface intentionally
// small — these are pragmatic exports for in-tree reuse, not a stable
// external API.

package filesystem

import "github.com/loopzedev/loopze-edge/internal/flow"

// File-write modes, exposed for callers that use WriteFile.
const (
	ModeOverwrite = fileOutModeOverwrite
	ModeAppend    = fileOutModeAppend
	ModeCreate    = fileOutModeCreate
)

// ErrFileExists is returned by WriteFile in ModeCreate when the target
// already exists. Callers can errors.Is against this sentinel.
var ErrFileExists = errFileExists

// ResolvePath renders a path template against a message, honors the
// msg.filename override, validates absoluteness, and enforces rootJail
// containment. Empty rootJail disables jail enforcement.
func ResolvePath(tmpl string, msg *flow.Message, rootJail string) (string, error) {
	return resolvePath(tmpl, msg, rootJail)
}

// WriteFile opens path with the flags for the configured mode and writes
// data in a single syscall. Use the Mode* constants for the mode arg.
func WriteFile(path string, data []byte, mode string) error {
	return writeFile(path, data, mode)
}

// FileWriteErrorLabel maps a write error to a stable status label
// (matches the labels used by file-out's status pill).
func FileWriteErrorLabel(err error) string {
	return fileWriteErrorLabel(err)
}

// HumanSize renders a byte count like "1.5 kB" for status text.
func HumanSize(b int) string {
	return humanSize(b)
}
