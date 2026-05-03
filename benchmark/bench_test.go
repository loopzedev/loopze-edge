// Benchmark comparing the candidate scripting engines for a LOOPZE Function-Node:
// - Native Go (baseline; what a dedicated Go-Node would do)
// - Goja        (current Function-Node engine)
// - expr        (compiled bytecode VM, pipeline-style)
// - Yaegi       (Go interpreter)
// - Python      (persistent subprocess worker — realistic for a LOOPZE
//                Python-Function-Node implementation. cgo-embedding would be
//                ~5-10x faster but isn't viable without python3-dev installed.)
//
// Two data shapes are exercised:
//
//   - "map":    []map[string]any   — current flow.Message storage shape
//   - "typed":  []Record           — what we'd get if LOOPZE introduced typed
//                                    payloads. Lets each engine's compiler
//                                    skip runtime type-asserts.
//
// Three sizes — 1k / 10k / 100k records — show how the per-call setup overhead
// (especially Goja's vm.ToValue) scales relative to the actual work.
//
// Run:  go test -bench=. -benchmem -benchtime=1s
package loopzebench

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/dop251/goja"
	"github.com/expr-lang/expr"
	exprvm "github.com/expr-lang/expr/vm"
	"github.com/traefik/yaegi/interp"
	"github.com/traefik/yaegi/stdlib"
)

var sizes = []int{1000, 10000, 100000}

// Record is the typed counterpart of the map[string]any shape — same fields,
// same semantics, but no per-access type-assertion needed.
type Record struct {
	Temperature float64
	Humidity    float64
	Timestamp   int64
}

func generateMapRecords(n int) []map[string]any {
	out := make([]map[string]any, n)
	for i := 0; i < n; i++ {
		out[i] = map[string]any{
			"temperature": 15.0 + float64(i%30),
			"humidity":    50.0 + float64(i%20),
			"timestamp":   int64(1700000000 + i),
		}
	}
	return out
}

func generateTypedRecords(n int) []Record {
	out := make([]Record, n)
	for i := 0; i < n; i++ {
		out[i] = Record{
			Temperature: 15.0 + float64(i%30),
			Humidity:    50.0 + float64(i%20),
			Timestamp:   int64(1700000000 + i),
		}
	}
	return out
}

func nativeMap(records []map[string]any) float64 {
	sum := 0.0
	for _, r := range records {
		if t, ok := r["temperature"].(float64); ok && t > 20 {
			sum += t*1.8 + 32
		}
	}
	return sum
}

func nativeTyped(records []Record) float64 {
	sum := 0.0
	for _, r := range records {
		if r.Temperature > 20 {
			sum += r.Temperature*1.8 + 32
		}
	}
	return sum
}

// ── Native Go ────────────────────────────────────────────────────────────────

func BenchmarkNative(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("map/%d", n), func(b *testing.B) {
			records := generateMapRecords(n)
			b.ReportAllocs()
			b.ResetTimer()
			var sink float64
			for i := 0; i < b.N; i++ {
				sink = nativeMap(records)
			}
			_ = sink
		})
		b.Run(fmt.Sprintf("typed/%d", n), func(b *testing.B) {
			records := generateTypedRecords(n)
			b.ReportAllocs()
			b.ResetTimer()
			var sink float64
			for i := 0; i < b.N; i++ {
				sink = nativeTyped(records)
			}
			_ = sink
		})
	}
}

// ── Goja ─────────────────────────────────────────────────────────────────────

// Wrapped as a function — LOOPZE's actual Function-Node pattern, so per-call
// `let` declarations don't pollute the VM globals.
const gojaCode = `(function(records){
    let sum = 0;
    for (let i = 0; i < records.length; i++) {
        if (records[i].temperature > 20) {
            sum += records[i].temperature * 1.8 + 32;
        }
    }
    return sum;
})`

// Goja exposes Go struct field names verbatim — uppercase Temperature.
const gojaCodeTyped = `(function(records){
    let sum = 0;
    for (let i = 0; i < records.length; i++) {
        if (records[i].Temperature > 20) {
            sum += records[i].Temperature * 1.8 + 32;
        }
    }
    return sum;
})`

func runGojaBench(b *testing.B, code string, records any) {
	vm := goja.New()
	prog, err := goja.Compile("bench.js", code, false)
	if err != nil {
		b.Fatal(err)
	}
	fnVal, err := vm.RunProgram(prog)
	if err != nil {
		b.Fatal(err)
	}
	fn, ok := goja.AssertFunction(fnVal)
	if !ok {
		b.Fatal("compiled value is not callable")
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := fn(goja.Undefined(), vm.ToValue(records))
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkGoja(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("map/%d", n), func(b *testing.B) {
			runGojaBench(b, gojaCode, generateMapRecords(n))
		})
		b.Run(fmt.Sprintf("typed/%d", n), func(b *testing.B) {
			runGojaBench(b, gojaCodeTyped, generateTypedRecords(n))
		})
	}
}

