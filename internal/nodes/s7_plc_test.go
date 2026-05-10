// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// ─── Unit tests (no PLC required) ─────────────────────────────────────────

func TestNewS7PLC_DefaultsAndConnectionTypes(t *testing.T) {
	cases := []struct {
		name        string
		connection  string
		extra       map[string]any
		wantRack    int
		wantSlot    int
		wantConnect int
	}{
		{"s7-1200-1500", "s7-1200-1500", nil, 0, 1, S7ConnectTypePG},
		{"s7-300-400", "s7-300-400", nil, 0, 2, S7ConnectTypePG},
		{"logo", "logo", nil, 0, 2, S7ConnectTypeBasic},
		{"custom defaults", "custom", nil, 0, 1, S7ConnectTypePG},
		{"custom override", "custom", map[string]any{"rack": 1, "slot": 5, "connectType": 2}, 1, 5, S7ConnectTypeOP},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg := flow.ConfigNode{
				ID:   "plc-1",
				Name: "Test",
				Config: map[string]any{
					"host":       "127.0.0.1",
					"port":       1102,
					"connection": c.connection,
				},
			}
			for k, v := range c.extra {
				cfg.Config[k] = v
			}
			inst, err := NewS7PLC(cfg)
			if err != nil {
				t.Fatalf("NewS7PLC error: %v", err)
			}
			plc := inst.(*S7PLC)
			if plc.rack != c.wantRack {
				t.Errorf("rack: got %d, want %d", plc.rack, c.wantRack)
			}
			if plc.slot != c.wantSlot {
				t.Errorf("slot: got %d, want %d", plc.slot, c.wantSlot)
			}
			if plc.connectType != c.wantConnect {
				t.Errorf("connectType: got %d, want %d", plc.connectType, c.wantConnect)
			}
		})
	}
}

func TestNewS7PLC_RejectMissingHost(t *testing.T) {
	_, err := NewS7PLC(flow.ConfigNode{ID: "plc-1", Name: "Test", Config: map[string]any{}})
	if err == nil {
		t.Fatal("NewS7PLC should reject missing host")
	}
}

func TestNewS7PLC_RejectInvalidConnection(t *testing.T) {
	_, err := NewS7PLC(flow.ConfigNode{
		ID: "plc-1", Name: "Test",
		Config: map[string]any{"host": "x", "connection": "fancy"},
	})
	if err == nil {
		t.Fatal("NewS7PLC should reject unknown connection")
	}
}

func TestNewS7PLC_RejectInvalidCustomConnectType(t *testing.T) {
	_, err := NewS7PLC(flow.ConfigNode{
		ID: "plc-1", Name: "Test",
		Config: map[string]any{"host": "x", "connection": "custom", "connectType": 99},
	})
	if err == nil {
		t.Fatal("NewS7PLC should reject invalid connectType")
	}
}

func TestSplitItemsForMultiRead_HardLimit(t *testing.T) {
	// 25 items, all small — should split at the 20-item gos7 hard cap.
	items := make([]S7Item, 25)
	for i := range items {
		items[i] = S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: i, Amount: 1}
	}
	chunks := splitItemsForMultiRead(items, 480)
	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks (20+5), got %d", len(chunks))
	}
	if len(chunks[0]) != 20 {
		t.Errorf("chunk 0: got %d items, want 20", len(chunks[0]))
	}
	if len(chunks[1]) != 5 {
		t.Errorf("chunk 1: got %d items, want 5", len(chunks[1]))
	}
}

func TestSplitItemsForMultiRead_PDUSplitByResponseSize(t *testing.T) {
	// 5 STRING(254) items = 256 bytes data + 4 byte response header each = 260 B / item.
	// PDU 480: 21 (resp header) + 260 = 281 fits one item; 21 + 520 = 541 doesn't.
	// So each item gets its own chunk.
	items := make([]S7Item, 5)
	for i := range items {
		items[i] = S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: i * 256, Amount: 256}
	}
	chunks := splitItemsForMultiRead(items, 480)
	if len(chunks) != 5 {
		t.Fatalf("expected 5 single-item chunks, got %d", len(chunks))
	}
	for i, c := range chunks {
		if len(c) != 1 {
			t.Errorf("chunk %d has %d items; expected 1", i, len(c))
		}
	}
}

