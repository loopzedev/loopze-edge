# Flint – Projekt-Entscheidungen & Offene Fragen

> Dieses Dokument dient als zentrales Entscheidungslog für das Projekt.
> Jede Frage wird gemeinsam beantwortet und die Entscheidung hier festgehalten.

---

## 1. Projektziel

| Frage | Antwort |
|---|---|
| Was ist die Vision? | Ein Node-RED-Clone mit modernem Tech-Stack |
| Zielgruppe? | ✅ Versierte Techniker (keine Fullstack-Entwickler), Automatisierungstechniker, SPS-Programmierer mit Script-Erfahrung |
| Lizenzmodell? | ✅ Elastic License 2.0 (ELv2) – kostenlos nutzbar, kein Einbetten, keine Modifikation |
| Projektname Edge App? | ✅ **Flint** |
| Projektname Management Platform? | ✅ **Flint Hub** |

### 1.1 Zielgruppe – Details & Design-Konsequenzen

**Profil des Nutzers:**
- Techniker, Ingenieure, SPS-Programmierer (z.B. IEC 61131-3: Ladder, FBD, ST)
- Script-Erfahrung (Bash, Python, VBA, Structured Text) – aber kein Fullstack-Hintergrund
- Denkt in **Prozessen, Signalflüssen und funktionellen Blöcken** (wie in der Automatisierungstechnik)
- Gewohnt an: klare I/O-Definitionen, deterministisches Verhalten, Fehlerdiagnose im Feld

**Konsequenzen für das UI/UX:**
| Aspekt | Konsequenz |
|---|---|
| Keine komplexen IDE-Features | Editor muss intuitiv sein, wenig Lernkurve |
| SPS-Denke: Funktionsblöcke | Nodes müssen klar beschriftete Inputs/Outputs haben (wie FBD-Bausteine) |
| Script-Erfahrung vorhanden | Function-Nodes mit einfacher Skriptsprache (kein volles JS-Framework nötig) |
| Feldnahe Anwendungen | Gute MQTT, Serial, Modbus, OPC-UA Unterstützung wichtig |
| Fehlerdiagnose im Betrieb | Debug-Ausgaben müssen klar, live und filterbar sein |
| Keine Package-Manager-Kenntnisse | Installation muss einfach sein → Single Binary bevorzugt |
| Dokumentation inline | Nodes sollten Tooltips / eingebaute Hilfe haben |

**Konsequenzen für das Node-System:**
| Aspekt | Konsequenz |
|---|---|
| SPS: klar typisierte Signale | Optionale Typisierung von msg-Feldern (String, Number, Bool, Byte) wünschenswert |
| SPS: zyklische Ausführung | Timer/Inject-Nodes müssen präzise und zuverlässig sein |
| Scripting | Function-Node-Sprache muss einfach sein – JavaScript (Goja) oder Lua bevorzugt |
| Modbus / OPC-UA / Serial | Diese Nodes haben hohe Priorität (industrielles Umfeld) |
| Deterministisches Verhalten | Klare Aussagen über Ausführungsreihenfolge und Fehlerverhalten |

### 1.2 Geschäftsmodell – Open Core ✅

Referenz: GitLab · n8n · Grafana – freier Core, kommerzielle Enterprise-Erweiterungen

╔══════════════════════════════════════════════════════════╗
║        STUFE 2 – FLEET/ENTERPRISE  (Kommerziell)        ║
║                                                          ║
║  [Edge A] ◀──▶ [Management Platform] ◀──▶ [Edge B]     ║
║  [Edge C] ◀────────────┘              [Edge D]          ║
║                                                          ║
║  Fleet-Mgmt · Nachrichten-Routing · Stammdaten-Sync     ║
╠══════════════════════════════════════════════════════════╣
║        STUFE 1 – EDGE  (Open Source / MIT)              ║
║                                                          ║
║  [Flow-Editor (Vue3)] [Runtime (Go)] [Node-Library]     ║
║  Läuft standalone · Single Binary · keine Abhängigkeiten║
╚══════════════════════════════════════════════════════════╝

