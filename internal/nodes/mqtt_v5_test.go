// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// Test that mqtt-in's onMessage handler maps MQTT v5 publish properties into
// the flow.Message according to the issue spec — userProperties as a map,
// contentType / responseTopic / correlationData / messageExpiry as their
// respective primitives, and only when present in the publish.
func TestMqttIn_OnMessage_V5Properties(t *testing.T) {
	expiry := uint32(60)
	pkt := &paho.Publish{
		Topic:   "sensor/temp",
		QoS:     1,
		Retain:  true,
		Payload: []byte("23.5"),
		Properties: &paho.PublishProperties{
			User: paho.UserProperties{
				{Key: "source", Value: "sensor-42"},
				{Key: "unit", Value: "celsius"},
			},
			ContentType:     "application/json",
			ResponseTopic:   "sensor/temp/reply",
			CorrelationData: []byte{0xCA, 0xFE},
			MessageExpiry:   &expiry,
		},
	}

	c := &collector{}
	n := &MqttInNode{BaseNode: BaseNode{Send: c.send}}
	n.onMessage(pkt)

	if c.count() != 1 {
		t.Fatalf("expected 1 message, got %d", c.count())
	}
	msg := c.last()

	if got, _ := msg.Get("topic").(string); got != "sensor/temp" {
		t.Errorf("topic = %q, want %q", got, "sensor/temp")
	}
	if got, _ := msg.Get("payload").(string); got != "23.5" {
		t.Errorf("payload = %q, want %q", got, "23.5")
	}
	if got, _ := msg.Get("qos").(int); got != 1 {
		t.Errorf("qos = %d, want 1", got)
	}
	if got, _ := msg.Get("retain").(bool); !got {
		t.Errorf("retain = false, want true")
	}

	up, ok := msg.Get("userProperties").(map[string]string)
	if !ok {
		t.Fatalf("userProperties type = %T, want map[string]string", msg.Get("userProperties"))
	}
	wantUP := map[string]string{"source": "sensor-42", "unit": "celsius"}
	if !reflect.DeepEqual(up, wantUP) {
		t.Errorf("userProperties = %v, want %v", up, wantUP)
	}

	if got, _ := msg.Get("contentType").(string); got != "application/json" {
		t.Errorf("contentType = %q, want %q", got, "application/json")
	}
	if got, _ := msg.Get("responseTopic").(string); got != "sensor/temp/reply" {
		t.Errorf("responseTopic = %q, want %q", got, "sensor/temp/reply")
	}
	if got, ok := msg.Get("correlationData").([]byte); !ok || !reflect.DeepEqual(got, []byte{0xCA, 0xFE}) {
		t.Errorf("correlationData = %v, want [202 254]", got)
	}
	if got, _ := msg.Get("messageExpiry").(uint32); got != 60 {
		t.Errorf("messageExpiry = %v, want 60", got)
	}
}

// Test that payloadFormat (v5 §3.3.2.3.2) and subscriptionIdentifier (v5
// §3.3.2.3.8) are mapped from the publish properties into the flow message.
func TestMqttIn_OnMessage_PayloadFormatAndSubscriptionIdentifier(t *testing.T) {
	pf := byte(1) // 1 = UTF-8 text
	subID := 42
	pkt := &paho.Publish{
		Topic:   "sensor/temp",
		Payload: []byte("hello"),
		Properties: &paho.PublishProperties{
			PayloadFormat:          &pf,
			SubscriptionIdentifier: &subID,
		},
	}

	c := &collector{}
	n := &MqttInNode{BaseNode: BaseNode{Send: c.send}}
	n.onMessage(pkt)

	msg := c.last()
	if got, _ := msg.Get("payloadFormat").(int); got != 1 {
		t.Errorf("payloadFormat = %v, want 1", got)
	}
	if got, _ := msg.Get("subscriptionIdentifier").(int); got != 42 {
		t.Errorf("subscriptionIdentifier = %v, want 42", got)
	}
}

