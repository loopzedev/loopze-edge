// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"sync"
	"time"
)

// Entry is the cached last value for a widget. The hub replays this to
// every newly connected client so dashboards are never blank after a
// refresh.
type Entry struct {
	Value any       `json:"value"`
	TS    time.Time `json:"ts"`
}

// Cache stores the latest value per widget ID. PR 4 will add a chart
// window cache alongside this last-value cache.
type Cache struct {
	mu      sync.RWMutex
	entries map[string]Entry
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{entries: make(map[string]Entry)}
}

// Put records the latest value for a widget.
func (c *Cache) Put(widgetID string, value any, ts time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.entries[widgetID] = Entry{Value: value, TS: ts}
}

// Get returns the cached entry for a widget, if any.
func (c *Cache) Get(widgetID string) (Entry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	e, ok := c.entries[widgetID]
	return e, ok
}

// Snapshot returns a copy of the entire cache. Safe to send to clients.
func (c *Cache) Snapshot() map[string]Entry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make(map[string]Entry, len(c.entries))
	for k, v := range c.entries {
		out[k] = v
	}
	return out
}

// Drop removes a single widget's cached value.
func (c *Cache) Drop(widgetID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.entries, widgetID)
}

// Retain drops cached values for widgets not present in the keep set.
// Called after a deploy when nodes are removed.
func (c *Cache) Retain(keep map[string]struct{}) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for id := range c.entries {
		if _, ok := keep[id]; !ok {
			delete(c.entries, id)
		}
	}
}

// Size returns the number of cached entries.
func (c *Cache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return len(c.entries)
}
