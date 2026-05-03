// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"testing"

	"github.com/eclipse/paho.golang/paho"
)

func TestTopicMatchesPattern(t *testing.T) {
	cases := []struct {
		pattern string
		topic   string
		match   bool
	}{
		// exact matches
		{"sensor/temp", "sensor/temp", true},
		{"sensor/temp", "sensor/humidity", false},
		{"a", "a", true},
		{"a", "a/b", false},
		{"a/b", "a", false},

		// + wildcard (single level)
		{"sensor/+", "sensor/temp", true},
		{"sensor/+", "sensor/humidity", true},
		{"sensor/+", "sensor/temp/value", false},
		{"sensor/+", "sensor", false},
		{"+/temp", "sensor/temp", true},
		{"+/temp", "actor/temp", true},
		{"sensor/+/value", "sensor/temp/value", true},
		{"sensor/+/value", "sensor/temp/avg", false},
		{"+/+", "a/b", true},
		{"+/+", "a/b/c", false},

		// # wildcard (multi-level, must be terminal)
		{"sensor/#", "sensor/temp", true},
		{"sensor/#", "sensor/temp/value", true},
		{"sensor/#", "sensor", true},
		{"sensor/#", "actor/temp", false},
		{"#", "anything", true},
		{"#", "a/b/c/d", true},
		{"a/+/#", "a/b/c", true},
		{"a/+/#", "a/b", true},
		{"a/+/#", "a", false},

		// edge cases
		{"", "", true},
		{"a/b/c", "a/b", false},
		{"a/b", "a/b/c", false},
	}

	for _, c := range cases {
		got := topicMatchesPattern(c.pattern, c.topic)
		if got != c.match {
			t.Errorf("topicMatchesPattern(%q, %q) = %v, want %v", c.pattern, c.topic, got, c.match)
		}
	}
}

// Tests for the in-memory subscribe/unsubscribe bookkeeping. These exercise
// muxSubscriptions and the shared-vs-separate paho-subscribe decision without
// touching the network — when cm is nil, the broker still maintains its
// internal state correctly so a later OnConnectionUp can issue the SUBSCRIBEs.

func newOfflineBroker(t *testing.T) *MqttBroker {
	t.Helper()
	return &MqttBroker{
		id:               "broker-test",
		subscribers:      make(map[string]map[string]muxKey),
		muxSubscriptions: make(map[muxKey]*muxEntry),
	}
}

func TestSubscribe_SameKey_SharesEntry(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	if err := b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0}, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.Subscribe("node-B", "sensor/+", SubscribeOptions{QoS: 0}, noop); err != nil {
		t.Fatal(err)
	}

	if got := len(b.muxSubscriptions); got != 1 {
		t.Errorf("expected 1 mux entry (shared), got %d", got)
	}
	for _, entry := range b.muxSubscriptions {
		if got := len(entry.handlers); got != 2 {
			t.Errorf("expected 2 handlers under shared entry, got %d", got)
		}
	}
}

func TestSubscribe_DifferentNoLocal_SeparateEntries(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	if err := b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0, NoLocal: false}, noop); err != nil {
		t.Fatal(err)
	}
	if err := b.Subscribe("node-B", "sensor/+", SubscribeOptions{QoS: 0, NoLocal: true}, noop); err != nil {
		t.Fatal(err)
	}

	if got := len(b.muxSubscriptions); got != 2 {
		t.Errorf("expected 2 mux entries (different NoLocal), got %d", got)
	}
}

func TestSubscribe_DifferentSubscriptionIdentifier_SeparateEntries(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0, SubscriptionIdentifier: 42}, noop)
	_ = b.Subscribe("node-B", "sensor/+", SubscribeOptions{QoS: 0, SubscriptionIdentifier: 7}, noop)

	if got := len(b.muxSubscriptions); got != 2 {
		t.Errorf("expected 2 mux entries (different SubID), got %d", got)
	}
}

func TestSubscribe_QoSEscalation(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0}, noop)
	_ = b.Subscribe("node-B", "sensor/+", SubscribeOptions{QoS: 2}, noop)

	if got := len(b.muxSubscriptions); got != 1 {
		t.Fatalf("expected 1 mux entry, got %d", got)
	}
	for _, entry := range b.muxSubscriptions {
		if entry.qos != 2 {
			t.Errorf("entry.qos = %d, want 2 (escalated)", entry.qos)
		}
	}
}

func TestUnsubscribe_KeepsEntryWhilePeerRemains(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0}, noop)
	_ = b.Subscribe("node-B", "sensor/+", SubscribeOptions{QoS: 0}, noop)

	if err := b.Unsubscribe("node-A", "sensor/+"); err != nil {
		t.Fatal(err)
	}

	if got := len(b.muxSubscriptions); got != 1 {
		t.Errorf("expected 1 mux entry after partial unsub, got %d", got)
	}
	for _, entry := range b.muxSubscriptions {
		if got := len(entry.handlers); got != 1 {
			t.Errorf("expected 1 handler left, got %d", got)
		}
	}
}

func TestUnsubscribe_LastPeer_RemovesEntry(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0}, noop)
	if err := b.Unsubscribe("node-A", "sensor/+"); err != nil {
		t.Fatal(err)
	}

	if got := len(b.muxSubscriptions); got != 0 {
		t.Errorf("expected 0 mux entries after last unsub, got %d", got)
	}
	if got := len(b.subscribers); got != 0 {
		t.Errorf("expected subscribers cleared, got %d", got)
	}
}

func TestSubscribe_ResubscribeWithDifferentOptions_ReplacesEntry(t *testing.T) {
	b := newOfflineBroker(t)
	noop := func(*paho.Publish) {}

	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0, NoLocal: false}, noop)
	_ = b.Subscribe("node-A", "sensor/+", SubscribeOptions{QoS: 0, NoLocal: true}, noop)

	if got := len(b.muxSubscriptions); got != 1 {
		t.Errorf("expected 1 mux entry after resub, got %d", got)
	}
	for key := range b.muxSubscriptions {
		if !key.noLocal {
			t.Errorf("expected new entry to have NoLocal=true, got key %+v", key)
		}
	}
}