// Tests for the configurable outputFormat: string (default), json, buffer.
// JSON-parse failure falls back to string and surfaces a parseError so flows
// can catch malformed payloads without losing the message.
func TestMqttIn_OnMessage_OutputFormat(t *testing.T) {
	t.Run("string (default) — bytes become Go string", func(t *testing.T) {
		c := &collector{}
		n := &MqttInNode{BaseNode: BaseNode{Send: c.send}, outputFormat: "string"}
		n.onMessage(&paho.Publish{Topic: "x", Payload: []byte(`{"a":1}`)})

		got, ok := c.last().Get("payload").(string)
		if !ok {
			t.Fatalf("payload type = %T, want string", c.last().Get("payload"))
		}
		if got != `{"a":1}` {
			t.Errorf("payload = %q", got)
		}
	})

	t.Run("buffer — bytes preserved as []int (round-trip-safe via JSON)", func(t *testing.T) {
		c := &collector{}
		n := &MqttInNode{BaseNode: BaseNode{Send: c.send}, outputFormat: "buffer"}
		raw := []byte{0xCA, 0xFE, 0xBA, 0xBE}
		n.onMessage(&paho.Publish{Topic: "x", Payload: raw})

		got, ok := c.last().Get("payload").([]int)
		if !ok {
			t.Fatalf("payload type = %T, want []int", c.last().Get("payload"))
		}
		want := []int{0xCA, 0xFE, 0xBA, 0xBE}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("payload = %v, want %v", got, want)
		}
		// Mutating the source bytes must not affect the emitted slice.
		raw[0] = 0
		got2, _ := c.last().Get("payload").([]int)
		if got2[0] != 0xCA {
			t.Errorf("emitted buffer aliased the source slice — got2[0]=%x", got2[0])
		}
	})

	t.Run("buffer — JSON round-trip preserves data and re-serialises to bytes", func(t *testing.T) {
		c := &collector{}
		n := &MqttInNode{BaseNode: BaseNode{Send: c.send}, outputFormat: "buffer"}
		raw := []byte{0xDE, 0xAD, 0xBE, 0xEF}
		n.onMessage(&paho.Publish{Topic: "x", Payload: raw})

		// Round-trip the message through JSON — exactly what happens on the
		// wire to the frontend or NATS. After this the payload is []any of
		// float64 (json.Unmarshal default).
		jsonBytes, err := c.last().MarshalJSON()
		if err != nil {
			t.Fatalf("marshal: %v", err)
		}
		// The wire format must be a number array, not a base64 string.
		if !strings.Contains(string(jsonBytes), `"payload":[222,173,190,239]`) {
			t.Errorf("expected number-array payload in JSON, got: %s", jsonBytes)
		}

		var roundtripped flow.Message
		if err := roundtripped.UnmarshalJSON(jsonBytes); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		// mqtt-out's converter must reconstruct the original bytes from the
		// post-JSON form (which is []any of float64).
		published, err := toBytes(roundtripped.Payload())
		if err != nil {
			t.Fatalf("toBytes: %v", err)
		}
		if !reflect.DeepEqual(published, raw) {
			t.Errorf("round-trip lost data: got %v, want %v", published, raw)
		}
	})

	t.Run("json — valid JSON is decoded to a structured value", func(t *testing.T) {
		c := &collector{}
		n := &MqttInNode{BaseNode: BaseNode{Send: c.send}, outputFormat: "json"}
		n.onMessage(&paho.Publish{Topic: "x", Payload: []byte(`{"a":1,"b":[true,null]}`)})

		got, ok := c.last().Get("payload").(map[string]any)
		if !ok {
			t.Fatalf("payload type = %T, want map[string]any", c.last().Get("payload"))
		}
		if v, _ := got["a"].(float64); v != 1 {
			t.Errorf("payload[a] = %v, want 1", got["a"])
		}
		if pe := c.last().Get("parseError"); pe != nil {
			t.Errorf("parseError should be absent for valid JSON, got %v", pe)
		}
	})

	t.Run("json — invalid JSON falls back to string + parseError", func(t *testing.T) {
		c := &collector{}
		n := &MqttInNode{BaseNode: BaseNode{Send: c.send}, outputFormat: "json"}
		n.onMessage(&paho.Publish{Topic: "x", Payload: []byte(`not json`)})

		got, ok := c.last().Get("payload").(string)
		if !ok {
			t.Fatalf("payload type = %T, want string fallback", c.last().Get("payload"))
		}
		if got != "not json" {
			t.Errorf("fallback payload = %q", got)
		}
		if pe, _ := c.last().Get("parseError").(string); pe == "" {
			t.Errorf("expected parseError to be set on JSON failure")
		}
	})
}