func TestSplitItemsForMultiRead_SingleItem(t *testing.T) {
	items := []S7Item{{Area: S7AreaDB, WordLen: S7WLDWord, DBNumber: 1, Start: 0, Amount: 1}}
	chunks := splitItemsForMultiRead(items, 480)
	if len(chunks) != 1 || len(chunks[0]) != 1 {
		t.Errorf("got %d chunks with sizes %v, want 1×1", len(chunks), chunkSizes(chunks))
	}
}

func TestSplitItemsForMultiRead_EmptyInput(t *testing.T) {
	chunks := splitItemsForMultiRead(nil, 480)
	if len(chunks) != 0 {
		t.Errorf("expected 0 chunks for empty input, got %d", len(chunks))
	}
}

func TestSplitItemsForMultiRead_TinyPDU(t *testing.T) {
	// Even a tiny PDU should produce ≥1 chunk per item — never zero.
	items := []S7Item{{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: 0, Amount: 1}}
	chunks := splitItemsForMultiRead(items, 20)
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk for tiny PDU, got %d", len(chunks))
	}
}

func chunkSizes(chunks [][]S7Item) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}

func TestSplitWriteItemsForMulti_HardLimit(t *testing.T) {
	items := make([]S7WriteItem, 25)
	for i := range items {
		items[i] = S7WriteItem{
			Item: S7Item{Area: S7AreaDB, WordLen: S7WLByte, DBNumber: 1, Start: i, Amount: 1},
			Data: []byte{0xAA},
		}
	}
	chunks := splitWriteItemsForMulti(items, 480)
	if len(chunks) != 2 || len(chunks[0]) != 20 || len(chunks[1]) != 5 {
		t.Errorf("got chunk sizes %v, want [20, 5]", writeChunkSizes(chunks))
	}
}

func writeChunkSizes(chunks [][]S7WriteItem) []int {
	out := make([]int, len(chunks))
	for i, c := range chunks {
		out[i] = len(c)
	}
	return out
}

func TestIsS7TransportError(t *testing.T) {
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("Item not available (0x05)"), false},
		{errors.New("Address out of range"), false},
		{errors.New("Connection to address 1.2.3.4:102 is null"), true},
		{errors.New("EOF"), true},
		{errors.New("write tcp 1.2.3.4:102: broken pipe"), true},
		{errors.New("read tcp 1.2.3.4:102: connection reset by peer"), true},
		{errors.New("dial tcp 1.2.3.4:102: connection refused"), true},
		{&net.OpError{Op: "dial", Net: "tcp", Err: errors.New("timeout")}, true},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%v", c.err), func(t *testing.T) {
			if got := isS7TransportError(c.err); got != c.want {
				t.Errorf("isS7TransportError(%v) = %v, want %v", c.err, got, c.want)
			}
		})
	}
}

func TestS7PLC_StatusFuncReceivesCurrentState(t *testing.T) {
	plc := &S7PLC{currentFill: "yellow", currentText: "connecting..."}
	var got struct {
		fill, text string
	}
	plc.RegisterStatusFunc(func(fill, text string) {
		got.fill, got.text = fill, text
	})
	if got.fill != "yellow" || got.text != "connecting..." {
		t.Errorf("got (%q, %q), want (yellow, connecting...)", got.fill, got.text)
	}
}

func TestS7PLC_StatusFuncReceivesChanges(t *testing.T) {
	plc := &S7PLC{currentFill: "grey", currentText: "ready"}
	var (
		mu    sync.Mutex
		calls []string
	)
	plc.RegisterStatusFunc(func(fill, text string) {
		mu.Lock()
		defer mu.Unlock()
		calls = append(calls, fmt.Sprintf("%s/%s", fill, text))
	})
	plc.mu.Lock()
	plc.setStatusLocked("green", "connected (PDU 240)")
	plc.setStatusLocked("green", "connected (PDU 240)") // dedupe — no extra call
	plc.setStatusLocked("red", "boom")
	plc.mu.Unlock()

	mu.Lock()
	defer mu.Unlock()
	want := []string{"grey/ready", "green/connected (PDU 240)", "red/boom"}
	if len(calls) != len(want) {
		t.Fatalf("got %d calls (%v), want %d (%v)", len(calls), calls, len(want), want)
	}
	for i, c := range calls {
		if c != want[i] {
			t.Errorf("call %d: got %q, want %q", i, c, want[i])
		}
	}
}

