// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package network

import "github.com/loopzedev/loopze-edge/internal/flow"

// noopDebug is a no-op DebugFunc used by tests that don't exercise the debug
// callback but still need to satisfy the engine's injection contract.
func noopDebug(_ flow.DebugMessage) {}

// noopStatus is a no-op StatusFunc.
func noopStatus(_ string, _ string) {}
