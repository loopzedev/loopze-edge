// Copyright (C) 2026 Dennis Bleul
// Licensed under the GNU Affero General Public License v3.0 or later.
// See LICENSE file for details.

package credentials

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

// Source values for CertEntry.
const (
	SourceInline = "inline"
	SourceFile   = "file"
)

// Type values for CertEntry.
const (
	TypeCABundle   = "ca-bundle"
	TypeClientPair = "client-pair"
	TypeServerPair = "server-pair"
)

// slugRegexp validates user-defined cert IDs. The format is intentionally
// restrictive so IDs are safe in URLs, filenames, and flow-JSON references.
var slugRegexp = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

// Validation errors returned by CertEntry.Validate / parseAndPopulate.
var (
	ErrCertIDInvalid     = errors.New("credentials: cert ID must match ^[a-z0-9][a-z0-9_-]{0,63}$")
	ErrCertTypeInvalid   = errors.New("credentials: cert type must be ca-bundle, client-pair, or server-pair")
	ErrCertSourceInvalid = errors.New("credentials: cert source must be inline or file")
	ErrCertMixedSource   = errors.New("credentials: PEM and path fields are mutually exclusive per source")
	ErrCertCertMissing   = errors.New("credentials: certificate material is required")
	ErrCertKeyMissing    = errors.New("credentials: client-pair and server-pair require a private key")
	ErrCertKeyForbidden  = errors.New("credentials: ca-bundle must not have a private key")
	ErrCertPathRelative  = errors.New("credentials: file source paths must be absolute")
	ErrCertPEMInvalid    = errors.New("credentials: PEM data could not be parsed")
	ErrCertKeyMismatch   = errors.New("credentials: certificate and private key do not match")
)

// CertEntry describes a single certificate (or CA bundle) tracked by the
// CertStore. It supports two source modes:
//
//   - Source == "inline": cert and (optionally) key live in CertPEM / KeyPEM
//     and are stored encrypted at rest as part of credentials.json.
//   - Source == "file": only CertPath / KeyPath are persisted; the actual
//     PEM material is read from disk on demand. This lets external tools
//     (cert-manager, Let's Encrypt, OPC UA) rotate the cert files without
//     touching the store.
//
// Fingerprint, Subject, Issuer, NotBefore, NotAfter are derived from the
// leaf certificate by parseAndPopulate at Store / Update time. They are
// also persisted so List() can present them without re-reading files.
type CertEntry struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Type   string `json:"type"`
	Source string `json:"source"`

	CertPEM string `json:"certPem,omitempty"`
	KeyPEM  string `json:"keyPem,omitempty"`

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

// requiresKey reports whether the entry's type requires a private key.
func (e *CertEntry) requiresKey() bool {
	return e.Type == TypeClientPair || e.Type == TypeServerPair
}

// Validate runs schema checks only — slug format, enum membership, and
// mutual exclusion of PEM vs. path fields. It performs no I/O and does
// not parse PEM data; that is parseAndPopulate's job.
func (e *CertEntry) Validate() error {
	if !slugRegexp.MatchString(e.ID) {
		return ErrCertIDInvalid
	}

	switch e.Type {
	case TypeCABundle, TypeClientPair, TypeServerPair:
	default:
		return ErrCertTypeInvalid
	}

	switch e.Source {
	case SourceInline:
		if e.CertPath != "" || e.KeyPath != "" {
			return ErrCertMixedSource
		}
		if e.CertPEM == "" {
			return ErrCertCertMissing
		}
		if e.requiresKey() && e.KeyPEM == "" {
			return ErrCertKeyMissing
		}
		if !e.requiresKey() && e.KeyPEM != "" {
			return ErrCertKeyForbidden
		}
	case SourceFile:
		if e.CertPEM != "" || e.KeyPEM != "" {
			return ErrCertMixedSource
		}
		if e.CertPath == "" {
			return ErrCertCertMissing
		}
		if !filepath.IsAbs(e.CertPath) {
			return ErrCertPathRelative
		}
		if e.requiresKey() {
			if e.KeyPath == "" {
				return ErrCertKeyMissing
			}
			if !filepath.IsAbs(e.KeyPath) {
				return ErrCertPathRelative
			}
		} else if e.KeyPath != "" {
			return ErrCertKeyForbidden
		}
	default:
		return ErrCertSourceInvalid
	}

	return nil
}

