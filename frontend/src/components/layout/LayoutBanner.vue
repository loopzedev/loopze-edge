<script setup lang="ts">
import { useFlowStore } from '@/stores/flowStore'

const flowStore = useFlowStore()

defineProps<{
  /** When true, the banner is shown and edit interactions are
   *  expected to be disabled by the parent (the banner does not
   *  enforce; it just narrates). */
  visible: boolean
}>()

async function handleDeploy() {
  await flowStore.deploy()
}
</script>

<template>
  <div v-if="visible" class="layout-banner">
    <span class="dot" />
    <div class="msg">
      Workspace has unsaved changes. Layout edits are disabled until you
      deploy so the live dashboard cannot drift.
    </div>
    <button class="deploy-btn" type="button" @click="handleDeploy">
      Deploy now
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
