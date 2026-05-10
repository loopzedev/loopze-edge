// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package s7

import "github.com/loopzedev/loopze-edge/internal/flow"

// Stub callbacks shared by S7 tests. The engine normally injects these via
// SetSend / SetStatus / SetDebug; tests bypass that machinery and wire the
// stubs directly onto the embedded BaseNode.

func noopDebug(_ flow.DebugMessage) {}
