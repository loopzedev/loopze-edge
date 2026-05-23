<script setup lang="ts">
import { computed } from 'vue'
import { useFlowStore } from '@/stores/flowStore'
import { useUiStore } from '@/stores/uiStore'

const flowStore = useFlowStore()
const ui = useUiStore()

defineProps<{
  /** When true the banner appears as an informational hint that
   *  pending changes haven't reached the live dashboard yet. It is
   *  NOT a lock — the layout view stays fully editable so multiple
   *  edits can be batched before a single deploy. */
  visible: boolean
}>()

const isDeploying = computed(() => ui.deployStatus === 'deploying')

async function handleDeploy() {
  await flowStore.deploy()
}
</script>

<template>
  <div v-if="visible" class="layout-banner">
    <span class="dot" />
    <div class="msg">
      Pending changes — the live dashboard still shows the last deployed
      layout. Batch edits and click <strong>Deploy</strong> when ready.
    </div>
    <button
      class="deploy-btn"
      type="button"
      :disabled="isDeploying"
      @click="handleDeploy"
    >
      {{ isDeploying ? 'Deploying…' : 'Deploy now' }}
    </button>
  </div>
</template>

<style scoped>
.layout-banner {
  display: flex;
  align-items: center;
  gap: 0.85rem;
  padding: 0.55rem 0.9rem;
  margin-bottom: 1rem;
  background: rgba(210, 153, 34, 0.12);
  border: 1px solid rgba(210, 153, 34, 0.45);
  border-radius: 6px;
  color: #d29922;
  font-size: 0.85rem;
}
.dot {
  width: 0.55rem;
  height: 0.55rem;
  border-radius: 50%;
  background: currentColor;
  flex-shrink: 0;
}
.msg {
  flex: 1 1 auto;
}
.deploy-btn {
  background: #d29922;
  color: #0d1117;
  border: none;
  padding: 0.35rem 0.85rem;
  font-size: 0.75rem;
  font-weight: 600;
  letter-spacing: 0.04em;
  text-transform: uppercase;
  border-radius: 3px;
  cursor: pointer;
}
.deploy-btn:hover {
  filter: brightness(1.1);
}
</style>