#### Stufe 1 – Edge Node (Open Source)
| Lizenz          | ✅ Elastic License 2.0 (ELv2)                                |
| Umfang          | Vollständiger Flow-Editor + Runtime auf einem Gerät         |
| Standalone      | Läuft autonom, keine Netzwerkabhängigkeit                   |
| Ziel            | Community, Adoption, Vertrauen                              |

#### Stufe 2 – Fleet & Management Platform (Kommerziell)
| Fleet Management  | Alle Edges zentral registrieren, überwachen, verwalten    |
| Nachrichten-Routing | Messages zwischen Edges weiterleiten + transformieren   |
| Stammdaten-Sync   | Konfigurationen/Lookup-Tables auf alle Nodes verteilen    |
| Management UI     | Zentrale Web-Oberfläche aller Nodes, Status, Flows        |
| Remote Deploy     | Flows zentral auf einzelne oder alle Edges ausrollen      |
| Audit Log         | Wer hat wann welchen Flow deployed/geändert               |
| Hosting           | ✅ Self-hosted On-Premise                                  |
| Pricing           | ✅ Pro Gerät/Node – Lizenzschlüssel pro Installation       |

#### Technische Konsequenzen
| Saubere Modul-Trennung   | Edge-Core ≠ Management-Code – von Anfang an getrennt    |
| Agent-Konzept im Edge    | Optionaler "Fleet Agent" – per Lizenzschlüssel aktivierbar|
| Kein Enterprise-Code im OSS | Management-Features NICHT im Open-Source-Code         |
| Offline-Fähigkeit        | Edge läuft autonom weiter wenn Management nicht erreichbar|
| Edge-Identität           | Jeder Edge braucht UUID + kryptografische Credentials     |
| Sichere Kommunikation    | Edge ↔ Management zwingend TLS / mTLS                    |

---

## 2. Tech-Stack

### 2.1 Backend

| Frage | Optionen | Entscheidung |
|---|---|---|
| Programmiersprache Backend | **Go** (schnell, single Binary, Goroutines für Concurrency) | ✅ Go |
| HTTP-Framework | `net/http` (stdlib), `Fiber`, `Echo`, `Chi`, `Gin` | ✅ Chi |
| WebSocket-Library | `gorilla/websocket`, `nhooyr/websocket`, `coder/websocket` | ✅ gorilla/websocket |

### 2.2 Frontend

| Frage | Optionen | Entscheidung |
|---|---|---|
| Frontend-Framework | **Vue 3** (mit Vue Flow), React (mit xyflow), Svelte | ✅ Vue 3 |
| Flow/Canvas Library | **Vue Flow** (`@vue-flow/core`) – 6.4k Stars, MIT, genutzt von n8n | ✅ Vue Flow |
| UI-Component Library | Vuetify, PrimeVue, Naive UI, Headless UI, Custom | ✅ Tailwind CSS + Radix Vue (eigene Komponenten) |
| State Management | Pinia, Vuex, oder nur Composables | ✅ Pinia |
| CSS/Styling | Tailwind CSS, UnoCSS, Plain CSS/SCSS | ✅ Tailwind CSS |
| Build-Tool | Vite | ✅ Vite (Standard für Vue 3) |

### 2.3 Deployment & Build

| Frage | Optionen | Entscheidung |
|---|---|---|
| Frontend embedded in Go-Binary? | Ja (`go:embed`) → Single Binary wie Node-RED | ja |
| Deployment-Ziel | Single Binary, Docker, beides? | beides |
| Zielplattformen | Linux, macOS, Windows, ARM (Raspberry Pi)? | alle |

### 2.4 Storage / Persistenz

| Frage | Optionen | Entscheidung |
|---|---|---|
| Flow-Speicherung | JSON-Files (wie Node-RED), SQLite, PostgreSQL | JSON File |
| Credentials/Secrets | Verschlüsselt in separatem File (Stufe 1), zentral im Management (Stufe 2) | ✅ Separates verschlüsseltes Credentials-File (AES-256-GCM) + auto-generiertes Keyfile (`goflux.key`) |
| Node-Konfigurationen | Im Flow-JSON eingebettet (wie Node-RED) | ja |


