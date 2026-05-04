// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

// ResponseHandle is the opaque token placed on msg.res by an http-in node
// so that a paired http-response node can locate the matching request in
// the engine's ResponseRegistry. The internal id is intentionally not
// exported — callers must not synthesise their own handles.
//
// MarshalJSON renders the handle as a literal placeholder string. The
// underlying http.ResponseWriter is not serialisable; the placeholder
// keeps Debug-node output and JSON snapshots usable without crashing,
// and prevents accidental leakage of internal state.
type ResponseHandle struct {
	id string
}

// ID returns the registry key the engine uses to resolve this handle.
func (h *ResponseHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.id
}

// MarshalJSON yields the literal placeholder string. Read by Debug
// viewers, COWClone JSON snapshots, and anything else that calls
// json.Marshal on a Message that carries the handle.
func (h *ResponseHandle) MarshalJSON() ([]byte, error) {
	return []byte(`"<response handle>"`), nil
}

// String mirrors MarshalJSON without the surrounding quotes — useful in
// log lines and error messages.
func (h *ResponseHandle) String() string {
	return "<response handle>"
}
