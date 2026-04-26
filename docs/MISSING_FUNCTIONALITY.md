# Fehlende Funktionalität für Node-RED-Parität

Fokus auf Plattform-Features, nicht einzelne Node-Typen.

## 1. Flow Editor (Frontend)

### 1.1 Subflows
- [ ] Subflow erstellen (Gruppe von Nodes als wiederverwendbaren Subflow kapseln)
- [ ] Subflow-Instanzen in anderen Flows verwenden
- [ ] Subflow-Properties (konfigurierbare Parameter pro Instanz)
- [ ] Subflow-Environment-Variables
- [ ] Subflow-Status-Anzeige

### 1.2 Node-Editing
- [ ] Node Enable/Disable (einzelne Nodes deaktivieren ohne Löschen)
- [ ] Node-Gruppen (visuelle Gruppierung mit Rahmen/Kommentar)
- [ ] Comment-Nodes (reine Kommentar-Blöcke im Flow)
- [ ] Wires: Bend-Points / Link-Routing
- [ ] Multi-Select + Bulk-Operationen (Enable/Disable/Delete)
- [ ] Node-Suche im Flow (Ctrl+F → Node finden und fokussieren)

### 1.3 Clipboard & Import/Export
- [ ] Flow Export als JSON (einzelner Flow oder Workspace)
- [ ] Flow Import aus JSON (Clipboard oder Datei)
- [ ] Node/Flow-Snippets aus Zwischenablage einfügen
- [ ] Export als Bild (PNG/SVG des Flows)
- [ ] Library: Flows/Subflows in lokaler Bibliothek speichern & wiederverwenden

### 1.4 Editor UX
- [ ] Undo/Redo (Ctrl+Z / Ctrl+Y)
- [ ] Minimap / Übersichtskarte des Flows
- [ ] Keyboard-Shortcuts (komplett: Quick-Add, Wires, Navigation)
- [ ] Info-Sidebar (Markdown-Dokumentation pro Node/Flow anzeigen)
- [ ] Drag & Drop: Nodes auf bestehende Wires einfügen
- [ ] Junction-Node (Wire-Splitter zum Aufräumen von Verbindungen)
- [ ] Flow-Reihenfolge per Drag ändern (Tabs umsortieren)
- [ ] Auto-Align / Grid-Snap für Nodes
- [ ] Zoom-to-Fit

### 1.5 Konfiguration im Editor
- [ ] Config-Node Editor UI (MQTT-Broker etc. im Property-Panel bearbeiten)
- [ ] Config-Node Übersicht (alle Config-Nodes auflisten, ungenutzte finden)
- [ ] Environment-Variables Editor (Flow- und Global-Scope im UI setzen)

## 2. Runtime / Flow Engine

### 2.1 Deployment-Modi
- [x] Partial Deploy (nur geänderte Flows/Nodes neu deployen, nicht alles)
- [x] Modified Flows Deploy (nur Flows mit Änderungen neu starten)
- [ ] Deploy-Diff anzeigen (was hat sich seit letztem Deploy geändert)

### 2.2 Error Handling
- [ ] Catch-Node (Errors eines Flows abfangen und verarbeiten)
- [ ] Status-Node (Status-Änderungen anderer Nodes als Messages empfangen)
- [ ] Unhandled Error Reporting (Flows ohne Catch-Node warnen)

### 2.3 Message Routing
- [ ] Switch-Node (Routing basierend auf Bedingungen → verschiedene Outputs)
- [ ] Split/Join (Messages aufteilen und wieder zusammenführen)
- [x] Delay-Node (Verzögerung, Rate-Limiting, Queue) – siehe `docs/issues/NODE_DELAY.md`
- [ ] Trigger-Node (Debounce, Throttle, Watchdog-Timer)
- [ ] Filter/RBE-Node (Report by Exception – nur bei Änderung weiterleiten)

### 2.4 Subflow-Runtime
- [ ] Subflow-Instanz-Isolation (eigener Kontext pro Instanz)
- [ ] Subflow-Environment-Variable-Auflösung
- [ ] Subflow In/Out Message-Routing

### 2.5 Context Storage
- [x] Persistent Context (überlebt Neustart – aktuell nur Memory via NATS)
- [x] Context Store Konfiguration (Memory vs. File vs. externe DB)
- [ ] Context Viewer im Editor (aktuelle Werte inspizieren)

