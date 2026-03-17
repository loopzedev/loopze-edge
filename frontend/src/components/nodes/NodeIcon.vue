<script setup lang="ts">
defineProps<{ type: string }>()

// Each icon is a set of SVG path commands for a 24x24 viewBox.
// Using stroke-based line art for consistency.
const icons: Record<string, string[]> = {
  // ── Core ──────────────────────────────────
  inject:        ['M6 4l12 8-12 8V4z'],                                               // play triangle
  debug:         ['M12 2C6.48 2 2 6.48 2 12s4.48 10 10 10 10-4.48 10-10S17.52 2 12 2z', 'M12 16v.01', 'M12 8v4'], // info circle
  function:      ['M7 4v16', 'M7 4c0 0 0-2 3-2s3 2 3 2', 'M7 12h6', 'M17 20V4', 'M17 20c0 0 0 2-3 2s-3-2-3-2'], // curly braces stylized

  // ── Logic ─────────────────────────────────
  change:        ['M5 12h14', 'M15 8l4 4-4 4', 'M19 12H5', 'M9 16l-4-4 4-4'],        // bidirectional arrows
  switch:        ['M4 12h6', 'M10 12l6-6h4', 'M10 12l6 6h4'],                         // fork/branch
  template:      ['M4 4h16v16H4z', 'M8 8h8', 'M8 12h8', 'M8 16h4'],                  // document with lines
  delay:         ['M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z', 'M12 6v6l4 2'],         // clock
  filter:        ['M3 4h18l-7 8v6l-4 2V12L3 4z'],                                     // funnel

  // ── Network ───────────────────────────────
  'http-in':     ['M3 12h12', 'M11 8l4 4-4 4', 'M19 4v16'],                           // arrow into wall
  'http-response':['M21 12H9', 'M13 8l-4 4 4 4', 'M5 4v16'],                          // arrow from wall
  'http-request':['M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z', 'M2 12h20', 'M12 2a15 15 0 0 1 0 20', 'M12 2a15 15 0 0 0 0 20'], // globe

  // ── Messaging ─────────────────────────────
  'mqtt-in':     ['M4 4h16', 'M12 4v16', 'M8 14l4 4 4-4'],                            // down into line
  'mqtt-out':    ['M4 20h16', 'M12 20V4', 'M8 10l4-4 4 4'],                           // up from line

  // ── Transport ─────────────────────────────
  'tcp-in':      ['M5 4v16', 'M5 12h10a4 4 0 0 0 0-8H9', 'M11 8l-4 4 4 4'],          // plug in
  'tcp-out':     ['M19 4v16', 'M19 12H9a4 4 0 0 1 0-8h6', 'M13 8l4 4-4 4'],          // plug out
  'udp-in':      ['M5 4v16', 'M5 12h10a4 4 0 0 0 0-8H9', 'M11 8l-4 4 4 4'],
  'udp-out':     ['M19 4v16', 'M19 12H9a4 4 0 0 1 0-8h6', 'M13 8l4 4-4 4'],

  // ── Industrial ────────────────────────────
  'modbus-read': ['M4 4h6v6H4z', 'M4 14h6v6H4z', 'M14 7h7', 'M14 17h7', 'M10 7h4', 'M10 17h4'], // registers
  'modbus-write':['M14 4h6v6h-6z', 'M14 14h6v6h-6z', 'M3 7h7', 'M3 17h7', 'M10 7h4', 'M10 17h4'],
  'opc-ua':      ['M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20z', 'M14.5 9a2.5 2.5 0 1 0-5 0 2.5 2.5 0 0 0 5 0z', 'M12 11.5V15', 'M9 15h6', 'M10 15v3', 'M14 15v3'], // industrial symbol

  // ── File ──────────────────────────────────
  'file-in':     ['M14 2H6v20h12V6l-4-4z', 'M14 2v4h4', 'M12 12v6', 'M9 15l3 3 3-3'], // file + down arrow
  'file-out':    ['M14 2H6v20h12V6l-4-4z', 'M14 2v4h4', 'M12 18v-6', 'M9 15l3-3 3 3'], // file + up arrow

  // ── Parser ────────────────────────────────
  json:          ['M7 4c-2 0-3 1-3 3v3c0 1-1 2-2 2 1 0 2 1 2 2v3c0 2 1 3 3 3', 'M17 4c2 0 3 1 3 3v3c0 1 1 2 2 2-1 0-2 1-2 2v3c0 2-1 3-3 3'], // { }
  xml:           ['M8 18l-6-6 6-6', 'M16 6l6 6-6 6', 'M14 4l-4 16'],                  // </>
  csv:           ['M4 4h16v16H4z', 'M4 10h16', 'M4 16h16', 'M10 4v16', 'M16 4v16'],  // grid/table

  // ── Context ──────────────────────────────
  'context-watch': ['M1 12s4-8 11-8 11 8 11 8-4 8-11 8-11-8-11-8z', 'M15 12a3 3 0 1 1-6 0 3 3 0 0 1 6 0z'], // eye

  // ── Utility ───────────────────────────────
  comment:       ['M21 4H3v12h13l5 4V4z'],                                             // speech bubble
  'link-in':     ['M10 13a5 5 0 0 1 0-6h4a5 5 0 0 1 0 6h-4z', 'M3 10h7', 'M6 7l-3 3 3 3'],  // chain + arrow in
  'link-out':    ['M10 13a5 5 0 0 1 0-6h4a5 5 0 0 1 0 6h-4z', 'M14 10h7', 'M18 7l3 3-3 3'],  // chain + arrow out
  catch:         ['M13 2L3 14h8l-1 8 10-12h-8l1-8z'],                                  // lightning bolt
  status:        ['M3 12h3l3-8 4 16 3-8h5'],                                           // heartbeat/pulse
}
</script>

<template>
  <svg
    xmlns="http://www.w3.org/2000/svg"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="2"
    stroke-linecap="round"
    stroke-linejoin="round"
    class="w-5 h-5"
  >
    <path v-for="(d, i) in (icons[type] ?? icons['debug'])" :key="i" :d="d" />
  </svg>
</template>
