# Issue: Terminal Log — Application-Log im Frontend sichtbar machen

## Status: Proposed

## Problembeschreibung

Der Application-Log läuft derzeit ausschließlich auf **stdout** des LOOPZE-Prozesses (`cmd/loopze/main.go`, `slog.NewTextHandler(os.Stdout, …)`). Wer den Log sehen will, braucht Zugriff auf das Terminal in dem LOOPZE gestartet wurde — bei deployten Instanzen also SSH, `journalctl`, Docker-Logs o.ä.

Für den Bediener im Browser ist der Log damit unsichtbar. Typische Fragen wie *"warum ist mein Deploy fehlgeschlagen?"*, *"kommt mein MQTT-Connect durch?"* oder *"warum loggt der Server gerade so viel?"* erfordern jedes Mal einen Wechsel auf die Server-Konsole.

Ziel: Den laufenden Log direkt im Frontend sichtbar machen — als zuschaltbares Vollflächen-Fenster über dem Flow-Editor, mit farbiger Level-Kennzeichnung und Live-Streaming.

## Sichtweise / Begründung

Die Vorarbeit existiert bereits:

- **WebSocket-Hub** (`internal/ws/hub.go`) mit generischem `Broadcast(eventType, payload)` — neue Event-Typen lassen sich ohne Architekturänderung ergänzen
- **REST-Pattern mit Limit-Param** (`GET /api/v1/debug/messages?limit=N` in `internal/api/handlers.go`) — exakt das Pattern, das wir für `/api/v1/logs` brauchen
- **Standardisierter Logger**: alles geht durch `slog.Default()` — ein einziger benutzerdefinierter `slog.Handler` reicht aus, um sämtliche Log-Aufrufe abzugreifen
- **Header-Icons + uiStore-Toggles** (`HeaderBar.vue`, `uiStore.ts`) — das Pattern für ein neues Toggle-Icon ist etabliert

Architekturentscheidung: **Ring-Buffer im Backend** (kein NATS-Stream). Logs sind kurzlebige Beobachtung, kein persistentes Event-Log. Bei Restart sind sie weg — das ist akzeptiert und entspricht dem heutigen Verhalten von stdout.

## Anforderungen

### 1. Header-Icon

In `HeaderBar.vue` rechts neben dem Info/Debug-Icon ein neues Icon "Terminal Log" (Terminal-/Konsolen-Symbol, passend zum Lucide-/Heroicon-Set, das aktuell verwendet wird).

- Klick toggelt `uiStore.logsPanelOpen` (`true`/`false`)
- Aktiver Zustand: Icon visuell hervorgehoben (selbe Logik wie Properties-/Info-Icon)
- Tooltip: "Terminal Log" / "Application Log anzeigen"

### 2. Vollflächen-Overlay

Anders als Properties- und Info-Panel (die als rechte Sidebar einklappen) ist der Terminal-Log ein **Overlay über dem kompletten Flow-Bereich** — wie der Bediener es vom Konsolenfenster gewohnt ist und wie der User-Wunsch ihn explizit beschreibt: *"über den kompletten Flow ein Fenster"*.

Layout:
- Position: absolut, deckt den gesamten Canvas-Bereich ab (HeaderBar bleibt sichtbar, Sidebars optional verdeckt — pragmatisch: `inset-0` unter der HeaderBar, Z-Index oberhalb der Sidebars)
- Hintergrund: `bg-terminal-bg` mit leichter Opazität-Abstufung wenn nötig, damit der Charakter "schwebendes Fenster" erkennbar bleibt
- Schließen: X-Button oben rechts im Panel + ESC-Taste

Bewusst **kein** Tab in der bestehenden `InformationSidebar` — der Log ist kein Inspect-Werkzeug für ein einzelnes Element, sondern eine Übersichtsansicht und braucht entsprechend Platz.

### 3. Toolbar im Panel

