// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package core

import (
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/loopzedev/loopze-edge/internal/flow"
)

// csvJSONStore is a memStore variant that JSON-round-trips on Set so test
// state mirrors the NATS production path (where structs come back as
// map[string]any after JSON unmarshalling). The streaming tests rely on
// loadCSVState reconstituting a typed csvFileState from that map shape.
type csvJSONStore struct {
	mu   sync.RWMutex
	data map[string]any
}

func newCSVJSONStore() *csvJSONStore {
	return &csvJSONStore{data: map[string]any{}}
}

func (s *csvJSONStore) Get(key string) (any, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.data[key]
	if !ok {
		return nil, nil
	}
	return v, nil
}

func (s *csvJSONStore) Set(key string, value any) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	var v any
	if err := json.Unmarshal(b, &v); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.data[key] = v
	return nil
}

func (s *csvJSONStore) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.data, key)
	return nil
}

func (s *csvJSONStore) Keys() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.data))
	for k := range s.data {
		out = append(out, k)
	}
	return out, nil
}

type csvStores struct {
	flowPers   *csvJSONStore
	statusFill string
	statusText string
}

func newCSVNode(t *testing.T, props map[string]any) (*CSVParserNode, *csvStores) {
	t.Helper()
	cfg := flow.NodeConfig{ID: "csv-test", Type: "csv", Properties: props}
	inst, err := NewCSVParserNode(cfg)
	if err != nil {
		t.Fatalf("NewCSVParserNode: %v", err)
	}
	n := inst.(*CSVParserNode)
	if err := n.Init(); err != nil {
		t.Fatalf("Init: %v", err)
	}
	stores := &csvStores{flowPers: newCSVJSONStore()}
	n.SetContext(nil, nil, nil, stores.flowPers)
	n.SetSend(func(int, *flow.Message) {})
	n.SetStatus(func(fill, text string) {
		stores.statusFill = fill
		stores.statusText = text
	})
	n.SetDebug(func(flow.DebugMessage) {})
	if err := n.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = n.Stop() })
	return n, stores
}

// ─── Parse basics ──────────────────────────────────────────────────

func TestCSV_Parse_HeaderRows(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a,b\n1,2\n3,4\n"
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out[0]) != 2 {
		t.Fatalf("expected 2 fan-out messages, got %d", len(out[0]))
	}
	first, _ := out[0][0].Get("payload").(map[string]any)
	if first["a"] != "1" || first["b"] != "2" {
		t.Errorf("row 0: got %v", first)
	}
	cols, _ := out[0][0].Get("columns").([]string)
	if len(cols) != 2 || cols[0] != "a" || cols[1] != "b" {
		t.Errorf("columns: got %v", cols)
	}
}

func TestCSV_Parse_HeaderArray(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "array"})
	in := "a,b\n1,2\n3,4\n"
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 message in array mode, got %d", len(out[0]))
	}
	arr, ok := out[0][0].Get("payload").([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("payload: want []any of len 2, got %T %v", out[0][0].Get("payload"), out[0][0].Get("payload"))
	}
}

func TestCSV_Parse_NoHeader_PositionalRows(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "header": false, "output": "rows",
	})
	in := "1,2,3\n4,5,6\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 2 {
		t.Fatalf("rows: got %d", len(out[0]))
	}
	row, _ := out[0][0].Get("payload").([]any)
	if len(row) != 3 || row[0] != "1" || row[2] != "3" {
		t.Errorf("row 0: got %v", row)
	}
}

func TestCSV_Parse_HeaderTrue_WithColumns_OverridesNames(t *testing.T) {
	// header=true + columns set: the file's header row is dropped, but the
	// configured columns supply the keys (so the user can rename / pin them).
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "header": true,
		"columns": "ts,val", "output": "rows",
	})
	in := "time,value\n2026-05-18,0.20\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 1 {
		t.Fatalf("expected exactly 1 data row (header dropped), got %d", len(out[0]))
	}
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["ts"] != "2026-05-18" || row["val"] != "0.20" {
		t.Errorf("row: want ts=2026-05-18 val=0.20, got %v", row)
	}
	if _, ok := row["time"]; ok {
		t.Errorf("row keyed by file header instead of columns config: %v", row)
	}
}

