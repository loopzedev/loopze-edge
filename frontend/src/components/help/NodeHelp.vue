<script setup lang="ts">
import type { NodeHelpDoc } from './types'

defineProps<{ doc: NodeHelpDoc }>()
</script>

<template>
  <div class="flex flex-col">
    <!-- Overview -->
    <div class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Overview
      </div>
      <p class="text-[11px] text-terminal-text leading-relaxed m-0 whitespace-pre-line">
        {{ doc.overview }}
      </p>
    </div>

    <!-- Inputs -->
    <div v-if="doc.inputs && doc.inputs.length > 0" class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Inputs
      </div>
      <ol class="m-0 pl-4 flex flex-col gap-1 text-[11px] text-terminal-text-dim leading-relaxed">
        <li v-for="(line, idx) in doc.inputs" :key="idx">{{ line }}</li>
      </ol>
    </div>

    <!-- Outputs -->
    <div v-if="doc.outputs && doc.outputs.length > 0" class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Outputs
      </div>
      <ol class="m-0 pl-4 flex flex-col gap-1 text-[11px] text-terminal-text-dim leading-relaxed">
        <li v-for="(line, idx) in doc.outputs" :key="idx">{{ line }}</li>
      </ol>
    </div>

    <!-- Properties -->
    <div v-if="doc.properties && doc.properties.length > 0" class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Properties
      </div>
      <div class="flex flex-col gap-1.5">
        <div
          v-for="p in doc.properties"
          :key="p.key"
          class="flex items-start gap-2"
        >
          <span class="text-[11px] font-mono text-accent shrink-0 w-20">{{ p.key }}</span>
          <span class="text-[11px] text-terminal-text-dim leading-relaxed">{{ p.desc }}</span>
        </div>
      </div>
    </div>

    <!-- Examples -->
    <div v-if="doc.examples && doc.examples.length > 0" class="px-4 py-3 border-b border-terminal-border">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Examples
      </div>
      <div class="flex flex-col gap-2">
        <div
          v-for="ex in doc.examples"
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
    <div v-if="doc.tips && doc.tips.length > 0" class="px-4 py-3">
      <div class="text-[10px] font-semibold uppercase tracking-wider text-terminal-text-dim mb-1.5">
        Tips
      </div>
      <ul class="m-0 pl-4 flex flex-col gap-1 text-[11px] text-terminal-text-dim leading-relaxed">
        <li v-for="(tip, idx) in doc.tips" :key="idx">{{ tip }}</li>
      </ul>
    </div>
  </div>
</template>