// ── expr ─────────────────────────────────────────────────────────────────────

const exprCodeMap = `sum(map(filter(records, .temperature > 20), .temperature * 1.8 + 32))`

// Field accessors match the struct field names exactly when the env declares
// the typed slice. expr's compiler can then specialise the bytecode.
const exprCodeTyped = `sum(map(filter(records, .Temperature > 20), .Temperature * 1.8 + 32))`

func runExprBench(b *testing.B, code string, env map[string]any) {
	program, err := expr.Compile(code, expr.Env(env))
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := exprvm.Run(program, env)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkExpr(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("map/%d", n), func(b *testing.B) {
			runExprBench(b, exprCodeMap, map[string]any{"records": generateMapRecords(n)})
		})
		b.Run(fmt.Sprintf("typed/%d", n), func(b *testing.B) {
			runExprBench(b, exprCodeTyped, map[string]any{"records": generateTypedRecords(n)})
		})
	}
}

// ── Yaegi ────────────────────────────────────────────────────────────────────

const yaegiSourceMap = `
package main

func Handle(records []map[string]any) float64 {
	sum := 0.0
	for _, r := range records {
		if t, ok := r["temperature"].(float64); ok && t > 20 {
			sum += t*1.8 + 32
		}
	}
	return sum
}
`

// To run yaegi'd code against a typed []Record from this package, we have to
// expose the type into the interpreter's import space. Yaegi sees the type
// under the import path "loopzebench/loopzebench" → so the user code does
// `import "loopzebench"` and uses `loopzebench.Record`.
const yaegiSourceTyped = `
package main

import "loopzebench"

func Handle(records []loopzebench.Record) float64 {
	sum := 0.0
	for _, r := range records {
		if r.Temperature > 20 {
			sum += r.Temperature*1.8 + 32
		}
	}
	return sum
}
`

var yaegiTypedExports = interp.Exports{
	"loopzebench/loopzebench": map[string]reflect.Value{
		"Record": reflect.ValueOf((*Record)(nil)),
	},
}

func runYaegiBench(b *testing.B, source string, withTyped bool, callFn func(any)) {
	i := interp.New(interp.Options{})
	if err := i.Use(stdlib.Symbols); err != nil {
		b.Fatal(err)
	}
	if withTyped {
		if err := i.Use(yaegiTypedExports); err != nil {
			b.Fatal(err)
		}
	}
	if _, err := i.Eval(source); err != nil {
		b.Fatal(err)
	}
	v, err := i.Eval("main.Handle")
	if err != nil {
		b.Fatal(err)
	}
	callFn(v.Interface())
}

func BenchmarkYaegi(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("map/%d", n), func(b *testing.B) {
			records := generateMapRecords(n)
			runYaegiBench(b, yaegiSourceMap, false, func(handle any) {
				fn, ok := handle.(func([]map[string]any) float64)
				if !ok {
					b.Fatalf("eval returned %T", handle)
				}
				b.ReportAllocs()
				b.ResetTimer()
				var sink float64
				for j := 0; j < b.N; j++ {
					sink = fn(records)
				}
				_ = sink
			})
		})
		b.Run(fmt.Sprintf("typed/%d", n), func(b *testing.B) {
			records := generateTypedRecords(n)
			runYaegiBench(b, yaegiSourceTyped, true, func(handle any) {
				fn, ok := handle.(func([]Record) float64)
				if !ok {
					b.Fatalf("eval returned %T", handle)
				}
				b.ReportAllocs()
				b.ResetTimer()
				var sink float64
				for j := 0; j < b.N; j++ {
					sink = fn(records)
				}
				_ = sink
			})
		})
	}
}

// ── Python (persistent subprocess, streamed per-call) ───────────────────────
//
// Honest per-message benchmark: each iteration JSON-encodes the records on the
// Go side, writes them through the pipe, the Python worker decodes the JSON,
// runs the compute, and sends the float result back. This mirrors what a real
// LOOPZE Python-Function-Node would have to do — every flow message arrives
// fresh on the Go side and has to cross the process boundary.
//
// Symmetry with the other engines:
//   - Goja pays vm.ToValue(records) per iter (Go→JS proxy materialisation).
//   - Yaegi/expr/Native get the records as a Go-native slice for free —
//     they live in the same process, no boundary, no conversion.
//   - Python pays json.Marshal + pipe write + json.loads — the unavoidable
//     cost of crossing into a separate process.
//
// The previous "preloaded" variant was misleading: a real Python-node-via-
// subprocess would never have the data already inside the worker.