func TestCSV_Parse_NoHeader_WithColumns(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "header": false, "columns": "ts,sensor,value",
		"output": "rows",
	})
	in := "1,T-101,25.4\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["ts"] != "1" || row["sensor"] != "T-101" || row["value"] != "25.4" {
		t.Errorf("row: got %v", row)
	}
}

func TestCSV_Parse_QuotedField(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a,b\n\"x,y\",z\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != "x,y" {
		t.Errorf("a: got %v", row["a"])
	}
}

func TestCSV_Parse_EscapedQuote(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a\n\"he said \"\"hi\"\"\"\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != `he said "hi"` {
		t.Errorf("a: got %v", row["a"])
	}
}

func TestCSV_Parse_EmbeddedNewline(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a,b\n\"x\ny\",z\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != "x\ny" {
		t.Errorf("a: got %q", row["a"])
	}
}

func TestCSV_Parse_BOMStripped(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "\xEF\xBB\xBFa,b\n1,2\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if _, ok := row["a"]; !ok {
		t.Errorf("BOM not stripped from header, got keys %v", row)
	}
}

func TestCSV_Parse_CRLF(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a,b\r\n1,2\r\n3,4\r\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 2 {
		t.Errorf("expected 2 rows from CRLF input, got %d", len(out[0]))
	}
}

func TestCSV_Parse_TabDelimiter(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "delimiter": "\t", "output": "rows",
	})
	in := "a\tb\n1\t2\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != "1" {
		t.Errorf("a: got %v", row["a"])
	}
}

func TestCSV_Parse_CommentSkipped(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "comment": "#", "output": "rows",
	})
	in := "# generated 2026\na,b\n1,2\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 1 {
		t.Errorf("expected 1 data row, got %d", len(out[0]))
	}
}

func TestCSV_Parse_EmptyLineSkipped(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "skipEmptyLines": true, "output": "rows",
	})
	in := "a,b\n1,2\n\n3,4\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 2 {
		t.Errorf("expected 2 rows, got %d", len(out[0]))
	}
}

func TestCSV_Parse_TrimSpaces(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "trimSpaces": true, "output": "rows",
	})
	in := "a,b\n  1  ,  2  \n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != "1" || row["b"] != "2" {
		t.Errorf("trim: got %v", row)
	}
}

func TestCSV_Parse_BufferInput(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "array"})
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": []byte("a,b\n1,2\n")}))
	arr, _ := out[0][0].Get("payload").([]any)
	if len(arr) != 1 {
		t.Errorf("expected 1 row, got %d", len(arr))
	}
}

func TestCSV_Parse_Cast_Bool(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "cast": true, "output": "rows",
	})
	in := "a,b\ntrue,FALSE\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != true || row["b"] != false {
		t.Errorf("cast bool: got a=%v b=%v", row["a"], row["b"])
	}
}

func TestCSV_Parse_Cast_Int(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "cast": true, "output": "rows",
	})
	in := "n\n42\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["n"] != int64(42) {
		t.Errorf("cast int: got %T %v", row["n"], row["n"])
	}
}

func TestCSV_Parse_Cast_Float(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "cast": true, "output": "rows",
	})
	in := "n\n3.14\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["n"] != 3.14 {
		t.Errorf("cast float: got %T %v", row["n"], row["n"])
	}
}

func TestCSV_Parse_Cast_EmptyToNil(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "cast": true, "output": "rows",
	})
	in := "a,b\n,foo\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != nil {
		t.Errorf("empty cell: got %v", row["a"])
	}
}

func TestCSV_Parse_Cast_Off(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "parse", "cast": false, "output": "rows",
	})
	in := "n,b\n42,true\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["n"] != "42" || row["b"] != "true" {
		t.Errorf("cast off: got %v", row)
	}
}

