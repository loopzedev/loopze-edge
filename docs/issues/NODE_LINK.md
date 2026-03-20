# Issue: Link Nodes – Cross-Flow Messaging (Input, Output, Request)

## Status: Open

## Problembeschreibung

Mit der Einführung mehrerer Flows pro Workspace fehlt die Möglichkeit, **Nachrichten über Flow-Grenzen hinweg** zu verschicken. Aktuell können Nodes nur innerhalb eines Flows miteinander verdrahtet werden. Für modulare Flow-Architekturen werden drei neue Node-Typen benötigt, die als virtuelle Brücken zwischen Flows fungieren.

## Übersicht der drei Node-Typen

| Node-Typ | Typ-ID | Canvas Inputs | Canvas Outputs | Beschreibung |
|---|---|---|---|---|
| **Link Input** | `link-in` | 0 | 1 | Empfängt Nachrichten von Link Output Nodes aus anderen Flows |
| **Link Output** | `link-out` | 1 | 0 | Sendet Nachrichten an Link Input Nodes in anderen Flows |
| **Link Request** | `link-call` | 1 | 1 | Sendet eine Anfrage an einen Link Input Node und wartet auf die Antwort |

```
Flow A                              Flow B
┌─────────────────────┐             ┌─────────────────────┐
│                      │             │                      │
│  [Inject] → [Link Output] ──────→ [Link Input] → [Debug] │
│                      │             │                      │
└─────────────────────┘             └─────────────────────┘

Flow C (Request/Response)           Flow D (Service)
┌──────────────────────────┐        ┌──────────────────────────┐
│                           │        │                           │
│  [Inject] → [Link Request] ─────→ [Link Input]               │
│              ↑ (Response)  │        │      ↓                   │
│              │             │        │  [Function]              │
│              └─────────────│────── [Link Output] ← ┘          │
│          [Debug] ←─┘       │        │                           │
└──────────────────────────┘        └──────────────────────────┘
```

## Anforderungen

### 1. Link Output Node (`link-out`)

- **Canvas**: 1 Input, 0 Outputs
- **Funktion**: Nimmt Nachrichten entgegen und leitet sie an konfigurierte Link Input Nodes weiter
- **Properties** (Doppelklick): Tabellarische Ansicht aller verfügbaren **Link Input** Nodes im gesamten Workspace
  - Spalten: Checkbox, Flow-Name, Node-Name
  - **Mehrfachauswahl** möglich — ein Link Output kann an mehrere Link Inputs senden
  - Nur Link Input Nodes werden angezeigt (keine anderen Node-Typen)
- **Konfiguration**:
  ```json
  {
    "links": ["link-in-node-id-1", "link-in-node-id-2"]
  }
  ```

### 2. Link Input Node (`link-in`)

- **Canvas**: 0 Inputs, 1 Output
- **Funktion**: Empfängt Nachrichten von Link Output Nodes und leitet sie an den verbundenen Output-Port weiter
- **Properties** (Doppelklick): Tabellarische Ansicht aller verfügbaren **Link Output** Nodes im gesamten Workspace
  - Spalten: Checkbox, Flow-Name, Node-Name
  - **Mehrfachauswahl** möglich — ein Link Input kann von mehreren Link Outputs empfangen
  - Nur Link Output Nodes werden angezeigt (keine anderen Node-Typen)
- **Konfiguration**:
  ```json
  {
    "links": ["link-out-node-id-1", "link-out-node-id-2"]
  }
  ```
- **Hinweis**: Die Link-Konfiguration ist **bidirektional gespiegelt** — wenn ein Link Output den Link Input `A` auswählt, erscheint der Link Output automatisch in der Properties-Tabelle von `A` als ausgewählt (und umgekehrt). Die Zuordnung wird von beiden Seiten aus gepflegt.

### 3. Link Request Node (`link-call`)

- **Canvas**: 1 Input, 1 Output
- **Funktion**: Sendet eine Nachricht an einen Link Input Node, wartet auf die Antwort des zugehörigen Link Output Nodes, und gibt die Antwort am eigenen Output-Port aus
- **Properties** (Doppelklick): Tabellarische Ansicht aller verfügbaren **Link Input** Nodes
  - Spalten: Radio-Button, Flow-Name, Node-Name
  - **Nur eine Auswahl** möglich (Radio statt Checkbox)
  - Nur Link Input Nodes werden angezeigt
- **Konfiguration**:
  ```json
  {
    "linkTarget": "link-in-node-id"
  }
  ```