---

## 2.6 NATS – Architektur & Scope ✅

### NATS wird verwendet für:

| Verwendung | Mechanismus | Detail |
|---|---|---|
| **Flint ↔ Flint Hub** | LeafNode Connection | Kommunikation zwischen Edge und Management Platform |
| **Context Speicher** | KV Store | Flow-Context und Global-Context für Function Nodes |
| **Stammdaten Sync** | KV Store Mirror | Flint Hub verteilt Stammdaten zu allen Edges |
| **Debug Messages** | JetStream (Ringbuffer) | Letzten N Debug-Messages persistent, live abrufbar |

### NATS wird NICHT verwendet für:

| Was | Stattdessen |
|---|---|
| **Node ↔ Node innerhalb eines Flows** | Go Channels – direkt, kein Overhead |
| **Flow ↔ Flow Kommunikation** | Go-intern |
| **Node State** | Go Memory |
| **Internes Message Handling** | Go Channels |

### Begründung Hybrid-Modell:

| Aspekt | Detail |
|---|---|
| **Performance** | Node-zu-Node innerhalb eines Flows über Go Channels – nanosecond Latenz |
| **Transparenz** | Debug Messages in JetStream – Techniker kann live mit NATS CLI reinschauen |
| **Offline-Fähigkeit** | NATS embedded – Edge läuft vollständig ohne Hub-Verbindung |
| **Einfachheit** | Klare Trennung: Go = intern, NATS = Persistenz + Fleet |

---

## 3. Architektur & Design

### 3.1 Node-System

| Frage | Optionen | Entscheidung |
|---|---|---|
| Node-RED-kompatibel? | Ja (gleiches JSON-Format) vs. Nein (eigenes Format, nur UX inspiriert) | nein |
| Scripting in Function-Nodes | JavaScript (via Goja), expr-lang, Lua, WASM | ✅ **Dual-Mode**: JavaScript (via Goja) + expr-lang |
| Node-Erweiterbarkeit | Go-Plugins, WASM-Module, Scripted Nodes, gRPC | ✅ Go-Plugins |
| Node-Kategorien | Common, Function, Network, Sequence, Parser, Storage, Industrial | ✅ Common · Function · Network · Industrial · Storage · Parser |

### 3.2 Flow-Runtime

| Frage | Optionen | Entscheidung |
|---|---|---|
| Message-Modell | Struct mit festen Feldern vs. freie `map[string]any` wie Node-RED | ✅ Freie `map[string]any` – alle Felder gleichberechtigt, `_id` unveränderlich (s. 5.1.5) |
| Concurrency-Modell | Goroutine pro Flow, pro Node, oder pro Message | ✅ Goroutine pro Node |
| Error Handling | Catch-Nodes (wie Node-RED), globaler Error-Handler, beides | ✅ Catch-Nodes + globaler Error-Handler |
| Flow-Typen | Normal Flows, Subflows (wiederverwendbar), Config-Nodes | ✅ Normal Flows + Subflows + Config-Nodes |

### 3.3 Kommunikation Frontend ↔ Backend

| Frage | Optionen | Entscheidung |
|---|---|---|
| API-Stil | REST + WebSocket, GraphQL + WebSocket, gRPC-Web | ✅ REST + WebSocket |
| Echtzeit-Events | WebSocket für Debug-Output, Flow-Status, Deploy-Events | ✅ WebSocket |
| Auth/Security | Erstmal ohne, Basic Auth, JWT, OAuth | ✅ MVP: ohne – Stufe 2: JWT |

---

## 4. MVP – Minimum Viable Product

### 4.1 Welche Features müssen im MVP sein?

| Feature | Priorität | Im MVP? |
|---|---|---|
| Flow-Editor Canvas (Nodes platzieren, verbinden, löschen) | Must | ✅ Ja |
| Node-Palette/Sidebar (Nodes per Drag&Drop hinzufügen) | Must | ✅ Ja |
| Node-Konfigurationsdialog (Doppelklick → Settings) | Must | ✅ Ja |
| Deploy-Button (Flow aktivieren) | Must | ✅ Ja |
| Debug-Sidebar (Messages live anzeigen via WebSocket) | Must | ✅ Ja |
| Import/Export von Flows (JSON) | Should | ✅ Ja |
| Undo/Redo | Should | ✅ Ja |
| Tabs (mehrere Flows gleichzeitig) | Should | ✅ Ja |
| Subflows | Could | ✅ Ja |
| Flow-Variablen (flow/global context) | Could | ✅ Ja |
| Dashboard / UI-Nodes | Won't (MVP) | ❌ Nein |

