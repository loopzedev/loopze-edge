// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

// This file controls which node groups are bundled into the LOOPZE binary.
// Each blank import triggers the subpackage's init() — which calls
// nodes.RegisterGroup — so the server can apply (and optionally disable)
// the group at startup via runtime config.
//
// To add a new node group, create internal/nodes/<name>/ with an init.go
// that calls nodes.RegisterGroup, then add a blank import here.
//
// To remove a group from a custom build (e.g. a stripped industrial-only
// distribution), comment out the corresponding line and rebuild.

package main

import (
	_ "github.com/loopzedev/loopze-edge/internal/nodes/core"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/filesystem"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/modbus"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/mqtt"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/network"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/opcua"
	_ "github.com/loopzedev/loopze-edge/internal/nodes/s7"
)