// Test that an MQTT v3-style publish (no Properties) doesn't surface any v5
// fields in the flow message — they must be absent, not zero-valued, so that
// downstream nodes can detect presence with a nil check.
func TestMqttIn_OnMessage_NoV5Properties(t *testing.T) {
	pkt := &paho.Publish{
		Topic:      "sensor/temp",
		QoS:        0,
		Retain:     false,
		Payload:    []byte("plain"),
		Properties: nil,
	}

	c := &collector{}
	n := &MqttInNode{BaseNode: BaseNode{Send: c.send}}
	n.onMessage(pkt)

	msg := c.last()
	for _, field := range []string{"userProperties", "contentType", "responseTopic", "correlationData", "messageExpiry", "payloadFormat", "subscriptionIdentifier"} {
		if v := msg.Get(field); v != nil {
			t.Errorf("expected %q to be absent, got %v", field, v)
		}
	}
}

// Test that applyDynamicOverrides starts from the configured defaults and
// only overlays the msg fields that are present and valid. Missing or invalid
// values must leave the configured defaults untouched.
func TestMqttIn_ApplyDynamicOverrides(t *testing.T) {
	t.Run("no msg fields → config defaults preserved", func(t *testing.T) {
		n := &MqttInNode{
			qos:                    1,
			noLocal:                true,
			retainAsPublished:      true,
			retainHandling:         2,
			subscriptionIdentifier: 7,
		}
		msg := flow.NewMessage()
		opts := n.applyDynamicOverrides(msg)

		if opts.QoS != 1 || !opts.NoLocal || !opts.RetainAsPublished || opts.RetainHandling != 2 || opts.SubscriptionIdentifier != 7 {
			t.Errorf("defaults not preserved: %+v", opts)
		}
	})

	t.Run("msg fields override config", func(t *testing.T) {
		n := &MqttInNode{
			qos:                    0,
			noLocal:                false,
			retainAsPublished:      false,
			retainHandling:         0,
			subscriptionIdentifier: 0,
		}
		msg := flow.NewMessage()
		msg.Set("qos", float64(2))
		msg.Set("noLocal", true)
		msg.Set("retainAsPublished", true)
		msg.Set("retainHandling", float64(1))
		msg.Set("subscriptionIdentifier", float64(99))

		opts := n.applyDynamicOverrides(msg)
		if opts.QoS != 2 {
			t.Errorf("QoS = %d, want 2", opts.QoS)
		}
		if !opts.NoLocal {
			t.Errorf("NoLocal = false, want true")
		}
		if !opts.RetainAsPublished {
			t.Errorf("RetainAsPublished = false, want true")
		}
		if opts.RetainHandling != 1 {
			t.Errorf("RetainHandling = %d, want 1", opts.RetainHandling)
		}
		if opts.SubscriptionIdentifier != 99 {
			t.Errorf("SubscriptionIdentifier = %d, want 99", opts.SubscriptionIdentifier)
		}
	})

	t.Run("invalid msg.retainHandling falls back to config", func(t *testing.T) {
		n := &MqttInNode{retainHandling: 2}
		msg := flow.NewMessage()
		msg.Set("retainHandling", float64(7)) // out of range

		opts := n.applyDynamicOverrides(msg)
		if opts.RetainHandling != 2 {
			t.Errorf("RetainHandling = %d, want 2 (config fallback)", opts.RetainHandling)
		}
	})

	t.Run("zero/negative subscriptionIdentifier falls back to config", func(t *testing.T) {
		n := &MqttInNode{subscriptionIdentifier: 5}
		msg := flow.NewMessage()
		msg.Set("subscriptionIdentifier", float64(0))

		opts := n.applyDynamicOverrides(msg)
		if opts.SubscriptionIdentifier != 5 {
			t.Errorf("SubscriptionIdentifier = %d, want 5 (config fallback)", opts.SubscriptionIdentifier)
		}
	})
}