// ─── Output mode mechanics ─────────────────────────────────────────

func TestCSV_Parse_OutputRows_FanOut(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := "a\n1\n2\n3\n"
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if len(out[0]) != 3 {
		t.Fatalf("expected 3, got %d", len(out[0]))
	}
	if !out[0][0].Get("isFirst").(bool) || out[0][0].Get("isLast").(bool) {
		t.Errorf("first message flags wrong")
	}
	if out[0][2].Get("isFirst").(bool) || !out[0][2].Get("isLast").(bool) {
		t.Errorf("last message flags wrong")
	}
	if out[0][1].Get("index") != 1 || out[0][1].Get("total") != 3 {
		t.Errorf("middle msg index/total wrong: %v / %v",
			out[0][1].Get("index"), out[0][1].Get("total"))
	}
}

func TestCSV_Parse_OutputRows_EmptyCSV(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": "a,b\n"}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != nil {
		t.Errorf("empty CSV in rows mode should emit nothing, got %d messages", len(out[0]))
	}
}

func TestCSV_Parse_OutputRows_PreservesMsg(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	in := msgWith(map[string]any{"payload": "a\n1\n2\n", "topic": "foo"})
	out, _ := n.HandleMessage(in)
	for _, m := range out[0] {
		if m.Get("topic") != "foo" {
			t.Errorf("topic not preserved on fan-out clone")
		}
	}
}

func TestCSV_Parse_OutputArray_EmptyCSV(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "array"})
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": "a,b\n"}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out[0]))
	}
	arr, _ := out[0][0].Get("payload").([]any)
	if len(arr) != 0 {
		t.Errorf("expected empty array, got %v", arr)
	}
}

func TestCSV_Parse_Malformed_Error(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse"})
	in := "a,b\n\"unbalanced\n"
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err == nil {
		t.Fatal("expected parse error")
	}
	if stores.statusFill != "red" || stores.statusText != "csv parse error" {
		t.Errorf("status: got %q/%q", stores.statusFill, stores.statusText)
	}
}

func TestCSV_Parse_WrongType_Error(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errCSVTypeMismatch) {
		t.Errorf("expected type mismatch, got %v", err)
	}
}

// ─── Stringify ─────────────────────────────────────────────────────

