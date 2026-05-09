// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import "time"

// CertEntrySummary is the wire-safe view of a CertEntry. It intentionally
// omits CertPEM and KeyPEM so HTTP responses cannot leak key material —
// only file paths are exposed, since operators legitimately need to see
// where a file-source entry points.
//
// All other fields, including the parsed metadata, are mirrored verbatim
// from CertEntry.
type CertEntrySummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Source string `json:"source"`

	CertPath string `json:"certPath,omitempty"`
	KeyPath  string `json:"keyPath,omitempty"`

	Fingerprint string    `json:"fingerprint,omitempty"`
	Subject     string    `json:"subject,omitempty"`
	Issuer      string    `json:"issuer,omitempty"`
	NotBefore   time.Time `json:"notBefore,omitempty"`
	NotAfter    time.Time `json:"notAfter,omitempty"`

	Notes     string    `json:"notes,omitempty"`
	CreatedAt time.Time `json:"createdAt,omitempty"`
	UpdatedAt time.Time `json:"updatedAt,omitempty"`
}

// summary returns the wire-safe view of e.
func (e *CertEntry) summary() CertEntrySummary {
	return CertEntrySummary{
		ID:          e.ID,
		Name:        e.Name,
		Type:        e.Type,
		Source:      e.Source,
		CertPath:    e.CertPath,
		KeyPath:     e.KeyPath,
		Fingerprint: e.Fingerprint,
		Subject:     e.Subject,
		Issuer:      e.Issuer,
		NotBefore:   e.NotBefore,
		NotAfter:    e.NotAfter,
		Notes:       e.Notes,
		CreatedAt:   e.CreatedAt,
		UpdatedAt:   e.UpdatedAt,
	}
}
