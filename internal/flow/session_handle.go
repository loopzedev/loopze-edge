// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package flow

// SessionHandle is the opaque token placed on msg.session by a tcp-in
// (server-mode) node so that a paired tcp-out (reply-mode) node can
// locate the matching net.Conn in the engine's SessionRegistry. The
// internal id is intentionally not exported — callers must not
// synthesise their own handles.
//
// MarshalJSON renders the handle as a literal placeholder string. The
// underlying net.Conn is not serialisable; the placeholder keeps
// Debug-node output and JSON snapshots usable without crashing, and
// prevents accidental leakage of internal state.
type SessionHandle struct {
	id string
}

// NewSessionHandle is exposed for use by the SessionRegistry; external
// callers should obtain handles through SessionRegistry.Register.
func NewSessionHandle(id string) *SessionHandle {
	return &SessionHandle{id: id}
}

// ID returns the registry key the engine uses to resolve this handle.
func (h *SessionHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.id
}

// MarshalJSON yields the literal placeholder string. Read by Debug
// viewers, COWClone JSON snapshots, and anything else that calls
// json.Marshal on a Message that carries the handle.
func (h *SessionHandle) MarshalJSON() ([]byte, error) {
	return []byte(`"<tcp session>"`), nil
}

// String mirrors MarshalJSON without the surrounding quotes — useful in
// log lines and error messages.
func (h *SessionHandle) String() string {
	return "<tcp session>"
}
