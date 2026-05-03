// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package logbuffer

import (
	"context"
	"log/slog"
	"sync/atomic"
)

// NotifyFunc is invoked synchronously after each record is appended to the
// buffer. It receives the entry with the assigned Seq. Implementations must
// not block: the buffer's notify call runs inline with the original log
// statement.
type NotifyFunc func(LogEntry)

// Handler decorates an inner slog.Handler so every record is also captured
// into a Buffer and (optionally) forwarded to a notify callback.
//
// The notify callback can be installed after construction with SetNotify,
// allowing the WebSocket hub to be wired in once it exists.
type Handler struct {
	inner  slog.Handler
	buf    *Buffer
	notify atomic.Pointer[NotifyFunc]
}

// NewHandler wraps inner so each record is delegated to it (preserving its
// stdout output) and also captured in buf.
func NewHandler(inner slog.Handler, buf *Buffer) *Handler {
	return &Handler{inner: inner, buf: buf}
}

// SetNotify installs (or removes, with nil) the notify callback. Safe to
// call from any goroutine.
func (h *Handler) SetNotify(fn NotifyFunc) {
	if fn == nil {
		h.notify.Store(nil)
		return
	}
	h.notify.Store(&fn)
}

// Enabled delegates to the inner handler.
func (h *Handler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}

// Handle delegates to the inner handler, then appends a LogEntry to the
// buffer and fires the notify callback if installed.
func (h *Handler) Handle(ctx context.Context, r slog.Record) error {
	if err := h.inner.Handle(ctx, r); err != nil {
		return err
	}
	h.capture(r, nil)
	return nil
}

// WithAttrs returns a child handler that carries the given attributes on
// the inner handler while sharing this handler's buffer and notify pointer.
//
// The attrs are also tracked on the child so capture can include them in
// the LogEntry — slog.Record.Attrs only iterates per-record attrs, not the
// ones accumulated through WithAttrs/With.
func (h *Handler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &childHandler{
		Handler:    h.inner.WithAttrs(attrs),
		parent:     h,
		boundAttrs: cloneAttrs(attrs),
	}
}

// WithGroup returns a child handler that opens the named group on the inner
// handler while sharing this handler's buffer and notify pointer.
//
// Groups are not separately tracked: any attrs added on the resulting
// handler are merged flat into the LogEntry's Attrs map. The text rendering
// preserves the group structure on stdout via the inner handler.
func (h *Handler) WithGroup(name string) slog.Handler {
	return &childHandler{
		Handler: h.inner.WithGroup(name),
		parent:  h,
	}
}

func (h *Handler) capture(r slog.Record, bound []slog.Attr) {
	entry := LogEntry{
		Time:    r.Time,
		Level:   r.Level.String(),
		Message: r.Message,
	}
	var attrs map[string]any
	add := func(a slog.Attr) {
		if attrs == nil {
			attrs = make(map[string]any, 4)
		}
		attrs[a.Key] = a.Value.Resolve().Any()
	}
	for _, a := range bound {
		add(a)
	}
	r.Attrs(func(a slog.Attr) bool {
		add(a)
		return true
	})
	if attrs != nil {
		entry.Attrs = attrs
	}
	stamped := h.buf.Add(entry)
	if p := h.notify.Load(); p != nil {
		(*p)(stamped)
	}
}

// childHandler is the result of WithAttrs / WithGroup. It uses a different
// inner handler (carrying the additional attrs/group) but routes capture
// through the parent so buffer and notify state are shared. boundAttrs
// accumulates the attrs added by WithAttrs along the chain.
type childHandler struct {
	slog.Handler
	parent     *Handler
	boundAttrs []slog.Attr
}

func (c *childHandler) Handle(ctx context.Context, r slog.Record) error {
	if err := c.Handler.Handle(ctx, r); err != nil {
		return err
	}
	c.parent.capture(r, c.boundAttrs)
	return nil
}

func (c *childHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(c.boundAttrs)+len(attrs))
	merged = append(merged, c.boundAttrs...)
	merged = append(merged, attrs...)
	return &childHandler{
		Handler:    c.Handler.WithAttrs(attrs),
		parent:     c.parent,
		boundAttrs: merged,
	}
}

func (c *childHandler) WithGroup(name string) slog.Handler {
	return &childHandler{
		Handler:    c.Handler.WithGroup(name),
		parent:     c.parent,
		boundAttrs: cloneAttrs(c.boundAttrs),
	}
}

func cloneAttrs(in []slog.Attr) []slog.Attr {
	if len(in) == 0 {
		return nil
	}
	out := make([]slog.Attr, len(in))
	copy(out, in)
	return out
}