### 4.2 Welche Nodes müssen im MVP sein?

| Node | Typ | Beschreibung | Priorität (Zielgruppe) |
|---|---|---|---|
| **Inject** | Input | Timer/Trigger, sendet Messages | 🔴 Must |
| **Debug** | Output | Zeigt Messages in der Sidebar | 🔴 Must |
| **Function** | Processing | Custom Script ausführen | 🔴 Must |
| **Change** | Processing | msg-Properties setzen/ändern/löschen | 🔴 Must |
| **Switch** | Routing | Messages anhand von Regeln routen | 🔴 Must |
| **HTTP In** | Input | HTTP-Endpunkt erstellen | 🟡 Should |
| **HTTP Response** | Output | HTTP-Antwort senden | 🟡 Should |
| **HTTP Request** | Processing | HTTP-Aufrufe machen | 🟡 Should |
| **Template** | Processing | Text-Templates rendern | 🟡 Should |
| **Delay** | Processing | Messages verzögern/rate-limiten | 🟡 Should |
| **MQTT In/Out** | I/O | MQTT Pub/Sub – industriell sehr relevant | 🔴 Must (Zielgruppe!) |
| **Modbus TCP/RTU** | I/O | SPS-Kommunikation – für SPS-Programmierer essenziell | 🟡 Should |
| **OPC-UA** | I/O | Industrie-Standard für Maschinen-Kommunikation | 🟢 Could |
| **Serial/COM** | I/O | RS232/RS485 – verbreitet in der Industrie | 🟢 Could |
| **WebSocket In/Out** | I/O | WebSocket Kommunikation | 🟢 Could |

---

## 5. Abgrenzung & Alleinstellungsmerkmale

| Frage | Antwort |
|---|---|
| Was soll Flint **besser** machen als Node-RED? | ✅ Performance (Go statt Node.js), Single Binary, kein npm/Node.js nötig, Industrie-Protokolle first-class (Modbus, OPC-UA), expr-lang für schnelle Ausdrücke |
| Was soll Flint **anders** machen als Node-RED? | ✅ Open Core mit Fleet Management (Flint Hub), Stammdaten-Sync, Retro-Industrial UI, Dual-Mode Scripting, ELv2 Lizenz |
| Was soll Flint bewusst **nicht** haben? | ✅ Kein npm-Package-Ökosystem, kein Dashboard/UI-Node-System (MVP), keine Cloud-Abhängigkeit |

### 5.1 UX-Verbesserungen gegenüber Node-RED ✅

> Konkrete Bedienungs- und Konzeptentscheidungen, die Flint bewusst anders (besser) machen soll als Node-RED.

#### 5.1.1 Node-Ports: Single Input, Multiple Outputs ✅

| Aspekt | Entscheidung |
|---|---|
| Eingänge pro Node | **Genau 1** (oder 0 bei reinen Source-Nodes wie Inject) |
| Ausgänge pro Node | **1 bis N** (je nach Node-Typ, z.B. Switch hat mehrere Ausgänge) |
| Begründung | Klares, deterministisches Signalfluss-Modell – wie in der SPS-Welt (FBD). Ein Eingang = ein Auslöser. Mehrere Eingänge erzeugen Mehrdeutigkeit über Ausführungsreihenfolge und Merge-Verhalten. Wer Signale zusammenführen will, nutzt explizit einen Join/Merge-Node. |
| Node-RED Vergleich | Node-RED erlaubt multiple Inputs – führt in der Praxis zu verwirrenden Flows, bei denen unklar ist, welcher Input die Ausführung triggert. |

