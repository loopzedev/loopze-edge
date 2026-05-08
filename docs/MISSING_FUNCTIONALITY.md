# Missing functionality for Node-RED parity

Focus on platform features, not individual node types.

## 1. Flow Editor (frontend)

### 1.1 Subflows
- [ ] Create subflow (encapsulate a group of nodes as a reusable subflow)
- [ ] Use subflow instances in other flows
- [ ] Subflow properties (configurable parameters per instance)
- [ ] Subflow environment variables
- [ ] Subflow status display

### 1.2 Node editing
- [x] Node enable/disable (deactivate individual nodes without deleting) – see `docs/issues/NODE_ENABLE_DISABLE.md`
- [ ] Node groups (visual grouping with frame/comment)
- [ ] Comment nodes (pure comment blocks in the flow)
- [ ] Wires: bend points / link routing
- [ ] Multi-select + bulk operations (enable/disable/delete)
- [ ] Node search in flow (Ctrl+F → find and focus node)

### 1.3 Clipboard & import/export
- [ ] Flow export as JSON (single flow or workspace)
- [ ] Flow import from JSON (clipboard or file)
- [ ] Paste node/flow snippets from clipboard
- [ ] Export as image (PNG/SVG of the flow)
- [ ] Library: save and reuse flows/subflows in a local library

### 1.4 Editor UX
- [ ] Undo/Redo (Ctrl+Z / Ctrl+Y)
- [ ] Minimap / overview map of the flow
- [ ] Keyboard shortcuts (complete: quick-add, wires, navigation)
- [ ] Info sidebar (show Markdown documentation per node/flow)
- [ ] Drag & drop: insert nodes onto existing wires
- [ ] Junction node (wire splitter for tidying up connections)
- [ ] Reorder flows by drag (rearrange tabs)
- [ ] Auto-align / grid snap for nodes
- [ ] Zoom to fit

### 1.5 Configuration in the editor
- [ ] Config node editor UI (edit MQTT broker etc. in the property panel)
- [ ] Config node overview (list all config nodes, find unused ones)
- [ ] Environment variables editor (set flow and global scope in the UI)

## 2. Runtime / flow engine

### 2.1 Deployment modes
- [x] Partial deploy (only redeploy changed flows/nodes, not everything)
- [x] Modified flows deploy (only restart flows with changes)
- [ ] Show deploy diff (what has changed since the last deploy)

### 2.2 Error handling
- [ ] Catch node (catch errors of a flow and process them)
- [ ] Status node (receive status changes of other nodes as messages)
- [ ] Unhandled error reporting (warn for flows without a Catch node)

### 2.3 Message routing
- [ ] Switch node (routing based on conditions → different outputs)
- [ ] Split/Join (split messages and merge them again)
- [x] Delay node (delay, rate limiting, queue) – see `docs/issues/NODE_DELAY.md`
- [ ] Trigger node (debounce, throttle, watchdog timer)
- [ ] Filter/RBE node (report by exception – only forward on change)

### 2.4 Subflow runtime
- [ ] Subflow instance isolation (own context per instance)
- [ ] Subflow environment variable resolution
- [ ] Subflow in/out message routing

### 2.5 Context storage
- [x] Persistent context (survives restart – currently only memory via NATS)
- [x] Context store configuration (memory vs. file vs. external DB)
- [ ] Context viewer in the editor (inspect current values)

## 3. Administration & management

### 3.1 Authentication & authorization
- [ ] User login (username/password)
- [ ] Token-based API authentication
- [ ] Roles/permissions (admin vs. read-only vs. editor)
- [ ] Editor access protection (UI only after login)
- [ ] Secure admin API

### 3.2 Projects (Projects feature)
- [ ] Git integration (manage flow files in a Git repo)
- [ ] Project switching in the editor
- [ ] Branch/merge support
- [ ] Project dependencies (npm packages for Function nodes)
- [ ] Show version history in the editor

### 3.3 Multi-user / collaboration
- [ ] Concurrent editing awareness (who is currently editing what)
- [ ] Conflict detection on simultaneous deploy
- [ ] Audit log (who deployed what when)

## 4. Monitoring & observability

### 4.2 Logging
- [ ] Configurable log level per node
- [ ] Log rotation / log archiving
- [ ] Structured logging (JSON format for external tools)
- [ ] Syslog / external logging integration

### 4.3 Health & diagnostics
- [ ] Health check endpoint (`/health`, `/ready`)
- [ ] Runtime info endpoint (version, uptime, node count, memory)
- [ ] Metrics endpoint (Prometheus-compatible)

## 5. API & integration

### 5.1 Admin API
- [ ] Complete REST admin API (Node-RED compatible or custom)
  - [ ] GET/PUT/DELETE individual flows
  - [ ] GET/PUT global flow config
  - [ ] GET node catalog
  - [ ] POST inject trigger ✅ (available)
  - [ ] GET/POST context values
- [ ] API documentation (Swagger/OpenAPI)

### 5.2 Runtime API
- [ ] HTTP In/Out nodes (define HTTP endpoints in the flow)
- [ ] WebSocket In/Out nodes (own WS endpoints in the flow)
- [ ] TCP/UDP In/Out nodes – see `specifications/issues/NODE_TCP_UDP.md`
- [ ] Webhook support

## 6. Configuration & operation

### 6.1 Settings
- [ ] settings.js equivalent (runtime configuration)
- [ ] Theme configuration (custom CSS, logo)
- [ ] Persistently store editor settings (grid, zoom, panel layout)
- [ ] Configurable dashboard title

### 6.3 Internationalization
- [ ] i18n for editor UI
- [ ] Node descriptions in multiple languages

## 7. Security

- [ ] HTTPS/TLS support (native, not only via reverse proxy)
- [ ] Credential handling in the editor (masked password fields)
- [ ] Content-Security-Policy headers
- [ ] Rate limiting on API endpoints
- [ ] Input validation / sanitization on the admin API

## 8. Scaling & high availability
- [ ] Persistent message queues (do not lose messages on restart)

---

## Priority assessment

### P0 – Core features for production use
- Error handling (Catch/Status/Complete nodes)
- Switch node (message routing)
- Subflows
- Undo/Redo
- Import/Export
- HTTP In/Out nodes
- Node enable/disable
- Health check endpoint
- Authentication (at least basic auth)

### P1 – Important for serious use
- Partial deploy
- Split/Join, Delay, Trigger nodes
- Context viewer
- Config node editor UI
- Keyboard shortcuts
- Persistent context storage
- Metrics/monitoring
- Plugin system / custom node API

### P2 – Nice-to-have / differentiation
- Projects/Git integration
- Multi-user
- Cluster mode
- i18n
- Palette manager
- Flow export as image