const pythonWorkerStreamed = `
import sys, json
sys.stdin.reconfigure(line_buffering=False)
for line in sys.stdin:
    line = line.rstrip()
    if not line:
        continue
    records = json.loads(line)
    s = 0.0
    for r in records:
        t = r["temperature"]
        if t > 20:
            s += t * 1.8 + 32
    sys.stdout.write(repr(s) + "\n"); sys.stdout.flush()
`

func pythonBench(b *testing.B, records []map[string]any) {
	cmd := exec.Command("python3", "-u", "-c", pythonWorkerStreamed)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		b.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		b.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		b.Fatal(err)
	}
	// Pipe-line buffer must accommodate the JSON payload at 100k records
	// (~8 MB); the default 4 KB ReadString buffer would only matter on the
	// reply side though (small floats), so a default reader is fine here.
	out := bufio.NewReader(stdout)
	b.Cleanup(func() {
		_ = stdin.Close()
		_ = cmd.Wait()
	})

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		jsonBytes, err := json.Marshal(records)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := stdin.Write(jsonBytes); err != nil {
			b.Fatal(err)
		}
		if _, err := io.WriteString(stdin, "\n"); err != nil {
			b.Fatal(err)
		}
		line, err := out.ReadString('\n')
		if err != nil {
			b.Fatal(err)
		}
		if _, err := strconv.ParseFloat(strings.TrimSpace(line), 64); err != nil {
			b.Fatalf("bad result line %q: %v", line, err)
		}
	}
}

func BenchmarkPython(b *testing.B) {
	for _, n := range sizes {
		b.Run(fmt.Sprintf("map/%d", n), func(b *testing.B) {
			pythonBench(b, generateMapRecords(n))
		})
	}
}

// ── Sanity: all engines + shapes agree on the result ────────────────────────

func TestAllEnginesAgree(t *testing.T) {
	mapRecords := generateMapRecords(1000)
	typedRecords := generateTypedRecords(1000)
	want := nativeMap(mapRecords)
	if got := nativeTyped(typedRecords); got != want {
		t.Fatalf("native typed vs map disagree: %v vs %v", got, want)
	}

	// Goja map
	{
		vm := goja.New()
		prog, _ := goja.Compile("bench.js", gojaCode, false)
		fnVal, _ := vm.RunProgram(prog)
		fn, _ := goja.AssertFunction(fnVal)
		v, _ := fn(goja.Undefined(), vm.ToValue(mapRecords))
		if got := v.ToFloat(); got != want {
			t.Errorf("goja map: got %v, want %v", got, want)
		}
	}
	// Goja typed
	{
		vm := goja.New()
		prog, _ := goja.Compile("bench.js", gojaCodeTyped, false)
		fnVal, _ := vm.RunProgram(prog)
		fn, _ := goja.AssertFunction(fnVal)
		v, _ := fn(goja.Undefined(), vm.ToValue(typedRecords))
		if got := v.ToFloat(); got != want {
			t.Errorf("goja typed: got %v, want %v", got, want)
		}
	}

	// expr map
	{
		env := map[string]any{"records": mapRecords}
		program, _ := expr.Compile(exprCodeMap, expr.Env(env))
		out, err := exprvm.Run(program, env)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := out.(float64); got != want {
			t.Errorf("expr map: got %v, want %v", got, want)
		}
	}
	// expr typed
	{
		env := map[string]any{"records": typedRecords}
		program, _ := expr.Compile(exprCodeTyped, expr.Env(env))
		out, err := exprvm.Run(program, env)
		if err != nil {
			t.Fatal(err)
		}
		if got, _ := out.(float64); got != want {
			t.Errorf("expr typed: got %v, want %v", got, want)
		}
	}

	// Yaegi map
	{
		i := interp.New(interp.Options{})
		_ = i.Use(stdlib.Symbols)
		_, _ = i.Eval(yaegiSourceMap)
		v, _ := i.Eval("main.Handle")
		fn := v.Interface().(func([]map[string]any) float64)
		if got := fn(mapRecords); got != want {
			t.Errorf("yaegi map: got %v, want %v", got, want)
		}
	}
	// Yaegi typed
	{
		i := interp.New(interp.Options{})
		_ = i.Use(stdlib.Symbols)
		_ = i.Use(yaegiTypedExports)
		_, err := i.Eval(yaegiSourceTyped)
		if err != nil {
			t.Fatal(err)
		}
		v, err := i.Eval("main.Handle")
		if err != nil {
			t.Fatal(err)
		}
		fn, ok := v.Interface().(func([]Record) float64)
		if !ok {
			t.Fatalf("yaegi typed eval returned %T", v.Interface())
		}
		if got := fn(typedRecords); got != want {
			t.Errorf("yaegi typed: got %v, want %v", got, want)
		}
	}
}