func TestCSV_Stringify_ObjectsWithHeader(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "stringify", "columns": "a,b"})
	in := []map[string]any{{"a": "1", "b": "2"}, {"a": "3", "b": "4"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "a,b\n1,2\n3,4\n"
	if got != want {
		t.Errorf("stringify: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_ObjectsExplicitColumns(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "columns": "a,c",
	})
	in := []map[string]any{{"a": "1", "b": "x", "c": "3"}, {"a": "4"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "a,c\n1,3\n4,\n"
	if got != want {
		t.Errorf("explicit cols: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_ObjectsNoHeader_NoColumns_Error(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false,
	})
	in := []map[string]any{{"a": "1"}}
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, errCSVStringifyNoCol) {
		t.Errorf("err: %v", err)
	}
	if stores.statusText != "csv stringify: columns required" {
		t.Errorf("status text: got %q", stores.statusText)
	}
}

func TestCSV_Stringify_ArrayOfArrays(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false,
	})
	in := [][]any{{"1", "2"}, {"3", "4"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "1,2\n3,4\n"
	if got != want {
		t.Errorf("arr: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_QuoteWhenNeeded(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false,
	})
	in := [][]any{{"a,b", "c"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "\"a,b\",c\n"
	if got != want {
		t.Errorf("quote: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_ForceQuote(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false, "forceQuote": true,
	})
	in := [][]any{{"a", "b"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "\"a\",\"b\"\n"
	if got != want {
		t.Errorf("forceQuote: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_CRLFNewline(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false, "newline": "\r\n",
	})
	in := [][]any{{"a"}, {"b"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	if !strings.Contains(got, "\r\n") {
		t.Errorf("expected CRLF, got %q", got)
	}
}

func TestCSV_Stringify_CustomDelimiter(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "header": false, "delimiter": ";",
	})
	in := [][]any{{"1", "2"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	if got != "1;2\n" {
		t.Errorf("delim: got %q", got)
	}
}

func TestCSV_Stringify_NilCellEmpty(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "stringify", "columns": "a,b"})
	in := []map[string]any{{"a": nil, "b": "x"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "a,b\n,x\n"
	if got != want {
		t.Errorf("nil cell: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_SingleObject(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "columns": "a,b",
	})
	in := map[string]any{"a": "1", "b": "2"}
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("single object: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_SingleObject_DerivedColumns(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "stringify"})
	in := map[string]any{"a": "1", "b": "2"}
	out, err := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	got, _ := out[0][0].Get("payload").(string)
	// Without explicit columns the order is alphabetical → "a,b".
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("derived columns: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_HeaderOnce_FirstCallEmitsHeader(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "columns": "a,b", "headerOnce": true,
	})
	in := []map[string]any{{"a": "1", "b": "2"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("first call: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_HeaderOnce_SubsequentCallsSuppressHeader(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "columns": "a,b", "headerOnce": true,
	})
	in := []map[string]any{{"a": "1", "b": "2"}}

	// First call — header expected.
	_, _ = n.HandleMessage(msgWith(map[string]any{"payload": in}))

	// Second call — header must be suppressed.
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "1,2\n"
	if got != want {
		t.Errorf("second call: want %q, got %q", want, got)
	}

	// Third call — same.
	out, _ = n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ = out[0][0].Get("payload").(string)
	if got != want {
		t.Errorf("third call: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_HeaderOnce_RestartReemitsHeader(t *testing.T) {
	// After Stop+Start (simulating a redeploy), the in-memory flag resets
	// and the first message of the new run emits the header again.
	n, _ := newCSVNode(t, map[string]any{
		"action": "stringify", "columns": "a,b", "headerOnce": true,
	})
	in := []map[string]any{{"a": "1", "b": "2"}}
	_, _ = n.HandleMessage(msgWith(map[string]any{"payload": in}))
	_, _ = n.HandleMessage(msgWith(map[string]any{"payload": in})) // header suppressed
	_ = n.Stop()
	_ = n.Start()
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	got, _ := out[0][0].Get("payload").(string)
	want := "a,b\n1,2\n"
	if got != want {
		t.Errorf("after restart: want %q, got %q", want, got)
	}
}

func TestCSV_Stringify_HeaderOnce_OffByDefault(t *testing.T) {
	// Without headerOnce, every call must include the header.
	n, _ := newCSVNode(t, map[string]any{"action": "stringify", "columns": "a,b"})
	in := []map[string]any{{"a": "1", "b": "2"}}
	for i := 0; i < 3; i++ {
		out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
		got, _ := out[0][0].Get("payload").(string)
		want := "a,b\n1,2\n"
		if got != want {
			t.Errorf("call %d: want %q, got %q", i, want, got)
		}
	}
}

func TestCSV_Stringify_StringInput_Error(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "stringify"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": "already csv\n"}))
	if !errors.Is(err, errCSVTypeMismatch) {
		t.Errorf("expected type mismatch, got %v", err)
	}
}

func TestCSV_Stringify_UnsupportedShape_Error(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "stringify"})
	_, err := n.HandleMessage(msgWith(map[string]any{"payload": float64(42)}))
	if !errors.Is(err, errCSVStringifyBadType) {
		t.Errorf("expected bad type, got %v", err)
	}
}

// ─── Auto mode ─────────────────────────────────────────────────────

func TestCSV_Auto_StringInput_Parses(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "auto", "output": "array"})
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": "a\n1\n"}))
	if _, ok := out[0][0].Get("payload").([]any); !ok {
		t.Errorf("expected []any from auto parse")
	}
}

func TestCSV_Auto_ObjectInput_Stringifies(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "auto", "columns": "a"})
	in := []map[string]any{{"a": "1"}}
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": in}))
	if _, ok := out[0][0].Get("payload").(string); !ok {
		t.Errorf("expected string from auto stringify")
	}
}

func TestCSV_RoundTrip(t *testing.T) {
	parseN, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "array"})
	out, _ := parseN.HandleMessage(msgWith(map[string]any{"payload": "a,b\n1,2\n3,4\n"}))
	parsed := out[0][0].Get("payload")

	stringN, _ := newCSVNode(t, map[string]any{"action": "stringify", "columns": "a,b"})
	out2, _ := stringN.HandleMessage(msgWith(map[string]any{"payload": parsed}))
	got, _ := out2[0][0].Get("payload").(string)
	want := "a,b\n1,2\n3,4\n"
	if got != want {
		t.Errorf("round trip: want %q, got %q", want, got)
	}
}

// ─── Streaming / partial-row safety ────────────────────────────────

func TestCSV_Stream_CompleteChunk(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{
		"action": "parse", "output": "rows",
	})
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,2\n3,4\n",
		"filename": "/tmp/test.csv",
		"position": int64(13),
	}))
	if len(out[0]) != 2 {
		t.Fatalf("rows: got %d", len(out[0]))
	}
	key := csvStateKey("csv-test", "/tmp/test.csv")
	state, err := loadCSVState(stores.flowPers, key)
	if err != nil {
		t.Fatalf("load state: %v", err)
	}
	if state.Residue != "" {
		t.Errorf("residue should be empty, got %q", state.Residue)
	}
	if len(state.Columns) != 2 || state.Columns[0] != "a" {
		t.Errorf("columns persisted: got %v", state.Columns)
	}
}

