// MQTT-specific option lists. Keep in sync with internal/nodes/mqtt/*.go.

import type { OptionEntry } from '@/components/config/enums'

// QoS levels for MQTT v3.1/v3.1.1/v5.
export const QOS_LEVELS: OptionEntry<number>[] = [
  { value: 0, label: '0 — at most once' },
  { value: 1, label: '1 — at least once' },
  { value: 2, label: '2 — exactly once' },
]

// MQTT v5 Retain Handling (subscribe option).
export const RETAIN_HANDLING_OPTIONS: OptionEntry<number>[] = [
  { value: 0, label: '0 — send retained at every subscribe' },
  { value: 1, label: '1 — send retained only on new subscription' },
  { value: 2, label: '2 — never send retained' },
]

// MQTT v5 Payload Format Indicator (publish property).
export const PAYLOAD_FORMAT_OPTIONS: OptionEntry<number>[] = [
  { value: 0, label: '0 — bytes / unspecified' },
  { value: 1, label: '1 — UTF-8 text' },
]

// mqtt-in output payload format.
export const MQTT_IN_OUTPUT_FORMATS: OptionEntry<string>[] = [
  { value: 'string', label: 'String' },
  { value: 'json',   label: 'JSON (parsed)' },
  { value: 'buffer', label: 'Buffer (raw bytes)' },
]

// mqtt-out publish target.
export const MQTT_OUT_TARGETS: OptionEntry<string>[] = [
  { value: 'topic',         label: 'Topic' },
  { value: 'responseTopic', label: 'Response to responseTopic' },
]

// mqtt-request timeout mode.
export const MQTT_REQUEST_TIMEOUT_MODES: OptionEntry<string>[] = [
  { value: 'error',       label: 'Error (catchable)' },
  { value: 'passthrough', label: 'Passthrough (msg.timedOut=true)' },
]