func TestS7PLC_ConfigTypeInfoExposesType(t *testing.T) {
	info := S7PLCConfigTypeInfo()
	if info.Type != "s7-plc" {
		t.Errorf("Type: got %q, want s7-plc", info.Type)
	}
	if info.Defaults["port"] != s7DefaultPort {
		t.Errorf("Defaults[port]: got %v, want %d", info.Defaults["port"], s7DefaultPort)
	}
}

// ─── Integration tests (require LOOPZE_S7_TEST_HOST against a live demo PLC) ─

// requireDemoPLC builds and starts an S7PLC pointing at the demo server, or
// skips the test if LOOPZE_S7_TEST_HOST is not set. Caller must Stop() the
// returned PLC.
func requireDemoPLC(t *testing.T) *S7PLC {
	t.Helper()
	host := os.Getenv("LOOPZE_S7_TEST_HOST")
	if host == "" {
		t.Skip("LOOPZE_S7_TEST_HOST not set; run `make demo-s7` and re-run with the env var")
	}
	port := s7DefaultPort
	if p := os.Getenv("LOOPZE_S7_TEST_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	cfg := flow.ConfigNode{
		ID:   "plc-test",
		Name: "Demo PLC",
		Config: map[string]any{
			"host":       host,
			"port":       port,
			"connection": "s7-1200-1500",
			"timeout":    3000,
		},
	}
	inst, err := NewS7PLC(cfg)
	if err != nil {
		t.Fatalf("NewS7PLC: %v", err)
	}
	plc := inst.(*S7PLC)
	if err := plc.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if !plc.connected {
		_ = plc.Stop()
		t.Skipf("could not connect to demo PLC at %s:%d (status: %s); is it running?",
			host, port, statusText(plc))
	}
	t.Cleanup(func() { _ = plc.Stop() })
	return plc
}

func statusText(p *S7PLC) string {
	fill, text := p.Status()
	return fmt.Sprintf("%s/%s", fill, text)
}

func TestS7PLC_Connect_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	if plc.PDUSize() < 240 {
		t.Errorf("negotiated PDU size %d looks too small", plc.PDUSize())
	}
	fill, text := plc.Status()
	if fill != "green" {
		t.Errorf("status: got (%q, %q), want green/...", fill, text)
	}
}

func TestS7PLC_ReadArea_DB1_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	buf, err := plc.ReadArea(S7AreaDB, 1, 0, 200)
	if err != nil {
		t.Fatalf("ReadArea: %v", err)
	}
	if len(buf) != 200 {
		t.Errorf("buf len: got %d, want 200", len(buf))
	}
	// Sanity — DB1.STRING50.20 starts at byte 50 with maxLen=20, actLen=14,
	// chars "LOOPZE-S7-DEMO". Verify the header bytes.
	if buf[50] != 20 {
		t.Errorf("DB1[50] (STRING maxLen byte): got %d, want 20", buf[50])
	}
	if buf[51] != 14 {
		t.Errorf("DB1[51] (STRING actLen byte): got %d, want 14", buf[51])
	}
	if string(buf[52:52+14]) != "LOOPZE-S7-DEMO" {
		t.Errorf("DB1[52..66]: got %q, want %q", string(buf[52:52+14]), "LOOPZE-S7-DEMO")
	}
}

func TestS7PLC_ReadArea_M_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	buf, err := plc.ReadArea(S7AreaMK, 0, 0, 16)
	if err != nil {
		t.Fatalf("ReadArea: %v", err)
	}
	if len(buf) != 16 {
		t.Errorf("buf len: got %d, want 16", len(buf))
	}
	// Heartbeat at M0.0 toggles every second, so we can't assert a fixed value;
	// just confirm we got a buffer back without error.
}

func TestS7PLC_ReadItems_MultiVar_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	items := []S7Item{
		mustParseS7(t, "DB1.DBD0", "real"),  // temperature
		mustParseS7(t, "DB1.DBD8", "dint"),  // tick counter
		mustParseS7(t, "DB1.DBW12", "int"),  // setpoint (default 200)
		mustParseS7(t, "DB10.DBD0", "real"), // sine wave
	}
	results, err := plc.ReadItems(items)
	if err != nil {
		skipIfMultiReadUnsupported(t, err)
		t.Fatalf("ReadItems: %v", err)
	}
	if len(results) != len(items) {
		t.Fatalf("results: got %d, want %d", len(results), len(items))
	}
	for i, r := range results {
		if r.Err != "" {
			t.Errorf("item %d (%s): unexpected per-item error %q", i, items[i].DataType, r.Err)
		}
		if len(r.Data) != items[i].ByteSize() {
			t.Errorf("item %d: data len %d, want %d", i, len(r.Data), items[i].ByteSize())
		}
	}
	// Decode setpoint and confirm it's 200 (from the demo's static fill).
	setpoint, err := DecodeS7Scalar("int", false, results[2].Data)
	if err != nil {
		t.Fatalf("decode setpoint: %v", err)
	}
	if setpoint != int16(200) {
		t.Errorf("setpoint: got %v, want 200", setpoint)
	}
}

