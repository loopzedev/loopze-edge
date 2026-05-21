// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package dashboard

import (
	"net/http"

	"github.com/loopzedev/loopze-edge/internal/ws"
)

// anonUserID is the synthetic user attached to dashboard clients that
// connected while ui-base.auth == "none". It shows up on WidgetEvent
// so audit logs can still distinguish "anonymous kiosk" from a logged-in
// operator.
const anonUserID = "anon"

// BuildAuthFunc returns a ws.AuthFunc that gates the dashboard WS
// upgrade according to the current ui-base.auth setting:
//
//   - "session" (default, also used when no ui-base is configured):
//     delegate to sessionAuth, which validates the loopze_session
//     cookie the same way the editor hub does.
//
//   - "none": always accept, returning a synthetic anon user ID so
//     downstream code can still carry it on WidgetClient.UserID.
//
// Locking-free read: hub.AuthMode acquires its own RLock on every call,
// so a live ui-base.auth change takes effect on the next upgrade
// attempt without having to bounce the listener.
func BuildAuthFunc(hub *Hub, sessionAuth ws.AuthFunc) ws.AuthFunc {
	return func(r *http.Request) (string, error) {
		switch hub.AuthMode() {
		case "none":
			return anonUserID, nil
		default: // "session" or unset
			return sessionAuth(r)
		}
	}
}
