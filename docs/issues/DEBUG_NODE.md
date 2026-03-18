# Issue: Debug Node – ON/OFF State Persistierung & Backend-Logik

## Status: Open

## Problembeschreibung

Der Debug Node besitzt ausgangsseitig einen rastenden Toggle-Taster (ON/OFF). Aktuell ist dieser Zustand nur lokal im Vue-Component (`enabled = ref(true)`) gespeichert und geht beim Neuladen des Browsers verloren. Der Zustand muss in `workspace.json` über das `config.active`-Feld des Nodes persistiert werden.

## Anforderungen

### 1. Persistierung in workspace.json

- Der ON/OFF-Zustand wird im Node-Config-Feld `active` (boolean) abgebildet
- Beispiel workspace.json Struktur:
  ```json
  {
    "id": "node-uuid",
    "type": "debug",
    "config": {
      "property": "payload",
      "active": true
    }
  }
  ```
- Default-Wert bei fehlendem Feld: `true` (ON)

### 2. Toggle → Dirty → Deploy Zyklus

- Wird der Toggle-Taster betätigt, ändert sich `config.active` im Frontend-State
- Die Änderung markiert den Node als **dirty** (`flowStore.markNodeDirty(nodeId)`)
- Der blaue Dirty-Indicator wird am Node angezeigt
- Erst mit **Deploy** wird der geänderte Zustand in `workspace.json` geschrieben
- Nach erfolgreichem Deploy wird der Dirty-State zurückgesetzt

### 3. Initialer Zustand beim Laden

- Beim Öffnen von FLINT im Browser wird `config.active` aus den geladenen Flow-Daten gelesen
- Der Toggle-Taster zeigt den gespeicherten Zustand korrekt an (ON/OFF)
- Wurde noch nie deployed, gilt der Default `active: true`

### 4. Frontend-Filterung mit Dirty-State

- Die Filterung passiert **beim Eingang** neuer Nachrichten im Frontend (`addMessage`), nicht retroaktiv auf der bestehenden Liste
- Nachrichten die bereits in der Debug-Liste stehen **bleiben bei Filteränderung erhalten**
- Wird ein Debug Node deaktiviert, werden nur **neue** eingehende Nachrichten dieses Nodes verworfen
- Wird ein Debug Node wieder aktiviert, erscheinen ab sofort wieder neue Nachrichten in der Liste
- Dabei greift der **aktuelle Toggle-State des Frontends**, auch wenn dieser dirty (noch nicht deployed) ist
- Der Bediener kann Debug Nodes sofort aktivieren/deaktivieren ohne vorher deployen zu müssen
- Erst das Persistieren des Zustands erfordert ein Deploy – die Eingangs-Filterung wirkt sofort

### 5. Backend-Logik: Debug Stream

- Der Debug Node streamt **immer** in den DEBUG Stream – unabhängig davon ob `active` true oder false ist
- Das Backend ignoriert den `active`-State bei der Nachrichtenverarbeitung: jede eingehende Message wird über `DebugFunc` in den NATS-Subject `debug.{flowId}.{nodeId}` publiziert
- **Die Filterung (anzeigen/unterdrücken) findet ausschließlich im Frontend statt**, nicht im Backend

### 6. Architektur-Entscheidung: Immer streamen vs. Subscription anpassen

**Zwei Optionen wurden abgewogen:**

| | Option A: Immer streamen, Frontend filtert | Option B: Frontend passt Subscriptions an |
|---|---|---|
| **Prinzip** | Backend publiziert alle Debug-Messages, Frontend blendet deaktivierte Nodes aus | Frontend subscribed/unsubscribed pro Debug Node bei Toggle-Änderung |
| **Latenz beim Toggle** | Sofort – reine UI-Filterung | Verzögerung durch Subscribe/Unsubscribe Roundtrip |
| **Dirty-State Kompatibilität** | Trivial – Frontend kennt den lokalen State und filtert direkt | Komplex – Subscription-Änderung ohne Deploy erfordert separaten Signalweg zum WebSocket-Layer |
| **Nachrichtenverlust** | Keiner – Stream läuft durchgehend | Möglich – Messages während Unsubscribe/Subscribe-Transition gehen verloren |
| **Traffic** | Etwas mehr – auch deaktivierte Nodes senden | Weniger – nur aktive Nodes senden |
| **Komplexität** | Gering – keine Subscription-Verwaltung | Hoch – Subscription-State muss synchron zu Toggle-State gehalten werden |