func TestS7PLC_WriteArea_M_RoundTrip_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	// MB1 is documented as writable scratch in the demo.
	want := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	if err := plc.WriteArea(S7AreaMK, 0, 1, want); err != nil {
		t.Fatalf("WriteArea: %v", err)
	}
	got, err := plc.ReadArea(S7AreaMK, 0, 1, len(want))
	if err != nil {
		t.Fatalf("ReadArea: %v", err)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, got[i], want[i])
		}
	}
}

func TestS7PLC_WriteItems_RoundTrip_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	// Write to MD4 (writable DWORD in the demo's M area) and read it back.
	wreq := []S7WriteItem{{
		Item: mustParseS7(t, "MD4", "dword"),
		Data: []byte{0xCA, 0xFE, 0xBA, 0xBE},
	}}
	wres, err := plc.WriteItems(wreq)
	if err != nil {
		t.Fatalf("WriteItems: %v", err)
	}
	if len(wres) != 1 || wres[0].Err != "" {
		t.Fatalf("WriteItems result: %+v", wres)
	}
	rres, err := plc.ReadItems([]S7Item{mustParseS7(t, "MD4", "dword")})
	if err != nil {
		t.Fatalf("ReadItems: %v", err)
	}
	if len(rres) != 1 || rres[0].Err != "" {
		t.Fatalf("ReadItems result: %+v", rres)
	}
	for i, b := range []byte{0xCA, 0xFE, 0xBA, 0xBE} {
		if rres[0].Data[i] != b {
			t.Errorf("byte %d: got 0x%02x, want 0x%02x", i, rres[0].Data[i], b)
		}
	}
}

func TestS7PLC_PerItemErrorSurvives_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	// Mix one valid item with one pointing at a non-existent DB. The whole
	// transaction should succeed (no transport error), but the bad item's Err
	// should be populated and the good item should still have data.
	items := []S7Item{
		mustParseS7(t, "DB1.DBD0", "real"),
		mustParseS7(t, "DB99.DBD0", "real"),
	}
	results, err := plc.ReadItems(items)
	if err != nil {
		skipIfMultiReadUnsupported(t, err)
		t.Fatalf("ReadItems: %v (expected nil even with per-item failure)", err)
	}
	if len(results) != 2 {
		t.Fatalf("results: got %d, want 2", len(results))
	}
	if results[0].Err != "" {
		t.Errorf("good item should have no error, got %q", results[0].Err)
	}
	if results[1].Err == "" {
		t.Error("DB99 read should have a per-item error")
	}
}

func TestS7PLC_Serialization_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	// Two parallel goroutines each issuing many reads. The mutex guarantees no
	// interleaving on the wire; we verify that all reads complete without
	// errors. Without serialisation the test would intermittently see protocol
	// framing errors.
	const reads = 20
	var wg sync.WaitGroup
	errs := make([]error, 2)
	for g := 0; g < 2; g++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			for i := 0; i < reads; i++ {
				if _, err := plc.ReadArea(S7AreaDB, 1, 0, 16); err != nil {
					errs[idx] = err
					return
				}
			}
		}(g)
	}
	wg.Wait()
	for i, e := range errs {
		if e != nil {
			t.Errorf("goroutine %d: %v", i, e)
		}
	}
}

func TestS7PLC_StopIsIdempotent_Demo(t *testing.T) {
	plc := requireDemoPLC(t)
	// requireDemoPLC schedules one Stop in t.Cleanup; calling another one here
	// should not panic.
	if err := plc.Stop(); err != nil {
		t.Errorf("first Stop: %v", err)
	}
	if err := plc.Stop(); err != nil {
		t.Errorf("second Stop: %v", err)
	}
}

