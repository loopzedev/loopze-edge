# Properties Panel: Cancel / Revert fuer Node-Aenderungen

## Kontext

Wenn ein Node im Properties Panel bearbeitet wird, gehen alle Aenderungen **sofort direkt in den Pinia Store** (`flowStore.updateNodeData()`). Es gibt keinen Zwischenpuffer, keine Transaktion und kein Undo. Das Panel schliesst ohne Rueckgaengig-Option — einmal geaenderte Werte bleiben bis zum naechsten Deploy oder Page-Reload bestehen.

Der Nutzer braucht einen **Abbrechen-Button**, um Aenderungen an einem Node rueckgaengig zu machen, bevor sie deployed werden.

## Anforderung

### Cancel-Button im Properties Panel

- Wird im Properties Panel angezeigt, wenn der selektierte Node **dirty** ist (`flowStore.isNodeDirty(nodeId)`)
- Klick auf "Cancel" setzt den Node auf den **zuletzt deployed-en Zustand** zurueck
- Nach Cancel: Node wird aus `dirtyNodeIds` entfernt
- Button verschwindet wenn der Node nicht mehr dirty ist

### Snapshot-Mechanismus

Beim **Deploy** wird ein Snapshot der Node-Daten gespeichert. Dieser Snapshot ist die Referenz fuer den Cancel/Revert.

- Neuer State im flowStore: `deployedNodeData: Map<string, Record<string, any>>` — speichert den letzten deployed-en Zustand jedes Nodes
- Wird in `deploy()` nach erfolgreichem Deploy befuellt
- Wird in `loadFlows()` beim initialen Laden befuellt
- `revertNode(nodeId)` stellt den Snapshot wieder her

### Datenfluss

```
Deploy erfolgreich
  → deployedNodeData.set(nodeId, deepCopy(node.data))  // Snapshot speichern

User bearbeitet Node
  → flowStore.updateNodeData()  // wie bisher, aendert Store direkt
  → markNodeDirty(nodeId)       // wie bisher

User klickt "Cancel"
  → flowStore.revertNode(nodeId)
  → node.data = deepCopy(deployedNodeData.get(nodeId))
  → dirtyNodeIds.delete(nodeId)
```

## Technische Details

### Betroffene Dateien

| Datei | Aenderung |
|-------|-----------|
| `frontend/src/stores/flowStore.ts` | `deployedNodeData` Map, `revertNode()`, Snapshot in `deploy()` und `loadFlows()` |
| `frontend/src/components/PropertyPanel.vue` | Cancel-Button (sichtbar wenn Node dirty) |

### flowStore Aenderungen

```typescript
// Neuer State
const deployedNodeData = ref<Map<string, Record<string, any>>>(new Map())

// Nach erfolgreichem Deploy: Snapshot aller Nodes speichern
function snapshotDeployedState() {
  const map = new Map<string, Record<string, any>>()
  for (const node of nodes.value) {
    map.set(node.id, structuredClone(node.data))
  }
  deployedNodeData.value = map
}

// Node auf letzten Deploy-Stand zuruecksetzen
function revertNode(nodeId: string) {
  const snapshot = deployedNodeData.value.get(nodeId)
  if (!snapshot) return
  const node = nodes.value.find(n => n.id === nodeId)
  if (!node) return
  node.data = structuredClone(snapshot)
  dirtyNodeIds.value.delete(nodeId)
}
```

### PropertyPanel Cancel-Button

```html
<button
  v-if="selectedNode && flowStore.isNodeDirty(selectedNode.id)"
  @click="flowStore.revertNode(selectedNode.id)"
>
  Cancel
</button>
```

Platzierung: Im Header-Bereich des Properties Panel, neben dem Node-Namen oder als Footer-Action.

## Abgrenzung

- Kein allgemeines Undo/Redo-System — nur Cancel fuer den aktuell selektierten Node
- Kein Cancel fuer Flow-Properties oder Config-Nodes (kann spaeter ergaenzt werden)
- Cancel bezieht sich immer auf den letzten Deploy-Stand, nicht auf einen vorherigen Edit-Schritt
- Neu hinzugefuegte Nodes (die noch nie deployed wurden) haben keinen Snapshot — Cancel entfernt den Node NICHT, sondern setzt nur auf Defaults zurueck