Oben im Panel:
- **Limit-Dropdown**: `100 / 200 / 500 / 1000` Zeilen. Default: `200`. Auswahl persistiert in `uiStore.logsLimit` (in `localStorage`)
- **Level-Filter** (optional Phase 1, vermutlich nützlich): Multi-Select `DEBUG / INFO / WARN / ERROR`. Default: alle. Wirkt nur clientseitig auf die bereits geladenen / gestreamten Einträge — kein zusätzlicher Backend-Round-Trip
- **CLR-Button**: leert die Anzeige (nicht den Backend-Buffer). Selbe UX wie DebugPanel
- **Auto-Scroll-Toggle**: an = neue Einträge scrollen mit. Wird beim manuellen Hochscrollen automatisch ausgeschaltet, wieder eingeschaltet wenn der User ans Ende scrollt (Pattern aus `DebugPanel.vue`)

### 4. Lazy Loading + Streaming

**Beim Öffnen** des Panels:
1. `GET /api/v1/logs?limit=<dropdown-wert>` holt die letzten N Einträge aus dem Ring-Buffer und befüllt die Liste
2. Frontend registriert einen WebSocket-Listener für `EventLog` und hängt jeden eingehenden Eintrag ans Ende der Liste

**Beim Schließen**:
- WebSocket-Listener wird deregistriert (kein State-Update, keine Re-Renders während das Panel zu ist)
- Die zuletzt angezeigte Liste wird verworfen — beim nächsten Öffnen frisch laden

**Bei Limit-Wechsel** im offenen Panel:
- Frischer `GET /api/v1/logs?limit=N` ersetzt die Liste
- Streaming läuft weiter

Dieses Verhalten erfüllt die User-Anforderung: *"Das Log soll erst abgerufen werden wenn ich die Log Seite öffne und neue Logs sollen bei geöffneten Fenster hinein streamen."*

### 5. Level-Farben

Jede Zeile zeigt das Level in eckigen Klammern und farblich:

| Level | Farbe |
|---|---|
| `DEBUG` | grau (`text-zinc-500`) |
| `INFO`  | blau / Standard-Foreground (`text-blue-400` oder Theme-Default) |
| `WARN`  | gelb (`text-yellow-400`) |
| `ERROR` | rot (`text-red-400`) |

Eingefärbt wird mindestens das **Level-Tag**; die Message bleibt im Standard-Foreground. Komplette Zeilen-Färbung (Hintergrund-Tinten für ERROR-Zeilen) ist Phase 2 falls gewünscht.

### 6. Anzeigeformat pro Zeile

Eine Log-Zeile rendert sich als monospace-Text:

```
10:23:14.428 [INFO ] starting LOOPZE version=0.1.0 commit=abc123 log_level=info
10:23:14.512 [WARN ] websocket broadcast channel full type=debug
10:23:15.001 [ERROR] failed to connect mqtt broker error="connection refused"
```

- Zeitstempel: lokale Zeit, Format `HH:mm:ss.SSS` (Sekunden + Millisekunden — knapp und ausreichend für Sequenzanalyse)
- Level rechtsbündig in 5 Zeichen breitem Feld (`INFO ` mit Leerzeichen, damit Spaltenausrichtung passt)
- Message + Attribute werden zusammenhängend gerendert: `key=value` für jeden Attr-Eintrag, Werte mit Leerzeichen werden mit `"…"` gequoted (entspricht slog TextHandler-Output, an den der Bediener vom Terminal gewöhnt ist)

## Technische Skizze

### Backend — Ring-Buffer + slog.Handler

Neues Package `internal/logbuffer` mit:

```go
type LogEntry struct {
    Seq     uint64         `json:"seq"`     // monoton wachsend, vom Buffer vergeben
    Time    time.Time      `json:"time"`
    Level   string         `json:"level"`   // "DEBUG" | "INFO" | "WARN" | "ERROR"
    Message string         `json:"message"`
    Attrs   map[string]any `json:"attrs,omitempty"`
}

type Buffer struct {
    mu      sync.RWMutex
    entries []LogEntry // ring; size = capacity
    head    int
    full    bool
    cap     int
    seq     uint64     // wird in Add() inkrementiert und in entry.Seq gesetzt
}

func New(capacity int) *Buffer
func (b *Buffer) Add(e LogEntry) LogEntry // gibt Entry mit gesetzter Seq zurück (für notify)
func (b *Buffer) Last(n int) []LogEntry   // oldest-first slice der letzten min(n, len) Einträge
```