```
  ┌──────────┐     ┌──────────────┐     ┌──────────┐
  │  Inject  ├────▶│   Function   ├──┬─▶│  Debug   │
  └──────────┘     └──────────────┘  │  └──────────┘
                        1 Input      │  ┌──────────┐
                        2 Outputs    └─▶│  MQTT Out│
                                        └──────────┘
```

#### 5.1.2 Universelles Node-Debugging ✅

| Aspekt | Entscheidung |
|---|---|
| Debug-Node | Gibt es weiterhin als eigenständigen Node (wie Node-RED) |
| **NEU: Debug per Node** | **Jeder Node** kann individuell in einen Debug-Modus versetzt werden |
| Aktivierung | Kleines Debug-Icon (🔍) direkt am Node im Flow-Editor – per Klick an/aus |
| Anzeige | Im Debug-Panel werden **alle eingehenden UND ausgehenden Messages** des Nodes angezeigt |
| Darstellung | Eingehende Messages mit `→ IN` Prefix, ausgehende mit `OUT →` Prefix, jeweils mit Timestamp |
| Vorteil | Kein manuelles Einfügen von Debug-Nodes zwischen Leitungen nötig – spart Klicks, hält den Flow sauber |
| Performance | Debug-Tap wird nur aktiviert wenn eingeschaltet – kein Overhead im Normalbetrieb |
| Node-RED Vergleich | Node-RED erfordert für jede Debug-Stelle einen separaten Debug-Node → Flow wird schnell unübersichtlich |

```
  ┌──────────┐         ┌──────────────┐         ┌──────────┐
  │  Inject  ├────────▶│  Function 🔍 ├────────▶│  MQTT Out│
  └──────────┘         └──────┬───────┘         └──────────┘
                              │
                   ┌──────────▼──────────┐
                   │   Debug-Panel:      │
                   │   09:14:01 → IN     │
                   │     { payload: 42 } │
                   │   09:14:01 OUT →    │
                   │     { payload: 84 } │
                   └─────────────────────┘
```

#### 5.1.3 Wire-Debugging (Signalleitungen) ✅

| Aspekt | Entscheidung |
|---|---|
| **NEU: Debug per Wire** | Auch einzelne **Verbindungsleitungen** (Wires) können in den Debug-Modus versetzt werden |
| Aktivierung | Klick auf eine Leitung → Kontextmenü oder kleines Debug-Icon an der Leitung |
| Anzeige | Im Debug-Panel werden alle Messages angezeigt, die über diese spezifische Leitung fließen |
| Darstellung | Wire-Debug zeigt: Source-Node → Target-Node, Timestamp, Message-Inhalt |
| Visualisierung | Aktive Debug-Wires werden im Flow-Editor visuell hervorgehoben (z.B. pulsierende Amber-Animation) |
| Vorteil | Präzise Fehlersuche bei komplexen Flows mit vielen Verzweigungen – man sieht exakt was auf welchem Pfad fließt |
| Node-RED Vergleich | Node-RED hat kein Wire-Debugging – man muss Debug-Nodes zwischen jede Verbindung setzen |

```
  ┌──────────┐    ⚡DebugWire     ┌──────────┐
  │  Switch  ├═══════════════════▶│  HTTP Out│
  └────┬─────┘                    └──────────┘
       │            ┌──────────┐
       └───────────▶│  MQTT Out│
                    └──────────┘

  Debug-Panel:
  ═══ Switch → HTTP Out ═══
  09:14:01  { payload: "ok", topic: "status" }
  09:14:02  { payload: "ok", topic: "status" }
```

#### Zusammenfassung: Debug-Konzept in Flint

| Feature | Node-RED | Flint |
|---|---|---|
| Debug-Node | ✅ Ja | ✅ Ja (bleibt erhalten) |
| Debug per Node | ❌ Nein | ✅ **Jeder Node kann gedebugt werden** |
| Debug zeigt IN + OUT | ❌ Nein (nur Eingang) | ✅ **Eingehende + Ausgehende Messages** |
| Debug per Wire | ❌ Nein | ✅ **Leitungen individuell debugbar** |
| Debug-Overhead im Normalbetrieb | Immer aktiv (Debug-Node) | ✅ **Nur wenn aktiviert – Zero-Cost wenn aus** |
| Flow-Sauberkeit | Debug-Nodes überall → unübersichtlich | ✅ **Flow bleibt sauber, Debug ist Overlay** |

