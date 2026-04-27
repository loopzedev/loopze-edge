// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package yaegi

import (
	"reflect"
	"strings"

	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

// allowedPackages lists every Go stdlib package that user code in a
// function-go node may import. Anything else (os, net, runtime, syscall,
// unsafe, …) is rejected at compile time because the import simply isn't
// resolvable in the interpreter.
//
// This list is intentionally narrow: it covers data manipulation, encoding,
// math, and string work — the use cases the function-go node actually exists
// for. Keep it conservative; growing it later is easier than tightening once
// users depend on something.
var allowedPackages = map[string]bool{
	"bytes":           true,
	"encoding/base64": true,
	"encoding/binary": true,
	"encoding/hex":    true,
	"encoding/json":   true,
	"errors":          true,
	"fmt":             true,
	"math":            true,
	"math/big":        true,
	"math/bits":       true,
	"math/rand":       true,
	"regexp":          true,
	"sort":            true,
	"strconv":         true,
	"strings":         true,
	"time":            true,
	"unicode":         true,
	"unicode/utf16":   true,
	"unicode/utf8":    true,
}

// blockedSymbols are individual identifiers inside otherwise-allowed packages
// that we refuse to expose. The Yaegi-side `time` package is the main offender:
// blocking goroutines (Sleep, Tick, NewTicker, …) would freeze the engine's
// per-node nodeLoop, so user code cannot have those.
var blockedSymbols = map[string]map[string]bool{
	"time/time": {
		"Sleep":     true,
		"After":     true,
		"AfterFunc": true,
		"NewTimer":  true,
		"NewTicker": true,
		"Tick":      true,
	},
}

// SafeSymbols returns a filtered copy of the yaegi stdlib symbol map: only
// allowedPackages are kept, and blockedSymbols are stripped from those.
//
// Each call returns a fresh top-level map but reuses the inner per-symbol
// maps where possible (filtered ones are copied). Engines call this once per
// Compile, and the result is fed straight to interp.Use.
func SafeSymbols() interp.Exports {
	out := make(interp.Exports)
	for path, syms := range stdlib.Symbols {
		if !allowedPackages[importPathOf(path)] {
			continue
		}
		blocked := blockedSymbols[path]
		if blocked == nil {
			out[path] = syms
			continue
		}
		// Copy and filter individual symbols.
		cleaned := make(map[string]reflect.Value, len(syms))
		for name, val := range syms {
			if blocked[name] {
				continue
			}
			cleaned[name] = val
		}
		out[path] = cleaned
	}
	return out
}

// importPathOf converts a yaegi symbol-map key like "encoding/binary/binary"
// or "fmt/fmt" into the Go import path the user would write
// ("encoding/binary", "fmt"). The yaegi convention is that the last segment
// repeats the package name.
func importPathOf(symbolKey string) string {
	idx := strings.LastIndex(symbolKey, "/")
	if idx < 0 {
		return symbolKey
	}
	return symbolKey[:idx]
}