## 3. Administration & Management

### 3.1 Authentifizierung & Autorisierung
- [ ] User-Login (Username/Password)
- [ ] Token-basierte API-Authentifizierung
- [ ] Rollen/Berechtigungen (Admin vs. Read-Only vs. Editor)
- [ ] Editor-Zugriffsschutz (UI nur nach Login)
- [ ] Admin API absichern

### 3.2 Projekte (Projects Feature)
- [ ] Git-Integration (Flow-Files in Git-Repo verwalten)
- [ ] Projekt-Wechsel im Editor
- [ ] Branch/Merge-Support
- [ ] Projekt-Dependencies (npm-Pakete für Function-Nodes)
- [ ] Versionshistorie im Editor anzeigen

### 3.3 Multi-User / Collaboration
- [ ] Concurrent Editing Awareness (wer bearbeitet gerade was)
- [ ] Konflikt-Erkennung bei gleichzeitigem Deploy
- [ ] Audit-Log (wer hat wann was deployed)

## 4. Monitoring & Observability

### 4.2 Logging
- [ ] Konfigurierbare Log-Level pro Node
- [ ] Log-Rotation / Log-Archivierung
- [ ] Strukturiertes Logging (JSON-Format für externe Tools)
- [ ] Syslog/External-Logging-Integration

### 4.3 Health & Diagnostics
- [ ] Health-Check Endpoint (`/health`, `/ready`)
- [ ] Runtime-Info-Endpoint (Version, Uptime, Node-Count, Memory)
- [ ] Metrics-Endpoint (Prometheus-kompatibel)

## 5. API & Integration

### 5.1 Admin API
- [ ] Vollständige REST Admin API (Node-RED kompatibel oder eigene)
  - [ ] GET/PUT/DELETE einzelne Flows
  - [ ] GET/PUT Global Flow Config
  - [ ] GET Node Catalog
  - [ ] POST Inject Trigger ✅ (vorhanden)
  - [ ] GET/POST Context Values
- [ ] API-Dokumentation (Swagger/OpenAPI)

### 5.2 Runtime API
- [ ] HTTP-In/Out Nodes (HTTP-Endpoints im Flow definieren)
- [ ] WebSocket-In/Out Nodes (eigene WS-Endpoints im Flow)
- [ ] TCP/UDP In/Out Nodes
- [ ] Webhook-Support

## 6. Konfiguration & Betrieb

### 6.1 Settings
- [ ] settings.js Äquivalent (Runtime-Konfiguration)
- [ ] Theme-Konfiguration (Custom CSS, Logo)
- [ ] Editor-Settings persistent speichern (Grid, Zoom, Panel-Layout)
- [ ] Dashboard-Titel konfigurierbar

### 6.3 Internationalisierung
- [ ] i18n für Editor-UI
- [ ] Node-Beschreibungen in mehreren Sprachen

## 7. Sicherheit

- [ ] HTTPS/TLS-Support (nativ, nicht nur via Reverse-Proxy)
- [ ] Credential-Handling im Editor (Passwort-Felder maskiert)
- [ ] Content-Security-Policy Headers
- [ ] Rate-Limiting auf API-Endpoints
- [ ] Input-Validation / Sanitization auf Admin-API

## 8. Skalierung & High Availability
- [ ] Persistent Message Queues (Messages bei Restart nicht verlieren)

---

## Prioritäts-Einschätzung

### P0 – Kern-Features für Produktivbetrieb
- Error Handling (Catch/Status/Complete Nodes)
- Switch-Node (Message Routing)
- Subflows
- Undo/Redo
- Import/Export
- HTTP-In/Out Nodes
- Node Enable/Disable
- Health-Check Endpoint
- Authentifizierung (mindestens Basic Auth)

### P1 – Wichtig für ernsthafte Nutzung
- Partial Deploy
- Split/Join, Delay, Trigger Nodes
- Context Viewer
- Config-Node Editor UI
- Keyboard-Shortcuts
- Persistent Context Storage
- Metrics/Monitoring
- Plugin-System / Custom Node API

### P2 – Nice-to-Have / Differenzierung
- Projects/Git-Integration
- Multi-User
- Cluster-Modus
- i18n
- Palette Manager
- Flow-Export als Bild