#### 5.1.4 Namensgebung: Workspace → Flow ✅

| Aspekt | Entscheidung |
|---|---|
| Top-Level-Container | **Workspace** – enthält alle Flows, Konfiguration, Credentials |
| Einzelner Tab | **Flow** – ein unabhängiger Datenfluss mit Nodes und Wires |
| Begründung | In Node-RED bedeutet "Flow" sowohl die gesamte Konfiguration als auch einen einzelnen Tab – das ist verwirrend. Flint trennt klar: ein **Workspace** hat mehrere **Flows**. |
| Node-RED Vergleich | Node-RED: "flows.json" = alles, "flow" = Tab → doppeldeutig. Flint: Workspace = Container, Flow = Tab → eindeutig. |

```
  ┌─────────────────────────────────────────────┐
  │  Workspace                                  │
  │  ┌─────────┐  ┌─────────┐  ┌─────────┐    │
  │  │ Flow 1  │  │ Flow 2  │  │ Flow 3  │    │
  │  │ (Tab)   │  │ (Tab)   │  │ (Tab)   │    │
  │  └─────────┘  └─────────┘  └─────────┘    │
  └─────────────────────────────────────────────┘
```

#### 5.1.5 Message-Modell: Freie Map mit unveränderlicher ID ✅

| Aspekt | Entscheidung |
|---|---|
| Datenstruktur | **`map[string]any`** – alle Felder sind gleichberechtigt, kein Struct mit festen Feldern |
| `_id` | Wird bei Erstellung automatisch vergeben (16 Bytes random hex), **unveränderlich** – kann nicht per Set/Delete überschrieben werden |
| Feldzugriff | Einheitlich über `Get("path.to.field")` / `Set("path.to.field", val)` – kein Unterschied zwischen `payload`, `topic` und beliebigen Custom-Feldern |
| JSON-Serialisierung | Flaches JSON-Objekt: `{"_id":"abc","payload":42,"topic":"x","myField":true}` |
| Clone | Erzeugt Deep-Copy mit **neuer** `_id` |
| Begründung | In Node-RED ist `msg` ein freies JS-Objekt – man kann beliebig Felder kopieren, verschieben, hinzufügen. Das ist ein Erfolgsgeheimnis. Feste Go-Structs brechen diese Flexibilität (First-class vs. Second-class Felder). |
| Node-RED Vergleich | Node-RED: `msg.payload = msg.topic` funktioniert sofort. Flint: identisch über `msg.Set("payload", msg.Get("topic"))`. |

```
  Geschützte Felder (Runtime-intern):
  ┌─────────────────────────────────────┐
  │  _id: "a1b2c3..."  ← IMMUTABLE     │
  └─────────────────────────────────────┘

  Freie Felder (User/Node):
  ┌─────────────────────────────────────┐
  │  payload: { id: 42, name: "S1" }   │
  │  topic: "sensors/temp"              │
  │  original_payload: { ... }          │
  │  myCustomField: true                │
  │  _timestamp: "2026-03-17T..."       │
  └─────────────────────────────────────┘
```

---

## 6. Entscheidungslog

> Hier werden getroffene Entscheidungen chronologisch dokumentiert.

