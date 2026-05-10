// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
)

// Editor-role-gated, so every test logs in first.

func TestS7TestConnectionHandler_BadJSON(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	resp, err := c.Post(ts.url+"/api/v1/s7/test-connection", "application/json", strings.NewReader("not-json"))
	if err != nil {
		t.Fatalf("post: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", resp.StatusCode)
	}
}

func TestS7TestConnectionHandler_MissingConfig(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	code, body := doJSON(t, c, "POST", ts.url+"/api/v1/s7/test-connection", map[string]any{
		"name": "Test",
		// no config
	})
	if code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400 (body=%v)", code, body)
	}
}

func TestS7TestConnectionHandler_BadHost(t *testing.T) {
	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	// Localhost port 1 — privileged port nobody listens on. Connect should
	// fail; the handler returns ok=false with a populated error.
	code, body := doJSON(t, c, "POST", ts.url+"/api/v1/s7/test-connection", map[string]any{
		"name": "Bad",
		"config": map[string]any{
			"host":       "127.0.0.1",
			"port":       1,
			"connection": "s7-1200-1500",
			"timeout":    500, // ms
		},
	})
	if code != http.StatusOK {
		t.Errorf("status: got %d, want 200 (errors are reported in the body)", code)
	}
	if ok, _ := body["ok"].(bool); ok {
		t.Errorf("ok: got true, want false (host unreachable). body=%v", body)
	}
	if errStr, _ := body["error"].(string); errStr == "" {
		t.Error("expected non-empty error string")
	}
}

func TestS7TestConnectionHandler_Demo(t *testing.T) {
	host := os.Getenv("LOOPZE_S7_TEST_HOST")
	if host == "" {
		t.Skip("LOOPZE_S7_TEST_HOST not set; run `make demo-s7` and re-run with the env var")
	}
	port := 102
	if p := os.Getenv("LOOPZE_S7_TEST_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}

	ts := newTestServer(t)
	defer ts.close()
	c := ts.jarClient(t)
	seedEditorAndLogin(t, ts, c)

	code, body := doJSON(t, c, "POST", ts.url+"/api/v1/s7/test-connection", map[string]any{
		"name": "Demo PLC",
		"config": map[string]any{
			"host": host, "port": port, "connection": "s7-1200-1500",
			"timeout": 3000,
		},
	})
	if code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%v)", code, body)
	}
	if ok, _ := body["ok"].(bool); !ok {
		t.Fatalf("ok: got false, want true. body=%v", body)
	}
	info, _ := body["info"].(map[string]any)
	if info == nil {
		t.Fatalf("info missing. body=%v", body)
	}
	if pdu, _ := info["negotiatedPduSize"].(float64); pdu < 240 {
		t.Errorf("negotiatedPduSize: got %v, want ≥ 240", info["negotiatedPduSize"])
	}
	// CPUType / OrderCode etc. should be either populated or "unknown" —
	// the demo server returns blanks which the handler substitutes.
	for _, key := range []string{"cpuType", "orderCode", "moduleName", "serialNumber"} {
		if v, _ := info[key].(string); v == "" {
			t.Errorf("%s: empty (handler should substitute 'unknown')", key)
		}
	}
}
