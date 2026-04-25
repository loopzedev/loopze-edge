<script setup lang="ts">
interface TriggerInfo {
  name: string
  desc: string
}

interface ValueTypeInfo {
  key: string
  label: string
  desc: string
}

interface Example {
  title: string
  config: string
  result: string
}

const triggers: TriggerInfo[] = [
  {
    name: 'Once at startup',
    desc: 'Sends a single message right after the flow starts. Useful for seeding state, initial fetches or boot-time triggers.',
  },
  {
    name: 'Repeat interval',
    desc: 'Periodically emits a message at the configured interval (in ms). Set to 0 to disable. Combine with "once" to also fire immediately on start.',
  },
  {
    name: 'Manual trigger',
    desc: 'A single message can be triggered on demand via the API or by an upstream link. The incoming message is ignored — a new one is generated.',
  },
]

const valueTypes: ValueTypeInfo[] = [
  { key: 'string', label: 'string', desc: 'Plain text value.' },
  { key: 'number', label: 'number', desc: 'Numeric value, parsed as float.' },
  { key: 'boolean', label: 'boolean', desc: 'true / false literal.' },
  { key: 'JSON', label: 'JSON', desc: 'Parsed JSON object or array.' },
  { key: 'timestamp', label: 'timestamp', desc: 'Current time as epoch ms or RFC3339 string.' },
  { key: 'env', label: 'env', desc: 'Reads value from the named environment variable.' },
  { key: 'flow', label: 'flow.', desc: 'Reads from flow context (memory or persistent).' },
  { key: 'global', label: 'global.', desc: 'Reads from global context (memory or persistent).' },
]

const examples: Example[] = [
  {
    title: 'Heartbeat every second',
    config: 'interval: 1000  •  msg.payload = timestamp',
    result: 'Emits { payload: <epoch> } once per second.',
  },
  {
    title: 'Boot-time config load',
    config: 'once: true  •  msg.payload = global.appConfig',
    result: 'On flow start, reads appConfig from global context and emits it.',
  },
  {
    title: 'Static topic + dynamic payload',
    config: 'msg.topic = "sensors/in"  •  msg.payload = flow.lastReading',
    result: 'Emits a message with a fixed topic and the latest reading.',
  },
]
</script>

<template>
  <div class="flex flex-col">
    <!-- Overview -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Overview
      </div>
      <p class="text-[11px] text-terminal-text leading-relaxed m-0">
        A source node that produces messages without any input. Use it to start
        flows on a schedule, seed state at boot, or fire test messages on demand.
      </p>
    </div>

    <!-- Triggers -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Triggers
      </div>
      <div class="flex flex-col gap-2">
        <div v-for="t in triggers" :key="t.name">
          <div class="text-[11px] font-semibold text-terminal-text">{{ t.name }}</div>
          <div class="text-[11px] text-terminal-text-dim leading-relaxed">{{ t.desc }}</div>
        </div>
      </div>
      <div class="mt-2 px-2 py-1.5 border border-terminal-border bg-terminal-bg">
        <div class="text-[10px] text-terminal-text-dim leading-relaxed">
          <span class="text-accent font-semibold">Note:</span>
          if neither "once" nor an interval is set, the node only fires when triggered manually.
        </div>
      </div>
    </div>

    <!-- Properties (Inject rules) -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Properties
      </div>
      <p class="text-[11px] text-terminal-text leading-relaxed m-0 mb-2">
        Each row defines one field on the outgoing <span class="font-mono text-accent">msg</span>
        object. Multiple rows are applied in order.
      </p>
      <div class="flex flex-col gap-1.5">
        <div class="flex items-start gap-2">
          <span class="text-[11px] font-mono text-accent shrink-0 w-14">msg.&lt;p&gt;</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">
            Target property on the message (e.g. <span class="font-mono">payload</span>,
            <span class="font-mono">topic</span>, dotted paths supported).
          </span>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-[11px] font-mono text-accent shrink-0 w-14">type</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">
            How the value is interpreted — see Value Types below.
          </span>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-[11px] font-mono text-accent shrink-0 w-14">value</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">
            Raw value or context key, depending on the chosen type.
          </span>
        </div>
        <div class="flex items-start gap-2">
          <span class="text-[11px] font-mono text-accent shrink-0 w-14">storage</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">
            For <span class="font-mono">flow.</span> / <span class="font-mono">global.</span> only:
            read from <span class="font-mono">memory</span> or <span class="font-mono">persistent</span> store.
          </span>
        </div>
      </div>
    </div>

    <!-- Value types -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Value Types
      </div>
      <div class="flex flex-col gap-1">
        <div
          v-for="vt in valueTypes"
          :key="vt.key"
          class="flex items-start gap-2 py-0.5"
        >
          <span class="text-[11px] font-mono text-accent shrink-0 w-16">{{ vt.label }}</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">{{ vt.desc }}</span>
        </div>
      </div>
    </div>

    <!-- Examples -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Examples
      </div>
      <div class="flex flex-col gap-2">
        <div
          v-for="ex in examples"
          :key="ex.title"
          class="border border-terminal-border bg-terminal-bg p-2"
        >
          <div class="text-[11px] font-semibold text-terminal-text mb-0.5">{{ ex.title }}</div>
          <div class="text-[10px] font-mono text-accent leading-relaxed">{{ ex.config }}</div>
          <div class="text-[10px] text-terminal-text-dim leading-relaxed mt-0.5">{{ ex.result }}</div>
        </div>
      </div>
    </div>

    <!-- Tips -->
    <div class="px-4 py-3">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Tips
      </div>
      <ul class="m-0 pl-4 flex flex-col gap-1 text-[11px] text-terminal-text-dim leading-relaxed">
        <li>Drag the handle on a property row to reorder how fields are applied.</li>
        <li>Use the timestamp type with <span class="font-mono text-accent">rfc3339</span> for human-readable logs, <span class="font-mono text-accent">epoch</span> for math.</li>
        <li>Pulling secrets at boot? Combine <span class="font-mono text-accent">once: true</span> with an <span class="font-mono text-accent">env</span> value.</li>
        <li>Very short intervals (&lt; 100&nbsp;ms) can flood downstream nodes — verify they keep up.</li>
      </ul>
    </div>
  </div>
</template>
