// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package filesystem

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/cbroglie/mustache"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// textExtensions resolves to "utf-8" when encoding is "auto". Everything not
// in this set falls through to "binary".
var textExtensions = map[string]bool{
	".txt":  true,
	".json": true,
	".xml":  true,
	".csv":  true,
	".log":  true,
	".yaml": true,
	".yml":  true,
	".toml": true,
	".md":   true,
}

// resolvePath renders the path template against the message, applies the
// optional msg.filename override (which always wins), validates absoluteness,
// and runs the jail check. Empty rootJail disables jail enforcement.
func resolvePath(tmpl string, msg *flow.Message, rootJail string) (string, error) {
	if msg != nil {
		if override, ok := msg.Get("filename").(string); ok && override != "" {
			return validatePath(override, rootJail)
		}
	}

	rendered := tmpl
	if strings.Contains(tmpl, "{{") {
		view := map[string]any{}
		if msg != nil {
			view = msg.DataView()
		}
		out, err := mustache.Render(tmpl, view)
		if err != nil {
			return "", fmt.Errorf("path template: %w", err)
		}
		rendered = out
	}

	return validatePath(rendered, rootJail)
}

// validatePath enforces non-empty, absolute, and jail-safe.
func validatePath(p, jail string) (string, error) {
	if p == "" {
		return "", fmt.Errorf("path is empty")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("path %q is not absolute", p)
	}
	clean := filepath.Clean(p)
	if jail != "" {
		if err := applyJail(clean, jail); err != nil {
			return "", err
		}
	}
	return clean, nil
}

// applyJail returns nil when absPath stays inside jail after symlink resolution.
// The jail directory must exist; the file itself may not (create-mode case).
func applyJail(absPath, jail string) error {
	if !filepath.IsAbs(jail) {
		return fmt.Errorf("jail %q is not absolute", jail)
	}
	realJail, err := filepath.EvalSymlinks(filepath.Clean(jail))
	if err != nil {
		return fmt.Errorf("jail not accessible: %w", err)
	}
	realPath, err := evalSymlinksLenient(absPath)
	if err != nil {
		return fmt.Errorf("path resolution: %w", err)
	}
	rel, err := filepath.Rel(realJail, realPath)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return fmt.Errorf("path %q escapes jail %q", absPath, jail)
	}
	return nil
}

// evalSymlinksLenient resolves symlinks for the longest existing ancestor of p,
// then re-attaches the non-existing tail. Lets jail enforcement work for paths
// whose terminal segment does not exist yet (create-mode).
func evalSymlinksLenient(p string) (string, error) {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return real, nil
	}
	dir := filepath.Dir(p)
	tail := filepath.Base(p)
	for dir != "/" && dir != "." && dir != "" {
		if real, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(real, tail), nil
		}
		tail = filepath.Join(filepath.Base(dir), tail)
		dir = filepath.Dir(dir)
	}
	return p, nil
}

// resolveEncoding picks the effective encoding. "auto" looks up the extension;
// explicit "utf-8" / "binary" pass through.
func resolveEncoding(path, cfgEncoding string) string {
	switch cfgEncoding {
	case "utf-8", "binary":
		return cfgEncoding
	}
	if textExtensions[strings.ToLower(filepath.Ext(path))] {
		return "utf-8"
	}
	return "binary"
}

// encodePayload converts a payload value to the bytes that hit disk.
//
// utf-8: strings and []byte pass through; structured values are JSON-marshalled.
// binary: []byte / []int / []any (number arrays from JSON-decoded JS) are
// reconstructed to raw bytes; strings are accepted as their UTF-8 representation.
func encodePayload(payload any, encoding string) ([]byte, error) {
	if payload == nil {
		return nil, nil
	}
	if encoding == "binary" {
		return encodeBinary(payload)
	}
	return encodeText(payload)
}

func encodeText(payload any) ([]byte, error) {
	switch v := payload.(type) {
	case string:
		return []byte(v), nil
	case []byte:
		return v, nil
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("payload encode: %w", err)
	}
	return b, nil
}

func encodeBinary(payload any) ([]byte, error) {
	switch v := payload.(type) {
	case []byte:
		return v, nil
	case string:
		return []byte(v), nil
	case []int:
		out := make([]byte, len(v))
		for i, n := range v {
			if n < 0 || n > 255 {
				return nil, fmt.Errorf("byte %d out of range [0,255]: %d", i, n)
			}
			out[i] = byte(n)
		}
		return out, nil
	case []any:
		out := make([]byte, len(v))
		for i, raw := range v {
			n, ok := toUint8(raw)
			if !ok {
				return nil, fmt.Errorf("byte %d is not a numeric in [0,255]: %v", i, raw)
			}
			out[i] = n
		}
		return out, nil
	}
	return nil, fmt.Errorf("unsupported binary payload type %T", payload)
}

func toUint8(v any) (byte, bool) {
	switch n := v.(type) {
	case float64:
		if n < 0 || n > 255 || n != float64(int(n)) {
			return 0, false
		}
		return byte(n), true
	case int:
		if n < 0 || n > 255 {
			return 0, false
		}
		return byte(n), true
	case int64:
		if n < 0 || n > 255 {
			return 0, false
		}
		return byte(n), true
	}
	return 0, false
}
