// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

//go:build !windows

package network

import "syscall"

// setBroadcastFD applies SO_BROADCAST to the given socket fd on
// unix-style platforms (Linux, macOS, BSD). The fd is an int.
func setBroadcastFD(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
}