func TestCSV_Stream_PartialLastRow(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,2\n3,",
		"filename": "/tmp/p.csv",
		"position": int64(10),
	}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 complete row, got %d", len(out[0]))
	}
	state, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/p.csv"))
	if state.Residue != "3," {
		t.Errorf("residue: got %q", state.Residue)
	}
	if state.Offset != 8 {
		t.Errorf("offset: want 8 (10 - 2), got %d", state.Offset)
	}
}

func TestCSV_Stream_PartialThenComplete(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	// chunk 1: header + first data row complete, second row partial
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,2\n3,",
		"filename": "/tmp/c.csv",
		"position": int64(10),
	}))
	// chunk 2: completes the row from chunk 1, plus one more
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "4\n5,6\n",
		"filename": "/tmp/c.csv",
		"position": int64(16),
	}))
	if len(out[0]) != 2 {
		t.Fatalf("expected 2 rows (3,4 and 5,6), got %d", len(out[0]))
	}
	r0, _ := out[0][0].Get("payload").(map[string]any)
	if r0["a"] != "3" || r0["b"] != "4" {
		t.Errorf("merged row 0: got %v", r0)
	}
}

func TestCSV_Stream_NoNewlineInChunk(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b,c",
		"filename": "/tmp/n.csv",
		"position": int64(5),
	}))
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if out != nil {
		t.Errorf("expected no output, got %v", out)
	}
	state, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/n.csv"))
	if state.Residue != "a,b,c" {
		t.Errorf("residue: got %q", state.Residue)
	}
}

func TestCSV_Stream_MultipleChunks(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	// Header in first chunk; partial trailing row each time.
	emitted := 0
	chunks := []struct {
		payload string
		pos     int64
	}{
		{"a,b\n1,", 6},
		{"2\n3,4", 11},
		{"\n5,", 14},
		{"6\n", 16},
	}
	for _, c := range chunks {
		out, _ := n.HandleMessage(msgWith(map[string]any{
			"payload":  c.payload,
			"filename": "/tmp/m.csv",
			"position": c.pos,
		}))
		if out != nil {
			emitted += len(out[0])
		}
	}
	if emitted != 3 {
		t.Errorf("expected 3 emitted rows across chunks, got %d", emitted)
	}
}