Capacity: konfigurierbar über `cfg.LogBufferSize` mit Default `1000`. Wert wird beim Start in `cmd/loopze/main.go` an `logbuffer.New` übergeben. UI-Maximum bleibt `1000` (der Dropdown-Cap), die Backend-Capacity darf größer sein wenn z.B. eine externe API künftig mehr abrufen soll. Validierung in `config`: Minimum `1`, kein Maximum erzwungen.

Die `Seq`-ID wird beim `Add` vom Buffer vergeben und sowohl in den Buffer geschrieben als auch im zurückgegebenen Entry an den Notify-Callback durchgereicht — damit haben REST-Antwort und WebSocket-Stream **dieselben** Seq-Werte für denselben Eintrag.

Neuer `slog.Handler`-Wrapper in `internal/logbuffer`:

```go
type Handler struct {
    inner  slog.Handler          // delegate für stdout (TextHandler)
    buf    *Buffer
    notify func(LogEntry)         // optional: Callback für Live-Broadcast
}

func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
    // 1) durchreichen an inner (stdout-Verhalten unverändert)
    if err := h.inner.Handle(ctx, r); err != nil {
        return err
    }
    // 2) zu LogEntry konvertieren, in Ring schreiben, Notify
    entry := toEntry(r)
    h.buf.Add(entry)
    if h.notify != nil {
        h.notify(entry)
    }
    return nil
}

// Enabled, WithAttrs, WithGroup an inner durchreichen.
```

Wichtig: **stdout-Verhalten bleibt 1:1 erhalten** — wer heute mit `journalctl` arbeitet, merkt nichts. Der Wrapper ist additiv.

### Backend — Wiring in `cmd/loopze/main.go`

```go
buf := logbuffer.New(cfg.LogBufferSize) // Default 1000, konfigurierbar
inner := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel})

// Hub muss vor dem Logger existieren, damit notify ihn aufrufen kann.
// Aktuell wird der Hub im Server erstellt — entweder Hub-Erstellung in main vorziehen,
// oder notify per zwei-Stufen-Hookup setzen (siehe unten).

handler := logbuffer.NewHandler(inner, buf, nil) // notify zunächst nil
slog.SetDefault(slog.New(handler))

srv, err := server.New(cfg, buf) // Server bekommt Buffer für REST-Endpoint
// Nach srv.New: Hub existiert. Notify nachträglich setzen.
handler.SetNotify(func(e logbuffer.LogEntry) {
    srv.Hub().Broadcast(ws.EventLog, e)
})
```

Alternativ: Hub aus dem Server herausziehen und in `main.go` instantiieren — sauberer, aber größerer Refactor. **Pragmatisch**: zwei-Stufen-Hookup mit `SetNotify` (ein einziges atomares Pointer-Field reicht).

### Backend — Neuer Event-Typ

`internal/ws/hub.go`:

```go
const (
    EventDebug        = "debug"
    EventStatus       = "status"
    EventDeploy       = "deploy"
    EventNotification = "notification"
    EventLog          = "log" // NEU
)
```

### Backend — REST-Endpoint

`internal/api/routes.go` registriert `r.Get("/logs", h.GetLogs)`.

`internal/api/handlers.go`:

```go
func (h *Handler) GetLogs(w http.ResponseWriter, r *http.Request) {
    limit := parseIntQuery(r, "limit", 200)
    if limit < 1 { limit = 1 }
    if limit > 1000 { limit = 1000 } // hard cap = buffer capacity
    entries := h.logBuffer.Last(limit)
    writeJSON(w, http.StatusOK, entries)
}
```

Pattern und Param-Validierung exakt wie `GetDebugMessages`.

### Backend — Selbst-Referenz vermeiden

