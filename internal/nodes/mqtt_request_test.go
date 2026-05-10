// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package nodes

import (
	"bytes"
	"encoding/hex"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eclipse/paho.golang/paho"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// TestMqttOut_EffectiveTopic exercises the new target=topic|responseTopic
// resolution logic on mqtt-out. The function is independent of any broker so
// we drive it directly without touching the network.
func TestMqttOut_EffectiveTopic(t *testing.T) {
	cases := []struct {
		name      string
		target    string
		topic     string
		msgFields map[string]any
		want      string
		wantErr   string
	}{
		{
			name:   "target=topic, configured topic wins",
			target: "topic",
			topic:  "actuator/cmd",
			msgFields: map[string]any{
				"topic": "ignored",
			},
			want: "actuator/cmd",
		},
		{
			name:      "target=topic, falls back to msg.topic",
			target:    "topic",
			topic:     "",
			msgFields: map[string]any{"topic": "from/msg"},
			want:      "from/msg",
		},
		{
			name:      "target=topic, no topic anywhere → error",
			target:    "topic",
			topic:     "",
			msgFields: map[string]any{},
			wantErr:   "no topic configured and msg.topic is empty",
		},
		{
			name:   "target=responseTopic, picks msg.responseTopic",
			target: "responseTopic",
			topic:  "config/should/be/ignored",
			msgFields: map[string]any{
				"topic":         "msgtopic/should/be/ignored",
				"responseTopic": "loopze/response/abc123",
			},
			want: "loopze/response/abc123",
		},
		{
			name:   "target=responseTopic, missing responseTopic → error",
			target: "responseTopic",
			topic:  "config/topic",
			msgFields: map[string]any{
				"topic": "still/ignored",
			},
			wantErr: "msg.responseTopic is missing",
		},
		{
			name:   "target=responseTopic, empty responseTopic → error",
			target: "responseTopic",
			msgFields: map[string]any{
				"responseTopic": "",
			},
			wantErr: "msg.responseTopic is missing",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			n := &MqttOutNode{
				config: flow.NodeConfig{ID: "test-out"},
				target: tc.target,
				topic:  tc.topic,
			}
			msg := flow.NewMessage()
			for k, v := range tc.msgFields {
				msg.Set(k, v)
			}

			got, err := n.effectiveTopic(msg)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got topic=%q nil", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("topic = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestMqttOut_Init_TargetValidation verifies Init() rejects unknown target
// values and accepts the documented ones (including the empty default).
func TestMqttOut_Init_TargetValidation(t *testing.T) {
	cases := []struct {
		target  any
		wantErr bool
		want    string
	}{
		{target: "", wantErr: false, want: "topic"},
		{target: nil, wantErr: false, want: "topic"},
		{target: "topic", wantErr: false, want: "topic"},
		{target: "responseTopic", wantErr: false, want: "responseTopic"},
		{target: "garbage", wantErr: true},
	}
	for _, tc := range cases {
		t.Run("target="+stringify(tc.target), func(t *testing.T) {
			n := &MqttOutNode{
				config: flow.NodeConfig{
					ID: "n1",
					Properties: map[string]any{
						"broker": "broker-1",
						"target": tc.target,
					},
				},
			}
			err := n.Init()
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n.target != tc.want {
				t.Errorf("n.target = %q, want %q", n.target, tc.want)
			}
		})
	}
}

func stringify(v any) string {
	if v == nil {
		return "nil"
	}
	return strings.ReplaceAll(strings.ReplaceAll(reflect.TypeOf(v).String()+":"+reflectValueString(v), " ", "_"), "/", "_")
}

func reflectValueString(v any) string {
	rv := reflect.ValueOf(v)
	if !rv.IsValid() {
		return "<invalid>"
	}
	return rv.String()
}

// ── mqtt-request tests ──────────────────────────────────────────────────────

// TestMqttRequest_BuildPublishProperties verifies that the per-request
// ResponseTopic / CorrelationData are always set, that user-supplied
// msg.responseTopic / msg.correlationData are NOT honoured, and that the v5
// merge logic mirrors mqtt-out's behaviour (msg overrides config defaults).
func TestMqttRequest_BuildPublishProperties(t *testing.T) {
	n := &MqttRequestNode{
		defaultUserProperties: map[string]string{"src": "config"},
		defaultContentType:    "application/json",
	}
	msg := flow.NewMessage()
	msg.Set("userProperties", map[string]string{"trace": "abc"})
	msg.Set("contentType", "text/plain")
	msg.Set("responseTopic", "should/be/ignored")
	msg.Set("correlationData", []byte("ignored"))

	correlation := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	props := n.buildPublishProperties(msg, "loopze/response/uniq", correlation)

	if props.ResponseTopic != "loopze/response/uniq" {
		t.Errorf("ResponseTopic = %q, want %q", props.ResponseTopic, "loopze/response/uniq")
	}
	if !bytes.Equal(props.CorrelationData, correlation) {
		t.Errorf("CorrelationData = %v, want %v", props.CorrelationData, correlation)
	}
	if props.ContentType != "text/plain" {
		t.Errorf("ContentType = %q, want %q (msg overrides default)", props.ContentType, "text/plain")
	}
	// User properties: config keys + msg keys present; msg keys win on overlap.
	if got := props.User.Get("src"); got != "config" {
		t.Errorf("User[src] = %q, want %q", got, "config")
	}
	if got := props.User.Get("trace"); got != "abc" {
		t.Errorf("User[trace] = %q, want %q", got, "abc")
	}
}

// fakeBrokerActions records every Subscribe/Unsubscribe/Publish call against
// MqttBroker so tests can assert behaviour without a network. Because the
// MqttRequestNode talks to *MqttBroker concretely (not an interface), we
// drive it with a real MqttBroker that has cm == nil; under those conditions
// Subscribe/Unsubscribe maintain the in-memory bookkeeping but never reach a
// paho client (Publish would fail with "not connected"). For tests that need
// to bypass the offline check we instead exercise the response/timeout/cleanup
// logic directly via the node's exported entry points.

// TestMqttRequest_OnResponse_HappyPath stages a pending inflight context
// directly and feeds in a matching paho.Publish — the node must clean up,
// emit one output message, and update its inflight count.
func TestMqttRequest_OnResponse_HappyPath(t *testing.T) {
	broker := newOfflineBroker(t)

	correlation := []byte{0x11, 0x22, 0x33, 0x44}
	responseTopic := "loopze/response/x"

	var sent []*flow.Message
	var sendMu sync.Mutex

	node := &MqttRequestNode{
		config:         flow.NodeConfig{ID: "req-1"},
		broker:         broker,
		responseFormat: "string",
		timeoutMode:    mqttRequestTimeoutModeError,
		pending:        make(map[string]*mqttRequestInflight),
		BaseNode: BaseNode{
			Send: func(port int, m *flow.Message) {
				sendMu.Lock()
				sent = append(sent, m)
				sendMu.Unlock()
			},
			Status: func(string, string) {},
		},
	}
	// Pre-register an inflight context, mimicking what HandleMessage would do.
	in := flow.NewMessage()
	in.Set("topic", "rpc/foo")
	in.Set("payload", "request-body")
	node.pending[hex.EncodeToString(correlation)] = &mqttRequestInflight{
		correlation:   correlation,
		responseTopic: responseTopic,
		inMsg:         in,
	}

	// Also register the broker subscription so Unsubscribe in onResponse
	// has a target to remove.
	if err := broker.Subscribe("req-1", responseTopic, SubscribeOptions{NoLocal: true, RetainHandling: 2}, func(*paho.Publish) {}); err != nil {
		t.Fatalf("seed subscribe: %v", err)
	}

	expiry := uint32(60)
	pf := byte(1)
	pub := &paho.Publish{
		Topic:   responseTopic,
		Payload: []byte(`{"ok":true}`),
		QoS:     1,
		Retain:  false,
		Properties: &paho.PublishProperties{
			CorrelationData: correlation,
			ContentType:     "application/json",
			MessageExpiry:   &expiry,
			PayloadFormat:   &pf,
		},
	}

	node.onResponse(pub)

	if len(sent) != 1 {
		t.Fatalf("expected 1 message sent, got %d", len(sent))
	}
	out := sent[0]
	if out.Get("topic") != responseTopic {
		t.Errorf("out.topic = %v, want %v", out.Get("topic"), responseTopic)
	}
	if out.Get("requestTopic") != "rpc/foo" {
		t.Errorf("out.requestTopic = %v, want rpc/foo", out.Get("requestTopic"))
	}
	if got := out.Payload(); got != `{"ok":true}` {
		t.Errorf("payload = %v, want raw string", got)
	}
	if got, _ := out.Get("correlationData").([]byte); !bytes.Equal(got, correlation) {
		t.Errorf("correlationData = %v, want %v", got, correlation)
	}
	if out.Get("contentType") != "application/json" {
		t.Errorf("contentType = %v", out.Get("contentType"))
	}
	if out.Get("payloadFormat") != 1 {
		t.Errorf("payloadFormat = %v, want 1", out.Get("payloadFormat"))
	}
	// Pending must be drained.
	if len(node.pending) != 0 {
		t.Errorf("pending = %d, want 0 after response", len(node.pending))
	}
	// Subscription must be released.
	if _, ok := broker.subscribers["req-1"]; ok {
		t.Errorf("expected no broker subscriptions for req-1 after response")
	}
}

// TestMqttRequest_OnResponse_WrongCorrelation drops responses whose
// correlation data doesn't match any pending request.
func TestMqttRequest_OnResponse_WrongCorrelation(t *testing.T) {
	broker := newOfflineBroker(t)
	pendingKey := []byte{0xAA, 0xBB}
	node := &MqttRequestNode{
		config:         flow.NodeConfig{ID: "req-1"},
		broker:         broker,
		responseFormat: "string",
		pending:        make(map[string]*mqttRequestInflight),
		BaseNode: BaseNode{
			Send: func(int, *flow.Message) {
				t.Errorf("send should NOT be called for unknown correlation")
			},
			Status: func(string, string) {},
		},
	}
	node.pending[hex.EncodeToString(pendingKey)] = &mqttRequestInflight{
		correlation:   pendingKey,
		responseTopic: "loopze/response/x",
		inMsg:         flow.NewMessage(),
	}

	pub := &paho.Publish{
		Topic:   "loopze/response/x",
		Payload: []byte("noise"),
		Properties: &paho.PublishProperties{
			CorrelationData: []byte{0xDE, 0xAD}, // does not match pendingKey
		},
	}
	node.onResponse(pub)

	if len(node.pending) != 1 {
		t.Errorf("pending = %d, want 1 (mismatched correlation must not drain pending)", len(node.pending))
	}
}

// TestMqttRequest_OnResponse_NoCorrelationProperty drops publishes that
// lack v5 Properties altogether (defensive — should not happen in practice).
func TestMqttRequest_OnResponse_NoCorrelationProperty(t *testing.T) {
	node := &MqttRequestNode{
		config:  flow.NodeConfig{ID: "req-1"},
		pending: make(map[string]*mqttRequestInflight),
		BaseNode: BaseNode{
			Send: func(int, *flow.Message) {
				t.Errorf("send should NOT fire without correlation")
			},
			Status: func(string, string) {},
		},
	}
	// nil properties
	node.onResponse(&paho.Publish{Topic: "x", Payload: []byte("noise")})
	// empty correlation
	node.onResponse(&paho.Publish{
		Topic:      "x",
		Payload:    []byte("noise"),
		Properties: &paho.PublishProperties{CorrelationData: nil},
	})
}

// TestMqttRequest_OnTimeout_ErrorMode raises a catchable error and drains
// the pending entry.
func TestMqttRequest_OnTimeout_ErrorMode(t *testing.T) {
	broker := newOfflineBroker(t)
	correlation := []byte{0xCA, 0xFE}
	responseTopic := "loopze/response/y"

	var caught error
	var caughtMsg *flow.Message
	node := &MqttRequestNode{
		config:      flow.NodeConfig{ID: "req-1"},
		broker:      broker,
		timeoutMode: mqttRequestTimeoutModeError,
		pending:     make(map[string]*mqttRequestInflight),
		errorFn: func(err error, m *flow.Message) {
			caught = err
			caughtMsg = m
		},
		BaseNode: BaseNode{
			Send: func(int, *flow.Message) {
				t.Errorf("send should NOT fire in error timeout mode")
			},
			Status: func(string, string) {},
		},
	}
	in := flow.NewMessage()
	in.Set("topic", "rpc/foo")
	node.pending[hex.EncodeToString(correlation)] = &mqttRequestInflight{
		correlation:   correlation,
		responseTopic: responseTopic,
		inMsg:         in,
	}
	_ = broker.Subscribe("req-1", responseTopic, SubscribeOptions{}, func(*paho.Publish) {})

	node.onTimeout(hex.EncodeToString(correlation))

	if caught == nil || !strings.Contains(caught.Error(), "timeout") {
		t.Fatalf("expected timeout error, got %v", caught)
	}
	if caughtMsg != in {
		t.Errorf("caught msg = %p, want input msg %p", caughtMsg, in)
	}
	if len(node.pending) != 0 {
		t.Errorf("pending = %d, want 0 after timeout", len(node.pending))
	}
	if _, ok := broker.subscribers["req-1"]; ok {
		t.Errorf("expected no broker subscriptions after timeout cleanup")
	}
}

// TestMqttRequest_OnTimeout_Passthrough emits a message with msg.timedOut=true
// instead of raising a catch error.
func TestMqttRequest_OnTimeout_Passthrough(t *testing.T) {
	broker := newOfflineBroker(t)
	correlation := []byte{0x11}

	var sent []*flow.Message
	node := &MqttRequestNode{
		config:      flow.NodeConfig{ID: "req-1"},
		broker:      broker,
		timeoutMode: mqttRequestTimeoutModePassthrough,
		pending:     make(map[string]*mqttRequestInflight),
		errorFn: func(error, *flow.Message) {
			t.Errorf("errorFn should NOT fire in passthrough mode")
		},
		BaseNode: BaseNode{
			Send: func(_ int, m *flow.Message) {
				sent = append(sent, m)
			},
			Status: func(string, string) {},
		},
	}
	in := flow.NewMessage()
	in.Set("topic", "rpc/foo")
	in.Set("payload", "preserved")
	node.pending[hex.EncodeToString(correlation)] = &mqttRequestInflight{
		correlation:   correlation,
		responseTopic: "loopze/response/z",
		inMsg:         in,
	}

	node.onTimeout(hex.EncodeToString(correlation))

	if len(sent) != 1 {
		t.Fatalf("sent = %d, want 1 (passthrough)", len(sent))
	}
	out := sent[0]
	if v, _ := out.Get("timedOut").(bool); !v {
		t.Errorf("out.timedOut = %v, want true", out.Get("timedOut"))
	}
	if out.Payload() != "preserved" {
		t.Errorf("payload = %v, want preserved", out.Payload())
	}
}

// TestMqttRequest_FailAllInflight_ErrorMode drains every pending entry on
// broker disconnect and raises a catch error per entry.
func TestMqttRequest_FailAllInflight_ErrorMode(t *testing.T) {
	broker := newOfflineBroker(t)
	var errCount int
	node := &MqttRequestNode{
		config:      flow.NodeConfig{ID: "req-1"},
		broker:      broker,
		timeoutMode: mqttRequestTimeoutModeError,
		pending:     make(map[string]*mqttRequestInflight),
		errorFn: func(err error, _ *flow.Message) {
			if !strings.Contains(err.Error(), "connection lost") {
				t.Errorf("err = %v, want connection lost", err)
			}
			errCount++
		},
		BaseNode: BaseNode{
			Send:   func(int, *flow.Message) {},
			Status: func(string, string) {},
		},
	}
	for i, c := range [][]byte{{0x01}, {0x02}, {0x03}} {
		_ = i
		topic := "loopze/response/" + hex.EncodeToString(c)
		_ = broker.Subscribe("req-1", topic, SubscribeOptions{}, func(*paho.Publish) {})
		node.pending[hex.EncodeToString(c)] = &mqttRequestInflight{
			correlation:   c,
			responseTopic: topic,
			inMsg:         flow.NewMessage(),
		}
	}

	node.failAllInflight()

	if errCount != 3 {
		t.Errorf("errorFn fired %d times, want 3", errCount)
	}
	if len(node.pending) != 0 {
		t.Errorf("pending = %d, want 0 after disconnect cleanup", len(node.pending))
	}
	if _, ok := broker.subscribers["req-1"]; ok {
		t.Errorf("expected all broker subscriptions removed after failAllInflight")
	}
}

// TestMqttRequest_OnTimeout_AlreadyDone exercises the race where a response
// arrived between the timer firing and onTimeout's lookup — onTimeout must
// be a no-op (no callbacks fire) because the entry is already gone.
func TestMqttRequest_OnTimeout_AlreadyDone(t *testing.T) {
	node := &MqttRequestNode{
		config:      flow.NodeConfig{ID: "req-1"},
		timeoutMode: mqttRequestTimeoutModeError,
		pending:     make(map[string]*mqttRequestInflight),
		errorFn: func(error, *flow.Message) {
			t.Errorf("errorFn should NOT fire when entry is already drained")
		},
		BaseNode: BaseNode{
			Send:   func(int, *flow.Message) {},
			Status: func(string, string) {},
		},
	}
	// no entry → onTimeout must short-circuit
	node.onTimeout("nonexistent")
}

// TestMqttRequest_RandomCorrelationData verifies the helpers produce 16
// bytes of correlation data and a non-empty hex suffix. Cheap sanity check
// to guard against regressions if the byte source changes.
func TestMqttRequest_RandomCorrelationData(t *testing.T) {
	cd, err := randomCorrelationData()
	if err != nil {
		t.Fatalf("randomCorrelationData: %v", err)
	}
	if len(cd) != 16 {
		t.Errorf("len(cd) = %d, want 16", len(cd))
	}
	suf := randomResponseSuffix()
	if len(suf) != 32 {
		t.Errorf("len(suffix) = %d, want 32 (16 bytes hex)", len(suf))
	}
	// Two consecutive calls should not collide (extremely unlikely).
	if suf == randomResponseSuffix() {
		t.Errorf("randomResponseSuffix produced the same suffix twice")
	}
}

// TestMqttRequest_Init_TimeoutMode rejects unknown timeout modes and accepts
// the documented values.
func TestMqttRequest_Init_TimeoutMode(t *testing.T) {
	cases := []struct {
		mode    any
		wantErr bool
		want    string
	}{
		{mode: "", wantErr: false, want: "error"},
		{mode: nil, wantErr: false, want: "error"},
		{mode: "error", wantErr: false, want: "error"},
		{mode: "passthrough", wantErr: false, want: "passthrough"},
		{mode: "explode", wantErr: true},
	}
	for _, tc := range cases {
		n := &MqttRequestNode{
			config: flow.NodeConfig{
				ID: "n1",
				Properties: map[string]any{
					"broker":      "broker-1",
					"topic":       "rpc/x",
					"timeoutMode": tc.mode,
				},
			},
			pending: make(map[string]*mqttRequestInflight),
		}
		err := n.Init()
		if tc.wantErr {
			if err == nil {
				t.Errorf("mode=%v: expected error", tc.mode)
			}
			continue
		}
		if err != nil {
			t.Errorf("mode=%v: unexpected error %v", tc.mode, err)
			continue
		}
		if n.timeoutMode != tc.want {
			t.Errorf("mode=%v: timeoutMode = %q, want %q", tc.mode, n.timeoutMode, tc.want)
		}
	}
}

// TestMqttRequest_Init_PrefixNormalization strips a trailing slash from the
// configured prefix so the runtime never builds "loopze/response//<uuid>".
func TestMqttRequest_Init_PrefixNormalization(t *testing.T) {
	n := &MqttRequestNode{
		config: flow.NodeConfig{
			ID: "n1",
			Properties: map[string]any{
				"broker":              "broker-1",
				"responseTopicPrefix": "loopze/response/",
			},
		},
		pending: make(map[string]*mqttRequestInflight),
	}
	if err := n.Init(); err != nil {
		t.Fatal(err)
	}
	if n.prefix != "loopze/response" {
		t.Errorf("prefix = %q, want %q (trailing slash should be trimmed)", n.prefix, "loopze/response")
	}
}

// TestMqttRequest_Init_DefaultTimeout uses the documented default of 30s
// when no timeout is configured.
func TestMqttRequest_Init_DefaultTimeout(t *testing.T) {
	n := &MqttRequestNode{
		config: flow.NodeConfig{
			ID: "n1",
			Properties: map[string]any{"broker": "broker-1"},
		},
		pending: make(map[string]*mqttRequestInflight),
	}
	if err := n.Init(); err != nil {
		t.Fatal(err)
	}
	if n.timeout != 30*time.Second {
		t.Errorf("timeout = %v, want 30s", n.timeout)
	}
}