func TestCSV_Stream_CsvPositionAttached(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a\n1\n2\n",
		"filename": "/tmp/x.csv",
		"position": int64(7),
	}))
	for _, m := range out[0] {
		pos, ok := m.Get("csvPosition").(int64)
		if !ok || pos != 7 {
			t.Errorf("csvPosition: got %v (ok=%v)", m.Get("csvPosition"), ok)
		}
	}
}

func TestCSV_Stream_FilenamePassthrough(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a\n1\n",
		"filename": "/tmp/f.csv",
		"position": int64(4),
	}))
	for _, m := range out[0] {
		if m.Get("filename") != "/tmp/f.csv" {
			t.Errorf("filename: got %v", m.Get("filename"))
		}
	}
}

func TestCSV_Stream_StatePersistedInFlowPers(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse"})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "a\n1\n",
		"filename": "/tmp/k.csv",
		"position": int64(4),
	}))
	key := csvStateKey("csv-test", "/tmp/k.csv")
	if _, ok := stores.flowPers.data[key]; !ok {
		t.Errorf("expected key %q in flowPers", key)
	}
}

func TestCSV_Stream_StateSurvivesRestart(t *testing.T) {
	// First node sees a partial chunk.
	n1, stores := newCSVNode(t, map[string]any{"action": "parse"})
	_, _ = n1.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,",
		"filename": "/tmp/r.csv",
		"position": int64(6),
	}))

	// Second node — same flowPers store, same node ID, simulating restart.
	cfg := flow.NodeConfig{ID: "csv-test", Type: "csv",
		Properties: map[string]any{"action": "parse", "output": "rows"}}
	inst, _ := NewCSVParserNode(cfg)
	n2 := inst.(*CSVParserNode)
	_ = n2.Init()
	n2.SetContext(nil, nil, nil, stores.flowPers)
	n2.SetSend(func(int, *flow.Message) {})
	n2.SetStatus(func(string, string) {})
	n2.SetDebug(func(flow.DebugMessage) {})
	_ = n2.Start()

	// Complete the partial row via the restarted node.
	out, _ := n2.HandleMessage(msgWith(map[string]any{
		"payload":  "2\n",
		"filename": "/tmp/r.csv",
		"position": int64(8),
	}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out[0]))
	}
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["a"] != "1" || row["b"] != "2" {
		t.Errorf("merged row: got %v", row)
	}
}

func TestCSV_Stream_HeaderOnlyInFirstChunk(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	// Chunk 1: header + one data row.
	out1, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,2\n",
		"filename": "/tmp/h.csv",
		"position": int64(8),
	}))
	if len(out1[0]) != 1 {
		t.Fatalf("chunk1: expected 1 row, got %d", len(out1[0]))
	}
	// Chunk 2: more data rows; the first one MUST be treated as data, not header.
	out2, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "3,4\n5,6\n",
		"filename": "/tmp/h.csv",
		"position": int64(16),
	}))
	if len(out2[0]) != 2 {
		t.Fatalf("chunk2: expected 2 rows, got %d", len(out2[0]))
	}
	r, _ := out2[0][0].Get("payload").(map[string]any)
	if r["a"] != "3" || r["b"] != "4" {
		t.Errorf("first row of chunk2 lost header context: got %v", r)
	}
}

func TestCSV_Stream_HeaderPersistedInState(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse"})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,2\n",
		"filename": "/tmp/hp.csv",
		"position": int64(8),
	}))
	state, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/hp.csv"))
	if len(state.Columns) != 2 || state.Columns[0] != "a" {
		t.Errorf("columns: got %v", state.Columns)
	}
}

func TestCSV_Stream_Reset_ClearsResidue(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,",
		"filename": "/tmp/rc.csv",
		"position": int64(6),
	}))
	// Reset: residue must be discarded; the new chunk starts fresh and the
	// first row is treated as the (new) header again.
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "x,y\n9,8\n",
		"filename": "/tmp/rc.csv",
		"position": int64(8),
		"reset":    true,
	}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 row after reset, got %d", len(out[0]))
	}
	row, _ := out[0][0].Get("payload").(map[string]any)
	if row["x"] != "9" {
		t.Errorf("post-reset row: got %v", row)
	}
	state, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/rc.csv"))
	if state.Residue != "" {
		t.Errorf("residue after reset: got %q", state.Residue)
	}
}

