// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

//go:build windows

package nodes

import "syscall"

// setBroadcastFD applies SO_BROADCAST to the given socket fd on
// Windows. The fd is a syscall.Handle (uintptr underneath), and
// syscall.SetsockoptInt's first argument is typed as Handle on
// this platform.
func setBroadcastFD(fd uintptr) error {
	return syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