// Tests for MqttOutNode.mergeV5PublishProperties — the function that combines
// node-level defaults with per-message overrides. With no defaults set, it
// behaves exactly like reading the msg fields straight; with defaults set, msg
// fields override them per the documented merge rules.
func TestMqttOut_MergeV5PublishProperties_NoDefaults(t *testing.T) {
	t.Run("no v5 fields → nil", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("payload", "hello")
		got := n.mergeV5PublishProperties(msg)
		if got != nil {
			t.Errorf("expected nil, got %+v", got)
		}
	})

	t.Run("user properties as map[string]string", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("userProperties", map[string]string{"src": "abc"})
		props := n.mergeV5PublishProperties(msg)
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if got := props.User.Get("src"); got != "abc" {
			t.Errorf("User[src] = %q, want %q", got, "abc")
		}
	})

	t.Run("user properties as map[string]any", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("userProperties", map[string]any{
			"src":    "abc",
			"ignore": 42, // non-string values are skipped
		})
		props := n.mergeV5PublishProperties(msg)
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if got := props.User.Get("src"); got != "abc" {
			t.Errorf("User[src] = %q, want %q", got, "abc")
		}
		if got := props.User.Get("ignore"); got != "" {
			t.Errorf("User[ignore] = %q, want empty (non-string)", got)
		}
	})

	t.Run("all scalar fields from msg", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("contentType", "application/json")
		msg.Set("responseTopic", "reply/topic")
		msg.Set("correlationData", []byte{0x01, 0x02})
		msg.Set("messageExpiry", float64(120))
		msg.Set("payloadFormat", float64(1))

		props := n.mergeV5PublishProperties(msg)
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if props.ContentType != "application/json" {
			t.Errorf("ContentType = %q", props.ContentType)
		}
		if props.ResponseTopic != "reply/topic" {
			t.Errorf("ResponseTopic = %q", props.ResponseTopic)
		}
		if !reflect.DeepEqual(props.CorrelationData, []byte{0x01, 0x02}) {
			t.Errorf("CorrelationData = %v", props.CorrelationData)
		}
		if props.MessageExpiry == nil || *props.MessageExpiry != 120 {
			t.Errorf("MessageExpiry = %v, want 120", props.MessageExpiry)
		}
		if props.PayloadFormat == nil || *props.PayloadFormat != 1 {
			t.Errorf("PayloadFormat = %v, want 1", props.PayloadFormat)
		}
	})

	t.Run("correlationData as string (non-base64 fallback)", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("correlationData", "abc") // 3 chars → invalid base64 (needs padding) → literal fallback
		props := n.mergeV5PublishProperties(msg)
		if props == nil || string(props.CorrelationData) != "abc" {
			t.Errorf("CorrelationData = %v, want \"abc\" (literal fallback)", props)
		}
	})

	t.Run("correlationData as base64 string is decoded", func(t *testing.T) {
		// Repro of the round-trip path: mqtt-in sets correlationData as []byte,
		// the msg goes through a function node (or NATS routing) that JSON-
		// roundtrips it, Go encodes []byte as base64 → mqtt-out must decode
		// back to the original bytes so mqtt-request can match the response.
		n := &MqttOutNode{}
		original := []byte{0x3a, 0xad, 0xe5, 0xfc, 0xce, 0x06, 0x6f, 0x85, 0x6b, 0x4f, 0xa3, 0x09, 0x0d, 0xed, 0x9d, 0x9c}
		msg := flow.NewMessage()
		msg.Set("correlationData", "Oq3l/M4Gb4VrT6MJDe2dnA==") // base64 of `original`
		props := n.mergeV5PublishProperties(msg)
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if !reflect.DeepEqual(props.CorrelationData, original) {
			t.Errorf("CorrelationData = %x, want %x", props.CorrelationData, original)
		}
	})

	t.Run("correlationData as []any (numeric array round-trip)", func(t *testing.T) {
		// JSON-roundtrip of an []int from JS function-node decoding.
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("correlationData", []any{float64(0xDE), float64(0xAD), float64(0xBE), float64(0xEF)})
		props := n.mergeV5PublishProperties(msg)
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if !reflect.DeepEqual(props.CorrelationData, []byte{0xDE, 0xAD, 0xBE, 0xEF}) {
			t.Errorf("CorrelationData = %x, want DEADBEEF", props.CorrelationData)
		}
	})

	t.Run("empty strings are not set", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("contentType", "")
		msg.Set("responseTopic", "")
		props := n.mergeV5PublishProperties(msg)
		if props != nil {
			t.Errorf("empty strings should not produce props, got %+v", props)
		}
	})

	t.Run("negative messageExpiry rejected", func(t *testing.T) {
		n := &MqttOutNode{}
		msg := flow.NewMessage()
		msg.Set("messageExpiry", float64(-5))
		props := n.mergeV5PublishProperties(msg)
		if props != nil {
			t.Errorf("negative messageExpiry should be rejected, got %+v", props)
		}
	})
}