| Datum | Entscheidung | Begründung |
|---|---|---|
| 2026-03-16 | Backend: **Go** | Performance, Single Binary, Goroutines für parallele Flow-Ausführung |
| 2026-03-16 | Frontend: **Vue 3 + Vue Flow** | Vue Flow ist ausgereift, MIT-Lizenz, von n8n genutzt, ideal für Node-Editor |
| 2026-03-16 | Build-Tool: **Vite** | Standard für Vue 3, schnell, einfach |
| 2026-03-16 | Zielgruppe: **Techniker & SPS-Programmierer** | Fokus auf industrielle Protokolle, einfache UX, Script-basierte Function-Nodes |
| 2026-03-16 | Lizenz Stufe 1: **Elastic License 2.0 (ELv2)** | Kostenlos nutzbar, kein Einbetten, keine Modifikation, kein Konkurrenz-Clone |
| 2026-03-16 | HTTP-Framework: **Chi** | 100% net/http kompatibel, minimal, Go-idiomatisch, URL-Parameter + Middleware |
| 2026-03-16 | WebSocket-Library: **gorilla/websocket** | De-facto Standard, battle-tested, kompatibel mit Chi |
| 2026-03-16 | UI-Styling: **Tailwind CSS + Radix Vue** | Eigene Komponenten, maximale Kontrolle, kein modernes Framework-Look |
| 2026-03-16 | Design: **Amber Terminal Retro-Look** | Consolas überall, eckige Komponenten, Dunkel-Theme, Amber-Akzent – passt zur Techniker-Zielgruppe |
| 2026-03-16 | Credentials: **Separates verschlüsseltes File** (Stufe 1) + **Central Vault** im Management (Stufe 2) | Klare Trennung, Passwörter nie im Flow-JSON, Stufe 2 als Differenzierungsmerkmal |
| 2026-03-16 | State Management: **Pinia** | Offizieller Vue 3 Standard, TypeScript-nativ, minimaler Boilerplate |
| 2026-03-16 | Encryption Key: **Auto-generiertes Keyfile** (`goflux.key`) | Einfachste UX für Techniker, Key getrennt von Credentials, AES-256-GCM |
| 2026-03-16 | Function-Node Scripting: **Dual-Mode** – JavaScript (Goja) + expr-lang | JS für komplexe Logik, expr-lang für schnelle Ausdrücke mit Runtime-Kompilierung |
| 2026-03-16 | Projektname: **Flint** (Edge App) + **Flint Hub** (Management Platform) | Kurz, einprägsam, industriell – Feuerstein als Metapher für klein, hart, zuverlässig |
| 2026-03-16 | Frontend embedded: **go:embed** | Single Binary, keine Abhängigkeiten, maximale Einfachheit für Techniker |
| 2026-03-16 | Storage: **JSON-Files** | Einfach, lesbar, kein Setup – wie Node-RED |
| 2026-03-16 | Flow-Format: **Eigenes Flint-Format** | Sauber designt, typisiert, erweiterbar, keine Kompromisse durch Node-RED Kompatibilität |
| 2026-03-16 | Zielplattformen: **Alle** | Linux x64/ARM64/ARM32, Windows x64, macOS – Go Cross-Compile out-of-the-box |
| 2026-03-16 | Lizenz Flint Hub: **Proprietär** | Closed Source, volle Kontrolle, klares kommerzielles Modell |
| 2026-03-16 | NATS Hybrid-Modell | LeafNode+KV+JetStream für Hub/Context/Debug – Go Channels für internes Node/Flow Handling |
| 2026-03-17 | Node-Ports: **Single Input, Multiple Outputs** | Klares Signalfluss-Modell wie in SPS/FBD – kein Merge-Ambiguity |
| 2026-03-17 | **Universelles Node-Debugging** | Jeder Node kann per Icon in Debug-Modus versetzt werden – zeigt IN + OUT Messages |
| 2026-03-17 | **Wire-Debugging** | Auch einzelne Signalleitungen können gedebugt werden – präzise Fehlersuche ohne Extra-Nodes |
| 2026-03-17 | **Namensgebung: Workspace → Flow** | Ein **Workspace** ist der Top-Level-Container (ersetzt das mehrdeutige "flows" aus Node-RED). Ein Workspace enthält mehrere **Flows** (Tabs). Klare Hierarchie ohne Verwechslungsgefahr. |
| 2026-03-17 | **Message: Freie `map[string]any` + unveränderliche `_id`** | Alle Felder gleichberechtigt wie in Node-RED. `_id` wird bei Erstellung gesetzt und ist per Set/Delete geschützt. Clone erzeugt neue `_id`. |

---

## Nächster Schritt

**→ Alle Entscheidungen getroffen! ✅**
**→ Phase 1 kann beginnen: Projekt-Skeleton aufsetzen.**
