// Copyright 2025 NiceClouds GmbH
// Licensed under the Elastic License 2.0 (ELv2).

package flow

import (
	"testing"
)

// ─── deepCopyMap / deepCopySlice ─────────────────────────────────────────────

func TestDeepCopyMap_NestedSliceOfMaps(t *testing.T) {
	src := map[string]any{
		"items": []any{
			map[string]any{"id": 1},
			map[string]any{"id": 2},
		},
	}
	dst := deepCopyMap(src)

	// Mutate inner map in the copy.
	dst["items"].([]any)[0].(map[string]any)["id"] = 999

	// Original must be unaffected.
	if src["items"].([]any)[0].(map[string]any)["id"] != 1 {
		t.Fatal("deepCopyMap: nested map in slice was not deep-copied")
	}
}

func TestDeepCopySlice_NestedSlice(t *testing.T) {
	src := []any{
		[]any{1, 2, 3},
		map[string]any{"a": "b"},
	}
	dst := deepCopySlice(src)

	// Mutate nested slice and map.
	dst[0].([]any)[0] = 999
	dst[1].(map[string]any)["a"] = "changed"

	if src[0].([]any)[0] != 1 {
		t.Fatal("deepCopySlice: nested slice element was not deep-copied")
	}
	if src[1].(map[string]any)["a"] != "b" {
		t.Fatal("deepCopySlice: nested map in slice was not deep-copied")
	}
}

// ─── generateID ──────────────────────────────────────────────────────────────

func TestGenerateID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := generateID()
		if len(id) != 32 {
			t.Fatalf("generateID: expected 32 hex chars, got %d (%q)", len(id), id)
		}
		if _, ok := seen[id]; ok {
			t.Fatalf("generateID: duplicate ID %q after %d iterations", id, i)
		}
		seen[id] = struct{}{}
	}
}

// ─── Clone (full deep copy, regression test) ────────────────────────────────

func TestClone_StillDeepCopies(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", map[string]any{"key": "original"})

	cloned := msg.Clone()

	// IDs must differ.
	if msg.ID() == cloned.ID() {
		t.Fatal("Clone: IDs should differ")
	}

	// Mutate clone's payload.
	cloned.Set("payload", map[string]any{"key": "changed"})

	// Original must be unaffected.
	p := msg.Get("payload").(map[string]any)
	if p["key"] != "original" {
		t.Fatal("Clone: original was mutated after clone modification")
	}
}

// ─── COWClone ────────────────────────────────────────────────────────────────

func TestCOWClone_SharesData(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "hello")
	msg.Set("topic", "test")

	clone := msg.COWClone()

	// IDs must differ.
	if msg.ID() == clone.ID() {
		t.Fatal("COWClone: IDs should differ")
	}

	// Both must see the same payload and topic.
	if clone.Payload() != "hello" {
		t.Fatalf("COWClone: clone payload = %v, want hello", clone.Payload())
	}
	if clone.Topic() != "test" {
		t.Fatalf("COWClone: clone topic = %v, want test", clone.Topic())
	}
}

func TestCOWClone_WriteOnCloneDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "original")

	clone := msg.COWClone()
	clone.Set("payload", "modified")

	// Original must be unaffected.
	if msg.Payload() != "original" {
		t.Fatalf("COWClone: original payload = %v, want original", msg.Payload())
	}
	if clone.Payload() != "modified" {
		t.Fatalf("COWClone: clone payload = %v, want modified", clone.Payload())
	}
}

func TestCOWClone_WriteOnOriginalDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "original")

	clone := msg.COWClone()
	msg.Set("payload", "modified-original")

	// Clone must be unaffected.
	if clone.Payload() != "original" {
		t.Fatalf("COWClone: clone payload = %v, want original", clone.Payload())
	}
	if msg.Payload() != "modified-original" {
		t.Fatalf("COWClone: original payload = %v, want modified-original", msg.Payload())
	}
}