### 4. Request/Response Mechanismus

Der Link Request Node implementiert ein **synchrones Request/Response-Pattern** über Flow-Grenzen hinweg:

1. **Request**: Der Link Request Node sendet die Nachricht an den konfigurierten Link Input Node. Dabei wird die **Requestor-ID** (Node-ID des Link Request Nodes) in die Nachricht eingebettet
2. **Verarbeitung**: Der Ziel-Flow verarbeitet die Nachricht normal über seine Nodes
3. **Response**: Wenn die verarbeitete Nachricht einen Link Output Node erreicht, prüft dieser ob eine Requestor-ID vorhanden ist:
   - **Ja**: Die Nachricht wird direkt an den aufrufenden Link Request Node zurückgeschickt (nicht an die regulären Link-Ziele)
   - **Nein**: Normale Weiterleitung an die konfigurierten Link Input Nodes
4. **Ausgabe**: Der Link Request Node empfängt die Antwort und gibt sie an seinem Output-Port aus

**Nachrichtenstruktur für Requestor-Tracking**:
```go
// Im Message-Objekt wird ein internes Feld mitgeführt:
msg.Set("_linkSource", requestNodeID)  // Vom Link Request gesetzt
msg.Get("_linkSource")                 // Vom Link Output gelesen
```

Das `_linkSource`-Feld wird nach der Zustellung vom Link Request Node wieder entfernt, damit es nicht in nachfolgende Nodes leakt.

### 5. Properties – Tabellarische Link-Ansicht

Die Properties-Tabelle ist für alle drei Node-Typen identisch aufgebaut (nur Selektionstyp und angezeigte Nodes unterscheiden sich):

```
┌──────────────────────────────────────────────┐
│  Link Output                                  │
├──────────────────────────────────────────────┤
│  Name                                         │
│  ┌────────────────────────────────────────┐   │
│  │ my-link-out                            │   │
│  └────────────────────────────────────────┘   │
│                                               │
│  Verfügbare Link Inputs                       │
│  ┌────┬──────────────┬────────────────────┐   │
│  │ ✓  │ Flow         │ Node               │   │
│  ├────┼──────────────┼────────────────────┤   │
│  │ [x]│ Flow 1       │ api-input          │   │
│  │ [ ]│ Flow 2       │ data-receiver      │   │
│  │ [x]│ Flow 3       │ event-handler      │   │
│  └────┴──────────────┴────────────────────┘   │
└──────────────────────────────────────────────┘
```

Für den Link Request Node wird statt Checkboxen ein **Radio-Button** verwendet (nur Einfachauswahl).

**Ermittlung der verfügbaren Link-Nodes**:
- Die Tabelle durchsucht **alle Flows** im `flowStore.flows` nach Nodes mit dem passenden Typ
- Für Link Output → zeigt alle `link-in` Nodes
- Für Link Input → zeigt alle `link-out` Nodes
- Für Link Request → zeigt alle `link-in` Nodes
- Der eigene Flow wird einbezogen (Links innerhalb des gleichen Flows sind erlaubt)

## Datenstruktur

### workspace.json

```json
[
  {
    "id": "flow-1",
    "type": "tab",
    "label": "Flow 1",
    "nodes": [
      {
        "id": "node-link-out-1",
        "type": "link-out",
        "name": "to-processor",
        "x": 400,
        "y": 200,
        "z": "flow-1",
        "inputs": 1,
        "outputs": 0,
        "wires": [],
        "config": {
          "links": ["node-link-in-1", "node-link-in-2"]
        }
      }
    ]
  },
  {
    "id": "flow-2",
    "type": "tab",
    "label": "Flow 2",
    "nodes": [
      {
        "id": "node-link-in-1",
        "type": "link-in",
        "name": "from-sender",
        "x": 100,
        "y": 200,
        "z": "flow-2",
        "inputs": 0,
        "outputs": 1,
        "wires": [["node-debug-1"]],
        "config": {
          "links": ["node-link-out-1"]
        }
      }
    ]
  }
]
```

## Betroffene Dateien

### Backend – Neue Dateien

- `internal/nodes/link_in.go` — Link Input Node: Registriert sich bei der Engine, empfängt Nachrichten und sendet sie an den Canvas-Output
- `internal/nodes/link_out.go` — Link Output Node: Leitet Nachrichten an konfigurierte Link Input Nodes weiter, prüft `_linkSource` für Request/Response
- `internal/nodes/link_call.go` — Link Request Node: Sendet mit `_linkSource`, empfängt Response und gibt sie am Output aus