// loadMaterial returns the cert and (when applicable) key PEM bytes for the
// entry, reading from disk for file sources and from the inline strings
// otherwise. The caller is expected to have called Validate first.
func (e *CertEntry) loadMaterial() (certPEM, keyPEM []byte, err error) {
	switch e.Source {
	case SourceInline:
		certPEM = []byte(e.CertPEM)
		if e.requiresKey() {
			keyPEM = []byte(e.KeyPEM)
		}
	case SourceFile:
		certPEM, err = os.ReadFile(e.CertPath)
		if err != nil {
			return nil, nil, fmt.Errorf("credentials: failed to read certPath %q: %w", e.CertPath, err)
		}
		if e.requiresKey() {
			keyPEM, err = os.ReadFile(e.KeyPath)
			if err != nil {
				return nil, nil, fmt.Errorf("credentials: failed to read keyPath %q: %w", e.KeyPath, err)
			}
		}
	default:
		return nil, nil, ErrCertSourceInvalid
	}
	return certPEM, keyPEM, nil
}

// parseAndPopulate loads the cert material, parses the leaf certificate,
// verifies cert/key match for *-pair types, and fills the derived metadata
// fields (Fingerprint, Subject, Issuer, NotBefore, NotAfter). Validate
// must have been called first.
func (e *CertEntry) parseAndPopulate() error {
	certPEM, keyPEM, err := e.loadMaterial()
	if err != nil {
		return err
	}

	leaf, err := parseLeafCertificate(certPEM)
	if err != nil {
		return err
	}

	if e.requiresKey() {
		if _, err := tls.X509KeyPair(certPEM, keyPEM); err != nil {
			return fmt.Errorf("%w: %v", ErrCertKeyMismatch, err)
		}
	}

	sum := sha256.Sum256(leaf.Raw)
	e.Fingerprint = hex.EncodeToString(sum[:])
	e.Subject = leaf.Subject.String()
	e.Issuer = leaf.Issuer.String()
	e.NotBefore = leaf.NotBefore
	e.NotAfter = leaf.NotAfter
	return nil
}

// ProbeCertEntry runs the same validation and parse pipeline that Store
// applies, but without persisting the result. Useful for "test before
// save" affordances on the API so operators can preview the parsed
// metadata (fingerprint, subject, expiry) of a PEM blob or file path
// before committing it to the store.
//
// On success, the returned entry has its derived metadata fields
// populated; CreatedAt / UpdatedAt are NOT set.
func ProbeCertEntry(entry CertEntry) (CertEntry, error) {
	if err := entry.Validate(); err != nil {
		return CertEntry{}, err
	}
	if err := entry.parseAndPopulate(); err != nil {
		return CertEntry{}, err
	}
	return entry, nil
}

// parseLeafCertificate scans pemData for the first CERTIFICATE block and
// parses it. Subsequent blocks (e.g. additional CAs in a bundle) are
// ignored — the leaf's metadata is what we surface to operators.
func parseLeafCertificate(pemData []byte) (*x509.Certificate, error) {
	rest := pemData
	for {
		block, remainder := pem.Decode(rest)
		if block == nil {
			return nil, ErrCertPEMInvalid
		}
		if block.Type == "CERTIFICATE" {
			cert, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				return nil, fmt.Errorf("%w: %v", ErrCertPEMInvalid, err)
			}
			return cert, nil
		}
		rest = remainder
	}
}