// Tests for the merge behaviour: defaults fall back when msg is silent; msg
// overrides defaults per-field; user-properties merge per-key (msg wins on
// conflicts, non-overlapping keys from both sides survive).
func TestMqttOut_MergeV5PublishProperties_WithDefaults(t *testing.T) {
	expiry := uint32(60)
	pf := byte(1)

	defaults := func() *MqttOutNode {
		return &MqttOutNode{
			defaultUserProperties: map[string]string{"app": "loopze", "shared": "config"},
			defaultContentType:    "application/json",
			defaultResponseTopic:  "default/reply",
			defaultMessageExpiry:  &expiry,
			defaultPayloadFormat:  &pf,
		}
	}

	t.Run("empty msg → all defaults applied", func(t *testing.T) {
		props := defaults().mergeV5PublishProperties(flow.NewMessage())
		if props == nil {
			t.Fatal("expected non-nil props")
		}
		if props.ContentType != "application/json" {
			t.Errorf("ContentType = %q, want default", props.ContentType)
		}
		if props.ResponseTopic != "default/reply" {
			t.Errorf("ResponseTopic = %q, want default", props.ResponseTopic)
		}
		if props.MessageExpiry == nil || *props.MessageExpiry != 60 {
			t.Errorf("MessageExpiry = %v, want 60", props.MessageExpiry)
		}
		if props.PayloadFormat == nil || *props.PayloadFormat != 1 {
			t.Errorf("PayloadFormat = %v, want 1", props.PayloadFormat)
		}
		if got := props.User.Get("app"); got != "loopze" {
			t.Errorf("User[app] = %q, want loopze", got)
		}
	})

	t.Run("msg overrides scalars", func(t *testing.T) {
		msg := flow.NewMessage()
		msg.Set("contentType", "text/plain")
		msg.Set("messageExpiry", float64(10))
		props := defaults().mergeV5PublishProperties(msg)
		if props.ContentType != "text/plain" {
			t.Errorf("ContentType = %q, want text/plain (msg wins)", props.ContentType)
		}
		if props.ResponseTopic != "default/reply" {
			t.Errorf("ResponseTopic = %q, want default (msg silent)", props.ResponseTopic)
		}
		if *props.MessageExpiry != 10 {
			t.Errorf("MessageExpiry = %d, want 10 (msg wins)", *props.MessageExpiry)
		}
	})

	t.Run("user properties merge — msg overrides matching keys, non-overlapping survive", func(t *testing.T) {
		msg := flow.NewMessage()
		msg.Set("userProperties", map[string]string{
			"app":     "override",     // overrides default "loopze"
			"request": "msg-specific", // new key from msg
		})
		props := defaults().mergeV5PublishProperties(msg)
		if got := props.User.Get("app"); got != "override" {
			t.Errorf("User[app] = %q, want override (msg wins)", got)
		}
		if got := props.User.Get("shared"); got != "config" {
			t.Errorf("User[shared] = %q, want config (default survives)", got)
		}
		if got := props.User.Get("request"); got != "msg-specific" {
			t.Errorf("User[request] = %q, want msg-specific (msg-only key)", got)
		}
	})

	t.Run("invalid msg.payloadFormat falls back to default", func(t *testing.T) {
		msg := flow.NewMessage()
		msg.Set("payloadFormat", float64(7)) // out of range
		props := defaults().mergeV5PublishProperties(msg)
		if props.PayloadFormat == nil || *props.PayloadFormat != 1 {
			t.Errorf("PayloadFormat = %v, want default 1", props.PayloadFormat)
		}
	})

	t.Run("correlationData has no default — only set when msg provides it", func(t *testing.T) {
		props := defaults().mergeV5PublishProperties(flow.NewMessage())
		if props.CorrelationData != nil {
			t.Errorf("CorrelationData should not have a default, got %v", props.CorrelationData)
		}
	})
}