Der WebSocket-Hub loggt selbst (`slog.Info("websocket client connected", …)`). Diese Logs landen über den Wrapper im Buffer und werden dann erneut über den Hub an alle Clients broadcastet — kein Loop, weil die Clients die Events nur empfangen, nicht zurücksenden. Trotzdem **Vorsicht** bei zukünftigen Änderungen: Falls ein Hub-Broadcast jemals selbst loggt (z.B. Error-Pfad in `Broadcast`), entsteht eine Endlosschleife. Schutz: `Broadcast` darf intern nur bei "channel full" loggen (heute der Fall) und das via `slog.Warn` — solange Warn-Logs an "channel full"-Bedingung gekoppelt sind, wirken sie selbstdrosselnd.

### Frontend — `uiStore.ts`

Analog zu `propertiesPanelOpen` / `infoPanelOpen`:

```ts
const logsPanelOpen = ref(false)
const logsLimit = ref<100|200|500|1000>(200)
function toggleLogsPanel() { logsPanelOpen.value = !logsPanelOpen.value }
function openLogsPanel()   { logsPanelOpen.value = true }
function closeLogsPanel()  { logsPanelOpen.value = false }
```

`logsLimit` wird in `localStorage` persistiert (selbes Pattern wie `propertiesPanelWidth`).

### Frontend — `HeaderBar.vue`

Neuer Button-Block nach dem Debug/Info-Icon (HeaderBar.vue ~Zeile 153):

```vue
<button
  @click="ui.toggleLogsPanel()"
  :class="['icon-btn', { active: ui.logsPanelOpen }]"
  title="Terminal Log"
>
  <TerminalIcon class="w-5 h-5" />
</button>
```

### Frontend — `TerminalLogPanel.vue` (neu)

Komponente mit folgender Struktur:

```vue
<template v-if="ui.logsPanelOpen">
  <div class="absolute inset-0 z-40 bg-terminal-bg flex flex-col">
    <header>
      <select v-model="ui.logsLimit" @change="reload">
        <option :value="100">100 Zeilen</option>
        <option :value="200">200 Zeilen</option>
        <option :value="500">500 Zeilen</option>
        <option :value="1000">1000 Zeilen</option>
      </select>
      <LevelFilterChips v-model="levelFilter" />
      <button @click="entries = []">CLR</button>
      <button @click="ui.closeLogsPanel()">✕</button>
    </header>
    <div ref="scrollEl" class="flex-1 overflow-auto font-mono text-xs">
      <LogLine v-for="e in visibleEntries" :key="e.idx" :entry="e" />
    </div>
  </div>
</template>
```

`onMounted` (genauer: `watch(() => ui.logsPanelOpen, …, { immediate: true })`):
- Wenn open wird:
  1. `unsubscribe = ws.onLog(stagedAdd)` **zuerst** registrieren — Streaming-Einträge landen ab sofort in einem Staging-Array
  2. `const initial = await api.getLogs(limit)` — REST-Antwort holen (kommt mit `Seq`-IDs)
  3. `entries.value = initial; lastSeqFromHttp = initial.at(-1)?.seq ?? 0`
  4. Staging-Array nach `entries.value` flushen, dabei alle Einträge mit `seq <= lastSeqFromHttp` verwerfen (= Doppelte aus dem HTTP-Set)
  5. `stagedAdd` auf direkten Append umschalten
- Wenn close wird: `unsubscribe?.()`; `entries.value = []`

Die Reihenfolge "WS subscribe → HTTP fetch → merge → switch" garantiert, dass kein Eintrag zwischen REST-Antwort und WebSocket-Subscribe verloren geht (würde er sonst in der Lücke), und dass Doppelte zuverlässig erkannt werden (gleiche `Seq` aus REST und WS).

Auto-Scroll: nach jedem `addLog` `nextTick` → wenn `scrollEl` zuvor am Ende war (`scrollHeight - scrollTop - clientHeight < 4`), neu ans Ende scrollen. User-Scroll-Up unterbricht Auto-Scroll, Scroll-To-Bottom reaktiviert.

### Frontend — `useWebSocket.ts`

Neuer Dispatcher `onLog(cb: (e: LogEntry) => void): () => void` analog zu `onDebug`/`onStatus`. Da das Panel die einzige Stelle ist, die Log-Events konsumiert, gibt es keinen globalen Store dafür — der Listener registriert sich nur solange das Panel offen ist (siehe Lazy Loading).

