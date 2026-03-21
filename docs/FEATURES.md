# Flint — Features & Abgrenzung zu Node-RED

Flint ist kein Fork, sondern ein Neuaufbau mit den Lehren aus Node-RED. Gleiche Philosophie (visual flow programming), aber mit bewussten Designentscheidungen die wiederkehrende Schmerzpunkte loesen.

---

## Message Models via Function Nodes

In Node-RED sind Messages untypisierte `msg`-Objekte — jeder Node kann beliebige Felder setzen oder weglassen. Das fuehrt in groesseren Flows schnell zu inkonsistenten Payloads, die erst zur Laufzeit auffallen.

In Flint koennen Function Nodes **Models definieren**: ein JSON-Schema das beschreibt wie die ausgehende Message aussieht. Sobald ein Model definiert ist, garantiert der Function Node strukturell konsistente Outputs. Nachfolgende Nodes koennen sich darauf verlassen welche Felder existieren und welchen Typ sie haben — kein defensives `if (msg.payload && msg.payload.temperature)` mehr.

---

## Validation Node

Ein dedizierter **Validation Node** prueft eingehende Messages gegen ein definiertes Model. Messages die dem Schema entsprechen werden weitergeleitet, nicht-konforme Messages werden verworfen (oder auf einen separaten Error-Output geroutet). Das ermoeglicht explizite Datenvertraege zwischen Flow-Abschnitten — besonders wertvoll an Systemgrenzen wo externe Daten (MQTT, HTTP, Sensoren) reinkommen und nicht blind vertraut werden sollte.

---

## Native Queuing ueber NATS Streams

Node-RED hat kein eingebautes Queuing. Wer Messages puffern, wiederholen oder persistent zwischenspeichern will, braucht externe Systeme (Redis, RabbitMQ) oder fragile Workarounds mit Context-Variablen.

Flint bringt einen eingebetteten NATS-Server mit JetStream mit. Ein nativer **Queue Node** kann Messages in einen NATS Stream schreiben und mit konfigurierbarer Delivery-Garantie (at-least-once, exactly-once) wieder konsumieren. Retry-Logik, Dead-Letter-Queues und Backpressure sind damit Bordmittel — kein externer Broker, kein Plugin, eine einzige Binary.

---

## Node Profiling & Durchsatz-Metriken

Node-RED bietet keine Moeglichkeit zu sehen welcher Node wie lange braucht oder wo ein Bottleneck sitzt. Man merkt erst dass etwas langsam ist, aber nicht wo.

Flint misst **pro Node die Verarbeitungszeit und den Durchsatz**. Im Editor kann ein Profiling-Overlay eingeblendet werden das direkt auf den Nodes zeigt: durchschnittliche Latenz, Messages pro Sekunde, Queue-Fuellstand. Langsame Nodes werden visuell hervorgehoben. Das macht Performance-Probleme sichtbar bevor sie kritisch werden — ohne externe Monitoring-Tools.

---


## Goroutine-per-Node Parallelitaet ✅

> Bereits implementiert — jeder Node laeuft in seiner eigenen Goroutine (`engine.go: nodeLoop`).

Node-RED laeuft single-threaded auf Node.js. Ein langsamer Function Node blockiert den gesamten Event-Loop — alle anderen Nodes warten.

Flint fuehrt **jeden Node in einer eigenen Goroutine** aus. CPU-intensive Berechnungen in einem Function Node blockieren keine anderen Nodes. Die Go-Runtime verteilt die Arbeit automatisch ueber alle CPU-Kerne. Tausende Nodes laufen echt parallel, nicht kooperativ-sequentiell.

---

## Single Binary, Zero Dependencies ✅

> Bereits implementiert — Go-Binary mit eingebettetem Frontend (`web.Embed`) und embedded NATS-Server.

Node-RED braucht Node.js, npm und ein Dateisystem voller `node_modules`. Auf einem frischen System ist die Installation ein Prozess mit mehreren Schritten und potentiellen Versionskonflikten.

Flint ist **eine einzige ausfuehrbare Datei**. Kein Node.js, kein npm, keine externen Abhaengigkeiten. Download, ausfuehren, fertig. Das Frontend ist in die Binary eingebettet, der NATS-Server laeuft embedded. Besonders auf Edge-Devices und in eingeschraenkten Umgebungen (kein Internet, kein Paketmanager) ist das ein entscheidender Vorteil.

---

## Reaktiver Context Store — Events statt Polling ✅

> Bereits implementiert — Context Watch Node basiert auf NATS KV Watcher (`context_watch.go`).