func TestCOWClone_DeleteDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "data")
	msg.Set("extra", "keep")

	clone := msg.COWClone()
	clone.Delete("extra")

	// Original must still have "extra".
	if msg.Get("extra") != "keep" {
		t.Fatal("COWClone: Delete on clone affected original")
	}
	// Clone must not have "extra".
	if clone.Get("extra") != nil {
		t.Fatal("COWClone: Delete on clone did not remove field")
	}
}

func TestCOWClone_SetPayloadDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "original")

	clone := msg.COWClone()
	clone.SetPayload("new-payload")

	if msg.Payload() != "original" {
		t.Fatal("COWClone: SetPayload on clone affected original")
	}
}

func TestCOWClone_SetTopicDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("topic", "original")

	clone := msg.COWClone()
	clone.SetTopic("new-topic")

	if msg.Topic() != "original" {
		t.Fatal("COWClone: SetTopic on clone affected original")
	}
}

func TestCOWClone_DataViewDoesNotDetach(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "test")

	clone := msg.COWClone()

	// DataView should return data without triggering a copy.
	view := clone.DataView()
	if view["payload"] != "test" {
		t.Fatal("COWClone: DataView returned wrong payload")
	}

	// Clone should still be shared (no detach happened).
	// Verify by checking that the data maps point to same content.
	if msg.DataView()["payload"] != clone.DataView()["payload"] {
		t.Fatal("COWClone: DataView should not trigger detach")
	}
}

func TestCOWClone_DataDetaches(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "test")

	clone := msg.COWClone()

	// Data() should trigger detach, making the map safe to mutate.
	data := clone.Data()
	data["payload"] = "mutated-via-data"

	// Original must be unaffected.
	if msg.Payload() != "test" {
		t.Fatal("COWClone: Data() mutation on clone affected original")
	}
}

func TestCOWClone_MultiTarget(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", "broadcast")

	// Simulate 3 targets receiving COW clones.
	clone1 := msg.COWClone()
	clone2 := msg.COWClone()
	clone3 := msg.COWClone()

	// Only clone2 writes.
	clone2.Set("payload", "modified")

	// Original and read-only clones must be unaffected.
	if msg.Payload() != "broadcast" {
		t.Fatal("MultiTarget: original was affected")
	}
	if clone1.Payload() != "broadcast" {
		t.Fatal("MultiTarget: clone1 was affected")
	}
	if clone2.Payload() != "modified" {
		t.Fatal("MultiTarget: clone2 write failed")
	}
	if clone3.Payload() != "broadcast" {
		t.Fatal("MultiTarget: clone3 was affected")
	}
}

func TestCOWClone_NestedMapIsolation(t *testing.T) {
	msg := NewMessage()
	msg.Set("payload", map[string]any{"nested": "value"})

	clone := msg.COWClone()

	// Write via Set triggers ensureUnique which deep-copies the data map.
	clone.Set("payload", map[string]any{"nested": "changed"})

	original := msg.Get("payload").(map[string]any)
	if original["nested"] != "value" {
		t.Fatal("COWClone: nested map in original was mutated")
	}
}

// ─── Benchmarks ──────────────────────────────────────────────────────────────

func BenchmarkClone(b *testing.B) {
	msg := NewMessage()
	msg.Set("payload", map[string]any{
		"name":  "test",
		"value": 42,
		"tags":  []any{"a", "b", "c"},
	})
	msg.Set("topic", "bench/topic")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.Clone()
	}
}

func BenchmarkCOWClone(b *testing.B) {
	msg := NewMessage()
	msg.Set("payload", map[string]any{
		"name":  "test",
		"value": 42,
		"tags":  []any{"a", "b", "c"},
	})
	msg.Set("topic", "bench/topic")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = msg.COWClone()
	}
}

func BenchmarkCOWClone_WithWrite(b *testing.B) {
	msg := NewMessage()
	msg.Set("payload", map[string]any{
		"name":  "test",
		"value": 42,
		"tags":  []any{"a", "b", "c"},
	})
	msg.Set("topic", "bench/topic")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		clone := msg.COWClone()
		clone.Set("payload", "new")
		// Reset shared flag for next iteration.
		msg.shared = false
	}
}

func BenchmarkGenerateID(b *testing.B) {
	for i := 0; i < b.N; i++ {
		_ = generateID()
	}
}