func TestCSV_Stream_TwoFilesIndependentState(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse"})
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "a,b\n1,",
		"filename": "/tmp/A.csv",
		"position": int64(6),
	}))
	_, _ = n.HandleMessage(msgWith(map[string]any{
		"payload":  "x,y\n",
		"filename": "/tmp/B.csv",
		"position": int64(4),
	}))
	stA, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/A.csv"))
	stB, _ := loadCSVState(stores.flowPers, csvStateKey("csv-test", "/tmp/B.csv"))
	if stA.Residue != "1," {
		t.Errorf("A residue: got %q", stA.Residue)
	}
	if stB.Residue != "" {
		t.Errorf("B residue: got %q", stB.Residue)
	}
	if stA.Columns[0] != "a" || stB.Columns[0] != "x" {
		t.Errorf("columns crossed over: A=%v B=%v", stA.Columns, stB.Columns)
	}
}

func TestCSV_FilenameWithoutPosition_OneShotPath(t *testing.T) {
	// file-read full-mode (non-incremental) sets msg.filename but no
	// msg.position. The CSV parser must take the one-shot path so the
	// header is consumed normally and no stale per-file state is loaded.
	n, stores := newCSVNode(t, map[string]any{
		"action": "parse", "output": "array",
	})

	// Pre-populate persistent state from a (simulated) earlier run.
	key := csvStateKey("csv-test", "/tmp/full.csv")
	_ = stores.flowPers.Set(key, csvFileState{
		Offset:  0,
		Residue: "",
		Columns: []string{"time", "value"},
	})

	// Full-file read: filename set, position absent.
	out, err := n.HandleMessage(msgWith(map[string]any{
		"payload":  "time,value\n2026-05-19,0.97\n",
		"filename": "/tmp/full.csv",
	}))
	if err != nil {
		t.Fatalf("HandleMessage: %v", err)
	}
	arr, _ := out[0][0].Get("payload").([]any)
	if len(arr) != 1 {
		t.Fatalf("expected 1 data row (header consumed), got %d: %v", len(arr), arr)
	}
	row, _ := arr[0].(map[string]any)
	if row["time"] != "2026-05-19" || row["value"] != "0.97" {
		t.Errorf("row: got %v", row)
	}
	// No csvPosition field on one-shot path.
	if out[0][0].Get("csvPosition") != nil {
		t.Errorf("csvPosition should be absent on one-shot path")
	}
}

func TestCSV_Stream_NoFilename_NormalMode(t *testing.T) {
	n, stores := newCSVNode(t, map[string]any{"action": "parse", "output": "rows"})
	out, _ := n.HandleMessage(msgWith(map[string]any{"payload": "a\n1\n"}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 row, got %d", len(out[0]))
	}
	if len(stores.flowPers.data) != 0 {
		t.Errorf("expected no state stored without filename, got %d entries", len(stores.flowPers.data))
	}
	// Also: csvPosition / filename must not be set.
	if out[0][0].Get("csvPosition") != nil {
		t.Errorf("csvPosition should be unset without streaming")
	}
}

func TestCSV_Stream_ArrayOutput(t *testing.T) {
	n, _ := newCSVNode(t, map[string]any{"action": "parse", "output": "array"})
	out, _ := n.HandleMessage(msgWith(map[string]any{
		"payload":  "a\n1\n2\n",
		"filename": "/tmp/arr.csv",
		"position": int64(7),
	}))
	if len(out[0]) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out[0]))
	}
	arr, _ := out[0][0].Get("payload").([]any)
	if len(arr) != 2 {
		t.Errorf("expected 2 rows in array, got %d", len(arr))
	}
	if pos, _ := out[0][0].Get("csvPosition").(int64); pos != 7 {
		t.Errorf("csvPosition: got %v", out[0][0].Get("csvPosition"))
	}
}