### Frontend — `useApi.ts`

Neue Methode:

```ts
async function getLogs(limit: number): Promise<LogEntry[]> {
  return request<LogEntry[]>(`/logs?limit=${limit}`)
}
```

### Frontend — `LogLine.vue` (neu)

Kleine Helper-Komponente für eine einzelne Zeile:
- Zeitformatierung
- Level-Klasse aus Mapping
- Attrs als `key=value` joined, Werte mit Whitespace gequoted

Reine Präsentation, keine eigene State.

## Betroffene Dateien

### Backend
- `internal/logbuffer/buffer.go` (neu) — Ring-Buffer + `LogEntry` mit `Seq`-Vergabe
- `internal/logbuffer/handler.go` (neu) — `slog.Handler`-Wrapper mit `SetNotify`
- `internal/logbuffer/buffer_test.go` (neu) — Wraparound, Last(n), Seq-Monotonie, Concurrency
- `internal/config/config.go` — neues Feld `LogBufferSize int` (Default `1000`, Min-Validierung), Flag `--log-buffer-size`, Env `LOOPZE_LOG_BUFFER_SIZE`
- `internal/ws/hub.go` — `EventLog = "log"` Konstante ergänzen
- `internal/server/server.go` — `New(cfg, buf)`-Signatur, `Hub()`-Getter, REST-Routing-Aufruf reicht den Buffer in `api.NewHandler` durch
- `internal/api/handlers.go` — `GetLogs` Handler, `logBuffer`-Feld im Handler-Struct
- `internal/api/routes.go` — `r.Get("/logs", h.GetLogs)`
- `cmd/loopze/main.go` — Buffer mit `cfg.LogBufferSize` + Wrapper-Handler instantiieren, nach `server.New` `SetNotify` mit Hub-Broadcast verdrahten

### Frontend
- `frontend/src/stores/uiStore.ts` — `logsPanelOpen`, `logsLimit`, Toggle-Actions, localStorage-Persistenz für `logsLimit`
- `frontend/src/components/HeaderBar.vue` — neues Icon-Button neben Debug/Info
- `frontend/src/components/TerminalLogPanel.vue` (neu) — Vollflächen-Overlay
- `frontend/src/components/LogLine.vue` (neu) — eine Log-Zeile mit Level-Farbe
- `frontend/src/views/FlowEditor.vue` — `<TerminalLogPanel />` einhängen (Position: über dem Canvas)
- `frontend/src/composables/useWebSocket.ts` — `onLog`-Dispatcher
- `frontend/src/composables/useApi.ts` — `getLogs(limit)`

## Abhängigkeiten

Keine externen. Nutzt:
- `log/slog` (Standard-Library) — bereits im Einsatz
- bestehender WebSocket-Hub
- bestehendes REST-Pattern (`/debug/messages?limit=N`)
- bestehendes Header-Icon-/uiStore-Pattern

## Out of Scope für Phase 1

- **Persistenter Log über Restart** — Buffer ist In-Memory, das ist Absicht (entspricht stdout-Verhalten)
- **Backend-seitiger Filter / Suche** — clientseitige Level-Filterung reicht für 1000 Einträge problemlos
- **Volltextsuche im Frontend** — kann später als Browser-typisches Ctrl+F bzw. Such-Inputfeld nachgerüstet werden
- **Download/Export** des aktuellen Log-Inhalts — denkbar als "Copy as text"-Button in Phase 2
- **Source-Filter** (nur Logs aus bestimmten Packages) — slog-Records tragen keine zuverlässige Quelle ohne `AddSource`, das treiben wir erst wenn Bedarf entsteht
- **Mehrere Server / Cluster-Logs** — LOOPZE ist single-instance, jeder Browser sieht den Log seines verbundenen Servers
- **ANSI-Color-Codes aus stdout in HTML übersetzen** — slog produziert keine ANSI-Codes, also nicht relevant
- **Hintergrund-Tinte für ERROR-Zeilen** — wenn der Wunsch konkret entsteht, leicht ergänzbar

## Offene Fragen

Keine.