func TestS7TestConnect_BadHost(t *testing.T) {
	// Pick a port that nobody listens on. dial-tcp connection-refused should
	// surface as a wrapped error, not a panic, and the function must return
	// promptly (within the configured 1s timeout).
	cfg := flow.ConfigNode{
		ID: "test", Name: "Bad", Type: "s7-plc",
		Config: map[string]any{
			"host":       "127.0.0.1",
			"port":       1, // privileged + nothing listening
			"connection": "s7-1200-1500",
			"timeout":    500, // ms — keep the test snappy
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, err := S7TestConnect(ctx, cfg)
	if err == nil {
		t.Fatal("S7TestConnect against unreachable host should error")
	}
	if !strings.Contains(err.Error(), "connect") {
		t.Errorf("expected connect-related error, got %q", err.Error())
	}
}

func TestS7TestConnect_RejectsMissingHost(t *testing.T) {
	cfg := flow.ConfigNode{
		ID: "test", Name: "NoHost", Type: "s7-plc",
		Config: map[string]any{}, // host missing
	}
	_, err := S7TestConnect(context.Background(), cfg)
	if err == nil {
		t.Fatal("S7TestConnect should reject config without host")
	}
}

func TestS7TestConnect_Demo(t *testing.T) {
	host := os.Getenv("LOOPZE_S7_TEST_HOST")
	if host == "" {
		t.Skip("LOOPZE_S7_TEST_HOST not set; run `make demo-s7` and re-run with the env var")
	}
	port := s7DefaultPort
	if p := os.Getenv("LOOPZE_S7_TEST_PORT"); p != "" {
		if n, err := strconv.Atoi(p); err == nil {
			port = n
		}
	}
	cfg := flow.ConfigNode{
		ID: "test", Name: "Demo", Type: "s7-plc",
		Config: map[string]any{
			"host": host, "port": port, "connection": "s7-1200-1500",
			"timeout": 3000,
		},
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	info, err := S7TestConnect(ctx, cfg)
	if err != nil {
		t.Fatalf("S7TestConnect: %v", err)
	}
	if info.NegotiatedPDUSize < 240 {
		t.Errorf("NegotiatedPDUSize: got %d, want ≥ 240", info.NegotiatedPDUSize)
	}
	if !strings.Contains(info.Address, host) {
		t.Errorf("Address: got %q, want it to contain %q", info.Address, host)
	}
	// CPUType / OrderCode may be empty against the python-snap7 demo —
	// real CPUs return populated strings. We don't assert on them here.
}

func TestS7PLC_ReconnectAfterIdleClose_Demo(t *testing.T) {
	// Reconnect smoke test: simulate a connection drop by closing the handler
	// directly, then issue a read. The lazy reconnect path should bring the
	// connection back transparently.
	plc := requireDemoPLC(t)

	// Force-close the underlying TCP connection.
	if err := plc.handler.Close(); err != nil {
		t.Fatalf("force-close handler: %v", err)
	}
	plc.mu.Lock()
	plc.connected = false
	plc.lastReconnectAttempt = time.Time{} // bypass backoff for the first retry
	plc.mu.Unlock()

	// Next call must successfully reconnect and read.
	buf, err := plc.ReadArea(S7AreaDB, 1, 0, 4)
	if err != nil {
		t.Fatalf("read after force-close: %v", err)
	}
	if len(buf) != 4 {
		t.Errorf("buf len after reconnect: got %d, want 4", len(buf))
	}
}

// mustParseS7 is a test helper: parses an address or fails the test.
func mustParseS7(t *testing.T, addr, dataType string) S7Item {
	t.Helper()
	item, err := ParseS7Address(addr, dataType)
	if err != nil {
		t.Fatalf("ParseS7Address(%q, %q): %v", addr, dataType, err)
	}
	return item
}

// skipIfMultiReadUnsupported recognises the python-snap7 server's response
// when it's asked to handle a multi-item AGReadMulti request — it doesn't
// implement multi-item read at the protocol level and surfaces "invalid CPU
// answer" because the response only has 1 item back. Real Siemens CPUs
// implement AGReadMulti correctly; we skip rather than fail so this test
// stays useful when run against actual hardware.
func skipIfMultiReadUnsupported(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(msg, "invalid CPU answer") || strings.Contains(msg, "Invalid CPU answer") {
		t.Skipf("python-snap7 demo server does not implement multi-item AGReadMulti (got %q); test would pass against a real Siemens CPU", err)
	}
}