### Backend – Anpassungen

- `internal/server/server.go` — Registrierung der drei neuen Node-Typen im `registerNodes()`
- `internal/flow/engine.go` — Cross-Flow Routing: Die Engine muss Link Output Nodes ermöglichen, Nachrichten direkt an Nodes in anderen Flows zu senden. Dazu benötigt die Engine eine **Link-Registry** die beim Deploy aufgebaut wird:
  ```go
  type linkRegistry struct {
      inputs  map[string]*runningNode   // nodeID → running link-in node
      outputs map[string]*runningNode   // nodeID → running link-out node
  }
  ```
  Die Link Nodes erhalten über einen neuen Callback (`SetLinkSend`) Zugriff auf diese Registry, um Nachrichten direkt an andere Nodes zu senden — unabhängig von der normalen Wire-Verdrahtung.

### Frontend – Neue Dateien

- `frontend/src/components/config/LinkConfig.vue` — Gemeinsame Config-Komponente für alle drei Link-Node-Typen mit tabellarischer Ansicht. Kapselt die Logik zur Ermittlung verfügbarer Link-Nodes aus allen Flows und unterscheidet per Prop zwischen Checkbox (Multi) und Radio (Single) Selektion.

### Frontend – Anpassungen

- `frontend/src/components/PropertyPanel.vue` — Dispatch für `link-in`, `link-out` und `link-call` auf die neue `LinkConfig` Komponente
- `frontend/src/components/nodes/tokens.ts` — Bereits vorhanden: `link-in` → input-Kategorie (grün), `link-out` → output-Kategorie (orange). Ergänzen: `link-call` → process-Kategorie
- `frontend/src/types/flow.ts` — `link-call` zum `NodeType` Union hinzufügen (link-in und link-out sind bereits definiert)

## Technische Hinweise

### Cross-Flow Routing in der Engine

Die normale Wire-Verdrahtung (`makeSendFunc`) funktioniert nur innerhalb eines Flows. Für Link Nodes wird ein paralleler Routing-Pfad benötigt:

1. **Beim Deploy**: Engine iteriert über alle Nodes, identifiziert Link Nodes und baut die `linkRegistry` auf
2. **Link Output → Link Input**: Der Link Output Node liest `config.links[]`, holt die entsprechenden `runningNode`-Referenzen aus der Registry, und sendet die Nachricht direkt in deren `inputCh`
3. **Link Request → Link Input → Link Output → Link Request**: Der Request Node setzt `_linkSource`, der Output Node liest es und routet die Antwort zurück

### Bidirektionale Link-Spiegelung (Frontend)

Wenn der Bediener in einem Link Output Node den Link Input `A` auswählt, muss auch der Link Input `A` den Link Output in seiner `config.links[]` Liste führen. Dies wird im Frontend beim Speichern synchronisiert:

```typescript
function toggleLink(targetNodeId: string, selected: boolean) {
  // Update own config
  updateOwnLinks(targetNodeId, selected)
  // Mirror: update target node's config
  updateTargetLinks(ownNodeId, selected)
}
```

Beide Nodes werden als dirty markiert.

### Engine – Link-Nodes brauchen Zugriff auf andere Nodes

Die bestehende `NodeInstance`-Schnittstelle bietet keinen Mechanismus für Cross-Node-Kommunikation. Optionen:

**Option A: Neuer Callback `SetLinkSend`**
```go
type LinkSendFunc func(targetNodeID string, msg *Message)

type LinkProvider interface {
    SetLinkSend(fn LinkSendFunc)
}
```
Die Engine prüft beim Deploy ob ein Node `LinkProvider` implementiert und setzt den Callback. Analog zu `ContextProvider`.

**Option B: Engine-Level Routing**
Die Engine übernimmt das Routing komplett: Nach `HandleMessage` prüft sie ob der Node ein Link Node ist und routet entsprechend. Die Link Nodes selbst sind dann "dumm" und geben die Nachricht einfach zurück.

**Empfehlung: Option A** — konsistent mit dem bestehenden Provider-Pattern (`ContextProvider`), hält die Routing-Logik in den Nodes.

## Abhängigkeiten

- **Flow-Management (FLOWS.md)**: Muss implementiert sein, damit mehrere Flows existieren und Link Nodes sinnvoll eingesetzt werden können
- Die Node-Typen `link-in` und `link-out` sind im Frontend bereits als Typen und in den Styling-Tokens definiert — sie erscheinen automatisch in der Palette sobald das Backend sie registriert