**Entscheidung: Option A – Immer streamen, Frontend filtert**

Begründung:
- Debug-Nachrichten sind klein und typischerweise low-volume
- Der Toggle muss **sofort** wirken, auch im Dirty-State ohne Deploy – das schließt Subscription-Management praktisch aus
- Keine Race Conditions oder Nachrichtenverlust bei schnellem Hin- und Herschalten
- Deutlich weniger Komplexität im gesamten Stack

### 7. Debug-Ausgabe konfigurieren

Der Bediener kann im Properties-Panel konfigurieren, **welche Informationen** aus der Nachricht in den Debug geschrieben werden und **welcher Inhalt als Node-Status** angezeigt wird.

#### Debug-Ausgabe (`output`)

Dropdown-Feld **"Ausgabe"** mit folgenden Optionen:

| Wert | Label | Beschreibung |
|---|---|---|
| `property` | `msg.` (+ Eingabefeld) | Gibt ein einzelnes Property der Message aus (Default: `payload`) |
| `message` | Kompletten Nachrichten-Objekt | Gibt die gesamte `msg` als JSON aus |
| `gjson` | GJSON | Wertet einen GJSON-Pfadausdruck auf der Message aus |

- Bei `property`: zusätzliches Textfeld für den Property-Pfad (z.B. `payload`, `topic`, `payload.temperature`)
- Bei `gjson`: zusätzliches Textfeld für den GJSON-Ausdruck (z.B. `payload.items.#`, `payload.items.0.name`)
- Default: `property` mit Wert `payload`

#### Node-Status (`statusOutput`)

Checkbox **"Node-Status"** (max. 32 Zeichen) mit zugehörigem Dropdown:

| Wert | Label | Beschreibung |
|---|---|---|
| `same` | Identisch mit Debug-Ausgabe | Zeigt denselben Inhalt wie die Debug-Ausgabe als Status an |
| `property` | `msg.` (+ Eingabefeld) | Zeigt ein spezifisches Property als Status an |
| `gjson` | GJSON | Wertet einen GJSON-Pfadausdruck aus und zeigt das Ergebnis als Status |
| `count` | message count | Zeigt die Anzahl empfangener Nachrichten als Status |

- Node-Status ist optional (Checkbox aktiviert/deaktiviert die Anzeige)
- Der Status-Text wird auf **max. 32 Zeichen** gekürzt
- Default: deaktiviert

#### Config-Struktur in workspace.json

```json
{
  "id": "node-uuid",
  "type": "debug",
  "config": {
    "active": true,
    "output": "property",
    "property": "payload",
    "statusEnabled": false,
    "statusOutput": "same",
    "statusProperty": ""
  }
}
```

## Betroffene Dateien

### Frontend
- `frontend/src/components/nodes/DebugNode.vue` – Toggle-State aus Config lesen, bei Toggle Config ändern + dirty markieren
- `frontend/src/stores/flowStore.ts` – Node-Config-Update und Dirty-Tracking
- `frontend/src/stores/debugStore.ts` – Filterung der Debug-Messages basierend auf aktuellem Toggle-State (inkl. dirty)

### Backend
- `internal/nodes/debug.go` – `active`-Feld aus Config lesen, aber Nachrichten **immer** streamen
- `internal/flow/registry.go` – `DebugMessage`-Struct (kein `active`-Feld nötig, da Backend immer streamt)
