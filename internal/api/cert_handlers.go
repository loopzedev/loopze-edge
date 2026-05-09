// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/loopzedev/loopze-edge/internal/credentials"
	"github.com/loopzedev/loopze-edge/internal/flow"
)

// handleListCerts returns every cert entry as a wire-safe summary, sorted
// by ID. PEM material and private keys are intentionally omitted; only
// file paths and parsed metadata are surfaced.
//
// GET /api/v1/certs
func (d *Deps) handleListCerts(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	jsonResponse(w, http.StatusOK, map[string]any{
		"certs": d.Certs.List(),
	})
}

// handleGetCert returns the summary for a single cert entry.
//
// GET /api/v1/certs/{id}
func (d *Deps) handleGetCert(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	id := chi.URLParam(r, "id")
	entry, ok := d.Certs.Get(id)
	if !ok {
		jsonError(w, http.StatusNotFound, "cert not found")
		return
	}
	summary := entrySummaryView(entry)
	jsonResponse(w, http.StatusOK, summary)
}

// handleCreateCert stores a new cert entry. The request body must conform
// to credentials.CertEntry (ID, Name, Type, Source, plus PEM or path
// fields). Validation and PEM parsing run inside the store; failures are
// returned as 400 / 409.
//
// POST /api/v1/certs
func (d *Deps) handleCreateCert(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	var entry credentials.CertEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	stored, err := d.Certs.Store(entry)
	if err != nil {
		writeCertStoreError(w, err)
		return
	}
	slog.Info("cert created via API", "id", stored.ID, "type", stored.Type, "source", stored.Source)
	jsonResponse(w, http.StatusCreated, entrySummaryView(stored))
}

// handleUpdateCert replaces the cert entry at {id}. The URL path ID wins;
// any ID field on the request body is ignored.
//
// PUT /api/v1/certs/{id}
func (d *Deps) handleUpdateCert(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	id := chi.URLParam(r, "id")
	var entry credentials.CertEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	updated, err := d.Certs.Update(id, entry)
	if err != nil {
		writeCertStoreError(w, err)
		return
	}
	slog.Info("cert updated via API", "id", id, "fingerprint", updated.Fingerprint)
	jsonResponse(w, http.StatusOK, entrySummaryView(updated))
}

// handleDeleteCert removes a cert entry. Before deleting, the workspace
// is scanned for references; if any node still references the cert, the
// request is rejected with 409 Conflict and a list of referencing nodes
// so the operator can untangle the dependency before retrying.
//
// DELETE /api/v1/certs/{id}
func (d *Deps) handleDeleteCert(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	id := chi.URLParam(r, "id")

	// Reference scan against the persisted workspace. Using the saved
	// workspace (rather than just the live engine state) means a stopped
	// flow that still holds the reference is also surfaced.
	if d.Storage != nil {
		ws, err := d.Storage.LoadWorkspace()
		if err != nil {
			slog.Error("cert delete: load workspace", "error", err)
			jsonError(w, http.StatusInternalServerError, "failed to scan workspace for references")
			return
		}
		if refs := flow.ScanCertReferences(ws, id); len(refs) > 0 {
			jsonResponse(w, http.StatusConflict, map[string]any{
				"error":      http.StatusText(http.StatusConflict),
				"message":    "cert is still referenced by deployed nodes",
				"references": refs,
			})
			return
		}
	}

	if err := d.Certs.Delete(id); err != nil {
		writeCertStoreError(w, err)
		return
	}
	slog.Info("cert deleted via API", "id", id)
	w.WriteHeader(http.StatusNoContent)
}

// handleValidateCert parses the supplied entry and returns the derived
// metadata WITHOUT persisting it. Used by the frontend "test before
// save" affordance so operators can verify a PEM or path before
// committing it to the store.
//
// POST /api/v1/certs/validate
func (d *Deps) handleValidateCert(w http.ResponseWriter, r *http.Request) {
	if d.Certs == nil {
		jsonError(w, http.StatusServiceUnavailable, "cert store not configured")
		return
	}
	var entry credentials.CertEntry
	if err := json.NewDecoder(r.Body).Decode(&entry); err != nil {
		jsonError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}
	// Avoid colliding with whatever ID the caller picked; we never
	// persist this entry. A fixed slug-shaped placeholder satisfies
	// CertEntry.Validate() without polluting the store namespace.
	if entry.ID == "" {
		entry.ID = "validate-probe"
	}
	parsed, err := credentials.ProbeCertEntry(entry)
	if err != nil {
		writeCertStoreError(w, err)
		return
	}
	jsonResponse(w, http.StatusOK, entrySummaryView(parsed))
}

// entrySummaryView returns the public, PEM-free view of a CertEntry. The
// CertStore.List path already returns CertEntrySummary; this helper is
// used by the single-entry endpoints to apply the same projection.
func entrySummaryView(e credentials.CertEntry) credentials.CertEntrySummary {
	return credentials.CertEntrySummary{
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

// writeCertStoreError maps cert-store sentinel errors to HTTP statuses
// without leaking internal error wrapping prose to the operator.
func writeCertStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, credentials.ErrCertNotFound):
		jsonError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, credentials.ErrCertExists):
		jsonError(w, http.StatusConflict, err.Error())
	case errors.Is(err, credentials.ErrCertIDInvalid),
		errors.Is(err, credentials.ErrCertTypeInvalid),
		errors.Is(err, credentials.ErrCertSourceInvalid),
		errors.Is(err, credentials.ErrCertMixedSource),
		errors.Is(err, credentials.ErrCertCertMissing),
		errors.Is(err, credentials.ErrCertKeyMissing),
		errors.Is(err, credentials.ErrCertKeyForbidden),
		errors.Is(err, credentials.ErrCertPathRelative),
		errors.Is(err, credentials.ErrCertPEMInvalid),
		errors.Is(err, credentials.ErrCertKeyMismatch),
		errors.Is(err, credentials.ErrCertTypeMismatch):
		jsonError(w, http.StatusBadRequest, err.Error())
	default:
		slog.Error("cert store error", "error", err)
		jsonError(w, http.StatusInternalServerError, "cert store error")
	}
}