In Node-RED ist der Context Store (Flow/Global Context) ein passiver Key-Value-Speicher. Man kann Werte lesen und schreiben, aber es gibt keine Moeglichkeit **benachrichtigt zu werden wenn sich ein Wert aendert**. Das fuehrt zu einem fundamentalen Widerspruch: Node-RED ist eine event-basierte Plattform, aber der zentrale Zustandsspeicher ist poll-basiert.

In der Praxis erzwingt das anti-patterns:
- **Polling-Loops**: Inject Nodes die alle 500ms den Context lesen und pruefen ob sich etwas geaendert hat — CPU-Last ohne Mehrwert
- **Redundante Verdrahtung**: Jeder Node der einen Context-Wert aendert muss zusaetzlich eine Message an alle interessierten Nodes schicken — doppelte Logik, fragile Flows
- **Race Conditions**: Zwischen zwei Poll-Zyklen kann ein Wert mehrfach geaendert worden sein — Zwischenzustaende gehen verloren

Flint loest das durch einen **reaktiven Context Store auf Basis von NATS JetStream KV**. Der Context Watch Node subscribt auf Aenderungen an bestimmten Keys oder Key-Patterns und feuert automatisch eine Message wenn sich ein Wert aendert — in Echtzeit, ohne Polling. Der Context wird damit zum vollwertigen Event-Source:

```
[Sensor] → [Change: set flow.temperature]
                                            → Context Watch (flow.temperature) → [Debug]
[HTTP In] → [Change: set flow.temperature]  ↗
```

Egal welcher Node den Wert aendert — der Context Watch reagiert sofort. Das eliminiert Polling komplett und haelt Flows sauber event-basiert.

---

## Native Industrie-Connectoren — kein Community-Roulette

Node-RED liefert von Haus aus keine Industrie-Protokolle mit. OPC-UA, Modbus, S7, MQTT mit Sparkplug B — alles muss ueber Community-Module nachinstalliert werden. Das funktioniert anfangs, fuehrt aber in der Praxis zu ernsthaften Problemen:

- **Verwaiste Module**: Der Maintainer verliert das Interesse, das Modul bekommt keine Updates mehr. Sicherheitsluecken bleiben offen, Kompatibilitaet mit neuen Node-RED Versionen bricht.
- **Qualitaetsschwankungen**: Fuer dasselbe Protokoll gibt es oft 3-5 Module mit unterschiedlicher Reife, Dokumentation und Fehlerbehandlung. Die Auswahl wird zum Gluecksspiel.
- **Abhaengigkeitsketten**: Community-Module bringen eigene npm-Dependencies mit die mit anderen Modulen kollidieren koennen. Ein `npm install` kann bestehende Flows brechen.
- **Kein einheitliches Config-Pattern**: Jedes Modul erfindet sein eigenes UI fuer Verbindungseinstellungen. Mal gibt es Reconnect-Logik, mal nicht. Mal werden Credentials verschluesselt, mal im Klartext gespeichert.

Flint loest das durch **native Industrie-Connectoren die fest im Produkt verankert sind**:

| Protokoll | Typ | Beschreibung |
|---|---|---|
| **MQTT** | Data Connector | v3.1.1 und v5, geteilte Broker-Verbindungen |
| **OPC-UA** | Industrial | Client fuer SPS- und SCADA-Anbindung |
| **Modbus** | Industrial | TCP/RTU, Read/Write Coils und Register |
| **HTTP** | Data Connector | Request/Response und Webhook-Endpoints |
| **TCP/UDP** | Data Connector | Raw Socket Kommunikation |
| **S7** | Industrial | Siemens S7-Protokoll fuer S7-300/400/1200/1500 |
| **Datenbanken** | Storage | PostgreSQL, SQLite, InfluxDB |

Alle Connectoren werden mit der Produktentwicklung mitgepflegt — gleiche Testabdeckung, gleiche Release-Zyklen, gleiche Qualitaetsstandards. Sie nutzen das einheitliche **Config Node Plugin-System** (geteilte Verbindungen, automatischer Reconnect, Status-Broadcast), sodass sich alle Connectoren konsistent verhalten. Kein `npm install` das nach 18 Monaten zum Risiko wird.

---

## Live-Debugging mit Message Tracing

Node-REDs Debug-Node zeigt Messages in einer separaten Sidebar — aber man sieht nicht welchen Weg eine Message durch den Flow genommen hat.

Flint ermoeglicht **Message Tracing**: eine einzelne Message kann visuell durch den Flow verfolgt werden. Der Pfad den die Message genommen hat wird im Canvas hervorgehoben, mit Timestamps und Payload-Snapshots an jedem Node. Das macht das Debugging komplexer Flows mit Verzweigungen, Filtern und Cross-Flow-Links nachvollziehbar.


