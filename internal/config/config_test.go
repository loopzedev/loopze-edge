// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package config

import "testing"

func TestNormalizeBasePath(t *testing.T) {
	tests := map[string]string{
		"":              "",
		"/":             "",
		" ":             "",
		"loopze":        "/loopze",
		"/loopze":       "/loopze",
		"/loopze/":      "/loopze",
		"/loopze/sub/":  "/loopze/sub",
		"  /loopze  ":   "/loopze",
	}
	for in, want := range tests {
		if got := NormalizeBasePath(in); got != want {
			t.Errorf("NormalizeBasePath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSplitCSV(t *testing.T) {
	tests := map[string][]string{
		"":                          nil,
		"   ":                       nil,
		"a":                         {"a"},
		"a,b":                       {"a", "b"},
		"a, b ,c":                   {"a", "b", "c"},
		",,":                        nil,
		"10.0.0.0/8, 192.168.1.1":   {"10.0.0.0/8", "192.168.1.1"},
	}
	for in, want := range tests {
		got := splitCSV(in)
		if len(got) != len(want) {
			t.Errorf("splitCSV(%q) len=%d, want %d (%v vs %v)", in, len(got), len(want), got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("splitCSV(%q)[%d] = %q, want %q", in, i, got[i], want[i])
			}
		}
	}
}
